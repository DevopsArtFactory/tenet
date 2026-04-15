package manifestfs

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/DevopsArtFactory/tenet/internal/manifest"
	"github.com/DevopsArtFactory/tenet/internal/spec"
	"gopkg.in/yaml.v3"
)

// Load reads a repository-like directory tree into a CatalogSpec.
func Load(root string) (manifest.CatalogSpec, error) {
	if err := ensureRootExists(root); err != nil {
		return manifest.CatalogSpec{}, err
	}
	if err := ensureRequiredDirs(root); err != nil {
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
		if entry.IsDir() || !isManifestFile(entry.Name()) {
			continue
		}

		content, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, err
		}

		var decoded T
		if err := decodeSpec(entry.Name(), content, &decoded); err != nil {
			return nil, fmt.Errorf("decode %s: %w", entry.Name(), err)
		}
		loaded = append(loaded, decoded)
	}

	return loaded, nil
}

func decodeSpec(name string, content []byte, out any) error {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".json":
		return json.Unmarshal(content, out)
	case ".yaml", ".yml":
		return yaml.Unmarshal(content, out)
	default:
		return fmt.Errorf("unsupported manifest extension %q", filepath.Ext(name))
	}
}

func isManifestFile(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".json", ".yaml", ".yml":
		return true
	default:
		return false
	}
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

func ensureRequiredDirs(root string) error {
	required := []string{"accounts", "environments", "deployments"}
	for _, name := range required {
		path := filepath.Join(root, name)
		info, err := os.Stat(path)
		if err != nil {
			return fmt.Errorf("stat required manifest dir %q: %w", name, err)
		}
		if !info.IsDir() {
			return fmt.Errorf("required manifest path %q is not a directory", path)
		}
	}
	return nil
}
