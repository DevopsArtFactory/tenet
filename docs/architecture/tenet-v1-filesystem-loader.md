# Tenet v1 Filesystem Loader

The first GitOps ingestion slice reads JSON or YAML manifests from a repository-shaped directory tree:

- `accounts/*.{json,yaml,yml}`
- `environments/*.{json,yaml,yml}`
- `deployments/*.{json,yaml,yml}`

Deployment manifests can now be authored in a goployer-compatible YAML shape for EC2 stacks, while Tenet-specific artifact intent is carried with:
- `binary` for EC2 deployments
- `image` for ECS deployments

The loader still keeps parsing centralized and parser-light so downstream compiler, controller, and API packages do not need to care whether the source file was JSON or YAML.

The same manifest codec now also backs GitHub ingestion primitives:

- `internal/githubcontents` lists one repo/ref/path directory and fetches one repo/ref/path file with optional token auth plus explicit operator-facing list/fetch/decode errors
- `internal/manifestgithub` mirrors the filesystem loader for `accounts`, `environments`, and `deployments`
- `internal/catalog.LoadFromGitHub(...)` validates and compiles that GitHub-backed manifest tree into the same runtime catalog shape

`tenet-api` now wires that additive path behind explicit source selection:

- `TENET_CATALOG_SOURCE=filesystem` keeps the existing `TENET_REPO_ROOT` contract unchanged
- `TENET_CATALOG_SOURCE=github` adds a read-only API mode backed by `TENET_CATALOG_GITHUB_REPOSITORY` plus optional `TENET_CATALOG_GITHUB_REF`, `TENET_CATALOG_GITHUB_PATH`, `TENET_CATALOG_GITHUB_TOKEN`, and `TENET_CATALOG_GITHUB_BASE_URL`

`tenet-controller` and controller-owned runtime writes still remain out of scope for GitHub mode; they continue to require the filesystem repository contract.
