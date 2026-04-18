package manifestgithub

import (
	"context"
	"fmt"
	pathpkg "path"
	"sort"
	"strings"

	"github.com/DevopsArtFactory/tenet/internal/githubcontents"
	"github.com/DevopsArtFactory/tenet/internal/manifest"
	"github.com/DevopsArtFactory/tenet/internal/manifestcodec"
	"github.com/DevopsArtFactory/tenet/internal/spec"
)

// Client lists and fetches GitHub manifest content.
type Client interface {
	List(context.Context, githubcontents.Request) ([]githubcontents.Entry, error)
	Fetch(context.Context, githubcontents.Request) (githubcontents.File, error)
}

// Load reads a repository/ref/path manifest tree from GitHub into a CatalogSpec.
func Load(ctx context.Context, client Client, req githubcontents.Request) (manifest.CatalogSpec, error) {
	if client == nil {
		return manifest.CatalogSpec{}, fmt.Errorf("GitHub manifest client is required")
	}

	root, err := normalizeRoot(req.Path)
	if err != nil {
		return manifest.CatalogSpec{}, fmt.Errorf("invalid GitHub manifest root: %w", err)
	}

	accounts, err := loadSpecs[spec.AccountSpec](ctx, client, withPath(req, joinRoot(root, "accounts")))
	if err != nil {
		return manifest.CatalogSpec{}, fmt.Errorf("load accounts: %w", err)
	}
	environments, err := loadSpecs[spec.EnvironmentSpec](ctx, client, withPath(req, joinRoot(root, "environments")))
	if err != nil {
		return manifest.CatalogSpec{}, fmt.Errorf("load environments: %w", err)
	}
	deployments, err := loadSpecs[spec.DeploymentSpec](ctx, client, withPath(req, joinRoot(root, "deployments")))
	if err != nil {
		return manifest.CatalogSpec{}, fmt.Errorf("load deployments: %w", err)
	}

	return manifest.CatalogSpec{
		Accounts:     accounts,
		Environments: environments,
		Deployments:  deployments,
	}, nil
}

func loadSpecs[T any](ctx context.Context, client Client, req githubcontents.Request) ([]T, error) {
	entries, err := client.List(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("list GitHub manifests in %q: %w", req.Path, err)
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Path < entries[j].Path
	})

	loaded := make([]T, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !manifestcodec.IsManifestFile(entry.Name) {
			continue
		}

		file, err := client.Fetch(ctx, withPath(req, entry.Path))
		if err != nil {
			return nil, fmt.Errorf("fetch GitHub manifest %q: %w", entry.Path, err)
		}

		var decoded T
		if err := file.Decode(&decoded); err != nil {
			return nil, fmt.Errorf("decode GitHub manifest %q: %w", file.Path, err)
		}
		loaded = append(loaded, decoded)
	}

	return loaded, nil
}

func withPath(req githubcontents.Request, path string) githubcontents.Request {
	req.Path = path
	return req
}

func joinRoot(root string, name string) string {
	if root == "" {
		return name
	}
	return pathpkg.Join(root, name)
}

func normalizeRoot(raw string) (string, error) {
	raw = strings.TrimSpace(strings.ReplaceAll(raw, "\\", "/"))
	if raw == "" {
		return "", nil
	}
	if strings.HasPrefix(raw, "/") {
		return "", fmt.Errorf("path must be repository-relative")
	}

	segments := strings.Split(raw, "/")
	for _, segment := range segments {
		switch segment {
		case "", ".", "..":
			return "", fmt.Errorf("path must not contain empty, current-directory, or parent-directory segments")
		}
	}
	return strings.Join(segments, "/"), nil
}
