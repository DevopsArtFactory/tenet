# Tenet v1 Filesystem Loader

The first GitOps ingestion slice reads JSON manifests from a repository-shaped directory tree:

- `accounts/*.json`
- `environments/*.json`
- `deployments/*.json`

This is intentionally parser-light. It lets Tenet exercise file discovery and manifest bundling now, while keeping room to swap in YAML parsing later without changing downstream compiler or controller packages.
