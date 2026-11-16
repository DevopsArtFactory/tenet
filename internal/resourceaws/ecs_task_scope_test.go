package resourceaws

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
)

func TestAuthorizeECSTaskChecksLiveServiceMembership(t *testing.T) {
	scope := ECSTaskScope{AccountID: "111111111111", Region: "ap-northeast-2", ClusterARN: "arn:aws:ecs:ap-northeast-2:111111111111:cluster/shared", ServiceARN: "arn:aws:ecs:ap-northeast-2:111111111111:service/shared/allowed", TaskARN: "arn:aws:ecs:ap-northeast-2:111111111111:task/shared/01234567890123456789012345678901"}
	for _, tc := range []struct {
		name    string
		change  func(map[string]any)
		allowed bool
	}{
		{"owned running", func(map[string]any) {}, true},
		{"owned stopped", func(m map[string]any) { m["lastStatus"] = "STOPPED" }, true},
		{"other service", func(m map[string]any) { m["group"] = "service:private" }, false},
		{"standalone task", func(m map[string]any) { m["group"] = "family:allowed" }, false},
		{"missing group", func(m map[string]any) { delete(m, "group") }, false},
		{"other cluster", func(m map[string]any) { m["clusterArn"] = strings.Replace(scope.ClusterARN, "shared", "other", 1) }, false},
		{"other task", func(m map[string]any) { m["taskArn"] = scope.TaskARN + "1" }, false},
		{"missing", nil, false},
		{"failure", nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				if !strings.HasSuffix(r.Header.Get("X-Amz-Target"), ".DescribeTasks") {
					t.Error("unexpected AWS operation")
				}
				var input struct {
					Cluster string
					Tasks   []string
				}
				if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
					t.Error(err)
				}
				if input.Cluster != scope.ClusterARN || len(input.Tasks) != 1 || input.Tasks[0] != scope.TaskARN {
					t.Errorf("unbound lookup: %+v", input)
				}
				task := map[string]any{"taskArn": scope.TaskARN, "clusterArn": scope.ClusterARN, "group": "service:allowed", "lastStatus": "RUNNING"}
				response := map[string]any{"tasks": []any{task}}
				if tc.change != nil {
					tc.change(task)
				} else {
					response["tasks"] = []any{}
				}
				if tc.name == "failure" {
					response["failures"] = []any{map[string]any{"arn": scope.TaskARN, "reason": "MISSING"}}
				}
				w.Header().Set("Content-Type", "application/x-amz-json-1.1")
				_ = json.NewEncoder(w).Encode(response)
			}))
			defer server.Close()
			deps := DefaultClientCacheDeps()
			deps.LoadConfig = func(context.Context, string, string, string) (aws.Config, error) {
				return aws.Config{Region: scope.Region, Credentials: credentials.NewStaticCredentialsProvider("fixture", "fixture", ""), HTTPClient: server.Client(), RetryMaxAttempts: 1}, nil
			}
			deps.NewECSClient = func(cfg aws.Config) *ecs.Client {
				return ecs.NewFromConfig(cfg, func(o *ecs.Options) { o.BaseEndpoint = aws.String(server.URL) })
			}
			err := NewClientCache(deps).AuthorizeECSTask(t.Context(), scope)
			if tc.allowed && err != nil {
				t.Fatal(err)
			}
			if !tc.allowed && !errors.Is(err, ErrECSTaskScope) {
				t.Fatalf("expected scope denial, got %v", err)
			}
			if requests.Load() != 1 {
				t.Fatalf("AWS requests: %d", requests.Load())
			}
		})
	}
}

func TestAuthorizeECSTaskRejectsInvalidScopeBeforeCredentials(t *testing.T) {
	original := ECSTaskScope{AccountID: "111111111111", Region: "ap-northeast-2", ClusterARN: "arn:aws:ecs:ap-northeast-2:111111111111:cluster/allowed", ServiceARN: "arn:aws:ecs:ap-northeast-2:111111111111:service/allowed/api", TaskARN: "arn:aws:ecs:ap-northeast-2:111111111111:task/allowed/01234567890123456789012345678901"}
	for _, tc := range []struct {
		name   string
		change func(*ECSTaskScope)
	}{
		{"missing service", func(s *ECSTaskScope) { s.ServiceARN = "" }},
		{"missing cluster", func(s *ECSTaskScope) { s.ClusterARN = "" }},
		{"bare task id", func(s *ECSTaskScope) { s.TaskARN = "01234567890123456789012345678901" }},
		{"foreign task cluster", func(s *ECSTaskScope) { s.TaskARN = strings.Replace(s.TaskARN, "/allowed/", "/private/", 1) }},
		{"foreign service cluster", func(s *ECSTaskScope) { s.ServiceARN = strings.Replace(s.ServiceARN, "/allowed/", "/private/", 1) }},
		{"foreign task account", func(s *ECSTaskScope) { s.TaskARN = strings.Replace(s.TaskARN, s.AccountID, "222222222222", 1) }},
		{"foreign service region", func(s *ECSTaskScope) { s.ServiceARN = strings.Replace(s.ServiceARN, s.Region, "us-east-1", 1) }},
		{"foreign partition", func(s *ECSTaskScope) { s.TaskARN = strings.Replace(s.TaskARN, "arn:aws:", "arn:aws-cn:", 1) }},
		{"foreign target account", func(s *ECSTaskScope) { s.AccountID = "222222222222" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			scope := original
			tc.change(&scope)
			deps := DefaultClientCacheDeps()
			deps.LoadConfig = func(context.Context, string, string, string) (aws.Config, error) {
				t.Fatal("invalid scope reached credential loading")
				return aws.Config{}, nil
			}
			if err := NewClientCache(deps).AuthorizeECSTask(t.Context(), scope); !errors.Is(err, ErrECSTaskScope) {
				t.Fatalf("expected scope denial, got %v", err)
			}
		})
	}
}
