package catalog

import (
	"context"
	"fmt"

	"github.com/DevopsArtFactory/tenet/internal/githubcontents"
	"github.com/DevopsArtFactory/tenet/internal/manifest"
	"github.com/DevopsArtFactory/tenet/internal/manifestfs"
	"github.com/DevopsArtFactory/tenet/internal/manifestgithub"
)

// LoadFromRepository loads, validates, and compiles a repository catalog.
func LoadFromRepository(root string) (manifest.Catalog, error) {
	raw, err := manifestfs.Load(root)
	if err != nil {
		return manifest.Catalog{}, fmt.Errorf("load repository manifests: %w", err)
	}

	compiled, err := manifest.Compile(raw)
	if err != nil {
		return manifest.Catalog{}, fmt.Errorf("compile repository manifests: %w", err)
	}

	return compiled, nil
}

// LoadFromGitHub loads, validates, and compiles a GitHub-backed repository catalog.
func LoadFromGitHub(ctx context.Context, client manifestgithub.Client, req githubcontents.Request) (manifest.Catalog, error) {
	raw, err := manifestgithub.Load(ctx, client, req)
	if err != nil {
		return manifest.Catalog{}, fmt.Errorf("load GitHub manifests: %w", err)
	}

	compiled, err := manifest.Compile(raw)
	if err != nil {
		return manifest.Catalog{}, fmt.Errorf("compile GitHub manifests: %w", err)
	}

	return compiled, nil
}
