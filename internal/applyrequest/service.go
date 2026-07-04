package applyrequest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/DevopsArtFactory/tenet/internal/planpreview"
	"github.com/DevopsArtFactory/tenet/internal/requestactor"
)

const disabledError = "apply execution is disabled"

var (
	marshalStableRecordPayload = json.Marshal
)

// PlanProvider loads one persisted immutable plan preview by ID.
type PlanProvider func(context.Context, string) (planpreview.Record, error)

// CurrentFingerprintProvider loads the current deployment-detail fingerprint for one deployment.
type CurrentFingerprintProvider func(context.Context, string) (string, error)

// Service derives and persists immutable apply requests from ready plan previews.
type Service struct {
	LoadPlan           PlanProvider
	CurrentFingerprint CurrentFingerprintProvider
	Store              Store
	Now                func() time.Time
	EnableAWSApply     bool
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
	for _, record := range records {
		if query.Status != "" && record.Status != query.Status {
			continue
		}
		listed = append(listed, NormalizeOperationIDs(record))
	}
	return list(listed), nil
}

// NormalizeOperationIDs returns a copy of record with additive backend-authored operation IDs.
// Existing non-empty IDs are preserved so immutable stored records do not need to be rewritten.
func NormalizeOperationIDs(record Record) Record {
	record.Operations = normalizeOperationIDs(record.Operations)
	return record
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

	record, err := s.Store.Get(ctx, id)
	if err != nil {
		return Record{}, err
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
	record.Lifecycle = transitionLifecycle(record.Lifecycle, len(record.Operations), request, now)
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

	if err := s.Store.Update(ctx, record); err != nil {
		return Record{}, err
	}
	stored, err := s.Store.Get(ctx, id)
	if err != nil {
		return Record{}, err
	}
	return NormalizeOperationIDs(stored), nil
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

	updated := append([]Operation(nil), existing...)
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
