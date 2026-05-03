package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/DevopsArtFactory/tenet/internal/api"
	"github.com/DevopsArtFactory/tenet/internal/applyrequest"
	"github.com/DevopsArtFactory/tenet/internal/auditlog"
	"github.com/DevopsArtFactory/tenet/internal/buildinfo"
	"github.com/DevopsArtFactory/tenet/internal/catalog"
	"github.com/DevopsArtFactory/tenet/internal/controllerrefresh"
	"github.com/DevopsArtFactory/tenet/internal/domain"
	"github.com/DevopsArtFactory/tenet/internal/githubcontents"
	"github.com/DevopsArtFactory/tenet/internal/githubwebhook"
	"github.com/DevopsArtFactory/tenet/internal/planpreview"
	"github.com/DevopsArtFactory/tenet/internal/requestactor"
	"github.com/DevopsArtFactory/tenet/internal/resourceinspection"
	"github.com/DevopsArtFactory/tenet/internal/resourcelogs"
	"github.com/DevopsArtFactory/tenet/internal/statusstore"
	"github.com/redis/go-redis/v9"
)

func TestStartApplyRequestExecutor(t *testing.T) {
	t.Run("does not start when disabled or repo root missing", func(t *testing.T) {
		logger := slogDiscardLogger()

		if cancel, started := startApplyRequestExecutor(logger, "", true, filepath.Join(t.TempDir(), "apply.json"), time.Millisecond); started || cancel != nil {
			t.Fatalf("expected executor not to start without repo root")
		}
		if cancel, started := startApplyRequestExecutor(logger, t.TempDir(), false, filepath.Join(t.TempDir(), "apply.json"), time.Millisecond); started || cancel != nil {
			t.Fatalf("expected executor not to start when apply is disabled")
		}
	})

	t.Run("enabled startup advances queued apply request from store", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "apply-requests.json")
		store := applyrequest.NewFileStore(path)
		record := applyrequest.Record{
			ID:             "apply_queued",
			PlanID:         "plan_ready_1234abcd",
			DeploymentName: "api-prod",
			Status:         applyrequest.StatusQueued,
			CreatedAt:      "2026-04-18T08:05:00Z",
			Lifecycle: applyrequest.Lifecycle{
				UpdatedAt:  "2026-04-18T08:05:00Z",
				TotalSteps: 1,
			},
			Operations: []applyrequest.Operation{
				{Status: applyrequest.OperationStatusQueued, Phase: "Provisioning", Target: "api-prod deployment", Summary: "Create replacement capacity for api-prod."},
			},
		}
		if err := store.Save(context.Background(), record); err != nil {
			t.Fatalf("Save returned error: %v", err)
		}

		cancel, started := startApplyRequestExecutor(slogDiscardLogger(), t.TempDir(), true, path, 10*time.Millisecond)
		if !started || cancel == nil {
			t.Fatalf("expected executor to start")
		}
		defer cancel()

		deadline := time.Now().Add(2 * time.Second)
		for {
			got, err := store.Get(context.Background(), record.ID)
			if err != nil {
				t.Fatalf("Get returned error: %v", err)
			}
			if got.Status == applyrequest.StatusFailed {
				if got.ID != record.ID || got.Error == "" || got.Lifecycle.StartedAt == "" || got.Lifecycle.FinishedAt == "" {
					t.Fatalf("unexpected auto-advanced apply request: %+v", got)
				}
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("timed out waiting for executor handoff, last record=%+v", got)
			}
			time.Sleep(10 * time.Millisecond)
		}
	})
}

func slogDiscardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestAPIOptionsOverviewDeploymentUsesNamedSnapshotLookup(t *testing.T) {
	repoRoot := writeRepositoryFixture(t)
	store := &stubStatusStore{
		getSnapshot: statusstore.Snapshot{
			DeploymentName: "api-prod",
			Status: domain.DeploymentStatus{
				ObservedGeneration: 7,
				Sync:               domain.Sync{State: domain.SyncStateOutOfSync, Reason: "PendingPromotion"},
				Health:             domain.Health{Overall: domain.HealthProgressing, Summary: "warming up"},
				DeployedArtifact:   domain.DeploymentArtifact{Image: "payments-api:v1"},
			},
		},
	}

	options := newAPIOptions(buildinfo.Info{Version: "dev", Commit: "abc123"}, repoRoot, store)
	if options.OverviewDeployment == nil {
		t.Fatalf("expected overview deployment provider")
	}

	detail, err := options.OverviewDeployment(context.Background(), "api-prod")
	if err != nil {
		t.Fatalf("OverviewDeployment returned error: %v", err)
	}
	if store.getCalls != 1 {
		t.Fatalf("expected one Get call, got %d", store.getCalls)
	}
	if store.gotGetName != "api-prod" {
		t.Fatalf("expected Get lookup for api-prod, got %q", store.gotGetName)
	}
	if store.listCalls != 0 {
		t.Fatalf("expected no List calls, got %d", store.listCalls)
	}
	if detail.Name != "api-prod" || detail.DeployedArtifact.Image != "payments-api:v1" || detail.AccountName != "prod-core" {
		t.Fatalf("unexpected deployment detail: %+v", detail)
	}
}

func TestRequestTimeoutFromEnv(t *testing.T) {
	t.Run("defaults to thirty seconds", func(t *testing.T) {
		t.Setenv("TENET_API_REQUEST_TIMEOUT", "")

		timeout, err := requestTimeoutFromEnv()
		if err != nil {
			t.Fatalf("requestTimeoutFromEnv returned error: %v", err)
		}
		if timeout != 30*time.Second {
			t.Fatalf("expected 30s default, got %s", timeout)
		}
	})

	t.Run("accepts explicit disable", func(t *testing.T) {
		t.Setenv("TENET_API_REQUEST_TIMEOUT", "0")

		timeout, err := requestTimeoutFromEnv()
		if err != nil {
			t.Fatalf("requestTimeoutFromEnv returned error: %v", err)
		}
		if timeout != 0 {
			t.Fatalf("expected disabled timeout, got %s", timeout)
		}
	})

	t.Run("parses durations", func(t *testing.T) {
		t.Setenv("TENET_API_REQUEST_TIMEOUT", "45s")

		timeout, err := requestTimeoutFromEnv()
		if err != nil {
			t.Fatalf("requestTimeoutFromEnv returned error: %v", err)
		}
		if timeout != 45*time.Second {
			t.Fatalf("expected 45s timeout, got %s", timeout)
		}
	})

	t.Run("rejects invalid durations", func(t *testing.T) {
		t.Setenv("TENET_API_REQUEST_TIMEOUT", "not-a-duration")

		if _, err := requestTimeoutFromEnv(); err == nil {
			t.Fatalf("expected invalid duration error")
		}
	})

	t.Run("rejects negative durations", func(t *testing.T) {
		t.Setenv("TENET_API_REQUEST_TIMEOUT", "-1s")

		if _, err := requestTimeoutFromEnv(); err == nil {
			t.Fatalf("expected negative duration error")
		}
	})
}

func TestTrustedAuthenticatedHeadersEnabled(t *testing.T) {
	t.Run("defaults to false", func(t *testing.T) {
		t.Setenv("TENET_API_TRUST_AUTHENTICATED_HEADERS", "")

		if trustedAuthenticatedHeadersEnabled() {
			t.Fatal("expected trusted authenticated headers to be disabled by default")
		}
	})

	t.Run("accepts true", func(t *testing.T) {
		t.Setenv("TENET_API_TRUST_AUTHENTICATED_HEADERS", "true")

		if !trustedAuthenticatedHeadersEnabled() {
			t.Fatal("expected trusted authenticated headers to be enabled")
		}
	})
}

func TestOperatorWriteAuthenticationEnforced(t *testing.T) {
	t.Run("defaults to false", func(t *testing.T) {
		t.Setenv("TENET_API_ENFORCE_AUTHENTICATED_IDENTITY", "")

		if operatorWriteAuthenticationEnforced() {
			t.Fatal("expected operator write authentication to be disabled by default")
		}
	})

	t.Run("accepts true", func(t *testing.T) {
		t.Setenv("TENET_API_ENFORCE_AUTHENTICATED_IDENTITY", "true")

		if !operatorWriteAuthenticationEnforced() {
			t.Fatal("expected operator write authentication to be enabled")
		}
	})
}

func TestOperatorWriteRoleAuthorizationEnforced(t *testing.T) {
	t.Run("defaults to false", func(t *testing.T) {
		t.Setenv("TENET_API_ENFORCE_OPERATOR_ROLE_AUTHORIZATION", "")

		if operatorWriteRoleAuthorizationEnforced() {
			t.Fatal("expected operator write role authorization to be disabled by default")
		}
	})

	t.Run("accepts true", func(t *testing.T) {
		t.Setenv("TENET_API_ENFORCE_OPERATOR_ROLE_AUTHORIZATION", "true")

		if !operatorWriteRoleAuthorizationEnforced() {
			t.Fatal("expected operator write role authorization to be enabled")
		}
	})
}

func TestOperatorDeploymentReadAuthorizationEnforced(t *testing.T) {
	t.Run("defaults to false", func(t *testing.T) {
		t.Setenv("TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION", "")

		if operatorDeploymentReadAuthorizationEnforced() {
			t.Fatal("expected deployment read authorization to be disabled by default")
		}
	})

	t.Run("accepts true", func(t *testing.T) {
		t.Setenv("TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION", "true")

		if !operatorDeploymentReadAuthorizationEnforced() {
			t.Fatal("expected deployment read authorization to be enabled")
		}
	})
}

func TestAuthSessionCookieSecureEnabled(t *testing.T) {
	t.Run("defaults to false", func(t *testing.T) {
		t.Setenv("TENET_AUTH_SESSION_COOKIE_SECURE", "")
		if authSessionCookieSecureEnabled() {
			t.Fatal("expected secure session cookie override to be disabled by default")
		}
	})

	t.Run("accepts true", func(t *testing.T) {
		t.Setenv("TENET_AUTH_SESSION_COOKIE_SECURE", "true")
		if !authSessionCookieSecureEnabled() {
			t.Fatal("expected secure session cookie override to be enabled")
		}
	})
}

func TestNewAPIOptionsIncludesGitHubWebhookSecretFromEnv(t *testing.T) {
	t.Setenv("TENET_GITHUB_WEBHOOK_SECRET", "top-secret")

	options := newAPIOptions(buildinfo.Info{Version: "dev", Commit: "abc123"}, writeRepositoryFixture(t), nil)

	if options.GitHubWebhookSecret != "top-secret" {
		t.Fatalf("expected webhook secret from env, got %q", options.GitHubWebhookSecret)
	}
	if options.TrustAuthenticatedHeaders {
		t.Fatalf("expected trusted authenticated headers to default off, got %+v", options)
	}
	if options.RequireAuthenticatedIdentity {
		t.Fatalf("expected operator write authentication to default off, got %+v", options)
	}
	if options.RequireDeploymentReadAuthorization {
		t.Fatalf("expected deployment read authorization to default off, got %+v", options)
	}
	if options.GitHubWebhookDesiredStateSource.Kind != githubwebhook.DesiredStateSourceKindFilesystem {
		t.Fatalf("expected filesystem desired-state source for webhook matching, got %+v", options.GitHubWebhookDesiredStateSource)
	}
}

func TestNewAPIOptionsIncludesManagedLoginRuntimeDiagnosticFromEnv(t *testing.T) {
	t.Setenv("TENET_AUTH_PROVIDER", "github")
	t.Setenv("TENET_GITHUB_OAUTH_CLIENT_ID", "client-id")
	t.Setenv("TENET_GITHUB_OAUTH_CLIENT_SECRET", "client-secret")
	t.Setenv("TENET_GITHUB_OAUTH_REDIRECT_URL", "https://tenet.example.com/auth/callback/github")
	t.Setenv("TENET_AUTH_STATE_STORE_PATH", filepath.Join(t.TempDir(), "oauth-state.json"))
	t.Setenv("TENET_AUTH_IDENTITY_STORE_PATH", filepath.Join(t.TempDir(), "oauth-identities.json"))
	t.Setenv("TENET_AUTH_SESSION_SECRET", "session-secret")

	options := newAPIOptions(buildinfo.Info{Version: "dev", Commit: "abc123"}, writeRepositoryFixture(t), nil)
	managedLogin := options.AuthRuntime.TenetManagedLogin

	if !managedLogin.Configured || !managedLogin.Active {
		t.Fatalf("expected configured and active managed-login diagnostic, got %+v", managedLogin)
	}
	if managedLogin.Provider != "github" || managedLogin.LoginRoute != "/auth/login/github" || managedLogin.CallbackRoute != "/auth/callback/github" {
		t.Fatalf("unexpected managed-login route/provider diagnostic: %+v", managedLogin)
	}
	if managedLogin.SessionCookieName != "tenet_session" || !managedLogin.StateStoreConfigured || !managedLogin.IdentityStoreConfigured || !managedLogin.SessionSecretConfigured {
		t.Fatalf("unexpected managed-login store/session diagnostic: %+v", managedLogin)
	}
	if options.AuthSessionCookieSecure {
		t.Fatalf("expected managed-login session cookie secure override to default off for local HTTP development")
	}
	if !reflect.DeepEqual(managedLogin.RequiredSettings, expectedManagedLoginRequiredSettings()) || len(managedLogin.MissingSettings) != 0 {
		t.Fatalf("unexpected managed-login required/missing settings: %+v", managedLogin)
	}
	assertManagedLoginEnvLifecycleDiagnostic(t, managedLogin, true)
}

func TestNewAPIOptionsCanForceManagedLoginSecureSessionCookie(t *testing.T) {
	t.Setenv("TENET_AUTH_PROVIDER", "github")
	t.Setenv("TENET_GITHUB_OAUTH_CLIENT_ID", "client-id")
	t.Setenv("TENET_GITHUB_OAUTH_CLIENT_SECRET", "client-secret")
	t.Setenv("TENET_GITHUB_OAUTH_REDIRECT_URL", "https://tenet.example.com/auth/callback/github")
	t.Setenv("TENET_AUTH_STATE_STORE_PATH", filepath.Join(t.TempDir(), "oauth-state.json"))
	t.Setenv("TENET_AUTH_IDENTITY_STORE_PATH", filepath.Join(t.TempDir(), "oauth-identities.json"))
	t.Setenv("TENET_AUTH_SESSION_SECRET", "session-secret")
	t.Setenv("TENET_AUTH_SESSION_COOKIE_SECURE", "true")

	options := newAPIOptions(buildinfo.Info{Version: "dev", Commit: "abc123"}, writeRepositoryFixture(t), nil)
	if !options.AuthRuntime.TenetManagedLogin.Active {
		t.Fatalf("expected active managed-login runtime, got %+v", options.AuthRuntime.TenetManagedLogin)
	}
	if !options.AuthSessionCookieSecure {
		t.Fatalf("expected TENET_AUTH_SESSION_COOKIE_SECURE=true to force secure managed-login cookies")
	}
}

func TestNewAPIOptionsReportsManagedLoginRuntimeMissingSettings(t *testing.T) {
	t.Setenv("TENET_AUTH_PROVIDER", "github")
	t.Setenv("TENET_GITHUB_OAUTH_CLIENT_ID", "client-id")
	t.Setenv("TENET_AUTH_SESSION_SECRET", "session-secret")

	options := newAPIOptions(buildinfo.Info{Version: "dev", Commit: "abc123"}, writeRepositoryFixture(t), nil)
	managedLogin := options.AuthRuntime.TenetManagedLogin

	if managedLogin.Configured || managedLogin.Active || managedLogin.Provider != "github" {
		t.Fatalf("expected disabled partial managed-login diagnostic, got %+v", managedLogin)
	}
	expectedMissing := []string{
		"TENET_GITHUB_OAUTH_CLIENT_SECRET",
		"TENET_GITHUB_OAUTH_REDIRECT_URL",
		"TENET_AUTH_STATE_STORE_PATH",
		"TENET_AUTH_IDENTITY_STORE_PATH",
	}
	if !reflect.DeepEqual(managedLogin.RequiredSettings, expectedManagedLoginRequiredSettings()) || !reflect.DeepEqual(managedLogin.MissingSettings, expectedMissing) {
		t.Fatalf("unexpected managed-login required/missing settings: %+v", managedLogin)
	}
	if !managedLogin.SessionSecretConfigured || managedLogin.StateStoreConfigured || managedLogin.IdentityStoreConfigured {
		t.Fatalf("unexpected managed-login store/session booleans for partial config: %+v", managedLogin)
	}
	assertManagedLoginEnvLifecycleDiagnostic(t, managedLogin, false)
}

func TestNewAPIOptionsReportsUnsupportedManagedLoginProviderMissingProviderSpecificSettings(t *testing.T) {
	t.Setenv("TENET_AUTH_PROVIDER", "gitlab")
	t.Setenv("TENET_GITHUB_OAUTH_CLIENT_ID", "client-id")
	t.Setenv("TENET_GITHUB_OAUTH_CLIENT_SECRET", "client-secret")
	t.Setenv("TENET_GITHUB_OAUTH_REDIRECT_URL", "https://tenet.example.com/auth/callback/github")
	t.Setenv("TENET_AUTH_STATE_STORE_PATH", filepath.Join(t.TempDir(), "oauth-state.json"))
	t.Setenv("TENET_AUTH_IDENTITY_STORE_PATH", filepath.Join(t.TempDir(), "oauth-identities.json"))
	t.Setenv("TENET_AUTH_SESSION_SECRET", "session-secret")

	options := newAPIOptions(buildinfo.Info{Version: "dev", Commit: "abc123"}, writeRepositoryFixture(t), nil)
	managedLogin := options.AuthRuntime.TenetManagedLogin

	if managedLogin.Configured || managedLogin.Active || managedLogin.Provider != "gitlab" {
		t.Fatalf("expected disabled unsupported-provider managed-login diagnostic, got %+v", managedLogin)
	}
	expectedMissing := expectedManagedLoginRequiredSettings()[1:]
	if !reflect.DeepEqual(managedLogin.MissingSettings, expectedMissing) {
		t.Fatalf("expected provider-specific settings to be reported missing for unsupported provider, got %+v", managedLogin.MissingSettings)
	}
}

func TestNewAPIOptionsLeavesManagedLoginRuntimeDiagnosticDisabledByDefault(t *testing.T) {
	options := newAPIOptions(buildinfo.Info{Version: "dev", Commit: "abc123"}, writeRepositoryFixture(t), nil)
	managedLogin := options.AuthRuntime.TenetManagedLogin

	if managedLogin.Configured || managedLogin.Active || managedLogin.Provider != "" {
		t.Fatalf("expected disabled unconfigured managed-login diagnostic, got %+v", managedLogin)
	}
	if !reflect.DeepEqual(managedLogin.RequiredSettings, expectedManagedLoginRequiredSettings()) || !reflect.DeepEqual(managedLogin.MissingSettings, expectedManagedLoginRequiredSettings()) {
		t.Fatalf("unexpected managed-login required/missing settings by default: %+v", managedLogin)
	}
	assertManagedLoginEnvLifecycleDiagnostic(t, managedLogin, false)
}

func assertManagedLoginEnvLifecycleDiagnostic(t *testing.T, managedLogin api.TenetManagedLoginDiagnostic, expectLogout bool) {
	t.Helper()
	if managedLogin.OAuthStateTTLSeconds != api.DefaultTenetManagedLoginOAuthStateTTLSeconds() {
		t.Fatalf("unexpected managed-login OAuth state TTL diagnostic: %+v", managedLogin)
	}
	if managedLogin.SessionTTLSeconds != api.DefaultTenetManagedLoginSessionTTLSeconds() || !managedLogin.SessionExpiryEnforced {
		t.Fatalf("unexpected managed-login session expiry diagnostic: %+v", managedLogin)
	}
	if managedLogin.ProviderAccessTokenStored || managedLogin.ProviderAccessTokenReturned || managedLogin.ProviderTokenRefreshSupported || managedLogin.ProviderTokenRevocationSupported {
		t.Fatalf("managed-login provider token diagnostics must stay non-secret/non-refreshing/non-revoking: %+v", managedLogin)
	}
	if managedLogin.LogoutClearsSessionCookie != expectLogout || managedLogin.LogoutRevokesProviderToken {
		t.Fatalf("unexpected managed-login logout lifecycle diagnostic: %+v", managedLogin)
	}
}

func expectedManagedLoginRequiredSettings() []string {
	return []string{
		"TENET_AUTH_PROVIDER",
		"TENET_GITHUB_OAUTH_CLIENT_ID",
		"TENET_GITHUB_OAUTH_CLIENT_SECRET",
		"TENET_GITHUB_OAUTH_REDIRECT_URL",
		"TENET_AUTH_STATE_STORE_PATH",
		"TENET_AUTH_IDENTITY_STORE_PATH",
		"TENET_AUTH_SESSION_SECRET",
	}
}

func TestNewAPIOptionsIncludesAuthenticatedIdentityConfigFromEnv(t *testing.T) {
	t.Setenv("TENET_API_TRUST_AUTHENTICATED_HEADERS", "true")
	t.Setenv("TENET_API_ENFORCE_AUTHENTICATED_IDENTITY", "true")
	t.Setenv("TENET_API_ENFORCE_OPERATOR_ROLE_AUTHORIZATION", "true")
	t.Setenv("TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION", "true")

	options := newAPIOptions(buildinfo.Info{Version: "dev", Commit: "abc123"}, writeRepositoryFixture(t), nil)

	if !options.TrustAuthenticatedHeaders {
		t.Fatalf("expected trusted authenticated headers to be enabled, got %+v", options)
	}
	if !options.RequireAuthenticatedIdentity {
		t.Fatalf("expected operator write authentication to be enabled, got %+v", options)
	}
	if !options.RequireOperatorRoleForWrite {
		t.Fatalf("expected operator write role authorization to be enabled, got %+v", options)
	}
	if !options.RequireDeploymentReadAuthorization {
		t.Fatalf("expected deployment read authorization to be enabled, got %+v", options)
	}
}

func TestNewAPIOptionsReadAuthorizationEnforcementAlsoTrustsAuthenticatedHeaders(t *testing.T) {
	t.Setenv("TENET_API_TRUST_AUTHENTICATED_HEADERS", "")
	t.Setenv("TENET_API_ENFORCE_AUTHENTICATED_IDENTITY", "")
	t.Setenv("TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION", "true")

	options := newAPIOptions(buildinfo.Info{Version: "dev", Commit: "abc123"}, writeRepositoryFixture(t), nil)

	if !options.TrustAuthenticatedHeaders {
		t.Fatalf("expected deployment read authorization to enable trusted authenticated headers, got %+v", options)
	}
	if options.RequireAuthenticatedIdentity {
		t.Fatalf("expected operator write authentication to remain disabled, got %+v", options)
	}
	if !options.RequireDeploymentReadAuthorization {
		t.Fatalf("expected deployment read authorization to be enabled, got %+v", options)
	}
}

func TestNewAPIOptionsRoleAuthorizationAlsoRequiresAuthenticatedIdentity(t *testing.T) {
	t.Setenv("TENET_API_TRUST_AUTHENTICATED_HEADERS", "")
	t.Setenv("TENET_API_ENFORCE_AUTHENTICATED_IDENTITY", "")
	t.Setenv("TENET_API_ENFORCE_OPERATOR_ROLE_AUTHORIZATION", "true")
	t.Setenv("TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION", "")

	options := newAPIOptions(buildinfo.Info{Version: "dev", Commit: "abc123"}, writeRepositoryFixture(t), nil)

	if !options.TrustAuthenticatedHeaders {
		t.Fatalf("expected operator role authorization to enable trusted authenticated headers, got %+v", options)
	}
	if !options.RequireAuthenticatedIdentity {
		t.Fatalf("expected operator role authorization to require authenticated identity, got %+v", options)
	}
	if !options.RequireOperatorRoleForWrite {
		t.Fatalf("expected operator write role authorization to be enabled, got %+v", options)
	}
}

func TestNewAPIOptionsWithGitHubCatalogSourceIncludesWebhookDesiredStateSource(t *testing.T) {
	source := apiCatalogSource{
		kind: apiCatalogSourceGitHub,
		githubRequest: githubcontents.Request{
			Repository: "acme/platform-config",
			Ref:        "main",
			Path:       "tenet",
		},
	}

	options := newAPIOptionsWithCatalogSource(buildinfo.Info{Version: "dev", Commit: "abc123"}, source, nil)

	want := githubwebhook.DesiredStateSourceConfig{
		Kind:       githubwebhook.DesiredStateSourceKindGitHub,
		Repository: "acme/platform-config",
		Ref:        "main",
		Path:       "tenet",
	}
	if options.GitHubWebhookDesiredStateSource != want {
		t.Fatalf("unexpected webhook desired-state source: got %+v want %+v", options.GitHubWebhookDesiredStateSource, want)
	}
}

func TestNewAPIOptionsWithRedisStatusStoreIncludesWebhookRefreshPublisher(t *testing.T) {
	client := redis.NewClient(&redis.Options{Addr: "127.0.0.1:6379"})
	t.Cleanup(func() {
		_ = client.Close()
	})

	store := statusstore.NewRedisStore(client, "tenet:test")
	options := newAPIOptionsWithCatalogSource(
		buildinfo.Info{Version: "dev", Commit: "abc123"},
		apiCatalogSource{
			kind: apiCatalogSourceGitHub,
			githubRequest: githubcontents.Request{
				Repository: "acme/platform-config",
				Ref:        "main",
				Path:       "tenet",
			},
		},
		store,
	)

	if options.GitHubWebhookRefreshPublisher == nil {
		t.Fatal("expected redis-backed webhook refresh publisher")
	}
	if _, ok := options.GitHubWebhookRefreshPublisher.(controllerrefresh.RedisHandoff); !ok {
		t.Fatalf("expected redis handoff publisher, got %T", options.GitHubWebhookRefreshPublisher)
	}
}

func TestAPIOptionsOverviewDeploymentReturnsCatalogOnlyDetailWhenSnapshotMissing(t *testing.T) {
	repoRoot := writeRepositoryFixture(t)
	store := &stubStatusStore{getErr: statusstore.ErrNotFound}

	options := newAPIOptions(buildinfo.Info{Version: "dev", Commit: "abc123"}, repoRoot, store)
	if options.OverviewDeployment == nil {
		t.Fatalf("expected overview deployment provider")
	}

	detail, err := options.OverviewDeployment(context.Background(), "worker-prod")
	if err != nil {
		t.Fatalf("OverviewDeployment returned error: %v", err)
	}
	if store.getCalls != 1 {
		t.Fatalf("expected one Get call, got %d", store.getCalls)
	}
	if store.gotGetName != "worker-prod" {
		t.Fatalf("expected Get lookup for worker-prod, got %q", store.gotGetName)
	}
	if store.listCalls != 0 {
		t.Fatalf("expected no List calls, got %d", store.listCalls)
	}
	if detail.Name != "worker-prod" || detail.AccountName != "prod-core" || detail.DesiredArtifact.Image != "payments-worker:v2" {
		t.Fatalf("unexpected deployment detail: %+v", detail)
	}
	if detail.DeployedArtifact != (domain.DeploymentArtifact{}) {
		t.Fatalf("expected zero deployed artifact, got %+v", detail.DeployedArtifact)
	}
	if detail.ObservedGeneration != 0 {
		t.Fatalf("expected zero observed generation, got %d", detail.ObservedGeneration)
	}
	if detail.Sync != (domain.Sync{}) {
		t.Fatalf("expected zero sync, got %+v", detail.Sync)
	}
	if detail.Health != (domain.Health{}) {
		t.Fatalf("expected zero health, got %+v", detail.Health)
	}
	if detail.Rollout != nil {
		t.Fatalf("expected nil rollout, got %+v", detail.Rollout)
	}
	if len(detail.Conditions) != 0 {
		t.Fatalf("expected no conditions, got %+v", detail.Conditions)
	}
}

func TestAPIOptionsOverviewDeploymentReturnsNotFoundForRepoUnknownDeployment(t *testing.T) {
	repoRoot := writeRepositoryFixture(t)
	store := &stubStatusStore{getErr: statusstore.ErrNotFound}

	options := newAPIOptions(buildinfo.Info{Version: "dev", Commit: "abc123"}, repoRoot, store)
	if options.OverviewDeployment == nil {
		t.Fatalf("expected overview deployment provider")
	}

	_, err := options.OverviewDeployment(context.Background(), "missing")
	if !errors.Is(err, statusstore.ErrNotFound) {
		t.Fatalf("expected not found error, got %v", err)
	}
	if store.getCalls != 1 {
		t.Fatalf("expected one Get call, got %d", store.getCalls)
	}
	if store.gotGetName != "missing" {
		t.Fatalf("expected Get lookup for missing, got %q", store.gotGetName)
	}
	if store.listCalls != 0 {
		t.Fatalf("expected no List calls, got %d", store.listCalls)
	}
}

func TestAPIOptionsPlanPreviewPersistsAndReadsBack(t *testing.T) {
	repoRoot := writeRepositoryFixture(t)
	t.Setenv("TENET_PLAN_STORE_PATH", filepath.Join(t.TempDir(), "plan-previews.json"))

	options := newAPIOptions(buildinfo.Info{Version: "dev", Commit: "abc123"}, repoRoot, nil)
	if options.CreateDeploymentPlanPreview == nil || options.PlanPreview == nil {
		t.Fatalf("expected plan preview providers to be wired")
	}

	created, err := options.CreateDeploymentPlanPreview(context.Background(), "api-prod", planpreview.CreateRequest{})
	if err != nil {
		t.Fatalf("CreateDeploymentPlanPreview returned error: %v", err)
	}
	if !created.Created || created.Record.ID == "" {
		t.Fatalf("expected a persisted immutable plan preview, got %+v", created)
	}
	if created.Record.DeploymentName != "api-prod" || created.Record.Status != planpreview.StatusReady {
		t.Fatalf("unexpected created plan preview: %+v", created.Record)
	}
	if len(created.Record.Operations) == 0 {
		t.Fatalf("expected derived plan operations, got %+v", created.Record)
	}

	fetched, err := options.PlanPreview(context.Background(), created.Record.ID)
	if err != nil {
		t.Fatalf("PlanPreview returned error: %v", err)
	}
	if fetched.ID != created.Record.ID || fetched.Source.Route != "/v1/overview/deployments/api-prod" {
		t.Fatalf("unexpected fetched immutable plan preview: %+v", fetched)
	}
}

func TestAPIOptionsApplyRequestPersistsDisabledRecordAndReadsBack(t *testing.T) {
	repoRoot := writeRepositoryFixture(t)
	t.Setenv("TENET_PLAN_STORE_PATH", filepath.Join(t.TempDir(), "plan-previews.json"))
	t.Setenv("TENET_APPLY_STORE_PATH", filepath.Join(t.TempDir(), "apply-requests.json"))

	options := newAPIOptions(buildinfo.Info{Version: "dev", Commit: "abc123"}, repoRoot, nil)
	if options.CreateDeploymentPlanPreview == nil || options.PlanPreview == nil || options.CreateApplyRequest == nil || options.ApplyRequest == nil {
		t.Fatalf("expected plan/apply providers to be wired")
	}

	plan, err := options.CreateDeploymentPlanPreview(context.Background(), "api-prod", planpreview.CreateRequest{})
	if err != nil {
		t.Fatalf("CreateDeploymentPlanPreview returned error: %v", err)
	}
	created, err := options.CreateApplyRequest(context.Background(), plan.Record.ID)
	if err != nil {
		t.Fatalf("CreateApplyRequest returned error: %v", err)
	}
	if !created.Created || created.Record.ID == "" {
		t.Fatalf("expected a persisted immutable apply request, got %+v", created)
	}
	if created.Record.PlanID != plan.Record.ID || created.Record.Status != applyrequest.StatusFailed {
		t.Fatalf("unexpected created apply request: %+v", created.Record)
	}
	if created.Record.Error != "apply execution is disabled" || len(created.Record.Details) == 0 {
		t.Fatalf("expected disabled apply contract, got %+v", created.Record)
	}
	if created.Record.Lifecycle.UpdatedAt == "" || created.Record.Lifecycle.FinishedAt == "" || created.Record.Lifecycle.TotalSteps == 0 {
		t.Fatalf("expected additive apply lifecycle metadata, got %+v", created.Record.Lifecycle)
	}
	if len(created.Record.Operations) == 0 || created.Record.Operations[0].Status != applyrequest.OperationStatusPending {
		t.Fatalf("expected pending projected operations for disabled contract, got %+v", created.Record.Operations)
	}

	fetched, err := options.ApplyRequest(context.Background(), created.Record.ID)
	if err != nil {
		t.Fatalf("ApplyRequest returned error: %v", err)
	}
	if fetched.ID != created.Record.ID || fetched.Source.Route != "/v1/plans/"+plan.Record.ID {
		t.Fatalf("unexpected fetched immutable apply request: %+v", fetched)
	}
}

func TestAPIOptionsApplyRequestPersistsQueuedRecordWhenApplyExecutionIsEnabled(t *testing.T) {
	repoRoot := writeRepositoryFixture(t)
	t.Setenv("TENET_PLAN_STORE_PATH", filepath.Join(t.TempDir(), "plan-previews.json"))
	t.Setenv("TENET_APPLY_STORE_PATH", filepath.Join(t.TempDir(), "apply-requests.json"))
	t.Setenv("TENET_AWS_ENABLE_APPLY", "true")

	options := newAPIOptions(buildinfo.Info{Version: "dev", Commit: "abc123"}, repoRoot, nil)
	if options.CreateDeploymentPlanPreview == nil || options.CreateApplyRequest == nil || options.ApplyRequest == nil {
		t.Fatalf("expected plan/apply providers to be wired")
	}

	plan, err := options.CreateDeploymentPlanPreview(context.Background(), "api-prod", planpreview.CreateRequest{})
	if err != nil {
		t.Fatalf("CreateDeploymentPlanPreview returned error: %v", err)
	}
	created, err := options.CreateApplyRequest(context.Background(), plan.Record.ID)
	if err != nil {
		t.Fatalf("CreateApplyRequest returned error: %v", err)
	}
	if !created.Created || created.Record.ID == "" {
		t.Fatalf("expected a persisted immutable queued apply request, got %+v", created)
	}
	if created.Record.Status != applyrequest.StatusQueued || created.Record.Error != "" {
		t.Fatalf("expected queued apply contract without disabled error, got %+v", created.Record)
	}
	if created.Record.Lifecycle.UpdatedAt == "" || created.Record.Lifecycle.FinishedAt != "" || created.Record.Lifecycle.TotalSteps == 0 {
		t.Fatalf("expected queued lifecycle metadata, got %+v", created.Record.Lifecycle)
	}
	if len(created.Record.Details) != 2 || created.Record.Operations[0].Status != applyrequest.OperationStatusQueued {
		t.Fatalf("expected queued apply details/operations, got %+v", created.Record)
	}

	fetched, err := options.ApplyRequest(context.Background(), created.Record.ID)
	if err != nil {
		t.Fatalf("ApplyRequest returned error: %v", err)
	}
	if fetched.Status != applyrequest.StatusQueued || fetched.Operations[0].Status != applyrequest.OperationStatusQueued {
		t.Fatalf("unexpected fetched queued apply request: %+v", fetched)
	}
}

func TestAPIOptionsApplyRequestTransitionPersistsLifecycleUpdatesAndReusesStableApplyID(t *testing.T) {
	repoRoot := writeRepositoryFixture(t)
	t.Setenv("TENET_PLAN_STORE_PATH", filepath.Join(t.TempDir(), "plan-previews.json"))
	t.Setenv("TENET_APPLY_STORE_PATH", filepath.Join(t.TempDir(), "apply-requests.json"))
	t.Setenv("TENET_AWS_ENABLE_APPLY", "true")

	options := newAPIOptions(buildinfo.Info{Version: "dev", Commit: "abc123"}, repoRoot, nil)
	if options.CreateDeploymentPlanPreview == nil || options.CreateApplyRequest == nil || options.ApplyRequest == nil || options.TransitionApplyRequest == nil {
		t.Fatalf("expected plan/apply providers to be wired")
	}

	plan, err := options.CreateDeploymentPlanPreview(context.Background(), "api-prod", planpreview.CreateRequest{})
	if err != nil {
		t.Fatalf("CreateDeploymentPlanPreview returned error: %v", err)
	}
	created, err := options.CreateApplyRequest(context.Background(), plan.Record.ID)
	if err != nil {
		t.Fatalf("CreateApplyRequest returned error: %v", err)
	}

	running, err := options.TransitionApplyRequest(context.Background(), created.Record.ID, applyrequest.TransitionRequest{
		Status:  applyrequest.StatusRunning,
		Step:    1,
		Details: []string{"Executor picked up the queued apply request."},
	})
	if err != nil {
		t.Fatalf("TransitionApplyRequest returned error: %v", err)
	}
	if running.ID != created.Record.ID || running.Status != applyrequest.StatusRunning {
		t.Fatalf("expected stable running apply request, got %+v", running)
	}
	if running.Error != "" || !reflect.DeepEqual(running.Details, []string{"Executor picked up the queued apply request."}) {
		t.Fatalf("expected running transition to update error/details, got %+v", running)
	}
	if running.Lifecycle.StartedAt == "" || running.Lifecycle.CurrentStep != 1 || running.Lifecycle.CompletedSteps != 0 {
		t.Fatalf("expected running lifecycle metadata, got %+v", running.Lifecycle)
	}
	if len(running.Operations) == 0 || running.Operations[0].Status != applyrequest.OperationStatusRunning {
		t.Fatalf("expected running operation projection, got %+v", running.Operations)
	}

	fetched, err := options.ApplyRequest(context.Background(), created.Record.ID)
	if err != nil {
		t.Fatalf("ApplyRequest returned error: %v", err)
	}
	if fetched.ID != running.ID || fetched.Status != applyrequest.StatusRunning || fetched.Lifecycle.CurrentStep != 1 {
		t.Fatalf("expected fetched running apply request, got %+v", fetched)
	}
}

func TestAPIOptionsApplyRequestRejectsStalePlanFingerprintBeforePersisting(t *testing.T) {
	repoRoot := writeRepositoryFixture(t)
	planStorePath := filepath.Join(t.TempDir(), "plan-previews.json")
	applyStorePath := filepath.Join(t.TempDir(), "apply-requests.json")
	t.Setenv("TENET_PLAN_STORE_PATH", planStorePath)
	t.Setenv("TENET_APPLY_STORE_PATH", applyStorePath)

	options := newAPIOptions(buildinfo.Info{Version: "dev", Commit: "abc123"}, repoRoot, nil)
	if options.CreateDeploymentPlanPreview == nil || options.CreateApplyRequest == nil {
		t.Fatalf("expected plan/apply providers to be wired")
	}

	plan, err := options.CreateDeploymentPlanPreview(context.Background(), "api-prod", planpreview.CreateRequest{})
	if err != nil {
		t.Fatalf("CreateDeploymentPlanPreview returned error: %v", err)
	}

	writeJSON(t, filepath.Join(repoRoot, "deployments", "api-prod.json"), `{"name":"api-prod","image":"payments-api:v3","target":{"type":"ecs"},"stacks":[{"stack":"api-prod","account":"prod-core","env":"prod-apne2","replacement_type":"linear","capacity":{"desired":1,"min":1,"max":1},"regions":[{"region":"ap-northeast-2"}]}]}`)

	_, err = options.CreateApplyRequest(context.Background(), plan.Record.ID)
	if !errors.Is(err, applyrequest.ErrSourceFingerprintConflict) {
		t.Fatalf("expected ErrSourceFingerprintConflict, got %v", err)
	}
	if _, statErr := os.Stat(applyStorePath); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("expected stale conflict to avoid apply persistence, stat err=%v", statErr)
	}
}

func TestAPIOptionsAuditLogsProjectPersistedPlanAndApplyRecords(t *testing.T) {
	repoRoot := writeRepositoryFixture(t)
	t.Setenv("TENET_PLAN_STORE_PATH", filepath.Join(t.TempDir(), "plan-previews.json"))
	t.Setenv("TENET_APPLY_STORE_PATH", filepath.Join(t.TempDir(), "apply-requests.json"))

	options := newAPIOptions(buildinfo.Info{Version: "dev", Commit: "abc123"}, repoRoot, nil)
	if options.CreateDeploymentPlanPreview == nil || options.CreateApplyRequest == nil || options.AuditLogs == nil {
		t.Fatalf("expected audit log dependencies to be wired")
	}

	ctx := requestactor.WithContext(context.Background(), requestactor.Actor{
		Available: true,
		ID:        "operator-123",
		Name:      "Jane Operator",
		Email:     "jane@example.com",
	})

	plan, err := options.CreateDeploymentPlanPreview(ctx, "api-prod", planpreview.CreateRequest{})
	if err != nil {
		t.Fatalf("CreateDeploymentPlanPreview returned error: %v", err)
	}
	apply, err := options.CreateApplyRequest(ctx, plan.Record.ID)
	if err != nil {
		t.Fatalf("CreateApplyRequest returned error: %v", err)
	}

	result, err := options.AuditLogs(context.Background(), auditlog.Query{Limit: 2})
	if err != nil {
		t.Fatalf("AuditLogs returned error: %v", err)
	}
	if len(result.Entries) != 2 {
		t.Fatalf("expected two limited audit entries, got %+v", result.Entries)
	}

	var applyEntry auditlog.Entry
	foundApply := false
	for _, entry := range result.Entries {
		if entry.ApplyID == apply.Record.ID {
			applyEntry = entry
			foundApply = true
			break
		}
	}
	if !foundApply {
		t.Fatalf("expected apply audit entry in %+v", result.Entries)
	}
	if applyEntry.Action != "apply.requested" || applyEntry.PlanID != plan.Record.ID {
		t.Fatalf("unexpected audit entry: %+v", applyEntry)
	}
	if applyEntry.DeploymentName != "api-prod" || applyEntry.AppRef == "" || applyEntry.EnvironmentRef != "prod-apne2" {
		t.Fatalf("expected catalog-derived audit context, got %+v", applyEntry)
	}
	if applyEntry.Source.RecordRoute != "/v1/applies/"+apply.Record.ID || applyEntry.Source.SourceRoute != "/v1/plans/"+plan.Record.ID {
		t.Fatalf("unexpected audit source details: %+v", applyEntry.Source)
	}
	if !applyEntry.Actor.Available || applyEntry.Actor.Name != "Jane Operator" || applyEntry.Actor.Email != "jane@example.com" {
		t.Fatalf("expected projected available actor contract, got %+v", applyEntry.Actor)
	}
}

func TestAPIOptionsAuditLogsProjectAuthenticatedIdentityIntoActorWhenExplicitActorMissing(t *testing.T) {
	repoRoot := writeRepositoryFixture(t)
	t.Setenv("TENET_PLAN_STORE_PATH", filepath.Join(t.TempDir(), "plan-previews.json"))
	t.Setenv("TENET_APPLY_STORE_PATH", filepath.Join(t.TempDir(), "apply-requests.json"))

	options := newAPIOptions(buildinfo.Info{Version: "dev", Commit: "abc123"}, repoRoot, nil)
	if options.CreateDeploymentPlanPreview == nil || options.CreateApplyRequest == nil || options.AuditLogs == nil {
		t.Fatalf("expected audit log dependencies to be wired")
	}

	ctx := requestactor.WithAuthenticatedIdentityContext(context.Background(), requestactor.AuthenticatedIdentity{
		Authenticated: true,
		ID:            "front-door-123",
		Name:          "Front Door Operator",
		Email:         "frontdoor@example.com",
	})

	plan, err := options.CreateDeploymentPlanPreview(ctx, "api-prod", planpreview.CreateRequest{})
	if err != nil {
		t.Fatalf("CreateDeploymentPlanPreview returned error: %v", err)
	}
	apply, err := options.CreateApplyRequest(ctx, plan.Record.ID)
	if err != nil {
		t.Fatalf("CreateApplyRequest returned error: %v", err)
	}

	result, err := options.AuditLogs(context.Background(), auditlog.Query{Limit: 2})
	if err != nil {
		t.Fatalf("AuditLogs returned error: %v", err)
	}

	var applyEntry auditlog.Entry
	foundApply := false
	for _, entry := range result.Entries {
		if entry.ApplyID == apply.Record.ID {
			applyEntry = entry
			foundApply = true
			break
		}
	}
	if !foundApply {
		t.Fatalf("expected apply audit entry in %+v", result.Entries)
	}
	if !applyEntry.Actor.Available || applyEntry.Actor.ID != "front-door-123" || applyEntry.Actor.Name != "Front Door Operator" || applyEntry.Actor.Email != "frontdoor@example.com" {
		t.Fatalf("expected projected actor from authenticated identity, got %+v", applyEntry.Actor)
	}
}

func TestAPIOptionsAuditLogsDeploymentFilterReturnsExplicitEmptyEntries(t *testing.T) {
	repoRoot := writeRepositoryFixture(t)
	t.Setenv("TENET_PLAN_STORE_PATH", filepath.Join(t.TempDir(), "plan-previews.json"))
	t.Setenv("TENET_APPLY_STORE_PATH", filepath.Join(t.TempDir(), "apply-requests.json"))

	options := newAPIOptions(buildinfo.Info{Version: "dev", Commit: "abc123"}, repoRoot, nil)
	if options.CreateDeploymentPlanPreview == nil || options.CreateApplyRequest == nil || options.AuditLogs == nil {
		t.Fatalf("expected audit log dependencies to be wired")
	}

	plan, err := options.CreateDeploymentPlanPreview(context.Background(), "api-prod", planpreview.CreateRequest{})
	if err != nil {
		t.Fatalf("CreateDeploymentPlanPreview returned error: %v", err)
	}
	if _, err := options.CreateApplyRequest(context.Background(), plan.Record.ID); err != nil {
		t.Fatalf("CreateApplyRequest returned error: %v", err)
	}

	result, err := options.AuditLogs(context.Background(), auditlog.Query{DeploymentName: "worker-prod"})
	if err != nil {
		t.Fatalf("AuditLogs returned error: %v", err)
	}
	if len(result.Entries) != 0 {
		t.Fatalf("expected empty filtered audit entries, got %+v", result.Entries)
	}
}

func TestAPIOptionsOverviewDeploymentEventsUsesNamedSnapshotLookup(t *testing.T) {
	repoRoot := writeRepositoryFixture(t)
	store := &stubStatusStore{
		getSnapshot: statusstore.Snapshot{
			DeploymentName: "api-prod",
			Status: domain.DeploymentStatus{
				History: []domain.StatusHistoryEntry{{
					Timestamp:          "2026-04-16T06:49:00Z",
					Source:             "controller",
					Summary:            "Shifted 10 percent of traffic",
					Sync:               domain.Sync{State: domain.SyncStateReconciling, Reason: "Traffic shift running"},
					Health:             domain.Health{Overall: domain.HealthHealthy, Summary: "steady"},
					DesiredGeneration:  8,
					ObservedGeneration: 7,
				}},
			},
		},
	}

	options := newAPIOptions(buildinfo.Info{Version: "dev", Commit: "abc123"}, repoRoot, store)
	if options.OverviewDeploymentEvents == nil {
		t.Fatalf("expected overview deployment events provider")
	}

	events, err := options.OverviewDeploymentEvents(context.Background(), "api-prod")
	if err != nil {
		t.Fatalf("OverviewDeploymentEvents returned error: %v", err)
	}
	if store.getCalls != 1 {
		t.Fatalf("expected one Get call, got %d", store.getCalls)
	}
	if store.gotGetName != "api-prod" {
		t.Fatalf("expected Get lookup for api-prod, got %q", store.gotGetName)
	}
	if store.listCalls != 0 {
		t.Fatalf("expected no List calls, got %d", store.listCalls)
	}
	if events.DeploymentName != "api-prod" || len(events.Events) != 1 || events.Events[0].Source != "controller" {
		t.Fatalf("unexpected deployment events: %+v", events)
	}
}

func TestAPIOptionsOverviewDeploymentEventsReturnsEmptyListWhenSnapshotMissing(t *testing.T) {
	repoRoot := writeRepositoryFixture(t)
	store := &stubStatusStore{getErr: statusstore.ErrNotFound}

	options := newAPIOptions(buildinfo.Info{Version: "dev", Commit: "abc123"}, repoRoot, store)
	if options.OverviewDeploymentEvents == nil {
		t.Fatalf("expected overview deployment events provider")
	}

	events, err := options.OverviewDeploymentEvents(context.Background(), "worker-prod")
	if err != nil {
		t.Fatalf("OverviewDeploymentEvents returned error: %v", err)
	}
	if store.getCalls != 1 {
		t.Fatalf("expected one Get call, got %d", store.getCalls)
	}
	if store.gotGetName != "worker-prod" {
		t.Fatalf("expected Get lookup for worker-prod, got %q", store.gotGetName)
	}
	if store.listCalls != 0 {
		t.Fatalf("expected no List calls, got %d", store.listCalls)
	}
	if events.DeploymentName != "worker-prod" || events.Events == nil || len(events.Events) != 0 {
		t.Fatalf("expected explicit empty events payload, got %+v", events)
	}
}

func TestAPIOptionsOverviewAccountUsesSnapshotListLookup(t *testing.T) {
	repoRoot := writeRepositoryFixture(t)
	store := &stubStatusStore{
		listSnapshots: []statusstore.Snapshot{{
			DeploymentName: "api-prod",
			Status: domain.DeploymentStatus{
				Sync:   domain.Sync{State: domain.SyncStateSynced, Reason: "steady"},
				Health: domain.Health{Overall: domain.HealthHealthy, Summary: "healthy"},
			},
		}},
	}

	options := newAPIOptions(buildinfo.Info{Version: "dev", Commit: "abc123"}, repoRoot, store)
	if options.OverviewAccount == nil {
		t.Fatalf("expected overview account provider")
	}

	detail, err := options.OverviewAccount(context.Background(), "123456789012")
	if err != nil {
		t.Fatalf("OverviewAccount returned error: %v", err)
	}
	if store.listCalls != 1 {
		t.Fatalf("expected one List call, got %d", store.listCalls)
	}
	if store.getCalls != 0 {
		t.Fatalf("expected no Get calls, got %d", store.getCalls)
	}
	if detail.AWSAccountID != "123456789012" || detail.Name != "prod-core" || len(detail.Deployments) != 2 {
		t.Fatalf("unexpected account detail: %+v", detail)
	}
	if detail.Deployments[0].Name != "api-prod" {
		t.Fatalf("expected api-prod first deployment, got %+v", detail.Deployments)
	}
}

func TestAPIOptionsOverviewAccountPropagatesSnapshotListErrors(t *testing.T) {
	repoRoot := writeRepositoryFixture(t)
	expected := errors.New("snapshots unavailable")
	store := &stubStatusStore{listErr: expected}

	options := newAPIOptions(buildinfo.Info{Version: "dev", Commit: "abc123"}, repoRoot, store)
	if options.OverviewAccount == nil {
		t.Fatalf("expected overview account provider")
	}

	_, err := options.OverviewAccount(context.Background(), "123456789012")
	if !errors.Is(err, expected) {
		t.Fatalf("expected snapshot list error, got %v", err)
	}
	if store.listCalls != 1 {
		t.Fatalf("expected one List call, got %d", store.listCalls)
	}
	if store.getCalls != 0 {
		t.Fatalf("expected no Get calls, got %d", store.getCalls)
	}
}

func TestAPIOptionsOverviewApplicationsUsesSnapshotListLookup(t *testing.T) {
	repoRoot := writeRepositoryFixture(t)
	store := &stubStatusStore{
		listSnapshots: []statusstore.Snapshot{{
			DeploymentName: "api-prod",
			Status: domain.DeploymentStatus{
				Sync:             domain.Sync{State: domain.SyncStateSynced, Reason: "steady"},
				Health:           domain.Health{Overall: domain.HealthHealthy, Summary: "healthy"},
				DeployedArtifact: domain.DeploymentArtifact{Image: "payments-api:v1"},
			},
		}},
	}

	options := newAPIOptions(buildinfo.Info{Version: "dev", Commit: "abc123"}, repoRoot, store)
	if options.OverviewApplications == nil {
		t.Fatalf("expected overview applications provider")
	}

	applications, err := options.OverviewApplications(context.Background())
	if err != nil {
		t.Fatalf("OverviewApplications returned error: %v", err)
	}
	if store.listCalls != 1 {
		t.Fatalf("expected one List call, got %d", store.listCalls)
	}
	if store.getCalls != 0 {
		t.Fatalf("expected no Get calls, got %d", store.getCalls)
	}
	if len(applications) != 2 {
		t.Fatalf("expected one flattened row per deployment, got %+v", applications)
	}
	if applications[0].Name != "api-prod" || applications[0].DeployedArtifact.Image != "payments-api:v1" || applications[0].Sync.State != domain.SyncStateSynced {
		t.Fatalf("unexpected application overview row: %+v", applications[0])
	}
}

func TestAPIOptionsOverviewApplicationsFallsBackToCatalogWhenStoreMissing(t *testing.T) {
	repoRoot := writeRepositoryFixture(t)

	options := newAPIOptions(buildinfo.Info{Version: "dev", Commit: "abc123"}, repoRoot, nil)
	if options.OverviewApplications == nil {
		t.Fatalf("expected overview applications provider")
	}

	applications, err := options.OverviewApplications(context.Background())
	if err != nil {
		t.Fatalf("OverviewApplications returned error: %v", err)
	}
	if len(applications) != 2 {
		t.Fatalf("expected one flattened row per deployment, got %+v", applications)
	}
	if applications[0].Name != "api-prod" || applications[0].AWSAccountID != "123456789012" || applications[0].RuntimeSummary != nil {
		t.Fatalf("expected catalog-only overview row with zero runtime state, got %+v", applications[0])
	}
	if applications[1].Name != "worker-prod" || applications[1].Sync != (domain.Sync{}) || applications[1].Health != (domain.Health{}) {
		t.Fatalf("expected zero-valued runtime fields for repo-only row, got %+v", applications[1])
	}
}

func TestAPIOptionsOverviewApplication(t *testing.T) {
	t.Run("uses named snapshot lookup", func(t *testing.T) {
		repoRoot := writeRepositoryFixture(t)
		store := &stubStatusStore{
			getSnapshot: statusstore.Snapshot{
				DeploymentName: "api-prod",
				Status: domain.DeploymentStatus{
					ObservedGeneration: 7,
					Sync:               domain.Sync{State: domain.SyncStateOutOfSync, Reason: "PendingPromotion"},
					Health:             domain.Health{Overall: domain.HealthProgressing, Summary: "warming up"},
					DeployedArtifact:   domain.DeploymentArtifact{Image: "payments-api:v1"},
				},
			},
		}

		options := newAPIOptions(buildinfo.Info{Version: "dev", Commit: "abc123"}, repoRoot, store)
		if options.OverviewApplication == nil {
			t.Fatalf("expected overview application provider")
		}

		detail, err := options.OverviewApplication(context.Background(), "api-prod")
		if err != nil {
			t.Fatalf("OverviewApplication returned error: %v", err)
		}
		if store.getCalls != 1 {
			t.Fatalf("expected one Get call, got %d", store.getCalls)
		}
		if store.gotGetName != "api-prod" {
			t.Fatalf("expected Get lookup for api-prod, got %q", store.gotGetName)
		}
		if store.listCalls != 0 {
			t.Fatalf("expected no List calls, got %d", store.listCalls)
		}
		if detail.Name != "api-prod" || detail.DeployedArtifact.Image != "payments-api:v1" || detail.AccountName != "prod-core" {
			t.Fatalf("unexpected application detail: %+v", detail)
		}
	})

	t.Run("returns catalog only detail when snapshot missing", func(t *testing.T) {
		repoRoot := writeRepositoryFixture(t)
		store := &stubStatusStore{getErr: statusstore.ErrNotFound}

		options := newAPIOptions(buildinfo.Info{Version: "dev", Commit: "abc123"}, repoRoot, store)
		if options.OverviewApplication == nil {
			t.Fatalf("expected overview application provider")
		}

		detail, err := options.OverviewApplication(context.Background(), "worker-prod")
		if err != nil {
			t.Fatalf("OverviewApplication returned error: %v", err)
		}
		if store.getCalls != 1 {
			t.Fatalf("expected one Get call, got %d", store.getCalls)
		}
		if store.gotGetName != "worker-prod" {
			t.Fatalf("expected Get lookup for worker-prod, got %q", store.gotGetName)
		}
		if store.listCalls != 0 {
			t.Fatalf("expected no List calls, got %d", store.listCalls)
		}
		if detail.Name != "worker-prod" || detail.AccountName != "prod-core" || detail.DesiredArtifact.Image != "payments-worker:v2" {
			t.Fatalf("unexpected application detail: %+v", detail)
		}
		if detail.DeployedArtifact != (domain.DeploymentArtifact{}) || detail.ObservedGeneration != 0 || detail.Sync != (domain.Sync{}) || detail.Health != (domain.Health{}) {
			t.Fatalf("expected zero runtime fields in catalog-only detail, got %+v", detail)
		}
	})
}

func TestAPIOptionsOverviewDetailProvidersFallBackToCatalogWhenStoreMissing(t *testing.T) {
	repoRoot := writeRepositoryFixture(t)

	options := newAPIOptions(buildinfo.Info{Version: "dev", Commit: "abc123"}, repoRoot, nil)
	if options.OverviewDeployment == nil {
		t.Fatalf("expected overview deployment provider")
	}
	if options.OverviewAccounts == nil {
		t.Fatalf("expected overview accounts provider")
	}
	if options.OverviewAccount == nil {
		t.Fatalf("expected overview account provider")
	}
	if options.OverviewProjectEnvironment == nil {
		t.Fatalf("expected overview project/environment provider")
	}

	deployment, err := options.OverviewDeployment(context.Background(), "api-prod")
	if err != nil {
		t.Fatalf("OverviewDeployment returned error: %v", err)
	}
	if deployment.Name != "api-prod" || deployment.AccountName != "prod-core" || deployment.Sync != (domain.Sync{}) || deployment.Health != (domain.Health{}) {
		t.Fatalf("expected catalog-only deployment detail, got %+v", deployment)
	}

	accounts, err := options.OverviewAccounts(context.Background())
	if err != nil {
		t.Fatalf("OverviewAccounts returned error: %v", err)
	}
	if len(accounts) != 1 || accounts[0].AWSAccountID != "123456789012" || len(accounts[0].Deployments) != 2 {
		t.Fatalf("expected catalog-only account overviews, got %+v", accounts)
	}

	account, err := options.OverviewAccount(context.Background(), "123456789012")
	if err != nil {
		t.Fatalf("OverviewAccount returned error: %v", err)
	}
	if account.Name != "prod-core" || account.DeploymentCount != 2 {
		t.Fatalf("expected catalog-only account detail, got %+v", account)
	}
	if len(account.Deployments) == 0 {
		t.Fatalf("expected account deployments in catalog-only detail, got %+v", account)
	}

	appRef := account.Deployments[0].AppRef
	environmentRef := account.Deployments[0].EnvironmentRef
	group, err := options.OverviewProjectEnvironment(context.Background(), "123456789012", appRef, environmentRef)
	if err != nil {
		t.Fatalf("OverviewProjectEnvironment returned error: %v", err)
	}
	if group.AppRef != appRef || group.EnvironmentRef != environmentRef || len(group.Deployments) == 0 {
		t.Fatalf("expected catalog-only project/environment detail, got %+v", group)
	}
}

func TestAPIOptionsOverviewDeploymentResourceLogsWiresObserverRoleAndPlacement(t *testing.T) {
	repoRoot := writeRepositoryFixture(t)
	store := &stubStatusStore{
		getSnapshot: statusstore.Snapshot{
			DeploymentName: "api-prod",
			Status: domain.DeploymentStatus{
				Rollout: &domain.RolloutSummary{Execution: []domain.RolloutExecutionDetail{{Resource: "ecsService", Name: "api-prod-live-service", Description: "live ECS service"}}},
			},
		},
	}
	provider := &fakeResourceLogProvider{result: resourcelogs.Result{Available: true}}
	restore := stubResourceLogProviderFactory(provider)
	defer restore()

	options := newAPIOptions(buildinfo.Info{Version: "dev", Commit: "abc123"}, repoRoot, store)
	if options.OverviewDeploymentResourceLogs == nil {
		t.Fatalf("expected overview deployment resource logs provider")
	}

	result, err := options.OverviewDeploymentResourceLogs(context.Background(), "api-prod", "ecsService", "api-prod-live-service", 7)
	if err != nil {
		t.Fatalf("OverviewDeploymentResourceLogs returned error: %v", err)
	}
	if !result.Available {
		t.Fatalf("expected wired provider result, got %+v", result)
	}
	want := resourcelogs.Request{
		DeploymentName:   "api-prod",
		ResourceType:     "ecsService",
		ResourceName:     "api-prod-live-service",
		ControlAccountID: "123456789012",
		TargetAccountID:  "123456789012",
		Region:           "ap-northeast-2",
		ObserverRoleARN:  "arn:aws:iam::123456789012:role/TenetObserverRole",
		Limit:            7,
	}
	if !reflect.DeepEqual(provider.lastRequest, want) {
		t.Fatalf("unexpected resource log request: %+v", provider.lastRequest)
	}
}

func TestAPIOptionsOverviewDeploymentResourceLogsPassesBootstrapSourceBackingResourceContext(t *testing.T) {
	repoRoot := writeRepositoryFixture(t)
	store := &stubStatusStore{
		getSnapshot: statusstore.Snapshot{
			DeploymentName: "api-prod",
			Status: domain.DeploymentStatus{
				Rollout: &domain.RolloutSummary{Execution: []domain.RolloutExecutionDetail{
					{Resource: "candidateAutoScalingGroup", Name: "api-prod-candidate", Description: "candidate ASG"},
					{Resource: "userDataBootstrapSource", Name: "file:userdata/api-prod.sh", Description: "bootstrap source"},
				}},
			},
		},
	}
	provider := &fakeResourceLogProvider{result: resourcelogs.Result{Available: true}}
	restore := stubResourceLogProviderFactory(provider)
	defer restore()

	options := newAPIOptions(buildinfo.Info{Version: "dev", Commit: "abc123"}, repoRoot, store)
	if options.OverviewDeploymentResourceLogs == nil {
		t.Fatalf("expected overview deployment resource logs provider")
	}

	_, err := options.OverviewDeploymentResourceLogs(context.Background(), "api-prod", "userDataBootstrapSource", "file:userdata/api-prod.sh", 0)
	if err != nil {
		t.Fatalf("OverviewDeploymentResourceLogs returned error: %v", err)
	}
	if provider.lastRequest.BackingResourceType != "candidateAutoScalingGroup" || provider.lastRequest.BackingResourceName != "api-prod-candidate" {
		t.Fatalf("expected bootstrap-source logs request to carry backing ASG context, got %+v", provider.lastRequest)
	}
}

func TestAPIOptionsOverviewDeploymentResourceLogsResolvesFilesystemBootstrapRouteAgainstGitUserData(t *testing.T) {
	repoRoot := writeGitUserDataCanaryEC2RepositoryFixture(t)
	store := &stubStatusStore{
		getSnapshot: statusstore.Snapshot{
			DeploymentName: "api-prod",
			Status: domain.DeploymentStatus{
				Rollout: &domain.RolloutSummary{Execution: []domain.RolloutExecutionDetail{
					{Resource: "candidateAutoScalingGroup", Name: "api-prod-candidate", Description: "candidate ASG"},
					{Resource: "userDataBootstrapSource", Name: "git:userdata/api-prod.sh", Description: "bootstrap source"},
				}},
			},
		},
	}
	provider := &fakeResourceLogProvider{result: resourcelogs.Result{Available: true}}
	restore := stubResourceLogProviderFactory(provider)
	defer restore()

	options := newAPIOptions(buildinfo.Info{Version: "dev", Commit: "abc123"}, repoRoot, store)

	_, err := options.OverviewDeploymentResourceLogs(context.Background(), "api-prod", "userDataBootstrapSource", "file:userdata/api-prod.sh", 0)
	if err != nil {
		t.Fatalf("expected filesystem bootstrap route to resolve against git userdata metadata, got %v", err)
	}
	if provider.calls != 1 {
		t.Fatalf("expected provider to run once, got %d calls", provider.calls)
	}
	if provider.lastRequest.ResourceType != "userDataBootstrapSource" || provider.lastRequest.ResourceName != "file:userdata/api-prod.sh" {
		t.Fatalf("expected requested bootstrap identity to be preserved, got %+v", provider.lastRequest)
	}
	if provider.lastRequest.BackingResourceType != "candidateAutoScalingGroup" || provider.lastRequest.BackingResourceName != "api-prod-candidate" {
		t.Fatalf("expected filesystem bootstrap route to carry backing ASG context, got %+v", provider.lastRequest)
	}
}

func TestAPIOptionsOverviewDeploymentResourceLogsRejectsResourcesOutsideDeploymentExecution(t *testing.T) {
	repoRoot := writeRepositoryFixture(t)
	store := &stubStatusStore{
		getSnapshot: statusstore.Snapshot{
			DeploymentName: "api-prod",
			Status: domain.DeploymentStatus{
				Rollout: &domain.RolloutSummary{Execution: []domain.RolloutExecutionDetail{{Resource: "ecsService", Name: "api-prod-live-service", Description: "live ECS service"}}},
			},
		},
	}
	provider := &fakeResourceLogProvider{}
	restore := stubResourceLogProviderFactory(provider)
	defer restore()

	options := newAPIOptions(buildinfo.Info{Version: "dev", Commit: "abc123"}, repoRoot, store)
	_, err := options.OverviewDeploymentResourceLogs(context.Background(), "api-prod", "ecsService", "other-service", 0)
	if !errors.Is(err, statusstore.ErrNotFound) {
		t.Fatalf("expected not found for unbound resource, got %v", err)
	}
	if provider.calls != 0 {
		t.Fatalf("expected provider not to run for unbound resource, got %d calls", provider.calls)
	}
}

func TestAPIOptionsOverviewDeploymentResourceLogsUsesCanonicalFallbackWhenExecutionIsMissing(t *testing.T) {
	repoRoot := writeRepositoryFixture(t)
	provider := &fakeResourceLogProvider{result: resourcelogs.Result{Available: true}}
	restore := stubResourceLogProviderFactory(provider)
	defer restore()

	options := newAPIOptions(buildinfo.Info{Version: "dev", Commit: "abc123"}, repoRoot, nil)
	_, err := options.OverviewDeploymentResourceLogs(context.Background(), "api-prod", "ecsService", "api-prod-service", 0)
	if err != nil {
		t.Fatalf("expected canonical fallback resource binding to work, got %v", err)
	}
	if provider.calls != 1 {
		t.Fatalf("expected provider to run once for fallback-bound resource, got %d calls", provider.calls)
	}
}

func TestAPIOptionsOverviewDeploymentResourceLogsUsesCodeDeployFallbackWhenExecutionIsMissing(t *testing.T) {
	repoRoot := writeRepositoryFixture(t)
	provider := &fakeResourceLogProvider{result: resourcelogs.Result{Available: true}}
	restore := stubResourceLogProviderFactory(provider)
	defer restore()

	options := newAPIOptions(buildinfo.Info{Version: "dev", Commit: "abc123"}, repoRoot, nil)
	for _, resource := range []struct {
		resourceType        string
		resourceName        string
		resourceDescription string
		wantBackingType     string
		wantBackingName     string
	}{
		{
			resourceType:        "codeDeployApplication",
			resourceName:        "api-prod-codedeploy-app",
			resourceDescription: "CodeDeploy application coordinating ECS traffic shifting (style: linear)",
		},
		{
			resourceType:        "codeDeployDeploymentGroup",
			resourceName:        "api-prod-codedeploy-group",
			resourceDescription: "CodeDeploy deployment group Tenet targets for ECS rollout (style: linear)",
		},
		{
			resourceType:        "productionListener",
			resourceName:        "api-prod-prod-listener",
			resourceDescription: "Production listener CodeDeploy shifts during ECS traffic shifting (style: linear)",
			wantBackingType:     "codeDeployDeploymentGroup",
			wantBackingName:     "api-prod-codedeploy-group",
		},
		{
			resourceType:        "testListener",
			resourceName:        "api-prod-test-listener",
			resourceDescription: "Test listener CodeDeploy uses to validate the replacement ECS task set before promotion (style: linear)",
			wantBackingType:     "codeDeployDeploymentGroup",
			wantBackingName:     "api-prod-codedeploy-group",
		},
		{
			resourceType:        "blueTaskSetTargetGroup",
			resourceName:        "api-prod-blue",
			resourceDescription: "Blue task set target group CodeDeploy treats as the live ECS traffic destination (style: linear)",
			wantBackingType:     "codeDeployDeploymentGroup",
			wantBackingName:     "api-prod-codedeploy-group",
		},
		{
			resourceType:        "greenTaskSetTargetGroup",
			resourceName:        "api-prod-green",
			resourceDescription: "Green task set target group CodeDeploy warms before shifting ECS traffic (style: linear)",
			wantBackingType:     "codeDeployDeploymentGroup",
			wantBackingName:     "api-prod-codedeploy-group",
		},
	} {
		provider.calls = 0
		provider.lastRequest = resourcelogs.Request{}

		_, err := options.OverviewDeploymentResourceLogs(context.Background(), "api-prod", resource.resourceType, resource.resourceName, 0)
		if err != nil {
			t.Fatalf("expected CodeDeploy fallback resource binding to work for %s/%s, got %v", resource.resourceType, resource.resourceName, err)
		}
		if provider.calls != 1 {
			t.Fatalf("expected provider to run once for fallback-bound %s/%s, got %d calls", resource.resourceType, resource.resourceName, provider.calls)
		}
		if provider.lastRequest.ResourceType != resource.resourceType || provider.lastRequest.ResourceName != resource.resourceName {
			t.Fatalf("unexpected resource log request for %s/%s: %+v", resource.resourceType, resource.resourceName, provider.lastRequest)
		}
		if provider.lastRequest.BackingResourceType != resource.wantBackingType || provider.lastRequest.BackingResourceName != resource.wantBackingName {
			t.Fatalf("unexpected backing resource context for %s/%s: %+v", resource.resourceType, resource.resourceName, provider.lastRequest)
		}
	}
}

func TestAPIOptionsOverviewDeploymentResourceLogsUsesECSRollingFallbackWhenExecutionIsMissing(t *testing.T) {
	repoRoot := writeRollingECSRepositoryFixture(t)
	provider := &fakeResourceLogProvider{result: resourcelogs.Result{Available: true}}
	restore := stubResourceLogProviderFactory(provider)
	defer restore()

	options := newAPIOptions(buildinfo.Info{Version: "dev", Commit: "abc123"}, repoRoot, nil)
	_, err := options.OverviewDeploymentResourceLogs(context.Background(), "api-prod", "ecsService", "api-prod-service", 0)
	if err != nil {
		t.Fatalf("expected ecs rolling fallback resource binding to work, got %v", err)
	}
	if provider.calls != 1 {
		t.Fatalf("expected provider to run once for ecs rolling fallback resource, got %d calls", provider.calls)
	}
	if provider.lastRequest.ResourceType != "ecsService" || provider.lastRequest.ResourceName != "api-prod-service" {
		t.Fatalf("unexpected resource log request for ecs rolling fallback: %+v", provider.lastRequest)
	}
}

func TestAPIOptionsOverviewDeploymentResourceLogsUsesWeightedTargetGroupFallbackWhenExecutionIsMissing(t *testing.T) {
	repoRoot := writeCanaryEC2RepositoryFixture(t)
	provider := &fakeResourceLogProvider{result: resourcelogs.Result{Available: true}}
	restore := stubResourceLogProviderFactory(provider)
	defer restore()

	options := newAPIOptions(buildinfo.Info{Version: "dev", Commit: "abc123"}, repoRoot, nil)
	for _, resource := range []struct {
		resourceType string
		resourceName string
	}{
		{resourceType: "candidateAutoScalingGroup", resourceName: "api-prod-candidate"},
		{resourceType: "trafficShiftPlan", resourceName: "api-prod-weighted-routing"},
		{resourceType: "userDataBootstrapSource", resourceName: "file:userdata/api-prod.sh"},
	} {
		provider.calls = 0
		provider.lastRequest = resourcelogs.Request{}

		_, err := options.OverviewDeploymentResourceLogs(context.Background(), "api-prod", resource.resourceType, resource.resourceName, 0)
		if err != nil {
			t.Fatalf("expected weighted target group fallback resource binding for %s/%s, got %v", resource.resourceType, resource.resourceName, err)
		}
		if provider.calls != 1 {
			t.Fatalf("expected provider to run once for %s/%s, got %d calls", resource.resourceType, resource.resourceName, provider.calls)
		}
		if provider.lastRequest.ResourceType != resource.resourceType || provider.lastRequest.ResourceName != resource.resourceName {
			t.Fatalf("unexpected resource log request for %s/%s: %+v", resource.resourceType, resource.resourceName, provider.lastRequest)
		}
		if resource.resourceType == "userDataBootstrapSource" {
			if provider.lastRequest.BackingResourceType != "candidateAutoScalingGroup" || provider.lastRequest.BackingResourceName != "api-prod-candidate" {
				t.Fatalf("expected bootstrap-source fallback to target candidate ASG, got %+v", provider.lastRequest)
			}
		}
	}
}

func TestCanonicalRolloutExecutionFallback(t *testing.T) {
	t.Run("weighted target groups", func(t *testing.T) {
		detail := catalog.DeploymentOverviewDetail{
			Name:          "api-prod",
			ExecutionMode: "asgWeightedTargetGroups",
			Strategy:      "canary",
			UserData:      domain.DeploymentUserData{Type: "file", Path: "userdata/api-prod.sh"},
			RolloutConfig: domain.DeploymentRolloutConfig{Capacity: domain.DeploymentRolloutCapacity{Desired: 4, Min: 2, Max: 8}, Strategy: domain.DeploymentRolloutStrategy{WarmupSeconds: 90}},
		}
		want := []domain.RolloutExecutionDetail{{
			Resource:    "candidateAutoScalingGroup",
			Name:        "api-prod-candidate",
			Description: "Candidate Auto Scaling Group Tenet creates from normalized userdata source file:userdata/api-prod.sh with desired 4, min 2, and max 8 instances and warmup 90s for weighted target group rollout",
		}, {
			Resource:    "activeTargetGroup",
			Name:        "api-prod-active",
			Description: "Currently active target group receiving production traffic",
		}, {
			Resource:    "candidateTargetGroup",
			Name:        "api-prod-candidate",
			Description: "Candidate target group Tenet will register and warm before traffic shifting",
		}, {
			Resource:    "productionListenerRule",
			Name:        "api-prod-prod-listener",
			Description: "Production listener rule operators inspect while Tenet updates weighted forwarding during rollout",
		}, {
			Resource:    "trafficShiftPlan",
			Name:        "api-prod-weighted-routing",
			Description: "Weighted routing plan Tenet follows to shift a canary slice of production traffic from the active target group to the candidate target group before full promotion while respecting warmup 90s",
		}, {
			Resource:    "userDataBootstrapSource",
			Name:        "file:userdata/api-prod.sh",
			Description: "Normalized userdata bootstrap source for candidate EC2 capacity",
		}}
		if got := canonicalRolloutExecutionFallback(detail); !reflect.DeepEqual(got, want) {
			t.Fatalf("unexpected weighted target group fallback resources: got %+v want %+v", got, want)
		}
	})

	t.Run("ecs code deploy", func(t *testing.T) {
		detail := catalog.DeploymentOverviewDetail{
			Name:          "worker-prod",
			ExecutionMode: "ecsCodeDeploy",
			Strategy:      "linear",
			RolloutConfig: domain.DeploymentRolloutConfig{Strategy: domain.DeploymentRolloutStrategy{MinHealthyPercent: 80, WarmupSeconds: 45}},
		}
		want := []domain.RolloutExecutionDetail{{
			Resource:    "ecsService",
			Name:        "worker-prod-service",
			Description: "ECS service Tenet updates during CodeDeploy traffic shifting (style: linear; qualifiers: min healthy 80% and warmup 45s)",
		}, {
			Resource:    "codeDeployApplication",
			Name:        "worker-prod-codedeploy-app",
			Description: "CodeDeploy application coordinating ECS traffic shifting (style: linear; qualifiers: min healthy 80% and warmup 45s)",
		}, {
			Resource:    "codeDeployDeploymentGroup",
			Name:        "worker-prod-codedeploy-group",
			Description: "CodeDeploy deployment group Tenet targets for ECS rollout (style: linear; qualifiers: min healthy 80% and warmup 45s)",
		}, {
			Resource:    "productionListener",
			Name:        "worker-prod-prod-listener",
			Description: "Production listener CodeDeploy shifts during ECS traffic shifting (style: linear; qualifiers: min healthy 80% and warmup 45s)",
		}, {
			Resource:    "testListener",
			Name:        "worker-prod-test-listener",
			Description: "Test listener CodeDeploy uses to validate the replacement ECS task set before promotion (style: linear; qualifiers: min healthy 80% and warmup 45s)",
		}, {
			Resource:    "blueTaskSetTargetGroup",
			Name:        "worker-prod-blue",
			Description: "Blue task set target group CodeDeploy treats as the live ECS traffic destination (style: linear; qualifiers: min healthy 80% and warmup 45s)",
		}, {
			Resource:    "greenTaskSetTargetGroup",
			Name:        "worker-prod-green",
			Description: "Green task set target group CodeDeploy warms before shifting ECS traffic (style: linear; qualifiers: min healthy 80% and warmup 45s)",
		}}
		if got := canonicalRolloutExecutionFallback(detail); !reflect.DeepEqual(got, want) {
			t.Fatalf("unexpected ecs code deploy fallback resources: got %+v want %+v", got, want)
		}
	})

	t.Run("ecs rolling", func(t *testing.T) {
		detail := catalog.DeploymentOverviewDetail{
			Name:          "api-prod",
			ExecutionMode: "ecsRolling",
			RolloutConfig: domain.DeploymentRolloutConfig{Strategy: domain.DeploymentRolloutStrategy{MinHealthyPercent: 90, WarmupSeconds: 30}},
		}
		want := []domain.RolloutExecutionDetail{{
			Resource:    "ecsService",
			Name:        "api-prod-service",
			Description: "ECS service Tenet updates with native rolling deployment while respecting min healthy 90% and warmup 30s",
		}}
		if got := canonicalRolloutExecutionFallback(detail); !reflect.DeepEqual(got, want) {
			t.Fatalf("unexpected ecs rolling fallback resources: got %+v want %+v", got, want)
		}
	})

	t.Run("asg instance refresh", func(t *testing.T) {
		detail := catalog.DeploymentOverviewDetail{
			Name:          "api-prod",
			ExecutionMode: "asgInstanceRefresh",
			UserData:      domain.DeploymentUserData{Type: "file", Path: "userdata/api-prod-refresh.sh"},
			RolloutConfig: domain.DeploymentRolloutConfig{Stacks: []domain.DeploymentRolloutStack{{RollingUpdateInstanceCount: 2}}},
		}
		want := []domain.RolloutExecutionDetail{{
			Resource:    "serviceAutoScalingGroup",
			Name:        "api-prod-service",
			Description: "Service Auto Scaling Group Tenet refreshes in place with rolling-update batch size 2",
		}, {
			Resource:    "userDataBootstrapSource",
			Name:        "file:userdata/api-prod-refresh.sh",
			Description: "Normalized userdata bootstrap source for refreshed EC2 instances",
		}}
		if got := canonicalRolloutExecutionFallback(detail); !reflect.DeepEqual(got, want) {
			t.Fatalf("unexpected asg instance refresh fallback resources: got %+v want %+v", got, want)
		}
	})
}

func TestAPIOptionsOverviewDeploymentResourceLogsUsesASGInstanceRefreshFallbackWhenExecutionIsMissing(t *testing.T) {
	repoRoot := writeRollingEC2RepositoryFixture(t)
	provider := &fakeResourceLogProvider{result: resourcelogs.Result{Available: true}}
	restore := stubResourceLogProviderFactory(provider)
	defer restore()

	options := newAPIOptions(buildinfo.Info{Version: "dev", Commit: "abc123"}, repoRoot, nil)
	for _, resource := range []struct {
		resourceType string
		resourceName string
	}{
		{resourceType: "serviceAutoScalingGroup", resourceName: "api-prod-service"},
		{resourceType: "userDataBootstrapSource", resourceName: "file:userdata/api-prod-refresh.sh"},
	} {
		provider.calls = 0
		provider.lastRequest = resourcelogs.Request{}

		_, err := options.OverviewDeploymentResourceLogs(context.Background(), "api-prod", resource.resourceType, resource.resourceName, 0)
		if err != nil {
			t.Fatalf("expected asg instance refresh fallback resource binding for %s/%s, got %v", resource.resourceType, resource.resourceName, err)
		}
		if provider.calls != 1 {
			t.Fatalf("expected provider to run once for %s/%s, got %d calls", resource.resourceType, resource.resourceName, provider.calls)
		}
		if provider.lastRequest.ResourceType != resource.resourceType || provider.lastRequest.ResourceName != resource.resourceName {
			t.Fatalf("unexpected resource log request for %s/%s: %+v", resource.resourceType, resource.resourceName, provider.lastRequest)
		}
		if resource.resourceType == "userDataBootstrapSource" {
			if provider.lastRequest.BackingResourceType != "serviceAutoScalingGroup" || provider.lastRequest.BackingResourceName != "api-prod-service" {
				t.Fatalf("expected refresh bootstrap-source fallback to target service ASG, got %+v", provider.lastRequest)
			}
		}
	}
}

func TestAPIOptionsOverviewDeploymentResourceInspectionWiresObserverRoleAndPlacement(t *testing.T) {
	repoRoot := writeRepositoryFixture(t)
	store := &stubStatusStore{
		getSnapshot: statusstore.Snapshot{
			DeploymentName: "api-prod",
			Status: domain.DeploymentStatus{
				Rollout: &domain.RolloutSummary{Execution: []domain.RolloutExecutionDetail{{Resource: "ecsService", Name: "api-prod-live-service", Description: "live ECS service"}}},
			},
		},
	}
	provider := &fakeResourceInspectionProvider{result: resourceinspection.Result{Available: true}}
	restore := stubResourceInspectionProviderFactory(provider)
	defer restore()

	options := newAPIOptions(buildinfo.Info{Version: "dev", Commit: "abc123"}, repoRoot, store)
	if options.OverviewDeploymentResourceInspection == nil {
		t.Fatalf("expected overview deployment resource inspection provider")
	}

	result, err := options.OverviewDeploymentResourceInspection(context.Background(), "api-prod", "ecsService", "api-prod-live-service")
	if err != nil {
		t.Fatalf("OverviewDeploymentResourceInspection returned error: %v", err)
	}
	if !result.Available {
		t.Fatalf("expected wired provider result, got %+v", result)
	}
	want := resourceinspection.Request{
		DeploymentName:      "api-prod",
		ResourceType:        "ecsService",
		ResourceName:        "api-prod-live-service",
		ResourceDescription: "live ECS service",
		ControlAccountID:    "123456789012",
		TargetAccountID:     "123456789012",
		Region:              "ap-northeast-2",
		ObserverRoleARN:     "arn:aws:iam::123456789012:role/TenetObserverRole",
	}
	if !reflect.DeepEqual(provider.lastRequest, want) {
		t.Fatalf("unexpected resource inspection request: %+v", provider.lastRequest)
	}
}

func TestAPIOptionsOverviewDeploymentResourceInspectionRejectsResourcesOutsideDeploymentExecution(t *testing.T) {
	repoRoot := writeRepositoryFixture(t)
	store := &stubStatusStore{
		getSnapshot: statusstore.Snapshot{
			DeploymentName: "api-prod",
			Status: domain.DeploymentStatus{
				Rollout: &domain.RolloutSummary{Execution: []domain.RolloutExecutionDetail{{Resource: "ecsService", Name: "api-prod-live-service", Description: "live ECS service"}}},
			},
		},
	}
	provider := &fakeResourceInspectionProvider{}
	restore := stubResourceInspectionProviderFactory(provider)
	defer restore()

	options := newAPIOptions(buildinfo.Info{Version: "dev", Commit: "abc123"}, repoRoot, store)
	_, err := options.OverviewDeploymentResourceInspection(context.Background(), "api-prod", "ecsService", "other-service")
	if !errors.Is(err, statusstore.ErrNotFound) {
		t.Fatalf("expected not found for unbound resource, got %v", err)
	}
	if provider.calls != 0 {
		t.Fatalf("expected provider not to run for unbound resource, got %d calls", provider.calls)
	}
}

func TestAPIOptionsOverviewDeploymentResourceInspectionUsesFallbackWhenExecutionIsMissing(t *testing.T) {
	repoRoot := writeCanaryEC2RepositoryFixture(t)
	provider := &fakeResourceInspectionProvider{result: resourceinspection.Result{Available: true}}
	restore := stubResourceInspectionProviderFactory(provider)
	defer restore()

	options := newAPIOptions(buildinfo.Info{Version: "dev", Commit: "abc123"}, repoRoot, nil)
	for _, resource := range []struct {
		resourceType        string
		resourceName        string
		resourceDescription string
	}{
		{
			resourceType:        "candidateAutoScalingGroup",
			resourceName:        "api-prod-candidate",
			resourceDescription: "Candidate Auto Scaling Group Tenet creates from normalized userdata source file:userdata/api-prod.sh with desired 4, min 2, and max 8 instances and warmup 90s for weighted target group rollout",
		},
		{
			resourceType:        "trafficShiftPlan",
			resourceName:        "api-prod-weighted-routing",
			resourceDescription: "Weighted routing plan Tenet follows to shift a canary slice of production traffic from the active target group to the candidate target group before full promotion while respecting warmup 90s",
		},
		{
			resourceType:        "userDataBootstrapSource",
			resourceName:        "file:userdata/api-prod.sh",
			resourceDescription: "Normalized userdata bootstrap source for candidate EC2 capacity",
		},
	} {
		provider.calls = 0
		provider.lastRequest = resourceinspection.Request{}

		_, err := options.OverviewDeploymentResourceInspection(context.Background(), "api-prod", resource.resourceType, resource.resourceName)
		if err != nil {
			t.Fatalf("expected fallback resource binding to work for %s/%s, got %v", resource.resourceType, resource.resourceName, err)
		}
		if provider.calls != 1 {
			t.Fatalf("expected provider to run once for fallback-bound %s/%s, got %d calls", resource.resourceType, resource.resourceName, provider.calls)
		}
		if provider.lastRequest.ResourceType != resource.resourceType || provider.lastRequest.ResourceName != resource.resourceName || provider.lastRequest.ResourceDescription != resource.resourceDescription {
			t.Fatalf("unexpected resource inspection request for %s/%s: %+v", resource.resourceType, resource.resourceName, provider.lastRequest)
		}
		if resource.resourceType == "userDataBootstrapSource" {
			if provider.lastRequest.BackingResourceType != "candidateAutoScalingGroup" || provider.lastRequest.BackingResourceName != "api-prod-candidate" {
				t.Fatalf("expected bootstrap-source fallback to target candidate ASG, got %+v", provider.lastRequest)
			}
		}
	}
}

func TestAPIOptionsOverviewDeploymentResourceInspectionUsesASGInstanceRefreshFallbackWhenExecutionIsMissing(t *testing.T) {
	repoRoot := writeRollingEC2RepositoryFixture(t)
	provider := &fakeResourceInspectionProvider{result: resourceinspection.Result{Available: true}}
	restore := stubResourceInspectionProviderFactory(provider)
	defer restore()

	options := newAPIOptions(buildinfo.Info{Version: "dev", Commit: "abc123"}, repoRoot, nil)
	for _, resource := range []struct {
		resourceType        string
		resourceName        string
		resourceDescription string
	}{
		{
			resourceType:        "serviceAutoScalingGroup",
			resourceName:        "api-prod-service",
			resourceDescription: "Service Auto Scaling Group Tenet refreshes in place",
		},
		{
			resourceType:        "userDataBootstrapSource",
			resourceName:        "file:userdata/api-prod-refresh.sh",
			resourceDescription: "Normalized userdata bootstrap source for refreshed EC2 instances",
		},
	} {
		provider.calls = 0
		provider.lastRequest = resourceinspection.Request{}

		_, err := options.OverviewDeploymentResourceInspection(context.Background(), "api-prod", resource.resourceType, resource.resourceName)
		if err != nil {
			t.Fatalf("expected instance-refresh fallback resource binding to work for %s/%s, got %v", resource.resourceType, resource.resourceName, err)
		}
		if provider.calls != 1 {
			t.Fatalf("expected provider to run once for %s/%s, got %d calls", resource.resourceType, resource.resourceName, provider.calls)
		}
		if provider.lastRequest.ResourceType != resource.resourceType || provider.lastRequest.ResourceName != resource.resourceName || provider.lastRequest.ResourceDescription != resource.resourceDescription {
			t.Fatalf("unexpected resource inspection request for %s/%s: %+v", resource.resourceType, resource.resourceName, provider.lastRequest)
		}
		if resource.resourceType == "userDataBootstrapSource" {
			if provider.lastRequest.BackingResourceType != "serviceAutoScalingGroup" || provider.lastRequest.BackingResourceName != "api-prod-service" {
				t.Fatalf("expected refresh bootstrap-source fallback to target service ASG, got %+v", provider.lastRequest)
			}
		}
	}
}

func TestAPIOptionsOverviewDeploymentResourceInspectionUsesCodeDeployFallbackWhenExecutionIsMissing(t *testing.T) {
	repoRoot := writeRepositoryFixture(t)
	provider := &fakeResourceInspectionProvider{result: resourceinspection.Result{Available: true}}
	restore := stubResourceInspectionProviderFactory(provider)
	defer restore()

	options := newAPIOptions(buildinfo.Info{Version: "dev", Commit: "abc123"}, repoRoot, nil)
	for _, resource := range []struct {
		resourceType        string
		resourceName        string
		resourceDescription string
	}{
		{
			resourceType:        "codeDeployApplication",
			resourceName:        "api-prod-codedeploy-app",
			resourceDescription: "CodeDeploy application coordinating ECS traffic shifting (style: linear)",
		},
		{
			resourceType:        "codeDeployDeploymentGroup",
			resourceName:        "api-prod-codedeploy-group",
			resourceDescription: "CodeDeploy deployment group Tenet targets for ECS rollout (style: linear)",
		},
		{
			resourceType:        "productionListener",
			resourceName:        "api-prod-prod-listener",
			resourceDescription: "Production listener CodeDeploy shifts during ECS traffic shifting (style: linear)",
		},
		{
			resourceType:        "testListener",
			resourceName:        "api-prod-test-listener",
			resourceDescription: "Test listener CodeDeploy uses to validate the replacement ECS task set before promotion (style: linear)",
		},
	} {
		provider.calls = 0
		provider.lastRequest = resourceinspection.Request{}

		_, err := options.OverviewDeploymentResourceInspection(context.Background(), "api-prod", resource.resourceType, resource.resourceName)
		if err != nil {
			t.Fatalf("expected CodeDeploy fallback resource binding to work for %s/%s, got %v", resource.resourceType, resource.resourceName, err)
		}
		if provider.calls != 1 {
			t.Fatalf("expected provider to run once for fallback-bound %s/%s, got %d calls", resource.resourceType, resource.resourceName, provider.calls)
		}
		if provider.lastRequest.ResourceType != resource.resourceType || provider.lastRequest.ResourceName != resource.resourceName || provider.lastRequest.ResourceDescription != resource.resourceDescription {
			t.Fatalf("unexpected resource inspection request for %s/%s: %+v", resource.resourceType, resource.resourceName, provider.lastRequest)
		}
	}
}

type stubStatusStore struct {
	getSnapshot   statusstore.Snapshot
	listSnapshots []statusstore.Snapshot
	getErr        error
	listErr       error
	gotGetName    string
	getCalls      int
	listCalls     int
}

func (s *stubStatusStore) List(context.Context) ([]statusstore.Snapshot, error) {
	s.listCalls++
	if s.listErr != nil {
		return nil, s.listErr
	}
	if s.listSnapshots != nil {
		return s.listSnapshots, nil
	}
	return []statusstore.Snapshot{s.getSnapshot}, nil
}

func (s *stubStatusStore) Get(_ context.Context, name string) (statusstore.Snapshot, error) {
	s.getCalls++
	s.gotGetName = name
	if s.getErr != nil {
		return statusstore.Snapshot{}, s.getErr
	}
	return s.getSnapshot, nil
}

func (s *stubStatusStore) Save(context.Context, []statusstore.Snapshot) error {
	return nil
}

type fakeResourceLogProvider struct {
	result      resourcelogs.Result
	err         error
	lastRequest resourcelogs.Request
	calls       int
}

func (p *fakeResourceLogProvider) Recent(_ context.Context, req resourcelogs.Request) (resourcelogs.Result, error) {
	p.calls++
	p.lastRequest = req
	return p.result, p.err
}

func stubResourceLogProviderFactory(provider resourceLogProvider) func() {
	original := newResourceLogProvider
	newResourceLogProvider = func() resourceLogProvider { return provider }
	return func() { newResourceLogProvider = original }
}

type fakeResourceInspectionProvider struct {
	result      resourceinspection.Result
	err         error
	lastRequest resourceinspection.Request
	calls       int
}

func (p *fakeResourceInspectionProvider) Inspect(_ context.Context, req resourceinspection.Request) (resourceinspection.Result, error) {
	p.calls++
	p.lastRequest = req
	return p.result, p.err
}

func stubResourceInspectionProviderFactory(provider resourceInspectionProvider) func() {
	original := newResourceInspectionProvider
	newResourceInspectionProvider = func() resourceInspectionProvider { return provider }
	return func() { newResourceInspectionProvider = original }
}

func writeRepositoryFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeJSON(t, filepath.Join(root, "accounts", "prod-core.json"), `{"name":"prod-core","awsAccountId":"123456789012","roleArn":"arn:aws:iam::123456789012:role/TenetDeployerRole","observerRoleArn":"arn:aws:iam::123456789012:role/TenetObserverRole","allowedRegions":["ap-northeast-2"]}`)
	writeJSON(t, filepath.Join(root, "environments", "prod-apne2.json"), `{"name":"prod-apne2","accountRef":"prod-core","region":"ap-northeast-2","networkRef":"prod-main"}`)
	writeJSON(t, filepath.Join(root, "deployments", "api-prod.json"), `{"name":"api-prod","image":"payments-api:v2","target":{"type":"ecs"},"stacks":[{"stack":"api-prod","account":"prod-core","env":"prod-apne2","replacement_type":"linear","capacity":{"desired":1,"min":1,"max":1},"regions":[{"region":"ap-northeast-2"}]}]}`)
	writeJSON(t, filepath.Join(root, "deployments", "worker-prod.json"), `{"name":"worker-prod","image":"payments-worker:v2","target":{"type":"ecs"},"stacks":[{"stack":"worker-prod","account":"prod-core","env":"prod-apne2","replacement_type":"linear","capacity":{"desired":1,"min":1,"max":1},"regions":[{"region":"ap-northeast-2"}]}]}`)
	return root
}

func writeCanaryEC2RepositoryFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeJSON(t, filepath.Join(root, "accounts", "prod-core.json"), `{"name":"prod-core","awsAccountId":"123456789012","roleArn":"arn:aws:iam::123456789012:role/TenetDeployerRole","observerRoleArn":"arn:aws:iam::123456789012:role/TenetObserverRole","allowedRegions":["ap-northeast-2"]}`)
	writeJSON(t, filepath.Join(root, "environments", "prod-apne2.json"), `{"name":"prod-apne2","accountRef":"prod-core","region":"ap-northeast-2","networkRef":"prod-main"}`)
	writeJSON(t, filepath.Join(root, "deployments", "api-prod.json"), `{"name":"api-prod","appRef":"payments-api","environmentRef":"prod-apne2","target":{"type":"ec2"},"strategy":{"type":"canary","warmupSeconds":90},"traffic":{"port":443},"capacity":{"desired":4,"min":2,"max":8},"binary":"s3://releases.example.com/payments-api/prod/payments-api-linux-amd64","userdata":{"type":"file","path":"userdata/api-prod.sh"}}`)
	return root
}

func writeGitUserDataCanaryEC2RepositoryFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeJSON(t, filepath.Join(root, "accounts", "prod-core.json"), `{"name":"prod-core","awsAccountId":"123456789012","roleArn":"arn:aws:iam::123456789012:role/TenetDeployerRole","observerRoleArn":"arn:aws:iam::123456789012:role/TenetObserverRole","allowedRegions":["ap-northeast-2"]}`)
	writeJSON(t, filepath.Join(root, "environments", "prod-apne2.json"), `{"name":"prod-apne2","accountRef":"prod-core","region":"ap-northeast-2","networkRef":"prod-main"}`)
	writeJSON(t, filepath.Join(root, "deployments", "api-prod.json"), `{"name":"api-prod","appRef":"payments-api","environmentRef":"prod-apne2","target":{"type":"ec2"},"strategy":{"type":"canary","warmupSeconds":90},"traffic":{"port":443},"capacity":{"desired":4,"min":2,"max":8},"binary":"s3://releases.example.com/payments-api/prod/payments-api-linux-amd64","userdata":{"type":"git","path":"userdata/api-prod.sh"}}`)
	return root
}

func writeRollingEC2RepositoryFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeJSON(t, filepath.Join(root, "accounts", "prod-core.json"), `{"name":"prod-core","awsAccountId":"123456789012","roleArn":"arn:aws:iam::123456789012:role/TenetDeployerRole","observerRoleArn":"arn:aws:iam::123456789012:role/TenetObserverRole","allowedRegions":["ap-northeast-2"]}`)
	writeJSON(t, filepath.Join(root, "environments", "prod-apne2.json"), `{"name":"prod-apne2","accountRef":"prod-core","region":"ap-northeast-2","networkRef":"prod-main"}`)
	writeJSON(t, filepath.Join(root, "deployments", "api-prod.json"), `{"name":"api-prod","appRef":"payments-api","environmentRef":"prod-apne2","target":{"type":"ec2"},"strategy":{"type":"rolling"},"traffic":{"port":443},"capacity":{"desired":2,"min":1,"max":3},"userdata":{"type":"file","path":"userdata/api-prod-refresh.sh"}}`)
	return root
}

func writeRollingECSRepositoryFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeJSON(t, filepath.Join(root, "accounts", "prod-core.json"), `{"name":"prod-core","awsAccountId":"123456789012","roleArn":"arn:aws:iam::123456789012:role/TenetDeployerRole","observerRoleArn":"arn:aws:iam::123456789012:role/TenetObserverRole","allowedRegions":["ap-northeast-2"]}`)
	writeJSON(t, filepath.Join(root, "environments", "prod-apne2.json"), `{"name":"prod-apne2","accountRef":"prod-core","region":"ap-northeast-2","networkRef":"prod-main"}`)
	writeJSON(t, filepath.Join(root, "deployments", "api-prod.json"), `{"name":"api-prod","appRef":"payments-api","environmentRef":"prod-apne2","target":{"type":"ecs"},"strategy":{"type":"rolling","minHealthyPercent":90,"warmupSeconds":30},"capacity":{"desired":2,"min":1,"max":2}}`)
	return root
}

func writeJSON(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("failed to create directory: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write %s: %v", path, err)
	}
}
