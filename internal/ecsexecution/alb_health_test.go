package ecsexecution

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/DevopsArtFactory/tenet/internal/applyrequest"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecs/types"
	elb "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	elbtypes "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
)

type targetHealthFake struct {
	t       *testing.T
	group   string
	targets []elbtypes.TargetHealthDescription
	calls   int
	err     error
}

func (f *targetHealthFake) DescribeTargetHealth(_ context.Context, input *elb.DescribeTargetHealthInput, _ ...func(*elb.Options)) (*elb.DescribeTargetHealthOutput, error) {
	f.calls++
	if aws.ToString(input.TargetGroupArn) != f.group || input.Targets != nil {
		f.t.Fatal("target health must inspect only the reviewed group and detect extra members")
	}
	if f.err != nil {
		return nil, f.err
	}
	return &elb.DescribeTargetHealthOutput{TargetHealthDescriptions: f.targets}, nil
}
func TestALBTargetHealthBindsExactTaskIPsAndRejectsFalseHealthyCounts(t *testing.T) {
	for _, name := range []string{"healthy", "unhealthy", "missing", "foreign member", "wrong port", "incomplete", "unavailable", "no task IP", "duplicate task IP", "IPv6 target", "attachment fallback", "permission denied"} {
		t.Run(name, func(t *testing.T) {
			_, _, intent, role := albFixture(t)
			f := &targetHealthFake{t: t, group: intent.Advanced.Routing.AlternateTargetGroupARN}
			var tasks []types.Task
			for i := 0; i < 2; i++ {
				ip := fmt.Sprintf("10.0.0.%d", i+1)
				tasks = append(tasks, types.Task{TaskArn: aws.String(fmt.Sprintf("arn:aws:ecs:ap-northeast-2:123456789012:task/production/health-%d", i)), ClusterArn: aws.String(intent.ClusterARN), Containers: []types.Container{{Name: aws.String(intent.ContainerName), NetworkInterfaces: []types.NetworkInterface{{PrivateIpv4Address: aws.String(ip)}}}}})
				f.targets = append(f.targets, elbtypes.TargetHealthDescription{Target: &elbtypes.TargetDescription{Id: aws.String(ip), Port: aws.Int32(intent.Advanced.Routing.ContainerPort)}, TargetHealth: &elbtypes.TargetHealth{State: elbtypes.TargetHealthStateEnumHealthy}})
			}
			probe := ALBTargetHealth{ClientFor: func(context.Context, applyrequest.DeployerRoleReadiness) (TargetHealthClient, error) { return f, nil }, CheckCapabilities: func(_ context.Context, _ applyrequest.DeployerRoleReadiness, checks []applyrequest.OperationExecutionCapabilityCheck) error {
				if len(checks) != 1 || checks[0].Action != "elasticloadbalancing:DescribeTargetHealth" || checks[0].ResourceName != "*" {
					t.Fatal("unexpected target-health capability scope")
				}
				return nil
			}}
			wantHealthy, wantExtra, wantError := 2, 0, false
			switch name {
			case "unhealthy":
				f.targets[0].TargetHealth.State = elbtypes.TargetHealthStateEnumUnhealthy
				wantHealthy = 1
			case "missing":
				f.targets = f.targets[:1]
				wantHealthy = 1
			case "foreign member":
				extra := f.targets[0]
				copy := *extra.Target
				extra.Target = &copy
				extra.Target.Id = aws.String("10.0.0.99")
				f.targets = append(f.targets, extra)
				wantExtra = 1
			case "wrong port":
				f.targets[0].Target.Port = aws.Int32(9999)
				wantHealthy = 1
				wantExtra = 1
			case "incomplete":
				f.targets[0].TargetHealth = nil
				wantError = true
			case "unavailable":
				f.err = errors.New("unavailable")
				wantError = true
			case "no task IP":
				tasks[0].Containers[0].NetworkInterfaces = nil
				wantError = true
			case "duplicate task IP":
				tasks[1].Containers[0].NetworkInterfaces[0].PrivateIpv4Address = aws.String("10.0.0.1")
				wantError = true
			case "IPv6 target":
				f.targets[0].Target.Id = aws.String("2001:db8::1")
				wantError = true
			case "attachment fallback":
				tasks[0].Containers[0].NetworkInterfaces = nil
				tasks[0].Attachments = []types.Attachment{{Type: aws.String("ElasticNetworkInterface"), Details: []types.KeyValuePair{{Name: aws.String("privateIPv4Address"), Value: aws.String("10.0.0.1")}}}}
			case "permission denied":
				probe.CheckCapabilities = func(context.Context, applyrequest.DeployerRoleReadiness, []applyrequest.OperationExecutionCapabilityCheck) error {
					return errors.New("denied")
				}
				wantError = true
			}
			proof, err := probe.Observe(context.Background(), intent, role, f.group, tasks)
			if wantError {
				if err == nil || len(proof.HealthyTaskARNs) != 0 {
					t.Fatal("unavailable target health became healthy")
				}
				return
			}
			if err != nil || len(proof.HealthyTaskARNs) != wantHealthy || proof.UnexpectedTargets != wantExtra {
				t.Fatalf("wrong exact-task target health: %+v %v", proof, err)
			}
		})
	}
}
