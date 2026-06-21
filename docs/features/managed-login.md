# Managed Login

Operator troubleshooting guide for Tenet-managed browser login.

Managed login is active only when `GET /v1/operator/context.authRuntime.tenetManagedLogin.active=true`. If it is `false`, inspect the same diagnostic object before changing routes: `requiredSettings`, `missingSettings`, `invalidSettings`, `loginRoute`, `callbackRoute`, `logoutRoute`, state-store, identity-store, and session-secret fields are non-secret activation evidence.

## Provider modes

Use concrete provider modes in runbooks and diagnostics:

- `TENET_AUTH_PROVIDER=github` activates GitHub OAuth when the GitHub client, redirect, state-store, identity-store, and session-secret settings are complete.
- `TENET_AUTH_PROVIDER=google_oidc` activates Google Workspace OIDC for identity login only. It does not grant Gmail, Drive, Calendar, or other Google API access.
- `TENET_AUTH_PROVIDER=okta_oidc` activates Okta OIDC through the same managed-session runtime shape as Google OIDC.
- `TENET_AUTH_PROVIDER=okta_saml` is diagnostic-only. It can report configured/missing/invalid SAML preflight evidence and same-origin route wiring, but it does not activate the managed-login runtime.

Never document `TENET_AUTH_PROVIDER` with a secret-shaped placeholder. The provider value is non-secret; use the exact mode being diagnosed.

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

Use `GET /v1/operator/context.authRuntime.tenetManagedLogin` to diagnose cookie behavior. The fields `sessionCookieSecureOverride`, `sessionCookieSecureForRequest`, and `sessionCookieSecureSources` tell you whether the final cookie decision came from an explicit override, direct HTTPS, or trusted forwarded-proto evidence. Local HTTP development can legitimately report a non-secure cookie when the override and HTTPS evidence are absent.

## Logout boundary

Logout clears only Tenet's signed `tenet_session` cookie with matching path, secure, HttpOnly, and SameSite attributes, then redirects to a validated in-app `returnUrl`. It does not revoke provider tokens, clear GitHub/Google/Okta sessions, or sign out an upstream trusted front door. Provider access tokens are used only for callback profile lookup and must not be persisted in identity/session stores or returned through diagnostics.

If an operator remains signed in at the IdP or the external front door after `/auth/logout`, that is expected. Clear those upstream sessions in the provider or front-door system being used.

## Trusted front door and policy

Trusted front-door identity still wins when trusted headers are present. Enable `TENET_API_TRUST_AUTHENTICATED_HEADERS=true` and the enforcement toggles only on API instances reachable exclusively through a front door that authenticates callers and strips spoofable inbound `X-Tenet-Authenticated-*`, `X-Tenet-Authorized-*`, and trusted-alias `X-User-*` headers before forwarding trusted values.

Managed login authenticates the browser operator; it does not grant Tenet roles or deployment access by itself. Repository-backed RBAC policy diagnostics can explain how users or groups map to Tenet roles and deployment/project/environment scopes, but route enforcement still depends on Tenet's trusted role and deployment/project/environment authorization settings. Treat policy diagnostics as evidence for front-door/RBAC wiring until policy-derived route enforcement is explicitly enabled in a future change.
