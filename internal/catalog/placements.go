package catalog

import (
	"fmt"
	"sort"

	"github.com/DevopsArtFactory/tenet/internal/manifest"
	"github.com/DevopsArtFactory/tenet/internal/placement"
)

// PlacementView is a UI-friendly placement listing entry per deployment.
type PlacementView struct {
	DeploymentName string `json:"deploymentName"`
	AppRef         string `json:"appRef"`
	EnvironmentRef string `json:"environmentRef"`
	AccountName    string `json:"accountName"`
	AWSAccountID   string `json:"awsAccountId"`
	Region         string `json:"region"`
	NetworkRef     string `json:"networkRef,omitempty"`
}

// ListPlacements projects a compiled catalog into a stable deployment placement listing.
func ListPlacements(c manifest.Catalog) ([]PlacementView, error) {
	resolver := placement.Resolver{}
	placements := make([]PlacementView, 0, len(c.Deployments))
	for _, deployment := range c.Deployments {
		resolved, err := resolver.Resolve(c.Accounts, c.Environments, deployment)
		if err != nil {
			return nil, fmt.Errorf("resolve placement for deployment %q: %w", deployment.Name, err)
		}
		placements = append(placements, PlacementView{
			DeploymentName: deployment.Name,
			AppRef:         deployment.AppRef,
			EnvironmentRef: deployment.EnvironmentRef,
			AccountName:    resolved.AccountName,
			AWSAccountID:   resolved.AccountID,
			Region:         resolved.Region,
			NetworkRef:     resolved.NetworkRef,
		})
	}
	sort.Slice(placements, func(i, j int) bool {
		return placements[i].DeploymentName < placements[j].DeploymentName
	})
	return placements, nil
}
