# Contributing Guide

Thanks for your interest in contributing to Tenet.

## Before You Start
- Read the [Code of Conduct](./CODE_OF_CONDUCT.md).
- Search existing issues before opening a new one.
- For substantial changes, open an issue first to discuss the proposal.

## Development Flow
1. Fork the repository.
2. Create a feature branch.
3. Make focused commits with clear messages.
4. Add or update tests for your changes.
5. Open a pull request.

## Local Tests

Run `go test ./...` (or `make test`) and `make smoke-local` without changing your shell's AWS settings. AWS-aware Go suites use `internal/testaws.Run` in `TestMain`; local smoke sources `scripts/test-aws-env.sh`. Both discard inherited `AWS_*` variables, replace shared config and credentials files with an empty device, and disable EC2 metadata credential lookup. They affect only test processes and their children.

When adding an AWS-aware test package, use the same `TestMain` setup. Provider-specific tests must supply mock clients or use `t.Setenv` with local fixture credentials/endpoints after suite initialization. The `awsemulator` tests use credentials and the loopback endpoint explicitly loaded from `TENET_EMULATOR_STATE`; no personal AWS profile is required. Production commands retain their normal AWS configuration behavior.

## Pull Request Checklist
- The PR is scoped to one clear change.
- Code and docs are updated together when needed.
- Tests were added/updated and pass locally.
- Backward-incompatible changes are clearly documented.

## Commit Messages
Use concise, descriptive commit messages.
Example:
- `feat: add rolling deployment option`
- `fix: handle empty target instance list`

## Licensing
By submitting a contribution, you agree that your contributions are licensed under the same license as this project (Apache License 2.0).
