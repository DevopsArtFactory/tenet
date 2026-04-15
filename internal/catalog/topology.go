package catalog

import "github.com/DevopsArtFactory/tenet/internal/manifest"

// TopologySnapshot is a UI-friendly read model for the current desired catalog.
type TopologySnapshot struct {
	Summary      Summary           `json:"summary"`
	Accounts     []AccountView     `json:"accounts"`
	Environments []EnvironmentView `json:"environments"`
	Deployments  []DeploymentView  `json:"deployments"`
	Placements   []PlacementView   `json:"placements"`
}

// BuildTopology projects a compiled catalog into a stable topology snapshot.
func BuildTopology(c manifest.Catalog) (TopologySnapshot, error) {
	placements, err := ListPlacements(c)
	if err != nil {
		return TopologySnapshot{}, err
	}

	return TopologySnapshot{
		Summary:      Summarize(c),
		Accounts:     ListAccounts(c),
		Environments: ListEnvironments(c),
		Deployments:  ListDeployments(c),
		Placements:   placements,
	}, nil
}
