package catalog

import (
	"sort"

	"github.com/DevopsArtFactory/tenet/internal/manifest"
)

// EnvironmentView is a UI-friendly environment listing entry.
type EnvironmentView struct {
	Name       string `json:"name"`
	AccountRef string `json:"accountRef"`
	Region     string `json:"region"`
	NetworkRef string `json:"networkRef,omitempty"`
}

// ListEnvironments projects a compiled catalog into a stable environment listing.
func ListEnvironments(c manifest.Catalog) []EnvironmentView {
	environments := make([]EnvironmentView, 0, len(c.Environments))
	for _, environment := range c.Environments {
		environments = append(environments, EnvironmentView{
			Name:       environment.Name,
			AccountRef: environment.AccountRef,
			Region:     environment.Region,
			NetworkRef: environment.NetworkRef,
		})
	}
	sort.Slice(environments, func(i, j int) bool {
		return environments[i].Name < environments[j].Name
	})
	return environments
}
