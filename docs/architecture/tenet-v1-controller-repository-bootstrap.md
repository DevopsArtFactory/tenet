# Tenet v1 Controller Repository Bootstrap

`tenet-controller` now understands the same repository-backed desired catalog used by the API.

When `TENET_REPO_ROOT` is set, controller startup:
- loads and validates the repository catalog
- summarizes account, environment, and deployment counts
- fails fast if the repository root is invalid or incomplete

This keeps controller bootstrap aligned with the read-only API surface while the actual reconciliation loop is still intentionally minimal.
