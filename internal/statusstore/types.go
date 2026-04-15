package statusstore

import "github.com/DevopsArtFactory/tenet/internal/domain"

// Snapshot is a persisted deployment status entry keyed by deployment name.
type Snapshot struct {
	DeploymentName string                  `json:"deploymentName"`
	Status         domain.DeploymentStatus `json:"status"`
}
