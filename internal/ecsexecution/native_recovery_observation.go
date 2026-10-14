package ecsexecution

import (
	"context"
	"github.com/DevopsArtFactory/tenet/internal/applyrequest"
)

// PrepareRollback starts a separately persisted observation deadline for the
// fixed source, without replacing the failed/cancelled forward Apply's evidence.
func (h NativeObservation) PrepareRollback(ctx context.Context, r applyrequest.ECSOperationRequest, target NativeRollbackTarget) (applyrequest.ECSOperationResult, error) {
	return h.prepare(ctx, r, &target)
}

// ReconcileRollback proves current source health as well as AWS rollback status.
// It has no mutation API and never turns stop acceptance into recovered health.
func (h NativeObservation) ReconcileRollback(ctx context.Context, r applyrequest.ECSOperationRequest, target NativeRollbackTarget) (applyrequest.ECSReconciliation, error) {
	return h.reconcile(ctx, r, &target)
}
func sameNativeRollbackResult(result applyrequest.ECSOperationResult, t NativeRollbackTarget) bool {
	expected := rollbackResult(t)
	return result.ServiceDeploymentARN == expected.ServiceDeploymentARN && result.ServiceRevisionARN == expected.ServiceRevisionARN && result.PreviousServiceRevisionARN == expected.PreviousServiceRevisionARN && result.TaskDefinitionARN == expected.TaskDefinitionARN && result.PreviousTaskDefinitionARN == expected.PreviousTaskDefinitionARN
}
