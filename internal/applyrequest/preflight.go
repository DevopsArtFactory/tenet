package applyrequest

import (
	"context"
	"fmt"
	"net/url"
	"strings"
)

const (
	ExecutionReadinessStateDisabled = "disabled"
	ExecutionReadinessStateBlocked  = "blocked"
)

// ExecutionReadiness is backend-authored non-mutating preflight evidence for one immutable apply request.
// It describes whether the stored operation inventory has enough local metadata to be considered by a
// future AWS executor, while explicitly avoiding any live AWS mutation claim.
type ExecutionReadiness struct {
	State      string                        `json:"state"`
	Summary    string                        `json:"summary"`
	CheckedAt  string                        `json:"checkedAt,omitempty"`
	Evidence   []string                      `json:"evidence,omitempty"`
	Operations []OperationExecutionPreflight `json:"operations,omitempty"`
}

// OperationExecutionPreflightHandoffs captures read-only troubleshooting routes for one preflight row.
type OperationExecutionPreflightHandoffs struct {
	DeploymentDetailRoute   string `json:"deploymentDetailRoute,omitempty"`
	PlanApplyWorkspaceRoute string `json:"planApplyWorkspaceRoute,omitempty"`
	ResourceInspectionRoute string `json:"resourceInspectionRoute,omitempty"`
	ResourceLogsRoute       string `json:"resourceLogsRoute,omitempty"`
}

// OperationExecutionPreflight captures one non-mutating readiness row for an apply operation.
type OperationExecutionPreflight struct {
	OperationID        string                               `json:"operationId,omitempty"`
	Phase              string                               `json:"phase,omitempty"`
	Target             string                               `json:"target"`
	ResourceType       string                               `json:"resourceType,omitempty"`
	ResourceName       string                               `json:"resourceName,omitempty"`
	RequiredCapability string                               `json:"requiredCapability"`
	RequiredRole       string                               `json:"requiredRole"`
	InputsSufficient   bool                                 `json:"inputsSufficient"`
	Ready              bool                                 `json:"ready"`
	UnavailableReason  string                               `json:"unavailableReason,omitempty"`
	Summary            string                               `json:"summary"`
	Handoffs           *OperationExecutionPreflightHandoffs `json:"handoffs,omitempty"`
}

// Preflight returns backend-authored execution readiness evidence for one apply request.
// The same immutable applyId is used; this method only reads persisted state and never calls AWS.
func (s Service) Preflight(ctx context.Context, id string) (ExecutionReadiness, error) {
	record, err := s.Get(ctx, id)
	if err != nil {
		return ExecutionReadiness{}, err
	}
	if record.ExecutionReadiness == nil {
		return deriveExecutionReadiness(record), nil
	}
	return *record.ExecutionReadiness, nil
}

func normalizeExecutionReadiness(record Record) Record {
	record.Operations = normalizeOperationIDs(record.Operations)
	readiness := deriveExecutionReadiness(record)
	record.ExecutionReadiness = &readiness
	return record
}

func deriveExecutionReadiness(record Record) ExecutionReadiness {
	checkedAt := record.Lifecycle.UpdatedAt
	if checkedAt == "" {
		checkedAt = record.CreatedAt
	}
	state := ExecutionReadinessStateBlocked
	summary := "Live AWS mutation preflight is blocked because Tenet currently records local-safe apply lifecycle metadata only."
	evidence := []string{
		"Preflight reused immutable applyId " + record.ID + " and did not contact AWS.",
		"Future live executors must satisfy these per-operation AWS capability and role checks before mutation.",
	}
	if record.Status == StatusFailed && record.Error == disabledError {
		state = ExecutionReadinessStateDisabled
		summary = "Live AWS mutation is disabled for this local-safe apply record; Tenet preserved reviewed intent without contacting AWS."
		evidence = []string{
			"TENET_AWS_ENABLE_APPLY is unset or false, so apply creation persisted a failed local-safe record by design and did not contact AWS.",
			"Reviewed operations remain visible for operator handoff, but no AWS mutation preflight passed.",
		}
	} else if record.Status.IsTerminal() {
		summary = fmt.Sprintf("Apply request %s is terminal with status %s; create or inspect a newer apply record before execution.", record.ID, record.Status)
		evidence = []string{
			"Terminal apply records are immutable audit/read-model evidence and are not eligible for executor claims.",
			"No live AWS mutation readiness is reported for terminal apply state.",
		}
	} else if record.Status == StatusQueued || record.Status == StatusRunning {
		summary = "Local-safe executor handoff metadata is present, but live AWS mutation remains blocked until a future executor implements these capability checks."
		evidence = []string{
			"Preflight reused immutable applyId " + record.ID + " and did not contact AWS.",
			"The apply record is eligible for the current local-safe claim/heartbeat/transition lifecycle, but Tenet has not implemented live AWS mutation for queued or running apply records yet.",
		}
	}
	return ExecutionReadiness{
		State:      state,
		Summary:    summary,
		CheckedAt:  checkedAt,
		Evidence:   evidence,
		Operations: deriveOperationExecutionPreflight(record),
	}
}

func deriveOperationExecutionPreflight(record Record) []OperationExecutionPreflight {
	if len(record.Operations) == 0 {
		return nil
	}
	rows := make([]OperationExecutionPreflight, 0, len(record.Operations))
	for _, operation := range record.Operations {
		resourceName, resourceType := parseOperationTarget(operation.Target)
		capability := requiredCapability(resourceType)
		inputsSufficient := strings.TrimSpace(operation.Target) != ""
		reason := "live_aws_mutation_not_implemented"
		summary := "Operation has reviewed apply intent, but live AWS mutation is not implemented for this local-safe executor boundary."
		if !inputsSufficient {
			reason = "missing_operation_target"
			summary = "Operation is missing a concrete target from the reviewed plan preview."
		}
		if record.Status == StatusFailed && record.Error == disabledError {
			reason = "execution_disabled"
			summary = "Operation belongs to a disabled local-safe apply record; reviewed intent is preserved but AWS mutation was not enabled."
		} else if record.Status.IsTerminal() {
			reason = "terminal_apply_request"
			summary = "Operation belongs to a terminal immutable apply record and will not be executed from this applyId."
		}
		rows = append(rows, OperationExecutionPreflight{
			OperationID:        operation.ID,
			Phase:              operation.Phase,
			Target:             operation.Target,
			ResourceType:       resourceType,
			ResourceName:       resourceName,
			RequiredCapability: capability,
			RequiredRole:       "future AWS mutation role for deployment " + record.DeploymentName,
			InputsSufficient:   inputsSufficient,
			Ready:              false,
			UnavailableReason:  reason,
			Summary:            summary,
			Handoffs:           operationExecutionHandoffs(record, resourceType, resourceName),
		})
	}
	return rows
}

func operationExecutionHandoffs(record Record, resourceType, resourceName string) *OperationExecutionPreflightHandoffs {
	deploymentName := strings.TrimSpace(record.DeploymentName)
	if deploymentName == "" {
		return nil
	}
	deploymentPath := "/v1/overview/deployments/" + url.PathEscape(deploymentName)
	handoffs := &OperationExecutionPreflightHandoffs{
		DeploymentDetailRoute:   deploymentPath,
		PlanApplyWorkspaceRoute: buildPlanApplyWorkspaceRoute(deploymentName, record.PlanID, record.ID),
	}
	resourceType = strings.TrimSpace(resourceType)
	resourceName = strings.TrimSpace(resourceName)
	if resourceType != "" && resourceName != "" {
		query := url.Values{}
		query.Set("resourceType", resourceType)
		query.Set("resourceName", resourceName)
		handoffs.ResourceInspectionRoute = deploymentPath + "/resource-inspection?" + query.Encode()
		handoffs.ResourceLogsRoute = deploymentPath + "/resource-logs?" + query.Encode()
	}
	return handoffs
}

func buildPlanApplyWorkspaceRoute(deploymentName, planID, applyID string) string {
	query := url.Values{}
	query.Set("page", "plan")
	query.Set("deployment", deploymentName)
	if strings.TrimSpace(planID) != "" {
		query.Set("planId", strings.TrimSpace(planID))
	}
	if strings.TrimSpace(applyID) != "" {
		query.Set("applyId", strings.TrimSpace(applyID))
	}
	return "/?" + query.Encode()
}

func parseOperationTarget(target string) (string, string) {
	target = strings.TrimSpace(target)
	if target == "" {
		return "", ""
	}
	if strings.HasSuffix(target, ")") {
		open := strings.LastIndex(target, "(")
		if open >= 0 {
			name := strings.TrimSpace(target[:open])
			resourceType := strings.TrimSpace(strings.TrimSuffix(target[open+1:], ")"))
			return name, resourceType
		}
	}
	return target, ""
}

func requiredCapability(resourceType string) string {
	switch resourceType {
	case "codeDeployDeploymentGroup":
		return "codedeploy:CreateDeployment"
	case "productionListener", "testListener", "productionListenerRule", "trafficShiftPlan":
		return "elasticloadbalancing:ModifyListener or elasticloadbalancing:ModifyRule"
	case "candidateAutoScalingGroup", "serviceAutoScalingGroup":
		return "autoscaling:UpdateAutoScalingGroup"
	case "ecsService":
		return "ecs:UpdateService"
	case "userDataBootstrapSource":
		return "ec2:DescribeLaunchTemplateVersions"
	case "codeDeployApplication":
		return "codedeploy:GetApplication"
	default:
		return "future AWS mutation capability"
	}
}
