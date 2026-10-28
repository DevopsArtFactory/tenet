package main

import (
	"context"
	"github.com/DevopsArtFactory/tenet/internal/applyrequest"
	"github.com/DevopsArtFactory/tenet/internal/ecsexecution"
)

func nativeALBReadiness() ecsexecution.ALBReadiness {
	return ecsexecution.ALBReadiness{ClientFor: func(ctx context.Context, role applyrequest.DeployerRoleReadiness) (ecsexecution.ALBClient, error) {
		return ecsexecution.DeployerALBClient(ctx, role)
	}, CheckCapabilities: checkECSCapabilities}
}
func newAdvancedServiceHandler() ecsexecution.AdvancedService {
	return ecsexecution.AdvancedService{ClientFor: func(ctx context.Context, role applyrequest.DeployerRoleReadiness) (ecsexecution.NativeServiceClient, error) {
		return ecsexecution.DeployerClient(ctx, role)
	}, CheckCapabilities: checkECSCapabilities, ObserveRouting: nativeALBReadiness().Observe}
}
func newAdvancedObservationHandler() ecsexecution.NativeObservation {
	health := ecsexecution.ALBTargetHealth{ClientFor: func(ctx context.Context, role applyrequest.DeployerRoleReadiness) (ecsexecution.TargetHealthClient, error) {
		return ecsexecution.DeployerALBClient(ctx, role)
	}, CheckCapabilities: checkECSCapabilities}
	return ecsexecution.NativeObservation{ClientFor: func(ctx context.Context, role applyrequest.DeployerRoleReadiness) (ecsexecution.NativeObservationClient, error) {
		return ecsexecution.DeployerClient(ctx, role)
	}, CheckCapabilities: checkECSCapabilities, ObserveRouting: nativeALBReadiness().Observe, ObserveTargetHealth: health.Observe}
}
func newNativeRollbackHandler() ecsexecution.NativeRollback {
	return ecsexecution.NativeRollback{ClientFor: func(ctx context.Context, role applyrequest.DeployerRoleReadiness) (ecsexecution.NativeRollbackClient, error) {
		return ecsexecution.DeployerClient(ctx, role)
	}, CheckCapabilities: checkECSCapabilities}
}
