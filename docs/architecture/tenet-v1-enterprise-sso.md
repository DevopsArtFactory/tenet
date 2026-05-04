# Tenet v1 enterprise SSO boundary

This note defines the intended boundary for the first enterprise SSO provider work in Tenet. It is a quality gate for issue #662: provider integration should add identity/session inputs without changing Tenet's existing trusted-front-door authorization contract or repository-backed RBAC direction.

## Goal

Tenet should be able to authenticate operators through organization identity providers while keeping the control-plane authorization model explicit and reviewable.

The first provider-neutral SSO slices should support:

- Google Workspace identity through Google OIDC profile claims
- Okta identity through SAML assertions or OIDC profile claims, depending on the configured provider mode
- normalized Tenet identity fields (`id`, `name`, `email`, `provider`, and provider subject)
- non-secret `GET /v1/operator/context` diagnostics that tell operators which provider mode is configured, active, or missing required settings
- local fake provider/assertion/profile tests instead of live Google or Okta calls

Google Workspace/Gmail means identity login only. It does not grant Tenet Gmail mailbox, Drive, Calendar, or other Google API access.

## Current shipped boundary

Tenet already supports two auth inputs that must remain compatible:

1. **Trusted front-door headers** for deployments where an authenticating proxy or load balancer has already authenticated the operator and forwards trusted identity, role, and deployment/project/environment scope headers.
2. **Tenet-managed login groundwork** for optional local browser login/session flows. That runtime uses GitHub OAuth, Google Workspace OIDC, or Okta OIDC for identity lookup only, stores Tenet-managed session cookies, and reports non-secret lifecycle diagnostics through `GET /v1/operator/context.authRuntime.tenetManagedLogin`.

Enterprise SSO should extend the second family as provider-neutral managed-login inputs while preserving the first family as the default production integration model.

## Provider-neutral configuration shape

Provider work is disabled by default. GitHub OAuth, Google Workspace OIDC, and Okta OIDC become active Tenet-managed login providers only when their configured provider mode and required non-secret state, identity, and session settings are complete. Google Workspace OIDC uses `TENET_AUTH_PROVIDER=google_oidc` (canonical diagnostic provider `google`; the legacy `google` alias is accepted), and Okta OIDC uses `TENET_AUTH_PROVIDER=okta_oidc`; Okta SAML remains diagnostic-only and inactive. Configure the diagnostic/runtime with settings such as:

```text
TENET_AUTH_PROVIDER=google_oidc        # set okta_oidc for Okta OIDC; okta_saml remains diagnostic-only
TENET_GOOGLE_OIDC_CLIENT_ID=google-client-id
TENET_GOOGLE_OIDC_CLIENT_SECRET=<secret from Google Workspace>
TENET_GOOGLE_OIDC_ISSUER_URL=https://accounts.google.com
TENET_GOOGLE_OIDC_REDIRECT_URL=https://tenet.example.com/auth/callback/google
TENET_OKTA_SAML_METADATA_URL=https://okta.example.com/app/tenet/sso/saml/metadata
TENET_OKTA_SAML_ACS_URL=https://tenet.example.com/auth/callback/okta/saml
TENET_OKTA_OIDC_ISSUER_URL=https://okta.example.com/oauth2/default
TENET_OKTA_OIDC_CLIENT_ID=okta-client-id
TENET_OKTA_OIDC_CLIENT_SECRET=<secret from Okta>
TENET_OKTA_OIDC_REDIRECT_URL=https://tenet.example.com/auth/callback/okta/oidc
TENET_AUTH_STATE_STORE_PATH=/var/lib/tenet/auth-state.json
TENET_AUTH_IDENTITY_STORE_PATH=/var/lib/tenet/auth-identities.json
TENET_AUTH_SESSION_SECRET=<32+ byte session signing secret>
```

The operator contract for these settings is:

- unset or incomplete provider settings keep the provider inactive
- diagnostics list required, missing, and invalid setting names without returning secret values
- Okta SAML diagnostics validate metadata/ACS values as absolute HTTP(S) URLs and require the ACS path to remain `/auth/callback/okta/saml` before reporting `configured=true`
- configured provider metadata is safe to expose only when it is not a credential
- secret values, raw tokens, SAML assertions, authorization codes, and refresh tokens are never returned from operator diagnostics

## Okta activation checklists

### Okta SAML activation checklist

Issue #704 should activate Okta SAML only after the runtime can satisfy the same managed-session boundary that GitHub OAuth and Google Workspace OIDC already use. Reviewers should treat the following as the minimum activation checklist for any backend/API PR that flips Okta SAML from diagnostic-only to active:

- `authRuntime.enterpriseSso.provider` stays `okta_saml`, `configured=true`, and `active=false` until every required non-secret setting and store is complete, every preflighted URL is valid, and same-origin route wiring exists.
- Complete Okta SAML settings expose same-origin `loginRoute=/auth/login/okta`, `callbackRoute=/auth/callback/okta/saml`, and `logoutRoute=/auth/logout`; incomplete or invalid settings keep login/callback routes diagnostic placeholders only, omit the logout/session-clear handoff, populate `missingSettings` or `invalidSettings`, and must not advertise an active sign-in handoff.
- Active callbacks mint only Tenet-managed session evidence through the signed `tenet_session` cookie, validate safe in-app `returnUrl` values, and keep logout scoped to clearing that local cookie rather than logging the operator out of Okta.
- SAML `NameID`, email, display-name, and optional group attributes normalize into Tenet identity fields, but raw assertions, metadata documents, private keys, and provider cookies are never persisted in identity/session stores or returned through `GET /v1/operator/context`.
- Tests use local fake metadata/assertions for successful, malformed, expired, and untrusted assertion paths; CI and smoke remain credential-free and must not contact live Okta.
- Trusted front-door headers, repository-backed RBAC policy, role checks, deployment/project/environment grants, and immutable plan/apply authorization remain unchanged; Okta SAML authenticates identity only and never grants Tenet access by itself.

Until a PR satisfies that checklist, Okta SAML should remain configured-but-inactive diagnostic evidence so operators can preflight settings without seeing a clickable sign-in path that cannot complete safely.

### Okta OIDC activation checklist

Okta OIDC is now active only through the same managed-session/OIDC runtime shape used by Google Workspace OIDC, not through a separate session or token lifecycle. Reviewers should treat the following as the ongoing regression checklist for any backend/API PR that touches Okta OIDC runtime behavior:

- `authRuntime.enterpriseSso.provider` stays `okta_oidc`, `configured=true`, and `active=false` until every required non-secret client/issuer/redirect setting and shared state, identity, and session-secret store is complete and the same-origin route wiring exists.
- Complete Okta OIDC settings expose same-origin `loginRoute=/auth/login/okta`, `callbackRoute=/auth/callback/okta/oidc`, and `logoutRoute=/auth/logout`; incomplete settings keep login/callback routes diagnostic placeholders only, omit the logout/session-clear handoff, populate `missingSettings`, and must not advertise an active sign-in handoff.
- Active callbacks mint only Tenet-managed session evidence through the signed `tenet_session` cookie, validate safe in-app `returnUrl` values, and keep logout scoped to clearing that local cookie rather than logging the operator out of Okta or revoking upstream Okta tokens.
- OIDC `sub`, email, display-name, and optional group claims normalize into Tenet identity fields, but raw ID/access/refresh tokens, authorization codes, and provider cookies are never persisted in identity/session stores or returned through `GET /v1/operator/context`.
- Tests use local fake Okta OIDC authorization-code/profile responses for successful, malformed, expired-state, and provider-error paths; CI and smoke remain credential-free and must not contact live Okta.
- Trusted front-door headers, repository-backed RBAC policy, role checks, deployment/project/environment grants, and immutable plan/apply authorization remain unchanged; Okta OIDC authenticates identity only and never grants Tenet access by itself.

This checklist is now the shipped Okta OIDC runtime contract: complete settings activate the same managed-session path as Google Workspace OIDC, while incomplete settings remain explicit inactive diagnostics so operators can preflight configuration without seeing a sign-in path that cannot complete safely.

## Identity normalization

Provider-specific inputs should normalize into Tenet's authenticated identity shape before route handlers make authorization decisions.

Recommended mapping:

| Tenet field | Google OIDC source | Okta SAML/OIDC source |
| --- | --- | --- |
| `provider` | canonical runtime provider `google` (`TENET_AUTH_PROVIDER=google_oidc` is accepted) | `okta_saml` or `okta_oidc` |
| `subject` / stable provider ID | OIDC `sub` | SAML `NameID` or OIDC `sub` |
| `email` | verified email claim when available | email attribute/claim |
| `name` | profile/display-name claim | display-name attribute/claim |
| groups | OIDC/SAML group claims when configured | Okta group attributes/claims |

Group claims are identity input only. They should not directly grant Tenet access until a repository-backed RBAC policy maps those users/groups to Tenet roles and scopes.

## Precedence and authorization safety

Enterprise SSO must not weaken existing fail-safe behavior.

- Trusted front-door identity remains authoritative when a request arrives through a configured trusted front door that forwards `X-Tenet-Authenticated-*` identity headers.
- Browser-supplied identity, role, group, or scope headers are never trusted directly; production front doors must strip spoofable inbound headers before forwarding trusted values.
- Provider login creates authenticated identity/session evidence, not implicit deployment access.
- Route authorization still depends on Tenet's existing role/deployment/project/environment enforcement toggles and, for future RBAC work, an explicit policy evaluator.
- Missing, unknown, or unsupported provider roles/groups fail closed until a Tenet policy maps them to `viewer`, `operator`, or `admin` and concrete scopes.

## Operator diagnostics

`GET /v1/operator/context` should remain the first troubleshooting stop. Enterprise SSO additions should be additive and non-secret, for example:

- configured provider type (`google` for active Google Workspace OIDC, `okta_saml`, `okta_oidc`)
- human-readable provider labels in operator-facing UI (`Google Workspace OIDC`, `Okta SAML`, `Okta OIDC`) so operators do not need to decode raw enum values during 401/403 recovery
- provider-boundary copy in the embedded Operator context details that keeps Google Workspace identity separate from Gmail/API access and keeps Okta SSO identity separate from Tenet role/deployment authorization
- whether managed enterprise SSO is configured and active
- required, missing, and invalid setting names
- normalized identity provider/subject/email/name when an authenticated session exists
- group claim presence/count or safe group names only when those names are already intended to be operator-visible
- explicit precedence notes when trusted front-door headers override a Tenet-managed session

Diagnostics must not expose client secrets, raw ID/access/refresh tokens, SAML assertions, authorization codes, private keys, or provider cookies.

## Testing and smoke expectations

Provider implementation and review should use local fakes:

- fake Google OIDC profile/claims for identity normalization
- fake Okta SAML assertion/metadata or fake Okta OIDC profile for parsing and normalization
- router/session tests for callback, return URL, cookie, and diagnostic behavior
- operator-context tests proving secrets are redacted and missing settings are explicit

CI and local smoke should not require live Google Workspace, Gmail, or Okta credentials. The current smoke guard restarts the direct-process and Compose API with fake local Google OIDC, Okta OIDC, and Okta SAML settings, then reads `GET /v1/operator/context` to prove Google and Okta OIDC report active `enterpriseSso` / `tenetManagedLogin` runtimes with same-origin `loginRoute`, `callbackRoute`, and `logoutRoute`, while Okta SAML stays configured-but-inactive with the stable inactive reason. The smoke guard also calls both `GET /auth/logout` and `POST /auth/logout` against the configured Google path, asserting a safe in-app redirect, Tenet `tenet_session` cookie expiry, and no provider-token/front-door-session revocation claim in the response body. The payloads stay non-secret, name the provider, preserve route placeholders, report required/missing setting lists, and mark shared state/identity/session-secret stores configured without contacting an external IdP. Focused unit/router tests cover Google and Okta OIDC fake-provider code exchange/profile lookup, missing-setting combinations, raw JSON field-name contracts, and the embedded guarded-read handoff preference when enterprise SSO reports both `enterpriseSso.active=true` and `tenetManagedLogin.active=true`: operator copy must use `Sign in with Google Workspace SSO`, not a GitHub-style managed-login label. The PR Docker-smoke scope is intentionally path-filtered rather than provider-credential-driven: changes under provider/session/auth runtime packages such as `internal/authprovider/**`, reserved Okta SAML/enterprise runtime paths `internal/authsaml/**` and `internal/authsso/**`, `internal/authidentity/**`, `internal/authstate/**`, and request-context/API wiring trigger `make smoke-docker`, while unrelated docs-only edits stay on the cheaper fast lane.

## Explicit non-goals for issue #662

This issue is not the RBAC policy redesign. Keep these concerns separate:

- no repository-backed RBAC policy parser/evaluator in the enterprise SSO provider slice
- no live Google or Okta credential dependency in CI
- no Gmail mailbox, Drive, or Calendar integration
- no automatic access grant from raw provider group names
- no removal of trusted-front-door header support

Repository-backed policy mapping for users/groups, Tenet roles, and deployment/project/environment scopes belongs to issue #663 and follow-up RBAC work.
