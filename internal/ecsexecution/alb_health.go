package ecsexecution

import (
	"context"
	"fmt"
	"net/netip"

	"github.com/DevopsArtFactory/tenet/internal/applyrequest"
	"github.com/DevopsArtFactory/tenet/internal/domain"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecs/types"
	elb "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	elbtypes "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
)

type TargetHealthClient interface {
	DescribeTargetHealth(context.Context, *elb.DescribeTargetHealthInput, ...func(*elb.Options)) (*elb.DescribeTargetHealthOutput, error)
}
type NativeTargetHealthEvidence struct {
	HealthyTaskARNs   []string
	UnexpectedTargets int
}
type ALBTargetHealth struct {
	ClientFor         func(context.Context, applyrequest.DeployerRoleReadiness) (TargetHealthClient, error)
	CheckCapabilities func(context.Context, applyrequest.DeployerRoleReadiness, []applyrequest.OperationExecutionCapabilityCheck) error
}

// Observe binds current ALB health to the exact sampled ECS task IP/port set.
// It does not count unrelated healthy targets as proof of the reviewed revision.
func (p ALBTargetHealth) Observe(ctx context.Context, intent domain.ECSDeploymentIntent, role applyrequest.DeployerRoleReadiness, group string, tasks []types.Task) (NativeTargetHealthEvidence, error) {
	var empty NativeTargetHealthEvidence
	if err := ctx.Err(); err != nil {
		return empty, err
	}
	if err := verifiedNativeRole(intent, role); err != nil {
		return empty, err
	}
	if p.ClientFor == nil || p.CheckCapabilities == nil || (group != intent.Advanced.Routing.TargetGroupARN && group != intent.Advanced.Routing.AlternateTargetGroupARN) {
		return empty, fmt.Errorf("target health requires reviewed target group and deployer read providers")
	}
	expected := map[string]string{}
	for _, task := range tasks {
		if aws.ToString(task.ClusterArn) != intent.ClusterARN || aws.ToString(task.TaskArn) == "" {
			return empty, fmt.Errorf("target task placement unavailable")
		}
		address, err := nativeTaskIPv4(task, intent.ContainerName)
		if err != nil {
			return empty, err
		}
		if _, exists := expected[address]; exists {
			return empty, fmt.Errorf("sampled native tasks have duplicate IP identities")
		}
		expected[address] = aws.ToString(task.TaskArn)
	}
	if err := p.CheckCapabilities(ctx, role, []applyrequest.OperationExecutionCapabilityCheck{{Action: "elasticloadbalancing:DescribeTargetHealth", ResourceName: "*", RequiredRole: role.RoleARN}}); err != nil {
		return empty, err
	}
	client, err := p.ClientFor(ctx, role)
	if err != nil {
		return empty, err
	}
	if client == nil {
		return empty, fmt.Errorf("target health client unavailable")
	}
	output, err := client.DescribeTargetHealth(ctx, &elb.DescribeTargetHealthInput{TargetGroupArn: aws.String(group)})
	if err != nil {
		return empty, err
	}
	if output == nil {
		return empty, fmt.Errorf("target health evidence unavailable")
	}
	evidence := NativeTargetHealthEvidence{}
	seen := map[string]bool{}
	for _, description := range output.TargetHealthDescriptions {
		if description.Target == nil || description.TargetHealth == nil || description.TargetHealth.State == "" || description.Target.Id == nil || description.Target.Port == nil {
			return empty, fmt.Errorf("ALB returned incomplete target health")
		}
		parsed, err := netip.ParseAddr(*description.Target.Id)
		if err != nil || !parsed.Is4() {
			return empty, fmt.Errorf("initial native target health supports IPv4 IP targets only")
		}
		address := parsed.String()
		key := fmt.Sprintf("%s:%d", address, *description.Target.Port)
		if seen[key] {
			return empty, fmt.Errorf("ALB returned duplicate target health identities")
		}
		seen[key] = true
		task, exists := expected[address]
		if !exists || *description.Target.Port != intent.Advanced.Routing.ContainerPort {
			evidence.UnexpectedTargets++
			continue
		}
		if description.TargetHealth.State == elbtypes.TargetHealthStateEnumHealthy {
			evidence.HealthyTaskARNs = append(evidence.HealthyTaskARNs, task)
		}
	}
	return evidence, nil
}
func nativeTaskIPv4(task types.Task, containerName string) (string, error) {
	addresses := map[string]bool{}
	add := func(value string) {
		if parsed, err := netip.ParseAddr(value); err == nil && parsed.Is4() {
			addresses[parsed.String()] = true
		}
	}
	for _, container := range task.Containers {
		if aws.ToString(container.Name) == containerName {
			for _, network := range container.NetworkInterfaces {
				add(aws.ToString(network.PrivateIpv4Address))
			}
		}
	}
	if len(addresses) == 0 {
		for _, attachment := range task.Attachments {
			if aws.ToString(attachment.Type) == "ElasticNetworkInterface" {
				for _, detail := range attachment.Details {
					if aws.ToString(detail.Name) == "privateIPv4Address" {
						add(aws.ToString(detail.Value))
					}
				}
			}
		}
	}
	if len(addresses) != 1 {
		return "", fmt.Errorf("native target requires one observed task IPv4 address")
	}
	for address := range addresses {
		return address, nil
	}
	return "", fmt.Errorf("native task address unavailable")
}
