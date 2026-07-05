package applyrequest

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"time"
)

const (
	defaultExecutorPollInterval = 250 * time.Millisecond
	executorFailedError         = "apply executor handoff stopped before live AWS mutation because this backend slice is still local-safe"
)

// Executor advances queued immutable apply requests through the local-safe executor handoff path.
type Executor struct {
	Service      Service
	PollInterval time.Duration
	WorkerID     string
	Logger       *slog.Logger
}

// Run polls for queued/running apply requests until the context is cancelled.
func (e Executor) Run(ctx context.Context) error {
	interval := e.pollInterval()
	if err := e.RunOnce(ctx); err != nil && ctx.Err() == nil {
		e.log(ctx, slog.LevelWarn, "apply executor scan failed", "error", err)
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := e.RunOnce(ctx); err != nil && ctx.Err() == nil {
				e.log(ctx, slog.LevelWarn, "apply executor scan failed", "error", err)
			}
		}
	}
}

// RunOnce advances each queued/running apply request by at most one lifecycle step.
func (e Executor) RunOnce(ctx context.Context) error {
	if e.Service.Store == nil {
		return errors.New("apply executor store is required")
	}

	records, err := e.Service.Store.List(ctx)
	if err != nil {
		return err
	}
	for _, record := range records {
		if err := e.advance(ctx, record); err != nil {
			return err
		}
	}
	return nil
}

func (e Executor) advance(ctx context.Context, record Record) error {
	if record.Status == StatusQueued {
		updated, err := e.Service.Claim(ctx, record.ID, ClaimRequest{WorkerID: e.workerID()})
		if err != nil {
			if errors.Is(err, ErrInvalidState) || errors.Is(err, ErrNotFound) || errors.Is(err, ErrAlreadyClaimed) {
				e.log(ctx, slog.LevelDebug, "apply executor skipped stale apply claim", "apply_id", record.ID, "status", record.Status, "error", err)
				return nil
			}
			return err
		}
		e.log(ctx, slog.LevelInfo, "apply executor claimed apply request", "apply_id", updated.ID, "from_status", record.Status, "to_status", updated.Status, "worker_id", e.workerID())
		return nil
	}

	var request TransitionRequest
	switch record.Status {
	case StatusRunning:
		request = TransitionRequest{
			Status:  StatusFailed,
			Step:    executorCurrentStep(record),
			Error:   executorFailedError,
			Details: runningFailureDetails(record),
		}
	default:
		return nil
	}

	updated, err := e.Service.Transition(ctx, record.ID, request)
	if err != nil {
		if errors.Is(err, ErrInvalidState) || errors.Is(err, ErrNotFound) {
			e.log(ctx, slog.LevelDebug, "apply executor skipped stale apply request", "apply_id", record.ID, "status", record.Status, "error", err)
			return nil
		}
		return err
	}
	e.log(ctx, slog.LevelInfo, "apply executor advanced apply request", "apply_id", updated.ID, "from_status", record.Status, "to_status", updated.Status)
	return nil
}

func (e Executor) pollInterval() time.Duration {
	if e.PollInterval > 0 {
		return e.PollInterval
	}
	return defaultExecutorPollInterval
}

func (e Executor) workerID() string {
	if e.WorkerID != "" {
		return e.WorkerID
	}
	return "tenet-local-executor"
}

func (e Executor) log(ctx context.Context, level slog.Level, msg string, args ...any) {
	if e.Logger == nil {
		return
	}
	e.Logger.Log(ctx, level, msg, args...)
}

func executorStep(record Record) int {
	if len(record.Operations) == 0 {
		return 0
	}
	return 1
}

func executorCurrentStep(record Record) int {
	if len(record.Operations) == 0 {
		return 0
	}
	if record.Lifecycle.CurrentStep > 0 {
		return record.Lifecycle.CurrentStep
	}
	return 1
}

func queuedHandoffDetails(record Record) []string {
	details := []string{
		"Tenet executor claimed this queued apply request locally and kept the same applyId for executor handoff.",
		"Live AWS mutation is still not implemented for this backend slice, so this handoff remains local-safe only.",
	}
	if len(record.Operations) > 0 {
		details = append(details, "Tenet marked step 1 as running while the local-safe executor stub evaluates the recorded apply sequence.")
	}
	return details
}

func runningFailureDetails(record Record) []string {
	details := []string{
		"Tenet executor advanced this apply request on the same applyId without calling the manual transition API.",
		"Tenet then marked the request failed because live AWS mutation is not implemented for this backend slice yet.",
	}
	if step := executorCurrentStep(record); step > 0 {
		details = append(details, "The local-safe executor stopped at step "+strconv.Itoa(step)+" after recording the handoff state.")
	}
	return details
}
