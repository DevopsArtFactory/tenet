# Tenet v1 Catalog Loading Flow

The validated catalog load path is now:

1. `internal/manifestfs` discovers raw manifest files in a repo-shaped tree.
2. `internal/manifestgithub` can discover the same manifest shape from a single GitHub repo/ref/path through `internal/githubcontents`.
3. `internal/manifest` validates references and compiles runtime domain objects.
4. `internal/catalog` offers simple entry points for future controllers and APIs whether the source is a local repository tree or GitHub.

This keeps repository I/O, semantic compilation, and runtime consumption separated while still giving the rest of the codebase one small catalog-loading surface.

`cmd/tenet-api` now selects that surface explicitly at startup:

- `TENET_CATALOG_SOURCE=filesystem` with `TENET_REPO_ROOT=/workspace/repo`
- `TENET_CATALOG_SOURCE=github` with `TENET_CATALOG_GITHUB_REPOSITORY=owner/repo`
- optional GitHub source refinements: `TENET_CATALOG_GITHUB_REF`, `TENET_CATALOG_GITHUB_PATH`, `TENET_CATALOG_GITHUB_TOKEN`, `TENET_CATALOG_GITHUB_BASE_URL`

Startup fails fast when the source selection is ambiguous or incomplete, and it validates the selected catalog before serving requests. The `/v1/catalog/*` and `/v1/overview/*` route contracts stay unchanged across filesystem and GitHub sources because both paths compile into the same runtime catalog. `POST /api/v1/github/webhook` now also consumes that same source selection as read-only context for `desiredStateSourceMatch`, so operators can see whether a signed push is relevant to the active filesystem or GitHub desired-state source without triggering reconcile side effects.
