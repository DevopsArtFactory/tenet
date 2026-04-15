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
	TargetType     string `json:"targetType,omitempty"`
	Strategy       string `json:"strategy,omitempty"`
	ExecutionMode  string `json:"executionMode,omitempty"`
	Binary         string `json:"binary,omitempty"`
	Image          string `json:"image,omitempty"`
}

// ListDeployments projects a compiled catalog into a stable deployment listing.
func ListDeployments(c manifest.Catalog) []DeploymentView {
	deployments := make([]DeploymentView, 0, len(c.Deployments))
	for _, deployment := range c.Deployments {
		deployments = append(deployments, DeploymentView{
			Name:           deployment.Name,
			AppRef:         deployment.AppRef,
			EnvironmentRef: deployment.EnvironmentRef,
			TargetType:     deployment.TargetType,
			Strategy:       deployment.Strategy,
			ExecutionMode:  deployment.ExecutionMode,
			Binary:         deployment.DesiredArtifact.Binary,
			Image:          deployment.DesiredArtifact.Image,
		})
	}
	sort.Slice(deployments, func(i, j int) bool {
		return deployments[i].Name < deployments[j].Name
	})
	return deployments
}
