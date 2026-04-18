# Tenet v1 Controller Repository Bootstrap

`tenet-controller` now understands the same source-selected desired catalog used by the API.

When a desired catalog source is configured, controller startup:
- loads and validates the repository catalog
- summarizes account, environment, and deployment counts
- fails fast if the selected catalog source is invalid or incomplete
- stays alive as a signal-aware long-running process after startup instead of exiting immediately

This keeps controller bootstrap aligned with the read-only API surface while the actual reconciliation loop is still intentionally minimal. `TENET_REPO_ROOT` remains the filesystem-mode input, and `TENET_CATALOG_SOURCE=github` now lets the controller read the same GitHub-backed desired state without pulling apply-specific filesystem constraints into the controller.
