package catalog

import (
	"fmt"

	"github.com/DevopsArtFactory/tenet/internal/manifest"
	"github.com/DevopsArtFactory/tenet/internal/manifestfs"
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
