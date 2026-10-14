package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"sync"
	"time"

	"github.com/DevopsArtFactory/tenet/internal/domain"
	"github.com/DevopsArtFactory/tenet/internal/githubcontents"
	"github.com/DevopsArtFactory/tenet/internal/manifest"
	"github.com/DevopsArtFactory/tenet/internal/manifestgithub"
	"github.com/DevopsArtFactory/tenet/internal/rbacpolicy"
)

type pinnedCatalogSnapshot struct {
	Spec    manifest.CatalogSpec
	Catalog manifest.Catalog
	Policy  rbacpolicy.Policy
}

func newPinnedGitHubCatalogLoader(client githubcontents.Client, request githubcontents.Request) func(context.Context) (pinnedCatalogSnapshot, error) {
	var mutex sync.Mutex
	var cachedRevision string
	var cachedJSON []byte
	return func(ctx context.Context) (pinnedCatalogSnapshot, error) {
		ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		mutex.Lock()
		defer mutex.Unlock()
		if err := ctx.Err(); err != nil {
			return pinnedCatalogSnapshot{}, err
		}
		// Resolve the current ref on every gate. Only immutable commit content is cached;
		// revoked repository policy must not be hidden behind a branch TTL cache.
		revision, err := client.ResolveCommit(ctx, request)
		if err != nil {
			return pinnedCatalogSnapshot{}, err
		}
		if revision != cachedRevision {
			pinned := request
			pinned.Ref = revision
			spec, err := manifestgithub.Load(ctx, client, pinned)
			if err != nil {
				return pinnedCatalogSnapshot{}, err
			}
			spec.Git = &domain.GitSource{Repository: request.Repository, Ref: request.Ref, Path: request.Path, Revision: revision}
			compiled, err := manifest.Compile(spec)
			if err != nil {
				return pinnedCatalogSnapshot{}, err
			}
			policy, err := loadPinnedGitHubPolicy(ctx, client, pinned)
			if err != nil {
				return pinnedCatalogSnapshot{}, err
			}
			content, err := json.Marshal(pinnedCatalogSnapshot{Spec: spec, Catalog: compiled, Policy: policy})
			if err != nil {
				return pinnedCatalogSnapshot{}, err
			}
			cachedRevision, cachedJSON = revision, content
		}
		// Each caller owns its projections; mutable pointers/slices cannot poison the cache.
		var snapshot pinnedCatalogSnapshot
		if err := json.Unmarshal(cachedJSON, &snapshot); err != nil {
			return pinnedCatalogSnapshot{}, fmt.Errorf("decode immutable GitHub catalog cache: %w", err)
		}
		return snapshot, nil
	}
}

func loadPinnedGitHubPolicy(ctx context.Context, client githubcontents.Client, request githubcontents.Request) (rbacpolicy.Policy, error) {
	root := request.Path
	for _, name := range []string{"policy.json", "policy.yaml", "policy.yml"} {
		request.Path = path.Join(root, "rbac", name)
		file, err := client.Fetch(ctx, request)
		if err != nil {
			var missing *githubcontents.Error
			if errors.As(err, &missing) && missing.Code == githubcontents.ErrorCodeNotFound {
				continue
			}
			return rbacpolicy.Policy{}, err
		}
		var policy rbacpolicy.Policy
		if err := file.Decode(&policy); err != nil {
			return rbacpolicy.Policy{}, err
		}
		policy = rbacpolicy.Normalize(policy)
		if err := rbacpolicy.Validate(policy); err != nil {
			return rbacpolicy.Policy{}, err
		}
		return policy, nil
	}
	return rbacpolicy.Policy{}, nil
}
