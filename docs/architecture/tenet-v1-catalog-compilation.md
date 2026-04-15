# Tenet v1 Catalog Compilation

The control plane needs an intermediate representation between raw Git-managed resource specs and runtime reconciliation objects.

`internal/manifest` is that bridge:
- validates each spec object with `internal/spec`
- validates cross-resource references
- converts parser-facing spec structs into runtime-facing domain structs

This keeps future YAML/Git ingestion separate from controller logic.
