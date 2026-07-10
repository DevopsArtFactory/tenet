package api

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func repoRootForContractTest(t *testing.T) string {
	t.Helper()

	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve current test file")
	}

	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
}

func readRepoFile(t *testing.T, relativePath string) string {
	t.Helper()

	body, err := os.ReadFile(filepath.Join(repoRootForContractTest(t), relativePath))
	if err != nil {
		t.Fatalf("read %s: %v", relativePath, err)
	}

	return string(body)
}

func redactedAuthProviderPlaceholder() string {
	return "TENET_AUTH_PROVIDER=" + strings.Repeat("*", 3)
}

func concreteAuthProviderModeExamples() []string {
	return []string{
		"`TENET_AUTH_PROVIDER=github`",
		"`TENET_AUTH_PROVIDER=google_oidc`",
		"`TENET_AUTH_PROVIDER=okta_oidc`",
		"`TENET_AUTH_PROVIDER=okta_saml`",
	}
}

func TestReadmeStaysProductOverview(t *testing.T) {
	readme := readRepoFile(t, "README.md")
	lineCount := strings.Count(readme, "\n")
	if !strings.HasSuffix(readme, "\n") {
		lineCount++
	}
	if lineCount > 350 {
		t.Fatalf("expected README to remain a product overview under 350 lines; got %d", lineCount)
	}

	for _, fragment := range []string{
		"Tenet is a repository-driven deployment control plane",
		"## Product Overview",
		"## Run Locally",
		"## Operator UI Entry Points",
		"## Documentation Map",
		"docs/architecture/tenet-v1-operator-contracts.md",
		"endpoint, read-model, auth, resource, and smoke-verification contracts formerly\n  kept in this README",
		"`internal/statusstore`, `internal/planpreview`, `internal/applyrequest`, and\n  `internal/auditlog` - runtime status, plan/apply, and audit stores",
		"`docs/features/resource-inspection.md` documents the focused live resource inspection route, selectors, lookup context, unavailable states, and detailed field contracts.",
		"Resource drill-down Recent logs and Live inspection panels summarize lookup context, bounded reads, request correlation, and unavailable states without duplicating the detailed resource field contract.",
		"live ASG reads may add optional\n  `ec2InstanceStatus` status-check evidence",
	} {
		if !strings.Contains(readme, fragment) {
			t.Fatalf("expected README product overview to include %q", fragment)
		}
	}

	if strings.Contains(readme, "`internal/runtime`") {
		t.Fatal("expected README repository layout to name current runtime-store packages instead of nonexistent internal/runtime")
	}
}

func TestECSEventRowHandoffOperatorContract(t *testing.T) {
	files := map[string][]string{
		"README.md": {
			"Use `Share ECS service event` or the adjacent `Copy ECS service event link` from a Live inspection event row to hand off one ECS service event without changing the resource-page selector state.",
		},
		"docs/features/resource-inspection.md": {
			"`ecsService.events[].eventId`",
			"When ECS service events are present",
			"`Share ECS service event` links to a same-page row anchor for one ECS service event",
			"`Copy ECS service event link` copies the same stable handoff URL without navigating away",
			"eventId-backed anchors remain stable across refreshes",
		},
		"docs/features/overview-dashboard.md": {
			"`Share ECS service event` or `Copy ECS service event link` on the Live inspection event row",
			"preserves the current resource page query state and changes only the hash",
		},
		"scripts/smoke-local.sh": {
			"assert_ecs_service_event_ids('worker_resource_inspection', worker_resource_inspection)",
			"assert_ecs_service_event_ids('selected_worker_resource_inspection', selected_worker_resource_inspection)",
			"createCopyLinkButton(eventShareURL, 'Copy ECS service event link', 'Copy ECS service event share link')",
		},
		"scripts/smoke-docker.sh": {
			"assert_ecs_service_event_ids(\"worker_resource_inspection\", worker_resource_inspection)",
			"assert_ecs_service_event_ids(\"selected_worker_resource_inspection\", selected_worker_resource_inspection)",
			"createCopyLinkButton(eventShareURL, 'Copy ECS service event link', 'Copy ECS service event share link')",
		},
	}

	for path, fragments := range files {
		body := readRepoFile(t, path)
		for _, fragment := range fragments {
			if !strings.Contains(body, fragment) {
				t.Fatalf("expected %s to include ECS service event-row handoff contract %q", path, fragment)
			}
		}
	}
}

func TestTrustedFrontDoorDocsUseReadableBooleanExamples(t *testing.T) {
	docPaths := []string{
		"README.md",
		"docs/architecture/tenet-v1-local-runtime.md",
		"docs/architecture/tenet-v1-operator-contracts.md",
		"docs/architecture/tenet-v1-audit-log-api.md",
		"docs/architecture/tenet-v1-deployment-status-api.md",
		"docs/architecture/tenet-v1-overview-ui.md",
		"docs/architecture/tenet-v1-runtime-rollout-summary.md",
		"docs/architecture/tenet-v1-aws-central-control-plane.md",
		"docs/features/overview-dashboard.md",
	}
	redactedPlaceholder := strings.Repeat("*", 3)
	for _, path := range docPaths {
		doc := readRepoFile(t, path)
		for _, toggle := range []string{
			"TENET_API_TRUST_AUTHENTICATED_HEADERS",
			"TENET_API_ENFORCE_AUTHENTICATED_IDENTITY",
			"TENET_API_ENFORCE_OPERATOR_ROLE_AUTHORIZATION",
			"TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION",
		} {
			placeholder := toggle + "=" + redactedPlaceholder
			if strings.Contains(doc, placeholder) {
				t.Fatalf("expected %s to use operator-readable boolean examples instead of placeholder %q", path, placeholder)
			}
			unbalancedInlineExample := "`" + toggle + "=true "
			if strings.Contains(doc, unbalancedInlineExample) {
				t.Fatalf("expected %s to close non-secret auth-toggle boolean inline code before prose after %q", path, unbalancedInlineExample)
			}
		}
	}

	localRuntimeDoc := readRepoFile(t, "docs/architecture/tenet-v1-local-runtime.md")
	if !strings.Contains(localRuntimeDoc, "The auth/RBAC toggles below are boolean examples. Set them to `true` only on API instances that are reachable exclusively through that trusted front door.") {
		t.Fatal("expected local runtime trusted-front-door docs to explain that auth/RBAC toggles are boolean examples")
	}

	for _, fragment := range []string{
		"`TENET_API_TRUST_AUTHENTICATED_HEADERS=true` trusts front-door-forwarded",
		"`TENET_API_ENFORCE_AUTHENTICATED_IDENTITY=true` requires authenticated identity",
		"`TENET_API_ENFORCE_OPERATOR_ROLE_AUTHORIZATION=true` trusts `X-Tenet-Authenticated-Roles`",
		"`TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION=true` trusts `X-Tenet-Authorized-Deployments`",
	} {
		if !strings.Contains(localRuntimeDoc, fragment) {
			t.Fatalf("expected local runtime trusted-front-door docs to include copyable boolean example %q", fragment)
		}
	}

	centralControlPlaneDoc := readRepoFile(t, "docs/architecture/tenet-v1-aws-central-control-plane.md")
	for _, fragment := range []string{
		"If you enable `TENET_API_TRUST_AUTHENTICATED_HEADERS=true`, `TENET_API_ENFORCE_AUTHENTICATED_IDENTITY=true`, and/or `TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION=true`, keep that `tenet-api` instance behind the same authenticating front door.",
		"The proxy/load balancer layer must strip any inbound `X-Tenet-Authenticated-*`, `X-Tenet-Authorized-Deployments`, `X-Tenet-Authorized-Projects`, `X-Tenet-Authorized-Environments`, and trusted-alias `X-User-*` headers from untrusted clients",
		"or callers can spoof operator identity or deployment access by sending those headers directly.",
	} {
		if !strings.Contains(centralControlPlaneDoc, fragment) {
			t.Fatalf("expected central-control-plane trusted-front-door docs to include warning fragment %q", fragment)
		}
	}

	overviewUIDoc := readRepoFile(t, "docs/architecture/tenet-v1-overview-ui.md")
	for _, fragment := range []string{
		"`TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION=true` is also enabled",
		"or matching `X-Tenet-Authorized-Projects` plus `X-Tenet-Authorized-Environments` catalog placement for that target",
	} {
		if !strings.Contains(overviewUIDoc, fragment) {
			t.Fatalf("expected overview UI auth docs to include guarded-read fragment %q", fragment)
		}
	}
	if strings.Contains(overviewUIDoc, "`TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION=true is also enabled") {
		t.Fatal("expected overview UI guarded-read auth toggle example to close inline code before prose")
	}

	operatorContractsDoc := readRepoFile(t, "docs/architecture/tenet-v1-operator-contracts.md")
	if !strings.Contains(operatorContractsDoc, "`GET /auth/login/github`, `GET /auth/login/google`, or `GET /auth/login/okta` creates one-time callback state") {
		t.Fatal("expected operator contracts doc auth runtime boundary to state public GitHub login route is shipped")
	}
	if !strings.Contains(operatorContractsDoc, "reports `authRuntime.tenetManagedLogin.active=true` from `GET /v1/operator/context`") {
		t.Fatal("expected operator contracts doc auth runtime boundary to state auth-provider runtime settings activate the diagnostic")
	}
	for _, fragment := range []string{
		"`GET /auth/login/github`, `GET /auth/login/google`, or `GET /auth/login/okta` creates one-time callback state",
		"reports `GET /v1/operator/context.authRuntime.tenetManagedLogin.active=true`",
		"When any setting is absent the runtime stays disabled",

		"The active managed-login runtime supports GitHub OAuth, Google Workspace OIDC, and Okta OIDC today",
		"Okta SAML provider selection remains diagnostic-only, while provider-token refresh/revocation, upstream provider/front-door logout, and long-lived token lifecycle management remain future work",

		"non-secret `requiredSettings` and `missingSettings`",
		"`sessionCookieSecureOverride`, `sessionCookieSecureForRequest`, and `sessionCookieSecureSources`",
		"`sessionCookiePath=/`, `sessionCookieHttpOnly=true`, `sessionCookieSameSite=Lax`",
		"Treat managed-login release coverage as two complementary checks",
		"without live GitHub, Google, or Okta credentials",
	} {
		if !strings.Contains(localRuntimeDoc, fragment) {
			t.Fatalf("expected local runtime auth groundwork boundary to include %q", fragment)
		}
	}
}

func TestLocalRuntimeDocsDescribeComposeRedisStatusStore(t *testing.T) {
	localRuntimeDoc := readRepoFile(t, "docs/architecture/tenet-v1-local-runtime.md")
	for _, fragment := range []string{
		"`compose.yaml` wires both processes plus Redis together",
		"`TENET_STATUS_BACKEND=redis`",
		"`TENET_REDIS_ADDR=redis:6379`",
		"`TENET_STATUS_SEED_PATH=/workspace/examples/local-status/deployments.json`",
		"Redis-backed status store can seed from the local deployment snapshot when empty",
	} {
		if !strings.Contains(localRuntimeDoc, fragment) {
			t.Fatalf("expected local runtime Compose status-store docs to include %q", fragment)
		}
	}

	if strings.Contains(localRuntimeDoc, "`TENET_STATUS_PATH=/workspace/examples/local-status/deployments.json`") {
		t.Fatal("expected local runtime Compose contract to document Redis seed inputs instead of TENET_STATUS_PATH")
	}
}

func TestEnterpriseSSOBoundaryDocs(t *testing.T) {
	operatorContractsDoc := readRepoFile(t, "docs/architecture/tenet-v1-operator-contracts.md")
	redactedProviderSetting := redactedAuthProviderPlaceholder()
	for _, source := range []struct {
		name     string
		body     string
		examples []string
	}{
		{name: "README.md", body: readRepoFile(t, "README.md"), examples: concreteAuthProviderModeExamples()},
		{name: "docs/architecture/tenet-v1-operator-contracts.md", body: operatorContractsDoc, examples: concreteAuthProviderModeExamples()},
		{name: "docs/architecture/tenet-v1-enterprise-sso.md", body: readRepoFile(t, "docs/architecture/tenet-v1-enterprise-sso.md"), examples: concreteAuthProviderModeExamples()},
		{name: "docs/features/managed-login.md", body: readRepoFile(t, "docs/features/managed-login.md"), examples: concreteAuthProviderModeExamples()},
		{name: "scripts/smoke-local.sh", body: readRepoFile(t, "scripts/smoke-local.sh"), examples: []string{
			"TENET_AUTH_PROVIDER=google_oidc",
			"TENET_AUTH_PROVIDER=okta_oidc",
			"TENET_AUTH_PROVIDER=okta_saml",
		}},
		{name: "scripts/smoke-docker.sh", body: readRepoFile(t, "scripts/smoke-docker.sh"), examples: []string{
			"TENET_AUTH_PROVIDER=google_oidc",
			"TENET_AUTH_PROVIDER=okta_oidc",
			"TENET_AUTH_PROVIDER=okta_saml",
		}},
	} {
		if strings.Contains(source.body, redactedProviderSetting) {
			t.Fatalf("expected %s to use concrete non-secret auth provider examples instead of placeholder %q", source.name, redactedProviderSetting)
		}
		for _, providerExample := range source.examples {
			if !strings.Contains(source.body, providerExample) {
				t.Fatalf("expected %s to include concrete provider example %q", source.name, providerExample)
			}
		}
	}
	for _, fragment := range []string{
		"`docs/architecture/tenet-v1-enterprise-sso.md`",

		"keeps GitHub OAuth, Google Workspace OIDC, and Okta OIDC as active managed-login providers only when explicitly configured",
		"keeps Okta SAML diagnostic-only",
		"repository-backed RBAC policy diagnostics as the shipped first policy foundation",
		"Repository-backed policy route authorization is shipped as an explicit filesystem-catalog opt-in",
		"`TENET_API_AUTHORIZE_WITH_REPOSITORY_RBAC_POLICY=true`",
		"`TENET_AUTH_PROVIDER=github`",
		"`TENET_AUTH_PROVIDER=google_oidc`",
		"`TENET_AUTH_PROVIDER=okta_oidc`",
		"`TENET_AUTH_PROVIDER=okta_saml`",
		"When the runtime variables are complete for Google (`TENET_AUTH_PROVIDER=google_oidc`",
		"The diagnostic canonicalizes the Google Workspace runtime provider to `google` even when operators configure the preferred `google_oidc` mode",
		`"provider": "google"`,
	} {
		if !strings.Contains(operatorContractsDoc, fragment) {
			t.Fatalf("expected operator contracts doc enterprise SSO boundary to include %q", fragment)
		}
	}

	doc := readRepoFile(t, "docs/architecture/tenet-v1-enterprise-sso.md")
	localRuntimeDoc := readRepoFile(t, "docs/architecture/tenet-v1-local-runtime.md")
	repositoryPolicyOptInSentence := "Repository-backed policy parsing/evaluation for users/groups, Tenet roles, and deployment/project/environment scopes is already shipped through `GET /v1/operator/context.repositoryPolicy`, and filesystem catalog runs can explicitly opt into using those policy-derived grants for guarded route authorization with `TENET_API_AUTHORIZE_WITH_REPOSITORY_RBAC_POLICY=true`."
	remainingProviderWorkSentence := "Remaining work is provider-token/group lifecycle hardening and any future provider-specific group synchronization beyond the current request-scoped `X-Tenet-Authenticated-Groups` input."
	if !strings.Contains(doc, repositoryPolicyOptInSentence+"\n\n"+remainingProviderWorkSentence) {
		t.Fatal("expected enterprise SSO docs to keep the repository RBAC opt-in example complete and separate from remaining provider work")
	}
	for _, malformed := range []string{
		"`TENET_API_AUTHORIZE_WITH_REPOSITORY_RBAC_POLICY=true. Remaining work",
		"`TENET_API_AUTHORIZE_WITH_REPOSITORY_RBAC_POLICY=true Remaining work",
		"TENET_API_AUTHORIZE_WITH_REPOSITORY_RBAC_POLICY=***",
		"TENET_AUTH_PROVIDER=***",
	} {
		if strings.Contains(doc, malformed) {
			t.Fatalf("expected enterprise SSO docs to avoid malformed/redacted provider-mode artifact %q", malformed)
		}
	}
	for _, fragment := range []string{
		"Google Workspace/Gmail means identity login only",
		"It does not grant Tenet Gmail mailbox, Drive, Calendar, or other Google API access.",
		"Google Workspace identity through Google OIDC profile claims",
		"GitHub OAuth uses `TENET_AUTH_PROVIDER=github`",
		"Google Workspace OIDC uses `TENET_AUTH_PROVIDER=google_oidc`",
		"Okta OIDC uses `TENET_AUTH_PROVIDER=okta_oidc`",
		"Okta SAML uses `TENET_AUTH_PROVIDER=okta_saml` for diagnostic-only preflight",
		"TENET_AUTH_PROVIDER=google_oidc # use okta_oidc for Okta OIDC, okta_saml for Okta SAML diagnostics, or github for GitHub OAuth",
		"canonical runtime provider `google` (`TENET_AUTH_PROVIDER=google_oidc` is the preferred configuration mode; `google` remains a legacy alias)",
		"configured provider type (`google` in runtime diagnostics for active Google Workspace OIDC configured with `TENET_AUTH_PROVIDER=google_oidc`, plus `okta_saml` and `okta_oidc`)",
		"human-readable provider labels in operator-facing UI (`Google Workspace OIDC`, `Okta SAML`, `Okta OIDC`)",
		"provider-boundary copy in the embedded Operator context details",
		"Okta identity through SAML assertions or OIDC profile claims",
		"Okta activation checklists",
		"Okta SAML activation checklist",
		"Okta OIDC activation checklist",
		"Okta OIDC is now active only through the same managed-session/OIDC runtime shape used by Google Workspace OIDC",
		"Complete Okta OIDC settings expose same-origin `loginRoute=/auth/login/okta`, `callbackRoute=/auth/callback/okta/oidc`, and `logoutRoute=/auth/logout`",
		"raw ID/access/refresh tokens, authorization codes, and provider cookies are never persisted in identity/session stores or returned through `GET /v1/operator/context`.",
		"Tests use local fake Okta OIDC authorization-code/profile responses for successful, malformed, expired-state, and provider-error paths",
		"Okta OIDC authenticates identity only and never grants Tenet access by itself.",
		"`authRuntime.enterpriseSso.provider` stays `okta_saml`, `configured=true`, and `active=false` until every required non-secret setting and store is complete, every preflighted URL is valid, and same-origin route wiring exists.",
		"Complete Okta SAML settings expose same-origin `loginRoute=/auth/login/okta`, `callbackRoute=/auth/callback/okta/saml`, and `logoutRoute=/auth/logout`",
		"omit the logout/session-clear handoff",
		"populate `missingSettings` or `invalidSettings`",
		"Active callbacks mint only Tenet-managed session evidence through the signed `tenet_session` cookie, validate safe in-app `returnUrl` values",
		"raw assertions, metadata documents, private keys, and provider cookies are never persisted in identity/session stores or returned through `GET /v1/operator/context`.",
		"Tests use local fake metadata/assertions for successful, malformed, expired, and untrusted assertion paths",
		"Okta SAML authenticates identity only and never grants Tenet access by itself.",
		"local fake provider/assertion/profile tests instead of live Google or Okta calls",
		"Trusted front-door identity remains authoritative",
		"Diagnostics must not expose client secrets, raw ID/access/refresh tokens, SAML assertions, authorization codes, private keys, or provider cookies.",
		"Group claims are identity input only.",
		"repository-backed RBAC policy evaluation that can map users/groups to Tenet roles and deployment/project/environment scopes",
		"Route authorization still depends on Tenet's existing trusted role/deployment/project/environment enforcement toggles by default",
		"repository-backed RBAC policy evaluation becomes the guarded route authorization source",
		"This issue is not the RBAC policy redesign.",
		repositoryPolicyOptInSentence,
		remainingProviderWorkSentence,
	} {
		if !strings.Contains(doc, fragment) {
			t.Fatalf("expected enterprise SSO docs to include %q", fragment)
		}
	}

	for _, stale := range []string{
		"policy-backed route enforcement remain future work",
		"future enforcement slice explicitly opts into policy-backed authorization",
		"explicit future decision to wire policy-derived grants into route enforcement",
		"guarded routes still evaluate the request-scoped identity, roles, and exact deployment allowlist independently",
	} {
		if strings.Contains(operatorContractsDoc, stale) || strings.Contains(doc, stale) || strings.Contains(localRuntimeDoc, stale) {
			t.Fatalf("expected enterprise SSO/operator/local-runtime docs to avoid stale repository-policy enforcement wording %q", stale)
		}
	}

	for _, source := range []struct {
		name string
		body string
	}{
		{name: "docs/architecture/tenet-v1-operator-contracts.md", body: operatorContractsDoc},
		{name: "docs/architecture/tenet-v1-local-runtime.md", body: localRuntimeDoc},
	} {
		if !strings.Contains(source.body, "guarded route authorization is still evaluated by Tenet on the server, using trusted front-door headers by default or repository-policy-derived roles and grants only when repository-policy authorization is explicitly enabled") {
			t.Fatalf("expected %s to clarify repository-policy authorization source", source.name)
		}
	}

	for _, source := range []struct {
		name string
		body string
	}{
		{name: "docs/architecture/tenet-v1-operator-contracts.md", body: operatorContractsDoc},
		{name: "docs/architecture/tenet-v1-enterprise-sso.md", body: doc},
		{name: "docs/features/managed-login.md", body: readRepoFile(t, "docs/features/managed-login.md")},
		{name: "README.md", body: readRepoFile(t, "README.md")},
		{name: "scripts/smoke-local.sh", body: readRepoFile(t, "scripts/smoke-local.sh")},
		{name: "scripts/smoke-docker.sh", body: readRepoFile(t, "scripts/smoke-docker.sh")},
	} {
		if strings.Contains(source.body, redactedProviderSetting) {
			t.Fatalf("expected %s to use concrete non-secret auth provider examples instead of placeholder %q", source.name, redactedProviderSetting)
		}
	}
}

func TestManagedLoginFeatureGuideDocs(t *testing.T) {
	readme := readRepoFile(t, "README.md")
	if !strings.Contains(readme, "`docs/features/managed-login.md` documents managed-login activation, provider routes, cookie-security diagnostics, logout boundaries, and trusted-front-door/policy relationships.") {
		t.Fatal("expected README documentation map to point operators at the managed-login troubleshooting guide")
	}

	doc := readRepoFile(t, "docs/features/managed-login.md")
	redactedProviderSetting := redactedAuthProviderPlaceholder()
	if strings.Contains(doc, redactedProviderSetting) {
		t.Fatalf("expected managed-login guide to use concrete non-secret provider modes instead of placeholder %q", redactedProviderSetting)
	}

	for _, fragment := range []string{
		"# Managed Login",
		"Operator troubleshooting guide",
		"Managed login is active only when `GET /v1/operator/context` reports `authRuntime.tenetManagedLogin.active=true`",
		"`TENET_AUTH_PROVIDER=github`",
		"`TENET_AUTH_PROVIDER=google_oidc`",
		"`TENET_AUTH_PROVIDER=okta_oidc`",
		"`TENET_AUTH_PROVIDER=okta_saml` is diagnostic-only",
		"`GET /auth/login/github`",
		"`GET /auth/login/google`",
		"`GET /auth/login/okta`",
		"`GET /auth/callback/github`",
		"`GET /auth/callback/google`",
		"`GET /auth/callback/okta/oidc`",
		"`GET /auth/callback/okta/saml`",
		"`GET /auth/logout` and `POST /auth/logout`",
		"`TENET_AUTH_SESSION_COOKIE_SECURE=true`",
		"Use `GET /v1/operator/context` and inspect `authRuntime.tenetManagedLogin` to diagnose cookie behavior.",
		"`sessionCookieSecureOverride`, `sessionCookieSecureForRequest`, and `sessionCookieSecureSources`",
		"HTTPS request evidence or trusted `X-Forwarded-Proto: https` / `Forwarded: proto=https` evidence",
		"Logout clears only Tenet's signed `tenet_session` cookie",
		"does not revoke provider tokens, clear GitHub/Google/Okta sessions, or sign out an upstream trusted front door",
		"Trusted front-door identity still wins when trusted headers are present",
		"Managed login authenticates the browser operator; it does not grant Tenet roles or deployment access by itself.",
		"Repository-backed RBAC policy diagnostics can explain how users or groups map to Tenet roles and deployment/project/environment scopes",
		"route enforcement still depends on Tenet's trusted role and deployment/project/environment authorization settings",
	} {
		if !strings.Contains(doc, fragment) {
			t.Fatalf("expected managed-login guide to include %q", fragment)
		}
	}
}

func TestOperatorTroubleshootingRunbookDocsContract(t *testing.T) {
	readme := readRepoFile(t, "README.md")
	if !strings.Contains(readme, "`docs/runbooks/operator-troubleshooting.md` documents the auth, route, evidence, state, and verification troubleshooting quick-reference.") {
		t.Fatal("expected README documentation map to point operators at the operator troubleshooting runbook")
	}

	runbook := readRepoFile(t, "docs/runbooks/operator-troubleshooting.md")
	for _, staleFragment := range []string{
		"raw API for active handoffs",
		"embedded monitor remains read-only over the queued subset",
	} {
		if strings.Contains(runbook, staleFragment) {
			t.Fatalf("expected operator troubleshooting runbook to drop stale apply-monitor wording %q", staleFragment)
		}
	}

	for _, fragment := range []string{
		"# Operator Troubleshooting",
		"Compact operator troubleshooting quick-reference",
		"Use this page for route/evidence/state/verification checks",
		"| Surface | UI route | Backing route(s) | Request-Id / lookup-context evidence | Expected local/no-AWS state | Verify |",
		"`GET /v1/operator/context`",
		"`authRuntime.tenetManagedLogin`",
		"Managed login authenticates browser identity; it does not grant Tenet roles or deployment access.",
		"| Applications overview | `/` | `GET /v1/overview/applications`",
		"| Application detail | `/?page=application&deployment={deploymentName}` | `GET /v1/overview/applications/{deploymentName}`",
		"| Resource detail | `/?page=resource&deployment={deploymentName}&resourceType={type}&resourceId={stableResourceId}` | `GET /v1/overview/deployments/{deploymentName}`",
		"| Recent logs | `#resource-recent-logs` | `GET /v1/overview/deployments/{deploymentName}/resource-logs?resourceType={type}&resourceName={name}`",
		"| Live inspection | `#resource-live-inspection` | `GET /v1/overview/deployments/{deploymentName}/resource-inspection?resourceType={type}&resourceName={name}`",
		"| Audit | `/?page=audit[&deployment={deploymentName}][&action={action}][&limit={n}]` | `GET /v1/audit-logs`",
		"| Plan/apply | `/?page=plan&deployment={deploymentName}[&planId={planId}][&applyId={applyId}]` | `POST /v1/plans/deployments/{deploymentName}`",
		"| Apply collections | `/?page=applies` for the queued monitor; `/?page=applies&appliesView=active` for active handoffs | `GET /v1/applies?status=active`; `GET /v1/applies?status=queued`",
		"The active API returns queued plus running records newest-first; the embedded monitor exposes the queued/active view control",
		"`Request-Id` / `Tenet request ID`",
		"`lookupContext.region`",
		"`available=false` / `unavailableReason=aws_unavailable`",
		"`available=false` / `unavailableReason=no_log_source`",
		"`status=filesystem_noop`",
		"`status=no_history`",
		"`make test`",
		"`make smoke-local`",
		"`make smoke-docker`",
	} {
		if !strings.Contains(runbook, fragment) {
			t.Fatalf("expected operator troubleshooting runbook to include %q", fragment)
		}
	}

	overviewDoc := readRepoFile(t, "docs/features/overview-dashboard.md")
	for _, fragment := range []string{
		"Operator troubleshooting quick-reference",
		"`docs/runbooks/operator-troubleshooting.md`",
		"compact route/evidence/state/verification matrix",
	} {
		if !strings.Contains(overviewDoc, fragment) {
			t.Fatalf("expected overview dashboard docs to cross-link operator troubleshooting quick-reference with %q", fragment)
		}
	}

	recentLogsDoc := readRepoFile(t, "docs/features/resource-recent-logs.md")
	for _, fragment := range []string{
		"Operator troubleshooting quick-reference",
		"`docs/runbooks/operator-troubleshooting.md`",
		"route/evidence/state/verification matrix",
	} {
		if !strings.Contains(recentLogsDoc, fragment) {
			t.Fatalf("expected recent logs docs to cross-link operator troubleshooting quick-reference with %q", fragment)
		}
	}
}

func TestEnterpriseSSOSmokeContracts(t *testing.T) {
	operatorContractsDoc := readRepoFile(t, "docs/architecture/tenet-v1-operator-contracts.md")
	for _, fragment := range []string{
		"`GET /v1/operator/context.authRuntime.enterpriseSso` is the provider-neutral diagnostic surface",
		"For `TENET_AUTH_PROVIDER=google_oidc` (or the legacy `google` alias), the managed-login runtime is active",
		"For `TENET_AUTH_PROVIDER=okta_oidc`, the same managed-login runtime is active",
		"`make smoke-local` and `make smoke-docker` restart the API with fake local Google OIDC, Okta OIDC, and Okta SAML settings",
		"Google and Okta OIDC report active managed-login/enterprise-SSO runtimes while Okta SAML remains configured-but-inactive, non-secret, credential-free",
		"required/missing/invalid settings",
		"sign-in/callback/logout routes",
		"does not contact Google or Okta; focused Go tests cover the Google and Okta OIDC fake-provider login/callback paths",
		"The smoke guard now also exercises `GET /auth/logout` and `POST /auth/logout` on the configured Google path",
		"does not claim provider-token/front-door-session revocation",
		"focused Go tests cover the Google and Okta OIDC fake-provider login/callback paths",
		"including the Google Workspace case where the API reports both `tenetManagedLogin.active=true` and `enterpriseSso.active=true`",
		"the embedded recovery copy deliberately prefers the enterprise SSO label",
		"reserved Okta SAML/enterprise runtime paths `internal/authsaml/**` and `internal/authsso/**`",
	} {
		if !strings.Contains(operatorContractsDoc, fragment) {
			t.Fatalf("expected operator contracts doc enterprise SSO smoke scope to include %q", fragment)
		}
	}

	enterpriseSSODoc := readRepoFile(t, "docs/architecture/tenet-v1-enterprise-sso.md")
	for _, fragment := range []string{
		"The current smoke guard restarts the direct-process and Compose API with fake local Google OIDC, Okta OIDC, and Okta SAML settings",
		"Google and Okta OIDC report active `enterpriseSso` / `tenetManagedLogin` runtimes with same-origin `loginRoute`, `callbackRoute`, and `logoutRoute`",
		"The smoke guard also calls both `GET /auth/logout` and `POST /auth/logout` against the configured Google path",
		"asserting a safe in-app redirect, Tenet `tenet_session` cookie expiry, and no provider-token/front-door-session revocation claim",
		"without contacting an external IdP",
		"Focused unit/router tests cover Google and Okta OIDC fake-provider code exchange/profile lookup",
		"the embedded guarded-read handoff preference when enterprise SSO reports both `enterpriseSso.active=true` and `tenetManagedLogin.active=true`",
		"operator copy must use `Sign in with Google Workspace SSO`",
		"reserved Okta SAML/enterprise runtime paths `internal/authsaml/**` and `internal/authsso/**`",
		"while unrelated docs-only edits stay on the cheaper fast lane",
	} {
		if !strings.Contains(enterpriseSSODoc, fragment) {
			t.Fatalf("expected enterprise SSO docs to describe smoke release guard scope with %q", fragment)
		}
	}

	compose := readRepoFile(t, "compose.yaml")
	for _, fragment := range []string{
		`TENET_AUTH_PROVIDER: ${TENET_AUTH_PROVIDER:-}`,
		`TENET_GOOGLE_OIDC_CLIENT_ID: ${TENET_GOOGLE_OIDC_CLIENT_ID:-}`,
		`TENET_GOOGLE_OIDC_CLIENT_SECRET: ${TENET_GOOGLE_OIDC_CLIENT_SECRET:-}`,
		`TENET_GOOGLE_OIDC_ISSUER_URL: ${TENET_GOOGLE_OIDC_ISSUER_URL:-}`,
		`TENET_GOOGLE_OIDC_REDIRECT_URL: ${TENET_GOOGLE_OIDC_REDIRECT_URL:-}`,
		`TENET_OKTA_SAML_METADATA_URL: ${TENET_OKTA_SAML_METADATA_URL:-}`,
		`TENET_OKTA_SAML_ACS_URL: ${TENET_OKTA_SAML_ACS_URL:-}`,
		`TENET_OKTA_OIDC_ISSUER_URL: ${TENET_OKTA_OIDC_ISSUER_URL:-}`,
		`TENET_OKTA_OIDC_CLIENT_ID: ${TENET_OKTA_OIDC_CLIENT_ID:-}`,
		`TENET_OKTA_OIDC_CLIENT_SECRET: ${TENET_OKTA_OIDC_CLIENT_SECRET:-}`,
		`TENET_OKTA_OIDC_REDIRECT_URL: ${TENET_OKTA_OIDC_REDIRECT_URL:-}`,
		`TENET_AUTH_STATE_STORE_PATH: ${TENET_AUTH_STATE_STORE_PATH:-}`,
		`TENET_AUTH_IDENTITY_STORE_PATH: ${TENET_AUTH_IDENTITY_STORE_PATH:-}`,
		`TENET_AUTH_SESSION_SECRET: ${TENET_AUTH_SESSION_SECRET:-}`,
	} {
		if !strings.Contains(compose, fragment) {
			t.Fatalf("expected compose enterprise SSO diagnostic passthrough to include %q", fragment)
		}
	}

	smokeScripts := map[string][]string{
		"scripts/smoke-local.sh": {
			`assert_enterprise_sso_diagnostic() {`,
			`assert_enterprise_sso_logout_boundary() {`,
			`TENET_AUTH_PROVIDER=google_oidc \`,
			`TENET_AUTH_PROVIDER=okta_oidc \`,
			`TENET_AUTH_PROVIDER=okta_saml \`,
			`enterprise_sso_google_context=$(curl -fsS "http://$HOST:$PORT/v1/operator/context")`,
			`assert_enterprise_sso_diagnostic "$enterprise_sso_google_context_path" google /auth/login/google /auth/callback/google true`,
			`enterprise_sso_okta_oidc_context=$(curl -fsS "http://$HOST:$PORT/v1/operator/context")`,
			`assert_enterprise_sso_diagnostic "$enterprise_sso_okta_oidc_context_path" okta_oidc /auth/login/okta /auth/callback/okta/oidc true`,
			`assert_enterprise_sso_diagnostic "$enterprise_sso_okta_saml_context_path" okta_saml /auth/login/okta /auth/callback/okta/saml false`,
			`assert_enterprise_sso_logout_boundary GET "/?page=operator-context"`,
			`assert_enterprise_sso_logout_boundary POST "/?page=operator-context&action=logout"`,
			`assert not enterprise_sso.get('missingSettings'), payload`,
			`assert enterprise_sso.get('logoutRoute') == '/auth/logout', payload`,
			`assert managed_login.get('logoutClearsSessionCookie') is True, payload`,
			`assert managed_login.get('logoutClearsSessionCookie') is False, payload`,
			`assert managed_login.get('logoutRevokesProviderToken') is False, payload`,
			`assert managed_login.get('providerTokenRevocationSupported') is False, payload`,
			`assert enterprise_sso.get('stateStoreConfigured') is True, payload`,
			`assert enterprise_sso.get('identityStoreConfigured') is True, payload`,
			`assert enterprise_sso.get('sessionSecretConfigured') is True, payload`,
			`expected_active = sys.argv[5] == 'true'`,
			`secret-value`,
			`configured enterprise SSO diagnostics ok`,
		},
		"scripts/smoke-docker.sh": {
			`assert_enterprise_sso_diagnostic() {`,
			`assert_enterprise_sso_logout_boundary() {`,
			`restart_api_with_enterprise_sso() {`,
			`TENET_AUTH_PROVIDER=google_oidc \`,
			`TENET_AUTH_PROVIDER=okta_oidc \`,
			`TENET_AUTH_PROVIDER=okta_saml \`,
			`restart_api_with_enterprise_sso google_oidc`,
			`restart_api_with_enterprise_sso okta_oidc`,
			`restart_api_with_enterprise_sso okta_saml`,
			`enterprise_sso_google_context=$(curl -fsS "$API_BASE_URL/v1/operator/context")`,
			`assert_enterprise_sso_diagnostic "$enterprise_sso_google_context_path" google /auth/login/google /auth/callback/google true`,
			`enterprise_sso_okta_oidc_context=$(curl -fsS "$API_BASE_URL/v1/operator/context")`,
			`assert_enterprise_sso_diagnostic "$enterprise_sso_okta_oidc_context_path" okta_oidc /auth/login/okta /auth/callback/okta/oidc true`,
			`assert_enterprise_sso_diagnostic "$enterprise_sso_okta_saml_context_path" okta_saml /auth/login/okta /auth/callback/okta/saml false`,
			`assert_enterprise_sso_logout_boundary GET "/?page=operator-context"`,
			`assert_enterprise_sso_logout_boundary POST "/?page=operator-context&action=logout"`,
			`assert managed_login.get('logoutClearsSessionCookie') is True, payload`,
			`assert managed_login.get('logoutClearsSessionCookie') is False, payload`,
			`assert managed_login.get('logoutRevokesProviderToken') is False, payload`,
			`assert managed_login.get('providerTokenRevocationSupported') is False, payload`,
			`TENET_AUTH_STATE_STORE_PATH=`,
			`TENET_AUTH_IDENTITY_STORE_PATH=`,
			`configured enterprise SSO diagnostics ok`,
		},
	}
	for path, fragments := range smokeScripts {
		script := readRepoFile(t, path)
		for _, fragment := range fragments {
			if !strings.Contains(script, fragment) {
				t.Fatalf("expected %s enterprise SSO smoke contract to include %q", path, fragment)
			}
		}
	}
}

func TestDockerUpOperatorContract(t *testing.T) {
	compose := readRepoFile(t, "compose.yaml")
	if !strings.Contains(compose, `TENET_AWS_ENABLE_APPLY: ${TENET_AWS_ENABLE_APPLY:-false}`) {
		t.Fatal("expected compose api service to expose TENET_AWS_ENABLE_APPLY with a default false local contract")
	}
	for _, fragment := range []string{
		`TENET_PLAN_STORE_PATH: /tmp/tenet/plan-previews.json`,
		`TENET_APPLY_STORE_PATH: /tmp/tenet/apply-requests.json`,
		`- tenet-api-tmp:/tmp`,
		`tenet-api-tmp:`,
	} {
		if !strings.Contains(compose, fragment) {
			t.Fatalf("expected compose local api runtime to include %q", fragment)
		}
	}

	makefile := readRepoFile(t, "Makefile")
	if !strings.Contains(makefile, "./scripts/docker-up.sh $(DOCKER_COMPOSE)") {
		t.Fatal("expected Makefile docker-up target to route through scripts/docker-up.sh")
	}

	script := readRepoFile(t, "scripts/docker-up.sh")
	for _, fragment := range []string{
		`port api "$PORT"`,
		`lsof -nP -iTCP:"$PORT" -sTCP:LISTEN`,
		"localhost:8080 is already occupied by something other than this Tenet compose stack",
		"lsof -nP -iTCP:8080 -sTCP:LISTEN",
		"make docker-down",
		"docker compose up --build -d",
		`exec "${compose_cmd[@]}" up --build -d`,
	} {
		if !strings.Contains(script, fragment) {
			t.Fatalf("expected docker-up preflight script to include %q", fragment)
		}
	}

	operatorContractsDoc := readRepoFile(t, "docs/architecture/tenet-v1-operator-contracts.md")
	for _, fragment := range []string{
		"If `make docker-up` exits before starting the stack, `localhost:8080` is already owned by something outside this Compose project.",
		"`lsof -nP -iTCP:8080 -sTCP:LISTEN`",
		"`make docker-down`",
		"`docker compose up --build -d`",
		"`TENET_AWS_ENABLE_APPLY=true make docker-up`",
	} {
		if !strings.Contains(operatorContractsDoc, fragment) {
			t.Fatalf("expected operator contracts doc docker troubleshooting to include %q", fragment)
		}
	}

	localRuntimeDoc := readRepoFile(t, "docs/architecture/tenet-v1-local-runtime.md")
	for _, fragment := range []string{
		"The `make docker-up` preflight intentionally fails fast when `localhost:8080` is already occupied by something other than this Compose stack.",
		"`lsof -nP -iTCP:8080 -sTCP:LISTEN`",
		"`make docker-down`",
		"`docker compose up --build -d`",
		"`TENET_AWS_ENABLE_APPLY=true make docker-up`",
	} {
		if !strings.Contains(localRuntimeDoc, fragment) {
			t.Fatalf("expected local runtime doc troubleshooting to include %q", fragment)
		}
	}
}

func TestReadOnlyApplyOperatorContract(t *testing.T) {
	dockerSmoke := readRepoFile(t, "scripts/smoke-docker.sh")
	for _, fragment := range []string{
		`json_string_field_from_path() {`,
		`response_path=$(write_payload wait-for-apply-status.json "$response")`,
		`current_status=$(json_string_field_from_path "$response_path" status)`,
		`restart_api_with_apply_mode() {`,
		`TENET_AWS_ENABLE_APPLY="$apply_enabled" "${COMPOSE[@]}" up -d api >/dev/null`,
		`restarted_plan_preview=$(curl -fsS \`,
		`"$API_BASE_URL/v1/plans/$plan_id")`,
		`restarted_apply_record=$(curl -fsS \`,
		`"$API_BASE_URL/v1/applies/$apply_id")`,
		`restarted_plan_preview_path=$(write_payload restarted-plan-preview.json "$restarted_plan_preview")`,
		`restarted_apply_record_path=$(write_payload restarted-apply-record.json "$restarted_apply_record")`,
		`RESTARTED_PLAN_PREVIEW_PATH="$restarted_plan_preview_path" \`,
		`RESTARTED_APPLY_RECORD_PATH="$restarted_apply_record_path" \`,
		`assert restarted_plan_preview["id"] == os.environ["PLAN_ID"], restarted_plan_preview`,
		`assert restarted_apply_record["id"] == os.environ["APPLY_ID"], restarted_apply_record`,
		`assert restarted_apply_record["planId"] == os.environ["PLAN_ID"], restarted_apply_record`,
		`drift_ui=$(curl -fsS "$API_BASE_URL/?page=drift&deployment=payments-api-prod")`,
		`"$API_BASE_URL/v1/applies/$apply_id/transitions"`,
		`"$API_BASE_URL/v1/applies/apply_smoke_missing/transitions"`,
		`deployment_detail_extract_path=$(write_payload deployment-detail.extract.json "$deployment_detail")`,
		`deployment_source_fingerprint=$(json_string_field_from_path "$deployment_detail_extract_path" sourceFingerprint)`,
		`plan_preview_extract_path=$(write_payload plan-preview.extract.json "$plan_preview")`,
		`plan_id=$(json_string_field_from_path "$plan_preview_extract_path" id)`,
		`plan_status=$(PLAN_PREVIEW_PATH="$plan_preview_extract_path" PLAN_PREVIEW_GET_PATH="$plan_preview_get_extract_path" DEPLOYMENT_SOURCE_FINGERPRINT="$deployment_source_fingerprint" python3 - <<'PY'`,
		`apply_record_extract_path=$(write_payload apply-record.extract.json "$apply_record")`,
		`apply_id=$(json_string_field_from_path "$apply_record_extract_path" id)`,
		`managed_drift = deployment_detail.get("managedDrift")`,
		`runtime_managed_drift = (deployment_detail.get("runtimeSummary") or {}).get("managedDrift")`,
		`assert isinstance(managed_drift, list) and managed_drift, deployment_detail`,
		`assert isinstance(runtime_managed_drift, list) and runtime_managed_drift, deployment_detail`,
		`assert runtime_managed_drift == managed_drift, deployment_detail`,
		`assert all(isinstance(row.get("field"), str) and row["field"] for row in managed_drift), managed_drift`,
		`assert all(isinstance(row.get("desired"), str) and row["desired"] for row in managed_drift), managed_drift`,
		`assert all(isinstance(row.get("actual"), str) and row["actual"] for row in managed_drift), managed_drift`,
		`assert all(row.get("severity") in {"high", "medium", "low"} for row in managed_drift), managed_drift`,
		`"Active drift source",`,
		`"Current source: controller-authored managedDrift rows.",`,
		`"managedDrift is present in the deployment detail payload, so fallback read-model heuristics stay suppressed for this deployment.",`,
		`"Drift summary",`,
		`"Field path",`,
		`"Desired",`,
		`"Actual",`,
		`"Severity",`,
		`"No controller-authored field-level drift was reported for this deployment.",`,
		`"No field-level drift rows match the current filters. Adjust the query or severity filters to inspect reported drift.",`,
		`assert invalid_apply_transition.strip() == "invalid apply transition request body", invalid_apply_transition`,
		`assert "apply request not found" in unknown_apply_transition, unknown_apply_transition`,
		`assert f"{apply_record['id']} is already terminal with status failed" in terminal_apply_transition, terminal_apply_transition`,
		`queued_deployment_detail=$(curl -fsS "$API_BASE_URL/v1/overview/deployments/worker-prod")`,
		`queued_deployment_detail_extract_path=$(write_payload queued-deployment-detail.extract.json "$queued_deployment_detail")`,
		`queued_deployment_source_fingerprint=$(json_string_field_from_path "$queued_deployment_detail_extract_path" sourceFingerprint)`,
		`queued_plan_preview_request_body=$(DEPLOYMENT_SOURCE_FINGERPRINT="$queued_deployment_source_fingerprint" python3 - <<'PY'`,
		`queued_plan_preview=$(curl -fsS -X POST -H 'Content-Type: application/json' -d "$queued_plan_preview_request_body" "$API_BASE_URL/v1/plans/deployments/worker-prod")`,
		`queued_apply_record_get=$(wait_for_apply_status "$queued_apply_id" failed)`,
		`queued_apply_record=$(curl -fsS -X POST "$API_BASE_URL/v1/applies/plans/$queued_plan_id")`,
		`assert queued_plan_preview["deploymentName"] == "worker-prod", queued_plan_preview`,
		`assert queued_apply_record["deploymentName"] == "worker-prod", queued_apply_record`,
		`assert queued_apply_record["status"] == "queued", queued_apply_record`,
		`assert not queued_apply_record.get("error"), queued_apply_record`,
		`"Tenet persisted this immutable apply request as queued because TENET_AWS_ENABLE_APPLY=true enabled the local-safe executor handoff path.",`,
		`"Tenet will advance this same applyId locally, but live AWS mutation is still not implemented for this backend slice.",`,
		`assert queued_apply_record_get["status"] == "failed", queued_apply_record_get`,
		`"Tenet executor advanced this apply request on the same applyId without calling the manual transition API.",`,
		`assert [operation["status"] for operation in queued_apply_record["operations"]] == ["queued"] * len(queued_apply_record["operations"]), queued_apply_record`,
	} {
		if !strings.Contains(dockerSmoke, fragment) {
			t.Fatalf("expected smoke-docker queued apply contract to include %q", fragment)
		}
	}
	for _, fragment := range []string{
		`DEPLOYMENT_DETAIL_JSON="$deployment_detail"`,
		`PLAN_PREVIEW_JSON="$plan_preview"`,
		`APPLY_RECORD_JSON="$apply_record"`,
		`QUEUED_PLAN_PREVIEW_JSON="$queued_plan_preview"`,
		`QUEUED_APPLY_RECORD_JSON="$queued_apply_record"`,
	} {
		if strings.Contains(dockerSmoke, fragment) {
			t.Fatalf("expected smoke-docker to avoid env-blob JSON parsing fragment %q", fragment)
		}
	}

	script := readRepoFile(t, "scripts/smoke-local.sh")
	for _, fragment := range []string{
		`json_string_field_from_path() {`,
		`response_path=$(write_payload wait-for-apply-status.json "$response")`,
		`current_status=$(json_string_field_from_path "$response_path" status)`,
		`TENET_APPLY_STORE_PATH="$APPLY_STORE_PATH"`,
		`TENET_AWS_ENABLE_APPLY="$APPLY_ENABLED"`,
		`curl -fsS -X POST "http://$HOST:$PORT/v1/applies/plans/$plan_id"`,
		`"http://$HOST:$PORT/v1/applies/$apply_id/transitions"`,
		`"http://$HOST:$PORT/v1/applies/apply_smoke_missing/transitions"`,
		`assert invalid_apply_transition.strip() == 'invalid apply transition request body', invalid_apply_transition`,
		`assert 'apply request not found' in unknown_apply_transition, unknown_apply_transition`,
		`assert f"{apply_record['id']} is already terminal with status failed" in terminal_apply_transition, terminal_apply_transition`,
		`curl -sS -o "$STALE_APPLY_CONFLICT_BODY" -w "%{http_code}" -X POST "http://$HOST:$PORT/v1/applies/plans/$stale_apply_plan_id"`,
		`apply_record_get=$(wait_for_apply_status "$apply_id" failed)`,
		`curl -fsS "http://$HOST:$PORT/v1/applies/$apply_id"`,
		`apply_enabled = os.environ['APPLY_ENABLED'].lower() == 'true'`,
		`assert apply_record['status'] == ('queued' if apply_enabled else 'failed'), apply_record`,
		`queued_apply_records=$(curl -fsS "http://$HOST:$PORT/v1/applies?status=queued")`,
		`queued_apply_records_path=$(write_payload queued-apply-records.json "$queued_apply_records")`,
		`QUEUED_APPLY_RECORDS_PATH="$queued_apply_records_path"`,
		`queued_apply_records = read_json('QUEUED_APPLY_RECORDS_PATH')`,
		`assert all(record.get('status') == 'queued' for record in queued_apply_records), queued_apply_records`,
		`assert all(record.get('deploymentName') for record in queued_apply_records), queued_apply_records`,
		`assert queued_apply_records == [], queued_apply_records`,
		`assert apply_record['error'] == 'apply execution is disabled', apply_record`,
		`assert isinstance(apply_record.get('lifecycle', {}), dict) and apply_record['lifecycle'], apply_record`,
		`assert apply_record['lifecycle']['updatedAt'] == apply_record['createdAt'], apply_record`,
		`assert apply_record['lifecycle']['totalSteps'] == len(apply_record['operations']), apply_record`,
		`assert apply_record['lifecycle']['finishedAt'] == apply_record['createdAt'], apply_record`,
		`'Tenet persisted this immutable apply request as queued because TENET_AWS_ENABLE_APPLY=true enabled the local-safe executor handoff path.'`,
		`'Tenet will advance this same applyId locally, but live AWS mutation is still not implemented for this backend slice.'`,
		`assert apply_record_get['status'] == 'failed', apply_record_get`,
		`assert apply_record_get['error'] == 'apply executor handoff stopped before live AWS mutation because this backend slice is still local-safe', apply_record_get`,
		`assert isinstance(apply_record.get('operations', []), list) and apply_record['operations'], apply_record`,
		`plan_operation_ids = [operation.get('id') for operation in plan_preview['operations']]`,
		`assert all(isinstance(operation_id, str) and operation_id.startswith('op_') for operation_id in plan_operation_ids), plan_preview`,
		`assert len(plan_operation_ids) == len(set(plan_operation_ids)), plan_preview`,
		`assert [operation.get('id') for operation in apply_record['operations']] == plan_operation_ids, (apply_record, plan_preview)`,
		`assert [operation.get('id') for operation in actor_apply_record['operations']] == actor_plan_operation_ids, (actor_apply_record, actor_plan_preview)`,
		`assert [operation.get('id') for operation in actor_apply_record_get['operations']] == [operation.get('id') for operation in actor_apply_record['operations']], (actor_apply_record_get, actor_apply_record)`,
		`assert [operation['status'] for operation in apply_record['operations']] == [('queued' if apply_enabled else 'pending')] * len(apply_record['operations']), apply_record`,
		`assert os.environ['STALE_APPLY_CONFLICT_STATUS'] == '409', os.environ['STALE_APPLY_CONFLICT_STATUS']`,
		`assert 'create a new plan preview before applying' in stale_apply_conflict, stale_apply_conflict`,
		`curl -fsS "http://$HOST:$PORT/v1/audit-logs?deployment=payments-api-stale&action=apply.requested"`,
		`deployment_detail_extract_path=$(write_payload deployment-detail.extract.json "$deployment_detail")`,
		`deployment_source_fingerprint=$(json_string_field_from_path "$deployment_detail_extract_path" sourceFingerprint)`,
		`plan_preview_extract_path=$(write_payload plan-preview.extract.json "$plan_preview")`,
		`plan_id=$(json_string_field_from_path "$plan_preview_extract_path" id)`,
		`apply_record_extract_path=$(write_payload apply-record.extract.json "$apply_record")`,
		`apply_id=$(json_string_field_from_path "$apply_record_extract_path" id)`,
		`frontdoor_plan_preview_path=$(write_payload frontdoor-plan-preview.json "$frontdoor_plan_preview")`,
		`FRONTDOOR_PLAN_PREVIEW_PATH="$frontdoor_plan_preview_path"`,
		`controller_detail_probe_path=$(write_payload controller-detail-probe.json "$controller_detail")`,
	} {
		if !strings.Contains(script, fragment) {
			t.Fatalf("expected smoke-local apply contract to include %q", fragment)
		}
	}
	for _, fragment := range []string{
		`DEPLOYMENT_DETAIL_JSON="$deployment_detail"`,
		`PLAN_PREVIEW_JSON="$plan_preview"`,
		`APPLY_RECORD_JSON="$apply_record"`,
		`FRONTDOOR_PLAN_PREVIEW_JSON="$frontdoor_plan_preview"`,
		`CONTROLLER_DETAIL_JSON="$controller_detail"`,
	} {
		if strings.Contains(script, fragment) {
			t.Fatalf("expected smoke-local to avoid env-blob JSON parsing fragment %q", fragment)
		}
	}

	operatorContractsDoc := readRepoFile(t, "docs/architecture/tenet-v1-operator-contracts.md")
	for _, fragment := range []string{
		"`POST /v1/applies/plans/{planId}`",
		"`GET /v1/applies/{applyId}`",
		"`POST /v1/applies/{applyId}/heartbeat`",
		"`POST /v1/applies/{applyId}/transitions`",
		"`applyrequest.TransitionRequest` state model internally",
		"`TENET_PLAN_STORE_PATH`",
		"`invalid apply heartbeat request body`",
		"`invalid apply transition request body`",
		"`apply request not found`",
		"`TENET_APPLY_STORE_PATH`",
		"`/tmp/tenet/plan-previews.json`",
		"`/tmp/tenet/apply-requests.json`",
		"`/tmp`",
		"survive the Compose `api` service restart",
		"`TENET_AWS_ENABLE_APPLY=true`",
		"disabled by default for live execution",
		"queued-on-create local-safe executor handoff contract",
		"live AWS mutation is still not implemented",
		"ordered `operations[]` rows",
		"Tenet-authored stable operation `id` values aligned",
		"non-empty `op_...` values",
		"preserved when plan operations project into apply operations",
		"inspect and share the future executor-facing",
		"stable per-operation `Share operation` anchors",
		"preserve the current `planId` / `applyId` workspace URL",
		"additive `lifecycle` block",
		"`updatedAt` / `startedAt` / `finishedAt`",
		"`400 Bad Request`",
		"`404 Not Found`",
		"`409 Conflict`",
		"terminal `applyId`",
		"clear operator-facing text",
		"create a fresh preview",
		"temp files via the shared `write_payload` helper",
		"environment variables",
		"Linux CI runners",
	} {
		if !strings.Contains(operatorContractsDoc, fragment) {
			t.Fatalf("expected operator contracts doc apply contract to include %q", fragment)
		}
	}

	localRuntimeDoc := readRepoFile(t, "docs/architecture/tenet-v1-local-runtime.md")
	for _, fragment := range []string{
		"`POST /v1/applies/{applyId}/heartbeat`",
		"`POST /v1/applies/{applyId}/transitions`",
		"`400` malformed JSON",
		"`404` unknown apply",
		"`409` invalid terminal move/heartbeat",
		"operator-facing text",
		"smoke-created terminal `applyId`",
		"unique non-empty `op_...` IDs",
		"must keep the reviewed plan operation IDs instead of falling back to array position",
		"embedded workspace keeps those stable IDs wired to row-level `Share operation` anchors",
		"bookmarkable `deployment`, `planId`, `applyId`, and hash state",
		"`TENET_PLAN_STORE_PATH=/tmp/tenet/plan-previews.json`",
		"`TENET_APPLY_STORE_PATH=/tmp/tenet/apply-requests.json`",
		"survive the Compose `api` service restart",
		"temp files through the shared `write_payload` helper",
		"Linux CI runners with tighter environment-size limits",
	} {
		if !strings.Contains(localRuntimeDoc, fragment) {
			t.Fatalf("expected local runtime doc apply contract to include %q", fragment)
		}
	}

	uiDoc := readRepoFile(t, "docs/architecture/tenet-v1-overview-ui.md")
	for _, fragment := range []string{
		"`POST /v1/applies/plans/{planId}`",
		"`GET /v1/applies/{applyId}`",
		"`POST /v1/applies/{applyId}/claim`",
		"accepts a JSON request body with `workerId`",
		"preserves that same stable `applyId`",
		"moves a queued apply record to `running`",
		"records `claim.workerId`, `claim.claimedAt`, `claim.lastHeartbeatAt`, and `claim.expiresAt`",
		"active-vs-stale `409 Conflict` guidance",
		"performs no live AWS mutation",
		"`POST /v1/applies/{applyId}/heartbeat`",
		"only the currently claimed worker may refresh the lease",
		"updates `claim.lastHeartbeatAt` and `claim.expiresAt`",
		"wrong-worker, unclaimed, or terminal-state heartbeats",
		"`POST /v1/applies/{applyId}/transitions`",
		"preserving the immutable stable `applyId`",
		"failed immutable contract with operator-facing error/details",
		"`TENET_AWS_ENABLE_APPLY=true`",
		"queued immutable apply request without contacting AWS",
		"`status=pending`",
		"`status=queued`",
		"live AWS mutation is not implemented yet",
		"does not require manual transition API calls",
		"additive `lifecycle` block",
		"`currentStep` / `completedSteps` / `totalSteps`",
		"`400 Bad Request`",
		"`409 Conflict`",
		"does not persist a new apply record",
		"stable DOM anchor plus a `_self` `Share operation` link",
		"preserves `deployment`, `planId`, `applyId`, and the selected `#operation-...` hash",
	} {
		if !strings.Contains(uiDoc, fragment) {
			t.Fatalf("expected overview UI doc apply contract to include %q", fragment)
		}
	}
}

func TestManagedLiveDriftDocsPreferFocusedContractCoverageOverLocalSmoke(t *testing.T) {
	operatorContractsDoc := readRepoFile(t, "docs/architecture/tenet-v1-operator-contracts.md")
	for _, fragment := range []string{
		"`runtimeContext.capacity.desired`, `runtimeContext.capacity.min`, and `runtimeContext.capacity.max`",
		"`runtimeContext.launchTemplate.bootstrapSource`",
		"`runtimeContext.launchTemplate.imageId`",
		"`runtimeContext.launchTemplate.instanceType`",
		"`runtimeContext.deploymentConfig.minimumHealthyPercent`",
		"`runtimeContext.deploymentConfig.maximumPercent`",
		"`runtimeContext.deploymentConfig.healthCheckGracePeriodSeconds`",
		"`runtimeContext.taskDefinition.image`",
		"Focused `go test ./internal/api` coverage locks the live-backed EC2 launch-template drift rows for `runtimeContext.launchTemplate.imageId` and `runtimeContext.launchTemplate.instanceType` because local smoke may legitimately lack AWS access and therefore does not claim to prove those rows.",
	} {
		if !strings.Contains(operatorContractsDoc, fragment) {
			t.Fatalf("expected operator contracts doc managed live drift docs to include %q", fragment)
		}
	}

	featureDoc := readRepoFile(t, "docs/features/overview-dashboard.md")
	for _, fragment := range []string{
		"`runtimeContext.capacity.{desired,min,max}`",
		"`runtimeContext.launchTemplate.bootstrapSource`",
		"`runtimeContext.launchTemplate.imageId`",
		"`runtimeContext.launchTemplate.instanceType`",
		"`runtimeContext.deploymentConfig.minimumHealthyPercent`",
		"`runtimeContext.deploymentConfig.maximumPercent`",
		"`runtimeContext.deploymentConfig.healthCheckGracePeriodSeconds`",
		"`runtimeContext.taskDefinition.image`",
		"`runtimeContext.trafficRouting.weightedTargetGroups`",
		"`runtimeContext.trafficRouting.codeDeployTargetGroups`",
		"summary-only traffic-routing resources, unavailable AWS access, and local fixture-only runs intentionally omit those rows rather than inventing drift from static rollout summaries",
		"focused `go test ./internal/api` coverage locks the live-backed EC2 launch-template drift rows for `runtimeContext.launchTemplate.imageId` and `runtimeContext.launchTemplate.instanceType` because local smoke may legitimately lack AWS access and therefore does not claim to prove those rows",
	} {
		if !strings.Contains(featureDoc, fragment) {
			t.Fatalf("expected overview dashboard managed live drift doc to include %q", fragment)
		}
	}

	localRuntimeDoc := readRepoFile(t, "docs/architecture/tenet-v1-local-runtime.md")
	for _, fragment := range []string{
		"`runtimeContext.launchTemplate.bootstrapSource`",
		"`runtimeContext.launchTemplate.imageId`",
		"`runtimeContext.launchTemplate.instanceType`",
		"`runtimeContext.deploymentConfig.minimumHealthyPercent`",
		"`runtimeContext.deploymentConfig.maximumPercent`",
		"`runtimeContext.deploymentConfig.healthCheckGracePeriodSeconds`",
		"`runtimeContext.taskDefinition.image`",
		"`runtimeContext.trafficRouting.*`",
		"Local runs may legitimately lack AWS access, so focused API/controller unit coverage locks those additive JSON/UI and live-evidence derivation details instead.",
	} {
		if !strings.Contains(localRuntimeDoc, fragment) {
			t.Fatalf("expected local runtime managed live drift doc to include %q", fragment)
		}
	}
}

func TestDriftRowShareLinkDocsContract(t *testing.T) {
	documents := map[string][]string{
		"docs/architecture/tenet-v1-operator-contracts.md": {
			"`Share drift row` link to a stable `#drift-row-...` anchor",
			"prefers backend `driftId` and falls back to a deterministic legacy row identity",
		},
		"docs/features/overview-dashboard.md": {
			"renders an `_self` `Share drift row` action for every field-level row",
			"stable row-level `#drift-row-...` anchor",
			"anchors prefer backend `driftId` / `id` / `entryId` values",
			"fall back deterministically from rendered row content plus original row order",
		},
		"docs/architecture/tenet-v1-overview-ui.md": {
			"Field-level rows also expose `_self` `Share drift row` links",
			"stable row-level `#drift-row-...` anchors",
			"preferring backend row identifiers when present and falling back deterministically for legacy derived rows",
		},
	}

	for path, fragments := range documents {
		body := readRepoFile(t, path)
		for _, fragment := range fragments {
			if !strings.Contains(body, fragment) {
				t.Fatalf("expected %s drift-row share-link docs to include %q", path, fragment)
			}
		}
	}
}

func TestDesiredStateSourceOperatorDocsContract(t *testing.T) {
	readme := readRepoFile(t, "README.md")
	for _, fragment := range []string{
		"`docs/architecture/tenet-v1-aws-central-control-plane.md` documents the AWS",
		"Terraform bootstrap, public repo-sync, and `repo_sync_token_secret_arn`.",
	} {
		if !strings.Contains(readme, fragment) {
			t.Fatalf("expected README documentation map to include %q", fragment)
		}
	}

	operatorContractsDoc := readRepoFile(t, "docs/architecture/tenet-v1-operator-contracts.md")
	for _, fragment := range []string{
		"Both `tenet-api` and `tenet-controller` can read that desired state from either:",
		"`TENET_REPO_ROOT` remains the filesystem-mode input when you want a local checkout; it is not a universal runtime requirement.",
		"The AWS central-control-plane Terraform bootstrap can optionally inject a Secrets Manager-backed `TENET_REPO_SYNC_TOKEN` into its two repo-sync sidecars with `repo_sync_token_secret_arn`.",
		"Leaving that ARN unset preserves anonymous public clone behavior.",
		"If you plan to use the AWS central-control-plane Terraform bootstrap, do the preflight before `terraform apply`: it builds and pushes the current local Tenet checkout from the machine running apply, requires Terraform 1.5+, Docker Buildx, and AWS CLI v2 with working credentials, and expects you to verify that `repo_url` and `repo_branch` are reachable from that machine first, for example with `git ls-remote --heads <repo_url> <repo_branch>`.",
	} {
		if !strings.Contains(operatorContractsDoc, fragment) {
			t.Fatalf("expected operator contracts doc desired-state source contract to include %q", fragment)
		}
	}

	architectureDoc := readRepoFile(t, "docs/architecture/tenet-v1-aws-central-control-plane.md")
	for _, fragment := range []string{
		"Tenet selects desired-state reads with `TENET_CATALOG_SOURCE`:",
		"`filesystem` uses `TENET_REPO_ROOT` to point `tenet-api` and `tenet-controller` at the same checked-out repo or synced repo tree",
		"`github` uses `TENET_CATALOG_GITHUB_*` so both services read the same desired-state manifests directly from GitHub in read-only mode",
		"The shipped Terraform/bootstrap path in this doc still uses repo-sync plus shared filesystem delivery today because that works for both services now.",
		"Public HTTPS `repo_url` values work anonymously.",
		"Set `repo_sync_token_secret_arn` to an AWS Secrets Manager secret ARN when the filesystem-mode desired-state repo is private.",
		"the shipped Terraform bootstrap below uses the shared repo tree path at `/workspace/repo`",
		"### 3. Choose desired-state delivery",
		"The shipped Terraform/bootstrap path still uses the shared checkout plus repo-sync pattern today.",
		"Leave `repo_sync_token_secret_arn` unset for public repos; the repo-sync sidecars keep using anonymous HTTPS clone/fetch.",
		"If you use the shipped Terraform bootstrap, run this operator preflight on the same machine before `terraform apply`:",
		"`terraform apply` builds and pushes the current local Tenet checkout from that machine, so run it from the repo checkout you intend to ship",
		"Terraform 1.5+ is installed",
		"Docker Buildx is installed and working locally",
		"AWS CLI v2 is installed and your current shell has working AWS credentials for this deployment",
		"verify that `repo_url` and `repo_branch` are reachable from the current machine before apply, for example `git ls-remote --heads <repo_url> <repo_branch>`",
		"public HTTPS `repo_url` values work anonymously when `repo_sync_token_secret_arn` is unset",
		"for a private HTTPS desired-state repo, store the repo token in AWS Secrets Manager and set `repo_sync_token_secret_arn` to that secret ARN",
		"#### Filesystem-mode repo delivery options",
		"TENET_CATALOG_SOURCE=filesystem",
	} {
		if !strings.Contains(architectureDoc, fragment) {
			t.Fatalf("expected AWS central control-plane doc desired-state source contract to include %q", fragment)
		}
	}

	if strings.Contains(architectureDoc, "The current runtime contract is still filesystem-root based via `TENET_REPO_ROOT`") {
		t.Fatal("expected AWS central control-plane doc to drop stale filesystem-only runtime wording")
	}

	terraformDoc := readRepoFile(t, "deploy/terraform/aws-central-control-plane/README.md")
	for _, fragment := range []string{
		"Both `tenet-api` and `tenet-controller` now support `TENET_CATALOG_SOURCE=filesystem|github` for desired-state reads.",
		"This Terraform bootstrap intentionally chooses the filesystem path and sets `TENET_REPO_ROOT` because the shared repo-sync volume works for both services today.",
		"Public HTTPS `repo_url` values work anonymously, and private HTTPS desired-state repos can use the optional Secrets Manager token ARN described below.",
		"`repo_sync_token_secret_arn` is optional. Leave it unset for anonymously readable public HTTPS repos.",
		"When set, ECS injects the secret value as `TENET_REPO_SYNC_TOKEN` only into the two `repo-sync` sidecars, and the ECS execution role receives `secretsmanager:GetSecretValue` for that secret ARN.",
		"`repo_url` (public HTTPS works anonymously; private HTTPS can use `repo_sync_token_secret_arn`)",
		"`repo_sync_token_secret_arn` (optional Secrets Manager ARN containing an HTTPS repo token for the repo-sync sidecars)",
		"## Preflight before `terraform apply`",
		"Run this checklist on the same machine that will run `terraform apply`:",
		"`terraform apply` builds and pushes the current local Tenet checkout from that machine, so run it from the repo checkout you intend to ship",
		"Terraform 1.5+ is installed",
		"Docker Buildx is installed and working locally",
		"AWS CLI v2 is installed and your current shell has working AWS credentials for this deployment",
		"verify that `repo_url` and `repo_branch` are reachable from the current machine before apply, for example `git ls-remote --heads <repo_url> <repo_branch>`:",
		"public HTTPS `repo_url` values work anonymously when `repo_sync_token_secret_arn` is unset",
		"for a private HTTPS desired-state repo, store the repo token in AWS Secrets Manager and set `repo_sync_token_secret_arn` to that secret ARN",
	} {
		if !strings.Contains(terraformDoc, fragment) {
			t.Fatalf("expected Terraform README desired-state source contract to include %q", fragment)
		}
	}

	if strings.Contains(terraformDoc, "The current Tenet runtime still expects `TENET_REPO_ROOT` to point at a filesystem tree.") {
		t.Fatal("expected Terraform README to drop stale filesystem-only runtime wording")
	}

	terraformVars := readRepoFile(t, "deploy/terraform/aws-central-control-plane/variables.tf")
	for _, fragment := range []string{
		`description = "Git clone URL that the repo-sync sidecars keep mounted into both Tenet tasks. Public HTTPS clone URLs work anonymously; private HTTPS repos can use repo_sync_token_secret_arn."`,
		`variable "repo_sync_token_secret_arn"`,
		`description = "Optional AWS Secrets Manager secret ARN containing an HTTPS token for the repo-sync sidecars. Leave null to keep anonymous public clone behavior."`,
	} {
		if !strings.Contains(terraformVars, fragment) {
			t.Fatalf("expected Terraform variables desired-state source contract to include %q", fragment)
		}
	}

	terraformTfvarsExample := readRepoFile(t, "deploy/terraform/aws-central-control-plane/terraform.tfvars.example")
	for _, fragment := range []string{
		"# Public HTTPS repo URLs work anonymously. For a private HTTPS desired-state",
		"# repo, store the repo token in Secrets Manager and set repo_sync_token_secret_arn.",
		`repo_url             = "https://github.com/DevopsArtFactory/tenet.git"`,
		`# repo_sync_token_secret_arn = "arn:aws:secretsmanager:ap-northeast-2:123456789012:secret:tenet/repo-sync-token-AbCdEf"`,
	} {
		if !strings.Contains(terraformTfvarsExample, fragment) {
			t.Fatalf("expected Terraform tfvars example desired-state source contract to include %q", fragment)
		}
	}

	terraformModule := readRepoFile(t, "deploy/terraform/aws-central-control-plane/main.tf")
	for _, fragment := range []string{
		`{ name = "TENET_CATALOG_SOURCE", value = "filesystem" },`,
		`{ name = "TENET_REPO_ROOT", value = "/workspace/repo" },`,
		`repo_sync_token_secrets = var.repo_sync_token_secret_arn == null ? [] : [`,
		`name      = "TENET_REPO_SYNC_TOKEN"`,
		`Action   = ["secretsmanager:GetSecretValue"]`,
	} {
		if !strings.Contains(terraformModule, fragment) {
			t.Fatalf("expected Terraform bootstrap task definition to include %q", fragment)
		}
	}
}

func TestAuditLogOperatorContract(t *testing.T) {
	script := readRepoFile(t, "scripts/smoke-local.sh")
	for _, fragment := range []string{
		`curl -fsS "http://$HOST:$PORT/v1/audit-logs"`,
		`curl -fsS "http://$HOST:$PORT/v1/audit-logs?action=apply.requested"`,
		`curl -fsS "http://$HOST:$PORT/v1/audit-logs?deployment=payments-api-prod&limit=1"`,
		`curl -fsS "http://$HOST:$PORT/v1/audit-logs?deployment=worker-prod"`,
		`curl -sS -o "$INVALID_AUDIT_ACTION_BODY" -w "%{http_code}" "http://$HOST:$PORT/v1/audit-logs?action=invalid.action"`,
		`assert any(entry['action'] == 'apply.requested' for entry in audit_logs['entries']), audit_logs`,
		`assert len(audit_logs_action_filtered['entries']) >= 2, audit_logs_action_filtered`,
		`assert all(entry['action'] == 'apply.requested' for entry in audit_logs_action_filtered['entries']), audit_logs_action_filtered`,
		`assert len(audit_logs_filtered['entries']) == 1, audit_logs_filtered`,
		`assert audit_logs_empty['entries'] == [], audit_logs_empty`,
		`assert os.environ['INVALID_AUDIT_ACTION_STATUS'] == '400', os.environ['INVALID_AUDIT_ACTION_STATUS']`,
		`assert entry['actor']['available'] is False, entry`,
	} {
		if !strings.Contains(script, fragment) {
			t.Fatalf("expected smoke-local audit contract to include %q", fragment)
		}
	}

	operatorContractsDoc := readRepoFile(t, "docs/architecture/tenet-v1-operator-contracts.md")
	for _, fragment := range []string{
		"`GET /v1/audit-logs?action=apply.requested`",
		"`GET /v1/audit-logs?action=plan.created`",
		"`GET /v1/audit-logs?limit={n}`",
		"`GET /v1/audit-logs?deployment={deploymentName}`",
		"`GET /v1/audit-logs?deployment={deploymentName}&action={action}`",
		"`action` is optional and must be exactly `plan.created`, `apply.requested`, `apply.transitioned`, or `controller.refresh.requested` when present",
		"`GET /v1/audit-logs?deployment={deploymentName}&limit={n}`",
		"`deployment` is optional and must name exactly one deployment when present",
		"backend-authored stable `entryId` on each row for shareable UI anchors",
		"`X-Tenet-Actor-Id`",
		"`X-Tenet-Actor-Name`",
		"`X-Tenet-Actor-Email`",
		"`X-User-ID`",
		"`X-User-Name`",
		"`X-User-Email`",
		"Add `&action=plan.created|apply.requested|apply.transitioned|controller.refresh.requested` to narrow the embedded timeline to one action",
		"actor information is unavailable because no authenticated request context was available for this local request",
		"`Audit filters: action Apply requested (apply.requested) · Limit cap: latest 2`",
		"`/v1/audit-logs` is the preferred v1 route shape for this local timeline",
		"full `GET /v1/audit-logs` collection with expected `plan.created`, `apply.requested`, lifecycle `apply.transitioned`, and manual `controller.refresh.requested` action support present",
		"fetching the full `GET /v1/audit-logs` collection",
	} {
		if !strings.Contains(operatorContractsDoc, fragment) {
			t.Fatalf("expected operator contracts doc audit contract to include %q", fragment)
		}
	}

	auditDoc := readRepoFile(t, "docs/architecture/tenet-v1-audit-log-api.md")
	for _, fragment := range []string{
		"`GET /v1/audit-logs`",
		"`GET /v1/audit-logs?deployment={deploymentName}`",
		"`GET /v1/audit-logs?action={action}`",
		"`limit` is optional and must be a positive integer when present",
		"`action` is optional and must be exactly `plan.created`, `apply.requested`, `apply.transitioned`, or `controller.refresh.requested` when present",
		"`deployment` is optional and must name exactly one deployment when present",
		"`GET /v1/audit-logs?action=apply.requested&limit=2`",
		"`entryId` is a backend-authored opaque stable row identifier",
		"`X-Tenet-Actor-Id`",
		"fetching the full `GET /v1/audit-logs` collection",
		"asserting the expected `plan.created`, `apply.requested`, lifecycle `apply.transitioned`, and manual `controller.refresh.requested` action support is present without depending on a two-entry window",
		"`action=invalid.action` with `400 Bad Request`",
		"`entries: []` with `200 OK` when the selected deployment exists but has no persisted audit records yet",
		"`X-Tenet-Actor-Id`",
		"`X-Tenet-Actor-Name`",
		"`X-Tenet-Actor-Email`",
		"`X-User-ID`",
		"`X-User-Name`",
		"`X-User-Email`",
		"Tenet prefers the explicit `X-Tenet-Actor-*` values over `X-User-*` aliases",
		"`actor.available=false` plus an unavailable reason",
	} {
		if !strings.Contains(auditDoc, fragment) {
			t.Fatalf("expected audit architecture doc to include %q", fragment)
		}
	}

	overviewUIDoc := readRepoFile(t, "docs/architecture/tenet-v1-overview-ui.md")
	for _, fragment := range []string{
		"`/?page=audit` route now renders that audit timeline directly",
		"`Audit filters: action Apply requested (apply.requested) · Limit cap: latest 2`",
		"`apply.transitioned` rows with previous/new status and step metadata",
		"accepts optional `&action={action}` / `&limit={n}`",
		"`Clear action/limit`",
		"`Clear audit controls`",
	} {
		if !strings.Contains(overviewUIDoc, fragment) {
			t.Fatalf("expected overview UI doc to include %q", fragment)
		}
	}
}

func TestDockerSmokeAuthFilteredAuditActionEmptyResultScope(t *testing.T) {
	dockerSmoke := readRepoFile(t, "scripts/smoke-docker.sh")
	for _, fragment := range []string{
		`assert empty_audit_collection["entries"] == [], empty_audit_collection`,
		`empty_audit_scope = empty_audit_collection.get("resultScope") or {}`,
		`assert empty_audit_scope.get("action") == "apply.requested", empty_audit_collection`,
		`assert empty_audit_scope.get("returnedEntryCount") == 0, empty_audit_collection`,
		`assert empty_audit_scope.get("returnedCount") == 0, empty_audit_collection`,
		`assert empty_audit_scope.get("totalEntryCount") == 0, empty_audit_collection`,
		`assert empty_audit_scope.get("effectiveLimit") == 0, empty_audit_collection`,
		`assert empty_audit_scope.get("truncationStatus") == "not_truncated", empty_audit_collection`,
	} {
		if !strings.Contains(dockerSmoke, fragment) {
			t.Fatalf("expected smoke-docker empty authorized audit action-filter contract to include %q", fragment)
		}
	}
	for _, forbidden := range []string{
		`assert empty_audit_collection == {"entries": []}, empty_audit_collection`,
	} {
		if strings.Contains(dockerSmoke, forbidden) {
			t.Fatalf("expected smoke-docker to preserve resultScope on empty authorized audit filters instead of %q", forbidden)
		}
	}
}

func TestFrontDoorAuthOperatorContract(t *testing.T) {
	compose := readRepoFile(t, "compose.yaml")
	for _, fragment := range []string{
		`TENET_API_TRUST_AUTHENTICATED_HEADERS: ${TENET_API_TRUST_AUTHENTICATED_HEADERS:-false}`,
		`TENET_API_ENFORCE_AUTHENTICATED_IDENTITY: ${TENET_API_ENFORCE_AUTHENTICATED_IDENTITY:-false}`,
		`TENET_API_ENFORCE_OPERATOR_ROLE_AUTHORIZATION: ${TENET_API_ENFORCE_OPERATOR_ROLE_AUTHORIZATION:-false}`,
		`TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION: ${TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION:-false}`,
	} {
		if !strings.Contains(compose, fragment) {
			t.Fatalf("expected compose auth contract to include %q", fragment)
		}
	}

	script := readRepoFile(t, "scripts/smoke-local.sh")
	for _, fragment := range []string{
		`AUTH_TRUSTED_HEADERS=${TENET_API_TRUST_AUTHENTICATED_HEADERS:-false}`,
		`AUTH_ENFORCED=${TENET_API_ENFORCE_AUTHENTICATED_IDENTITY:-false}`,
		`AUTH_ROLE_ENFORCED=${TENET_API_ENFORCE_OPERATOR_ROLE_AUTHORIZATION:-false}`,
		`AUTH_READ_ENFORCED=${TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION:-false}`,
		`TENET_API_TRUST_AUTHENTICATED_HEADERS="$trust_authenticated_headers"`,
		`TENET_API_ENFORCE_AUTHENTICATED_IDENTITY="$enforce_authenticated_identity"`,
		`TENET_API_ENFORCE_OPERATOR_ROLE_AUTHORIZATION="$enforce_operator_role_authorization"`,
		`TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION="$enforce_deployment_read_authorization"`,
		`start_api true true false false`,
		`start_api true false true true`,
		`"http://$HOST:$PORT/v1/plans/deployments/payments-api-prod"`,
		`"http://$HOST:$PORT/v1/applies/plans/$plan_id"`,
		`"http://$HOST:$PORT/v1/plans/$frontdoor_plan_id"`,
		`"http://$HOST:$PORT/v1/applies/$frontdoor_apply_id"`,
		`"X-Tenet-Authenticated-Id: front-door-123"`,
		`"X-Tenet-Authenticated-Name: Front Door Operator"`,
		`"X-Tenet-Authenticated-Email: frontdoor@example.com"`,
		`"http://$HOST:$PORT/v1/overview/applications"`,
		`"http://$HOST:$PORT/v1/status/deployments"`,
		`"http://$HOST:$PORT/v1/status/deployments/payments-api-staging"`,
		`"http://$HOST:$PORT/v1/operator/context")`,
		`operator_context_trusted_path=$(write_payload operator-context-trusted.json "$operator_context_trusted")`,
		`operator_context_mixed_roles_path=$(write_payload operator-context-mixed-roles.json "$operator_context_mixed_roles")`,
		`operator_context_unmatched_path=$(write_payload operator-context-unmatched.json "$operator_context_unmatched")`,
		`operator_context_alias_path=$(write_payload operator-context-alias.json "$operator_context_alias")`,
		`operator_context_no_allowlist_path=$(write_payload operator-context-no-allowlist.json "$operator_context_no_allowlist")`,
		`operator_context_no_roles_path=$(write_payload operator-context-no-roles.json "$operator_context_no_roles")`,
		`"X-Tenet-Authorized-Deployments: worker-prod"`,
		`"X-Tenet-Authorized-Deployments: payments-api-staging, worker-prod"`,
		`"X-User-ID: alias-456"`,
		`"X-User-Name: Alias Operator"`,
		`"X-User-Email: alias@example.com"`,
		`"X-Tenet-Authorized-Deployments: payments-api-prod, worker-prod"`,
		`assert os.environ['AUTH_UNAUTHENTICATED_COLLECTION_STATUS'] == '401', os.environ['AUTH_UNAUTHENTICATED_COLLECTION_STATUS']`,
		`assert os.environ['AUTH_UNAUTHENTICATED_STATUS_COLLECTION_STATUS'] == '401', os.environ['AUTH_UNAUTHENTICATED_STATUS_COLLECTION_STATUS']`,
		`assert unauthenticated_collection_body.strip() == 'unauthenticated: authenticated identity is required for this operator read route', unauthenticated_collection_body`,
		`assert unauthenticated_status_collection_body.strip() == 'unauthenticated: authenticated identity is required for this operator read route', unauthenticated_status_collection_body`,
		`assert os.environ['AUTH_UNAUTHENTICATED_PLAN_STATUS'] == '401', os.environ['AUTH_UNAUTHENTICATED_PLAN_STATUS']`,
		`assert unauthenticated_plan_body.strip() == 'unauthenticated: authenticated identity is required for this operator write route', unauthenticated_plan_body`,
		`assert os.environ['AUTH_UNAUTHENTICATED_APPLY_STATUS'] == '401', os.environ['AUTH_UNAUTHENTICATED_APPLY_STATUS']`,
		`assert os.environ['AUTH_UNAUTHENTICATED_TRANSITION_STATUS'] == '401', os.environ['AUTH_UNAUTHENTICATED_TRANSITION_STATUS']`,
		`assert unauthenticated_transition_body.strip() == 'unauthenticated: authenticated identity is required for this operator write route', unauthenticated_transition_body`,
		`assert os.environ['AUTH_UNAUTHENTICATED_READ_STATUS'] == '401', os.environ['AUTH_UNAUTHENTICATED_READ_STATUS']`,
		`assert os.environ['AUTH_UNAUTHENTICATED_STATUS_READ_STATUS'] == '401', os.environ['AUTH_UNAUTHENTICATED_STATUS_READ_STATUS']`,
		`assert os.environ['AUTH_UNAUTHENTICATED_PLAN_READ_STATUS'] == '401', os.environ['AUTH_UNAUTHENTICATED_PLAN_READ_STATUS']`,
		`assert os.environ['AUTH_UNAUTHENTICATED_APPLY_READ_STATUS'] == '401', os.environ['AUTH_UNAUTHENTICATED_APPLY_READ_STATUS']`,
		`assert os.environ['AUTH_FORBIDDEN_PLAN_WRITE_STATUS'] == '403', os.environ['AUTH_FORBIDDEN_PLAN_WRITE_STATUS']`,
		`assert os.environ['AUTH_FORBIDDEN_APPLY_WRITE_STATUS'] == '403', os.environ['AUTH_FORBIDDEN_APPLY_WRITE_STATUS']`,
		`assert os.environ['AUTH_FORBIDDEN_TRANSITION_STATUS'] == '403', os.environ['AUTH_FORBIDDEN_TRANSITION_STATUS']`,
		`assert unauthenticated_read_body.strip() == 'unauthenticated: authenticated identity is required for this operator read route', unauthenticated_read_body`,
		`assert unauthenticated_status_read_body.strip() == 'unauthenticated: authenticated identity is required for this operator read route', unauthenticated_status_read_body`,
		`assert os.environ['AUTH_FORBIDDEN_READ_STATUS'] == '403', os.environ['AUTH_FORBIDDEN_READ_STATUS']`,
		`assert os.environ['AUTH_FORBIDDEN_STATUS_READ_STATUS'] == '403', os.environ['AUTH_FORBIDDEN_STATUS_READ_STATUS']`,
		`assert os.environ['AUTH_FORBIDDEN_PLAN_READ_STATUS'] == '403', os.environ['AUTH_FORBIDDEN_PLAN_READ_STATUS']`,
		`assert os.environ['AUTH_FORBIDDEN_APPLY_READ_STATUS'] == '403', os.environ['AUTH_FORBIDDEN_APPLY_READ_STATUS']`,
		`assert forbidden_plan_write_body.strip() == 'forbidden: operator does not have access to this deployment', forbidden_plan_write_body`,
		`assert forbidden_apply_write_body.strip() == 'forbidden: operator does not have access to this deployment', forbidden_apply_write_body`,
		`assert forbidden_transition_body.strip() == 'forbidden: operator does not have access to this deployment', forbidden_transition_body`,
		`assert forbidden_read_body.strip() == 'forbidden: operator does not have access to this deployment', forbidden_read_body`,
		`assert forbidden_status_read_body.strip() == 'forbidden: operator does not have access to this deployment', forbidden_status_read_body`,
		`assert allowed_status_read['deploymentName'] == 'payments-api-staging', allowed_status_read`,
		`filtered_status_deployment_names = {row['deploymentName'] for row in filtered_status_collection}`,
		`assert filtered_status_deployment_names == {'payments-api-prod'}, filtered_status_collection`,
		`assert allowed_guarded_read['name'] == 'payments-api-staging', allowed_guarded_read`,
		`filtered_application_names = {row['name'] for row in filtered_applications_collection}`,
		`assert 'worker-prod' in filtered_application_names, filtered_applications_collection`,
		`assert filtered_application_names == {'payments-api-prod', 'worker-prod'}, filtered_applications_collection`,
		`assert empty_applications_collection == [], empty_applications_collection`,
		`assert empty_status_collection == [], empty_status_collection`,
		`assert allowed_plan_read['deploymentName'] == 'payments-api-staging', allowed_plan_read`,
		`assert allowed_apply_read['deploymentName'] == 'payments-api-staging', allowed_apply_read`,
		`assert os.environ['AUTH_FORBIDDEN_AUDIT_STATUS'] == '403', os.environ['AUTH_FORBIDDEN_AUDIT_STATUS']`,
		`assert forbidden_audit_body.strip() == 'forbidden: operator does not have access to this deployment', forbidden_audit_body`,
		`assert operator_context_trusted['identity']['id'] == 'front-door-123', operator_context_trusted`,
		`assert operator_context_trusted['roles']['names'] == ['viewer', 'operator'], operator_context_trusted`,
		`assert operator_context_trusted['roles']['provenance']['source'] == 'canonical', operator_context_trusted`,
		`assert enforcement.get('mode') == 'trusted_front_door_read_write', payload`,
		`assert enforcement.get('collectionReadsFiltered') is True, payload`,
		`assert access_verdicts.get('catalogRead', {}).get('exampleRoutes') == ['GET /v1/catalog/deployments', 'GET /v1/catalog/accounts', 'GET /v1/catalog/placements', 'GET /v1/catalog/topology', 'GET /v1/catalog/summary'], payload`,
		`assert operator_context_trusted['deploymentAccess']['names'] == ['payments-api-prod', 'worker-prod'], operator_context_trusted`,
		`canonical_identity_headers = ['X-Tenet-Authenticated-Id', 'X-Tenet-Authenticated-Name', 'X-Tenet-Authenticated-Email']`,
		`alias_identity_headers = ['X-User-ID', 'X-User-Name', 'X-User-Email']`,
		`assert identity_header_support.get('canonical') == canonical_identity_headers, payload`,
		`assert identity_header_support.get('aliases') == alias_identity_headers, payload`,
		`assert (payload.get('deploymentAccess') or {}).get('supportedHeaderNames') == access_headers, payload`,
		`assert operator_context_mixed_roles['roles']['recognizedNames'] == ['viewer'], operator_context_mixed_roles`,
		`assert operator_context_mixed_roles['roles']['unrecognizedNames'] == ['auditor'], operator_context_mixed_roles`,
		`assert operator_context_mixed_roles['accessVerdicts']['operatorWrite']['allowed'] is False, operator_context_mixed_roles`,
		`assert trusted_summary['matchedCount'] == 2, operator_context_trusted`,
		`assert trusted_summary.get('unmatchedNames', []) == [], operator_context_trusted`,
		`assert trusted_summary['visibleDeploymentExamples'], operator_context_trusted`,
		`trusted_visible = {row['deploymentName']: row for row in trusted_summary['visibleDeploymentExamples']}`,
		`assert trusted_visible['payments-api-prod']['matchSources'] == ['exactDeployment'], operator_context_trusted`,
		`operator_context_combined_access_path=$(write_payload operator-context-combined-access.json "$operator_context_combined_access")`,
		`OPERATOR_CONTEXT_COMBINED_ACCESS_PATH="$operator_context_combined_access_path" \`,
		`assert operator_context_combined_access['deploymentAccess']['names'] == ['payments-api-prod', 'worker-prod'], operator_context_combined_access`,
		`combined_scopes = operator_context_combined_access['deploymentAccess']['scopes']`,
		`assert combined_scopes['projects'] == ['payments-api'], operator_context_combined_access`,
		`assert combined_scopes['environments'] == ['prod-apne2'], operator_context_combined_access`,
		`combined_visible = {row['deploymentName']: row for row in combined_summary['visibleDeploymentExamples']}`,
		`assert combined_payment['deploymentName'] == 'payments-api-prod', operator_context_combined_access`,
		`assert combined_payment['appRef'] == 'payments-api', operator_context_combined_access`,
		`assert combined_payment['environmentRef'] == 'prod-apne2', operator_context_combined_access`,
		`assert combined_payment['region'], operator_context_combined_access`,
		`assert combined_payment['awsAccountId'], operator_context_combined_access`,
		`assert combined_payment['matchSources'] == ['exactDeployment', 'projectEnvironment'], operator_context_combined_access`,
		`assert trusted_summary['projectEnvironmentExamples'], operator_context_trusted`,
		`assert_operator_context_project_environment_examples(trusted_summary['projectEnvironmentExamples'], operator_context_trusted)`,
		`trusted_project_envs = {(row['appRef'], row['environmentRef']) for row in trusted_summary['projectEnvironmentExamples']}`,
		`assert operator_context_unmatched['deploymentAccess']['names'] == ['payments-api-prod', 'stale-api', 'worker-prod'], operator_context_unmatched`,
		`assert unmatched_summary['matchedCount'] == 2, operator_context_unmatched`,
		`assert unmatched_summary['unmatchedNames'] == ['stale-api'], operator_context_unmatched`,
		`unmatched_visible = {row['deploymentName']: row for row in unmatched_summary['visibleDeploymentExamples']}`,
		`assert 'stale-api' not in unmatched_visible, operator_context_unmatched`,
		`assert operator_context_alias['identity']['id'] == 'alias-456', operator_context_alias`,
		`assert operator_context_alias['roles']['canWrite'] is False, operator_context_alias`,
		`assert operator_context_alias['roles']['provenance']['source'] == 'canonical', operator_context_alias`,
		`assert operator_context_no_allowlist['roles']['provenance']['source'] == 'canonical', operator_context_no_allowlist`,
		`assert 'names' not in operator_context_no_allowlist['deploymentAccess'], operator_context_no_allowlist`,
		`assert operator_context_no_roles['roles'].get('names', []) == [], operator_context_no_roles`,
		`assert operator_context_no_roles['identityProvenance'] == {'idSource': 'canonical', 'nameSource': 'canonical', 'emailSource': 'absent'}, operator_context_no_roles`,
		`assert operator_context_no_roles['roles']['provenance']['source'] == 'absent', operator_context_no_roles`,
		`assert no_roles_verdicts['operatorRead']['allowed'] is False, operator_context_no_roles`,
		`assert allowed_scoped_audit['entries'][0]['deploymentName'] == 'payments-api-staging', allowed_scoped_audit`,
		`-H "X-Tenet-Authorized-Projects: payments-api" \`,
		`-H "X-Tenet-Authorized-Environments: staging-apne2" \`,
		`-H "X-Tenet-Authorized-Projects: archived-payments" \`,
		`assert 'Authenticated operator required' in ui, ui`,
		`assert 'const operatorWriteAuthRequiredMessage = (route, action, context = operatorContextState.context) => {' in ui, ui`,
		`assert '${signInRuntime.message}; sign in through ${signInRuntime.loginRoute}, then return to this same bookmarkable workspace and retry ${action}.' in ui, ui`,
		`assert 'Trusted front-door X-Tenet-Authenticated-* identity headers still take precedence when present, and Tenet will not send the write until authenticated identity is available.' in ui, ui`,
		`assert 'Tenet does not start a built-in GitHub OAuth login, enterprise SSO login, or Tenet-managed browser session from this page; retry ${action} for this same bookmarkable workspace through the trusted front door' in ui, ui`,
		`assert 'const operatorWriteForbiddenMessage = (route, action) => ` + "`Tenet rejected ${route} after recognizing the caller\\x27s identity, but this operator is not authorized to ${action.toLowerCase()} for that deployment through this route. Ask a front-door/RBAC admin to authorize this deployment through either X-Tenet-Authorized-Deployments or matching X-Tenet-Authorized-Projects plus X-Tenet-Authorized-Environments catalog placement, then retry ${action}. Partial, stale, or unavailable project/environment placement still fails closed, so use the Operator context banner to confirm the forwarded scopes match this deployment.`;" + `' in ui, ui`,
		`assert 'Operator access denied' in ui, ui`,
		`assert actor['id'] == 'front-door-123', payload`,
		`assert entry['actor']['email'] == 'frontdoor@example.com', entry`,
	} {
		if !strings.Contains(script, fragment) {
			t.Fatalf("expected smoke-local front-door auth contract to include %q", fragment)
		}
	}

	dockerSmoke := readRepoFile(t, "scripts/smoke-docker.sh")
	for _, fragment := range []string{
		`AUTH_UNAUTHENTICATED_COLLECTION_BODY=$(mktemp)`,
		`AUTH_UNAUTHENTICATED_STATUS_COLLECTION_BODY=$(mktemp)`,
		`AUTH_UNAUTHENTICATED_AUDIT_COLLECTION_BODY=$(mktemp)`,
		`AUTH_UNAUTHENTICATED_PLAN_READ_BODY=$(mktemp)`,
		`AUTH_UNAUTHENTICATED_APPLY_READ_BODY=$(mktemp)`,
		`AUTH_ROLE_REQUIRED_COLLECTION_BODY=$(mktemp)`,
		`AUTH_ROLE_REQUIRED_STATUS_READ_BODY=$(mktemp)`,
		`AUTH_ROLE_REQUIRED_AUDIT_COLLECTION_BODY=$(mktemp)`,
		`AUTH_ROLE_REQUIRED_PLAN_READ_BODY=$(mktemp)`,
		`AUTH_ROLE_REQUIRED_APPLY_READ_BODY=$(mktemp)`,
		`AUTH_UNAUTHENTICATED_TRANSITION_BODY=$(mktemp)`,
		`AUTH_UNAUTHENTICATED_EVENTS_BODY=$(mktemp)`,
		`AUTH_UNAUTHENTICATED_RESOURCE_LOGS_BODY=$(mktemp)`,
		`AUTH_UNAUTHENTICATED_RESOURCE_INSPECTION_BODY=$(mktemp)`,
		`AUTH_FORBIDDEN_STATUS_READ_BODY=$(mktemp)`,
		`restart_api_with_frontdoor_read_auth() {`,
		`TENET_API_TRUST_AUTHENTICATED_HEADERS=true \`,
		`TENET_API_ENFORCE_AUTHENTICATED_IDENTITY=true \`,
		`TENET_API_ENFORCE_OPERATOR_ROLE_AUTHORIZATION=true \`,
		`TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION=true \`,
		`auth_unauthenticated_collection_status=$(curl -sS -o "$AUTH_UNAUTHENTICATED_COLLECTION_BODY" -w "%{http_code}" \`,
		`"$API_BASE_URL/v1/overview/applications")`,
		`auth_unauthenticated_status_collection_status=$(curl -sS -o "$AUTH_UNAUTHENTICATED_STATUS_COLLECTION_BODY" -w "%{http_code}" \`,
		`"$API_BASE_URL/v1/status/deployments")`,
		`"$API_BASE_URL/v1/operator/context")`,
		`operator_context_trusted_path=$(write_payload operator-context-trusted.json "$operator_context_trusted")`,
		`operator_context_unmatched_path=$(write_payload operator-context-unmatched.json "$operator_context_unmatched")`,
		`operator_context_alias_path=$(write_payload operator-context-alias.json "$operator_context_alias")`,
		`operator_context_no_allowlist_path=$(write_payload operator-context-no-allowlist.json "$operator_context_no_allowlist")`,
		`operator_context_no_roles_path=$(write_payload operator-context-no-roles.json "$operator_context_no_roles")`,
		`auth_unauthenticated_transition_status=$(curl -sS -o "$AUTH_UNAUTHENTICATED_TRANSITION_BODY" -w "%{http_code}" \`,
		`"$API_BASE_URL/v1/applies/$apply_id/transitions")`,
		`auth_forbidden_status_read_status=$(curl -sS -o "$AUTH_FORBIDDEN_STATUS_READ_BODY" -w "%{http_code}" \`,
		`"$API_BASE_URL/v1/status/deployments/payments-api-staging")`,
		`auth_forbidden_transition_status=$(curl -sS -o "$AUTH_FORBIDDEN_TRANSITION_BODY" -w "%{http_code}" \`,
		`"$API_BASE_URL/v1/applies/$apply_id/transitions")`,
		`allowed_status_read=$(curl -fsS \`,
		`filtered_status_collection=$(curl -fsS \`,
		`empty_status_collection=$(curl -fsS \`,
		`"X-Tenet-Authenticated-Id: front-door-123"`,
		`"X-Tenet-Authenticated-Name: Front Door Operator"`,
		`"X-Tenet-Authenticated-Email: frontdoor@example.com"`,
		`"X-Tenet-Authenticated-Roles: viewer"`,
		`"X-Tenet-Authenticated-Roles: operator"`,
		`"X-Tenet-Authorized-Deployments: payments-api-prod, worker-prod"`,
		`project_environment_scope_headers = ["X-Tenet-Authorized-Projects", "X-Tenet-Authorized-Environments"]`,
		`assert access_verdicts.get("deploymentScopedRead", {}).get("requiredTrustedHeaders") == identity_headers + role_headers + allowlist_headers + project_environment_scope_headers, payload`,
		`assert access_verdicts.get("collectionRead", {}).get("requiredTrustedHeaders") == identity_headers + role_headers + allowlist_headers + project_environment_scope_headers, payload`,
		`assert access_verdicts.get("catalogRead", {}).get("exampleRoutes") == ["GET /v1/catalog/deployments", "GET /v1/catalog/accounts", "GET /v1/catalog/placements", "GET /v1/catalog/topology", "GET /v1/catalog/summary"], payload`,
		`assert trusted_verdicts["deploymentScopedRead"]["reason"] == "deployment-scoped reads can pass for deployments named in X-Tenet-Authorized-Deployments or matched by X-Tenet-Authorized-Projects plus X-Tenet-Authorized-Environments", operator_context_trusted`,
		`assert os.environ["AUTH_UNAUTHENTICATED_COLLECTION_STATUS"] == "401", os.environ["AUTH_UNAUTHENTICATED_COLLECTION_STATUS"]`,
		`assert unauthenticated_collection_body.strip() == "unauthenticated: authenticated identity is required for this operator read route", unauthenticated_collection_body`,
		`assert os.environ["AUTH_UNAUTHENTICATED_STATUS_COLLECTION_STATUS"] == "401", os.environ["AUTH_UNAUTHENTICATED_STATUS_COLLECTION_STATUS"]`,
		`assert unauthenticated_status_collection_body.strip() == "unauthenticated: authenticated identity is required for this operator read route", unauthenticated_status_collection_body`,
		`assert os.environ["AUTH_UNAUTHENTICATED_TRANSITION_STATUS"] == "401", os.environ["AUTH_UNAUTHENTICATED_TRANSITION_STATUS"]`,
		`assert unauthenticated_transition_body.strip() == "unauthenticated: authenticated identity is required for this operator write route", unauthenticated_transition_body`,
		`assert os.environ["AUTH_FORBIDDEN_STATUS_READ_STATUS"] == "403", os.environ["AUTH_FORBIDDEN_STATUS_READ_STATUS"]`,
		`assert forbidden_status_read_body.strip() == "forbidden: operator does not have access to this deployment", forbidden_status_read_body`,
		`assert os.environ["AUTH_FORBIDDEN_TRANSITION_STATUS"] == "403", os.environ["AUTH_FORBIDDEN_TRANSITION_STATUS"]`,
		`assert forbidden_transition_body.strip() == "forbidden: operator does not have access to this deployment", forbidden_transition_body`,
		`assert operator_context_trusted["identity"]["id"] == "front-door-123", operator_context_trusted`,
		`assert operator_context_trusted["roles"]["names"] == ["viewer", "operator"], operator_context_trusted`,
		`assert tenet_managed_login.get("configured") is False, payload`,
		`assert tenet_managed_login.get("requiredSettings") == managed_login_required_settings, payload`,
		`assert tenet_managed_login.get("missingSettings") == managed_login_required_settings, payload`,
		`assert "loginRoute" not in tenet_managed_login, payload`,
		`assert operator_context_trusted["roles"]["provenance"]["source"] == "canonical", operator_context_trusted`,
		`assert enforcement.get("mode") == "trusted_front_door_read_write", payload`,
		`assert enforcement.get("collectionReadsFiltered") is True, payload`,
		`assert operator_context_trusted["deploymentAccess"]["names"] == ["payments-api-prod", "worker-prod"], operator_context_trusted`,
		`canonical_identity_headers = ["X-Tenet-Authenticated-Id", "X-Tenet-Authenticated-Name", "X-Tenet-Authenticated-Email"]`,
		`alias_identity_headers = ["X-User-ID", "X-User-Name", "X-User-Email"]`,
		`assert identity_header_support.get("canonical") == canonical_identity_headers, payload`,
		`assert identity_header_support.get("aliases") == alias_identity_headers, payload`,
		`assert (payload.get("deploymentAccess") or {}).get("supportedHeaderNames") == allowlist_headers + project_environment_scope_headers, payload`,
		`assert trusted_summary["matchedCount"] == 2, operator_context_trusted`,
		`assert trusted_summary.get("unmatchedNames", []) == [], operator_context_trusted`,
		`assert trusted_summary["visibleDeploymentExamples"], operator_context_trusted`,
		`trusted_visible = {row["deploymentName"]: row for row in trusted_summary["visibleDeploymentExamples"]}`,
		`assert trusted_visible["payments-api-prod"]["matchSources"] == ["exactDeployment"], operator_context_trusted`,
		`operator_context_combined_access_path=$(write_payload operator-context-combined-access.json "$operator_context_combined_access")`,
		`OPERATOR_CONTEXT_COMBINED_ACCESS_PATH="$operator_context_combined_access_path" \`,
		`assert operator_context_combined_access["deploymentAccess"]["names"] == ["payments-api-prod", "worker-prod"], operator_context_combined_access`,
		`combined_scopes = operator_context_combined_access["deploymentAccess"]["scopes"]`,
		`assert combined_scopes["projects"] == ["payments-api"], operator_context_combined_access`,
		`assert combined_scopes["environments"] == ["prod-apne2"], operator_context_combined_access`,
		`combined_visible = {row["deploymentName"]: row for row in combined_summary["visibleDeploymentExamples"]}`,
		`assert combined_payment["deploymentName"] == "payments-api-prod", operator_context_combined_access`,
		`assert combined_payment["appRef"] == "payments-api", operator_context_combined_access`,
		`assert combined_payment["environmentRef"] == "prod-apne2", operator_context_combined_access`,
		`assert combined_payment["region"], operator_context_combined_access`,
		`assert combined_payment["awsAccountId"], operator_context_combined_access`,
		`assert combined_payment["matchSources"] == ["exactDeployment", "projectEnvironment"], operator_context_combined_access`,
		`assert trusted_summary["projectEnvironmentExamples"], operator_context_trusted`,
		`assert_operator_context_project_environment_examples(trusted_summary["projectEnvironmentExamples"], operator_context_trusted)`,
		`trusted_project_envs = {(row["appRef"], row["environmentRef"]) for row in trusted_summary["projectEnvironmentExamples"]}`,
		`assert operator_context_unmatched["deploymentAccess"]["names"] == ["payments-api-prod", "stale-api", "worker-prod"], operator_context_unmatched`,
		`assert unmatched_summary["matchedCount"] == 2, operator_context_unmatched`,
		`assert unmatched_summary["unmatchedNames"] == ["stale-api"], operator_context_unmatched`,
		`unmatched_visible = {row["deploymentName"]: row for row in unmatched_summary["visibleDeploymentExamples"]}`,
		`assert "stale-api" not in unmatched_visible, operator_context_unmatched`,
		`assert operator_context_alias["identity"]["id"] == "alias-456", operator_context_alias`,
		`assert operator_context_alias["roles"]["canWrite"] is False, operator_context_alias`,
		`assert operator_context_alias["roles"]["provenance"]["source"] == "canonical", operator_context_alias`,
		`assert operator_context_no_allowlist["roles"]["provenance"]["source"] == "canonical", operator_context_no_allowlist`,
		`assert "names" not in operator_context_no_allowlist["deploymentAccess"], operator_context_no_allowlist`,
		`assert operator_context_no_roles["roles"].get("names", []) == [], operator_context_no_roles`,
		`assert operator_context_no_roles["identityProvenance"] == {"idSource": "canonical", "nameSource": "canonical", "emailSource": "absent"}, operator_context_no_roles`,
		`assert operator_context_no_roles["roles"]["provenance"]["source"] == "absent", operator_context_no_roles`,
		`assert no_roles_verdicts["operatorRead"]["allowed"] is False, operator_context_no_roles`,
		`assert allowed_status_read["deploymentName"] == "payments-api-staging", allowed_status_read`,
		`filtered_application_names = {row["name"] for row in filtered_applications_collection}`,
		`assert "worker-prod" in filtered_application_names, filtered_applications_collection`,
		`assert filtered_application_names == {"payments-api-prod", "worker-prod"}, filtered_applications_collection`,
		`filtered_status_deployment_names = {row["deploymentName"] for row in filtered_status_collection}`,
		`assert filtered_status_deployment_names == {"payments-api-prod", "worker-prod"}, filtered_status_collection`,
		`assert empty_applications_collection == [], empty_applications_collection`,
		`assert empty_status_collection == [], empty_status_collection`,
	} {
		if !strings.Contains(dockerSmoke, fragment) {
			t.Fatalf("expected smoke-docker front-door auth contract to include %q", fragment)
		}
	}
	redactedPlaceholder := strings.Repeat("*", 3)
	for _, forbidden := range []string{
		"TENET_API_TRUST_AUTHENTICATED_HEADERS=" + redactedPlaceholder + " \\",
		"TENET_API_ENFORCE_AUTHENTICATED_IDENTITY=" + redactedPlaceholder + " \\",
		"TENET_API_ENFORCE_OPERATOR_ROLE_AUTHORIZATION=" + redactedPlaceholder + " \\",
		"TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION=" + redactedPlaceholder + " \\",
	} {
		if strings.Contains(dockerSmoke, forbidden) {
			t.Fatalf("expected smoke-docker front-door auth contract to reject %q", forbidden)
		}
	}

	operatorContractsDoc := readRepoFile(t, "docs/architecture/tenet-v1-operator-contracts.md")
	for _, fragment := range []string{
		"TENET_API_TRUST_AUTHENTICATED_HEADERS",
		"TENET_API_ENFORCE_AUTHENTICATED_IDENTITY",
		"TENET_API_ENFORCE_OPERATOR_ROLE_AUTHORIZATION",
		"TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION",
		"`X-Tenet-Authenticated-Id`",
		"`X-Tenet-Authenticated-Name`",
		"`X-Tenet-Authenticated-Email`",
		"`X-Tenet-Authenticated-Roles`",
		"`X-User-ID`, `X-User-Name`, and/or `X-User-Email` aliases",
		"`X-Tenet-Authorized-Deployments`",
		"`401 Unauthorized`",
		"`unauthenticated: authenticated identity is required for this operator write route`",
		"`unauthenticated: authenticated identity is required for this operator read route`",
		"`forbidden: authenticated operator role is required for this operator write route`",
		"`forbidden: authenticated viewer-capable role is required for this operator read route`",
		"`operator` / `admin`",
		"`GET /v1/operator/context` is a read-only diagnostic endpoint",
		"reports the normalized trusted identity, additive `identityHeaderSupport.canonical` / `identityHeaderSupport.aliases` lists",
		"roles.supportedNames",
		"roles.recognizedNames` / `roles.unrecognizedNames` diagnostics",
		"`X-Tenet-Authenticated-Roles: viewer,auditor`",
		"`roles.recognizedNames=[\"viewer\"]`, `roles.unrecognizedNames=[\"auditor\"]`, `roles.canRead=true`, and `roles.canWrite=false`",
		"it does not grant Tenet read or write capability",
		"whether the request used canonical `X-Tenet-Authenticated-*` fields, compatibility `X-User-*` aliases, or the Tenet-managed `managed_session_cookie` source",
		"Role forwarding intentionally has no `X-User-*` compatibility alias",
		"roles.provenance.source=absent` means the front door must forward `X-Tenet-Authenticated-Roles` before role-gated reads or writes can pass",
		"which forwarded values Tenet recognized versus ignored",
		"When deployment-read authorization is enforced and a catalog placement provider is available, `deploymentAccess.summary` also reports forwarded, matched/visible, and total unmatched deployment-name counts",
		"`supportedHeaderNames`",
		"`visibleDeploymentExamples`",
		"`matchSources` (`exactDeployment`, `projectEnvironment`, or both)",
		"deterministic `projectEnvironmentExamples`",
		"`emptyReason`; if catalog placement data cannot be loaded, the summary stays present with `available=false` and `unavailableReason`",
		"plus `requiredTrustedHeaders` such as `X-Tenet-Authenticated-Id`, `X-User-ID`, `X-Tenet-Authenticated-Roles`, `X-Tenet-Authorized-Deployments`, `X-Tenet-Authorized-Projects`, and `X-Tenet-Authorized-Environments`",
		"`acceptedRoleNames`: read route families list `viewer`, `operator`, and `admin`, while write route families list `operator` and `admin`",
		"trusted `X-Tenet-Authenticated-Roles: viewer` deployment detail/status/events and immutable plan/apply reads succeed through matching `X-Tenet-Authorized-Projects` plus `X-Tenet-Authorized-Environments` scopes even when `X-Tenet-Authorized-Deployments` does not name the deployment",
		"stale project/environment scopes return empty guarded collections",
		"both smoke lanes assert `GET /v1/operator/context` `identityHeaderSupport.canonical` / `identityHeaderSupport.aliases`, `actorAttribution.precedence`, `roles.provenance.source` for canonical forwarded roles and missing-role `absent` cases while preserving canonical `identityProvenance` in the missing-role case, and both smoke lanes prove a mixed `viewer, auditor` forwarded-role header yields `roles.recognizedNames=[\"viewer\"]` plus `roles.unrecognizedNames=[\"auditor\"]` while keeping the embedded shell's `Recognized capability roles:`, `Unsupported forwarded roles ignored for capability checks:`, and read-versus-write capability guidance locked, plus `deploymentAccess.summary` for all-matched, combined exact-plus-project/environment, and stale/unmatched forwarded allowlists, including `visibleDeploymentExamples` rows with deployment name, project, environment, region, AWS account, and `matchSources` evidence plus region/AWS account/deployment-count fields on `projectEnvironmentExamples`",
		"That summary is diagnostic-only: it does not mint sessions, replace OAuth/RBAC, or authorize routes by itself",
		"filter to the union of exact deployment grants and project/environment catalog matches",
		"Missing, stale, or non-matching grants return `200 OK` with `[]` or `{\"entries\":[]}` on collection routes",
		"scoped reads return the existing operator-read `403` unless exact deployment membership or project/environment placement proves access",
		"retry through the trusted front door",
		"Use these settings only when every request reaches Tenet through a front door that authenticates the caller and strips any client-supplied `X-Tenet-Authenticated-*`, `X-Tenet-Authenticated-Roles`, `X-Tenet-Authorized-Deployments`, `X-Tenet-Authorized-Projects`, `X-Tenet-Authorized-Environments`, and trusted-alias `X-User-*` headers before forwarding trusted values.",
		"strips any client-supplied `X-Tenet-Authorized-Deployments`, `X-Tenet-Authorized-Projects`, and `X-Tenet-Authorized-Environments` headers",
		"Do not expose a header-trusting `tenet-api` instance directly to untrusted clients",
		"Tenet currently has two intentionally separate identity header families",
		"`X-Tenet-Authenticated-*`, `X-Tenet-Authenticated-Roles`, `X-Tenet-Authorized-Deployments`, `X-Tenet-Authorized-Projects`, `X-Tenet-Authorized-Environments` | Authenticating front door only | Request-scoped route authorization",
		"`X-Tenet-Actor-*` plus compatible `X-User-*` aliases | Local proxy, smoke, or trusted integration creating immutable records | Persisted actor metadata",
		"Tenet does not treat raw browser-supplied `X-User-*` headers as a login session",
		"the trusted authenticated identity path projects that identity into persisted plan/apply/audit actor fields",
		"restarting `tenet-api`",
		"The same trusted front-door read contract covers `GET /v1/overview/accounts/{awsAccountId}`",
		"trusted `X-Tenet-Authenticated-Roles: viewer` deployment detail/status/events and immutable plan/apply reads succeed through matching `X-Tenet-Authorized-Projects` plus `X-Tenet-Authorized-Environments` scopes even when `X-Tenet-Authorized-Deployments` does not name the deployment",
		"Operator troubleshooting checklist for guarded reads and writes",
		"Start in the embedded `Operator context` banner, or call `GET /v1/operator/context` directly",
		"When Tenet-managed login is inactive, unauthenticated writes tell operators to retry the same bookmarkable plan/apply action through the trusted front door",
		"guarded read recovery can offer the configured Tenet-managed GitHub sign-in handoff while preserving the current `returnUrl`",
		"switches to a same-shell sign-in handoff only when `/v1/operator/context` reports an active same-origin Tenet-managed login or enterprise SSO route",
		"`authRuntime.enterpriseSso.provider=okta_oidc`, `active=true`, and `loginRoute=/auth/login/okta` produces `Sign in with Okta SSO` while preserving the current bookmarkable URL as `returnUrl`; configured-but-inactive Okta diagnostics remain non-clickable",
		"Inspect the `accessVerdicts` chips and their bounded route examples",
		"Collection-read chips show `Collection scope: filtered to visible deployments`",
		"add `Collection result: likely empty` when those grants are expected to produce an empty collection",
		"deployment-scoped read/write chips show `Scope grants: exact deployments or matching project/environment scopes`",
		"Debug the specific guarded route class by checking the banner's full `Required trusted headers:` fragment",
		"If an operator write returns `403 Forbidden`, confirm `roles.canWrite=true` (`operator` or `admin`) plus either exact deployment membership or matching project/environment catalog placement",
		"deployment-scoped plan/apply writes require both `X-Tenet-Authorized-Projects` and `X-Tenet-Authorized-Environments` to match catalog placement",
		"curl -fsS http://localhost:8080/v1/operator/context",
	} {
		if !strings.Contains(operatorContractsDoc, fragment) {
			t.Fatalf("expected operator contracts doc front-door auth contract to include %q", fragment)
		}
	}

	auditDoc := readRepoFile(t, "docs/architecture/tenet-v1-audit-log-api.md")
	for _, fragment := range []string{
		"TENET_API_TRUST_AUTHENTICATED_HEADERS",
		"TENET_API_ENFORCE_AUTHENTICATED_IDENTITY",
		"TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION",
		"`X-Tenet-Authenticated-Id`",
		"`X-Tenet-Authenticated-Name`",
		"`X-Tenet-Authenticated-Email`",
		"`X-User-ID`, `X-User-Name`, and/or `X-User-Email` aliases",
		"`X-Tenet-Authenticated-*` values still win over `X-User-*` aliases",
		"`X-Tenet-Authorized-Deployments`",
		"`401 Unauthorized` plus `unauthenticated: authenticated identity is required for this operator write route`",
		"`401 Unauthorized` plus `unauthenticated: authenticated identity is required for this operator read route`",
		"`403 Forbidden` plus `forbidden: operator does not have access to this deployment`",
		"`POST /v1/plans/deployments/{deploymentName}` and `POST /v1/applies/plans/{planId}`",
		"front-door identity headers",
		"strips any client-supplied `X-Tenet-Authenticated-*`, `X-Tenet-Authenticated-Roles`, `X-Tenet-Authorized-Deployments`, `X-Tenet-Authorized-Projects`, `X-Tenet-Authorized-Environments`, and trusted-alias `X-User-*` headers before forwarding trusted values",
		"not a safe deployment shape",
		"`X-Tenet-Actor-*` values still win over `X-User-*` aliases for local actor context.",
		"persisted actor fields for immutable plan/apply records",
	} {
		if !strings.Contains(auditDoc, fragment) {
			t.Fatalf("expected audit doc front-door auth contract to include %q", fragment)
		}
	}

	localRuntimeDoc := readRepoFile(t, "docs/architecture/tenet-v1-local-runtime.md")
	for _, fragment := range []string{
		"`TENET_API_TRUST_AUTHENTICATED_HEADERS=true`",
		"`TENET_API_ENFORCE_AUTHENTICATED_IDENTITY=true`",
		"`TENET_API_ENFORCE_OPERATOR_ROLE_AUTHORIZATION=true`",
		"`TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION=true`",
		"TENET_API_TRUST_AUTHENTICATED_HEADERS",
		"TENET_API_ENFORCE_AUTHENTICATED_IDENTITY",
		"TENET_API_ENFORCE_OPERATOR_ROLE_AUTHORIZATION",
		"TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION",
		"`X-Tenet-Authenticated-Roles`",
		"`forbidden: authenticated operator role is required for this operator write route`",
		"`operator` / `admin`",
		"`X-Tenet-Authorized-Deployments`",
		"persisted plan/apply/audit actor fields",
		"Tenet currently has two intentionally separate identity header families",
		"`X-Tenet-Authenticated-*`, `X-Tenet-Authenticated-Roles`, `X-Tenet-Authorized-Deployments`, `X-Tenet-Authorized-Projects`, `X-Tenet-Authorized-Environments` | Authenticating front door only | Request-scoped route authorization",
		"`X-Tenet-Actor-*` plus compatible `X-User-*` aliases | Local proxy, smoke, or trusted integration creating immutable records | Persisted actor metadata",
		"Tenet does not treat raw browser-supplied `X-User-*` headers as a login session",
		"deployment-scoped reads enforce trusted `X-Tenet-Authenticated-*` identity plus `X-Tenet-Authorized-Deployments` or matching project/environment scopes",
		"including the persisted `POST /v1/applies/{applyId}/transitions` write route",
		"the bookmarkable `/?page=plan` workspace",
		"strips inbound trust headers, including `X-User-*` aliases, before forwarding its own values",
		"Trusted front-door auth by restarting the Compose `api` service",
		"trusted `X-Tenet-Authorized-Deployments` still filters returned application rows, raw status snapshots, account aggregates, and unscoped audit entries",
		"`GET /v1/operator/context` reports `authRuntime.tenetManagedLogin` disabled-mode settings evidence, `identityHeaderSupport.canonical` / `identityHeaderSupport.aliases`, `actorAttribution.precedence`, canonical forwarded-role provenance, recognized/unrecognized role diagnostics, including a mixed `viewer, auditor` probe with `roles.recognizedNames=[\"viewer\"]` plus `roles.unrecognizedNames=[\"auditor\"]`, missing-role `roles.provenance.source=absent` diagnostics while preserving canonical identity provenance",
		"Role provenance intentionally has no `X-User-*` alias path, so missing roles must be corrected by forwarding `X-Tenet-Authenticated-Roles`",
		"`[]` for guarded collections",
		"trusted `X-Tenet-Authenticated-Roles: viewer` deployment detail/status/events and immutable plan/apply reads succeed through matching `X-Tenet-Authorized-Projects` plus `X-Tenet-Authorized-Environments` scopes even when `X-Tenet-Authorized-Deployments` does not name the deployment",
		"Guarded-route troubleshooting workflow",
		"Start in the embedded `Operator context` banner",
		"plus its bounded route examples, any additive `acceptedRoleNames` list (`viewer` / `operator` / `admin` for read route families and `operator` / `admin` for write route families), any collection-read `Collection scope: filtered to visible deployments` cue, any likely-empty `Collection result: likely empty` cue, any deployment-scoped `Scope grants: exact deployments or matching project/environment scopes` cue, and the embedded banner's full `Required trusted headers:` label",
		"`identity.authenticated`: `false` explains `401 Unauthorized` on guarded reads or writes",
		"`roles.provenance.source`, `roles.supportedHeaderNames`, `roles.supportedNames`, `roles.recognizedNames` / `roles.unrecognizedNames`, and `roles.canRead` / `roles.canWrite`",
		"`accessVerdicts.*.acceptedRoleNames`",
		"`acceptedRoleNames` lists the exact role labels that satisfy each guarded route family (`viewer` / `operator` / `admin` for reads and `operator` / `admin` for writes)",
		"Unsupported forwarded values appear under `unrecognizedNames`",
		"only changes embedded `401` recovery into a `_self` managed sign-in handoff when `active=true` plus a same-origin `loginRoute` is present",
		"disabled/inactive managed-login deployments tell the operator to reopen the same bookmarkable URL through the trusted front door",
		"active Tenet-managed login deployments can offer `_self` `Sign in with Tenet-managed login` with the current URL preserved as `returnUrl`",
		"`deploymentAccess.summary`: `matchedCount=0`, `emptyReason`, or non-empty `unmatchedNames` explains `200 OK` empty collections",
		"`exactDeploymentMatchedCount` and `projectEnvironmentMatchedCount` summarize how many visible deployments were proven by exact `X-Tenet-Authorized-Deployments` grants versus project/environment scope matches",
		"`visibleDeploymentExamples` names a bounded set of concrete visible deployments",
		"`projectEnvironmentExamples` shows which visible project/environment groups",
		"additive `requiredTrustedHeaders` (`X-Tenet-Authenticated-Id` or `X-User-ID`, `X-Tenet-Authenticated-Roles`, and the exact `X-Tenet-Authorized-Deployments` allowlist and/or matching `X-Tenet-Authorized-Projects` plus `X-Tenet-Authorized-Environments` scopes depending on the route class)",
		"`acceptsProjectEnvironmentScopes` markers for deployment-scoped read/write classes that can be satisfied by matched project/environment placement",
		"`accessVerdicts`: use the blocked or likely-empty verdict reason plus its bounded route examples, any additive `acceptedRoleNames` list (`viewer` / `operator` / `admin` for read route families and `operator` / `admin` for write route families), any collection-read `Collection scope: filtered to visible deployments` cue, any likely-empty `Collection result: likely empty` cue, any deployment-scoped `Scope grants: exact deployments or matching project/environment scopes` cue, and the embedded banner's full `Required trusted headers:` label",
		"Project/environment-scoped reads and deployment-scoped plan/apply writes require both forwarded scope headers and a catalog placement match",
	} {
		if !strings.Contains(localRuntimeDoc, fragment) {
			t.Fatalf("expected local runtime doc front-door auth contract to include %q", fragment)
		}
	}
	redactedPlaceholder = strings.Repeat("*", 3)
	for _, forbidden := range []string{
		"`TENET_API_TRUST_AUTHENTICATED_HEADERS=" + redactedPlaceholder + "`",
		"`TENET_API_ENFORCE_AUTHENTICATED_IDENTITY=" + redactedPlaceholder + "`",
		"`TENET_API_ENFORCE_OPERATOR_ROLE_AUTHORIZATION=" + redactedPlaceholder + "`",
		"`TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION=" + redactedPlaceholder + "`",
	} {
		if strings.Contains(localRuntimeDoc, forbidden) {
			t.Fatalf("expected local runtime doc front-door auth contract to reject %q", forbidden)
		}
	}

	deploymentStatusDoc := readRepoFile(t, "docs/architecture/tenet-v1-deployment-status-api.md")
	for _, fragment := range []string{
		"`TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION` now covers this raw status family too.",
		"`X-Tenet-Authenticated-*`",
		"`X-Tenet-Authorized-Deployments`",
		"`GET /v1/status/deployments` requires trusted `X-Tenet-Authenticated-*` identity and filters returned snapshots through the union of exact `X-Tenet-Authorized-Deployments` grants and matching `X-Tenet-Authorized-Projects` plus `X-Tenet-Authorized-Environments` catalog placement",
		"`GET /v1/status/deployments/{deploymentName}` allows exact deployment membership in that same allowlist or matching project/environment catalog placement",
		"`401 Unauthorized` plus `unauthenticated: authenticated identity is required for this operator read route`",
		"`403 Forbidden` plus `forbidden: operator does not have access to this deployment`",
		"`200 OK` with `[]`",
		"`make smoke-docker` restarts the Compose `api` service with trusted-header auth enabled and proves this exact route contract",
		"The same deployment-access model now covers immutable deployment-scoped writes",
		"`POST /v1/plans/deployments/{deploymentName}`, `POST /v1/applies/plans/{planId}`, and `POST /v1/applies/{applyId}/transitions` require trusted identity plus either an exact `X-Tenet-Authorized-Deployments` grant for the target deployment or matching `X-Tenet-Authorized-Projects` plus `X-Tenet-Authorized-Environments` catalog placement",
		"matching project/environment scopes can authorize the same read surface without an exact deployment-name grant",
		"omitting all deployment/project/environment grants returns `[]`",
		"Only enable that trust when a front door authenticates the caller and strips any client-supplied `X-Tenet-Authenticated-*`, `X-Tenet-Authorized-Deployments`, `X-Tenet-Authorized-Projects`, `X-Tenet-Authorized-Environments`, and trusted-alias `X-User-*` headers before forwarding trusted values.",
	} {
		if !strings.Contains(deploymentStatusDoc, fragment) {
			t.Fatalf("expected deployment status doc front-door auth contract to include %q", fragment)
		}
	}

	runtimeRolloutSummaryDoc := readRepoFile(t, "docs/architecture/tenet-v1-runtime-rollout-summary.md")
	for _, fragment := range []string{
		"`GET /v1/status/deployments`",
		"`GET /v1/status/deployments/{deploymentName}`",
		"`TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION=true`",
		"`TENET_API_ENFORCE_OPERATOR_ROLE_AUTHORIZATION=true`",
		"`X-Tenet-Authorized-Deployments`",
		"the raw status collection requires trusted authenticated identity and filters snapshots to the authorized deployment subset from exact trusted `X-Tenet-Authorized-Deployments` grants plus matching `X-Tenet-Authorized-Projects` / `X-Tenet-Authorized-Environments` catalog placement",
		"returning `200 OK` with `[]` when no forwarded deployment name or project/environment scope matches persisted snapshots",
		"The raw status detail route follows the same deployment-scoped `401 Unauthorized` / `403 Forbidden` contract as other guarded operator reads",
		"missing trusted identity returns `unauthenticated: authenticated identity is required for this operator read route`",
		"a recognized operator without exact deployment membership or matching project/environment placement gets `forbidden: operator does not have access to this deployment`",
		"`TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION=true` is the concrete toggle example for that guarded raw-status collection/detail contract",
	} {
		if !strings.Contains(runtimeRolloutSummaryDoc, fragment) {
			t.Fatalf("expected runtime rollout summary doc front-door auth contract to include %q", fragment)
		}
	}
	for _, forbidden := range []string{
		"`TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION=" + redactedPlaceholder + "`",
		"`TENET_API_ENFORCE_OPERATOR_ROLE_AUTHORIZATION=" + redactedPlaceholder + "`",
		"the raw status collection now requires",
		"Stated directly: the raw status detail route now follows",
	} {
		if strings.Contains(runtimeRolloutSummaryDoc, forbidden) {
			t.Fatalf("expected runtime rollout summary doc front-door auth contract to reject %q", forbidden)
		}
	}

	overviewFeatureDoc := readRepoFile(t, "docs/features/overview-dashboard.md")
	for _, fragment := range []string{
		"`TENET_API_ENFORCE_OPERATOR_ROLE_AUTHORIZATION=true`",
		"`TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION=true`",
		"`TENET_API_ENFORCE_OPERATOR_ROLE_AUTHORIZATION=true`",
		"`TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION=true`",
		"`X-Tenet-Authenticated-Roles`",
		"`X-Tenet-Authorized-Deployments`",
		"`GET /v1/plans/{planId}`",
		"`GET /v1/applies/{applyId}`",
		"`POST /v1/applies/{applyId}/transitions`",
		"`forbidden: authenticated operator role is required for this operator write route`",
		"`operator` / `admin`",
		"`POST /v1/plans/deployments/{deploymentName}`, `POST /v1/applies/plans/{planId}`, `POST /v1/applies/{applyId}/claim`, `POST /v1/applies/{applyId}/heartbeat`, and `POST /v1/applies/{applyId}/transitions` require either an exact trusted `X-Tenet-Authorized-Deployments` grant",
		"`GET /v1/overview/deployments/{deploymentName}/events`",
		"`GET /v1/overview/deployments/{deploymentName}/resource-logs`",
		"`GET /v1/overview/deployments/{deploymentName}/resource-inspection`",
		"`GET /v1/overview/applications` requires trusted `X-Tenet-Authenticated-*` identity and filters returned rows through the union of trusted `X-Tenet-Authorized-Deployments` exact grants and matching `X-Tenet-Authorized-Projects` plus `X-Tenet-Authorized-Environments` catalog placement",
		"returns `200 OK` with `[]` when no grant matches anything",
		"`GET /v1/audit-logs`",
		"`{\"entries\":[]}`",
		"Guarded-read troubleshooting from the embedded dashboard follows the same diagnostic order as the API docs",
		"start with the `Operator context` banner",
		"inspect `identity.authenticated` and `identityProvenance` first",
		"then inspect `roles.provenance.source`, `roles.supportedHeaderNames`, `roles.recognizedNames` / `roles.unrecognizedNames`, and `roles.canRead` before allowlist debugging for viewer-role `403`",
		"non-empty `unrecognizedNames` means forwarded labels are not one of Tenet's capability-granting `viewer` / `operator` / `admin` roles",
		"role provenance intentionally has no `X-User-*` alias path, so `source=absent` means the front door must forward `X-Tenet-Authenticated-Roles`",
		"then inspect `accessVerdicts`, their bounded route examples, any collection-read `Collection scope: filtered to visible deployments` cue, any likely-empty `Collection result: likely empty` cue, any deployment-scoped `Scope grants: exact deployments or matching project/environment scopes` cue, and the `Required trusted headers:` fragment to choose the exact guarded route class",
		"it does not authorize routes client-side",
	} {
		if !strings.Contains(overviewFeatureDoc, fragment) {
			t.Fatalf("expected overview feature doc front-door auth contract to include %q", fragment)
		}
	}
	redactedPlaceholder = strings.Repeat("*", 3)
	for _, forbidden := range []string{
		"`TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION=" + redactedPlaceholder + "`",
		"`TENET_API_ENFORCE_OPERATOR_ROLE_AUTHORIZATION=" + redactedPlaceholder + "`",
	} {
		if strings.Contains(overviewFeatureDoc, forbidden) {
			t.Fatalf("expected overview feature doc front-door auth contract to reject %q", forbidden)
		}
	}

	overviewUIDoc := readRepoFile(t, "docs/architecture/tenet-v1-overview-ui.md")
	for _, fragment := range []string{
		"`TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION=true`",
		"`X-Tenet-Authorized-Deployments`",
		"`GET /v1/plans/{planId}`",
		"`GET /v1/applies/{applyId}`",
		"`POST /v1/applies/{applyId}/transitions`",
		"either an exact trusted `X-Tenet-Authorized-Deployments` grant for the requested deployment",
		"matching `X-Tenet-Authorized-Projects` plus `X-Tenet-Authorized-Environments` catalog placement for that target",
		"deployment-scoped `GET /v1/audit-logs?deployment={deploymentName}`",
		"`GET /v1/overview/applications`, which now requires trusted identity and filters returned rows through the union of trusted `X-Tenet-Authorized-Deployments` exact grants and matching `X-Tenet-Authorized-Projects` plus `X-Tenet-Authorized-Environments` catalog placement",
		"returning `200 OK` with `[]` when no grant matches anything",
		"`GET /v1/audit-logs` now follows that same collection contract under read enforcement",
		"`{\"entries\":[]}`",
		"deployment-scoped `acceptsProjectEnvironmentScopes` evidence in each verdict chip",
	} {
		if !strings.Contains(overviewUIDoc, fragment) {
			t.Fatalf("expected overview UI doc front-door auth contract to include %q", fragment)
		}
	}
	for _, forbidden := range []string{
		"compatibility fragments for current string-contract tests",
		"`TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION=" + redactedPlaceholder + "` `TENET_API_ENFORCE_OPERATOR_ROLE_AUTHORIZATION=" + redactedPlaceholder + "`",
	} {
		if strings.Contains(overviewUIDoc, forbidden) {
			t.Fatalf("expected overview UI doc to keep operator-facing prose instead of compatibility-only fragment %q", forbidden)
		}
	}
}

func TestResourceDrillDownFrontDoorAuthOperatorContract(t *testing.T) {
	localSmoke := readRepoFile(t, "scripts/smoke-local.sh")
	for _, fragment := range []string{
		`AUTH_UNAUTHENTICATED_EVENTS_BODY=`,
		`AUTH_FORBIDDEN_EVENTS_BODY=`,

		`AUTH_FORBIDDEN_RESOURCE_INSPECTION_BODY=`,
		`curl_status_body_request_id "$AUTH_UNAUTHENTICATED_RESOURCE_LOGS_BODY" \`,
		`auth_unauthenticated_resource_logs_status="$CURL_STATUS_BODY_REQUEST_ID_STATUS"`,
		`auth_unauthenticated_resource_logs_request_id="$CURL_STATUS_BODY_REQUEST_ID_REQUEST_ID"`,
		`curl_status_body_request_id "$AUTH_UNAUTHENTICATED_RESOURCE_INSPECTION_BODY" \`,
		`auth_unauthenticated_resource_inspection_status="$CURL_STATUS_BODY_REQUEST_ID_STATUS"`,
		`auth_unauthenticated_resource_inspection_request_id="$CURL_STATUS_BODY_REQUEST_ID_REQUEST_ID"`,
		`curl_status_body_request_id "$AUTH_FORBIDDEN_RESOURCE_LOGS_BODY" \`,
		`auth_forbidden_resource_logs_status="$CURL_STATUS_BODY_REQUEST_ID_STATUS"`,
		`auth_forbidden_resource_logs_request_id="$CURL_STATUS_BODY_REQUEST_ID_REQUEST_ID"`,
		`curl_status_body_request_id "$AUTH_FORBIDDEN_RESOURCE_INSPECTION_BODY" \`,
		`auth_forbidden_resource_inspection_status="$CURL_STATUS_BODY_REQUEST_ID_STATUS"`,
		`auth_forbidden_resource_inspection_request_id="$CURL_STATUS_BODY_REQUEST_ID_REQUEST_ID"`,
		`allowed_resource_logs=$(curl -fsS \`,
		`allowed_resource_inspection=$(curl -fsS \`,
		`"http://$HOST:$PORT/v1/overview/deployments/worker-prod/resource-logs?resourceType=ecsService&resourceName=worker-prod-service")`,
		`"http://$HOST:$PORT/v1/overview/deployments/worker-prod/resource-inspection?resourceType=ecsService&resourceName=worker-prod-service")`,
		`assert os.environ['AUTH_UNAUTHENTICATED_RESOURCE_LOGS_STATUS'] == '401', os.environ['AUTH_UNAUTHENTICATED_RESOURCE_LOGS_STATUS']`,
		`assert os.environ['AUTH_UNAUTHENTICATED_RESOURCE_INSPECTION_STATUS'] == '401', os.environ['AUTH_UNAUTHENTICATED_RESOURCE_INSPECTION_STATUS']`,
		`assert os.environ['AUTH_FORBIDDEN_RESOURCE_LOGS_STATUS'] == '403', os.environ['AUTH_FORBIDDEN_RESOURCE_LOGS_STATUS']`,
		`assert os.environ['AUTH_FORBIDDEN_RESOURCE_INSPECTION_STATUS'] == '403', os.environ['AUTH_FORBIDDEN_RESOURCE_INSPECTION_STATUS']`,
		`assert allowed_resource_logs['deploymentName'] == 'worker-prod', allowed_resource_logs`,
		`assert allowed_resource_logs['resourceType'] == 'ecsService', allowed_resource_logs`,
		`assert allowed_resource_logs['resourceName'] == 'worker-prod-service', allowed_resource_logs`,
		`assert isinstance(allowed_resource_logs['available'], bool), allowed_resource_logs`,
		`assert allowed_resource_inspection['deploymentName'] == 'worker-prod', allowed_resource_inspection`,
		`assert allowed_resource_inspection['resourceType'] == 'ecsService', allowed_resource_inspection`,
		`assert allowed_resource_inspection['resourceName'] == 'worker-prod-service', allowed_resource_inspection`,
		`assert isinstance(allowed_resource_inspection['available'], bool), allowed_resource_inspection`,
		`assert isinstance(allowed_resource_logs.get('unavailableReason'), str) and allowed_resource_logs['unavailableReason'], allowed_resource_logs`,
		`assert isinstance(allowed_resource_inspection.get('unavailableReason'), str) and allowed_resource_inspection['unavailableReason'], allowed_resource_inspection`,
	} {
		if !strings.Contains(localSmoke, fragment) {
			t.Fatalf("expected smoke-local resource drill-down auth contract to include %q", fragment)
		}
	}

	dockerSmoke := readRepoFile(t, "scripts/smoke-docker.sh")
	for _, fragment := range []string{
		`AUTH_UNAUTHENTICATED_EVENTS_BODY=`,
		`AUTH_FORBIDDEN_EVENTS_BODY=`,

		`AUTH_FORBIDDEN_RESOURCE_INSPECTION_BODY=`,
		`curl_status_body_request_id "$AUTH_UNAUTHENTICATED_RESOURCE_LOGS_BODY" \`,
		`auth_unauthenticated_resource_logs_status="$CURL_STATUS_BODY_REQUEST_ID_STATUS"`,
		`auth_unauthenticated_resource_logs_request_id="$CURL_STATUS_BODY_REQUEST_ID_REQUEST_ID"`,
		`curl_status_body_request_id "$AUTH_UNAUTHENTICATED_RESOURCE_INSPECTION_BODY" \`,
		`auth_unauthenticated_resource_inspection_status="$CURL_STATUS_BODY_REQUEST_ID_STATUS"`,
		`auth_unauthenticated_resource_inspection_request_id="$CURL_STATUS_BODY_REQUEST_ID_REQUEST_ID"`,
		`curl_status_body_request_id "$AUTH_FORBIDDEN_RESOURCE_LOGS_BODY" \`,
		`auth_forbidden_resource_logs_status="$CURL_STATUS_BODY_REQUEST_ID_STATUS"`,
		`auth_forbidden_resource_logs_request_id="$CURL_STATUS_BODY_REQUEST_ID_REQUEST_ID"`,
		`curl_status_body_request_id "$AUTH_FORBIDDEN_RESOURCE_INSPECTION_BODY" \`,
		`auth_forbidden_resource_inspection_status="$CURL_STATUS_BODY_REQUEST_ID_STATUS"`,
		`auth_forbidden_resource_inspection_request_id="$CURL_STATUS_BODY_REQUEST_ID_REQUEST_ID"`,
		`allowed_resource_logs=$(curl -fsS \`,
		`allowed_resource_inspection=$(curl -fsS \`,
		`"$API_BASE_URL/v1/overview/deployments/worker-prod/resource-logs?resourceType=ecsService&resourceName=worker-prod-service")`,
		`"$API_BASE_URL/v1/overview/deployments/worker-prod/resource-inspection?resourceType=ecsService&resourceName=worker-prod-service")`,
		`assert os.environ["AUTH_UNAUTHENTICATED_RESOURCE_LOGS_STATUS"] == "401", os.environ["AUTH_UNAUTHENTICATED_RESOURCE_LOGS_STATUS"]`,
		`assert os.environ["AUTH_UNAUTHENTICATED_RESOURCE_INSPECTION_STATUS"] == "401", os.environ["AUTH_UNAUTHENTICATED_RESOURCE_INSPECTION_STATUS"]`,
		`assert os.environ["AUTH_FORBIDDEN_RESOURCE_LOGS_STATUS"] == "403", os.environ["AUTH_FORBIDDEN_RESOURCE_LOGS_STATUS"]`,
		`assert os.environ["AUTH_FORBIDDEN_RESOURCE_INSPECTION_STATUS"] == "403", os.environ["AUTH_FORBIDDEN_RESOURCE_INSPECTION_STATUS"]`,
		`assert allowed_resource_logs["deploymentName"] == "worker-prod", allowed_resource_logs`,
		`assert allowed_resource_logs["resourceType"] == "ecsService", allowed_resource_logs`,
		`assert allowed_resource_logs["resourceName"] == "worker-prod-service", allowed_resource_logs`,
		`assert isinstance(allowed_resource_logs["available"], bool), allowed_resource_logs`,
		`assert allowed_resource_inspection["deploymentName"] == "worker-prod", allowed_resource_inspection`,
		`assert allowed_resource_inspection["resourceType"] == "ecsService", allowed_resource_inspection`,
		`assert allowed_resource_inspection["resourceName"] == "worker-prod-service", allowed_resource_inspection`,
		`assert isinstance(allowed_resource_inspection["available"], bool), allowed_resource_inspection`,
		`assert isinstance(allowed_resource_logs.get("unavailableReason"), str) and allowed_resource_logs["unavailableReason"], allowed_resource_logs`,
		`assert isinstance(allowed_resource_inspection.get("unavailableReason"), str) and allowed_resource_inspection["unavailableReason"], allowed_resource_inspection`,
	} {
		if !strings.Contains(dockerSmoke, fragment) {
			t.Fatalf("expected smoke-docker resource drill-down auth contract to include %q", fragment)
		}
	}

	operatorContractsDoc := readRepoFile(t, "docs/architecture/tenet-v1-operator-contracts.md")
	for _, fragment := range []string{
		"`GET /v1/overview/deployments/{deploymentName}/resource-logs`",
		"`GET /v1/overview/deployments/{deploymentName}/resource-inspection`",
		"`401 Unauthorized` unauthenticated reads",
		"`403 Forbidden` authenticated-but-unauthorized reads",
		"`200 OK` authorized reads",
		"`available=false` plus explicit `unavailableReason` payload contract",
		"unknown live-inspection codes out of operator copy",
	} {
		if !strings.Contains(operatorContractsDoc, fragment) {
			t.Fatalf("expected operator contracts doc resource drill-down auth contract to include %q", fragment)
		}
	}

	localRuntimeDoc := readRepoFile(t, "docs/architecture/tenet-v1-local-runtime.md")
	for _, fragment := range []string{
		"`GET /v1/overview/deployments/{deploymentName}/resource-logs`",
		"`GET /v1/overview/deployments/{deploymentName}/resource-inspection`",
		"The same trusted front-door read contract also covers",
	} {
		if !strings.Contains(localRuntimeDoc, fragment) {
			t.Fatalf("expected local runtime doc resource drill-down auth contract to include %q", fragment)
		}
	}

	overviewUIDoc := readRepoFile(t, "docs/architecture/tenet-v1-overview-ui.md")
	for _, fragment := range []string{
		"`GET /v1/overview/deployments/{deploymentName}/resource-logs`",
		"`GET /v1/overview/deployments/{deploymentName}/resource-inspection`",
		"`401`",
		"`403`",
		"`available=false` plus `unavailableReason`",
		"unknown reason codes use a generic operator-facing fallback instead of raw machine codes",
	} {
		if !strings.Contains(overviewUIDoc, fragment) {
			t.Fatalf("expected overview UI doc resource drill-down auth contract to include %q", fragment)
		}
	}
}

func TestECSTaskSampleUnavailableReleaseGuardOperatorContract(t *testing.T) {
	documents := map[string][]string{
		"docs/architecture/tenet-v1-operator-contracts.md": {
			"ECS service resource pages also expose a compact `Task sample limit` control",
			"sibling `tasksScope` reports `available`, optional `requestedLimit`, `effectiveLimit`, `returnedTaskCount`, `truncationStatus`",
			"the embedded UI renders those optional placement facts with the operator-facing labels `Container instance`, `EC2 instance`, `Capacity provider`, and `Availability Zone`, and local/no-AWS payloads that omit them do not show empty placeholders",
			"Interpret the scope fields literally: `available=true` with `returnedTaskCount=0` is a complete empty task sample",
			"`available=false` / `truncationStatus=not_applicable` means the service inspection succeeded but the optional task bridge could not be sampled",
			"\"unavailableReason\": \"aws_unavailable\"",
		},
		"docs/features/overview-dashboard.md": {
			"`resource-inspection?resourceType=ecsService&resourceName={serviceName}&limit={n}` accepts an optional positive task-sample cap",
			"`ecsService.tasks[]` renders as `Recent ECS tasks` with task/container/awslogs evidence",
			"UI labels them as `Container instance`, `EC2 instance`, `Capacity provider`, and `Availability Zone`, omits those placement rows entirely when local/no-AWS payloads do not include them",
			"`ecsService.tasksScope` renders as a task-sample scope note with `available`, optional `requestedLimit`, `effectiveLimit`, `returnedTaskCount`",
			"That separates true empty successful samples (`available=true`, `returnedTaskCount=0`) from partial/unavailable task-bridge evidence",
			"local and compose smoke also lock the embedded shell helper/copy that renders `ecsService.tasksScope` as either the normal `Task sample scope` note or the distinct `Task sample unavailable` warning",
		},
		"docs/architecture/tenet-v1-overview-ui.md": {
			"`resource-inspection?resourceType=ecsService&resourceName={serviceName}&limit={n}` accepts an optional positive task-sample cap",
			"each task row may also include optional container identity evidence (`imageDigest`, `runtimeId`), optional live placement evidence (`containerInstanceArn`, `ec2InstanceId`, `capacityProviderName`, `availabilityZone`) from `DescribeTasks` and the optional ECS-on-EC2 container-instance host lookup, plus optional nested `ec2Instance` host facts",
			"local/no-AWS payloads simply omit those optional fields",
			"sibling `tasksScope` keeps the row cap and sampling state self-describing with `available`, optional `requestedLimit`, `effectiveLimit`, `returnedTaskCount`",
			"`not_applicable` when the optional task bridge is unavailable",
			"Empty successful samples remain `available=true` with `returnedTaskCount=0`",
			"a distinct `Task sample unavailable` warning when `tasksScope.available=false`",
		},
	}

	for path, fragments := range documents {
		body := readRepoFile(t, path)
		for _, fragment := range fragments {
			if !strings.Contains(body, fragment) {
				t.Fatalf("expected %s to lock ECS task-sample release-guard wording %q", path, fragment)
			}
		}
	}

	smokeContracts := map[string][]string{
		"scripts/smoke-local.sh": {
			`assert "'Task sample limit'" in body, body`,
			`assert "Task sample unavailable · ${reasonText} · requested ${countText(scope.requestedLimit)} · effective ${countText(effectiveLimit)}" in body, body`,
			`assert "Task sampling is unavailable for this service; service inspection remains available." in body, body`,
			`assert "Task sample scope · returned ${countText(returnedTaskCount)} task(s) from effective limit ${countText(effectiveLimit)} · ${truncationStatus}" in body, body`,
			`assert "const resourceInspectionResultScopeFields = (scope) => {" in body, body`,
			`assert "'Requested sample cap'" in body, body`,
			`assert "'Effective sample cap'" in body, body`,
			`assert "'Returned sample items'" in body, body`,
			`assert 'Container instance' in ui, ui`,
			`assert 'EC2 instance' in ui, ui`,
			`assert 'Capacity provider' in ui, ui`,
			`assert 'Availability Zone' in ui, ui`,
		},
		"scripts/smoke-docker.sh": {
			`"'Task sample limit'",`,
			`"Task sample unavailable · ${reasonText} · requested ${countText(scope.requestedLimit)} · effective ${countText(effectiveLimit)}",`,
			`"Task sampling is unavailable for this service; service inspection remains available.",`,
			`"Task sample scope · returned ${countText(returnedTaskCount)} task(s) from effective limit ${countText(effectiveLimit)} · ${truncationStatus}",`,
			`"const resourceInspectionResultScopeFields = (scope) => {",`,
			`"'Requested sample cap'",`,
			`"'Effective sample cap'",`,
			`"'Returned sample items'",`,
			`"Container instance",`,
			`"EC2 instance",`,
			`"Capacity provider",`,
			`"Availability Zone",`,
		},
	}

	for path, fragments := range smokeContracts {
		body := readRepoFile(t, path)
		for _, fragment := range fragments {
			if !strings.Contains(body, fragment) {
				t.Fatalf("expected %s to lock ECS task-sample smoke release-guard fragment %q", path, fragment)
			}
		}
	}
}

func TestDeploymentEventsFrontDoorAuthOperatorContract(t *testing.T) {
	localSmoke := readRepoFile(t, "scripts/smoke-local.sh")
	for _, fragment := range []string{
		`AUTH_UNAUTHENTICATED_EVENTS_BODY=`,
		`AUTH_FORBIDDEN_EVENTS_BODY=`,

		`"http://$HOST:$PORT/v1/overview/deployments/payments-api-staging/events?limit=1")`,
		`deployment_events_fixture_limited=$(curl -fsS "http://$HOST:$PORT/v1/overview/deployments/payments-api-prod/events?limit=1")`,
		`DEPLOYMENT_EVENTS_FIXTURE_LIMITED_PATH="$deployment_events_fixture_limited_path"`,
		`assert fixture_events_scope.get('returnedEventCount') == 0, deployment_events_fixture_limited`,
		`assert fixture_events_scope.get('returnedCount') == fixture_events_scope.get('returnedEventCount'), deployment_events_fixture_limited`,
		`assert fixture_events_scope.get('truncationStatus') == 'not_truncated', deployment_events_fixture_limited`,
		`assert fixture_events_scope.get('truncated') is False, deployment_events_fixture_limited`,
		`auth_forbidden_events_status=$(curl -sS -o "$AUTH_FORBIDDEN_EVENTS_BODY" -w "%{http_code}" \`,
		`allowed_events_read=$(curl -fsS \`,
		`allowed_events_read_path=$(write_payload allowed-events-read.json "$allowed_events_read")`,
		`AUTH_UNAUTHENTICATED_EVENTS_STATUS="$auth_unauthenticated_events_status"`,
		`AUTH_FORBIDDEN_EVENTS_STATUS="$auth_forbidden_events_status"`,
		`ALLOWED_EVENTS_READ_PATH="$allowed_events_read_path"`,
		`assert os.environ['AUTH_UNAUTHENTICATED_EVENTS_STATUS'] == '401', os.environ['AUTH_UNAUTHENTICATED_EVENTS_STATUS']`,
		`assert unauthenticated_events_body.strip() == 'unauthenticated: authenticated identity is required for this operator read route', unauthenticated_events_body`,
		`assert os.environ['AUTH_FORBIDDEN_EVENTS_STATUS'] == '403', os.environ['AUTH_FORBIDDEN_EVENTS_STATUS']`,
		`assert forbidden_events_body.strip() == 'forbidden: operator does not have access to this deployment', forbidden_events_body`,
		`assert allowed_events_read['deploymentName'] == 'payments-api-staging', allowed_events_read`,
		`assert isinstance(allowed_events_read.get('events'), list), allowed_events_read`,
		`allowed_events_scope = allowed_events_read.get('resultScope') or {}`,
		`assert allowed_events_scope.get('requestedLimit') == 1, allowed_events_read`,
		`assert allowed_events_scope.get('effectiveLimit') == 1, allowed_events_read`,
		`assert allowed_events_scope.get('returnedEventCount') == len(allowed_events_read['events']), allowed_events_read`,
		`assert allowed_events_scope.get('returnedCount') == allowed_events_scope.get('returnedEventCount'), allowed_events_read`,
		`assert isinstance(allowed_events_scope.get('totalEventCount'), int), allowed_events_read`,
		`assert allowed_events_scope.get('totalEventCount') >= allowed_events_scope.get('returnedEventCount'), allowed_events_read`,
		`assert isinstance(allowed_events_scope.get('limited'), bool), allowed_events_read`,
		`assert allowed_events_scope.get('limited') == (allowed_events_scope.get('totalEventCount') > allowed_events_scope.get('returnedEventCount')), allowed_events_read`,
		`assert allowed_events_scope.get('truncationStatus') in {'truncated', 'not_truncated', 'unknown'}, allowed_events_read`,
		`assert allowed_events_scope.get('truncated') == (allowed_events_scope.get('truncationStatus') == 'truncated'), allowed_events_read`,
		`assert all('controllerRefresh' not in event for event in deployment_events_fixture['events']), deployment_events_fixture`,
		`assert 'controllerRefresh' not in event, events_payload`,
	} {
		if !strings.Contains(localSmoke, fragment) {
			t.Fatalf("expected smoke-local deployment events auth contract to include %q", fragment)
		}
	}

	dockerSmoke := readRepoFile(t, "scripts/smoke-docker.sh")
	for _, fragment := range []string{
		`AUTH_UNAUTHENTICATED_EVENTS_BODY=`,
		`AUTH_FORBIDDEN_EVENTS_BODY=`,

		`"$API_BASE_URL/v1/overview/deployments/payments-api-staging/events?limit=1")`,
		`deployment_events=$(curl -fsS "$API_BASE_URL/v1/overview/deployments/payments-api-prod/events?limit=1")`,
		`events_scope = deployment_events.get("resultScope") or {}`,
		`assert events_scope.get("returnedEventCount") == len(deployment_events["events"]), deployment_events`,
		`assert events_scope.get("returnedCount") == events_scope.get("returnedEventCount"), deployment_events`,
		`assert events_scope.get("limited") == (events_scope.get("totalEventCount") > events_scope.get("returnedEventCount")), deployment_events`,
		`assert events_scope.get("truncated") == (events_scope.get("truncationStatus") == "truncated"), deployment_events`,
		`auth_forbidden_events_status=$(curl -sS -o "$AUTH_FORBIDDEN_EVENTS_BODY" -w "%{http_code}" \`,
		`allowed_events_read=$(curl -fsS \`,
		`allowed_events_read_path=$(write_payload allowed-events-read.json "$allowed_events_read")`,
		`AUTH_UNAUTHENTICATED_EVENTS_STATUS="$auth_unauthenticated_events_status"`,
		`AUTH_FORBIDDEN_EVENTS_STATUS="$auth_forbidden_events_status"`,
		`ALLOWED_EVENTS_READ_PATH="$allowed_events_read_path"`,
		`assert os.environ["AUTH_UNAUTHENTICATED_EVENTS_STATUS"] == "401", os.environ["AUTH_UNAUTHENTICATED_EVENTS_STATUS"]`,
		`assert unauthenticated_events_body.strip() == "unauthenticated: authenticated identity is required for this operator read route", unauthenticated_events_body`,
		`assert os.environ["AUTH_FORBIDDEN_EVENTS_STATUS"] == "403", os.environ["AUTH_FORBIDDEN_EVENTS_STATUS"]`,
		`assert forbidden_events_body.strip() == "forbidden: operator does not have access to this deployment", forbidden_events_body`,
		`assert allowed_events_read["deploymentName"] == "payments-api-staging", allowed_events_read`,
		`assert isinstance(allowed_events_read.get("events"), list), allowed_events_read`,
		`allowed_events_scope = allowed_events_read.get("resultScope") or {}`,
		`assert allowed_events_scope.get("requestedLimit") == 1, allowed_events_read`,
		`assert allowed_events_scope.get("effectiveLimit") == 1, allowed_events_read`,
		`assert allowed_events_scope.get("returnedEventCount") == len(allowed_events_read["events"]), allowed_events_read`,
		`assert allowed_events_scope.get("returnedCount") == allowed_events_scope.get("returnedEventCount"), allowed_events_read`,
		`assert isinstance(allowed_events_scope.get("totalEventCount"), int), allowed_events_read`,
		`assert allowed_events_scope.get("totalEventCount") >= allowed_events_scope.get("returnedEventCount"), allowed_events_read`,
		`assert isinstance(allowed_events_scope.get("limited"), bool), allowed_events_read`,
		`assert allowed_events_scope.get("limited") == (allowed_events_scope.get("totalEventCount") > allowed_events_scope.get("returnedEventCount")), allowed_events_read`,
		`assert allowed_events_scope.get("truncationStatus") in {"truncated", "not_truncated", "unknown"}, allowed_events_read`,
		`assert allowed_events_scope.get("truncated") == (allowed_events_scope.get("truncationStatus") == "truncated"), allowed_events_read`,
	} {
		if !strings.Contains(dockerSmoke, fragment) {
			t.Fatalf("expected smoke-docker deployment events auth contract to include %q", fragment)
		}
	}

	operatorContractsDoc := readRepoFile(t, "docs/architecture/tenet-v1-operator-contracts.md")
	for _, fragment := range []string{
		"`GET /v1/overview/deployments/{deploymentName}/events`",
		"`401 Unauthorized` unauthenticated reads",
		"`403 Forbidden` authenticated-but-unauthorized reads",
		"`200 OK` authorized reads",
		"`deploymentName` plus `events` list payload contract",
		"additive `managedDriftSummary` severity counts",
		"top-level additive `sourceSummary` entries (`source`, `count`) computed from the retained event window before `?source=` filtering",
		"top-level `retention` metadata (`limit`, `retainedEventCount`, `status`)",
		"router-authored `resultScope` metadata (`sourceFilter` when exact `?source=` filtering is used, `requestedLimit`, `effectiveLimit`, `returnedEventCount`, `totalEventCount`, `retentionLimit`, `retainedEventCount`, `retentionStatus`, `limited`, optional `newestEventTimestamp` / `oldestEventTimestamp` from the returned window, and `truncationStatus`)",
		"`returnedCount` mirrors `returnedEventCount`, and `truncated` mirrors `truncationStatus == \"truncated\"`",
		"ticker/default controller events do not include `controllerRefresh`",
	} {
		if !strings.Contains(operatorContractsDoc, fragment) {
			t.Fatalf("expected operator contracts doc deployment events auth contract to include %q", fragment)
		}
	}

	localRuntimeDoc := readRepoFile(t, "docs/architecture/tenet-v1-local-runtime.md")
	for _, fragment := range []string{
		"`GET /v1/overview/deployments/{deploymentName}/events`",
		"`401` / `403` / `200 OK` contract",
		"`deploymentName` plus `events` list shape",
		"additive `managedDriftSummary` object carrying explicit zero/non-zero severity counts",
		"top-level `retention` metadata (`limit`, `retainedEventCount`, `status`)",
		"router-authored `resultScope` object (`sourceFilter` when exact `?source=` filtering is used, `requestedLimit`, `effectiveLimit`, optional `newestEventTimestamp` / `oldestEventTimestamp` from the returned window, `returnedEventCount`, compact alias `returnedCount`, `totalEventCount`, `retentionLimit`, `retainedEventCount`, `retentionStatus`, `limited`, `truncationStatus`, compact alias `truncated`) for both default and source-filtered/bounded `?limit=` reads",
		"ticker/default controller-event path omit `controllerRefresh`",
	} {
		if !strings.Contains(localRuntimeDoc, fragment) {
			t.Fatalf("expected local runtime doc deployment events auth contract to include %q", fragment)
		}
	}
}

func TestGitHubWebhookOperatorContract(t *testing.T) {
	compose := readRepoFile(t, "compose.yaml")
	for _, fragment := range []string{
		`TENET_GITHUB_WEBHOOK_SECRET: ${TENET_GITHUB_WEBHOOK_SECRET:-smoke-webhook-secret}`,
	} {
		if !strings.Contains(compose, fragment) {
			t.Fatalf("expected compose github webhook contract to include %q", fragment)
		}
	}

	operatorContractsDoc := readRepoFile(t, "docs/architecture/tenet-v1-operator-contracts.md")
	for _, fragment := range []string{
		"`POST /api/v1/github/webhook`",
		"`TENET_GITHUB_WEBHOOK_SECRET`",
		"`smoke-webhook-secret`",
		"`TENET_GITHUB_WEBHOOK_SECRET=smoke-webhook-secret`",
		"`X-GitHub-Delivery`",
		"`X-GitHub-Event`",
		"`X-Hub-Signature-256`",
		"`202 Accepted`",
		"`200 OK`",
		"`400 Bad Request`",
		"`status=accepted`",
		"`status=ignored`",
		"`deleted=true`",
		"`processed=false`",
		"`processed=true`",
		"`desiredStateSourceMatch`",
		"`matchStatus`",
		"`controllerRefresh`",
		"`TENET_STATUS_BACKEND=redis`",
	} {
		if !strings.Contains(operatorContractsDoc, fragment) {
			t.Fatalf("expected operator contracts doc github webhook contract to include %q", fragment)
		}
	}

	for _, malformedFragment := range []string{
		"`TENET_GITHUB_WEBHOOK_SECRET=*** so `make smoke-docker`",
		"`TENET_GITHUB_WEBHOOK_SECRET=smoke-webhook-secret so `make smoke-docker`",
	} {
		if strings.Contains(operatorContractsDoc, malformedFragment) {
			t.Fatalf("expected operator contracts doc to avoid malformed inline webhook-secret fragment %q", malformedFragment)
		}
	}

	overviewDoc := readRepoFile(t, "docs/architecture/tenet-v1-overview.md")
	for _, fragment := range []string{
		"`POST /api/v1/github/webhook`",
		"`TENET_GITHUB_WEBHOOK_SECRET`",
		"`accepted` or `ignored` responses",
		"`deleted=true`",
		"`desiredStateSourceMatch`",
		"`controllerRefresh`",
		"`processed=false`",
		"`processed=true`",
	} {
		if !strings.Contains(overviewDoc, fragment) {
			t.Fatalf("expected overview architecture doc to include %q", fragment)
		}
	}

	controlPlaneDoc := readRepoFile(t, "docs/architecture/tenet-v1-aws-central-control-plane.md")
	for _, fragment := range []string{
		"`TENET_API_TRUST_AUTHENTICATED_HEADERS=true`",
		"`TENET_API_ENFORCE_AUTHENTICATED_IDENTITY=true`",
		"`TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION=true`",
		"`X-Tenet-Authorized-Deployments`",
		"strip any inbound `X-Tenet-Authenticated-*`, `X-Tenet-Authorized-Deployments`, `X-Tenet-Authorized-Projects`, `X-Tenet-Authorized-Environments`, and trusted-alias `X-User-*` headers from untrusted clients",
		"callers can spoof operator identity or deployment access by sending those headers directly",
		"can now also run an explicitly configured Tenet-managed GitHub OAuth, Google Workspace OIDC, or Okta OIDC browser login inside `tenet-api`",
		"internal `authidentity` package provides JSON-backed normalized identity-store groundwork toward a future durable `auth_identities` table",
		"internal `authstate` package provides JSON-backed OAuth callback state groundwork with expiry and one-time consume semantics for future CSRF/state validation",
		"exposes the matching login/callback routes such as `GET /auth/login/github`, `GET /auth/login/google`, `GET /auth/login/okta`",
		"reports `GET /v1/operator/context.authRuntime.tenetManagedLogin.active=true`",
		"issues a signed `tenet_session` cookie",
		"`HttpOnly`, `SameSite=Lax`",
		"`TENET_AUTH_SESSION_COOKIE_SECURE` override",
		"local HTTP development keeps the cookie non-`Secure`",
		"Okta SAML activation, provider-token refresh/revocation, and long-lived token lifecycle management remain future work",
		"The legacy `/api/v1/auth/*` route family remains unregistered.",
		"Reviewer verification for managed-login changes should include `go test ./internal/api ./cmd/tenet-api`",
		"go test ./internal/authprovider ./internal/authidentity ./internal/authstate",
		"unless those OAuth environment variables are supplied",
		"`TENET_GITHUB_WEBHOOK_SECRET`",
		"`POST /api/v1/github/webhook`",
		"`smoke-webhook-secret`",
		"HMAC-SHA256 signatures",
		"`202 Accepted`",
		"`200 OK`",
		"`400 Bad Request`",
		"`repository_mismatch`",
		"`ref_mismatch`",
		"`path_untouched`",
		"`matched`",
		"`deleted=true`",
		"`controllerRefresh`",
		"`processed=true`",
		"`processed=false`",
	} {
		if !strings.Contains(controlPlaneDoc, fragment) {
			t.Fatalf("expected aws central control plane doc to include %q", fragment)
		}
	}

	dockerSmoke := readRepoFile(t, "scripts/smoke-docker.sh")
	for _, fragment := range []string{
		`WEBHOOK_SECRET=${TENET_GITHUB_WEBHOOK_SECRET:-smoke-webhook-secret}`,
		`webhook_push_status=$(curl -sS -o "$WEBHOOK_PUSH_BODY_FILE" -w "%{http_code}" -X POST \`,
		`"$API_BASE_URL/api/v1/github/webhook")`,
		`assert os.environ["WEBHOOK_PUSH_STATUS"] == "202", os.environ["WEBHOOK_PUSH_STATUS"]`,
		`assert webhook_push["desiredStateSourceMatch"]["kind"] == "filesystem", webhook_push`,
		`assert webhook_push["desiredStateSourceMatch"]["matchStatus"] == "filesystem_source", webhook_push`,
		`assert os.environ["WEBHOOK_DELETED_STATUS"] == "200", os.environ["WEBHOOK_DELETED_STATUS"]`,
		`assert webhook_deleted["status"] == "ignored", webhook_deleted`,
		`assert webhook_deleted["deleted"] is True, webhook_deleted`,
		`assert "controller refresh handoff skipped" in webhook_deleted["reason"], webhook_deleted`,
		`assert "controllerRefresh" not in webhook_deleted, webhook_deleted`,
		`assert os.environ["WEBHOOK_PING_STATUS"] == "200", os.environ["WEBHOOK_PING_STATUS"]`,
		`assert webhook_ping["status"] == "ignored", webhook_ping`,
		`assert os.environ["INVALID_WEBHOOK_STATUS"] == "400", os.environ["INVALID_WEBHOOK_STATUS"]`,
		`assert "github webhook signature mismatch" in invalid_webhook, invalid_webhook`,
	} {
		if !strings.Contains(dockerSmoke, fragment) {
			t.Fatalf("expected smoke-docker github webhook contract to include %q", fragment)
		}
	}

	script := readRepoFile(t, "scripts/smoke-local.sh")
	for _, fragment := range []string{
		`TENET_GITHUB_WEBHOOK_SECRET="$WEBHOOK_SECRET"`,
		`webhook_push=$(curl -fsS -X POST \`,
		`http://$HOST:$PORT/api/v1/github/webhook`,
		`assert webhook_push['status'] == 'accepted', webhook_push`,
		`assert webhook_push['desiredStateSourceMatch']['kind'] == 'filesystem', webhook_push`,
		`assert webhook_push['desiredStateSourceMatch']['matchStatus'] == 'filesystem_source', webhook_push`,
		`assert webhook_deleted['status'] == 'ignored', webhook_deleted`,
		`assert webhook_deleted['deleted'] is True, webhook_deleted`,
		`assert 'controller refresh handoff skipped' in webhook_deleted['reason'], webhook_deleted`,
		`assert 'controllerRefresh' not in webhook_deleted, webhook_deleted`,
		`assert webhook_ping['status'] == 'ignored', webhook_ping`,
		`assert 'desiredStateSourceMatch' not in webhook_ping, webhook_ping`,
		`assert os.environ['INVALID_WEBHOOK_STATUS'] == '400', os.environ['INVALID_WEBHOOK_STATUS']`,
		`assert 'github webhook signature mismatch' in invalid_webhook, invalid_webhook`,
	} {
		if !strings.Contains(script, fragment) {
			t.Fatalf("expected smoke-local github webhook contract to include %q", fragment)
		}
	}
}

func TestResourceLogsOperatorContract(t *testing.T) {
	script := readRepoFile(t, "scripts/smoke-local.sh")
	for _, fragment := range []string{
		`assert isinstance(resource_logs.get('lookupContext'), dict), resource_logs`,
		`assert isinstance(resource_logs['lookupContext'].get('region'), str) and resource_logs['lookupContext']['region'], resource_logs`,
		`assert isinstance(resource_logs['lookupContext'].get('controlAccountId'), str) and resource_logs['lookupContext']['controlAccountId'], resource_logs`,
		`assert isinstance(resource_logs['lookupContext'].get('targetAccountId'), str) and resource_logs['lookupContext']['targetAccountId'], resource_logs`,
		`assert isinstance(resource_logs['lookupContext'].get('crossAccount'), bool), resource_logs`,
		`assert isinstance(resource_logs['lookupContext'].get('observerRoleConfigured'), bool), resource_logs`,
		`assert isinstance(worker_resource_logs.get('lookupContext'), dict), worker_resource_logs`,
		`assert isinstance(worker_resource_logs['lookupContext'].get('observerRoleConfigured'), bool), worker_resource_logs`,
		`selected_worker_resource_logs=$(curl -fsS "http://$HOST:$PORT/v1/overview/deployments/worker-prod/resource-logs?resourceType=ecsService&resourceName=worker-prod-service&clusterArn=arn%3Aaws%3Aecs%3Aap-northeast-2%3A123456789012%3Acluster%2Fworker-prod&limit=1")`,
		`assert selected_worker_resource_logs.get('clusterArn') == worker_cluster_arn, selected_worker_resource_logs`,
		`selected_worker_resource_logs_path=$(write_payload selected-worker-resource-logs.json "$selected_worker_resource_logs")`,
		`SELECTED_WORKER_RESOURCE_LOGS_PATH="$selected_worker_resource_logs_path"`,
		`traffic_resource_logs=$(curl -fsS "http://$HOST:$PORT/v1/overview/deployments/payments-api-prod/resource-logs?resourceType=trafficShiftPlan&resourceName=payments-api-prod-weighted-routing")`,
		`assert traffic_resource_logs.get('unavailableReason') == 'no_log_source', traffic_resource_logs`,
	} {
		if !strings.Contains(script, fragment) {
			t.Fatalf("expected smoke-local resource logs contract to include %q", fragment)
		}
	}

	dockerScript := readRepoFile(t, "scripts/smoke-docker.sh")
	for _, fragment := range []string{
		`selected_worker_resource_logs=$(curl -fsS "$API_BASE_URL/v1/overview/deployments/worker-prod/resource-logs?resourceType=ecsService&resourceName=worker-prod-service&clusterArn=arn%3Aaws%3Aecs%3Aap-northeast-2%3A123456789012%3Acluster%2Fworker-prod&limit=1")`,
		`assert selected_worker_resource_logs.get("clusterArn") == worker_cluster_arn, selected_worker_resource_logs`,
		`selected_worker_resource_logs_path=$(write_payload selected-worker-resource-logs.json "$selected_worker_resource_logs")`,
		`SELECTED_WORKER_RESOURCE_LOGS_PATH="$selected_worker_resource_logs_path"`,
		`traffic_resource_logs=$(curl -fsS "$API_BASE_URL/v1/overview/deployments/payments-api-prod/resource-logs?resourceType=trafficShiftPlan&resourceName=payments-api-prod-weighted-routing")`,
		`assert traffic_resource_logs.get("unavailableReason") == "no_log_source", traffic_resource_logs`,
	} {
		if !strings.Contains(dockerScript, fragment) {
			t.Fatalf("expected smoke-docker resource logs contract to include %q", fragment)
		}
	}

	readme := readRepoFile(t, "README.md")
	for _, fragment := range []string{
		"`GET /v1/overview/deployments/{deploymentName}/resource-logs?resourceType={type}&resourceName={name}[&limit={positiveInteger}][&containerName={container}][&instanceId={ec2InstanceId}][&clusterArn={ecsClusterArn}]`",
		"`resultScope.requestedLimit`",
		"`effectiveLimit`",
		"`returnedEntryCount`",
		"`returnedCount`",
		"`truncationStatus`",
		"`truncated`",
		"compact aliases such as `limit`/`returned` keep bounded reads terse",
		"ECS\n  task container, ASG instance, and ECS cluster selectors keep focused reads explicit",
	} {
		if !strings.Contains(readme, fragment) {
			t.Fatalf("expected README resource logs contract to include %q", fragment)
		}
	}

	operatorContractsDoc := readRepoFile(t, "docs/architecture/tenet-v1-operator-contracts.md")
	for _, fragment := range []string{
		"`lookupContext` object",
		"`lookupContext.region`",
		"`lookupContext.controlAccountId`",
		"`lookupContext.targetAccountId`",
		"`lookupContext.crossAccount`",
		"`lookupContext.observerRoleConfigured`",
		"present for both `available=true` and unavailable responses",
		"`trafficShiftPlan`, `productionListenerRule`, `activeTargetGroup`, and `candidateTargetGroup` also preserve the same route and lookup-context envelope",
		"`unavailableReason=no_log_source`",
		"capture the HTTP `Request-Id` response header next to the `lookupContext` object before escalating",
		"the request ID anchors Tenet's structured API logs while the lookup context explains which region, control account, target account, and observer-role path the provider attempted",
		"resource-logs?resourceType=ecsService&resourceName=worker-prod-service&clusterArn=arn%3Aaws%3Aecs%3Aap-northeast-2%3A123456789012%3Acluster%2Fworker-prod",
		"resource-logs?resourceType=ecsTask&resourceName=arn%3Aaws%3Aecs%3Aap-northeast-2%3A123456789012%3Atask%2Fworker%2Fabc123&containerName=api&clusterArn=arn%3Aaws%3Aecs%3Aap-northeast-2%3A123456789012%3Acluster%2Fworker-prod&limit=5",
		"`returnedCount` plus compatibility `returnedEntryCount`",
		"boolean `truncated`",
	} {
		if !strings.Contains(operatorContractsDoc, fragment) {
			t.Fatalf("expected operator contracts doc resource log contract to include %q", fragment)
		}
	}

	featureDoc := readRepoFile(t, "docs/features/resource-recent-logs.md")
	for _, fragment := range []string{
		"- `lookupContext`",
		"- `controlAccountId`",
		"- `targetAccountId`",
		"- `crossAccount`",
		"- `observerRoleConfigured`",
		"`lookupContext` object is always present, including unavailable responses",
		"newest running and recently stopped service task/container streams",
		"failed rollouts and crash loops can still surface useful logs",
		"`containerName` is optional and only meaningful for `resourceType=ecsTask`",
		"optional `resourceDescription` when Tenet resolved a rollout execution description",
		"echoes it as `resourceDescription` on both available and unavailable payloads",
		"`ecsTask` reads recent CloudWatch log events for the exact ECS task ARN or task identifier",
		"resourceType=ecsTask&containerName=worker&clusterArn={ecsClusterArn}&limit=1",
		"Add `clusterArn={arn:aws:ecs:...:cluster/name}` when the parent service drill-down selected a concrete cluster",
		"Tenet validates and echoes that selector just like `ecsService`",
		"`aws_unavailable`, `resource_not_found`, `cross_account_not_configured`, or `no_log_source`",
		"Current route-stable rollout resources with explicit no-log-source responses",
		"weighted-target-group traffic-routing resources (`trafficShiftPlan`, `productionListenerRule`, `activeTargetGroup`, and `candidateTargetGroup`)",
		"This resource is a valid drill-down target, but Tenet has no independent recent-log stream for it yet.",
		"Operators should capture the HTTP `Request-Id` response header together with the `lookupContext` object before escalating",
		"that pair gives reviewers the exact Tenet request log correlation key plus the attempted region/account/observer-role path",
		"Example selected-cluster service request",
		"- `returnedCount`",
		"- `truncated`",
		"compatibility `returnedEntryCount={len(entries)}`",
		"resource-logs?resourceType=ecsService&resourceName=worker-prod-service&clusterArn=arn%3Aaws%3Aecs%3Aap-northeast-2%3A123456789012%3Acluster%2Fworker-prod&limit=5",
		"The embedded resource page also preserves bookmarkable `clusterArn` URL state for ECS service and ECS task resources",
		"local/compose smoke now lock the response-level `clusterArn` echo",
		"optional selected-host `autoScalingGroup.instances[].ec2Instance` facts from live `DescribeInstances`",
		"local/no-AWS runs may omit those host facts while keeping the membership row and console-log selector useful",
	} {
		if !strings.Contains(featureDoc, fragment) {
			t.Fatalf("expected resource recent logs doc to include %q", fragment)
		}
	}
}

func TestResourceInspectionOperatorContract(t *testing.T) {
	localSmoke := readRepoFile(t, "scripts/smoke-local.sh")
	for _, fragment := range []string{
		`candidate_resource_inspection=$(curl -fsS "http://$HOST:$PORT/v1/overview/deployments/payments-api-prod/resource-inspection?resourceType=candidateAutoScalingGroup&resourceName=payments-api-prod-candidate")`,
		`codedeploy_group_resource_inspection=$(curl -fsS "http://$HOST:$PORT/v1/overview/deployments/payments-api-staging/resource-inspection?resourceType=codeDeployDeploymentGroup&resourceName=payments-api-staging-codedeploy-group")`,
		`production_listener_resource_inspection=$(curl -fsS "http://$HOST:$PORT/v1/overview/deployments/payments-api-staging/resource-inspection?resourceType=productionListener&resourceName=payments-api-staging-prod-listener")`,
		`test_listener_resource_inspection=$(curl -fsS "http://$HOST:$PORT/v1/overview/deployments/payments-api-staging/resource-inspection?resourceType=testListener&resourceName=payments-api-staging-test-listener")`,
		`production_listener_rule_resource_inspection=$(curl -fsS "http://$HOST:$PORT/v1/overview/deployments/payments-api-prod/resource-inspection?resourceType=productionListenerRule&resourceName=payments-api-prod-prod-listener")`,
		`active_target_group_resource_inspection=$(curl -fsS "http://$HOST:$PORT/v1/overview/deployments/payments-api-prod/resource-inspection?resourceType=activeTargetGroup&resourceName=payments-api-prod-active")`,
		`candidate_target_group_resource_inspection=$(curl -fsS "http://$HOST:$PORT/v1/overview/deployments/payments-api-prod/resource-inspection?resourceType=candidateTargetGroup&resourceName=payments-api-prod-candidate")`,
		`blue_task_set_target_group_resource_inspection=$(curl -fsS "http://$HOST:$PORT/v1/overview/deployments/payments-api-staging/resource-inspection?resourceType=blueTaskSetTargetGroup&resourceName=payments-api-staging-blue")`,
		`assert payload['available'] is True, payload`,
		`assert payload.get('source') in {'rollout_execution_summary', 'live'}, payload`,
		`assert payload.get('unavailableReason') != 'aws_unavailable', payload`,
		`assert isinstance(payload['trafficRoutingResource'].get('displayName'), str) and payload['trafficRoutingResource']['displayName'], payload`,
		`assert payload['trafficRoutingResource'].get('inspectionMode') in {'summary_only', 'live'}, payload`,
		`selected_worker_resource_inspection=$(curl -fsS "http://$HOST:$PORT/v1/overview/deployments/worker-prod/resource-inspection?resourceType=ecsService&resourceName=worker-prod-service&clusterArn=arn%3Aaws%3Aecs%3Aap-northeast-2%3A123456789012%3Acluster%2Fworker-prod&limit=1")`,
		`assert selected_worker_resource_inspection.get('clusterArn') == worker_cluster_arn, selected_worker_resource_inspection`,
		`assert ecs_task_resource_inspection.get('resourceDescription') == 'ECS task selected from live service inspection', ecs_task_resource_inspection`,
		`selected_worker_resource_inspection_path=$(write_payload selected-worker-resource-inspection.json "$selected_worker_resource_inspection")`,
		`SELECTED_WORKER_RESOURCE_INSPECTION_PATH="$selected_worker_resource_inspection_path"`,
		`minimum_healthy_percent = worker_resource_inspection['ecsService'].get('minimumHealthyPercent')`,
		`maximum_percent = worker_resource_inspection['ecsService'].get('maximumPercent')`,
		`health_check_grace_period_seconds = worker_resource_inspection['ecsService'].get('healthCheckGracePeriodSeconds')`,
		`def assert_ecs_service_deployments_contract(label, payload):`,
		`def assert_ecs_service_event_scope_contract(label, payload):`,
		`events_scope = service.get('eventsScope')`,
		`assert type(events_scope.get('effectiveLimit')) is int and events_scope['effectiveLimit'] >= 0, {label: payload}`,
		`assert events_scope.get('returnedEventCount') == len(events), {label: payload}`,
		`assert events_scope.get('truncationStatus') in {'not_truncated', 'truncated', 'unknown', 'not_applicable'}, {label: payload}`,
		`for field in ('newestEventAt', 'oldestEventAt', 'summary'):`,
		`assert_ecs_service_deployments_contract('worker_resource_inspection', worker_resource_inspection)`,
		`assert_ecs_service_deployments_contract('selected_worker_resource_inspection', selected_worker_resource_inspection)`,
		`assert_ecs_service_event_scope_contract('worker_resource_inspection', worker_resource_inspection)`,
		`assert_ecs_service_event_scope_contract('selected_worker_resource_inspection', selected_worker_resource_inspection)`,
		`assert "const createECSServiceEventScopeSummary = (scope) => {" in ui, ui`,
		"assert \"Event window scope · ${parts.join(' · ')}${summary ? ` · ${summary}` : ''}\" in ui, ui",
		`assert "[createECSServiceEventScopeSummary(service?.eventsScope)]" in ui, ui`,
		`assert 'Recent ECS service events' in ui, ui`,
		`assert 'Event timestamp' in ui, ui`,
		`launch_template = asg.get('launchTemplate')`,
		`mixed_instances_policy = asg.get('mixedInstancesPolicy')`,
		`override_launch_template = override.get('launchTemplate')`,
		`assert "const appendAutoScalingEC2InstanceDetail = (item, instance) => {" in ui, ui`,
		`assert "const ec2Instance = instance?.ec2Instance;" in ui, ui`,
		`assert "AMI image ID" in ui, ui`,
		`assert "Public IP" in ui, ui`,
	} {
		if !strings.Contains(localSmoke, fragment) {
			t.Fatalf("expected smoke-local resource inspection contract to include %q", fragment)
		}
	}

	dockerSmoke := readRepoFile(t, "scripts/smoke-docker.sh")
	for _, fragment := range []string{
		`candidate_resource_inspection=$(curl -fsS "$API_BASE_URL/v1/overview/deployments/payments-api-prod/resource-inspection?resourceType=candidateAutoScalingGroup&resourceName=payments-api-prod-candidate")`,
		`codedeploy_group_resource_inspection=$(curl -fsS "$API_BASE_URL/v1/overview/deployments/payments-api-staging/resource-inspection?resourceType=codeDeployDeploymentGroup&resourceName=payments-api-staging-codedeploy-group")`,
		`production_listener_resource_inspection=$(curl -fsS "$API_BASE_URL/v1/overview/deployments/payments-api-staging/resource-inspection?resourceType=productionListener&resourceName=payments-api-staging-prod-listener")`,
		`test_listener_resource_inspection=$(curl -fsS "$API_BASE_URL/v1/overview/deployments/payments-api-staging/resource-inspection?resourceType=testListener&resourceName=payments-api-staging-test-listener")`,
		`production_listener_rule_resource_inspection=$(curl -fsS "$API_BASE_URL/v1/overview/deployments/payments-api-prod/resource-inspection?resourceType=productionListenerRule&resourceName=payments-api-prod-prod-listener")`,
		`active_target_group_resource_inspection=$(curl -fsS "$API_BASE_URL/v1/overview/deployments/payments-api-prod/resource-inspection?resourceType=activeTargetGroup&resourceName=payments-api-prod-active")`,
		`candidate_target_group_resource_inspection=$(curl -fsS "$API_BASE_URL/v1/overview/deployments/payments-api-prod/resource-inspection?resourceType=candidateTargetGroup&resourceName=payments-api-prod-candidate")`,
		`blue_task_set_target_group_resource_inspection=$(curl -fsS "$API_BASE_URL/v1/overview/deployments/payments-api-staging/resource-inspection?resourceType=blueTaskSetTargetGroup&resourceName=payments-api-staging-blue")`,
		`assert payload.get("unavailableReason") != "aws_unavailable", payload`,
		`assert payload.get("source") in {"rollout_execution_summary", "live"}, payload`,
		`assert payload["available"] is True, payload`,
		`assert isinstance(payload["trafficRoutingResource"].get("displayName"), str) and payload["trafficRoutingResource"]["displayName"], payload`,
		`assert payload["trafficRoutingResource"].get("inspectionMode") in {"summary_only", "live"}, payload`,
		`selected_worker_resource_inspection=$(curl -fsS "$API_BASE_URL/v1/overview/deployments/worker-prod/resource-inspection?resourceType=ecsService&resourceName=worker-prod-service&clusterArn=arn%3Aaws%3Aecs%3Aap-northeast-2%3A123456789012%3Acluster%2Fworker-prod&limit=1")`,
		`assert selected_worker_resource_inspection.get("clusterArn") == worker_cluster_arn, selected_worker_resource_inspection`,
		`assert ecs_task_resource_inspection.get("resourceDescription") == "ECS task selected from live service inspection", ecs_task_resource_inspection`,
		`selected_worker_resource_inspection_path=$(write_payload selected-worker-resource-inspection.json "$selected_worker_resource_inspection")`,
		`SELECTED_WORKER_RESOURCE_INSPECTION_PATH="$selected_worker_resource_inspection_path"`,
		`minimum_healthy_percent = worker_resource_inspection["ecsService"].get("minimumHealthyPercent")`,
		`maximum_percent = worker_resource_inspection["ecsService"].get("maximumPercent")`,
		`health_check_grace_period_seconds = worker_resource_inspection["ecsService"].get("healthCheckGracePeriodSeconds")`,
		`def assert_ecs_service_deployments_contract(label, payload):`,
		`def assert_ecs_service_event_scope_contract(label, payload):`,
		`events_scope = service.get("eventsScope")`,
		`assert type(events_scope.get("effectiveLimit")) is int and events_scope["effectiveLimit"] >= 0, {label: payload}`,
		`assert events_scope.get("returnedEventCount") == len(events), {label: payload}`,
		`assert events_scope.get("truncationStatus") in {"not_truncated", "truncated", "unknown", "not_applicable"}, {label: payload}`,
		`for field in ("newestEventAt", "oldestEventAt", "summary"):`,
		`assert_ecs_service_deployments_contract("worker_resource_inspection", worker_resource_inspection)`,
		`assert_ecs_service_deployments_contract("selected_worker_resource_inspection", selected_worker_resource_inspection)`,
		`assert_ecs_service_event_scope_contract("worker_resource_inspection", worker_resource_inspection)`,
		`assert_ecs_service_event_scope_contract("selected_worker_resource_inspection", selected_worker_resource_inspection)`,
		`"const createECSServiceEventScopeSummary = (scope) => {",`,
		"\"Event window scope · ${parts.join(' · ')}${summary ? ` · ${summary}` : ''}\",",
		`"[createECSServiceEventScopeSummary(service?.eventsScope)]",`,
		`"Recent ECS service events",`,
		`"Event timestamp",`,
		`launch_template = asg.get("launchTemplate")`,
		`mixed_instances_policy = asg.get("mixedInstancesPolicy")`,
		`override_launch_template = override.get("launchTemplate")`,
		`"const appendAutoScalingEC2InstanceDetail = (item, instance) => {",`,
		`"const ec2Instance = instance?.ec2Instance;",`,
		`"AMI image ID",`,
		`"Public IP",`,
	} {
		if !strings.Contains(dockerSmoke, fragment) {
			t.Fatalf("expected smoke-docker resource inspection contract to include %q", fragment)
		}
	}

	readme := readRepoFile(t, "README.md")
	for _, fragment := range []string{
		"`GET /v1/overview/deployments/{deploymentName}/resource-inspection?resourceType={type}&resourceName={name}[&clusterArn={ecsClusterArn}][&limit={positiveInteger}][&instanceId={ec2InstanceId}]`",
		"the route requires a concrete resource type/name",
	} {
		if !strings.Contains(readme, fragment) {
			t.Fatalf("expected README resource inspection contract to include %q", fragment)
		}
	}

	operatorContractsDoc := readRepoFile(t, "docs/architecture/tenet-v1-operator-contracts.md")
	for _, fragment := range []string{
		"`GET /v1/overview/deployments/{deploymentName}/resource-inspection?resourceType={type}&resourceName={name}`",
		"both require a concrete resource type/name before provider lookup",
		"ECS service and direct ASG inspection reads may add selectors such as `clusterArn` or a positive `limit`",
		"bootstrap-source inspection uses `instanceId` for focused ASG-member troubleshooting rather than bounded `limit` sampling",
		"Auto Scaling payloads add `autoScalingGroup`, including optional `launchTemplate` and `mixedInstancesPolicy` metadata",
		"`productionListenerRule`",
		"`activeTargetGroup`",
		"`candidateTargetGroup`",
		"`blueTaskSetTargetGroup`",
		"`greenTaskSetTargetGroup`",
		"`launchTemplate.imageId` and `launchTemplate.instanceType`",
		"`minimumHealthyPercent`",
		"`maximumPercent`",
		"`healthCheckGracePeriodSeconds`",
		"`launchTemplate`",
		"`mixedInstancesPolicy`",
		"`launchTemplate.imageId`",
		"`launchTemplate.instanceType`",
		"`listenerRule`",
		"`ruleArn`, `priority`, `isDefault`, `conditions`, `actions`",
		"resource-inspection?resourceType=ecsService&resourceName=worker-prod-service&clusterArn=arn%3Aaws%3Aecs%3Aap-northeast-2%3A123456789012%3Acluster%2Fworker-prod",
		"resource-inspection?resourceType=ecsTask&resourceName=arn%3Aaws%3Aecs%3Aap-northeast-2%3A123456789012%3Atask%2Fworker%2Fabc123&clusterArn=arn%3Aaws%3Aecs%3Aap-northeast-2%3A123456789012%3Acluster%2Fworker-prod",
		"optional `resourceDescription` when Tenet resolved a rollout execution description for the selected resource",
		"Selected ASG host detail is optional live AWS evidence",
		"`autoScalingGroup.instances[].ec2Instance`",
		"`EC2 state`, `EC2 instance type`, `AMI image ID`, `Private IP`, `Public IP`, `Subnet`, `VPC`, and `Launch time`",
		"ECS service `eventsScope` / `Event window scope` smoke coverage stays explicit",
	} {
		if !strings.Contains(operatorContractsDoc, fragment) {
			t.Fatalf("expected operator contracts doc resource inspection contract to include %q", fragment)
		}
	}

	featureDoc := readRepoFile(t, "docs/features/overview-dashboard.md")
	for _, fragment := range []string{
		"`minimumHealthyPercent`",
		"`maximumPercent`",
		"`healthCheckGracePeriodSeconds`",
		"`launchTemplate`",
		"`mixedInstancesPolicy`",
		"`DescribeAutoScalingGroups`",
		"`blueTaskSetTargetGroup`",
		"`greenTaskSetTargetGroup`",
		"`launchTemplate.imageId`",
		"`launchTemplate.instanceType`",
		"`listenerRule` metadata",
		"resource-logs?resourceType=ecsService&resourceName=worker-prod-service&clusterArn=arn%3Aaws%3Aecs%3Aap-northeast-2%3A123456789012%3Acluster%2Fworker-prod&limit=5",
		"resource-inspection?resourceType=ecsService&resourceName=worker-prod-service&clusterArn=arn%3Aaws%3Aecs%3Aap-northeast-2%3A123456789012%3Acluster%2Fworker-prod&limit=5",
		"embedded resource page preserves selected-cluster URL state through bookmarkable ECS service drill-down links",
		"local and compose smoke now also fetch ECS service selected-cluster backend routes",
		"optional `resourceDescription` when Tenet resolved rollout execution context for the selected resource",
		"selected ASG EC2 host-detail and status-check evidence is optional live AWS evidence",
		"local and compose runs may omit `autoScalingGroup.instances[].ec2Instance` and `autoScalingGroup.instances[].ec2InstanceStatus`",
		"the exact operator labels `EC2 state`, `EC2 instance type`, `AMI image ID`, `Private IP`, `Public IP`, `Subnet`, `VPC`, and `Launch time`",
		"the exact local fallback identifiers `codeDeployDeploymentGroup`, `productionListener`, and `testListener`",
		"`available=true`, `source=rollout_execution_summary`, `trafficRoutingResource.inspectionMode=summary_only`",
		"`available=false` / `unavailableReason=aws_unavailable`",
		"`source=live`",
		"only when the live payload includes health counts — a target-health pill/cue and raw `Target group health` counts",
	} {
		if !strings.Contains(featureDoc, fragment) {
			t.Fatalf("expected overview dashboard doc to include %q", fragment)
		}
	}

	uiDoc := readRepoFile(t, "docs/architecture/tenet-v1-overview-ui.md")
	for _, fragment := range []string{
		"resource-detail Troubleshooting summary",
		"derived from the selected resource identity plus the latest completed Live inspection state and the latest completed Recent logs panel state",
		"covers loading, empty, unavailable, and fetch-error states without hiding the selected resource",
		"keeps `Current resource read: GET /v1/overview/deployments/{deploymentName}` visible",
		"`minimumHealthyPercent`",
		"`maximumPercent`",
		"`healthCheckGracePeriodSeconds`",
		"`launchTemplate`",
		"`mixedInstancesPolicy`",
		"`blueTaskSetTargetGroup`",
		"`greenTaskSetTargetGroup`",
		"`launchTemplate.imageId`",
		"`launchTemplate.instanceType`",
		"`listenerRule` details",
		"resource-inspection?resourceType=ecsService&resourceName=worker-prod-service&clusterArn=arn%3Aaws%3Aecs%3Aap-northeast-2%3A123456789012%3Acluster%2Fworker-prod&limit=5",
		"accepted selectors echo as response-level `clusterArn`",
		"may include additive `resourceDescription` when Tenet resolved rollout execution context",
		"resource-logs?resourceType=ecsTask&resourceName=arn%3Aaws%3Aecs%3Aap-northeast-2%3A123456789012%3Atask%2Fworker%2Fabc123&containerName=worker&clusterArn=arn%3Aaws%3Aecs%3Aap-northeast-2%3A123456789012%3Acluster%2Fworker-prod&limit=1",
		"paired `resource-inspection?resourceType=ecsTask&...&clusterArn=...` route",
		"**Clear selected container** removes only `containerName`",
		"**Clear selected cluster** removes only `clusterArn` without losing the other task scope",
		"local/compose smoke lock that backend echo alongside the embedded selected-cluster URL-state guards",
	} {
		if !strings.Contains(uiDoc, fragment) {
			t.Fatalf("expected overview UI doc to include %q", fragment)
		}
	}
}
