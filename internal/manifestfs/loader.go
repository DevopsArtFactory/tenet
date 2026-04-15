package manifestfs

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/DevopsArtFactory/tenet/internal/manifest"
	"github.com/DevopsArtFactory/tenet/internal/spec"
)

// Load reads a repository-like directory tree into a CatalogSpec.
func Load(root string) (manifest.CatalogSpec, error) {
	if err := ensureRootExists(root); err != nil {
		return manifest.CatalogSpec{}, err
	}

	accounts, err := loadSpecs[spec.AccountSpec](filepath.Join(root, "accounts"))
	if err != nil {
		return manifest.CatalogSpec{}, fmt.Errorf("load accounts: %w", err)
	}
	environments, err := loadSpecs[spec.EnvironmentSpec](filepath.Join(root, "environments"))
	if err != nil {
		return manifest.CatalogSpec{}, fmt.Errorf("load environments: %w", err)
	}
	deployments, err := loadSpecs[spec.DeploymentSpec](filepath.Join(root, "deployments"))
	if err != nil {
		return manifest.CatalogSpec{}, fmt.Errorf("load deployments: %w", err)
	}

	return manifest.CatalogSpec{
		Accounts:     accounts,
		Environments: environments,
		Deployments:  deployments,
	}, nil
}

func loadSpecs[T any](dir string) ([]T, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if isMissingDir(err) {
			return nil, nil
		}
		return nil, err
	}

	loaded := make([]T, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		content, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, err
		}

		var decoded T
		if err := json.Unmarshal(content, &decoded); err != nil {
			return nil, fmt.Errorf("decode %s: %w", entry.Name(), err)
		}
		loaded = append(loaded, decoded)
	}

	return loaded, nil
}

func isMissingDir(err error) bool {
	return err != nil && os.IsNotExist(err)
}

func ensureRootExists(root string) error {
	info, err := os.Stat(root)
	if err != nil {
		return fmt.Errorf("stat manifest root: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("manifest root %q is not a directory", root)
	}
	return nil
}
