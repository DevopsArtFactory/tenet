package planpreview

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/DevopsArtFactory/tenet/internal/catalog"
	"github.com/DevopsArtFactory/tenet/internal/domain"
	"github.com/DevopsArtFactory/tenet/internal/reconcile"
	"github.com/DevopsArtFactory/tenet/internal/requestactor"
)

var (
	marshalStableRecordPayload = json.Marshal
)

// DetailProvider resolves the canonical deployment overview detail used to derive previews.
type DetailProvider func(context.Context, string) (catalog.DeploymentOverviewDetail, error)

// Service derives and persists immutable plan previews.
type Service struct {
	LoadDetail DetailProvider
	Store      Store
	Now        func() time.Time
}

// CreateResult captures the stored preview plus whether it was newly created.
type CreateResult struct {
	Record  Record
	Created bool
}

// Create derives and persists an immutable plan preview for one deployment.
func (s Service) Create(ctx context.Context, deploymentName string, req CreateRequest) (CreateResult, error) {
	if s.LoadDetail == nil {
		return CreateResult{}, fmt.Errorf("plan detail provider is required")
	}
	if s.Store == nil {
		return CreateResult{}, fmt.Errorf("plan preview store is required")
	}

	detail, err := s.LoadDetail(ctx, deploymentName)
	if err != nil {
		return CreateResult{}, err
	}
	source, err := deriveSourceContext(detail)
	if err != nil {
		return CreateResult{}, err
	}

	source.ManagedDrift = req.ManagedDrift.Clone()
	record := Record{
		DeploymentName: detail.Name,
		Status:         StatusReady,
		CreatedAt:      s.now().UTC().Format(time.RFC3339),
		Actor:          requestactor.FromContext(ctx),
		Operations:     derivePlanOperations(detail),
		Risk:           derivePlanRisk(detail),
		Source:         source,
	}
	record.Approval = Approval{
		Required: record.Risk.Level != "low",
		Summary:  record.Risk.Guidance,
		Reasons:  append([]string(nil), record.Risk.Reasons...),
	}
	if req.IfMatch != "" && req.IfMatch != source.Fingerprint {
		record.Status = StatusFailed
		record.Errors = []string{
			fmt.Sprintf("stale source fingerprint: expected %s but current deployment detail fingerprint is %s", req.IfMatch, source.Fingerprint),
		}
		record.Approval.Required = true
		record.Approval.Summary = "Approval should stop until the operator refreshes deployment detail and recreates the preview."
		record.Approval.Reasons = append(record.Approval.Reasons, "client-provided source fingerprint no longer matches the current deployment detail")
	}
	if len(record.Operations) == 0 {
		record.Status = StatusFailed
		record.Errors = append(record.Errors, "no plan operations could be derived from the current deployment detail")
		record.Approval.Required = true
		record.Approval.Summary = "Approval should stop until Tenet can derive rollout operations from the deployment detail."
		record.Approval.Reasons = append(record.Approval.Reasons, "derived plan operations are empty")
	}
	record.ID, err = stableRecordID(record)
	if err != nil {
		return CreateResult{}, err
	}

	existing, err := s.Store.Get(ctx, record.ID)
	if err == nil {
		return CreateResult{Record: NormalizeOperationIDs(existing), Created: false}, nil
	}
	if err != nil && !errors.Is(err, ErrNotFound) {
		return CreateResult{}, err
	}
	if err := s.Store.Save(ctx, record); err != nil {
		return CreateResult{}, err
	}
	stored, err := s.Store.Get(ctx, record.ID)
	if err != nil {
		return CreateResult{}, err
	}
	return CreateResult{Record: NormalizeOperationIDs(stored), Created: true}, nil
}

// Get returns one persisted immutable plan preview by ID.
func (s Service) Get(ctx context.Context, id string) (Record, error) {
	if s.Store == nil {
		return Record{}, fmt.Errorf("plan preview store is required")
	}
	record, err := s.Store.Get(ctx, id)
	if err != nil {
		return Record{}, err
	}
	return NormalizeOperationIDs(record), nil
}

// NormalizeOperationIDs returns a copy of record with backend-authored operation IDs filled in.
// It preserves existing non-empty IDs so legacy records can be projected with additive IDs
// without rewriting the immutable stored payload.
func NormalizeOperationIDs(record Record) Record {
	record.Source.Git = record.Source.Git.Clone()
	record.Source.ManagedDrift = record.Source.ManagedDrift.Clone()
	record.Operations = normalizeOperationIDs(record.Operations)
	return record
}

func (s Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now().UTC()
}

func deriveSourceContext(detail catalog.DeploymentOverviewDetail) (SourceContext, error) {
	fingerprint, err := catalog.DeriveDeploymentSourceFingerprint(detail)
	if err != nil {
		return SourceContext{}, err
	}
	return SourceContext{
		Git:                detail.Git.Clone(),
		DeploymentName:     detail.Name,
		Route:              "/v1/overview/deployments/" + detail.Name,
		Fingerprint:        fingerprint,
		TargetType:         detail.TargetType,
		Strategy:           detail.Strategy,
		ExecutionMode:      detail.ExecutionMode,
		RuntimeTimestamp:   runtimeTimestamp(detail),
		RuntimeSource:      runtimeSource(detail),
		CurrentStep:        effectiveCurrentStep(detail),
		DesiredGeneration:  desiredGeneration(detail),
		ObservedGeneration: observedGeneration(detail),
		Sync:               detail.Sync,
		Health:             detail.Health,
	}, nil
}

func stableRecordID(record Record) (string, error) {
	type operationPayload struct {
		ECS     *domain.ECSDeploymentIntent `json:"ecs,omitempty"`
		Kind    string                      `json:"kind"`
		Status  string                      `json:"status"`
		Phase   string                      `json:"phase"`
		Target  string                      `json:"target"`
		Summary string                      `json:"summary"`
	}
	operations := make([]operationPayload, 0, len(record.Operations))
	for _, operation := range record.Operations {
		operations = append(operations, operationPayload{
			ECS:     operation.ECS,
			Kind:    operation.Kind,
			Status:  operation.Status,
			Phase:   operation.Phase,
			Target:  operation.Target,
			Summary: operation.Summary,
		})
	}
	payload := struct {
		DeploymentName string             `json:"deploymentName"`
		Status         Status             `json:"status"`
		Operations     []operationPayload `json:"operations,omitempty"`
		Risk           Risk               `json:"risk"`
		Approval       Approval           `json:"approval"`
		Source         SourceContext      `json:"source"`
		Errors         []string           `json:"errors,omitempty"`
	}{
		DeploymentName: record.DeploymentName,
		Status:         record.Status,
		Operations:     operations,
		Risk:           record.Risk,
		Approval:       record.Approval,
		Source:         record.Source,
		Errors:         record.Errors,
	}
	content, err := marshalStableRecordPayload(payload)
	if err != nil {
		return "", fmt.Errorf("encode stable plan preview id: %w", err)
	}
	sum := sha256.Sum256(content)
	return "plan_" + hex.EncodeToString(sum[:8]), nil
}

func derivePlanOperations(detail catalog.DeploymentOverviewDetail) []Operation {
	if detail.ECS != nil {
		return deriveECSPlanOperations(detail)
	}
	steps := rolloutSteps(detail)
	resources := executionResources(detail)
	operations := make([]Operation, 0, len(steps))
	for index, step := range steps {
		operations = append(operations, Operation{
			Kind:    step.Kind,
			Status:  rolloutStepStatus(step, index, steps),
			Phase:   rolloutPhaseForStepKind(step.Kind),
			Target:  derivePlanOperationTarget(detail, step.Kind, resources),
			Summary: derivePlanOperationSummary(detail, step),
		})
	}
	return normalizeOperationIDs(operations)
}

func normalizeOperationIDs(operations []Operation) []Operation {
	if len(operations) == 0 {
		return nil
	}
	normalized := append([]Operation(nil), operations...)
	for i := range normalized {
		normalized[i].ECS = normalized[i].ECS.Clone()
	}
	used := make(map[string]struct{}, len(normalized))
	for _, operation := range normalized {
		if operation.ID != "" {
			used[operation.ID] = struct{}{}
		}
	}
	counts := make(map[string]int, len(normalized))
	for index := range normalized {
		if normalized[index].ID != "" {
			continue
		}
		base := planOperationIDBase(normalized[index])
		for {
			counts[base]++
			candidate := base + "-" + strconv.Itoa(counts[base])
			if _, exists := used[candidate]; exists {
				continue
			}
			normalized[index].ID = candidate
			used[candidate] = struct{}{}
			break
		}
	}
	return normalized
}

func planOperationIDBase(operation Operation) string {
	payload := struct {
		ECS     *domain.ECSDeploymentIntent `json:"ecs,omitempty"`
		Kind    string                      `json:"kind"`
		Phase   string                      `json:"phase"`
		Target  string                      `json:"target"`
		Summary string                      `json:"summary"`
	}{
		ECS:     operation.ECS,
		Kind:    operation.Kind,
		Phase:   operation.Phase,
		Target:  operation.Target,
		Summary: operation.Summary,
	}
	content, _ := json.Marshal(payload)
	sum := sha256.Sum256(content)
	return "op_" + hex.EncodeToString(sum[:6])
}

func derivePlanRisk(detail catalog.DeploymentOverviewDetail) Risk {
	driftRows := deriveDriftRows(detail)
	reasons := []string{}
	currentStep := effectiveCurrentStep(detail)
	hasHighDrift := false
	hasMediumDrift := false
	for _, row := range driftRows {
		switch row.severity {
		case "high":
			hasHighDrift = true
		case "medium":
			hasMediumDrift = true
		}
	}
	activeTrafficShift := currentStep == string(reconcile.StepShiftTrafficFull) ||
		currentStep == string(reconcile.StepShiftTrafficLinear) ||
		currentStep == string(reconcile.StepShiftTrafficCanary) ||
		currentStep == string(reconcile.StepPromoteCandidateCapacity)
	unhealthy := detail.Health.Overall == domain.HealthUnhealthy
	errorState := detail.Sync.State == domain.SyncStateError

	level := "low"
	if errorState || unhealthy || hasHighDrift || activeTrafficShift {
		level = "high"
	} else if detail.ECS != nil || hasMediumDrift || detail.Sync.State == domain.SyncStateReconciling || detail.Sync.State == domain.SyncStateOutOfSync || detail.Sync.State == domain.SyncStateDrifted || detail.Sync.State == domain.SyncStatePaused || len(rolloutSteps(detail)) > 1 {
		level = "medium"
	}

	if detail.ECS != nil {
		reasons = append(reasons, "native ECS registration/update intent requires review of the existing service and pinned base revision")
		if !strings.Contains(detail.ECS.Image, "@sha256:") {
			reasons = append(reasons, "image reference is mutable; pin a digest to commit to reviewed image content")
		}
	}
	if activeTrafficShift {
		reasons = append(reasons, fmt.Sprintf("current step %s changes live traffic", currentStep))
	}
	if hasHighDrift {
		reasons = append(reasons, "high-severity drift is present in the deployment read model")
	}
	if hasMediumDrift {
		reasons = append(reasons, "generation or runtime-intent drift is still visible")
	}
	if unhealthy {
		reasons = append(reasons, "runtime health is unhealthy")
	}
	if errorState {
		reasons = append(reasons, "sync state is Error")
	}
	if len(reasons) == 0 && detail.Sync.State != "" && detail.Sync.State != domain.SyncStateSynced {
		reasons = append(reasons, fmt.Sprintf("sync state is %s", detail.Sync.State))
	}
	if len(reasons) == 0 {
		reasons = append(reasons, "desired and runtime signals currently look stable")
	}

	guidance := "Approval can stay lightweight because the read model does not show active drift, unhealthy runtime signals, or traffic movement."
	if level == "medium" {
		guidance = "Approval should confirm the latest runtime summary, target resource, and step detail before enabling plan creation or apply flows."
	} else if level == "high" {
		guidance = "Approval should wait for explicit operator review because the current read model shows live-traffic movement, unhealthy runtime signals, or high-severity drift."
	}

	return Risk{
		Level:    level,
		Label:    level + " risk",
		Guidance: guidance,
		Reasons:  reasons,
	}
}

type driftRow struct {
	severity string
}

func deriveDriftRows(detail catalog.DeploymentOverviewDetail) []driftRow {
	rows := []driftRow{}
	desiredArtifact := artifact(detail.DesiredArtifact)
	actualArtifact := artifact(effectiveDeployedArtifact(detail))
	if desiredArtifact != "" && actualArtifact != "" && desiredArtifact != actualArtifact {
		rows = append(rows, driftRow{severity: "high"})
	}

	wantGeneration := desiredGeneration(detail)
	gotGeneration := observedGeneration(detail)
	if wantGeneration > 0 && gotGeneration > 0 && wantGeneration != gotGeneration {
		rows = append(rows, driftRow{severity: "medium"})
	}

	if detail.TargetType != "" && detail.RuntimeSummary != nil && detail.RuntimeSummary.TargetType != "" && detail.TargetType != detail.RuntimeSummary.TargetType {
		rows = append(rows, driftRow{severity: "medium"})
	}
	if detail.Strategy != "" && detail.RuntimeSummary != nil && detail.RuntimeSummary.Strategy != "" && detail.Strategy != detail.RuntimeSummary.Strategy {
		rows = append(rows, driftRow{severity: "medium"})
	}
	if detail.ExecutionMode != "" && detail.RuntimeSummary != nil && detail.RuntimeSummary.ExecutionMode != "" && detail.ExecutionMode != detail.RuntimeSummary.ExecutionMode {
		rows = append(rows, driftRow{severity: "medium"})
	}
	for _, condition := range detail.Conditions {
		if condition.Type == "DriftDetected" && condition.Status {
			rows = append(rows, driftRow{severity: "high"})
		}
		if condition.Type == "Healthy" && !condition.Status {
			rows = append(rows, driftRow{severity: "low"})
		}
	}
	return rows
}

func rolloutPhaseForStepKind(kind string) string {
	switch kind {
	case string(reconcile.StepCreateCandidateCapacity), string(reconcile.StepStartInstanceRefresh), string(reconcile.StepCreateCodeDeployDeployment), string(reconcile.StepUpdateECSService):
		return "Provisioning"
	case string(reconcile.StepVerifyCandidateCapacity):
		return "Health checking"
	case string(reconcile.StepShiftTrafficCanary), string(reconcile.StepShiftTrafficLinear), string(reconcile.StepShiftTrafficFull), string(reconcile.StepPromoteCandidateCapacity):
		return "Promoting"
	case string(reconcile.StepVerifyReplacementCapacity), string(reconcile.StepVerifyServiceStability):
		return "Stabilizing"
	case string(reconcile.StepMarkSynced):
		return "Completed"
	default:
		return "Accepted"
	}
}

func rolloutStepStatus(step domain.RolloutStepSummary, index int, steps []domain.RolloutStepSummary) string {
	switch step.Status {
	case domain.RolloutStepStatusCompleted:
		return string(domain.RolloutStepStatusCompleted)
	case domain.RolloutStepStatusCurrent:
		return string(domain.RolloutStepStatusCurrent)
	case domain.RolloutStepStatusPending:
		return string(domain.RolloutStepStatusPending)
	}
	currentIndex := -1
	for i, candidate := range steps {
		if candidate.Current {
			currentIndex = i
			break
		}
	}
	if currentIndex == -1 {
		if step.Current {
			return string(domain.RolloutStepStatusCurrent)
		}
		return string(domain.RolloutStepStatusPending)
	}
	if step.Current {
		return string(domain.RolloutStepStatusCurrent)
	}
	if index < currentIndex {
		return string(domain.RolloutStepStatusCompleted)
	}
	return string(domain.RolloutStepStatusPending)
}

func derivePlanOperationTarget(detail catalog.DeploymentOverviewDetail, stepKind string, resources []domain.RolloutExecutionDetail) string {
	preferredTypes := stepTargetResourceTypes(detail, stepKind)
	for _, resource := range resources {
		for _, preferred := range preferredTypes {
			if resource.Resource == preferred {
				return fmt.Sprintf("%s (%s)", resource.Name, resource.Resource)
			}
		}
	}
	if stepKind == string(reconcile.StepMarkSynced) {
		return detail.Name + " deployment read model"
	}
	return detail.Name + " deployment"
}

func stepTargetResourceTypes(detail catalog.DeploymentOverviewDetail, stepKind string) []string {
	executionMode := detail.ExecutionMode
	if detail.RuntimeSummary != nil && detail.RuntimeSummary.ExecutionMode != "" {
		executionMode = detail.RuntimeSummary.ExecutionMode
	}
	switch stepKind {
	case string(reconcile.StepCreateCandidateCapacity), string(reconcile.StepVerifyCandidateCapacity), string(reconcile.StepPromoteCandidateCapacity):
		return []string{"candidateAutoScalingGroup", "serviceAutoScalingGroup"}
	case string(reconcile.StepShiftTrafficCanary), string(reconcile.StepShiftTrafficLinear), string(reconcile.StepShiftTrafficFull):
		if executionMode == "ecsCodeDeploy" {
			return []string{"codeDeployDeploymentGroup", "productionListener", "testListener"}
		}
		return []string{"trafficShiftPlan", "productionListenerRule", "activeTargetGroup", "candidateTargetGroup"}
	case string(reconcile.StepStartInstanceRefresh), string(reconcile.StepVerifyReplacementCapacity):
		return []string{"serviceAutoScalingGroup"}
	case string(reconcile.StepUpdateECSService), string(reconcile.StepVerifyServiceStability):
		return []string{"ecsService", "codeDeployDeploymentGroup"}
	case string(reconcile.StepCreateCodeDeployDeployment):
		return []string{"codeDeployDeploymentGroup", "codeDeployApplication"}
	default:
		return nil
	}
}

func derivePlanOperationSummary(detail catalog.DeploymentOverviewDetail, step domain.RolloutStepSummary) string {
	executionMode := detail.ExecutionMode
	if detail.RuntimeSummary != nil && detail.RuntimeSummary.ExecutionMode != "" {
		executionMode = detail.RuntimeSummary.ExecutionMode
	}
	if step.Detail != "" {
		return step.Detail
	}
	switch step.Kind {
	case string(reconcile.StepCreateCandidateCapacity):
		return "Prepare fresh candidate EC2 capacity before traffic shifting."
	case string(reconcile.StepVerifyCandidateCapacity):
		return "Confirm the candidate EC2 capacity is healthy enough for live traffic."
	case string(reconcile.StepShiftTrafficCanary):
		if executionMode == "ecsCodeDeploy" {
			return "Shift a small live ECS traffic slice through CodeDeploy before wider promotion."
		}
		return "Shift a small live EC2 traffic slice through weighted target groups before wider promotion."
	case string(reconcile.StepShiftTrafficLinear):
		if executionMode == "ecsCodeDeploy" {
			return "Advance ECS traffic in controller-defined CodeDeploy increments."
		}
		return "Advance EC2 traffic in controller-defined weighted target-group increments."
	case string(reconcile.StepShiftTrafficFull):
		if executionMode == "ecsCodeDeploy" {
			return "Cut all ECS traffic to the replacement task set in one promotion step."
		}
		return "Cut all EC2 traffic to the candidate target group in one promotion step."
	case string(reconcile.StepPromoteCandidateCapacity):
		return "Promote the candidate EC2 capacity to become the steady-state fleet."
	case string(reconcile.StepStartInstanceRefresh):
		return "Start the EC2 instance refresh against the existing service Auto Scaling Group."
	case string(reconcile.StepVerifyReplacementCapacity):
		return "Confirm refreshed EC2 instances are ready before the rollout is considered stable."
	case string(reconcile.StepUpdateECSService):
		return "Update the ECS service to create the replacement deployment."
	case string(reconcile.StepCreateCodeDeployDeployment):
		return "Create the CodeDeploy deployment Tenet will use for traffic shifting."
	case string(reconcile.StepVerifyServiceStability):
		if executionMode == "ecsCodeDeploy" {
			return "Confirm the ECS service stays healthy after the CodeDeploy traffic shift."
		}
		return "Confirm the ECS service stays healthy after the rolling update."
	case string(reconcile.StepMarkSynced):
		return "No further rollout mutation is implied by the current read model."
	default:
		return "Await the next controller-authored rollout detail."
	}
}

func rolloutSteps(detail catalog.DeploymentOverviewDetail) []domain.RolloutStepSummary {
	if detail.Rollout != nil && len(detail.Rollout.Steps) > 0 {
		return append([]domain.RolloutStepSummary(nil), detail.Rollout.Steps...)
	}
	plan := reconcile.Planner{}.Build(domain.Deployment{
		Name:            detail.Name,
		AppRef:          detail.AppRef,
		EnvironmentRef:  detail.EnvironmentRef,
		Generation:      desiredGeneration(detail),
		TargetType:      detail.TargetType,
		Strategy:        detail.Strategy,
		ExecutionMode:   detail.ExecutionMode,
		DesiredArtifact: detail.DesiredArtifact,
		UserData:        detail.UserData,
		RolloutConfig:   detail.RolloutConfig,
	}, domain.DeploymentStatus{
		ObservedGeneration: observedGeneration(detail),
		Sync:               detail.Sync,
		Health:             detail.Health,
		Conditions:         append([]domain.Condition(nil), detail.Conditions...),
		History:            append([]domain.StatusHistoryEntry(nil), detail.History...),
		DeployedArtifact:   effectiveDeployedArtifact(detail),
		Rollout:            detail.Rollout,
	})
	steps := make([]domain.RolloutStepSummary, 0, len(plan.Steps))
	for _, step := range plan.Steps {
		steps = append(steps, domain.RolloutStepSummary{Kind: string(step.Kind)})
	}
	return steps
}

func executionResources(detail catalog.DeploymentOverviewDetail) []domain.RolloutExecutionDetail {
	if detail.ECS != nil {
		return []domain.RolloutExecutionDetail{{Resource: "ecsService", Name: detail.ECS.ServiceARN}}
	}
	if detail.Rollout != nil && len(detail.Rollout.Execution) > 0 {
		return append([]domain.RolloutExecutionDetail(nil), detail.Rollout.Execution...)
	}
	switch detail.ExecutionMode {
	case "ecsCodeDeploy":
		return []domain.RolloutExecutionDetail{
			{Resource: "ecsService", Name: detail.Name + "-service"},
			{Resource: "codeDeployApplication", Name: detail.Name + "-codedeploy-app"},
			{Resource: "codeDeployDeploymentGroup", Name: detail.Name + "-codedeploy-group"},
			{Resource: "productionListener", Name: detail.Name + "-prod-listener"},
			{Resource: "testListener", Name: detail.Name + "-test-listener"},
			{Resource: "blueTaskSetTargetGroup", Name: detail.Name + "-blue"},
			{Resource: "greenTaskSetTargetGroup", Name: detail.Name + "-green"},
		}
	case "ecsRolling":
		return []domain.RolloutExecutionDetail{{Resource: "ecsService", Name: detail.Name + "-service"}}
	case "asgWeightedTargetGroups":
		return []domain.RolloutExecutionDetail{
			{Resource: "candidateAutoScalingGroup", Name: detail.Name + "-candidate"},
			{Resource: "activeTargetGroup", Name: detail.Name + "-active"},
			{Resource: "candidateTargetGroup", Name: detail.Name + "-candidate"},
			{Resource: "productionListenerRule", Name: detail.Name + "-prod-listener"},
			{Resource: "trafficShiftPlan", Name: detail.Name + "-weighted-routing"},
			{Resource: "userDataBootstrapSource", Name: canonicalUserDataBootstrapSource(detail.UserData)},
		}
	case "asgInstanceRefresh":
		return []domain.RolloutExecutionDetail{
			{Resource: "serviceAutoScalingGroup", Name: detail.Name + "-service"},
			{Resource: "userDataBootstrapSource", Name: canonicalUserDataBootstrapSource(detail.UserData)},
		}
	default:
		return nil
	}
}

func canonicalUserDataBootstrapSource(userData domain.DeploymentUserData) string {
	if userData.Type == "" && userData.Path == "" {
		return "unspecified"
	}
	if userData.Path == "" {
		return userData.Type
	}
	if userData.Type == "" {
		return userData.Path
	}
	return userData.Type + ":" + userData.Path
}

func desiredGeneration(detail catalog.DeploymentOverviewDetail) int64 {
	switch {
	case detail.Rollout != nil && detail.Rollout.DesiredGeneration > 0:
		return detail.Rollout.DesiredGeneration
	case detail.RuntimeSummary != nil && detail.RuntimeSummary.DesiredGeneration > 0:
		return detail.RuntimeSummary.DesiredGeneration
	case observedGeneration(detail) > 0 && detail.Sync.State == domain.SyncStateSynced && detail.Health.Overall.IsHealthy():
		return observedGeneration(detail)
	case observedGeneration(detail) > 0:
		return observedGeneration(detail) + 1
	default:
		return 1
	}
}

func observedGeneration(detail catalog.DeploymentOverviewDetail) int64 {
	if detail.RuntimeSummary != nil && detail.RuntimeSummary.ObservedGeneration > 0 {
		return detail.RuntimeSummary.ObservedGeneration
	}
	return detail.ObservedGeneration
}

func runtimeTimestamp(detail catalog.DeploymentOverviewDetail) string {
	if detail.RuntimeSummary != nil {
		return detail.RuntimeSummary.Timestamp
	}
	return ""
}

func runtimeSource(detail catalog.DeploymentOverviewDetail) string {
	if detail.RuntimeSummary != nil {
		return detail.RuntimeSummary.Source
	}
	return ""
}

func runtimeChanges(detail catalog.DeploymentOverviewDetail) []string {
	if detail.RuntimeSummary != nil {
		return append([]string(nil), detail.RuntimeSummary.Changes...)
	}
	return nil
}

func runtimeChangeDetails(detail catalog.DeploymentOverviewDetail) []string {
	if detail.RuntimeSummary != nil {
		return append([]string(nil), detail.RuntimeSummary.ChangeDetails...)
	}
	return nil
}

func effectiveCurrentStep(detail catalog.DeploymentOverviewDetail) string {
	if detail.Rollout != nil && detail.Rollout.CurrentStep != "" {
		return detail.Rollout.CurrentStep
	}
	if detail.RuntimeSummary != nil {
		return detail.RuntimeSummary.CurrentStep
	}
	return ""
}

func effectiveDeployedArtifact(detail catalog.DeploymentOverviewDetail) domain.DeploymentArtifact {
	if detail.DeployedArtifact != (domain.DeploymentArtifact{}) {
		return detail.DeployedArtifact
	}
	if detail.RuntimeSummary != nil {
		return detail.RuntimeSummary.DeployedArtifact
	}
	return domain.DeploymentArtifact{}
}

func artifact(value domain.DeploymentArtifact) string {
	if value.Binary != "" {
		return value.Binary
	}
	if value.Image != "" {
		return value.Image
	}
	return ""
}
