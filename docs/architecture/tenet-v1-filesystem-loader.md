# Tenet v1 Filesystem Loader

The first GitOps ingestion slice reads JSON or YAML manifests from a repository-shaped directory tree:

- `accounts/*.{json,yaml,yml}`
- `environments/*.{json,yaml,yml}`
- `deployments/*.{json,yaml,yml}`

Deployment manifests can now be authored in a goployer-compatible YAML shape for EC2 stacks, while Tenet-specific artifact intent is carried with:
- `binary` for EC2 deployments
- `image` for ECS deployments

The loader still keeps parsing centralized and parser-light so downstream compiler, controller, and API packages do not need to care whether the source file was JSON or YAML.
