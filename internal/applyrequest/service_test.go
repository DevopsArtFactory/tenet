package applyrequest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DevopsArtFactory/tenet/internal/planpreview"
	"github.com/DevopsArtFactory/tenet/internal/requestactor"
)

type memoryStore struct {
	mu        sync.Mutex
	records   map[string]Record
	saveCalls int
}

func (s *memoryStore) Get(_ context.Context, id string) (Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.records[id]
	if !ok {
		return Record{}, ErrNotFound
	}
	return record, nil
}

func (s *memoryStore) List(_ context.Context) ([]Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	records := make([]Record, 0, len(s.records))
	for _, record := range s.records {
		records = append(records, record)
	}
	return list(records), nil
}

func (s *memoryStore) Save(_ context.Context, record Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.saveCalls++
	if s.records == nil {
		s.records = map[string]Record{}
	}
	if _, exists := s.records[record.ID]; !exists {
		s.records[record.ID] = record
	}
	return nil
}

func (s *memoryStore) Update(_ context.Context, record Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.records == nil {
		s.records = map[string]Record{}
	}
	if _, exists := s.records[record.ID]; !exists {
		return ErrNotFound
	}
	s.records[record.ID] = record
	return nil
}

func (s *memoryStore) Claim(_ context.Context, id string, _ ClaimRequest, claim func(Record) (Record, error)) (Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.records == nil {
		return Record{}, ErrNotFound
	}
	record, ok := s.records[id]
	if !ok {
		return Record{}, ErrNotFound
	}
	updated, err := claim(record)
	if err != nil {
		return Record{}, err
	}
	s.records[id] = updated
	return updated, nil
}

func (s *memoryStore) Heartbeat(_ context.Context, id string, _ HeartbeatRequest, heartbeat func(Record) (Record, error)) (Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.records == nil {
		return Record{}, ErrNotFound
	}
	record, ok := s.records[id]
	if !ok {
		return Record{}, ErrNotFound
	}
	updated, err := heartbeat(record)
	if err != nil {
		return Record{}, err
	}
	s.records[id] = updated
	return updated, nil
}

func TestServiceCreatePersistsImmutableFailedApplyRequestWhenExecutionIsDisabled(t *testing.T) {
	store := &memoryStore{}
	plan := planpreview.Record{
		ID:             "plan_ready_1234abcd",
		DeploymentName: "api-prod",
		Status:         planpreview.StatusReady,
		CreatedAt:      "2026-04-17T08:00:00Z",
		Operations: []planpreview.Operation{
			{Status: "completed", Phase: "Provisioning", Target: "api-prod-codedeploy-group (codeDeployDeploymentGroup)", Summary: "Create the CodeDeploy deployment for api-prod."},
			{Status: "current", Phase: "Promoting", Target: "api-prod-prod-listener (productionListener)", Summary: "Shift production traffic toward the replacement capacity for api-prod."},
		},
		Source: planpreview.SourceContext{DeploymentName: "api-prod", Route: "/v1/overview/deployments/api-prod", Fingerprint: "plan_input"},
	}
	service := Service{
		LoadPlan: func(ctx context.Context, planID string) (planpreview.Record, error) {
			if planID != plan.ID {
				t.Fatalf("unexpected plan lookup: %s", planID)
			}
			return plan, nil
		},
		Store: store,
		Now:   func() time.Time { return time.Date(2026, 4, 18, 8, 0, 0, 0, time.UTC) },
	}

	first, err := service.Create(context.Background(), plan.ID)
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if !first.Created {
		t.Fatalf("expected first create to persist a new apply request")
	}
	if first.Record.ID == "" || first.Record.Status != StatusFailed {
		t.Fatalf("unexpected created apply request: %+v", first.Record)
	}
	if first.Record.PlanID != plan.ID || first.Record.Source.Route != "/v1/plans/"+plan.ID {
		t.Fatalf("unexpected apply source context: %+v", first.Record)
	}
	if first.Record.Actor.Available || first.Record.Actor.Reason != requestactor.UnavailableReason {
		t.Fatalf("expected explicit unavailable actor, got %+v", first.Record.Actor)
	}
	if first.Record.Error != disabledError || len(first.Record.Details) != 2 {
		t.Fatalf("expected disabled error/details, got %+v", first.Record)
	}
	if first.Record.ExecutionReadiness == nil || first.Record.ExecutionReadiness.State != ExecutionReadinessStateDisabled {
		t.Fatalf("expected disabled backend execution readiness evidence, got %+v", first.Record.ExecutionReadiness)
	}
	if len(first.Record.ExecutionReadiness.Operations) != 2 || first.Record.ExecutionReadiness.Operations[0].RequiredCapability != "codedeploy:CreateDeployment" || first.Record.ExecutionReadiness.Operations[0].Ready {
		t.Fatalf("expected non-mutating per-operation preflight rows, got %+v", first.Record.ExecutionReadiness.Operations)
	}
	checks := first.Record.ExecutionReadiness.Operations[0].CapabilityChecks
	if len(checks) != 2 || checks[0].Action != "codedeploy:GetDeploymentGroup" || checks[1].Action != "codedeploy:CreateDeployment" || checks[0].State != "blocked" || checks[0].Reason != "execution_disabled" {
		t.Fatalf("expected deterministic CodeDeploy capability checks, got %+v", checks)
	}
	if handoffs := first.Record.ExecutionReadiness.Operations[0].Handoffs; handoffs == nil || handoffs.DeploymentDetailRoute != "/v1/overview/deployments/api-prod" || handoffs.PlanApplyWorkspaceRoute != "/?applyId="+first.Record.ID+"&deployment=api-prod&page=plan&planId="+plan.ID || handoffs.ResourceInspectionRoute != "/v1/overview/deployments/api-prod/resource-inspection?resourceName=api-prod-codedeploy-group&resourceType=codeDeployDeploymentGroup" || handoffs.ResourceLogsRoute != "/v1/overview/deployments/api-prod/resource-logs?resourceName=api-prod-codedeploy-group&resourceType=codeDeployDeploymentGroup" {
		t.Fatalf("expected operation troubleshooting handoffs, got %+v", handoffs)
	}
	if first.Record.Lifecycle.UpdatedAt != "2026-04-18T08:00:00Z" || first.Record.Lifecycle.FinishedAt != "2026-04-18T08:00:00Z" || first.Record.Lifecycle.TotalSteps != 2 {
		t.Fatalf("expected additive lifecycle metadata on create, got %+v", first.Record.Lifecycle)
	}
	if len(first.Record.Operations) != 2 {
		t.Fatalf("expected projected operations, got %+v", first.Record.Operations)
	}
	if first.Record.Operations[0].Status != OperationStatusPending || first.Record.Operations[0].Phase != "Provisioning" {
		t.Fatalf("expected pending projected operation rows, got %+v", first.Record.Operations)
	}
	if first.Record.Operations[1].Target != "api-prod-prod-listener (productionListener)" || first.Record.Operations[1].Summary != "Shift production traffic toward the replacement capacity for api-prod." {
		t.Fatalf("expected ordered plan projection into apply operations, got %+v", first.Record.Operations)
	}

	second, err := service.Create(context.Background(), plan.ID)
	if err != nil {
		t.Fatalf("second Create returned error: %v", err)
	}
	if second.Created {
		t.Fatalf("expected repeated create to reuse the immutable apply request")
	}
	if second.Record.ID != first.Record.ID || second.Record.CreatedAt != "2026-04-18T08:00:00Z" {
		t.Fatalf("expected stable readback, got first=%+v second=%+v", first.Record, second.Record)
	}

	fetched, err := service.Get(context.Background(), first.Record.ID)
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if fetched.ID != first.Record.ID || fetched.PlanID != plan.ID || fetched.Status != StatusFailed {
		t.Fatalf("unexpected fetched apply request: %+v", fetched)
	}
}

func TestServiceCreatePersistsImmutableQueuedApplyRequestWhenExecutionIsEnabled(t *testing.T) {
	store := &memoryStore{}
	plan := planpreview.Record{
		ID:             "plan_ready_1234abcd",
		DeploymentName: "api-prod",
		Status:         planpreview.StatusReady,
		CreatedAt:      "2026-04-17T08:00:00Z",
		Operations: []planpreview.Operation{
			{Status: "completed", Phase: "Provisioning", Target: "api-prod deployment", Summary: "Create replacement capacity for api-prod."},
			{Status: "current", Phase: "Promoting", Target: "api-prod-prod-listener (productionListener)", Summary: "Shift production traffic toward the replacement capacity for api-prod."},
		},
		Source: planpreview.SourceContext{DeploymentName: "api-prod", Route: "/v1/overview/deployments/api-prod", Fingerprint: "plan_input"},
	}
	service := Service{
		LoadPlan: func(context.Context, string) (planpreview.Record, error) {
			return plan, nil
		},
		Store:          store,
		Now:            func() time.Time { return time.Date(2026, 4, 18, 8, 5, 0, 0, time.UTC) },
		EnableAWSApply: true,
	}

	result, err := service.Create(context.Background(), plan.ID)
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if !result.Created {
		t.Fatalf("expected first create to persist a queued apply request")
	}
	if result.Record.Status != StatusQueued || result.Record.Error != "" {
		t.Fatalf("expected queued apply request without disabled error, got %+v", result.Record)
	}
	if result.Record.Lifecycle.UpdatedAt != "2026-04-18T08:05:00Z" || result.Record.Lifecycle.FinishedAt != "" || result.Record.Lifecycle.TotalSteps != 2 {
		t.Fatalf("expected queued lifecycle metadata, got %+v", result.Record.Lifecycle)
	}
	if len(result.Record.Details) != 2 || !strings.Contains(result.Record.Details[0], "TENET_AWS_ENABLE_APPLY=true") || !strings.Contains(result.Record.Details[1], "same applyId locally") {
		t.Fatalf("expected queued details, got %+v", result.Record.Details)
	}
	if result.Record.ExecutionReadiness == nil || result.Record.ExecutionReadiness.State != ExecutionReadinessStateBlocked || !strings.Contains(result.Record.ExecutionReadiness.Summary, "Local-safe executor handoff") {
		t.Fatalf("expected queued preflight to report blocked live mutation with local-safe handoff evidence, got %+v", result.Record.ExecutionReadiness)
	}
	if len(result.Record.Operations) != 2 {
		t.Fatalf("expected projected operations, got %+v", result.Record.Operations)
	}
	if result.Record.Operations[0].Status != OperationStatusQueued || result.Record.Operations[1].Status != OperationStatusQueued {
		t.Fatalf("expected queued projected operation rows, got %+v", result.Record.Operations)
	}
}

func TestServicePreflightReturnsBackendAuthoredReadinessEvidence(t *testing.T) {
	store := &memoryStore{records: map[string]Record{
		"apply_1234abcd": {
			ID:             "apply_1234abcd",
			PlanID:         "plan_1234abcd",
			DeploymentName: "api-prod",
			Status:         StatusQueued,
			CreatedAt:      "2026-04-18T08:05:00Z",
			Lifecycle:      Lifecycle{UpdatedAt: "2026-04-18T08:06:00Z", TotalSteps: 1},
			Operations: []Operation{{
				ID:      "op_codedeploy-1",
				Status:  OperationStatusQueued,
				Phase:   "Promoting",
				Target:  "api-prod-codedeploy-group (codeDeployDeploymentGroup)",
				Summary: "Create the CodeDeploy deployment for api-prod.",
			}},
			Source: SourceContext{PlanID: "plan_1234abcd", Route: "/v1/plans/plan_1234abcd", PlanStatus: "ready"},
		},
	}}
	service := Service{Store: store}

	readiness, err := service.Preflight(context.Background(), "apply_1234abcd")
	if err != nil {
		t.Fatalf("Preflight returned error: %v", err)
	}
	if readiness.State != ExecutionReadinessStateBlocked || readiness.CheckedAt != "2026-04-18T08:06:00Z" {
		t.Fatalf("unexpected readiness summary: %+v", readiness)
	}
	if len(readiness.Evidence) != 2 || !strings.Contains(readiness.Evidence[0], "apply_1234abcd") {
		t.Fatalf("expected immutable applyId evidence, got %+v", readiness.Evidence)
	}
	if len(readiness.Operations) != 1 {
		t.Fatalf("expected one operation preflight row, got %+v", readiness.Operations)
	}
	operation := readiness.Operations[0]
	if operation.OperationID != "op_codedeploy-1" || operation.ResourceName != "api-prod-codedeploy-group" || operation.ResourceType != "codeDeployDeploymentGroup" {
		t.Fatalf("expected parsed operation resource identity, got %+v", operation)
	}
	if operation.RequiredCapability != "codedeploy:CreateDeployment" || operation.RequiredRole == "" || !operation.InputsSufficient || operation.Ready || operation.UnavailableReason != "live_aws_mutation_not_implemented" {
		t.Fatalf("expected explicit blocked AWS capability preflight row, got %+v", operation)
	}
	if len(operation.CapabilityChecks) != 2 || operation.CapabilityChecks[0].Action != "codedeploy:GetDeploymentGroup" || operation.CapabilityChecks[1].Action != "codedeploy:CreateDeployment" || operation.CapabilityChecks[0].RequiredRole != operation.RequiredRole || operation.CapabilityChecks[0].State != "blocked" || operation.CapabilityChecks[0].Reason != "live_aws_mutation_not_implemented" {
		t.Fatalf("expected nested CodeDeploy capability matrix, got %+v", operation.CapabilityChecks)
	}
	if operation.Handoffs == nil || operation.Handoffs.DeploymentDetailRoute != "/v1/overview/deployments/api-prod" || operation.Handoffs.PlanApplyWorkspaceRoute != "/?applyId=apply_1234abcd&deployment=api-prod&page=plan&planId=plan_1234abcd" {
		t.Fatalf("expected deployment and workspace handoffs, got %+v", operation.Handoffs)
	}
	if operation.Handoffs.ResourceInspectionRoute != "/v1/overview/deployments/api-prod/resource-inspection?resourceName=api-prod-codedeploy-group&resourceType=codeDeployDeploymentGroup" || operation.Handoffs.ResourceLogsRoute != "/v1/overview/deployments/api-prod/resource-logs?resourceName=api-prod-codedeploy-group&resourceType=codeDeployDeploymentGroup" {
		t.Fatalf("expected resource troubleshooting handoffs, got %+v", operation.Handoffs)
	}
}

func TestServicePreflightIncludesASGCapabilityMatrix(t *testing.T) {
	store := &memoryStore{records: map[string]Record{
		"apply_asg": {
			ID:             "apply_asg",
			PlanID:         "plan_asg",
			DeploymentName: "web-prod",
			Status:         StatusQueued,
			CreatedAt:      "2026-04-18T08:05:00Z",
			Operations: []Operation{{
				ID:      "op-asg",
				Phase:   "Provisioning",
				Target:  "web-prod-candidate (candidateAutoScalingGroup)",
				Summary: "Create replacement EC2 capacity for web-prod.",
			}},
		},
	}}
	service := Service{Store: store}

	readiness, err := service.Preflight(context.Background(), "apply_asg")
	if err != nil {
		t.Fatalf("Preflight returned error: %v", err)
	}
	if len(readiness.Operations) != 1 {
		t.Fatalf("expected one operation, got %+v", readiness.Operations)
	}
	operation := readiness.Operations[0]
	if operation.RequiredCapability != "autoscaling:UpdateAutoScalingGroup" || operation.ResourceType != "candidateAutoScalingGroup" {
		t.Fatalf("expected ASG capability row, got %+v", operation)
	}
	if len(operation.CapabilityChecks) != 2 || operation.CapabilityChecks[0].Action != "autoscaling:DescribeAutoScalingGroups" || operation.CapabilityChecks[1].Action != "autoscaling:UpdateAutoScalingGroup" {
		t.Fatalf("expected deterministic ASG capability checks, got %+v", operation.CapabilityChecks)
	}
	if operation.CapabilityChecks[0].State != "blocked" || operation.CapabilityChecks[0].ResourceName != "web-prod-candidate" || !strings.Contains(operation.CapabilityChecks[0].Summary, "EC2/ASG rollout boundary") {
		t.Fatalf("expected operator-facing ASG capability summary, got %+v", operation.CapabilityChecks[0])
	}
}

func TestServicePreflightReportsTerminalApplyRequestsAsBlocked(t *testing.T) {
	store := &memoryStore{records: map[string]Record{
		"apply_failed": {ID: "apply_failed", PlanID: "plan_1234", DeploymentName: "api-prod", Status: StatusFailed, CreatedAt: "2026-04-18T08:05:00Z", Error: "promotion failed", Operations: []Operation{{Target: "api-prod listener (productionListener)"}}},
	}}
	service := Service{Store: store}

	readiness, err := service.Preflight(context.Background(), "apply_failed")
	if err != nil {
		t.Fatalf("Preflight returned error: %v", err)
	}
	if readiness.State != ExecutionReadinessStateBlocked || !strings.Contains(readiness.Summary, "terminal") {
		t.Fatalf("expected terminal apply request preflight blocker, got %+v", readiness)
	}
	if len(readiness.Operations) != 1 || readiness.Operations[0].UnavailableReason != "terminal_apply_request" || readiness.Operations[0].Ready {
		t.Fatalf("expected terminal operation to stay blocked, got %+v", readiness.Operations)
	}
}

func TestServicePreflightSkipsDeployerRoleProbeForTerminalApplyRequests(t *testing.T) {
	store := &memoryStore{records: map[string]Record{
		"apply_failed": {
			ID:             "apply_failed",
			PlanID:         "plan_1234",
			DeploymentName: "api-prod",
			Status:         StatusFailed,
			CreatedAt:      "2026-04-18T08:05:00Z",
			Error:          disabledError,
			Operations:     []Operation{{Target: "api-prod listener (productionListener)"}},
		},
	}}
	service := Service{
		Store: store,
		DeployerRoleReadiness: func(ctx context.Context, target DeployerRoleTarget) (DeployerRoleReadiness, error) {
			t.Fatalf("terminal apply request should not probe live deployer-role readiness")
			return DeployerRoleReadiness{}, nil
		},
	}

	readiness, err := service.Preflight(context.Background(), "apply_failed")
	if err != nil {
		t.Fatalf("Preflight returned error: %v", err)
	}
	if readiness.DeployerRole != nil {
		t.Fatalf("expected no live deployer-role evidence for terminal local-safe record, got %+v", readiness.DeployerRole)
	}
	if readiness.State != ExecutionReadinessStateDisabled || !strings.Contains(readiness.Summary, "local-safe apply record") {
		t.Fatalf("expected deterministic disabled readiness without AWS probing, got %+v", readiness)
	}
}

func TestServicePreflightIncludesConfiguredDeployerRoleProbeEvidence(t *testing.T) {
	store := &memoryStore{records: map[string]Record{
		"apply_queued": {
			ID:             "apply_queued",
			PlanID:         "plan_1234",
			DeploymentName: "api-prod",
			Status:         StatusQueued,
			CreatedAt:      "2026-04-18T08:05:00Z",
			Operations: []Operation{{
				ID:      "op-codedeploy",
				Phase:   "Promoting",
				Target:  "api-prod-codedeploy-group (codeDeployDeploymentGroup)",
				Summary: "Create CodeDeploy deployment.",
			}},
		},
	}}
	var gotTarget DeployerRoleTarget
	service := Service{
		Store: store,
		DeployerRoleReadiness: func(ctx context.Context, target DeployerRoleTarget) (DeployerRoleReadiness, error) {
			gotTarget = target
			return DeployerRoleReadiness{
				AWSAccountID:      "123456789012",
				Region:            "ap-northeast-2",
				RoleARN:           "arn:aws:iam::123456789012:role/TenetDeployerRole",
				Configured:        true,
				Ready:             true,
				Summary:           "Configured deployer role was assumed with a read-only identity probe.",
				Evidence:          []string{"STS GetCallerIdentity succeeded after assuming the configured deployer role."},
				CheckedWithAWS:    true,
				UnavailableReason: "",
			}, nil
		},
	}

	readiness, err := service.Preflight(context.Background(), "apply_queued")
	if err != nil {
		t.Fatalf("Preflight returned error: %v", err)
	}
	if gotTarget.DeploymentName != "api-prod" {
		t.Fatalf("expected deployment target api-prod, got %+v", gotTarget)
	}
	if readiness.DeployerRole == nil || !readiness.DeployerRole.Configured || !readiness.DeployerRole.Ready || !readiness.DeployerRole.CheckedWithAWS {
		t.Fatalf("expected configured deployer-role readiness evidence, got %+v", readiness.DeployerRole)
	}
	if readiness.DeployerRole.RoleARN != "arn:aws:iam::123456789012:role/TenetDeployerRole" || readiness.DeployerRole.AWSAccountID != "123456789012" || readiness.DeployerRole.Region != "ap-northeast-2" {
		t.Fatalf("expected target account/region/role metadata, got %+v", readiness.DeployerRole)
	}
	if len(readiness.Operations) != 1 || readiness.Operations[0].RequiredRole != "arn:aws:iam::123456789012:role/TenetDeployerRole" || readiness.Operations[0].UnavailableReason != "live_aws_mutation_not_implemented" || readiness.Operations[0].Ready {
		t.Fatalf("expected operation to keep local-safe mutation blocker while naming deployer role, got %+v", readiness.Operations)
	}
}

func TestServiceExecutionPreviewNamesNextCodeDeployActionAndAWSContext(t *testing.T) {
	store := &memoryStore{records: map[string]Record{
		"apply_queued": {
			ID:             "apply_queued",
			PlanID:         "plan_1234",
			DeploymentName: "api-prod",
			Status:         StatusQueued,
			CreatedAt:      "2026-04-18T08:05:00Z",
			Lifecycle:      Lifecycle{UpdatedAt: "2026-04-18T08:06:00Z"},
			Operations: []Operation{{
				ID:      "op-codedeploy",
				Phase:   "Promoting",
				Target:  "api-prod-codedeploy-group (codeDeployDeploymentGroup)",
				Summary: "Create CodeDeploy deployment.",
			}},
			Source: SourceContext{PlanID: "plan_1234", Route: "/v1/plans/plan_1234", PlanStatus: "ready"},
		},
	}}
	service := Service{
		Store: store,
		DeployerRoleReadiness: func(ctx context.Context, target DeployerRoleTarget) (DeployerRoleReadiness, error) {
			return DeployerRoleReadiness{
				AWSAccountID:   "123456789012",
				Region:         "ap-northeast-2",
				RoleARN:        "arn:aws:iam::123456789012:role/TenetDeployerRole",
				Configured:     true,
				Ready:          true,
				CheckedWithAWS: true,
				Summary:        "Configured deployer role was assumed with a read-only identity probe.",
			}, nil
		},
	}

	preview, err := service.ExecutionPreview(context.Background(), "apply_queued")
	if err != nil {
		t.Fatalf("ExecutionPreview returned error: %v", err)
	}
	if !preview.ReadOnly || preview.ApplyID != "apply_queued" || preview.DeploymentName != "api-prod" || preview.CheckedAt != "2026-04-18T08:06:00Z" {
		t.Fatalf("expected immutable read-only preview identity, got %+v", preview)
	}
	if preview.Operation == nil || preview.Operation.Action != "codedeploy:CreateDeployment" || preview.Operation.ResourceType != "codeDeployDeploymentGroup" || preview.Operation.ResourceName != "api-prod-codedeploy-group" {
		t.Fatalf("expected next CodeDeploy action/resource preview, got %+v", preview.Operation)
	}
	if preview.Operation.RequiredRole != "arn:aws:iam::123456789012:role/TenetDeployerRole" || len(preview.Operation.CapabilityChecks) != 2 || preview.Operation.CapabilityChecks[1].Action != "codedeploy:CreateDeployment" {
		t.Fatalf("expected role-aware CodeDeploy capability checks, got %+v", preview.Operation)
	}
	if len(preview.Operations) != 1 || preview.Operations[0].OperationID != preview.Operation.OperationID || preview.Operations[0].Action != preview.Operation.Action || preview.Operations[0].Ready || !preview.Operations[0].ReadOnly || preview.Operations[0].Blocker == nil || preview.Operations[0].Handoffs == nil {
		t.Fatalf("expected compatibility operation mirrored into ordered operations preview, got operation=%+v operations=%+v", preview.Operation, preview.Operations)
	}
	if preview.AWSContext.AWSAccountID != "123456789012" || preview.AWSContext.Region != "ap-northeast-2" || !preview.AWSContext.RoleConfigured || !preview.AWSContext.RoleReady || !preview.AWSContext.CheckedWithAWS {
		t.Fatalf("expected target AWS context, got %+v", preview.AWSContext)
	}
	if preview.Blocker == nil || preview.Blocker.Reason != "live_aws_mutation_not_implemented" || !strings.Contains(preview.Blocker.Summary, "live AWS mutation is not implemented") {
		t.Fatalf("expected explicit non-mutating blocker, got %+v", preview.Blocker)
	}
}

func TestServiceExecutionPreviewNamesNextASGActionWithMissingRoleBlocker(t *testing.T) {
	store := &memoryStore{records: map[string]Record{
		"apply_asg": {ID: "apply_asg", PlanID: "plan_asg", DeploymentName: "web-prod", Status: StatusQueued, CreatedAt: "2026-04-18T08:05:00Z", Operations: []Operation{{ID: "op-asg", Phase: "Provisioning", Target: "web-prod-candidate (candidateAutoScalingGroup)", Summary: "Create replacement EC2 capacity for web-prod."}}},
	}}
	service := Service{
		Store: store,
		DeployerRoleReadiness: func(ctx context.Context, target DeployerRoleTarget) (DeployerRoleReadiness, error) {
			return DeployerRoleReadiness{AWSAccountID: "210987654321", Region: "us-west-2", Configured: false, Ready: false, UnavailableReason: "missing_deployer_role", Summary: "No deployer role is configured for this target account and region."}, nil
		},
	}

	preview, err := service.ExecutionPreview(context.Background(), "apply_asg")
	if err != nil {
		t.Fatalf("ExecutionPreview returned error: %v", err)
	}
	if preview.Operation == nil || preview.Operation.Action != "autoscaling:UpdateAutoScalingGroup" || preview.Operation.ResourceType != "candidateAutoScalingGroup" || preview.Operation.ResourceName != "web-prod-candidate" {
		t.Fatalf("expected next ASG action/resource preview, got %+v", preview.Operation)
	}
	if preview.AWSContext.AWSAccountID != "210987654321" || preview.AWSContext.Region != "us-west-2" || preview.AWSContext.RoleConfigured || preview.AWSContext.RoleReady || preview.AWSContext.UnavailableReason != "missing_deployer_role" {
		t.Fatalf("expected missing-role AWS context, got %+v", preview.AWSContext)
	}
	if preview.Blocker == nil || preview.Blocker.Reason != "missing_deployer_role" || !strings.Contains(preview.Blocker.Summary, "No deployer role") {
		t.Fatalf("expected missing deployer role blocker, got %+v", preview.Blocker)
	}
}

func TestServiceExecutionPreviewIncludesOrderedOperationRows(t *testing.T) {
	store := &memoryStore{records: map[string]Record{
		"apply_multi": {
			ID:             "apply_multi",
			PlanID:         "plan_multi",
			DeploymentName: "web-prod",
			Status:         StatusQueued,
			CreatedAt:      "2026-04-18T08:05:00Z",
			Operations: []Operation{
				{ID: "op-traffic", Phase: "Promoting", Target: "web-prod traffic shift (trafficShiftPlan)", Summary: "Shift production listener rule traffic."},
				{ID: "op-asg", Phase: "Provisioning", Target: "web-prod-candidate (candidateAutoScalingGroup)", Summary: "Scale the candidate ASG."},
			},
			Source: SourceContext{PlanID: "plan_multi", Route: "/v1/plans/plan_multi", PlanStatus: "ready"},
		},
	}}
	service := Service{Store: store}

	preview, err := service.ExecutionPreview(context.Background(), "apply_multi")
	if err != nil {
		t.Fatalf("ExecutionPreview returned error: %v", err)
	}
	if preview.Operation == nil || preview.Operation.OperationID != "op-traffic" || preview.Operation.Action != "elasticloadbalancing:ModifyListener or elasticloadbalancing:ModifyRule" {
		t.Fatalf("expected compatibility first action to remain first reviewed operation, got %+v", preview.Operation)
	}
	if len(preview.Operations) != 2 {
		t.Fatalf("expected ordered execution preview rows for every apply operation, got %+v", preview.Operations)
	}
	gotIDs := []string{preview.Operations[0].OperationID, preview.Operations[1].OperationID}
	if want := []string{"op-traffic", "op-asg"}; !reflect.DeepEqual(gotIDs, want) {
		t.Fatalf("expected preview operations to preserve immutable operation order: got %+v want %+v", gotIDs, want)
	}
	if preview.Operations[0].Summary != "Operation has reviewed apply intent, but live AWS mutation is not implemented for this local-safe executor boundary." || preview.Operations[0].Blocker == nil || preview.Operations[0].Blocker.Reason != "live_aws_mutation_not_implemented" {
		t.Fatalf("expected first operation row to include read-only blocker evidence, got %+v", preview.Operations[0])
	}
	if preview.Operations[0].Handoffs == nil || preview.Operations[0].Handoffs.ResourceInspectionRoute != "/v1/overview/deployments/web-prod/resource-inspection?resourceName=web-prod+traffic+shift&resourceType=trafficShiftPlan" {
		t.Fatalf("expected per-operation troubleshooting handoff evidence, got %+v", preview.Operations[0].Handoffs)
	}
	if preview.Operations[1].Action != "autoscaling:UpdateAutoScalingGroup" || preview.Operations[1].ResourceType != "candidateAutoScalingGroup" || preview.Operations[1].Ready {
		t.Fatalf("expected second operation to expose its own derived ASG action/readiness, got %+v", preview.Operations[1])
	}
}

func TestServicePreflightReportsMissingDeployerRole(t *testing.T) {
	store := &memoryStore{records: map[string]Record{
		"apply_queued": {ID: "apply_queued", PlanID: "plan_1234", DeploymentName: "api-prod", Status: StatusQueued, CreatedAt: "2026-04-18T08:05:00Z", Operations: []Operation{{Target: "api-prod-service (ecsService)"}}},
	}}
	service := Service{
		Store: store,
		DeployerRoleReadiness: func(ctx context.Context, target DeployerRoleTarget) (DeployerRoleReadiness, error) {
			return DeployerRoleReadiness{AWSAccountID: "123456789012", Region: "ap-northeast-2", Configured: false, Ready: false, UnavailableReason: "missing_deployer_role", Summary: "No deployer role is configured for this target account and region."}, nil
		},
	}

	readiness, err := service.Preflight(context.Background(), "apply_queued")
	if err != nil {
		t.Fatalf("Preflight returned error: %v", err)
	}
	if readiness.DeployerRole == nil || readiness.DeployerRole.Configured || readiness.DeployerRole.UnavailableReason != "missing_deployer_role" {
		t.Fatalf("expected missing deployer-role evidence, got %+v", readiness.DeployerRole)
	}
	if len(readiness.Operations) != 1 || readiness.Operations[0].UnavailableReason != "missing_deployer_role" || readiness.Operations[0].InputsSufficient || readiness.Operations[0].Ready {
		t.Fatalf("expected missing deployer role to block operation inputs, got %+v", readiness.Operations)
	}
}

func TestServicePreflightReportsAWSUnavailableForConfiguredDeployerRole(t *testing.T) {
	store := &memoryStore{records: map[string]Record{
		"apply_queued": {ID: "apply_queued", PlanID: "plan_1234", DeploymentName: "api-prod", Status: StatusQueued, CreatedAt: "2026-04-18T08:05:00Z", Operations: []Operation{{Target: "api-prod-service (ecsService)"}}},
	}}
	service := Service{
		Store: store,
		DeployerRoleReadiness: func(ctx context.Context, target DeployerRoleTarget) (DeployerRoleReadiness, error) {
			return DeployerRoleReadiness{AWSAccountID: "123456789012", Region: "ap-northeast-2", RoleARN: "arn:aws:iam::123456789012:role/TenetDeployerRole", Configured: true, Ready: false, CheckedWithAWS: true, UnavailableReason: "aws_unavailable", Summary: "AWS credentials or provider access are unavailable in this local-safe environment."}, nil
		},
	}

	readiness, err := service.Preflight(context.Background(), "apply_queued")
	if err != nil {
		t.Fatalf("Preflight returned error: %v", err)
	}
	if readiness.DeployerRole == nil || readiness.DeployerRole.UnavailableReason != "aws_unavailable" || readiness.DeployerRole.Ready {
		t.Fatalf("expected AWS-unavailable deployer-role evidence, got %+v", readiness.DeployerRole)
	}
	if len(readiness.Operations) != 1 || readiness.Operations[0].RequiredRole != "arn:aws:iam::123456789012:role/TenetDeployerRole" || readiness.Operations[0].UnavailableReason != "aws_unavailable" || readiness.Operations[0].Ready {
		t.Fatalf("expected AWS-unavailable role probe to block operation readiness, got %+v", readiness.Operations)
	}
}

func TestServiceListFiltersQueuedApplyRequests(t *testing.T) {
	store := &memoryStore{
		records: map[string]Record{
			"apply_running": {
				ID:             "apply_running",
				PlanID:         "plan_running",
				DeploymentName: "api-prod",
				Status:         StatusRunning,
				CreatedAt:      "2026-04-18T08:06:00Z",
			},
			"apply_queued_b": {
				ID:             "apply_queued_b",
				PlanID:         "plan_queued_b",
				DeploymentName: "worker-prod",
				Status:         StatusQueued,
				CreatedAt:      "2026-04-18T08:05:00Z",
				Operations: []Operation{{
					Status:  OperationStatusQueued,
					Phase:   "Promoting",
					Target:  "worker-prod listener",
					Summary: "Shift production traffic.",
				}},
			},
			"apply_failed": {
				ID:             "apply_failed",
				PlanID:         "plan_failed",
				DeploymentName: "api-prod",
				Status:         StatusFailed,
				CreatedAt:      "2026-04-18T08:00:00Z",
			},
			"apply_queued_a": {
				ID:             "apply_queued_a",
				PlanID:         "plan_queued_a",
				DeploymentName: "api-prod",
				Status:         StatusQueued,
				CreatedAt:      "2026-04-18T08:04:00Z",
			},
		},
	}
	service := Service{Store: store}

	records, err := service.List(context.Background(), ListQuery{Status: StatusQueued})
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	got := make([]string, 0, len(records))
	for _, record := range records {
		got = append(got, record.ID)
		if record.Status != StatusQueued {
			t.Fatalf("expected only queued records, got %+v", records)
		}
	}
	if want := []string{"apply_queued_a", "apply_queued_b"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected queued apply records: got %+v want %+v", got, want)
	}
	if len(records[1].Operations) != 1 || records[1].Operations[0].ID == "" {
		t.Fatalf("expected listed records to normalize additive operation ids, got %+v", records[1].Operations)
	}
}

func TestServiceListFiltersActiveApplyRequestsNewestFirst(t *testing.T) {
	store := &memoryStore{
		records: map[string]Record{
			"apply_failed_newest": {
				ID:             "apply_failed_newest",
				PlanID:         "plan_failed",
				DeploymentName: "api-prod",
				Status:         StatusFailed,
				CreatedAt:      "2026-04-18T08:08:00Z",
			},
			"apply_queued_new": {
				ID:             "apply_queued_new",
				PlanID:         "plan_queued_new",
				DeploymentName: "worker-prod",
				Status:         StatusQueued,
				CreatedAt:      "2026-04-18T08:07:00Z",
			},
			"apply_running": {
				ID:             "apply_running",
				PlanID:         "plan_running",
				DeploymentName: "api-prod",
				Status:         StatusRunning,
				CreatedAt:      "2026-04-18T08:06:00Z",
				Claim:          &Claim{WorkerID: "worker-a", ClaimedAt: "2026-04-18T08:06:30Z", LastHeartbeatAt: "2026-04-18T08:07:30Z", ExpiresAt: "2026-04-18T08:12:30Z"},
				Operations: []Operation{{
					Status:  OperationStatusRunning,
					Phase:   "Promoting",
					Target:  "api-prod listener",
					Summary: "Shift production traffic.",
				}},
			},
			"apply_cancelled": {
				ID:             "apply_cancelled",
				PlanID:         "plan_cancelled",
				DeploymentName: "worker-prod",
				Status:         StatusCancelled,
				CreatedAt:      "2026-04-18T08:05:00Z",
			},
			"apply_queued_old": {
				ID:             "apply_queued_old",
				PlanID:         "plan_queued_old",
				DeploymentName: "api-prod",
				Status:         StatusQueued,
				CreatedAt:      "2026-04-18T08:04:00Z",
			},
		},
	}
	service := Service{Store: store}

	records, err := service.List(context.Background(), ListQuery{Status: Status("active")})
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	got := make([]string, 0, len(records))
	for _, record := range records {
		got = append(got, record.ID)
		if record.Status != StatusQueued && record.Status != StatusRunning {
			t.Fatalf("expected only queued/running active records, got %+v", records)
		}
	}
	if want := []string{"apply_queued_new", "apply_running", "apply_queued_old"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected active apply records: got %+v want %+v", got, want)
	}
	if records[1].Claim == nil || records[1].Claim.LastHeartbeatAt != "2026-04-18T08:07:30Z" || records[1].Claim.ExpiresAt != "2026-04-18T08:12:30Z" {
		t.Fatalf("expected listed running record to preserve claim freshness fields, got %+v", records[1].Claim)
	}
	if len(records[1].Operations) != 1 || records[1].Operations[0].ID == "" {
		t.Fatalf("expected active listed records to normalize additive operation ids, got %+v", records[1].Operations)
	}
}

func TestServiceListFiltersStaleActiveApplyClaimsNewestFirst(t *testing.T) {
	store := &memoryStore{
		records: map[string]Record{
			"apply_running_expired_new": {
				ID:             "apply_running_expired_new",
				PlanID:         "plan_expired_new",
				DeploymentName: "api-prod",
				Status:         StatusRunning,
				CreatedAt:      "2026-04-18T08:08:00Z",
				Claim:          &Claim{WorkerID: "worker-expired-new", ClaimedAt: "2026-04-18T08:00:00Z", LastHeartbeatAt: "2026-04-18T08:02:00Z", ExpiresAt: "2026-04-18T08:07:00Z"},
			},
			"apply_queued": {
				ID:             "apply_queued",
				PlanID:         "plan_queued",
				DeploymentName: "worker-prod",
				Status:         StatusQueued,
				CreatedAt:      "2026-04-18T08:07:30Z",
			},
			"apply_running_fresh": {
				ID:             "apply_running_fresh",
				PlanID:         "plan_fresh",
				DeploymentName: "api-prod",
				Status:         StatusRunning,
				CreatedAt:      "2026-04-18T08:07:00Z",
				Claim:          &Claim{WorkerID: "worker-fresh", ClaimedAt: "2026-04-18T08:07:00Z", LastHeartbeatAt: "2026-04-18T08:09:00Z", ExpiresAt: "2026-04-18T08:14:00Z"},
			},
			"apply_running_legacy_stale": {
				ID:             "apply_running_legacy_stale",
				PlanID:         "plan_legacy_stale",
				DeploymentName: "legacy-prod",
				Status:         StatusRunning,
				CreatedAt:      "2026-04-18T08:06:00Z",
				Claim:          &Claim{WorkerID: "worker-legacy", ClaimedAt: "2026-04-18T08:00:00Z"},
				Operations: []Operation{{
					Status:  OperationStatusRunning,
					Phase:   "Promoting",
					Target:  "legacy-prod listener",
					Summary: "Shift production traffic.",
				}},
			},
			"apply_failed_expired": {
				ID:             "apply_failed_expired",
				PlanID:         "plan_failed",
				DeploymentName: "failed-prod",
				Status:         StatusFailed,
				CreatedAt:      "2026-04-18T08:05:00Z",
				Claim:          &Claim{WorkerID: "worker-failed", ClaimedAt: "2026-04-18T08:00:00Z", ExpiresAt: "2026-04-18T08:05:00Z"},
			},
		},
	}
	service := Service{
		Store: store,
		Now:   func() time.Time { return time.Date(2026, 4, 18, 8, 10, 0, 0, time.UTC) },
	}

	records, err := service.List(context.Background(), ListQuery{Status: ListStatusActive, Claim: ListClaimStale})
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	got := make([]string, 0, len(records))
	for _, record := range records {
		got = append(got, record.ID)
		if record.Status != StatusRunning || record.Claim == nil {
			t.Fatalf("expected only running records with stale claims, got %+v", records)
		}
	}
	if want := []string{"apply_running_expired_new", "apply_running_legacy_stale"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected stale active apply records: got %+v want %+v", got, want)
	}
	if len(records[1].Operations) != 1 || records[1].Operations[0].ID == "" {
		t.Fatalf("expected stale listed records to normalize additive operation ids, got %+v", records[1].Operations)
	}
}

func TestServiceListRequiresStore(t *testing.T) {
	_, err := (Service{}).List(context.Background(), ListQuery{Status: StatusQueued})
	if err == nil || !strings.Contains(err.Error(), "apply request store is required") {
		t.Fatalf("expected missing store error, got %v", err)
	}
}

func TestServiceClaimQueuedApplyRequest(t *testing.T) {
	store := &memoryStore{
		records: map[string]Record{
			"apply_queued": {
				ID:             "apply_queued",
				PlanID:         "plan_ready",
				DeploymentName: "api-prod",
				Status:         StatusQueued,
				CreatedAt:      "2026-04-18T08:05:00Z",
				Lifecycle:      Lifecycle{UpdatedAt: "2026-04-18T08:05:00Z", TotalSteps: 2},
				Operations: []Operation{
					{Status: OperationStatusQueued, Phase: "Provisioning", Target: "api-prod deployment", Summary: "Create replacement capacity."},
					{Status: OperationStatusQueued, Phase: "Promoting", Target: "api-prod listener", Summary: "Shift production traffic."},
				},
			},
		},
	}
	service := Service{Store: store, Now: func() time.Time { return time.Date(2026, 4, 18, 8, 10, 0, 0, time.UTC) }}

	claimed, err := service.Claim(context.Background(), "apply_queued", ClaimRequest{WorkerID: "worker-a"})
	if err != nil {
		t.Fatalf("Claim returned error: %v", err)
	}
	if claimed.ID != "apply_queued" || claimed.Status != StatusRunning {
		t.Fatalf("expected stable running claimed record, got %+v", claimed)
	}
	if claimed.Claim == nil || claimed.Claim.WorkerID != "worker-a" || claimed.Claim.ClaimedAt != "2026-04-18T08:10:00Z" || claimed.Claim.LastHeartbeatAt != "2026-04-18T08:10:00Z" || claimed.Claim.ExpiresAt != "2026-04-18T08:15:00Z" {
		t.Fatalf("expected claim metadata, got %+v", claimed.Claim)
	}
	if claimed.Lifecycle.StartedAt != "2026-04-18T08:10:00Z" || claimed.Lifecycle.CurrentStep != 1 || claimed.Lifecycle.CompletedSteps != 0 || claimed.Lifecycle.TotalSteps != 2 {
		t.Fatalf("unexpected claim lifecycle metadata: %+v", claimed.Lifecycle)
	}
	if statuses := []OperationStatus{claimed.Operations[0].Status, claimed.Operations[1].Status}; !reflect.DeepEqual(statuses, []OperationStatus{OperationStatusRunning, OperationStatusQueued}) {
		t.Fatalf("unexpected claimed operation statuses: %+v", claimed.Operations)
	}
	if attempt := claimed.Operations[0].ExecutionAttempt; attempt == nil || attempt.WorkerID != "worker-a" || attempt.Status != OperationStatusRunning || attempt.StartedAt != "2026-04-18T08:10:00Z" || attempt.LastHeartbeatAt != "2026-04-18T08:10:00Z" || attempt.FinishedAt != "" || !strings.Contains(attempt.Summary, "no live AWS mutation") {
		t.Fatalf("expected claim to persist current operation attempt metadata, got %+v", attempt)
	}
	if claimed.Operations[1].ExecutionAttempt != nil {
		t.Fatalf("expected queued operation to omit attempt metadata, got %+v", claimed.Operations[1].ExecutionAttempt)
	}
	if len(claimed.Transitions) != 1 || claimed.Transitions[0].PreviousStatus != StatusQueued || claimed.Transitions[0].Status != StatusRunning || claimed.Transitions[0].Step != 1 {
		t.Fatalf("expected claim to append running transition history, got %+v", claimed.Transitions)
	}
	if !strings.Contains(claimed.Details[0], "worker-a") || !strings.Contains(claimed.Details[0], "no live AWS mutation") {
		t.Fatalf("expected local-safe claim detail, got %+v", claimed.Details)
	}
}

func TestServiceClaimIsIdempotentForSameWorker(t *testing.T) {
	store := &memoryStore{
		records: map[string]Record{
			"apply_claimed": {
				ID:             "apply_claimed",
				PlanID:         "plan_ready",
				DeploymentName: "api-prod",
				Status:         StatusRunning,
				CreatedAt:      "2026-04-18T08:05:00Z",
				Claim:          &Claim{WorkerID: "worker-a", ClaimedAt: "2026-04-18T08:10:00Z", LastHeartbeatAt: "2026-04-18T08:10:00Z", ExpiresAt: "2026-04-18T08:15:00Z"},
				Transitions:    []Transition{{TransitionedAt: "2026-04-18T08:10:00Z", PreviousStatus: StatusQueued, Status: StatusRunning, Step: 1}},
			},
		},
	}
	service := Service{Store: store, Now: func() time.Time { return time.Date(2026, 4, 18, 8, 11, 0, 0, time.UTC) }}

	claimed, err := service.Claim(context.Background(), "apply_claimed", ClaimRequest{WorkerID: "worker-a"})
	if err != nil {
		t.Fatalf("same-worker Claim returned error: %v", err)
	}
	if claimed.Claim == nil || claimed.Claim.ClaimedAt != "2026-04-18T08:10:00Z" || claimed.Claim.LastHeartbeatAt != "2026-04-18T08:10:00Z" || claimed.Claim.ExpiresAt != "2026-04-18T08:15:00Z" || len(claimed.Transitions) != 1 {
		t.Fatalf("expected idempotent claim to return existing lease unchanged, got %+v", claimed)
	}
}

func TestServiceClaimRejectsCompetingWorkerAndInvalidStates(t *testing.T) {
	store := &memoryStore{
		records: map[string]Record{
			"apply_claimed": {
				ID:             "apply_claimed",
				PlanID:         "plan_ready",
				DeploymentName: "api-prod",
				Status:         StatusRunning,
				Claim:          &Claim{WorkerID: "worker-a", ClaimedAt: "2026-04-18T08:10:00Z", LastHeartbeatAt: "2026-04-18T08:10:00Z", ExpiresAt: "2026-04-18T08:15:00Z"},
			},
			"apply_legacy_stale_claimed": {
				ID:             "apply_legacy_stale_claimed",
				PlanID:         "plan_ready",
				DeploymentName: "api-prod",
				Status:         StatusRunning,
				Claim:          &Claim{WorkerID: "worker-a", ClaimedAt: "2026-04-18T08:00:00Z"},
			},
			"apply_failed": {
				ID:             "apply_failed",
				PlanID:         "plan_ready",
				DeploymentName: "api-prod",
				Status:         StatusFailed,
			},
			"apply_failed_claimed_by_same_worker": {
				ID:             "apply_failed_claimed_by_same_worker",
				PlanID:         "plan_ready",
				DeploymentName: "api-prod",
				Status:         StatusFailed,
				Claim:          &Claim{WorkerID: "worker-a", ClaimedAt: "2026-04-18T08:10:00Z"},
			},
		},
	}
	service := Service{Store: store, Now: func() time.Time { return time.Date(2026, 4, 18, 8, 14, 0, 0, time.UTC) }}

	_, err := service.Claim(context.Background(), "apply_claimed", ClaimRequest{WorkerID: "worker-b"})
	if !errors.Is(err, ErrAlreadyClaimed) {
		t.Fatalf("expected ErrAlreadyClaimed for competing worker, got %v", err)
	}
	for _, fragment := range []string{"active claim", "worker worker-a", "last heartbeat 2026-04-18T08:10:00Z", "expires at 2026-04-18T08:15:00Z", "retry with the same workerId for idempotent recovery"} {
		if !strings.Contains(err.Error(), fragment) {
			t.Fatalf("expected active competing-worker detail %q in %v", fragment, err)
		}
	}
	_, err = service.Claim(context.Background(), "apply_legacy_stale_claimed", ClaimRequest{WorkerID: "worker-b"})
	if !errors.Is(err, ErrAlreadyClaimed) {
		t.Fatalf("expected ErrAlreadyClaimed for stale competing worker, got %v", err)
	}
	for _, fragment := range []string{"stale claim", "worker worker-a", "last heartbeat 2026-04-18T08:00:00Z", "expires at 2026-04-18T08:05:00Z", "legacy claim metadata used claimedAt as lastHeartbeatAt", "manual recovery is required before another worker can claim it"} {
		if !strings.Contains(err.Error(), fragment) {
			t.Fatalf("expected stale competing-worker detail %q in %v", fragment, err)
		}
	}
	_, err = service.Claim(context.Background(), "apply_failed", ClaimRequest{WorkerID: "worker-a"})
	if !errors.Is(err, ErrInvalidState) {
		t.Fatalf("expected ErrInvalidState for terminal record, got %v", err)
	}
	_, err = service.Claim(context.Background(), "apply_failed_claimed_by_same_worker", ClaimRequest{WorkerID: "worker-a"})
	if !errors.Is(err, ErrInvalidState) {
		t.Fatalf("expected ErrInvalidState for terminal same-worker claim, got %v", err)
	}
	_, err = service.Claim(context.Background(), "missing", ClaimRequest{WorkerID: "worker-a"})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound for missing record, got %v", err)
	}
	_, err = service.Claim(context.Background(), "apply_claimed", ClaimRequest{})
	if !errors.Is(err, ErrInvalidClaimRequest) || !strings.Contains(err.Error(), "workerId is required") {
		t.Fatalf("expected workerId validation error, got %v", err)
	}
}

func TestServiceClaimAllowsExplicitExpiredClaimTakeover(t *testing.T) {
	store := &memoryStore{
		records: map[string]Record{
			"apply_expired_claim": {
				ID:             "apply_expired_claim",
				PlanID:         "plan_ready",
				DeploymentName: "api-prod",
				Status:         StatusRunning,
				CreatedAt:      "2026-04-18T08:05:00Z",
				Lifecycle:      Lifecycle{UpdatedAt: "2026-04-18T08:10:00Z", StartedAt: "2026-04-18T08:10:00Z", CurrentStep: 2, CompletedSteps: 1, TotalSteps: 2},
				Claim:          &Claim{WorkerID: "worker-a", ClaimedAt: "2026-04-18T08:10:00Z", LastHeartbeatAt: "2026-04-18T08:12:00Z", ExpiresAt: "2026-04-18T08:15:00Z"},
				Transitions:    []Transition{{TransitionedAt: "2026-04-18T08:10:00Z", PreviousStatus: StatusQueued, Status: StatusRunning, Step: 1}},
				Operations: []Operation{
					{Status: OperationStatusSucceeded, Phase: "Provisioning", Target: "api-prod deployment", Summary: "Create replacement capacity."},
					{Status: OperationStatusRunning, Phase: "Promoting", Target: "api-prod listener", Summary: "Shift production traffic."},
				},
			},
		},
	}
	service := Service{Store: store, Now: func() time.Time { return time.Date(2026, 4, 18, 8, 16, 0, 0, time.UTC) }}

	claimed, err := service.Claim(context.Background(), "apply_expired_claim", ClaimRequest{WorkerID: "worker-b", AllowExpiredClaim: true})
	if err != nil {
		t.Fatalf("expired claim takeover returned error: %v", err)
	}
	if claimed.Status != StatusRunning {
		t.Fatalf("expected takeover to keep running status, got %+v", claimed)
	}
	if claimed.Claim == nil || claimed.Claim.WorkerID != "worker-b" || claimed.Claim.ClaimedAt != "2026-04-18T08:16:00Z" || claimed.Claim.LastHeartbeatAt != "2026-04-18T08:16:00Z" || claimed.Claim.ExpiresAt != "2026-04-18T08:21:00Z" {
		t.Fatalf("expected refreshed takeover claim metadata, got %+v", claimed.Claim)
	}
	if claimed.Lifecycle.CurrentStep != 2 || claimed.Lifecycle.CompletedSteps != 1 || claimed.Lifecycle.StartedAt != "2026-04-18T08:10:00Z" || claimed.Lifecycle.UpdatedAt != "2026-04-18T08:16:00Z" {
		t.Fatalf("expected takeover to preserve current step while updating lifecycle timestamp, got %+v", claimed.Lifecycle)
	}
	if statuses := []OperationStatus{claimed.Operations[0].Status, claimed.Operations[1].Status}; !reflect.DeepEqual(statuses, []OperationStatus{OperationStatusSucceeded, OperationStatusRunning}) {
		t.Fatalf("expected takeover to preserve current operation progress, got %+v", claimed.Operations)
	}
	if attempt := claimed.Operations[1].ExecutionAttempt; attempt == nil || attempt.WorkerID != "worker-b" || attempt.StartedAt != "2026-04-18T08:16:00Z" || attempt.LastHeartbeatAt != "2026-04-18T08:16:00Z" || !strings.Contains(attempt.Summary, "recovered") {
		t.Fatalf("expected takeover to persist replacement worker attempt metadata, got %+v", attempt)
	}
	if len(claimed.Transitions) != 2 || claimed.Transitions[1].PreviousStatus != StatusRunning || claimed.Transitions[1].Status != StatusRunning || claimed.Transitions[1].Step != 2 {
		t.Fatalf("expected takeover to append running recovery transition, got %+v", claimed.Transitions)
	}
	if !strings.Contains(claimed.Details[0], "recovered expired local-safe claim") || !strings.Contains(claimed.Details[0], "worker-a") || !strings.Contains(claimed.Details[0], "worker-b") {
		t.Fatalf("expected takeover detail to name old and new workers, got %+v", claimed.Details)
	}
	stored, err := store.Get(context.Background(), "apply_expired_claim")
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if stored.Claim == nil || stored.Claim.WorkerID != "worker-b" {
		t.Fatalf("expected takeover claim to persist, got %+v", stored.Claim)
	}
}

func TestServiceClaimRequiresExpiredPersistedExpiresAtForTakeover(t *testing.T) {
	store := &memoryStore{
		records: map[string]Record{
			"apply_expired_without_opt_in": {
				ID:             "apply_expired_without_opt_in",
				PlanID:         "plan_ready",
				DeploymentName: "api-prod",
				Status:         StatusRunning,
				Claim:          &Claim{WorkerID: "worker-a", ClaimedAt: "2026-04-18T08:10:00Z", LastHeartbeatAt: "2026-04-18T08:12:00Z", ExpiresAt: "2026-04-18T08:15:00Z"},
			},
			"apply_fresh_with_opt_in": {
				ID:             "apply_fresh_with_opt_in",
				PlanID:         "plan_ready",
				DeploymentName: "api-prod",
				Status:         StatusRunning,
				Claim:          &Claim{WorkerID: "worker-a", ClaimedAt: "2026-04-18T08:10:00Z", LastHeartbeatAt: "2026-04-18T08:12:00Z", ExpiresAt: "2026-04-18T08:20:00Z"},
			},
			"apply_legacy_without_expires_at": {
				ID:             "apply_legacy_without_expires_at",
				PlanID:         "plan_ready",
				DeploymentName: "api-prod",
				Status:         StatusRunning,
				Claim:          &Claim{WorkerID: "worker-a", ClaimedAt: "2026-04-18T08:00:00Z"},
			},
		},
	}
	service := Service{Store: store, Now: func() time.Time { return time.Date(2026, 4, 18, 8, 16, 0, 0, time.UTC) }}

	for _, tc := range []struct {
		id      string
		request ClaimRequest
		want    []string
	}{
		{
			id:      "apply_expired_without_opt_in",
			request: ClaimRequest{WorkerID: "worker-b"},
			want:    []string{"stale claim", "retry with allowExpiredClaim=true to explicitly recover the expired local-safe claim"},
		},
		{
			id:      "apply_fresh_with_opt_in",
			request: ClaimRequest{WorkerID: "worker-b", AllowExpiredClaim: true},
			want:    []string{"active claim", "retry with the same workerId for idempotent recovery"},
		},
		{
			id:      "apply_legacy_without_expires_at",
			request: ClaimRequest{WorkerID: "worker-b", AllowExpiredClaim: true},
			want:    []string{"stale claim", "legacy claim metadata used claimedAt as lastHeartbeatAt"},
		},
	} {
		t.Run(tc.id, func(t *testing.T) {
			_, err := service.Claim(context.Background(), tc.id, tc.request)
			if !errors.Is(err, ErrAlreadyClaimed) {
				t.Fatalf("expected ErrAlreadyClaimed, got %v", err)
			}
			for _, fragment := range tc.want {
				if !strings.Contains(err.Error(), fragment) {
					t.Fatalf("expected error fragment %q in %v", fragment, err)
				}
			}
			stored, getErr := store.Get(context.Background(), tc.id)
			if getErr != nil {
				t.Fatalf("Get returned error: %v", getErr)
			}
			if stored.Claim == nil || stored.Claim.WorkerID != "worker-a" {
				t.Fatalf("claim conflict should preserve existing worker, got %+v", stored.Claim)
			}
		})
	}
}

func TestServiceClaimUsesAtomicStoreBoundary(t *testing.T) {
	store := &memoryStore{
		records: map[string]Record{
			"apply_queued": {
				ID:             "apply_queued",
				PlanID:         "plan_ready",
				DeploymentName: "api-prod",
				Status:         StatusQueued,
				CreatedAt:      "2026-04-18T08:05:00Z",
				Lifecycle:      Lifecycle{UpdatedAt: "2026-04-18T08:05:00Z", TotalSteps: 1},
				Operations:     []Operation{{Status: OperationStatusQueued, Phase: "Promoting", Target: "api-prod listener", Summary: "Shift production traffic."}},
			},
		},
	}
	service := Service{Store: store, Now: func() time.Time { return time.Date(2026, 4, 18, 8, 10, 0, 0, time.UTC) }}

	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, workerID := range []string{"worker-a", "worker-b"} {
		wg.Add(1)
		go func(workerID string) {
			defer wg.Done()
			_, err := service.Claim(context.Background(), "apply_queued", ClaimRequest{WorkerID: workerID})
			errs <- err
		}(workerID)
	}
	wg.Wait()
	close(errs)

	successes := 0
	conflicts := 0
	for err := range errs {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, ErrAlreadyClaimed):
			conflicts++
		default:
			t.Fatalf("unexpected claim result: %v", err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("expected one successful claim and one conflict, got successes=%d conflicts=%d", successes, conflicts)
	}
	stored, err := store.Get(context.Background(), "apply_queued")
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if stored.Claim == nil || (stored.Claim.WorkerID != "worker-a" && stored.Claim.WorkerID != "worker-b") {
		t.Fatalf("expected one persisted worker claim, got %+v", stored.Claim)
	}
}

func TestServiceHeartbeatRefreshesCurrentWorkerClaimOnly(t *testing.T) {
	store := &memoryStore{
		records: map[string]Record{
			"apply_claimed": {
				ID:             "apply_claimed",
				PlanID:         "plan_ready",
				DeploymentName: "api-prod",
				Status:         StatusRunning,
				CreatedAt:      "2026-04-18T08:05:00Z",
				Lifecycle:      Lifecycle{UpdatedAt: "2026-04-18T08:10:00Z", StartedAt: "2026-04-18T08:10:00Z", CurrentStep: 1, TotalSteps: 2},
				Claim:          &Claim{WorkerID: "worker-a", ClaimedAt: "2026-04-18T08:10:00Z", LastHeartbeatAt: "2026-04-18T08:10:00Z", ExpiresAt: "2026-04-18T08:15:00Z"},
				Transitions:    []Transition{{TransitionedAt: "2026-04-18T08:10:00Z", PreviousStatus: StatusQueued, Status: StatusRunning, Step: 1}},
				Operations: []Operation{
					{Status: OperationStatusRunning, Phase: "Promoting", Target: "api-prod listener", Summary: "Shift production traffic."},
					{Status: OperationStatusQueued, Phase: "Finalizing", Target: "api-prod alarms", Summary: "Verify final health."},
				},
			},
		},
	}
	service := Service{Store: store, Now: func() time.Time { return time.Date(2026, 4, 18, 8, 12, 30, 0, time.UTC) }}

	heartbeat, err := service.Heartbeat(context.Background(), "apply_claimed", HeartbeatRequest{WorkerID: "worker-a"})
	if err != nil {
		t.Fatalf("Heartbeat returned error: %v", err)
	}
	if heartbeat.ID != "apply_claimed" || heartbeat.Status != StatusRunning {
		t.Fatalf("expected stable running apply record, got %+v", heartbeat)
	}
	if heartbeat.Claim == nil || heartbeat.Claim.WorkerID != "worker-a" || heartbeat.Claim.ClaimedAt != "2026-04-18T08:10:00Z" || heartbeat.Claim.LastHeartbeatAt != "2026-04-18T08:12:30Z" || heartbeat.Claim.ExpiresAt != "2026-04-18T08:17:30Z" {
		t.Fatalf("expected refreshed claim heartbeat metadata, got %+v", heartbeat.Claim)
	}
	if heartbeat.Lifecycle.UpdatedAt != "2026-04-18T08:10:00Z" || heartbeat.Lifecycle.CurrentStep != 1 || heartbeat.Lifecycle.TotalSteps != 2 {
		t.Fatalf("heartbeat must preserve lifecycle state, got %+v", heartbeat.Lifecycle)
	}
	if statuses := []OperationStatus{heartbeat.Operations[0].Status, heartbeat.Operations[1].Status}; !reflect.DeepEqual(statuses, []OperationStatus{OperationStatusRunning, OperationStatusQueued}) {
		t.Fatalf("heartbeat must preserve operation state, got %+v", heartbeat.Operations)
	}
	if attempt := heartbeat.Operations[0].ExecutionAttempt; attempt == nil || attempt.WorkerID != "worker-a" || attempt.LastHeartbeatAt != "2026-04-18T08:12:30Z" || !strings.Contains(attempt.Summary, "refreshed") {
		t.Fatalf("expected heartbeat to update current operation attempt evidence, got %+v", attempt)
	}
	if len(heartbeat.Transitions) != 1 {
		t.Fatalf("heartbeat must not append lifecycle transitions, got %+v", heartbeat.Transitions)
	}
}

func TestServiceHeartbeatRejectsInvalidClaimStates(t *testing.T) {
	store := &memoryStore{
		records: map[string]Record{
			"apply_claimed": {
				ID:     "apply_claimed",
				Status: StatusRunning,
				Claim:  &Claim{WorkerID: "worker-a", ClaimedAt: "2026-04-18T08:10:00Z", LastHeartbeatAt: "2026-04-18T08:10:00Z", ExpiresAt: "2026-04-18T08:15:00Z"},
			},
			"apply_unclaimed": {
				ID:     "apply_unclaimed",
				Status: StatusRunning,
			},
			"apply_failed": {
				ID:     "apply_failed",
				Status: StatusFailed,
				Claim:  &Claim{WorkerID: "worker-a", ClaimedAt: "2026-04-18T08:10:00Z"},
			},
			"apply_stale_claim": {
				ID:     "apply_stale_claim",
				Status: StatusRunning,
				Claim:  &Claim{WorkerID: "worker-a", ClaimedAt: "2026-04-18T08:00:00Z", LastHeartbeatAt: "2026-04-18T08:00:00Z", ExpiresAt: "2026-04-18T08:05:00Z"},
			},
			"apply_legacy_stale_claim": {
				ID:     "apply_legacy_stale_claim",
				Status: StatusRunning,
				Claim:  &Claim{WorkerID: "worker-a", ClaimedAt: "2026-04-18T08:00:00Z"},
			},
		},
	}
	service := Service{Store: store, Now: func() time.Time { return time.Date(2026, 4, 18, 8, 12, 0, 0, time.UTC) }}

	_, err := service.Heartbeat(context.Background(), "apply_claimed", HeartbeatRequest{WorkerID: "worker-b"})
	if !errors.Is(err, ErrAlreadyClaimed) {
		t.Fatalf("expected ErrAlreadyClaimed for wrong worker heartbeat, got %v", err)
	}
	for _, fragment := range []string{"held by worker worker-a", "worker worker-b cannot heartbeat"} {
		if !strings.Contains(err.Error(), fragment) {
			t.Fatalf("expected wrong-worker detail %q in %v", fragment, err)
		}
	}
	_, err = service.Heartbeat(context.Background(), "apply_unclaimed", HeartbeatRequest{WorkerID: "worker-a"})
	if !errors.Is(err, ErrInvalidState) || !strings.Contains(err.Error(), "is not currently claimed") {
		t.Fatalf("expected unclaimed invalid state, got %v", err)
	}
	_, err = service.Heartbeat(context.Background(), "apply_failed", HeartbeatRequest{WorkerID: "worker-a"})
	if !errors.Is(err, ErrInvalidState) || !strings.Contains(err.Error(), "already terminal") {
		t.Fatalf("expected terminal invalid state, got %v", err)
	}
	_, err = service.Heartbeat(context.Background(), "apply_stale_claim", HeartbeatRequest{WorkerID: "worker-a"})
	if !errors.Is(err, ErrInvalidState) || !strings.Contains(err.Error(), "claim for worker worker-a is stale") || !strings.Contains(err.Error(), "expired at 2026-04-18T08:05:00Z") {
		t.Fatalf("expected stale-claim invalid state, got %v", err)
	}
	_, err = service.Heartbeat(context.Background(), "apply_legacy_stale_claim", HeartbeatRequest{WorkerID: "worker-a"})
	if !errors.Is(err, ErrInvalidState) || !strings.Contains(err.Error(), "legacy claim metadata used claimedAt as lastHeartbeatAt") {
		t.Fatalf("expected legacy stale-claim invalid state, got %v", err)
	}
	_, err = service.Heartbeat(context.Background(), "missing", HeartbeatRequest{WorkerID: "worker-a"})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound for missing record, got %v", err)
	}
	_, err = service.Heartbeat(context.Background(), "apply_claimed", HeartbeatRequest{})
	if !errors.Is(err, ErrInvalidClaimRequest) || !strings.Contains(err.Error(), "workerId is required") {
		t.Fatalf("expected workerId validation error, got %v", err)
	}
}

func TestServiceCreateCarriesPlanOperationIDsIntoApplyRows(t *testing.T) {
	store := &memoryStore{}
	plan := planpreview.Record{
		ID:             "plan_ready_with_operation_ids",
		DeploymentName: "api-prod",
		Status:         planpreview.StatusReady,
		CreatedAt:      "2026-04-17T08:00:00Z",
		Operations: []planpreview.Operation{
			{ID: "operation-shift-traffic", Status: "current", Phase: "Promoting", Target: "listener", Summary: "shift traffic"},
		},
		Source: planpreview.SourceContext{DeploymentName: "api-prod", Route: "/v1/overview/deployments/api-prod", Fingerprint: "plan_input"},
	}
	service := Service{
		LoadPlan: func(context.Context, string) (planpreview.Record, error) { return plan, nil },
		Store:    store,
		Now:      func() time.Time { return time.Date(2026, 4, 18, 8, 6, 0, 0, time.UTC) },
	}

	result, err := service.Create(context.Background(), plan.ID)
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if len(result.Record.Operations) != 1 || result.Record.Operations[0].ID != "operation-shift-traffic" {
		t.Fatalf("expected apply operation to preserve plan operation id, got %+v", result.Record.Operations)
	}
}

func TestServiceCreateReturnsNotFoundWhenPlanIsMissing(t *testing.T) {
	service := Service{
		LoadPlan: func(context.Context, string) (planpreview.Record, error) {
			return planpreview.Record{}, planpreview.ErrNotFound
		},
		Store: &memoryStore{},
	}

	_, err := service.Create(context.Background(), "plan_missing")
	if !errors.Is(err, planpreview.ErrNotFound) {
		t.Fatalf("expected plan not found error, got %v", err)
	}
}

func TestServiceCreateRequiresReadyPlan(t *testing.T) {
	service := Service{
		LoadPlan: func(context.Context, string) (planpreview.Record, error) {
			return planpreview.Record{
				ID:             "plan_failed_1234abcd",
				DeploymentName: "api-prod",
				Status:         planpreview.StatusFailed,
			}, nil
		},
		Store: &memoryStore{},
	}

	_, err := service.Create(context.Background(), "plan_failed_1234abcd")
	if !errors.Is(err, ErrPlanNotReady) {
		t.Fatalf("expected ErrPlanNotReady, got %v", err)
	}
}

func TestServiceCreateRejectsStalePlanFingerprintBeforePersisting(t *testing.T) {
	store := &memoryStore{}
	plan := planpreview.Record{
		ID:             "plan_ready_1234abcd",
		DeploymentName: "api-prod",
		Status:         planpreview.StatusReady,
		CreatedAt:      "2026-04-17T08:00:00Z",
		Source:         planpreview.SourceContext{DeploymentName: "api-prod", Route: "/v1/overview/deployments/api-prod", Fingerprint: "plan_input"},
	}
	service := Service{
		LoadPlan: func(context.Context, string) (planpreview.Record, error) {
			return plan, nil
		},
		CurrentFingerprint: func(_ context.Context, deploymentName string) (string, error) {
			if deploymentName != plan.DeploymentName {
				t.Fatalf("unexpected fingerprint lookup: %s", deploymentName)
			}
			return "plan_newer", nil
		},
		Store: store,
		Now:   func() time.Time { return time.Date(2026, 4, 18, 8, 7, 0, 0, time.UTC) },
	}

	_, err := service.Create(context.Background(), plan.ID)
	if !errors.Is(err, ErrSourceFingerprintConflict) {
		t.Fatalf("expected ErrSourceFingerprintConflict, got %v", err)
	}
	if !strings.Contains(err.Error(), "create a new plan preview before applying") {
		t.Fatalf("expected operator-facing refresh guidance, got %v", err)
	}
	if store.saveCalls != 0 || len(store.records) != 0 {
		t.Fatalf("expected stale plan conflict to avoid persistence, got %+v", store.records)
	}
}

func TestServiceCreateReturnsStableRecordIDEncodingErrors(t *testing.T) {
	originalMarshal := marshalStableRecordPayload
	marshalStableRecordPayload = func(any) ([]byte, error) {
		return nil, fmt.Errorf("boom")
	}
	t.Cleanup(func() {
		marshalStableRecordPayload = originalMarshal
	})

	service := Service{
		LoadPlan: func(context.Context, string) (planpreview.Record, error) {
			return planpreview.Record{
				ID:             "plan_ready_1234abcd",
				DeploymentName: "api-prod",
				Status:         planpreview.StatusReady,
			}, nil
		},
		Store: &memoryStore{},
		Now:   func() time.Time { return time.Date(2026, 4, 18, 8, 10, 0, 0, time.UTC) },
	}

	_, err := service.Create(context.Background(), "plan_ready_1234abcd")
	if err == nil || !strings.Contains(err.Error(), "encode stable apply request id") {
		t.Fatalf("expected stable id encoding error, got %v", err)
	}
}

func TestStableRecordIDIgnoresAdditiveOperationsProjection(t *testing.T) {
	base := Record{
		PlanID:         "plan_ready_1234abcd",
		DeploymentName: "api-prod",
		Source: SourceContext{
			PlanID:        "plan_ready_1234abcd",
			Route:         "/v1/plans/plan_ready_1234abcd",
			PlanStatus:    "ready",
			PlanCreatedAt: "2026-04-17T08:00:00Z",
			Fingerprint:   "plan_input",
		},
	}

	first, err := stableRecordID(base)
	if err != nil {
		t.Fatalf("stableRecordID returned error: %v", err)
	}

	base.Operations = []Operation{{
		Status:  OperationStatusPending,
		Phase:   "Provisioning",
		Target:  "api-prod deployment",
		Summary: "Create replacement capacity for api-prod.",
	}}

	second, err := stableRecordID(base)
	if err != nil {
		t.Fatalf("stableRecordID with additive operations returned error: %v", err)
	}
	if first != second {
		t.Fatalf("expected stable apply ID to ignore additive operations projection, got %q and %q", first, second)
	}
}

func TestServiceCreatePersistsAvailableActorFromRequestContext(t *testing.T) {
	store := &memoryStore{}
	plan := planpreview.Record{
		ID:             "plan_ready_1234abcd",
		DeploymentName: "api-prod",
		Status:         planpreview.StatusReady,
		CreatedAt:      "2026-04-17T08:00:00Z",
		Operations: []planpreview.Operation{
			{Status: "pending", Phase: "Provisioning", Target: "api-prod deployment", Summary: "Create replacement capacity for api-prod."},
		},
	}
	service := Service{
		LoadPlan: func(context.Context, string) (planpreview.Record, error) {
			return plan, nil
		},
		Store: store,
		Now:   func() time.Time { return time.Date(2026, 4, 18, 8, 12, 0, 0, time.UTC) },
	}

	ctx := requestactor.WithContext(context.Background(), requestactor.Actor{
		Available: true,
		ID:        "operator-123",
		Name:      "Jane Operator",
		Email:     "jane@example.com",
	})

	result, err := service.Create(ctx, plan.ID)
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if !result.Record.Actor.Available {
		t.Fatalf("expected available actor, got %+v", result.Record.Actor)
	}
	if result.Record.Actor.ID != "operator-123" || result.Record.Actor.Name != "Jane Operator" || result.Record.Actor.Email != "jane@example.com" {
		t.Fatalf("unexpected available actor details: %+v", result.Record.Actor)
	}
	if len(result.Record.Operations) != 1 || result.Record.Operations[0].Status != OperationStatusPending {
		t.Fatalf("expected additive projected operations for available actor record, got %+v", result.Record.Operations)
	}
}

func TestServiceTransitionAdvancesQueuedApplyRequestToRunningAndSucceeded(t *testing.T) {
	store := &memoryStore{
		records: map[string]Record{
			"apply_1234abcd": {
				ID:             "apply_1234abcd",
				PlanID:         "plan_ready_1234abcd",
				DeploymentName: "api-prod",
				Status:         StatusQueued,
				CreatedAt:      "2026-04-18T08:05:00Z",
				Details: []string{
					"Tenet persisted this immutable apply request as queued because TENET_AWS_ENABLE_APPLY=true enabled the local-safe executor handoff path.",
					"Tenet will advance this same applyId locally, but live AWS mutation is still not implemented for this backend slice.",
				},
				Lifecycle: Lifecycle{
					UpdatedAt:  "2026-04-18T08:05:00Z",
					TotalSteps: 2,
				},
				Operations: []Operation{
					{Status: OperationStatusQueued, Phase: "Provisioning", Target: "api-prod deployment", Summary: "Create replacement capacity for api-prod."},
					{Status: OperationStatusQueued, Phase: "Promoting", Target: "api-prod listener", Summary: "Shift production traffic."},
				},
			},
		},
	}
	service := Service{
		Store: store,
		Now:   func() time.Time { return time.Date(2026, 4, 18, 8, 6, 0, 0, time.UTC) },
	}

	running, err := service.Transition(context.Background(), "apply_1234abcd", TransitionRequest{
		Status: StatusRunning,
		Step:   1,
	})
	if err != nil {
		t.Fatalf("Transition to running returned error: %v", err)
	}
	if running.ID != "apply_1234abcd" || running.Status != StatusRunning {
		t.Fatalf("expected stable apply id with running status, got %+v", running)
	}
	if running.Lifecycle.StartedAt != "2026-04-18T08:06:00Z" || running.Lifecycle.UpdatedAt != "2026-04-18T08:06:00Z" || running.Lifecycle.CurrentStep != 1 || running.Lifecycle.CompletedSteps != 0 || running.Lifecycle.TotalSteps != 2 {
		t.Fatalf("unexpected running lifecycle metadata: %+v", running.Lifecycle)
	}
	if statuses := []OperationStatus{running.Operations[0].Status, running.Operations[1].Status}; !reflect.DeepEqual(statuses, []OperationStatus{OperationStatusRunning, OperationStatusQueued}) {
		t.Fatalf("unexpected running operations: %+v", running.Operations)
	}
	if attempt := running.Operations[0].ExecutionAttempt; attempt == nil || attempt.WorkerID != "transition-route" || attempt.Status != OperationStatusRunning || attempt.StartedAt != "2026-04-18T08:06:00Z" || attempt.LastHeartbeatAt != "2026-04-18T08:06:00Z" || !strings.Contains(attempt.Summary, "no live AWS mutation") {
		t.Fatalf("expected running transition to persist operation attempt metadata, got %+v", attempt)
	}

	service.Now = func() time.Time { return time.Date(2026, 4, 18, 8, 7, 0, 0, time.UTC) }
	succeeded, err := service.Transition(context.Background(), "apply_1234abcd", TransitionRequest{
		Status: StatusSucceeded,
	})
	if err != nil {
		t.Fatalf("Transition to succeeded returned error: %v", err)
	}
	if succeeded.ID != "apply_1234abcd" || succeeded.Status != StatusSucceeded {
		t.Fatalf("expected stable apply id with succeeded status, got %+v", succeeded)
	}
	if succeeded.Lifecycle.StartedAt != "2026-04-18T08:06:00Z" || succeeded.Lifecycle.FinishedAt != "2026-04-18T08:07:00Z" || succeeded.Lifecycle.UpdatedAt != "2026-04-18T08:07:00Z" || succeeded.Lifecycle.CurrentStep != 2 || succeeded.Lifecycle.CompletedSteps != 2 || succeeded.Lifecycle.TotalSteps != 2 {
		t.Fatalf("unexpected succeeded lifecycle metadata: %+v", succeeded.Lifecycle)
	}
	if statuses := []OperationStatus{succeeded.Operations[0].Status, succeeded.Operations[1].Status}; !reflect.DeepEqual(statuses, []OperationStatus{OperationStatusSucceeded, OperationStatusSucceeded}) {
		t.Fatalf("unexpected succeeded operations: %+v", succeeded.Operations)
	}
	if attempt := succeeded.Operations[0].ExecutionAttempt; attempt == nil || attempt.Status != OperationStatusSucceeded || attempt.FinishedAt != "2026-04-18T08:07:00Z" || !strings.Contains(attempt.Summary, "not proof of live AWS mutation") {
		t.Fatalf("expected succeeded transition to finish existing attempt metadata, got %+v", attempt)
	}
	if succeeded.Operations[1].ExecutionAttempt != nil {
		t.Fatalf("expected never-started operation to omit attempt metadata, got %+v", succeeded.Operations[1].ExecutionAttempt)
	}
	if len(succeeded.Transitions) != 2 {
		t.Fatalf("expected each successful transition to append audit history, got %+v", succeeded.Transitions)
	}
	if first := succeeded.Transitions[0]; first.TransitionedAt != "2026-04-18T08:06:00Z" || first.PreviousStatus != StatusQueued || first.Status != StatusRunning || first.Step != 1 {
		t.Fatalf("unexpected running transition history: %+v", first)
	}
	if second := succeeded.Transitions[1]; second.TransitionedAt != "2026-04-18T08:07:00Z" || second.PreviousStatus != StatusRunning || second.Status != StatusSucceeded || second.Step != 0 {
		t.Fatalf("unexpected succeeded transition history: %+v", second)
	}
}

func TestServiceTransitionPersistsRequestActorInTransitionHistory(t *testing.T) {
	store := &memoryStore{
		records: map[string]Record{
			"apply_actor": {
				ID:             "apply_actor",
				PlanID:         "plan_actor",
				DeploymentName: "api-prod",
				Status:         StatusQueued,
				CreatedAt:      "2026-04-18T08:05:00Z",
				Lifecycle:      Lifecycle{UpdatedAt: "2026-04-18T08:05:00Z", TotalSteps: 1},
				Operations: []Operation{
					{Status: OperationStatusQueued, Phase: "Promoting", Target: "api-prod listener", Summary: "Shift production traffic."},
				},
			},
		},
	}
	service := Service{
		Store: store,
		Now:   func() time.Time { return time.Date(2026, 4, 18, 8, 9, 0, 0, time.UTC) },
	}
	ctx := requestactor.WithContext(context.Background(), requestactor.Actor{
		Available: true,
		ID:        "operator-123",
		Name:      "Release Operator",
		Email:     "release@example.com",
	})

	result, err := service.Transition(ctx, "apply_actor", TransitionRequest{Status: StatusRunning, Step: 1, Details: []string{"picked up by executor"}})
	if err != nil {
		t.Fatalf("Transition returned error: %v", err)
	}
	if len(result.Transitions) != 1 {
		t.Fatalf("expected one persisted transition event, got %+v", result.Transitions)
	}
	transition := result.Transitions[0]
	if transition.Actor.ID != "operator-123" || transition.Actor.Name != "Release Operator" || transition.Actor.Email != "release@example.com" {
		t.Fatalf("expected transition actor from request context, got %+v", transition.Actor)
	}
	if !reflect.DeepEqual(transition.Details, []string{"picked up by executor"}) {
		t.Fatalf("expected transition details snapshot, got %+v", transition.Details)
	}
}

func TestServiceTransitionMarksRunningStepAsFailed(t *testing.T) {
	store := &memoryStore{
		records: map[string]Record{
			"apply_1234abcd": {
				ID:             "apply_1234abcd",
				PlanID:         "plan_ready_1234abcd",
				DeploymentName: "api-prod",
				Status:         StatusRunning,
				CreatedAt:      "2026-04-18T08:05:00Z",
				Lifecycle: Lifecycle{
					UpdatedAt:   "2026-04-18T08:06:00Z",
					StartedAt:   "2026-04-18T08:06:00Z",
					CurrentStep: 2,
					TotalSteps:  3,
				},
				Operations: []Operation{
					{Status: OperationStatusSucceeded, Phase: "Provisioning", Target: "api-prod deployment", Summary: "Create replacement capacity for api-prod."},
					{Status: OperationStatusRunning, Phase: "Promoting", Target: "api-prod listener", Summary: "Shift production traffic."},
					{Status: OperationStatusQueued, Phase: "Finalizing", Target: "api-prod alarms", Summary: "Verify final health."},
				},
			},
		},
	}
	service := Service{
		Store: store,
		Now:   func() time.Time { return time.Date(2026, 4, 18, 8, 8, 0, 0, time.UTC) },
	}

	result, err := service.Transition(context.Background(), "apply_1234abcd", TransitionRequest{
		Status: StatusFailed,
		Step:   2,
		Error:  "promotion failed",
	})
	if err != nil {
		t.Fatalf("Transition to failed returned error: %v", err)
	}
	if result.Status != StatusFailed || result.Error != "promotion failed" {
		t.Fatalf("expected failed apply record with propagated error, got %+v", result)
	}
	if result.Lifecycle.StartedAt != "2026-04-18T08:06:00Z" || result.Lifecycle.FinishedAt != "2026-04-18T08:08:00Z" || result.Lifecycle.CurrentStep != 2 || result.Lifecycle.CompletedSteps != 1 || result.Lifecycle.TotalSteps != 3 {
		t.Fatalf("unexpected failed lifecycle metadata: %+v", result.Lifecycle)
	}
	if statuses := []OperationStatus{result.Operations[0].Status, result.Operations[1].Status, result.Operations[2].Status}; !reflect.DeepEqual(statuses, []OperationStatus{OperationStatusSucceeded, OperationStatusFailed, OperationStatusQueued}) {
		t.Fatalf("unexpected failed operations: %+v", result.Operations)
	}
	if attempt := result.Operations[1].ExecutionAttempt; attempt == nil || attempt.Status != OperationStatusFailed || attempt.Reason != "promotion failed" || attempt.FinishedAt != "2026-04-18T08:08:00Z" || !strings.Contains(attempt.Summary, "promotion failed") {
		t.Fatalf("expected failed transition to persist terminal operation attempt metadata, got %+v", attempt)
	}
}

func TestServiceTransitionRunningClearsErrorAndReplacesDetails(t *testing.T) {
	store := &memoryStore{
		records: map[string]Record{
			"apply_1234abcd": {
				ID:             "apply_1234abcd",
				PlanID:         "plan_ready_1234abcd",
				DeploymentName: "api-prod",
				Status:         StatusQueued,
				CreatedAt:      "2026-04-18T08:05:00Z",
				Error:          "stale executor error",
				Details:        []string{"old detail"},
				Lifecycle: Lifecycle{
					UpdatedAt:  "2026-04-18T08:05:00Z",
					TotalSteps: 2,
				},
				Operations: []Operation{
					{Status: OperationStatusQueued, Phase: "Provisioning", Target: "api-prod deployment", Summary: "Create replacement capacity for api-prod."},
					{Status: OperationStatusQueued, Phase: "Promoting", Target: "api-prod listener", Summary: "Shift production traffic."},
				},
			},
		},
	}
	service := Service{
		Store: store,
		Now:   func() time.Time { return time.Date(2026, 4, 18, 8, 6, 30, 0, time.UTC) },
	}

	result, err := service.Transition(context.Background(), "apply_1234abcd", TransitionRequest{
		Status:  StatusRunning,
		Step:    1,
		Details: []string{"executor picked up queued apply request"},
	})
	if err != nil {
		t.Fatalf("Transition to running returned error: %v", err)
	}
	if result.ID != "apply_1234abcd" || result.Status != StatusRunning {
		t.Fatalf("expected stable running apply request, got %+v", result)
	}
	if result.Error != "" {
		t.Fatalf("expected running transition to clear error, got %+v", result)
	}
	if !reflect.DeepEqual(result.Details, []string{"executor picked up queued apply request"}) {
		t.Fatalf("expected running transition to replace details, got %+v", result.Details)
	}
	if result.Lifecycle.StartedAt != "2026-04-18T08:06:30Z" || result.Lifecycle.CurrentStep != 1 {
		t.Fatalf("unexpected running lifecycle metadata: %+v", result.Lifecycle)
	}
}

func TestServiceTransitionRejectsInvalidLifecycleUpdates(t *testing.T) {
	store := &memoryStore{
		records: map[string]Record{
			"apply_queued": {
				ID:             "apply_queued",
				PlanID:         "plan_ready_1234abcd",
				DeploymentName: "api-prod",
				Status:         StatusQueued,
				Operations: []Operation{
					{Status: OperationStatusQueued, Phase: "Provisioning", Target: "api-prod deployment", Summary: "Create replacement capacity for api-prod."},
					{Status: OperationStatusQueued, Phase: "Promoting", Target: "api-prod listener", Summary: "Shift production traffic."},
				},
			},
			"apply_done": {
				ID:             "apply_done",
				PlanID:         "plan_ready_1234abcd",
				DeploymentName: "api-prod",
				Status:         StatusSucceeded,
				Operations: []Operation{
					{Status: OperationStatusSucceeded, Phase: "Provisioning", Target: "api-prod deployment", Summary: "Create replacement capacity for api-prod."},
				},
			},
		},
	}
	service := Service{Store: store}

	for _, tc := range []struct {
		name    string
		id      string
		request TransitionRequest
	}{
		{
			name: "unsupported status",
			id:   "apply_queued",
			request: TransitionRequest{
				Status: Status("mystery"),
			},
		},
		{
			name: "running requires valid step",
			id:   "apply_queued",
			request: TransitionRequest{
				Status: StatusRunning,
				Step:   3,
			},
		},
		{
			name: "succeeded rejects partial step",
			id:   "apply_queued",
			request: TransitionRequest{
				Status: StatusSucceeded,
				Step:   1,
			},
		},
		{
			name: "terminal record cannot advance",
			id:   "apply_done",
			request: TransitionRequest{
				Status: StatusRunning,
				Step:   1,
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := service.Transition(context.Background(), tc.id, tc.request)
			if !errors.Is(err, ErrInvalidState) {
				t.Fatalf("expected ErrInvalidState, got %v", err)
			}
		})
	}
}

func TestApplyLifecycleStatusHelpers(t *testing.T) {
	if !StatusQueued.Valid() || !StatusFailed.Valid() || !StatusCancelled.IsTerminal() {
		t.Fatalf("expected queued/failed/cancelled lifecycle helpers to report supported states")
	}
	if StatusRunning.IsTerminal() {
		t.Fatalf("running apply status should not be terminal")
	}
	if Status("mystery").Valid() {
		t.Fatalf("unexpected unknown apply status to validate")
	}
	if !OperationStatusQueued.Valid() || !OperationStatusSucceeded.IsTerminal() {
		t.Fatalf("expected queued/succeeded operation helpers to report supported states")
	}
	if OperationStatusRunning.IsTerminal() {
		t.Fatalf("running apply operation status should not be terminal")
	}
	if OperationStatus("mystery").Valid() {
		t.Fatalf("unexpected unknown apply operation status to validate")
	}
}

func TestNormalizeOperationIDsBackfillsLegacyApplyDuplicateRows(t *testing.T) {
	record := Record{
		ID:             "apply_legacy",
		PlanID:         "plan_legacy",
		DeploymentName: "api-prod",
		Status:         StatusRunning,
		Operations: []Operation{
			{Status: OperationStatusRunning, Phase: "Promoting", Target: "listener (productionListener)", Summary: "Shift traffic."},
			{Status: OperationStatusPending, Phase: "Promoting", Target: "listener (productionListener)", Summary: "Shift traffic."},
		},
	}

	normalized := NormalizeOperationIDs(record)
	if normalized.Operations[0].ID == "" || normalized.Operations[1].ID == "" {
		t.Fatalf("expected legacy apply operation IDs to be backfilled: %+v", normalized.Operations)
	}
	if normalized.Operations[0].ID == normalized.Operations[1].ID {
		t.Fatalf("expected duplicate apply rows to be disambiguated: %+v", normalized.Operations)
	}
	if !strings.HasSuffix(normalized.Operations[0].ID, "-1") || !strings.HasSuffix(normalized.Operations[1].ID, "-2") {
		t.Fatalf("expected ordinal ID suffixes for duplicate apply rows: %+v", normalized.Operations)
	}
	record.Operations[0].ID = normalized.Operations[0].ID
	record.Operations[1].ID = ""
	mixed := NormalizeOperationIDs(record)
	if mixed.Operations[0].ID != normalized.Operations[0].ID || mixed.Operations[1].ID == normalized.Operations[0].ID {
		t.Fatalf("expected existing generated apply IDs to be preserved without collision: %+v", mixed.Operations)
	}
	encoded, err := json.Marshal(normalized)
	if err != nil {
		t.Fatalf("Marshal returned error: %v", err)
	}
	if !strings.Contains(string(encoded), `"id":"op_`) {
		t.Fatalf("expected raw JSON operation id field, got %s", string(encoded))
	}
}

func TestApplyOperationAttemptJSONIsAdditiveAndOmittedWhenLegacy(t *testing.T) {
	legacy := Record{
		ID:             "apply_legacy",
		PlanID:         "plan_legacy",
		DeploymentName: "api-prod",
		Status:         StatusRunning,
		Operations: []Operation{{
			ID:      "op-legacy",
			Status:  OperationStatusRunning,
			Phase:   "Promoting",
			Target:  "listener",
			Summary: "Shift traffic.",
		}},
	}
	legacyJSON, err := json.Marshal(legacy)
	if err != nil {
		t.Fatalf("Marshal legacy returned error: %v", err)
	}
	if strings.Contains(string(legacyJSON), "executionAttempt") {
		t.Fatalf("legacy apply operations must omit absent executionAttempt, got %s", string(legacyJSON))
	}

	legacy.Operations[0].ExecutionAttempt = &OperationExecutionAttempt{
		WorkerID:        "worker-a",
		Status:          OperationStatusRunning,
		StartedAt:       "2026-04-18T08:10:00Z",
		LastHeartbeatAt: "2026-04-18T08:12:00Z",
		Summary:         "Local-safe executor evidence; no live AWS mutation has been attempted.",
	}
	attemptJSON, err := json.Marshal(legacy)
	if err != nil {
		t.Fatalf("Marshal attempt returned error: %v", err)
	}
	for _, fragment := range []string{`"executionAttempt"`, `"workerId":"worker-a"`, `"lastHeartbeatAt":"2026-04-18T08:12:00Z"`, `"summary":"Local-safe executor evidence; no live AWS mutation has been attempted."`} {
		if !strings.Contains(string(attemptJSON), fragment) {
			t.Fatalf("expected raw JSON fragment %s in %s", fragment, string(attemptJSON))
		}
	}
}

func TestDeriveOperationsPreservesPlanOperationIDs(t *testing.T) {
	plan := planpreview.Record{
		ID:             "plan_ready",
		DeploymentName: "api-prod",
		Status:         planpreview.StatusReady,
		Operations: []planpreview.Operation{
			{ID: "op_planrow-1", Status: "current", Phase: "Promoting", Target: "listener (productionListener)", Summary: "Shift traffic."},
		},
	}
	operations := deriveOperations(plan, OperationStatusQueued)
	if len(operations) != 1 || operations[0].ID != "op_planrow-1" {
		t.Fatalf("expected apply projection to preserve plan operation ID, got %+v", operations)
	}
}
