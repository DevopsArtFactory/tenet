package applyrequest

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

	"github.com/DevopsArtFactory/tenet/internal/planpreview"
	"github.com/DevopsArtFactory/tenet/internal/requestactor"
)

const (
	disabledError     = "apply execution is disabled"
	claimFreshnessTTL = 5 * time.Minute
)

var (
	marshalStableRecordPayload = json.Marshal
)

// PlanProvider loads one persisted immutable plan preview by ID.
type PlanProvider func(context.Context, string) (planpreview.Record, error)

// CurrentFingerprintProvider loads the current deployment-detail fingerprint for one deployment.
type CurrentFingerprintProvider func(context.Context, string) (string, error)

// DeployerRoleReadinessProvider loads non-mutating deployer-role readiness
// evidence for the deployment targeted by one immutable apply request.
type DeployerRoleReadinessProvider func(context.Context, DeployerRoleTarget) (DeployerRoleReadiness, error)

// Service derives and persists immutable apply requests from ready plan previews.
type Service struct {
	LoadPlan              PlanProvider
	CurrentFingerprint    CurrentFingerprintProvider
	DeployerRoleReadiness DeployerRoleReadinessProvider
	PermissionSimulation  PermissionSimulationProvider
	Store                 Store
	Now                   func() time.Time
	EnableAWSApply        bool
}

// CreateResult captures the stored apply request plus whether it was newly created.
type CreateResult struct {
	Record  Record
	Created bool
}

// Create persists one immutable apply request for a ready plan preview.
func (s Service) Create(ctx context.Context, planID string) (CreateResult, error) {
	if s.LoadPlan == nil {
		return CreateResult{}, fmt.Errorf("apply request plan provider is required")
	}
	if s.Store == nil {
		return CreateResult{}, fmt.Errorf("apply request store is required")
	}

	plan, err := s.LoadPlan(ctx, planID)
	if err != nil {
		return CreateResult{}, err
	}
	plan = planpreview.NormalizeOperationIDs(plan)
	if plan.Status != planpreview.StatusReady {
		return CreateResult{}, fmt.Errorf("%w: %s is %s", ErrPlanNotReady, plan.ID, plan.Status)
	}
	if err := s.validateCurrentFingerprint(ctx, plan); err != nil {
		return CreateResult{}, err
	}

	record := Record{
		PlanID:         plan.ID,
		DeploymentName: plan.DeploymentName,
		Status:         s.initialStatus(),
		CreatedAt:      s.now().UTC().Format(time.RFC3339),
		Actor:          requestactor.FromContext(ctx),
		Error:          s.initialError(),
		Details:        s.initialDetails(),
		Operations:     deriveOperations(plan, s.initialOperationStatus()),
		Source: SourceContext{
			PlanID:        plan.ID,
			Route:         "/v1/plans/" + plan.ID,
			PlanStatus:    string(plan.Status),
			PlanCreatedAt: plan.CreatedAt,
			Fingerprint:   plan.Source.Fingerprint,
		},
	}
	record.Lifecycle = initialLifecycle(record)
	record.ID, err = stableRecordID(record)
	if err != nil {
		return CreateResult{}, err
	}
	readiness := deriveExecutionReadiness(record)
	record.ExecutionReadiness = &readiness

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

func (s Service) validateCurrentFingerprint(ctx context.Context, plan planpreview.Record) error {
	if s.CurrentFingerprint == nil || plan.Source.Fingerprint == "" {
		return nil
	}

	currentFingerprint, err := s.CurrentFingerprint(ctx, plan.DeploymentName)
	if err != nil {
		return err
	}
	if currentFingerprint == plan.Source.Fingerprint {
		return nil
	}

	return fmt.Errorf(
		"%w: reviewed plan %s expects deployment detail fingerprint %s but current fingerprint for %s is %s; create a new plan preview before applying",
		ErrSourceFingerprintConflict,
		plan.ID,
		plan.Source.Fingerprint,
		plan.DeploymentName,
		currentFingerprint,
	)
}

// Get returns one persisted immutable apply request by ID.
func (s Service) Get(ctx context.Context, id string) (Record, error) {
	if s.Store == nil {
		return Record{}, fmt.Errorf("apply request store is required")
	}
	record, err := s.Store.Get(ctx, id)
	if err != nil {
		return Record{}, err
	}
	return NormalizeOperationIDs(record), nil
}

// List returns persisted immutable apply requests matching query.
func (s Service) List(ctx context.Context, query ListQuery) ([]Record, error) {
	if s.Store == nil {
		return nil, fmt.Errorf("apply request store is required")
	}
	records, err := s.Store.List(ctx)
	if err != nil {
		return nil, err
	}
	listed := make([]Record, 0, len(records))
	now := s.now().UTC()
	for _, record := range records {
		if !query.Matches(record, now) {
			continue
		}
		listed = append(listed, NormalizeOperationIDs(record))
	}
	if query.NewestFirst() {
		return listNewestFirst(listed), nil
	}
	return list(listed), nil
}

// NormalizeOperationIDs returns a copy of record with additive backend-authored operation IDs.
// Existing non-empty IDs are preserved so immutable stored records do not need to be rewritten.
func NormalizeOperationIDs(record Record) Record {
	return normalizeExecutionReadiness(record)
}

// Claim records a local-safe executor lease and advances one queued apply request to running.
// The stable immutable apply ID is preserved; no live AWS mutation is attempted.
func (s Service) Claim(ctx context.Context, id string, request ClaimRequest) (Record, error) {
	if s.Store == nil {
		return Record{}, fmt.Errorf("apply request store is required")
	}
	if _, err := normalizeClaimRequest(request); err != nil {
		return Record{}, err
	}
	claimFn := s.claimRecord(ctx, request)
	if claimStore, ok := s.Store.(ClaimStore); ok {
		record, err := claimStore.Claim(ctx, id, request, claimFn)
		if err != nil {
			return Record{}, err
		}
		return NormalizeOperationIDs(record), nil
	}

	record, err := s.Store.Get(ctx, id)
	if err != nil {
		return Record{}, err
	}
	record, err = claimFn(record)
	if err != nil {
		return Record{}, err
	}
	if err := s.Store.Update(ctx, record); err != nil {
		return Record{}, err
	}
	stored, err := s.Store.Get(ctx, id)
	if err != nil {
		return Record{}, err
	}
	return NormalizeOperationIDs(stored), nil
}

func normalizeClaimRequest(request ClaimRequest) (string, error) {
	return normalizeWorkerID(request.WorkerID)
}

func normalizeHeartbeatRequest(request HeartbeatRequest) (string, error) {
	return normalizeWorkerID(request.WorkerID)
}

func normalizeWorkerID(value string) (string, error) {
	workerID := strings.TrimSpace(value)
	if workerID == "" {
		return "", fmt.Errorf("%w: workerId is required", ErrInvalidClaimRequest)
	}
	return workerID, nil
}

func (s Service) claimRecord(ctx context.Context, request ClaimRequest) func(Record) (Record, error) {
	return func(record Record) (Record, error) {
		workerID, err := normalizeClaimRequest(request)
		if err != nil {
			return Record{}, err
		}
		record = NormalizeOperationIDs(record)
		if record.Status != StatusQueued && record.Status != StatusRunning {
			return Record{}, fmt.Errorf("%w: only queued apply requests can be claimed; %s is %s", ErrInvalidState, record.ID, record.Status)
		}
		now := s.now().UTC()
		if record.Claim != nil {
			if record.Claim.WorkerID == workerID && record.Status == StatusRunning {
				return record, nil
			}
			if record.Claim.WorkerID != workerID {
				if request.AllowExpiredClaim {
					if expired, expiresAt := persistedClaimExpiresAtExpired(*record.Claim, now); expired {
						return recoverExpiredClaim(ctx, record, workerID, record.Claim.WorkerID, expiresAt, now), nil
					}
				}
				return Record{}, alreadyClaimedError(record, now)
			}
			return Record{}, fmt.Errorf("%w: only queued apply requests can be claimed; %s is %s", ErrInvalidState, record.ID, record.Status)
		}
		if record.Status != StatusQueued {
			return Record{}, fmt.Errorf("%w: only queued apply requests can be claimed; %s is %s", ErrInvalidState, record.ID, record.Status)
		}

		claimedAt := now.Format(time.RFC3339)
		claim := Claim{
			WorkerID:        workerID,
			ClaimedAt:       claimedAt,
			LastHeartbeatAt: claimedAt,
			ExpiresAt:       now.Add(claimFreshnessTTL).Format(time.RFC3339),
		}
		record.Claim = &claim
		previousStatus := record.Status
		record.Status = StatusRunning
		record.Error = ""
		record.Details = []string{"Executor " + workerID + " claimed this queued apply request for local-safe handoff; no live AWS mutation has been attempted by the claim route."}
		step := firstRunnableStep(record)
		record.Operations = transitionOperations(record.Operations, StatusRunning, step)
		record.Operations = recordOperationAttempt(record.Operations, operationAttemptEvent{
			Status:    StatusRunning,
			Step:      step,
			WorkerID:  workerID,
			Timestamp: claimedAt,
			Summary:   "Local-safe executor " + workerID + " claimed this operation for handoff; no live AWS mutation has been attempted.",
		})
		record.Lifecycle = transitionLifecycle(record.Lifecycle, len(record.Operations), TransitionRequest{Status: StatusRunning, Step: step}, now)
		readiness := deriveExecutionReadiness(record)
		record.ExecutionReadiness = &readiness
		record.Transitions = appendTransition(record.Transitions, Transition{
			TransitionedAt: claim.ClaimedAt,
			PreviousStatus: previousStatus,
			Status:         StatusRunning,
			Step:           step,
			Actor:          requestactor.FromContext(ctx),
			Details:        append([]string(nil), record.Details...),
			Lifecycle:      record.Lifecycle,
		})
		return record, nil
	}
}

func recoverExpiredClaim(ctx context.Context, record Record, workerID string, previousWorkerID string, expiredAt string, now time.Time) Record {
	claimedAt := now.UTC().Format(time.RFC3339)
	claim := Claim{
		WorkerID:        workerID,
		ClaimedAt:       claimedAt,
		LastHeartbeatAt: claimedAt,
		ExpiresAt:       now.UTC().Add(claimFreshnessTTL).Format(time.RFC3339),
	}
	previousStatus := record.Status
	record.Claim = &claim
	record.Status = StatusRunning
	record.Error = ""
	detail := "Executor " + workerID + " recovered expired local-safe claim from worker " + previousWorkerID + " for local-safe handoff"
	if strings.TrimSpace(expiredAt) != "" {
		detail += "; previous claim expired at " + expiredAt
	}
	detail += "; no live AWS mutation has been attempted by the claim route."
	record.Details = []string{detail}
	step := recoveryStep(record)
	record.Operations = transitionOperations(record.Operations, StatusRunning, step)
	record.Operations = recordOperationAttempt(record.Operations, operationAttemptEvent{
		Status:    StatusRunning,
		Step:      step,
		WorkerID:  workerID,
		Timestamp: claimedAt,
		Summary:   "Local-safe executor " + workerID + " recovered this operation from an expired claim; no live AWS mutation has been attempted.",
	})
	record.Lifecycle = transitionLifecycle(record.Lifecycle, len(record.Operations), TransitionRequest{Status: StatusRunning, Step: step}, now)
	readiness := deriveExecutionReadiness(record)
	record.ExecutionReadiness = &readiness
	record.Transitions = appendTransition(record.Transitions, Transition{
		TransitionedAt: claim.ClaimedAt,
		PreviousStatus: previousStatus,
		Status:         StatusRunning,
		Step:           step,
		Actor:          requestactor.FromContext(ctx),
		Details:        append([]string(nil), record.Details...),
		Lifecycle:      record.Lifecycle,
	})
	return record
}

func recoveryStep(record Record) int {
	if len(record.Operations) == 0 {
		return 0
	}
	if record.Lifecycle.CurrentStep >= 1 && record.Lifecycle.CurrentStep <= len(record.Operations) {
		return record.Lifecycle.CurrentStep
	}
	return firstRunnableStep(record)
}

func persistedClaimExpiresAtExpired(claim Claim, now time.Time) (bool, string) {
	expiresAt := strings.TrimSpace(claim.ExpiresAt)
	if expiresAt == "" {
		return false, ""
	}
	parsed, err := time.Parse(time.RFC3339, expiresAt)
	if err != nil {
		return false, expiresAt
	}
	return !parsed.After(now.UTC()), expiresAt
}

func alreadyClaimedError(record Record, now time.Time) error {
	claim := record.Claim
	if claim == nil {
		return fmt.Errorf("%w: %s is already claimed", ErrAlreadyClaimed, record.ID)
	}
	lastHeartbeatAt, heartbeatSource := claim.LastHeartbeatAt, "lastHeartbeatAt"
	if strings.TrimSpace(lastHeartbeatAt) == "" {
		lastHeartbeatAt, heartbeatSource = claim.ClaimedAt, "claimedAt"
	}
	expiresAt := claim.ExpiresAt
	if strings.TrimSpace(expiresAt) == "" {
		if parsed, err := time.Parse(time.RFC3339, lastHeartbeatAt); err == nil {
			expiresAt = parsed.Add(claimFreshnessTTL).UTC().Format(time.RFC3339)
		}
	}
	state := "active claim"
	if expired, _, _ := claimExpired(*claim, now); expired {
		state = "stale claim"
	}
	detail := fmt.Sprintf("%s has %s held by worker %s", record.ID, state, claim.WorkerID)
	if strings.TrimSpace(lastHeartbeatAt) != "" {
		detail += "; last heartbeat " + lastHeartbeatAt
	}
	if strings.TrimSpace(expiresAt) != "" {
		detail += "; expires at " + expiresAt
	}
	if heartbeatSource == "claimedAt" {
		detail += "; legacy claim metadata used claimedAt as lastHeartbeatAt"
	}
	if state == "active claim" {
		detail += "; retry with the same workerId for idempotent recovery or wait for the lease to become stale"
	} else if strings.TrimSpace(claim.ExpiresAt) != "" {
		detail += "; retry with allowExpiredClaim=true to explicitly recover the expired local-safe claim"
	} else {
		detail += "; manual recovery is required before another worker can claim it"
	}
	return fmt.Errorf("%w: %s", ErrAlreadyClaimed, detail)
}

func claimExpired(claim Claim, now time.Time) (bool, string, string) {
	lastHeartbeatAt, source := claim.LastHeartbeatAt, "lastHeartbeatAt"
	if strings.TrimSpace(lastHeartbeatAt) == "" {
		lastHeartbeatAt, source = claim.ClaimedAt, "claimedAt"
	}
	expiresAt := claim.ExpiresAt
	if strings.TrimSpace(expiresAt) == "" {
		if parsed, err := time.Parse(time.RFC3339, lastHeartbeatAt); err == nil {
			expiresAt = parsed.Add(claimFreshnessTTL).UTC().Format(time.RFC3339)
		}
	}
	parsed, err := time.Parse(time.RFC3339, expiresAt)
	if err != nil {
		return false, expiresAt, source
	}
	return !parsed.After(now.UTC()), expiresAt, source
}

// Heartbeat refreshes the current executor lease for a claimed apply request.
// It preserves the stable immutable apply ID and lifecycle/operation state.
func (s Service) Heartbeat(ctx context.Context, id string, request HeartbeatRequest) (Record, error) {
	if s.Store == nil {
		return Record{}, fmt.Errorf("apply request store is required")
	}
	if _, err := normalizeHeartbeatRequest(request); err != nil {
		return Record{}, err
	}
	heartbeatFn := s.heartbeatRecord(request)
	if heartbeatStore, ok := s.Store.(HeartbeatStore); ok {
		record, err := heartbeatStore.Heartbeat(ctx, id, request, heartbeatFn)
		if err != nil {
			return Record{}, err
		}
		return NormalizeOperationIDs(record), nil
	}

	record, err := s.Store.Get(ctx, id)
	if err != nil {
		return Record{}, err
	}
	record, err = heartbeatFn(record)
	if err != nil {
		return Record{}, err
	}
	if err := s.Store.Update(ctx, record); err != nil {
		return Record{}, err
	}
	stored, err := s.Store.Get(ctx, id)
	if err != nil {
		return Record{}, err
	}
	return NormalizeOperationIDs(stored), nil
}

func (s Service) heartbeatRecord(request HeartbeatRequest) func(Record) (Record, error) {
	return func(record Record) (Record, error) {
		workerID, err := normalizeHeartbeatRequest(request)
		if err != nil {
			return Record{}, err
		}
		record = NormalizeOperationIDs(record)
		if record.Status.IsTerminal() {
			return Record{}, fmt.Errorf("%w: %s is already terminal with status %s", ErrInvalidState, record.ID, record.Status)
		}
		if record.Claim == nil {
			return Record{}, fmt.Errorf("%w: %s is not currently claimed", ErrInvalidState, record.ID)
		}
		if record.Claim.WorkerID != workerID {
			return Record{}, fmt.Errorf("%w: %s claim is held by worker %s; worker %s cannot heartbeat it", ErrAlreadyClaimed, record.ID, record.Claim.WorkerID, workerID)
		}
		if record.Status != StatusRunning {
			return Record{}, fmt.Errorf("%w: only running apply requests can heartbeat; %s is %s", ErrInvalidState, record.ID, record.Status)
		}

		now := s.now().UTC()
		if expired, expiresAt, source := claimExpired(*record.Claim, now); expired {
			detail := fmt.Sprintf("%s claim for worker %s is stale", record.ID, workerID)
			if strings.TrimSpace(expiresAt) != "" {
				detail += "; expired at " + expiresAt
			}
			if source == "claimedAt" {
				detail += "; legacy claim metadata used claimedAt as lastHeartbeatAt"
			}
			detail += "; create a new claim after manual stale-lease recovery instead of extending an expired lease"
			return Record{}, fmt.Errorf("%w: %s", ErrInvalidState, detail)
		}
		claim := *record.Claim
		heartbeatAt := now.Format(time.RFC3339)
		claim.LastHeartbeatAt = heartbeatAt
		claim.ExpiresAt = now.Add(claimFreshnessTTL).Format(time.RFC3339)
		record.Claim = &claim
		record.Operations = recordOperationAttempt(record.Operations, operationAttemptEvent{
			Status:        StatusRunning,
			Step:          currentOperationStep(record),
			WorkerID:      workerID,
			Timestamp:     heartbeatAt,
			HeartbeatOnly: true,
			Summary:       "Local-safe executor " + workerID + " refreshed this operation heartbeat; no live AWS mutation has been attempted.",
		})
		readiness := deriveExecutionReadiness(record)
		record.ExecutionReadiness = &readiness
		return record, nil
	}
}

// Transition advances one persisted apply request to a validated lifecycle state without changing
// its stable immutable apply ID.
func (s Service) Transition(ctx context.Context, id string, request TransitionRequest) (Record, error) {
	if s.Store == nil {
		return Record{}, fmt.Errorf("apply request store is required")
	}
	if !request.Status.Valid() {
		return Record{}, fmt.Errorf("%w: unsupported target status %q", ErrInvalidState, request.Status)
	}

	change := func(record Record) (Record, error) {
		for _, operation := range record.Operations {
			if operation.ECSCheckpoint != nil {
				return Record{}, fmt.Errorf("%w: live ECS requests are advanced by their owning executor", ErrInvalidState)
			}
		}
		record = NormalizeOperationIDs(record)
		if err := validateTransition(record, request); err != nil {
			return Record{}, err
		}

		previousStatus := record.Status
		now := s.now()
		record.Status = request.Status
		if request.Error != "" || request.Status == StatusRunning || request.Status == StatusSucceeded {
			record.Error = request.Error
		}
		if request.Details != nil {
			record.Details = append([]string(nil), request.Details...)
		}
		record.Operations = transitionOperations(record.Operations, request.Status, request.Step)
		record.Operations = recordOperationAttempt(record.Operations, operationAttemptEvent{
			Status:    request.Status,
			Step:      request.Step,
			WorkerID:  transitionWorkerID(record),
			Timestamp: now.UTC().Format(time.RFC3339),
			Reason:    transitionAttemptReason(request),
			Summary:   transitionAttemptSummary(record, request),
		})
		record.Lifecycle = transitionLifecycle(record.Lifecycle, len(record.Operations), request, now)
		readiness := deriveExecutionReadiness(record)
		record.ExecutionReadiness = &readiness
		record.Transitions = appendTransition(record.Transitions, Transition{
			TransitionedAt: now.UTC().Format(time.RFC3339),
			PreviousStatus: previousStatus,
			Status:         request.Status,
			Step:           request.Step,
			Actor:          requestactor.FromContext(ctx),
			Error:          request.Error,
			Details:        append([]string(nil), request.Details...),
			Lifecycle:      record.Lifecycle,
		})

		return record, nil
	}
	if store, ok := s.Store.(MutationStore); ok {
		return store.Mutate(ctx, id, change)
	}
	record, err := s.Store.Get(ctx, id)
	if err != nil {
		return Record{}, err
	}
	record, err = change(record)
	if err != nil {
		return Record{}, err
	}
	if err := s.Store.Update(ctx, record); err != nil {
		return Record{}, err
	}
	return s.Get(ctx, id)
}

func (s Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now().UTC()
}

func (s Service) initialStatus() Status {
	if s.EnableAWSApply {
		return StatusQueued
	}
	return StatusFailed
}

func (s Service) initialOperationStatus() OperationStatus {
	if s.EnableAWSApply {
		return OperationStatusQueued
	}
	return OperationStatusPending
}

func (s Service) initialError() string {
	if s.EnableAWSApply {
		return ""
	}
	return disabledError
}

func (s Service) initialDetails() []string {
	if s.EnableAWSApply {
		return []string{
			"Tenet persisted this immutable apply request as queued because TENET_AWS_ENABLE_APPLY=true enabled the local-safe executor handoff path.",
			"Tenet will advance this same applyId locally, but live AWS mutation is still not implemented for this backend slice.",
		}
	}
	return []string{
		"Tenet persisted this immutable apply request in read-only mode and did not contact AWS.",
		"Live execution is intentionally disabled by default for this backend slice.",
	}
}

func stableRecordID(record Record) (string, error) {
	// Operations are intentionally excluded here. The source plan ID already commits to the
	// ordered plan-operation intent, and the apply operations are an additive projection for
	// operator/executor read models. Keeping the hash anchored to the source plan avoids
	// churning immutable apply IDs when this duplicated projection evolves without source-intent
	// changes.
	payload := struct {
		PlanID         string        `json:"planId"`
		DeploymentName string        `json:"deploymentName"`
		Source         SourceContext `json:"source"`
	}{
		PlanID:         record.PlanID,
		DeploymentName: record.DeploymentName,
		Source:         record.Source,
	}
	content, err := marshalStableRecordPayload(payload)
	if err != nil {
		return "", fmt.Errorf("encode stable apply request id: %w", err)
	}
	sum := sha256.Sum256(content)
	return "apply_" + hex.EncodeToString(sum[:8]), nil
}

func deriveOperations(plan planpreview.Record, status OperationStatus) []Operation {
	if len(plan.Operations) == 0 {
		return nil
	}

	operations := make([]Operation, 0, len(plan.Operations))
	for _, operation := range plan.Operations {
		operations = append(operations, Operation{
			Kind:    operation.Kind,
			ECS:     operation.ECS.Clone(),
			ID:      operation.ID,
			Status:  status,
			Phase:   operation.Phase,
			Target:  operation.Target,
			Summary: operation.Summary,
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
		if normalized[i].ECSCheckpoint != nil {
			checkpoint := *normalized[i].ECSCheckpoint
			normalized[i].ECSCheckpoint = &checkpoint
		}
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
		base := applyOperationIDBase(normalized[index])
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

func applyOperationIDBase(operation Operation) string {
	payload := struct {
		Phase   string `json:"phase"`
		Target  string `json:"target"`
		Summary string `json:"summary"`
	}{
		Phase:   operation.Phase,
		Target:  operation.Target,
		Summary: operation.Summary,
	}
	content, _ := json.Marshal(payload)
	sum := sha256.Sum256(content)
	return "op_" + hex.EncodeToString(sum[:6])
}

func initialLifecycle(record Record) Lifecycle {
	lifecycle := Lifecycle{
		UpdatedAt:  record.CreatedAt,
		TotalSteps: len(record.Operations),
	}
	if record.Status.IsTerminal() {
		lifecycle.FinishedAt = record.CreatedAt
		if record.Status == StatusSucceeded && len(record.Operations) > 0 {
			lifecycle.CurrentStep = len(record.Operations)
			lifecycle.CompletedSteps = len(record.Operations)
		}
	}
	return lifecycle
}

func firstRunnableStep(record Record) int {
	if len(record.Operations) == 0 {
		return 0
	}
	return 1
}

func validateTransition(record Record, request TransitionRequest) error {
	if record.Status.IsTerminal() {
		return fmt.Errorf("%w: %s is already terminal with status %s", ErrInvalidState, record.ID, record.Status)
	}
	if !canTransition(record.Status, request.Status) {
		return fmt.Errorf("%w: cannot transition from %s to %s", ErrInvalidState, record.Status, request.Status)
	}

	totalSteps := len(record.Operations)
	if totalSteps == 0 {
		if request.Step != 0 {
			return fmt.Errorf("%w: apply request %s does not define steps", ErrInvalidState, record.ID)
		}
		return nil
	}

	switch request.Status {
	case StatusRunning, StatusFailed, StatusCancelled:
		if request.Step < 1 || request.Step > totalSteps {
			return fmt.Errorf("%w: step must be between 1 and %d", ErrInvalidState, totalSteps)
		}
	case StatusSucceeded:
		if request.Step != 0 && request.Step != totalSteps {
			return fmt.Errorf("%w: succeeded apply request must finish at step %d", ErrInvalidState, totalSteps)
		}
	default:
		if request.Step != 0 {
			return fmt.Errorf("%w: status %s does not accept an explicit step", ErrInvalidState, request.Status)
		}
	}

	return nil
}

func canTransition(from Status, to Status) bool {
	switch from {
	case StatusQueued:
		switch to {
		case StatusRunning, StatusSucceeded, StatusFailed, StatusCancelled:
			return true
		}
	case StatusRunning:
		switch to {
		case StatusRunning, StatusSucceeded, StatusFailed, StatusCancelled:
			return true
		}
	}
	return false
}

func transitionOperations(existing []Operation, status Status, step int) []Operation {
	if len(existing) == 0 {
		return nil
	}

	updated := cloneOperations(existing)
	waiting := waitingOperationStatus(existing)
	switch status {
	case StatusRunning:
		for i := range updated {
			switch {
			case i < step-1:
				updated[i].Status = OperationStatusSucceeded
			case i == step-1:
				updated[i].Status = OperationStatusRunning
			default:
				updated[i].Status = waiting
			}
		}
	case StatusSucceeded:
		for i := range updated {
			updated[i].Status = OperationStatusSucceeded
		}
	case StatusFailed, StatusCancelled:
		terminalStatus := operationTerminalStatus(status)
		for i := range updated {
			switch {
			case i < step-1:
				updated[i].Status = OperationStatusSucceeded
			case i == step-1:
				updated[i].Status = terminalStatus
			default:
				updated[i].Status = waiting
			}
		}
	}
	return updated
}

type operationAttemptEvent struct {
	Status        Status
	Step          int
	WorkerID      string
	Timestamp     string
	Reason        string
	Summary       string
	HeartbeatOnly bool
}

func cloneOperations(existing []Operation) []Operation {
	if len(existing) == 0 {
		return nil
	}
	updated := append([]Operation(nil), existing...)
	for i := range updated {
		updated[i].ECS = updated[i].ECS.Clone()
		if updated[i].ECSCheckpoint != nil {
			checkpoint := *updated[i].ECSCheckpoint
			updated[i].ECSCheckpoint = &checkpoint
		}
		if updated[i].ExecutionAttempt != nil {
			attempt := *updated[i].ExecutionAttempt
			updated[i].ExecutionAttempt = &attempt
		}
	}
	return updated
}

func recordOperationAttempt(existing []Operation, event operationAttemptEvent) []Operation {
	if len(existing) == 0 || strings.TrimSpace(event.Timestamp) == "" {
		return cloneOperations(existing)
	}
	updated := cloneOperations(existing)
	switch event.Status {
	case StatusRunning:
		if event.Step < 1 || event.Step > len(updated) {
			return updated
		}
		attempt := ensureOperationAttempt(updated[event.Step-1].ExecutionAttempt)
		applyAttemptWorker(attempt, event.WorkerID)
		attempt.Status = OperationStatusRunning
		if attempt.StartedAt == "" && !event.HeartbeatOnly {
			attempt.StartedAt = event.Timestamp
		}
		if attempt.StartedAt == "" {
			attempt.StartedAt = event.Timestamp
		}
		attempt.LastHeartbeatAt = event.Timestamp
		attempt.FinishedAt = ""
		attempt.Reason = ""
		attempt.Summary = event.Summary
		updated[event.Step-1].ExecutionAttempt = attempt
	case StatusFailed, StatusCancelled:
		if event.Step < 1 || event.Step > len(updated) {
			return updated
		}
		attempt := ensureOperationAttempt(updated[event.Step-1].ExecutionAttempt)
		applyAttemptWorker(attempt, event.WorkerID)
		if event.Status == StatusFailed {
			attempt.Status = OperationStatusFailed
		} else {
			attempt.Status = OperationStatusCancelled
		}
		if attempt.StartedAt == "" {
			attempt.StartedAt = event.Timestamp
		}
		attempt.LastHeartbeatAt = event.Timestamp
		attempt.FinishedAt = event.Timestamp
		attempt.Reason = event.Reason
		attempt.Summary = event.Summary
		updated[event.Step-1].ExecutionAttempt = attempt
	case StatusSucceeded:
		for i := range updated {
			if updated[i].ExecutionAttempt == nil {
				continue
			}
			attempt := ensureOperationAttempt(updated[i].ExecutionAttempt)
			applyAttemptWorker(attempt, event.WorkerID)
			attempt.Status = OperationStatusSucceeded
			if attempt.StartedAt == "" {
				attempt.StartedAt = event.Timestamp
			}
			attempt.LastHeartbeatAt = event.Timestamp
			attempt.FinishedAt = event.Timestamp
			attempt.Reason = event.Reason
			attempt.Summary = event.Summary
			updated[i].ExecutionAttempt = attempt
		}
	}
	return updated
}

func ensureOperationAttempt(existing *OperationExecutionAttempt) *OperationExecutionAttempt {
	if existing == nil {
		return &OperationExecutionAttempt{}
	}
	attempt := *existing
	return &attempt
}

func applyAttemptWorker(attempt *OperationExecutionAttempt, workerID string) {
	workerID = strings.TrimSpace(workerID)
	if workerID != "" {
		attempt.WorkerID = workerID
	}
}

func currentOperationStep(record Record) int {
	if record.Lifecycle.CurrentStep >= 1 && record.Lifecycle.CurrentStep <= len(record.Operations) {
		return record.Lifecycle.CurrentStep
	}
	for i, operation := range record.Operations {
		if operation.Status == OperationStatusRunning {
			return i + 1
		}
	}
	return 0
}

func transitionWorkerID(record Record) string {
	if record.Claim != nil && strings.TrimSpace(record.Claim.WorkerID) != "" {
		return record.Claim.WorkerID
	}
	return "transition-route"
}

func transitionAttemptReason(request TransitionRequest) string {
	if strings.TrimSpace(request.Error) != "" {
		return strings.TrimSpace(request.Error)
	}
	switch request.Status {
	case StatusSucceeded:
		return "apply request marked succeeded"
	case StatusFailed:
		return "apply request marked failed"
	case StatusCancelled:
		return "apply request marked cancelled"
	default:
		return ""
	}
}

func transitionAttemptSummary(record Record, request TransitionRequest) string {
	workerID := transitionWorkerID(record)
	switch request.Status {
	case StatusRunning:
		return "Apply transition marked this operation running for local-safe executor " + workerID + "; no live AWS mutation has been attempted."
	case StatusSucceeded:
		return "Apply transition marked attempted operations succeeded for local-safe executor " + workerID + "; this metadata is not proof of live AWS mutation."
	case StatusFailed:
		if strings.TrimSpace(request.Error) != "" {
			return "Apply transition marked this operation failed for local-safe executor " + workerID + ": " + strings.TrimSpace(request.Error)
		}
		return "Apply transition marked this operation failed for local-safe executor " + workerID + "; no live AWS mutation success is claimed."
	case StatusCancelled:
		return "Apply transition marked this operation cancelled for local-safe executor " + workerID + "; no live AWS mutation success is claimed."
	default:
		return ""
	}
}

func waitingOperationStatus(existing []Operation) OperationStatus {
	for _, operation := range existing {
		if operation.Status == OperationStatusPending {
			return OperationStatusPending
		}
	}
	return OperationStatusQueued
}

func operationTerminalStatus(status Status) OperationStatus {
	switch status {
	case StatusFailed:
		return OperationStatusFailed
	case StatusCancelled:
		return OperationStatusCancelled
	default:
		return OperationStatusSucceeded
	}
}

func appendTransition(existing []Transition, transition Transition) []Transition {
	transition.Actor = requestactor.Normalize(transition.Actor)
	transition.Details = append([]string(nil), transition.Details...)
	updated := append([]Transition(nil), existing...)
	updated = append(updated, transition)
	return updated
}

func transitionLifecycle(lifecycle Lifecycle, totalSteps int, request TransitionRequest, now time.Time) Lifecycle {
	timestamp := now.UTC().Format(time.RFC3339)
	lifecycle.UpdatedAt = timestamp
	lifecycle.TotalSteps = totalSteps

	switch request.Status {
	case StatusRunning:
		if lifecycle.StartedAt == "" {
			lifecycle.StartedAt = timestamp
		}
		lifecycle.FinishedAt = ""
		lifecycle.CurrentStep = request.Step
		lifecycle.CompletedSteps = completedStepsForStatus(request.Status, request.Step, totalSteps)
	case StatusSucceeded:
		if lifecycle.StartedAt == "" {
			lifecycle.StartedAt = timestamp
		}
		lifecycle.FinishedAt = timestamp
		if totalSteps > 0 {
			lifecycle.CurrentStep = totalSteps
			lifecycle.CompletedSteps = totalSteps
		}
	case StatusFailed, StatusCancelled:
		lifecycle.FinishedAt = timestamp
		if request.Step > 0 {
			lifecycle.CurrentStep = request.Step
			lifecycle.CompletedSteps = completedStepsForStatus(request.Status, request.Step, totalSteps)
		}
	}

	return lifecycle
}

func completedStepsForStatus(status Status, step int, totalSteps int) int {
	switch status {
	case StatusSucceeded:
		return totalSteps
	case StatusRunning, StatusFailed, StatusCancelled:
		if step > 0 {
			return step - 1
		}
	}
	return 0
}
