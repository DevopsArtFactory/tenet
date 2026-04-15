package catalog

import "github.com/DevopsArtFactory/tenet/internal/manifest"

// Summary is a UI-friendly count view of a compiled catalog.
type Summary struct {
	AccountCount     int `json:"accountCount"`
	EnvironmentCount int `json:"environmentCount"`
	DeploymentCount  int `json:"deploymentCount"`
}

// Summarize reduces a compiled catalog to stable count metadata.
func Summarize(c manifest.Catalog) Summary {
	return Summary{
		AccountCount:     len(c.Accounts),
		EnvironmentCount: len(c.Environments),
		DeploymentCount:  len(c.Deployments),
	}
}
