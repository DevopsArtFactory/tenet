package ecsexecution

import (
	"context"
	"fmt"
	"github.com/DevopsArtFactory/tenet/internal/applyrequest"
)

// StrategyHandler retains Rolling behavior while explicitly selecting native
// advanced handlers. Enablement remains an executor/authorization boundary.
type StrategyHandler struct {
	Rolling, Advanced applyrequest.ECSOperationHandler
}

func (h StrategyHandler) selectHandler(r applyrequest.ECSOperationRequest) (applyrequest.ECSOperationHandler, error) {
	var handler applyrequest.ECSOperationHandler
	switch r.Intent.DeploymentStrategy {
	case "ROLLING":
		handler = h.Rolling
	case "BLUE_GREEN", "CANARY", "LINEAR":
		handler = h.Advanced
	}
	if handler == nil {
		return nil, fmt.Errorf("strategy execution handler unavailable")
	}
	return handler, nil
}
func (h StrategyHandler) Preflight(ctx context.Context, r applyrequest.ECSOperationRequest) error {
	p, e := h.selectHandler(r)
	if e != nil {
		return e
	}
	return p.Preflight(ctx, r)
}
func (h StrategyHandler) Prepare(ctx context.Context, r applyrequest.ECSOperationRequest) (applyrequest.ECSOperationResult, error) {
	p, e := h.selectHandler(r)
	if e != nil {
		return applyrequest.ECSOperationResult{}, e
	}
	if prepare, ok := p.(applyrequest.ECSOperationPreparer); ok {
		return prepare.Prepare(ctx, r)
	}
	return r.PreparedResult.Clone(), nil
}
func (h StrategyHandler) Execute(ctx context.Context, r applyrequest.ECSOperationRequest) (applyrequest.ECSOperationResult, error) {
	p, e := h.selectHandler(r)
	if e != nil {
		return applyrequest.ECSOperationResult{}, e
	}
	return p.Execute(ctx, r)
}
func (h StrategyHandler) Reconcile(ctx context.Context, r applyrequest.ECSOperationRequest) (applyrequest.ECSReconciliation, error) {
	p, e := h.selectHandler(r)
	if e != nil {
		return applyrequest.ECSReconciliation{}, e
	}
	return p.Reconcile(ctx, r)
}

// NativeRollbackOperation adapts the fixed server-authored target to the durable
// operation protocol; caller-provided targets never reach this adapter.
type NativeRollbackOperation struct{ NativeRollback }

func (h NativeRollbackOperation) Preflight(ctx context.Context, r applyrequest.ECSOperationRequest) error {
	_, e := h.Prepare(ctx, r)
	return e
}
func (h NativeRollbackOperation) Prepare(ctx context.Context, r applyrequest.ECSOperationRequest) (applyrequest.ECSOperationResult, error) {
	if r.NativeRollback == nil {
		return applyrequest.ECSOperationResult{}, fmt.Errorf("fixed native recovery approval required")
	}
	return h.NativeRollback.Prepare(ctx, r, *r.NativeRollback)
}
func (h NativeRollbackOperation) Execute(ctx context.Context, r applyrequest.ECSOperationRequest) (applyrequest.ECSOperationResult, error) {
	if r.NativeRollback == nil {
		return applyrequest.ECSOperationResult{}, fmt.Errorf("fixed native recovery approval required")
	}
	return h.NativeRollback.Execute(ctx, r, *r.NativeRollback)
}
func (h NativeRollbackOperation) Reconcile(ctx context.Context, r applyrequest.ECSOperationRequest) (applyrequest.ECSReconciliation, error) {
	if r.NativeRollback == nil || !sameNativeRollbackResult(r.PreparedResult, *r.NativeRollback) {
		return applyrequest.ECSReconciliation{}, fmt.Errorf("persisted native recovery preparation required")
	}
	return h.NativeRollback.Reconcile(ctx, r, *r.NativeRollback)
}
