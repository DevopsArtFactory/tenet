package ecsexecution

import (
	"context"
	"errors"
	"fmt"
	"github.com/DevopsArtFactory/tenet/internal/applyrequest"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
)

func TestDeployerClientAssumesConfiguredRoleInSameAccount(t *testing.T) {
	_, _, request := registrationFixture()
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if err := r.ParseForm(); err != nil {
			t.Error(err)
		}
		if r.Form.Get("Action") != "AssumeRole" || r.Form.Get("RoleArn") != request.Role.RoleARN {
			t.Errorf("wrong STS identity request: %v", r.Form)
		}
		w.Header().Set("Content-Type", "text/xml")
		fmt.Fprint(w, `<AssumeRoleResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/"><AssumeRoleResult><Credentials><AccessKeyId>assumed-access-key</AccessKeyId><SecretAccessKey>fixture-secret</SecretAccessKey><SessionToken>fixture-token</SessionToken><Expiration>2099-01-01T00:00:00Z</Expiration></Credentials><AssumedRoleUser><AssumedRoleId>fixture:tenet</AssumedRoleId><Arn>arn:aws:sts::123456789012:assumed-role/tenet-deployer/tenet</Arn></AssumedRoleUser></AssumeRoleResult></AssumeRoleResponse>`)
	}))
	defer server.Close()
	t.Setenv("AWS_ACCESS_KEY_ID", "source-access-key")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "source-fixture-secret")
	t.Setenv("AWS_SESSION_TOKEN", "")
	t.Setenv("AWS_PROFILE", "")
	t.Setenv("AWS_CONFIG_FILE", filepath.Join(t.TempDir(), "absent-config"))
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", filepath.Join(t.TempDir(), "absent-credentials"))
	t.Setenv("AWS_ENDPOINT_URL_STS", server.URL)
	client, err := DeployerClient(context.Background(), request.Role)
	if err != nil {
		t.Fatal(err)
	}
	actual := client
	creds, err := actual.Options().Credentials.Retrieve(context.Background())
	if err != nil || creds.AccessKeyID != "assumed-access-key" || calls.Load() != 1 {
		t.Fatalf("client reused source credentials instead of assuming role: %+v %v", creds, err)
	}
	albClient, err := DeployerALBClient(context.Background(), request.Role)
	if err != nil {
		t.Fatal(err)
	}
	albCreds, err := albClient.Options().Credentials.Retrieve(context.Background())
	if err != nil || albCreds.AccessKeyID != "assumed-access-key" || calls.Load() != 2 {
		t.Fatal("ALB preflight reused source credentials instead of assuming deployer role")
	}
}

type httpRegistration struct {
	*registrationFake
	live *ecs.Client
}

func (f httpRegistration) RegisterTaskDefinition(ctx context.Context, input *ecs.RegisterTaskDefinitionInput, options ...func(*ecs.Options)) (*ecs.RegisterTaskDefinitionOutput, error) {
	return f.live.RegisterTaskDefinition(ctx, input, options...)
}

func TestRegistrationDoesNotLetSDKRetryAmbiguousServerResponse(t *testing.T) {
	fake, handler, request := registrationFixture()
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/x-amz-json-1.1")
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `{"__type":"ServerException","message":"response outcome unknown"}`)
	}))
	defer server.Close()
	client := ecs.NewFromConfig(aws.Config{Region: request.Role.Region, Credentials: credentials.NewStaticCredentialsProvider("fixture-key", "fixture-secret", "")}, func(options *ecs.Options) { options.BaseEndpoint = aws.String(server.URL) })
	handler.ClientFor = func(context.Context, applyrequest.DeployerRoleReadiness) (RegistrationClient, error) {
		return httpRegistration{registrationFake: fake, live: client}, nil
	}
	_, err := handler.Execute(context.Background(), request)
	if err == nil || calls.Load() != 1 {
		t.Fatalf("SDK replayed non-idempotent registration: calls=%d err=%v", calls.Load(), err)
	}
	var classified *applyrequest.ECSCallError
	if errors.As(err, &classified) && classified.Rejected {
		t.Fatal("ambiguous server response was marked rejected")
	}
}
