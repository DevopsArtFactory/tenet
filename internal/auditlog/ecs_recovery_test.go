package auditlog

import (
	"testing"

	"github.com/DevopsArtFactory/tenet/internal/applyrequest"
	"github.com/DevopsArtFactory/tenet/internal/catalog"
)

func TestRecoveryAuditPreservesForwardFailureAndSeparateRecoverySuccess(t *testing.T) {
	forward := applyrequest.Record{ID: "failed-forward", Status: applyrequest.StatusFailed, Error: "ECS deployment failed"}
	recovery := applyrequest.Record{ID: "recovery", Status: applyrequest.StatusSucceeded, Recovery: &applyrequest.ECSRecovery{ApplyID: forward.ID, Automatic: true}}
	failedEntry := projectApply(forward, catalog.DeploymentView{})
	recoveredEntry := projectApply(recovery, catalog.DeploymentView{})
	transition := projectApplyTransition(recovery, applyrequest.Transition{Status: applyrequest.StatusSucceeded}, catalog.DeploymentView{})
	if failedEntry.Metadata.Status != "failed" || failedEntry.Metadata.Error == "" || recoveredEntry.Metadata.Status != "succeeded" || recoveredEntry.Metadata.RecoveryOf != forward.ID || !recoveredEntry.Metadata.RecoveryAutomatic || transition.Metadata.RecoveryOf != forward.ID {
		t.Fatal("recovery masked forward failure or lost audit correlation")
	}
}
