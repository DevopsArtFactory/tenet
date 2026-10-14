package planpreview

import (
	"fmt"
	"strings"

	"github.com/DevopsArtFactory/tenet/internal/catalog"
	"github.com/DevopsArtFactory/tenet/internal/reconcile"
)

func deriveECSPlanOperations(detail catalog.DeploymentOverviewDetail) []Operation {
	e := detail.ECS
	before := effectiveDeployedArtifact(detail).Image
	if before == "" {
		before = "unknown (live observation required)"
	}
	warning := ""
	if !strings.Contains(e.Image, "@sha256:") {
		warning = " Image tag is mutable; pin a digest before execution for reproducible content."
	}
	operations := []Operation{
		{Kind: string(reconcile.StepRegisterECSTaskDefinition), Status: "pending", Phase: "Provisioning", Target: e.BaseTaskDefinitionARN, Summary: fmt.Sprintf("Derive a new revision from pinned base %s; container %s image: %s → %s.%s Role, network and runtime settings are inherited; validate the named container and required settings before execution.", e.BaseTaskDefinitionARN, e.ContainerName, before, e.Image, warning), ECS: e.Clone()},
		{Kind: string(reconcile.StepUpdateECSService), Status: "pending", Phase: "Provisioning", Target: e.ServiceARN, Summary: fmt.Sprintf("Update existing service in cluster %s using the registered revision; controller %s, strategy %s, desired count %d, rolling bounds %d/%d. Current service configuration must be observed before mutation.", e.ClusterARN, e.DeploymentController, e.DeploymentStrategy, e.DesiredCount, e.MinimumHealthyPercent, e.MaximumPercent), ECS: e.Clone()},
		{Kind: string(reconcile.StepVerifyServiceStability), Status: "pending", Phase: "Stabilizing", Target: e.ServiceARN, Summary: "Verify the intended ECS revision converges; accepting an update request is not deployment success.", ECS: e.Clone()},
	}
	return normalizeOperationIDs(operations)
}
