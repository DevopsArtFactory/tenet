package planpreview

import (
	"fmt"
	"strings"

	"github.com/DevopsArtFactory/tenet/internal/catalog"
	"github.com/DevopsArtFactory/tenet/internal/domain"
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
	if e.RollbackOnFailure {
		warning += " Automatic Rolling recovery is approved only after correlated ECS failure; failed intent remains held until a new Git intent or explicit retry approval."
	}
	operations := []Operation{
		{Kind: string(reconcile.StepRegisterECSTaskDefinition), Status: "pending", Phase: "Provisioning", Target: e.BaseTaskDefinitionARN, Summary: fmt.Sprintf("Derive a new revision from pinned base %s; container %s image: %s → %s.%s Role, network and runtime settings are inherited; validate the named container and required settings before execution.", e.BaseTaskDefinitionARN, e.ContainerName, before, e.Image, warning), ECS: e.Clone()},
		{Kind: string(reconcile.StepUpdateECSService), Status: "pending", Phase: "Provisioning", Target: e.ServiceARN, Summary: fmt.Sprintf("Update existing service in cluster %s using the registered revision; controller %s, strategy %s, desired count %d, rolling bounds %d/%d. Current service configuration must be observed before mutation.", e.ClusterARN, e.DeploymentController, e.DeploymentStrategy, e.DesiredCount, e.MinimumHealthyPercent, e.MaximumPercent), ECS: e.Clone()},
		{Kind: string(reconcile.StepVerifyServiceStability), Status: "pending", Phase: "Stabilizing", Target: e.ServiceARN, Summary: "Verify the intended ECS revision converges; accepting an update request is not deployment success.", ECS: e.Clone()},
	}

	if e.DeploymentStrategy != "ROLLING" {
		operations[1].Summary = fmt.Sprintf("Request native ECS %s for service %s with registered revision and %d tasks. %s ECS owns traffic progression; no CodeDeploy application or AppSpec is used.", e.DeploymentStrategy, e.ServiceARN, e.DesiredCount, reviewedNativeTraffic(e.Advanced))
		operations[2].Summary = "Observe the correlated ECS service deployment/revision, traffic and bake lifecycle, alarms/hooks and terminal outcome; request acceptance is not traffic or health evidence."
	}
	return normalizeOperationIDs(operations)
}

func reviewedNativeTraffic(c *domain.ECSAdvancedConfiguration) string {
	if c == nil {
		return "Advanced configuration is not reviewed; live execution is unavailable."
	}
	summary := fmt.Sprintf("ALB target pair %s / %s; production rule %s; infrastructure role %s; port %d.", c.Routing.TargetGroupARN, c.Routing.AlternateTargetGroupARN, c.Routing.ProductionListenerRuleARN, c.Routing.InfrastructureRoleARN, c.Routing.ContainerPort)
	if c.Routing.TestListenerRuleARN != "" {
		summary += " Test rule " + c.Routing.TestListenerRuleARN + "."
	}
	if c.BakeTimeInMinutes != nil {
		summary += fmt.Sprintf(" Post-traffic bake %d minutes.", *c.BakeTimeInMinutes)
	}
	if c.Canary != nil && c.Canary.Percent != nil && c.Canary.BakeTimeInMinutes != nil {
		summary += fmt.Sprintf(" Canary %g%% for %d minutes.", *c.Canary.Percent, *c.Canary.BakeTimeInMinutes)
	}
	if c.Linear != nil && c.Linear.StepPercent != nil && c.Linear.StepBakeTimeInMinutes != nil {
		summary += fmt.Sprintf(" Linear steps %g%% with %d minutes between steps.", *c.Linear.StepPercent, *c.Linear.StepBakeTimeInMinutes)
	}
	summary += fmt.Sprintf(" Lambda hooks: %d.", len(c.LifecycleHooks))
	if c.Alarms != nil {
		summary += fmt.Sprintf(" Alarms: %s; ECS rollback %t.", strings.Join(c.Alarms.Names, ", "), c.Alarms.Rollback)
	} else {
		summary += " Alarms: none."
	}
	return summary
}
