# Tenet v1 Repository-Backed Summary Provider

The API can now be wired to a repository root through a small provider function.

That keeps the transport layer simple:
- `cmd/tenet-api` decides whether a repo-backed provider is available
- `internal/catalog` owns repository loading and summarization
- `internal/api` only knows how to call an injected function
