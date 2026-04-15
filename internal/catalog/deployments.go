package catalog

import (
	"sort"

	"github.com/DevopsArtFactory/tenet/internal/manifest"
)

// DeploymentView is a UI-friendly deployment listing entry.
type DeploymentView struct {
	Name           string `json:"name"`
	AppRef         string `json:"appRef"`
	EnvironmentRef string `json:"environmentRef"`
}

// ListDeployments projects a compiled catalog into a stable deployment listing.
func ListDeployments(c manifest.Catalog) []DeploymentView {
	deployments := make([]DeploymentView, 0, len(c.Deployments))
	for _, deployment := range c.Deployments {
		deployments = append(deployments, DeploymentView{
			Name:           deployment.Name,
			AppRef:         deployment.AppRef,
			EnvironmentRef: deployment.EnvironmentRef,
		})
	}
	sort.Slice(deployments, func(i, j int) bool {
		return deployments[i].Name < deployments[j].Name
	})
	return deployments
}
