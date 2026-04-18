package githubwebhook

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
	"testing"
)

func TestDecodeAcceptedPushEvent(t *testing.T) {
	const secret = "top-secret"
	body := `{
		"ref":"refs/heads/main",
		"before":"1111111111111111111111111111111111111111",
		"after":"2222222222222222222222222222222222222222",
		"repository":{"full_name":"acme/platform-config"},
		"head_commit":{
			"id":"2222222222222222222222222222222222222222",
			"message":"update deployment",
			"timestamp":"2026-04-18T15:00:00Z"
		},
		"commits":[
			{"id":"a","added":["deployments/api.yaml"],"modified":["environments/prod.yaml"],"removed":[]},
			{"id":"b","added":["deployments/api.yaml","accounts/prod.yaml"],"modified":["environments/prod.yaml","deployments/worker.yaml"],"removed":["deployments/old.yaml"]}
		]
	}`

	response, err := Decode(context.Background(), Request{
		Secret: secret,
		Header: signedHeader(secret, "push", body),
		Body:   strings.NewReader(body),
	})
	if err != nil {
		t.Fatalf("Decode returned error: %v", err)
	}

	if response.Status != StatusAccepted || response.Processed {
		t.Fatalf("expected accepted unprocessed response, got %+v", response)
	}
	if response.Reason != "github push event validated, but reconcile wiring is not implemented in this slice" {
		t.Fatalf("unexpected reason: %q", response.Reason)
	}
	if response.Repository != "acme/platform-config" || response.Ref != "refs/heads/main" {
		t.Fatalf("unexpected repository/ref: %+v", response)
	}
	if response.Before != "1111111111111111111111111111111111111111" || response.After != "2222222222222222222222222222222222222222" {
		t.Fatalf("unexpected before/after: %+v", response)
	}
	if response.HeadCommit == nil || response.HeadCommit.ID != "2222222222222222222222222222222222222222" {
		t.Fatalf("expected head commit, got %+v", response.HeadCommit)
	}
	if got, want := response.ChangedFiles.Added, []string{"deployments/api.yaml", "accounts/prod.yaml"}; !equalStrings(got, want) {
		t.Fatalf("unexpected added files: got %v want %v", got, want)
	}
	if got, want := response.ChangedFiles.Modified, []string{"environments/prod.yaml", "deployments/worker.yaml"}; !equalStrings(got, want) {
		t.Fatalf("unexpected modified files: got %v want %v", got, want)
	}
	if got, want := response.ChangedFiles.Removed, []string{"deployments/old.yaml"}; !equalStrings(got, want) {
		t.Fatalf("unexpected removed files: got %v want %v", got, want)
	}
	if got, want := response.ChangedFiles.All, []string{"deployments/api.yaml", "accounts/prod.yaml", "environments/prod.yaml", "deployments/worker.yaml", "deployments/old.yaml"}; !equalStrings(got, want) {
		t.Fatalf("unexpected all files: got %v want %v", got, want)
	}
}

func TestDecodeIgnoredNonPushEvent(t *testing.T) {
	const secret = "top-secret"
	body := `{"zen":"keep it logically awesome"}`

	response, err := Decode(context.Background(), Request{
		Secret: secret,
		Header: signedHeader(secret, "ping", body),
		Body:   strings.NewReader(body),
	})
	if err != nil {
		t.Fatalf("Decode returned error: %v", err)
	}

	if response.Status != StatusIgnored || response.Processed {
		t.Fatalf("expected ignored unprocessed response, got %+v", response)
	}
	if response.Reason != `github event "ping" is ignored because reconcile wiring is not implemented in this slice` {
		t.Fatalf("unexpected ignored reason: %q", response.Reason)
	}
	if response.Repository != "" || response.ChangedFiles != nil {
		t.Fatalf("expected no push summary for ignored event, got %+v", response)
	}
}

func TestDecodeValidationFailures(t *testing.T) {
	const secret = "top-secret"
	validBody := `{"repository":{"full_name":"acme/platform-config"},"ref":"refs/heads/main","before":"111","after":"222","commits":[]}`

	tests := []struct {
		name    string
		req     Request
		wantErr string
	}{
		{
			name: "missing delivery header",
			req: Request{
				Secret: secret,
				Header: mapHeader(HeaderEvent, "push", HeaderSignature, signature(secret, validBody)),
				Body:   strings.NewReader(validBody),
			},
			wantErr: HeaderDeliveryID + " header is required",
		},
		{
			name: "missing event header",
			req: Request{
				Secret: secret,
				Header: mapHeader(HeaderDeliveryID, "delivery-1", HeaderSignature, signature(secret, validBody)),
				Body:   strings.NewReader(validBody),
			},
			wantErr: HeaderEvent + " header is required",
		},
		{
			name: "missing signature header",
			req: Request{
				Secret: secret,
				Header: mapHeader(HeaderDeliveryID, "delivery-1", HeaderEvent, "push"),
				Body:   strings.NewReader(validBody),
			},
			wantErr: HeaderSignature + " header is required",
		},
		{
			name: "bad signature prefix",
			req: Request{
				Secret: secret,
				Header: mapHeader(HeaderDeliveryID, "delivery-1", HeaderEvent, "push", HeaderSignature, "sha1=abc"),
				Body:   strings.NewReader(validBody),
			},
			wantErr: HeaderSignature + " header must use sha256",
		},
		{
			name: "signature mismatch",
			req: Request{
				Secret: secret,
				Header: mapHeader(HeaderDeliveryID, "delivery-1", HeaderEvent, "push", HeaderSignature, signature("wrong-secret", validBody)),
				Body:   strings.NewReader(validBody),
			},
			wantErr: "github webhook signature mismatch",
		},
		{
			name: "invalid json",
			req: Request{
				Secret: secret,
				Header: signedHeader(secret, "push", `{"repository":`),
				Body:   strings.NewReader(`{"repository":`),
			},
			wantErr: "invalid webhook JSON body:",
		},
		{
			name: "multiple json values",
			req: Request{
				Secret: secret,
				Header: signedHeader(secret, "push", `{} {}`),
				Body:   strings.NewReader(`{} {}`),
			},
			wantErr: "invalid webhook JSON body: multiple JSON values are not allowed",
		},
		{
			name: "missing repository",
			req: Request{
				Secret: secret,
				Header: signedHeader(secret, "push", `{"ref":"refs/heads/main","before":"111","after":"222","commits":[]}`),
				Body:   strings.NewReader(`{"ref":"refs/heads/main","before":"111","after":"222","commits":[]}`),
			},
			wantErr: "push payload repository.full_name is required",
		},
		{
			name: "empty secret",
			req: Request{
				Header: signedHeader(secret, "push", validBody),
				Body:   strings.NewReader(validBody),
			},
			wantErr: "webhook secret is required",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Decode(context.Background(), tc.req)
			if err == nil {
				t.Fatalf("expected error %q", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tc.wantErr, err)
			}
		})
	}
}

func TestDecodeReturnsContextError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := Decode(ctx, Request{
		Secret: "top-secret",
		Header: signedHeader("top-secret", "push", `{}`),
		Body:   strings.NewReader(`{}`),
	})
	if err == nil {
		t.Fatal("expected context cancellation error")
	}
	if err != context.Canceled {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func signedHeader(secret, event, body string) http.Header {
	return mapHeader(
		HeaderDeliveryID, "delivery-1",
		HeaderEvent, event,
		HeaderSignature, signature(secret, body),
	)
}

func mapHeader(values ...string) http.Header {
	header := http.Header{}
	for i := 0; i < len(values); i += 2 {
		header.Set(values[i], values[i+1])
	}
	return header
}

func signature(secret, body string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(body))
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func equalStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for index := range want {
		if got[index] != want[index] {
			return false
		}
	}
	return true
}
