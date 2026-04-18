package controllerrefresh

import "testing"

func TestNewRedisHandoffUsesDefaultPrefix(t *testing.T) {
	handoff := NewRedisHandoff(nil, "")
	if got := handoff.channel(); got != "tenet:status:controller-refresh" {
		t.Fatalf("unexpected default controller refresh channel: %q", got)
	}
}

func TestNewRedisHandoffUsesCustomPrefix(t *testing.T) {
	handoff := NewRedisHandoff(nil, "tenet:test")
	if got := handoff.channel(); got != "tenet:test:controller-refresh" {
		t.Fatalf("unexpected custom controller refresh channel: %q", got)
	}
}

func TestRequestValidate(t *testing.T) {
	valid := Request{
		RequestID:   "github-webhook:delivery-1:after",
		DeliveryID:  "delivery-1",
		Repository:  "acme/platform-config",
		Ref:         "refs/heads/main",
		RequestedAt: "2026-04-19T09:30:00Z",
	}
	if err := valid.validate(); err != nil {
		t.Fatalf("expected valid request, got %v", err)
	}

	tests := []struct {
		name    string
		request Request
	}{
		{name: "missing request id", request: Request{DeliveryID: "delivery-1", Repository: "acme/platform-config", Ref: "refs/heads/main", RequestedAt: "2026-04-19T09:30:00Z"}},
		{name: "missing delivery id", request: Request{RequestID: "github-webhook:delivery-1:after", Repository: "acme/platform-config", Ref: "refs/heads/main", RequestedAt: "2026-04-19T09:30:00Z"}},
		{name: "missing repository", request: Request{RequestID: "github-webhook:delivery-1:after", DeliveryID: "delivery-1", Ref: "refs/heads/main", RequestedAt: "2026-04-19T09:30:00Z"}},
		{name: "missing ref", request: Request{RequestID: "github-webhook:delivery-1:after", DeliveryID: "delivery-1", Repository: "acme/platform-config", RequestedAt: "2026-04-19T09:30:00Z"}},
		{name: "missing requested at", request: Request{RequestID: "github-webhook:delivery-1:after", DeliveryID: "delivery-1", Repository: "acme/platform-config", Ref: "refs/heads/main"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.request.validate(); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}
