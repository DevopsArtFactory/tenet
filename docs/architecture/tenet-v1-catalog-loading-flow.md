# Tenet v1 Catalog Loading Flow

The validated catalog load path is now:

1. `internal/manifestfs` discovers raw manifest files in a repo-shaped tree.
2. `internal/manifest` validates references and compiles runtime domain objects.
3. `internal/catalog` offers a single entry point for future controllers and APIs.

This keeps repository I/O, semantic compilation, and runtime consumption separated while still giving the rest of the codebase one simple load function.
