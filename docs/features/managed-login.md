# Managed Login

Operator troubleshooting guide for Tenet-managed browser login.

Managed login is active only when `GET /v1/operator/context` reports `authRuntime.tenetManagedLogin.active=true`. If it is `false`, inspect the same diagnostic object before changing routes: `requiredSettings`, `missingSettings`, `invalidSettings`, `loginRoute`, `callbackRoute`, `logoutRoute`, state-store, identity-store, and session-secret fields are non-secret activation evidence.

## Provider modes

Use concrete provider modes in runbooks and diagnostics:

- `TENET_AUTH_PROVIDER=github` activates GitHub OAuth when the GitHub client, redirect, state-store, identity-store, and session-secret settings are complete.
- `TENET_AUTH_PROVIDER=google_oidc` activates Google Workspace OIDC for identity login only. It does not grant Gmail, Drive, Calendar, or other Google API access.
- `TENET_AUTH_PROVIDER=okta_oidc` activates Okta OIDC through the same managed-session runtime shape as Google OIDC.
- `TENET_AUTH_PROVIDER=okta_saml` is diagnostic-only. It can report configured/missing/invalid SAML preflight evidence and same-origin route wiring, but it does not activate the managed-login runtime.

Never document `TENET_AUTH_PROVIDER` with a secret-shaped placeholder. The provider value is non-secret; use the exact mode being diagnosed. Copy these mode values exactly in operator runbooks and issue evidence; redaction should stop at credential-bearing settings.
Credential-bearing settings such as client secrets and session secrets may stay redacted in runbooks, logs, and snippets; the provider mode itself must stay concrete.

## Routes

Login routes are same-origin entry points:

- `GET /auth/login/github`
- `GET /auth/login/google`
- `GET /auth/login/okta`

Active callback routes consume one-time state and mint only Tenet session evidence:

- `GET /auth/callback/github`
- `GET /auth/callback/google`
- `GET /auth/callback/okta/oidc`

The Okta SAML route `GET /auth/callback/okta/saml` is diagnostic route wiring until SAML activation ships.

Logout routes are same-origin session-clear handoffs:

- `GET /auth/logout` and `POST /auth/logout`

The legacy `/api/v1/auth/*` family is not the managed-login route contract.

## Cookie secure diagnostics

Managed login issues a signed `tenet_session` cookie with `HttpOnly`, `SameSite=Lax`, and path `/`. The cookie is marked `Secure` when Tenet has HTTPS request evidence or trusted `X-Forwarded-Proto: https` / `Forwarded: proto=https` evidence, or when operators set `TENET_AUTH_SESSION_COOKIE_SECURE=true`.

Use `GET /v1/operator/context` and inspect `authRuntime.tenetManagedLogin` to diagnose cookie behavior. The fields `sessionCookieSecureOverride`, `sessionCookieSecureForRequest`, and `sessionCookieSecureSources` tell you whether the final cookie decision came from an explicit override, direct HTTPS, or trusted forwarded-proto evidence. Local HTTP development can legitimately report a non-secure cookie when the override and HTTPS evidence are absent.

## Logout boundary

Logout clears only Tenet's signed `tenet_session` cookie with matching path, secure, HttpOnly, and SameSite attributes, then redirects to a validated in-app `returnUrl`. It does not revoke provider tokens, clear GitHub/Google/Okta sessions, or sign out an upstream trusted front door. Provider access tokens are used only for callback profile lookup and must not be persisted in identity/session stores or returned through diagnostics.

If an operator remains signed in at the IdP or the external front door after `/auth/logout`, that is expected. Clear those upstream sessions in the provider or front-door system being used.

## Trusted front door and policy

Authentication and RBAC enforcement flags, including live ECS execution, do not enable header trust. Keep `TENET_API_TRUST_AUTHENTICATED_HEADERS=false` for managed-login-only installations; request-supplied identity, role, group, and scope headers are ignored. Upgrading proxy-based installations that previously relied on enforcement flags implicitly enabling header trust requires setting this flag explicitly.

Trusted front-door identity wins over the session cookie only when header trust is explicitly enabled. Enable `TENET_API_TRUST_AUTHENTICATED_HEADERS=true` only on API instances reachable exclusively through a front door that authenticates callers and strips spoofable inbound `X-Tenet-Authenticated-*`, `X-Tenet-Authorized-*`, and trusted-alias `X-User-*` headers before forwarding trusted values.

Managed login authenticates the browser operator; it does not grant Tenet roles or deployment access by itself. Enable `TENET_API_AUTHORIZE_WITH_REPOSITORY_RBAC_POLICY=true` together with authenticated-identity, operator-role and deployment-authorization enforcement to apply the versioned `rbac/policy.yaml`. That policy maps authenticated identities or groups to roles and deployment/project/environment scopes. A valid login without a matching grant is denied; managed-login-only deployments do not need to trust forwarded identity headers.

## Verification

Use focused Go tests for route/session/env wiring and provider helpers:

- `go test ./internal/api ./cmd/tenet-api`
- `go test ./internal/authprovider ./internal/authidentity ./internal/authstate`

Local and Docker smoke intentionally cover the out-of-box disabled managed-login runtime unless OAuth/OIDC settings are supplied. Local fake-provider router tests and the embedded UI harness cover active GitHub, Google Workspace, and Okta OIDC sign-in/sign-out flows without live GitHub, Google, or Okta credentials. The fake logout path must keep `/auth/logout?returnUrl=...` in the same shell, clear only Tenet's signed `tenet_session` cookie, and avoid claiming provider-token revocation or upstream front-door sign-out.

### Browser-bound login transactions

Login sets a short-lived, host-only, HttpOnly, SameSite=Lax cookie signed with the session secret. The callback must present that cookie before the server consumes OAuth state or exchanges a code. A callback opened in a different browser, with missing cookies, or after a newer login replaced the cookie is rejected; start login again in the intended browser. Successful login clears the binding cookie. HTTPS deployments must enable secure cookies. Pending login transactions must be restarted after upgrading.

### Verified OIDC email and session upgrade

Google and Okta email-based RBAC grants require a boolean `email_verified: true`
from the provider's userinfo endpoint. Missing, null or false verification removes
the email from the persisted identity and session. Login still succeeds with the
stable provider subject, so explicit `google:<sub>` or `okta_oidc:<sub>` policy
grants continue to work. A preferred username is display data, not a verified
email grant.

Upgrading to verified-email enforcement invalidates all previously issued Tenet
session cookies, including GitHub sessions. Users must sign in again; the new
versioned session format prevents an old cookie from retaining an unverified
email grant until expiry. Upgrade all API replicas together: old binaries still
accept the old session format. No manual identity-store edits are required;
successful login replaces the stored profile, including removal of stale email.

### Login backpressure and state retention

The file-backed state store retains at most 1,024 unexpired login records and
accepts at most 120 new login records per rolling minute. Both limits apply to
all providers and API processes using the same shared store path. A successful
callback consumes its record without resetting the initiation quota. Consumed
records remain until their original expiry (normally 10 minutes), and the next
login prunes expired records under the same process-safe lock. Existing pending
logins are not evicted to admit new ones. Pre-upgrade files over the limit stop
accepting new records until expiry makes room; expired records are not an audit
log and should not be used for long-term security evidence.

An overloaded login endpoint returns HTTP 429 with `Retry-After: 60`, without
issuing an OAuth redirect or binding cookie. Retry after that interval; capacity
may remain exhausted until pending records expire. Login `returnUrl` is limited
to 2,048 bytes to bound each stored callback path. This shared-store backpressure
bounds growth, but does not replace ingress rate limiting against sustained
request floods. Independent state paths have independent limits.
