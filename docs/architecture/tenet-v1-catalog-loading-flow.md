# Tenet v1 Catalog Loading Flow

The validated catalog load path is now:

1. `internal/manifestfs` discovers raw manifest files in a repo-shaped tree.
2. `internal/manifestgithub` can discover the same manifest shape from a single GitHub repo/ref/path through `internal/githubcontents`.
3. `internal/manifest` validates references and compiles runtime domain objects.
4. `internal/catalog` offers simple entry points for future controllers and APIs whether the source is a local repository tree or GitHub.

This keeps repository I/O, semantic compilation, and runtime consumption separated while still giving the rest of the codebase one small catalog-loading surface.
