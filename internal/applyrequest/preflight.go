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
	State        string                        `json:"state"`
	Summary      string                        `json:"summary"`
	CheckedAt    string                        `json:"checkedAt,omitempty"`
	Evidence     []string                      `json:"evidence,omitempty"`
	DeployerRole *DeployerRoleReadiness        `json:"deployerRole,omitempty"`
	Operations   []OperationExecutionPreflight `json:"operations,omitempty"`
}

// DeployerRoleTarget identifies the deployment placement whose deployer role is checked.
type DeployerRoleTarget struct {
	DeploymentName string
}

// DeployerRoleReadiness captures non-mutating identity readiness for the
// configured target-account deployer role. It is troubleshooting evidence only,
// not proof that rollout mutation ran or succeeded.
type DeployerRoleReadiness struct {
	AWSAccountID      string   `json:"awsAccountId,omitempty"`
	Region            string   `json:"region,omitempty"`
	RoleARN           string   `json:"roleArn,omitempty"`
	Configured        bool     `json:"configured"`
	CheckedWithAWS    bool     `json:"checkedWithAws"`
	Ready             bool     `json:"ready"`
	UnavailableReason string   `json:"unavailableReason,omitempty"`
	Summary           string   `json:"summary"`
	Evidence          []string `json:"evidence,omitempty"`
}

// OperationExecutionPreflightHandoffs captures read-only troubleshooting routes for one preflight row.
type OperationExecutionPreflightHandoffs struct {
	DeploymentDetailRoute   string `json:"deploymentDetailRoute,omitempty"`
	PlanApplyWorkspaceRoute string `json:"planApplyWorkspaceRoute,omitempty"`
	ResourceInspectionRoute string `json:"resourceInspectionRoute,omitempty"`
	ResourceLogsRoute       string `json:"resourceLogsRoute,omitempty"`
}

// OperationExecutionCapabilityCheck captures one deterministic, non-mutating capability item
// that a future live executor would need to satisfy before mutating AWS resources.
type OperationExecutionCapabilityCheck struct {
	Action       string `json:"action"`
	ResourceType string `json:"resourceType,omitempty"`
	ResourceName string `json:"resourceName,omitempty"`
	RequiredRole string `json:"requiredRole"`
	State        string `json:"state"`
	Reason       string `json:"reason,omitempty"`
	Summary      string `json:"summary"`
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
	CapabilityChecks   []OperationExecutionCapabilityCheck  `json:"capabilityChecks,omitempty"`
	Handoffs           *OperationExecutionPreflightHandoffs `json:"handoffs,omitempty"`
}

// Preflight returns backend-authored execution readiness evidence for one apply request.
// The same immutable applyId is used; this method never mutates AWS. A configured
// DeployerRoleReadiness provider may add non-mutating identity-readiness evidence.
func (s Service) Preflight(ctx context.Context, id string) (ExecutionReadiness, error) {
	record, err := s.Get(ctx, id)
	if err != nil {
		return ExecutionReadiness{}, err
	}
	readiness := deriveExecutionReadiness(record)
	if record.ExecutionReadiness != nil {
		readiness = *record.ExecutionReadiness
		if len(readiness.Operations) == 0 && len(record.Operations) > 0 {
			readiness.Operations = deriveOperationExecutionPreflight(record, nil)
		} else {
			readiness.Operations = backfillOperationCapabilityChecks(readiness.Operations)
		}
	}
	if s.DeployerRoleReadiness == nil {
		return readiness, nil
	}
	role, err := s.DeployerRoleReadiness(ctx, DeployerRoleTarget{DeploymentName: record.DeploymentName})
	if err != nil {
		return ExecutionReadiness{}, err
	}
	readiness.DeployerRole = &role
	readiness.Operations = deriveOperationExecutionPreflight(record, &role)
	return readiness, nil
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
		Operations: deriveOperationExecutionPreflight(record, nil),
	}
}

func deriveOperationExecutionPreflight(record Record, role *DeployerRoleReadiness) []OperationExecutionPreflight {
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
		requiredRole := "future AWS mutation role for deployment " + record.DeploymentName
		if role != nil {
			if strings.TrimSpace(role.RoleARN) != "" {
				requiredRole = strings.TrimSpace(role.RoleARN)
			}
			if !role.Ready {
				inputsSufficient = false
				reason = strings.TrimSpace(role.UnavailableReason)
				if reason == "" {
					reason = "deployer_role_unavailable"
				}
				summary = strings.TrimSpace(role.Summary)
				if summary == "" {
					summary = "Configured deployer role readiness could not be proven for this local-safe apply preflight."
				}
			}
		}
		if !inputsSufficient && strings.TrimSpace(operation.Target) == "" {
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
			RequiredRole:       requiredRole,
			InputsSufficient:   inputsSufficient,
			Ready:              false,
			UnavailableReason:  reason,
			Summary:            summary,
			CapabilityChecks:   operationCapabilityChecks(resourceType, resourceName, requiredRole, reason, inputsSufficient),
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

func backfillOperationCapabilityChecks(rows []OperationExecutionPreflight) []OperationExecutionPreflight {
	if len(rows) == 0 {
		return rows
	}
	backfilled := make([]OperationExecutionPreflight, len(rows))
	copy(backfilled, rows)
	for i := range backfilled {
		if len(backfilled[i].CapabilityChecks) > 0 {
			continue
		}
		inputsSufficient := backfilled[i].InputsSufficient
		if strings.TrimSpace(backfilled[i].Target) == "" {
			inputsSufficient = false
		}
		backfilled[i].CapabilityChecks = operationCapabilityChecks(backfilled[i].ResourceType, backfilled[i].ResourceName, backfilled[i].RequiredRole, backfilled[i].UnavailableReason, inputsSufficient)
	}
	return backfilled
}

func operationCapabilityChecks(resourceType, resourceName, requiredRole, reason string, inputsSufficient bool) []OperationExecutionCapabilityCheck {
	actions := capabilityActions(resourceType)
	if len(actions) == 0 {
		actions = []string{"future AWS mutation capability"}
	}
	state := "blocked"
	if !inputsSufficient {
		state = "unknown"
	}
	summary := capabilityCheckSummary(resourceType, state)
	checks := make([]OperationExecutionCapabilityCheck, 0, len(actions))
	for _, action := range actions {
		checks = append(checks, OperationExecutionCapabilityCheck{
			Action:       action,
			ResourceType: resourceType,
			ResourceName: resourceName,
			RequiredRole: requiredRole,
			State:        state,
			Reason:       reason,
			Summary:      summary,
		})
	}
	return checks
}

func capabilityActions(resourceType string) []string {
	switch resourceType {
	case "codeDeployDeploymentGroup":
		return []string{"codedeploy:GetDeploymentGroup", "codedeploy:CreateDeployment"}
	case "codeDeployApplication":
		return []string{"codedeploy:GetApplication"}
	case "ecsService":
		return []string{"ecs:DescribeServices", "ecs:UpdateService"}
	case "blueTaskSetTargetGroup", "greenTaskSetTargetGroup", "activeTargetGroup", "candidateTargetGroup":
		return []string{"elasticloadbalancing:DescribeTargetGroups"}
	case "productionListener", "testListener":
		return []string{"elasticloadbalancing:DescribeListeners", "elasticloadbalancing:ModifyListener"}
	case "productionListenerRule", "trafficShiftPlan":
		return []string{"elasticloadbalancing:DescribeRules", "elasticloadbalancing:ModifyRule"}
	case "candidateAutoScalingGroup", "serviceAutoScalingGroup":
		return []string{"autoscaling:DescribeAutoScalingGroups", "autoscaling:UpdateAutoScalingGroup"}
	case "userDataBootstrapSource":
		return []string{"ec2:DescribeLaunchTemplateVersions"}
	default:
		return []string{"future AWS mutation capability"}
	}
}

func capabilityCheckSummary(resourceType, state string) string {
	family := "this rollout resource"
	switch resourceType {
	case "codeDeployDeploymentGroup", "codeDeployApplication", "ecsService", "blueTaskSetTargetGroup", "greenTaskSetTargetGroup", "productionListener", "testListener":
		family = "the ECS/CodeDeploy rollout boundary"
	case "candidateAutoScalingGroup", "serviceAutoScalingGroup", "activeTargetGroup", "candidateTargetGroup", "productionListenerRule", "trafficShiftPlan", "userDataBootstrapSource":
		family = "the EC2/ASG rollout boundary"
	}
	if state == "unknown" {
		return "Tenet cannot evaluate the required capability because the reviewed operation lacks a concrete resource target."
	}
	return "Future live executors must prove this AWS capability for " + family + "; this local-safe preflight does not perform live mutation."
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
