//go:build awsemulator

package ecsexecution

import (
	"context"

	"github.com/DevopsArtFactory/tenet/internal/applyrequest"
	"github.com/DevopsArtFactory/tenet/internal/reconcile"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
)

// Interrupt after AWS accepts a mutation and before the handler returns to the
// executor. The abandoned checkpoint must be reconciled by a fresh worker.
type emulatorInterrupted string

type emulatorRestartClient struct {
	*ecs.Client
	writes  map[string]int
	restart bool
}

func (c *emulatorRestartClient) accepted(kind string, err error) {
	if err == nil && c.restart && c.writes[kind] == 1 {
		panic(emulatorInterrupted(kind))
	}
}

func (c *emulatorRestartClient) RegisterTaskDefinition(ctx context.Context, in *ecs.RegisterTaskDefinitionInput, opts ...func(*ecs.Options)) (*ecs.RegisterTaskDefinitionOutput, error) {
	kind := string(reconcile.StepRegisterECSTaskDefinition)
	c.writes[kind]++
	out, err := c.Client.RegisterTaskDefinition(ctx, in, opts...)
	c.accepted(kind, err)
	return out, err
}

func (c *emulatorRestartClient) UpdateService(ctx context.Context, in *ecs.UpdateServiceInput, opts ...func(*ecs.Options)) (*ecs.UpdateServiceOutput, error) {
	kind := string(reconcile.StepUpdateECSService)
	if aws.ToString(in.TaskDefinition) == "" {
		kind = applyrequest.NativeCapacityRestoreOperationKind
	}
	c.writes[kind]++
	out, err := c.Client.UpdateService(ctx, in, opts...)
	c.accepted(kind, err)
	return out, err
}

func (c *emulatorRestartClient) StopServiceDeployment(ctx context.Context, in *ecs.StopServiceDeploymentInput, opts ...func(*ecs.Options)) (*ecs.StopServiceDeploymentOutput, error) {
	kind := applyrequest.NativeRollbackOperationKind
	c.writes[kind]++
	out, err := c.Client.StopServiceDeployment(ctx, in, opts...)
	c.accepted(kind, err)
	return out, err
}
