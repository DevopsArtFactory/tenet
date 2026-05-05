package resourceinspection

import (
	"context"
	"encoding/base64"
	"errors"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	asgtypes "github.com/aws/aws-sdk-go-v2/service/autoscaling/types"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	ecstypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
	"github.com/aws/smithy-go"

	"github.com/DevopsArtFactory/tenet/internal/resourceaws"
)

func expectedLookupContext(region, controlAccountID, targetAccountID, observerRoleARN string) LookupContext {
	return LookupContext{
		Region:                 region,
		ControlAccountID:       controlAccountID,
		TargetAccountID:        targetAccountID,
		CrossAccount:           controlAccountID != "" && targetAccountID != "" && controlAccountID != targetAccountID,
		ObserverRoleConfigured: observerRoleARN != "",
	}
}

func TestServiceInspect(t *testing.T) {
	t.Run("ecs service returns explicit inspection payload", func(t *testing.T) {
		var gotLookup ECSRequest
		service := NewService(Options{
			InspectECSService: func(ctx context.Context, req ECSRequest) (ECSServiceInspection, error) {
				gotLookup = req
				return ECSServiceInspection{
					ClusterARN:                    "arn:aws:ecs:ap-northeast-2:123456789012:cluster/payments",
					ServiceARN:                    "arn:aws:ecs:ap-northeast-2:123456789012:service/payments/payments-api-prod-service",
					ServiceName:                   "payments-api-prod-service",
					Status:                        "ACTIVE",
					LaunchType:                    "FARGATE",
					DesiredCount:                  2,
					RunningCount:                  2,
					PendingCount:                  0,
					MinimumHealthyPercent:         int32Ptr(100),
					MaximumPercent:                int32Ptr(200),
					HealthCheckGracePeriodSeconds: int32Ptr(45),
					TaskDefinition:                "payments-api:42",
					TaskDefinitionImage:           "ghcr.io/devopsartfactory/payments-api:2026-04-18.2",
					Deployments: []ECSDeploymentSummary{{
						ID:             "ecs-svc/123",
						Status:         "PRIMARY",
						RolloutState:   "COMPLETED",
						TaskDefinition: "payments-api:42",
						DesiredCount:   2,
						RunningCount:   2,
						PendingCount:   0,
						CreatedAt:      "2026-04-18T00:10:00Z",
						UpdatedAt:      "2026-04-18T00:11:00Z",
					}},
					Events: []ECSServiceEvent{{
						CreatedAt: "2026-04-18T00:12:00Z",
						Message:   "service reached steady state",
					}},
				}, nil
			},
		})

		result, err := service.Inspect(context.Background(), Request{
			DeploymentName:   "payments-api-prod",
			ResourceType:     "ecsService",
			ResourceName:     "payments-api-prod-service",
			ControlAccountID: "123456789012",
			TargetAccountID:  "123456789012",
			Region:           "ap-northeast-2",
			ClusterARN:       " arn:aws:ecs:ap-northeast-2:123456789012:cluster/payments ",
		})
		if err != nil {
			t.Fatalf("Inspect returned error: %v", err)
		}
		if !reflect.DeepEqual(gotLookup, ECSRequest{
			DeploymentName:  "payments-api-prod",
			ResourceName:    "payments-api-prod-service",
			ClusterARN:      "arn:aws:ecs:ap-northeast-2:123456789012:cluster/payments",
			Region:          "ap-northeast-2",
			TargetAccountID: "123456789012",
		}) {
			t.Fatalf("unexpected ECS lookup request: %+v", gotLookup)
		}
		want := Result{
			DeploymentName: "payments-api-prod",
			ResourceType:   "ecsService",
			ResourceName:   "payments-api-prod-service",
			ClusterARN:     "arn:aws:ecs:ap-northeast-2:123456789012:cluster/payments",
			LookupContext:  expectedLookupContext("ap-northeast-2", "123456789012", "123456789012", ""),
			Available:      true,
			Source:         SourceECSService,
			ECSService: &ECSServiceInspection{
				ClusterARN:                    "arn:aws:ecs:ap-northeast-2:123456789012:cluster/payments",
				ServiceARN:                    "arn:aws:ecs:ap-northeast-2:123456789012:service/payments/payments-api-prod-service",
				ServiceName:                   "payments-api-prod-service",
				Status:                        "ACTIVE",
				LaunchType:                    "FARGATE",
				DesiredCount:                  2,
				RunningCount:                  2,
				PendingCount:                  0,
				MinimumHealthyPercent:         int32Ptr(100),
				MaximumPercent:                int32Ptr(200),
				HealthCheckGracePeriodSeconds: int32Ptr(45),
				TaskDefinition:                "payments-api:42",
				TaskDefinitionImage:           "ghcr.io/devopsartfactory/payments-api:2026-04-18.2",
				Deployments: []ECSDeploymentSummary{{
					ID:             "ecs-svc/123",
					Status:         "PRIMARY",
					RolloutState:   "COMPLETED",
					TaskDefinition: "payments-api:42",
					DesiredCount:   2,
					RunningCount:   2,
					PendingCount:   0,
					CreatedAt:      "2026-04-18T00:10:00Z",
					UpdatedAt:      "2026-04-18T00:11:00Z",
				}},
				Events: []ECSServiceEvent{{
					CreatedAt: "2026-04-18T00:12:00Z",
					Message:   "service reached steady state",
				}},
			},
		}
		if !reflect.DeepEqual(result, want) {
			t.Fatalf("unexpected ECS inspection result: %+v", result)
		}
	})

	t.Run("ecs task returns explicit task inspection payload", func(t *testing.T) {
		var gotLookup ECSTaskRequest
		service := NewService(Options{
			InspectECSTask: func(ctx context.Context, req ECSTaskRequest) (ECSTaskInspection, error) {
				gotLookup = req
				return ECSTaskInspection{
					ECSTaskSummary: ECSTaskSummary{
						TaskARN:              "arn:aws:ecs:ap-northeast-2:123456789012:task/payments/abc123",
						TaskID:               "abc123",
						ClusterARN:           "arn:aws:ecs:ap-northeast-2:123456789012:cluster/payments",
						ContainerInstanceARN: "arn:aws:ecs:ap-northeast-2:123456789012:container-instance/payments/ci-abc123",
						CapacityProviderName: "payments-ec2",
						AvailabilityZone:     "ap-northeast-2a",
						LastStatus:           "STOPPED",
						DesiredStatus:        "STOPPED",
						StoppedReason:        "Essential container in task exited",
						Containers: []ECSTaskContainerInfo{{
							Name:       "api",
							Image:      "ghcr.io/devopsartfactory/payments-api:broken",
							LastStatus: "STOPPED",
							ExitCode:   int32Ptr(1),
							Reason:     "Error",
							AWSLogs:    &AWSLogsInfo{Group: "/ecs/payments", Stream: "ecs/api/abc123"},
						}},
					},
					TaskDefinition: "payments-api:42",
				}, nil
			},
		})

		result, err := service.Inspect(context.Background(), Request{
			DeploymentName:   "payments-api-prod",
			ResourceType:     "ecsTask",
			ResourceName:     "arn:aws:ecs:ap-northeast-2:123456789012:task/payments/abc123",
			ControlAccountID: "123456789012",
			TargetAccountID:  "123456789012",
			Region:           "ap-northeast-2",
			ClusterARN:       " arn:aws:ecs:ap-northeast-2:123456789012:cluster/payments ",
		})
		if err != nil {
			t.Fatalf("Inspect returned error: %v", err)
		}
		if gotLookup.ResourceName != "arn:aws:ecs:ap-northeast-2:123456789012:task/payments/abc123" || gotLookup.Region != "ap-northeast-2" || gotLookup.ClusterARN != "arn:aws:ecs:ap-northeast-2:123456789012:cluster/payments" {
			t.Fatalf("unexpected ECS task lookup request: %+v", gotLookup)
		}
		if !result.Available || result.Source != SourceECSService || result.ClusterARN != "arn:aws:ecs:ap-northeast-2:123456789012:cluster/payments" || result.ECSTask == nil {
			t.Fatalf("expected available ECS task inspection payload with selected cluster echo, got %+v", result)
		}
		if result.ECSTask.TaskID != "abc123" || len(result.ECSTask.Containers) != 1 || result.ECSTask.Containers[0].AWSLogs == nil {
			t.Fatalf("expected task/container/awslogs evidence, got %+v", result.ECSTask)
		}
		if result.ECSTask.ContainerInstanceARN != "arn:aws:ecs:ap-northeast-2:123456789012:container-instance/payments/ci-abc123" || result.ECSTask.CapacityProviderName != "payments-ec2" || result.ECSTask.AvailabilityZone != "ap-northeast-2a" {
			t.Fatalf("expected ECS placement evidence on task inspection, got %+v", result.ECSTask)
		}
	})

	t.Run("auto scaling group returns explicit inspection payload", func(t *testing.T) {
		var gotLookup AutoScalingGroupRequest
		service := NewService(Options{
			InspectAutoScalingGroup: func(ctx context.Context, req AutoScalingGroupRequest) (AutoScalingInspection, error) {
				gotLookup = req
				return AutoScalingInspection{
					AutoScalingGroupARN:    "arn:aws:autoscaling:ap-northeast-2:123456789012:autoScalingGroup:*:autoScalingGroupName/payments-api-prod-candidate",
					AutoScalingGroupName:   "payments-api-prod-candidate",
					MinSize:                2,
					MaxSize:                8,
					DesiredCapacity:        4,
					DefaultCooldown:        300,
					HealthCheckType:        "EC2",
					HealthCheckGracePeriod: 120,
					LaunchTemplate: &AutoScalingLaunchTemplateSummary{
						ID:           "lt-abc123",
						Name:         "payments-api-prod-candidate",
						Version:      "$Default",
						ImageID:      "ami-abc123",
						InstanceType: "m6i.large",
					},
					MixedInstancesPolicy: &AutoScalingMixedInstancesPolicy{
						LaunchTemplate: &AutoScalingLaunchTemplateSummary{
							ID:      "lt-abc123",
							Name:    "payments-api-prod-candidate",
							Version: "$Default",
						},
						Overrides: []AutoScalingLaunchTemplateOverride{{
							InstanceType:     "m6i.large",
							WeightedCapacity: "2",
							LaunchTemplate: &AutoScalingLaunchTemplateSummary{
								ID:      "lt-override123",
								Name:    "payments-api-prod-candidate-arm",
								Version: "3",
							},
						}},
					},
					TargetGroupARNs: []string{
						"arn:aws:elasticloadbalancing:ap-northeast-2:123456789012:targetgroup/payments-api-prod-candidate/abc123",
					},
					Instances: []AutoScalingGroupInstanceSummary{{
						InstanceID:           "i-abc123",
						AvailabilityZone:     "ap-northeast-2a",
						LifecycleState:       "InService",
						HealthStatus:         "Healthy",
						ProtectedFromScaleIn: true,
					}},
				}, nil
			},
		})

		result, err := service.Inspect(context.Background(), Request{
			DeploymentName:   "payments-api-prod",
			ResourceType:     "candidateAutoScalingGroup",
			ResourceName:     "payments-api-prod-candidate",
			ControlAccountID: "123456789012",
			TargetAccountID:  "123456789012",
			Region:           "ap-northeast-2",
		})
		if err != nil {
			t.Fatalf("Inspect returned error: %v", err)
		}
		if !reflect.DeepEqual(gotLookup, AutoScalingGroupRequest{
			DeploymentName:  "payments-api-prod",
			ResourceName:    "payments-api-prod-candidate",
			Region:          "ap-northeast-2",
			TargetAccountID: "123456789012",
		}) {
			t.Fatalf("unexpected ASG lookup request: %+v", gotLookup)
		}
		if !result.Available || result.Source != SourceAutoScalingGroup || result.AutoScalingGroup == nil {
			t.Fatalf("expected available ASG inspection payload, got %+v", result)
		}
		if result.AutoScalingGroup.LaunchTemplate == nil || result.AutoScalingGroup.LaunchTemplate.Name != "payments-api-prod-candidate" {
			t.Fatalf("expected launch template metadata in ASG inspection payload, got %+v", result.AutoScalingGroup)
		}
		if result.AutoScalingGroup.MixedInstancesPolicy == nil || len(result.AutoScalingGroup.MixedInstancesPolicy.Overrides) != 1 {
			t.Fatalf("expected mixed instances launch template metadata in ASG inspection payload, got %+v", result.AutoScalingGroup)
		}
	})

	t.Run("auto scaling group applies default instance scope", func(t *testing.T) {
		service := NewService(Options{
			InspectAutoScalingGroup: func(ctx context.Context, req AutoScalingGroupRequest) (AutoScalingInspection, error) {
				if req.InstanceLimit != 0 {
					t.Fatalf("expected omitted ASG limit to reach provider as zero, got %d", req.InstanceLimit)
				}
				return AutoScalingInspection{
					AutoScalingGroupName: "payments-api-prod-candidate",
					Instances: []AutoScalingGroupInstanceSummary{
						{InstanceID: "i-011"}, {InstanceID: "i-002"}, {InstanceID: "i-001"}, {InstanceID: "i-003"}, {InstanceID: "i-004"}, {InstanceID: "i-005"},
						{InstanceID: "i-006"}, {InstanceID: "i-007"}, {InstanceID: "i-008"}, {InstanceID: "i-009"}, {InstanceID: "i-010"},
					},
				}, nil
			},
		})

		result, err := service.Inspect(context.Background(), Request{ResourceType: "candidateAutoScalingGroup", ResourceName: "payments-api-prod-candidate"})
		if err != nil {
			t.Fatalf("Inspect returned error: %v", err)
		}
		if result.AutoScalingGroup == nil || result.AutoScalingGroup.InstancesScope == nil {
			t.Fatalf("expected ASG instancesScope, got %+v", result.AutoScalingGroup)
		}
		scope := result.AutoScalingGroup.InstancesScope
		if scope.RequestedLimit != nil || scope.EffectiveLimit != 10 || scope.ReturnedInstanceCount != 10 || scope.TruncationStatus != "truncated" {
			t.Fatalf("unexpected default ASG instancesScope: %+v", scope)
		}
		if got := result.AutoScalingGroup.Instances; len(got) != 10 || got[0].InstanceID != "i-001" || got[9].InstanceID != "i-010" {
			t.Fatalf("expected deterministic capped ASG instances, got %+v", got)
		}
	})

	t.Run("auto scaling group honors explicit instance limit without truncation", func(t *testing.T) {
		service := NewService(Options{
			InspectAutoScalingGroup: func(ctx context.Context, req AutoScalingGroupRequest) (AutoScalingInspection, error) {
				if req.InstanceLimit != 5 {
					t.Fatalf("expected explicit ASG limit 5, got %d", req.InstanceLimit)
				}
				return AutoScalingInspection{
					AutoScalingGroupName: "worker-prod-asg",
					Instances: []AutoScalingGroupInstanceSummary{
						{InstanceID: "i-003"}, {InstanceID: "i-001"}, {InstanceID: "i-002"},
					},
				}, nil
			},
		})

		result, err := service.Inspect(context.Background(), Request{ResourceType: "serviceAutoScalingGroup", ResourceName: "worker-prod-asg", AutoScalingInstanceLimit: 5})
		if err != nil {
			t.Fatalf("Inspect returned error: %v", err)
		}
		if result.AutoScalingGroup == nil || result.AutoScalingGroup.InstancesScope == nil {
			t.Fatalf("expected ASG instancesScope, got %+v", result.AutoScalingGroup)
		}
		scope := result.AutoScalingGroup.InstancesScope
		if scope.RequestedLimit == nil || *scope.RequestedLimit != 5 || scope.EffectiveLimit != 5 || scope.ReturnedInstanceCount != 3 || scope.TruncationStatus != "not_truncated" {
			t.Fatalf("unexpected explicit ASG instancesScope: %+v", scope)
		}
		if got := result.AutoScalingGroup.Instances; len(got) != 3 || got[0].InstanceID != "i-001" || got[2].InstanceID != "i-003" {
			t.Fatalf("expected deterministic ASG instances, got %+v", got)
		}
	})

	t.Run("auto scaling group focuses selected instance", func(t *testing.T) {
		var gotLookup AutoScalingGroupRequest
		service := NewService(Options{
			InspectAutoScalingGroup: func(ctx context.Context, req AutoScalingGroupRequest) (AutoScalingInspection, error) {
				gotLookup = req
				return AutoScalingInspection{
					AutoScalingGroupName: "payments-api-prod-candidate",
					LaunchTemplate:       &AutoScalingLaunchTemplateSummary{Name: "payments-api-prod-candidate", Version: "$Default"},
					Instances: []AutoScalingGroupInstanceSummary{
						{InstanceID: "i-0missingmember"},
						{InstanceID: "i-0abc123def4567890", AvailabilityZone: "ap-northeast-2a", LifecycleState: "InService", HealthStatus: "Healthy"},
					},
				}, nil
			},
		})

		result, err := service.Inspect(context.Background(), Request{
			ResourceType:    "candidateAutoScalingGroup",
			ResourceName:    "payments-api-prod-candidate",
			InstanceID:      " i-0abc123def4567890 ",
			TargetAccountID: "123456789012",
			Region:          "ap-northeast-2",
		})
		if err != nil {
			t.Fatalf("Inspect returned error: %v", err)
		}
		if gotLookup.InstanceID != "i-0abc123def4567890" {
			t.Fatalf("expected trimmed selected instance in provider request, got %+v", gotLookup)
		}
		if !result.Available || result.InstanceID != "i-0abc123def4567890" || result.AutoScalingGroup == nil {
			t.Fatalf("expected selected ASG inspection result, got %+v", result)
		}
		if got := result.AutoScalingGroup.Instances; len(got) != 1 || got[0].InstanceID != "i-0abc123def4567890" {
			t.Fatalf("expected only selected ASG instance, got %+v", got)
		}
		if scope := result.AutoScalingGroup.InstancesScope; scope == nil || scope.RequestedLimit == nil || *scope.RequestedLimit != 1 || scope.EffectiveLimit != 1 || scope.ReturnedInstanceCount != 1 || scope.TruncationStatus != "not_truncated" {
			t.Fatalf("unexpected selected ASG instancesScope: %+v", scope)
		}
	})

	t.Run("auto scaling group selected instance missing returns unavailable", func(t *testing.T) {
		service := NewService(Options{
			InspectAutoScalingGroup: func(ctx context.Context, req AutoScalingGroupRequest) (AutoScalingInspection, error) {
				return AutoScalingInspection{AutoScalingGroupName: "payments-api-prod-candidate", Instances: []AutoScalingGroupInstanceSummary{{InstanceID: "i-0present123456789"}}}, nil
			},
		})

		result, err := service.Inspect(context.Background(), Request{ResourceType: "serviceAutoScalingGroup", ResourceName: "payments-api-prod-asg", InstanceID: "i-0missing12345678", Region: "us-west-2", TargetAccountID: "123456789012"})
		if err != nil {
			t.Fatalf("Inspect returned error: %v", err)
		}
		if result.Available || result.UnavailableReason != resourceaws.UnavailableReasonResourceNotFound || result.InstanceID != "i-0missing12345678" {
			t.Fatalf("expected selected instance unavailable response with echoed instanceId, got %+v", result)
		}
	})

	t.Run("user data bootstrap source reuses auto scaling inspection evidence", func(t *testing.T) {
		var gotLookup AutoScalingGroupRequest
		service := NewService(Options{
			InspectAutoScalingGroup: func(ctx context.Context, req AutoScalingGroupRequest) (AutoScalingInspection, error) {
				gotLookup = req
				return AutoScalingInspection{
					AutoScalingGroupName:          "payments-api-prod-candidate",
					LaunchTemplateBootstrapSource: "file:userdata/payments-api-prod-legacy.sh",
					LaunchTemplate: &AutoScalingLaunchTemplateSummary{
						ID:           "lt-abc123",
						Name:         "payments-api-prod-candidate",
						Version:      "$Default",
						ImageID:      "ami-legacy123",
						InstanceType: "m6i.large",
					},
				}, nil
			},
		})

		result, err := service.Inspect(context.Background(), Request{
			DeploymentName:      "payments-api-prod",
			ResourceType:        "userDataBootstrapSource",
			ResourceName:        "file:userdata/payments-api-prod.sh",
			BackingResourceType: "candidateAutoScalingGroup",
			BackingResourceName: "payments-api-prod-candidate",
			ControlAccountID:    "123456789012",
			TargetAccountID:     "123456789012",
			Region:              "ap-northeast-2",
		})
		if err != nil {
			t.Fatalf("Inspect returned error: %v", err)
		}
		if !reflect.DeepEqual(gotLookup, AutoScalingGroupRequest{
			DeploymentName:  "payments-api-prod",
			ResourceName:    "payments-api-prod-candidate",
			Region:          "ap-northeast-2",
			TargetAccountID: "123456789012",
		}) {
			t.Fatalf("unexpected bootstrap-source lookup request: %+v", gotLookup)
		}
		want := Result{
			DeploymentName:      "payments-api-prod",
			ResourceType:        "userDataBootstrapSource",
			ResourceName:        "file:userdata/payments-api-prod.sh",
			BackingResourceType: "candidateAutoScalingGroup",
			BackingResourceName: "payments-api-prod-candidate",
			LookupContext:       expectedLookupContext("ap-northeast-2", "123456789012", "123456789012", ""),
			Available:           true,
			Source:              SourceAutoScalingGroup,
			UserDataBootstrapSource: &UserDataBootstrapSourceInspection{
				DesiredBootstrapSource:  "file:userdata/payments-api-prod.sh",
				ObservedBootstrapSource: "file:userdata/payments-api-prod-legacy.sh",
				BackingResourceType:     "candidateAutoScalingGroup",
				BackingResourceName:     "payments-api-prod-candidate",
				LaunchTemplate: &AutoScalingLaunchTemplateSummary{
					ID:           "lt-abc123",
					Name:         "payments-api-prod-candidate",
					Version:      "$Default",
					ImageID:      "ami-legacy123",
					InstanceType: "m6i.large",
				},
			},
		}
		if !reflect.DeepEqual(result, want) {
			t.Fatalf("unexpected bootstrap-source inspection result: %+v", result)
		}
	})

	t.Run("user data bootstrap source focuses selected ASG instance", func(t *testing.T) {
		var gotLookup AutoScalingGroupRequest
		service := NewService(Options{
			InspectAutoScalingGroup: func(ctx context.Context, req AutoScalingGroupRequest) (AutoScalingInspection, error) {
				gotLookup = req
				return AutoScalingInspection{
					AutoScalingGroupName:          "payments-api-prod-candidate",
					LaunchTemplateBootstrapSource: "file:userdata/payments-api-prod.sh",
					LaunchTemplate:                &AutoScalingLaunchTemplateSummary{Name: "payments-api-prod-candidate", Version: "$Default"},
					Instances: []AutoScalingGroupInstanceSummary{
						{InstanceID: "i-0oldermember"},
						{InstanceID: "i-0abc123def4567890", AvailabilityZone: "ap-northeast-2a", LifecycleState: "InService", EC2Instance: &EC2InstanceDetail{InstanceState: "running", InstanceType: "m6i.large", ImageID: "ami-abc123"}},
					},
				}, nil
			},
		})

		result, err := service.Inspect(context.Background(), Request{
			DeploymentName:      "payments-api-prod",
			ResourceType:        "userDataBootstrapSource",
			ResourceName:        "file:userdata/payments-api-prod.sh",
			InstanceID:          " i-0abc123def4567890 ",
			BackingResourceType: "candidateAutoScalingGroup",
			BackingResourceName: "payments-api-prod-candidate",
			TargetAccountID:     "123456789012",
			Region:              "ap-northeast-2",
		})
		if err != nil {
			t.Fatalf("Inspect returned error: %v", err)
		}
		if gotLookup.ResourceName != "payments-api-prod-candidate" || gotLookup.InstanceID != "i-0abc123def4567890" {
			t.Fatalf("expected selected bootstrap-source lookup to target backing ASG with trimmed instance, got %+v", gotLookup)
		}
		if !result.Available || result.InstanceID != "i-0abc123def4567890" || result.BackingResourceType != "candidateAutoScalingGroup" || result.BackingResourceName != "payments-api-prod-candidate" {
			t.Fatalf("expected selected bootstrap-source result with selector/backing metadata, got %+v", result)
		}
		if result.UserDataBootstrapSource == nil || result.UserDataBootstrapSource.LaunchTemplate == nil {
			t.Fatalf("expected bootstrap-source inspection body to preserve launch-template evidence, got %+v", result.UserDataBootstrapSource)
		}
		selected := result.UserDataBootstrapSource.SelectedInstance
		if selected == nil || selected.InstanceID != "i-0abc123def4567890" || selected.EC2Instance == nil || selected.EC2Instance.InstanceType != "m6i.large" || selected.EC2Instance.ImageID != "ami-abc123" {
			t.Fatalf("expected bootstrap-source body to include selected ASG member EC2 detail, got %+v", result.UserDataBootstrapSource)
		}
	})

	t.Run("user data bootstrap source selected instance missing returns unavailable with backing metadata", func(t *testing.T) {
		service := NewService(Options{
			InspectAutoScalingGroup: func(ctx context.Context, req AutoScalingGroupRequest) (AutoScalingInspection, error) {
				return AutoScalingInspection{AutoScalingGroupName: "payments-api-prod-candidate", Instances: []AutoScalingGroupInstanceSummary{{InstanceID: "i-0present123456789"}}}, nil
			},
		})

		result, err := service.Inspect(context.Background(), Request{
			ResourceType:        "userDataBootstrapSource",
			ResourceName:        "file:userdata/payments-api-prod.sh",
			InstanceID:          "i-0missing12345678",
			BackingResourceType: "candidateAutoScalingGroup",
			BackingResourceName: "payments-api-prod-candidate",
			TargetAccountID:     "123456789012",
			Region:              "ap-northeast-2",
		})
		if err != nil {
			t.Fatalf("Inspect returned error: %v", err)
		}
		if result.Available || result.UnavailableReason != resourceaws.UnavailableReasonResourceNotFound || result.InstanceID != "i-0missing12345678" {
			t.Fatalf("expected selected bootstrap-source unavailable response with echoed instanceId, got %+v", result)
		}
		if result.BackingResourceType != "candidateAutoScalingGroup" || result.BackingResourceName != "payments-api-prod-candidate" {
			t.Fatalf("expected selected bootstrap-source unavailable response to preserve backing metadata, got %+v", result)
		}
	})

	t.Run("traffic routing resources return explicit summary payloads when AWS inspection is disabled", func(t *testing.T) {
		service := NewService(Options{})
		for _, tc := range []struct {
			resourceType string
			resourceName string
			displayName  string
			summary      string
		}{
			{
				resourceType: "codeDeployApplication",
				resourceName: "payments-api-staging-codedeploy-app",
				displayName:  "CodeDeploy application",
				summary:      "CodeDeploy application coordinating ECS traffic shifting (style: linear; qualifiers: min healthy 80% and warmup 45s)",
			},
			{
				resourceType: "codeDeployDeploymentGroup",
				resourceName: "payments-api-staging-codedeploy-group",
				displayName:  "CodeDeploy deployment group",
				summary:      "CodeDeploy deployment group Tenet targets for ECS rollout (style: linear; qualifiers: min healthy 80% and warmup 45s)",
			},
			{
				resourceType: "productionListener",
				resourceName: "payments-api-staging-prod-listener",
				displayName:  "Production listener",
				summary:      "Production listener CodeDeploy shifts during ECS traffic shifting (style: linear; qualifiers: min healthy 80% and warmup 45s)",
			},
			{
				resourceType: "testListener",
				resourceName: "payments-api-staging-test-listener",
				displayName:  "Test listener",
				summary:      "Test listener CodeDeploy uses to validate the replacement ECS task set before promotion (style: linear; qualifiers: min healthy 80% and warmup 45s)",
			},
			{
				resourceType: "trafficShiftPlan",
				resourceName: "payments-api-prod-weighted-routing",
				displayName:  "Traffic shift plan",
				summary:      "Weighted routing plan Tenet follows to shift a canary slice of production traffic from the active target group to the candidate target group before full promotion while respecting warmup 90s",
			},
			{
				resourceType: "activeTargetGroup",
				resourceName: "payments-api-prod-active",
				displayName:  "Active target group",
				summary:      "Currently active target group receiving production traffic",
			},
			{
				resourceType: "candidateTargetGroup",
				resourceName: "payments-api-prod-candidate",
				displayName:  "Candidate target group",
				summary:      "Candidate target group Tenet will register and warm before traffic shifting",
			},
		} {
			result, err := service.Inspect(context.Background(), Request{
				DeploymentName:      "payments-api-prod",
				ResourceType:        tc.resourceType,
				ResourceName:        tc.resourceName,
				ResourceDescription: tc.summary,
				ControlAccountID:    "111111111111",
				TargetAccountID:     "222222222222",
				Region:              "ap-northeast-2",
			})
			if err != nil {
				t.Fatalf("Inspect returned error for %s/%s: %v", tc.resourceType, tc.resourceName, err)
			}
			want := Result{
				DeploymentName: "payments-api-prod",
				ResourceType:   tc.resourceType,
				ResourceName:   tc.resourceName,
				LookupContext:  expectedLookupContext("ap-northeast-2", "111111111111", "222222222222", ""),
				Available:      true,
				Source:         SourceRolloutSummary,
				TrafficRoutingResource: &TrafficRoutingInspection{
					Kind:           tc.resourceType,
					DisplayName:    tc.displayName,
					InspectionMode: "summary_only",
					Summary:        tc.summary,
				},
			}
			if !reflect.DeepEqual(result, want) {
				t.Fatalf("unexpected traffic routing inspection result for %s/%s: %+v", tc.resourceType, tc.resourceName, result)
			}
		}
	})

	t.Run("traffic routing resources return live inspection payloads when provider is available", func(t *testing.T) {
		type call struct {
			DeploymentName      string
			ResourceType        string
			ResourceName        string
			ResourceDescription string
			Region              string
			TargetAccountID     string
			ObserverRoleARN     string
		}
		var calls []call
		service := NewService(Options{
			InspectTrafficRoutingResource: func(ctx context.Context, req TrafficRoutingRequest) (TrafficRoutingInspection, error) {
				calls = append(calls, call(req))
				switch req.ResourceType {
				case "codeDeployApplication":
					return TrafficRoutingInspection{
						Kind:           req.ResourceType,
						DisplayName:    "CodeDeploy application",
						InspectionMode: "live",
						Summary:        req.ResourceDescription,
						CodeDeployApplication: &CodeDeployApplicationInspection{
							ApplicationName:  "payments-api-prod-codedeploy-app",
							ApplicationID:    "a-1234567890",
							ComputePlatform:  "ECS",
							LinkedToGitHub:   true,
							DeploymentGroups: []string{"payments-api-prod-codedeploy-group", "payments-api-prod-codedeploy-group-canary"},
						},
					}, nil
				case "codeDeployDeploymentGroup":
					return TrafficRoutingInspection{
						Kind:           req.ResourceType,
						DisplayName:    "CodeDeploy deployment group",
						InspectionMode: "live",
						Summary:        req.ResourceDescription,
						CodeDeployDeploymentGroup: &CodeDeployDeploymentGroupInspection{
							ApplicationName:       "payments-api-prod-codedeploy-app",
							DeploymentGroupName:   "payments-api-prod-codedeploy-group",
							DeploymentConfigName:  "CodeDeployDefault.ECSLinear10PercentEvery1Minutes",
							ServiceRoleARN:        "arn:aws:iam::123456789012:role/TenetCodeDeployRole",
							DeploymentType:        "BLUE_GREEN",
							DeploymentOption:      "WITH_TRAFFIC_CONTROL",
							ECSServices:           []CodeDeployECSServiceBinding{{ClusterName: "payments-prod", ServiceName: "payments-api-prod-service"}},
							TargetGroups:          []string{"payments-api-prod-blue", "payments-api-prod-green"},
							ProductionListenerARN: "arn:aws:elasticloadbalancing:ap-northeast-2:123456789012:listener/app/payments/abc123/prod456",
							TestListenerARN:       "arn:aws:elasticloadbalancing:ap-northeast-2:123456789012:listener/app/payments/abc123/test789",
						},
					}, nil
				case "productionListener":
					return TrafficRoutingInspection{
						Kind:           req.ResourceType,
						DisplayName:    "Production listener",
						InspectionMode: "live",
						Summary:        req.ResourceDescription,
						Listener: &ListenerInspection{
							ListenerARN:     "arn:aws:elasticloadbalancing:ap-northeast-2:123456789012:listener/app/payments/abc123/prod456",
							LoadBalancerARN: "arn:aws:elasticloadbalancing:ap-northeast-2:123456789012:loadbalancer/app/payments/abc123",
							Port:            443,
							Protocol:        "HTTPS",
							SSLPolicy:       "ELBSecurityPolicy-TLS13-1-2-2021-06",
							Certificates:    []string{"arn:aws:acm:ap-northeast-2:123456789012:certificate/prod"},
							DefaultActions: []ListenerActionInspection{{
								Type:           "forward",
								TargetGroupARN: "arn:aws:elasticloadbalancing:ap-northeast-2:123456789012:targetgroup/payments-api-prod-blue/abc123",
								TargetGroups: []ListenerActionTargetInspection{
									{TargetGroupARN: "arn:aws:elasticloadbalancing:ap-northeast-2:123456789012:targetgroup/payments-api-prod-blue/abc123", Weight: 100},
								},
							}},
						},
					}, nil
				case "testListener":
					return TrafficRoutingInspection{
						Kind:           req.ResourceType,
						DisplayName:    "Test listener",
						InspectionMode: "live",
						Summary:        req.ResourceDescription,
						Listener: &ListenerInspection{
							ListenerARN:     "arn:aws:elasticloadbalancing:ap-northeast-2:123456789012:listener/app/payments/abc123/test789",
							LoadBalancerARN: "arn:aws:elasticloadbalancing:ap-northeast-2:123456789012:loadbalancer/app/payments/abc123",
							Port:            8443,
							Protocol:        "HTTPS",
							DefaultActions: []ListenerActionInspection{{
								Type:           "forward",
								TargetGroupARN: "arn:aws:elasticloadbalancing:ap-northeast-2:123456789012:targetgroup/payments-api-prod-green/def456",
							}},
						},
					}, nil
				case "productionListenerRule":
					return TrafficRoutingInspection{
						Kind:           req.ResourceType,
						DisplayName:    "Production listener rule",
						InspectionMode: "live",
						Summary:        req.ResourceDescription,
						ListenerRule: &ListenerRuleInspection{
							RuleARN:  "arn:aws:elasticloadbalancing:ap-northeast-2:123456789012:listener-rule/app/payments/abc123/prod456/rule789",
							Priority: "10",
							Conditions: []ListenerRuleConditionInspection{
								{Field: "host-header", Values: []string{"payments.example.com"}},
								{Field: "path-pattern", Values: []string{"/api/*"}},
							},
							Actions: []ListenerActionInspection{{
								Type:  "forward",
								Order: 1,
								TargetGroups: []ListenerActionTargetInspection{
									{TargetGroupARN: "arn:aws:elasticloadbalancing:ap-northeast-2:123456789012:targetgroup/payments-api-prod-active/aaa111", Weight: 90},
									{TargetGroupARN: "arn:aws:elasticloadbalancing:ap-northeast-2:123456789012:targetgroup/payments-api-prod-candidate/bbb222", Weight: 10},
								},
							}},
						},
					}, nil
				case "blueTaskSetTargetGroup":
					return TrafficRoutingInspection{
						Kind:           req.ResourceType,
						DisplayName:    "Blue task set target group",
						InspectionMode: "live",
						Summary:        req.ResourceDescription,
						TargetGroup: &TargetGroupInspection{
							TargetGroupName:            "payments-api-prod-blue",
							TargetGroupARN:             "arn:aws:elasticloadbalancing:ap-northeast-2:123456789012:targetgroup/payments-api-prod-blue/abc123",
							Protocol:                   "HTTP",
							Port:                       8080,
							TargetType:                 "ip",
							LoadBalancerARNs:           []string{"arn:aws:elasticloadbalancing:ap-northeast-2:123456789012:loadbalancer/app/payments/abc123"},
							HealthCheckProtocol:        "HTTP",
							HealthCheckPort:            "traffic-port",
							HealthCheckPath:            "/healthz",
							HealthCheckIntervalSeconds: 30,
							HealthCheckTimeoutSeconds:  5,
							HealthyThresholdCount:      2,
							UnhealthyThresholdCount:    3,
							TargetHealthCounts:         map[string]int{"healthy": 2, "unhealthy": 1},
							Targets: []TargetHealthInspection{
								{TargetID: "10.0.1.10", Port: 8080, State: "healthy"},
								{TargetID: "10.0.2.10", Port: 8080, State: "unhealthy", Reason: "Target.ResponseCodeMismatch", Description: "Health checks failed"},
							},
						},
					}, nil
				case "greenTaskSetTargetGroup":
					return TrafficRoutingInspection{
						Kind:           req.ResourceType,
						DisplayName:    "Green task set target group",
						InspectionMode: "live",
						Summary:        req.ResourceDescription,
						TargetGroup: &TargetGroupInspection{
							TargetGroupName: "payments-api-prod-green",
							TargetGroupARN:  "arn:aws:elasticloadbalancing:ap-northeast-2:123456789012:targetgroup/payments-api-prod-green/def456",
							Protocol:        "HTTP",
							Port:            8080,
							TargetType:      "ip",
						},
					}, nil
				case "activeTargetGroup":
					return TrafficRoutingInspection{
						Kind:           req.ResourceType,
						DisplayName:    "Active target group",
						InspectionMode: "live",
						Summary:        req.ResourceDescription,
						TargetGroup: &TargetGroupInspection{
							TargetGroupName:    "payments-api-prod-active",
							TargetGroupARN:     "arn:aws:elasticloadbalancing:ap-northeast-2:123456789012:targetgroup/payments-api-prod-active/aaa111",
							Protocol:           "HTTP",
							Port:               8080,
							TargetType:         "instance",
							TargetHealthCounts: map[string]int{"healthy": 3},
						},
					}, nil
				case "candidateTargetGroup":
					return TrafficRoutingInspection{
						Kind:           req.ResourceType,
						DisplayName:    "Candidate target group",
						InspectionMode: "live",
						Summary:        req.ResourceDescription,
						TargetGroup: &TargetGroupInspection{
							TargetGroupName:    "payments-api-prod-candidate",
							TargetGroupARN:     "arn:aws:elasticloadbalancing:ap-northeast-2:123456789012:targetgroup/payments-api-prod-candidate/bbb222",
							Protocol:           "HTTP",
							Port:               8080,
							TargetType:         "instance",
							TargetHealthCounts: map[string]int{"initial": 1},
						},
					}, nil
				default:
					t.Fatalf("unexpected traffic routing provider request: %+v", req)
					return TrafficRoutingInspection{}, nil
				}
			},
		})

		for _, tc := range []struct {
			resourceType string
			resourceName string
			summary      string
		}{
			{
				resourceType: "codeDeployApplication",
				resourceName: "payments-api-prod-codedeploy-app",
				summary:      "CodeDeploy application coordinating ECS traffic shifting (style: linear)",
			},
			{
				resourceType: "codeDeployDeploymentGroup",
				resourceName: "payments-api-prod-codedeploy-group",
				summary:      "CodeDeploy deployment group Tenet targets for ECS rollout (style: linear)",
			},
			{
				resourceType: "productionListener",
				resourceName: "payments-api-prod-prod-listener",
				summary:      "Production listener CodeDeploy shifts during ECS traffic shifting (style: linear)",
			},
			{
				resourceType: "testListener",
				resourceName: "payments-api-prod-test-listener",
				summary:      "Test listener CodeDeploy uses to validate the replacement ECS task set before promotion (style: linear)",
			},
			{
				resourceType: "productionListenerRule",
				resourceName: "arn:aws:elasticloadbalancing:ap-northeast-2:123456789012:listener-rule/app/payments/50dc6c495c0c9188/abcdef0123456789/1234567890abcdef",
				summary:      "Production listener rule operators inspect while Tenet updates weighted forwarding during rollout",
			},
			{
				resourceType: "blueTaskSetTargetGroup",
				resourceName: "payments-api-prod-blue",
				summary:      "Blue task set target group CodeDeploy treats as the live ECS traffic destination (style: linear)",
			},
			{
				resourceType: "greenTaskSetTargetGroup",
				resourceName: "payments-api-prod-green",
				summary:      "Green task set target group CodeDeploy warms before shifting ECS traffic (style: linear)",
			},
			{
				resourceType: "activeTargetGroup",
				resourceName: "payments-api-prod-active",
				summary:      "Currently active target group receiving production traffic",
			},
			{
				resourceType: "candidateTargetGroup",
				resourceName: "payments-api-prod-candidate",
				summary:      "Candidate target group Tenet will register and warm before traffic shifting",
			},
		} {
			result, err := service.Inspect(context.Background(), Request{
				DeploymentName:      "payments-api-prod",
				ResourceType:        tc.resourceType,
				ResourceName:        tc.resourceName,
				ResourceDescription: tc.summary,
				ControlAccountID:    "123456789012",
				TargetAccountID:     "123456789012",
				Region:              "ap-northeast-2",
				ObserverRoleARN:     "arn:aws:iam::123456789012:role/TenetObserverRole",
			})
			if err != nil {
				t.Fatalf("Inspect returned error for %s/%s: %v", tc.resourceType, tc.resourceName, err)
			}
			if !result.Available {
				t.Fatalf("expected %s/%s to be available, got %+v", tc.resourceType, tc.resourceName, result)
			}
			if result.Source != SourceTrafficRoutingAWS {
				t.Fatalf("expected live traffic routing source for %s/%s, got %q", tc.resourceType, tc.resourceName, result.Source)
			}
			if result.TrafficRoutingResource == nil || result.TrafficRoutingResource.InspectionMode != "live" {
				t.Fatalf("expected live traffic routing payload for %s/%s, got %+v", tc.resourceType, tc.resourceName, result)
			}
			if tc.resourceType == "blueTaskSetTargetGroup" {
				targetGroup := result.TrafficRoutingResource.TargetGroup
				if targetGroup == nil {
					t.Fatalf("expected live targetGroup payload, got %+v", result.TrafficRoutingResource)
				}
				if targetGroup.TargetGroupName != "payments-api-prod-blue" || targetGroup.Protocol != "HTTP" || targetGroup.Port != 8080 || targetGroup.TargetType != "ip" {
					t.Fatalf("unexpected target group metadata: %+v", targetGroup)
				}
				if !reflect.DeepEqual(targetGroup.TargetHealthCounts, map[string]int{"healthy": 2, "unhealthy": 1}) {
					t.Fatalf("unexpected target health counts: %+v", targetGroup.TargetHealthCounts)
				}
				if len(targetGroup.Targets) != 2 || targetGroup.Targets[1].Reason != "Target.ResponseCodeMismatch" {
					t.Fatalf("unexpected target health rows: %+v", targetGroup.Targets)
				}
			}
			if tc.resourceType == "activeTargetGroup" || tc.resourceType == "candidateTargetGroup" {
				targetGroup := result.TrafficRoutingResource.TargetGroup
				if targetGroup == nil {
					t.Fatalf("expected live weighted targetGroup payload for %s, got %+v", tc.resourceType, result.TrafficRoutingResource)
				}
				if targetGroup.TargetGroupName != tc.resourceName || targetGroup.Protocol != "HTTP" || targetGroup.Port != 8080 || targetGroup.TargetType != "instance" {
					t.Fatalf("unexpected weighted target group metadata for %s: %+v", tc.resourceType, targetGroup)
				}
			}
		}
		if !reflect.DeepEqual(calls, []call{
			{
				DeploymentName:      "payments-api-prod",
				ResourceType:        "codeDeployApplication",
				ResourceName:        "payments-api-prod-codedeploy-app",
				ResourceDescription: "CodeDeploy application coordinating ECS traffic shifting (style: linear)",
				Region:              "ap-northeast-2",
				TargetAccountID:     "123456789012",
				ObserverRoleARN:     "arn:aws:iam::123456789012:role/TenetObserverRole",
			},
			{
				DeploymentName:      "payments-api-prod",
				ResourceType:        "codeDeployDeploymentGroup",
				ResourceName:        "payments-api-prod-codedeploy-group",
				ResourceDescription: "CodeDeploy deployment group Tenet targets for ECS rollout (style: linear)",
				Region:              "ap-northeast-2",
				TargetAccountID:     "123456789012",
				ObserverRoleARN:     "arn:aws:iam::123456789012:role/TenetObserverRole",
			},
			{
				DeploymentName:      "payments-api-prod",
				ResourceType:        "productionListener",
				ResourceName:        "payments-api-prod-prod-listener",
				ResourceDescription: "Production listener CodeDeploy shifts during ECS traffic shifting (style: linear)",
				Region:              "ap-northeast-2",
				TargetAccountID:     "123456789012",
				ObserverRoleARN:     "arn:aws:iam::123456789012:role/TenetObserverRole",
			},
			{
				DeploymentName:      "payments-api-prod",
				ResourceType:        "testListener",
				ResourceName:        "payments-api-prod-test-listener",
				ResourceDescription: "Test listener CodeDeploy uses to validate the replacement ECS task set before promotion (style: linear)",
				Region:              "ap-northeast-2",
				TargetAccountID:     "123456789012",
				ObserverRoleARN:     "arn:aws:iam::123456789012:role/TenetObserverRole",
			},
			{
				DeploymentName:      "payments-api-prod",
				ResourceType:        "productionListenerRule",
				ResourceName:        "arn:aws:elasticloadbalancing:ap-northeast-2:123456789012:listener-rule/app/payments/50dc6c495c0c9188/abcdef0123456789/1234567890abcdef",
				ResourceDescription: "Production listener rule operators inspect while Tenet updates weighted forwarding during rollout",
				Region:              "ap-northeast-2",
				TargetAccountID:     "123456789012",
				ObserverRoleARN:     "arn:aws:iam::123456789012:role/TenetObserverRole",
			},
			{
				DeploymentName:      "payments-api-prod",
				ResourceType:        "blueTaskSetTargetGroup",
				ResourceName:        "payments-api-prod-blue",
				ResourceDescription: "Blue task set target group CodeDeploy treats as the live ECS traffic destination (style: linear)",
				Region:              "ap-northeast-2",
				TargetAccountID:     "123456789012",
				ObserverRoleARN:     "arn:aws:iam::123456789012:role/TenetObserverRole",
			},
			{
				DeploymentName:      "payments-api-prod",
				ResourceType:        "greenTaskSetTargetGroup",
				ResourceName:        "payments-api-prod-green",
				ResourceDescription: "Green task set target group CodeDeploy warms before shifting ECS traffic (style: linear)",
				Region:              "ap-northeast-2",
				TargetAccountID:     "123456789012",
				ObserverRoleARN:     "arn:aws:iam::123456789012:role/TenetObserverRole",
			},
			{
				DeploymentName:      "payments-api-prod",
				ResourceType:        "activeTargetGroup",
				ResourceName:        "payments-api-prod-active",
				ResourceDescription: "Currently active target group receiving production traffic",
				Region:              "ap-northeast-2",
				TargetAccountID:     "123456789012",
				ObserverRoleARN:     "arn:aws:iam::123456789012:role/TenetObserverRole",
			},
			{
				DeploymentName:      "payments-api-prod",
				ResourceType:        "candidateTargetGroup",
				ResourceName:        "payments-api-prod-candidate",
				ResourceDescription: "Candidate target group Tenet will register and warm before traffic shifting",
				Region:              "ap-northeast-2",
				TargetAccountID:     "123456789012",
				ObserverRoleARN:     "arn:aws:iam::123456789012:role/TenetObserverRole",
			},
		}) {
			t.Fatalf("unexpected traffic routing provider calls: %+v", calls)
		}
	})

	t.Run("traffic shift plan remains summary only even when live provider is configured", func(t *testing.T) {
		service := NewService(Options{
			InspectTrafficRoutingResource: func(context.Context, TrafficRoutingRequest) (TrafficRoutingInspection, error) {
				t.Fatalf("trafficShiftPlan should not invoke live traffic routing provider")
				return TrafficRoutingInspection{}, nil
			},
		})

		result, err := service.Inspect(context.Background(), Request{
			DeploymentName:      "payments-api-prod",
			ResourceType:        "trafficShiftPlan",
			ResourceName:        "payments-api-prod-weighted-routing",
			ResourceDescription: "Weighted routing plan Tenet follows to shift a canary slice of production traffic from the active target group to the candidate target group before full promotion while respecting warmup 90s",
			ControlAccountID:    "123456789012",
			TargetAccountID:     "123456789012",
			Region:              "ap-northeast-2",
		})
		if err != nil {
			t.Fatalf("Inspect returned error: %v", err)
		}
		if result.Source != SourceRolloutSummary || result.TrafficRoutingResource == nil || result.TrafficRoutingResource.InspectionMode != "summary_only" {
			t.Fatalf("expected summary-only traffic shift plan payload, got %+v", result)
		}
	})

	t.Run("production listener rule name keeps summary fallback without live ARN", func(t *testing.T) {
		service := NewService(Options{
			InspectTrafficRoutingResource: func(context.Context, TrafficRoutingRequest) (TrafficRoutingInspection, error) {
				t.Fatalf("productionListenerRule without a listener-rule ARN should not invoke live traffic routing provider")
				return TrafficRoutingInspection{}, nil
			},
		})

		result, err := service.Inspect(context.Background(), Request{
			DeploymentName:      "payments-api-prod",
			ResourceType:        "productionListenerRule",
			ResourceName:        "payments-api-prod-prod-listener",
			ResourceDescription: "Production listener rule operators inspect while Tenet updates weighted forwarding during rollout",
			ControlAccountID:    "123456789012",
			TargetAccountID:     "123456789012",
			Region:              "ap-northeast-2",
		})
		if err != nil {
			t.Fatalf("Inspect returned error: %v", err)
		}
		if result.Source != SourceRolloutSummary || result.TrafficRoutingResource == nil || result.TrafficRoutingResource.InspectionMode != "summary_only" {
			t.Fatalf("expected summary-only production listener rule payload, got %+v", result)
		}
	})

	t.Run("target-group live lookup falls back to summary when AWS is unavailable", func(t *testing.T) {
		for _, tc := range []struct {
			resourceType string
			resourceName string
			summary      string
		}{
			{
				resourceType: "blueTaskSetTargetGroup",
				resourceName: "payments-api-staging-blue",
				summary:      "Blue task set target group CodeDeploy treats as the live ECS traffic destination",
			},
			{
				resourceType: "activeTargetGroup",
				resourceName: "payments-api-prod-active",
				summary:      "Currently active target group receiving production traffic",
			},
			{
				resourceType: "candidateTargetGroup",
				resourceName: "payments-api-prod-candidate",
				summary:      "Candidate target group Tenet will register and warm before traffic shifting",
			},
		} {
			t.Run(tc.resourceType, func(t *testing.T) {
				providerCalled := false
				service := NewService(Options{
					InspectTrafficRoutingResource: func(ctx context.Context, req TrafficRoutingRequest) (TrafficRoutingInspection, error) {
						providerCalled = true
						if req.ResourceType != tc.resourceType || req.ResourceName != tc.resourceName {
							t.Fatalf("unexpected traffic routing provider request: %+v", req)
						}
						return TrafficRoutingInspection{}, resourceaws.NewUnavailableError(resourceaws.UnavailableReasonAWSUnavailable)
					},
				})

				result, err := service.Inspect(context.Background(), Request{
					DeploymentName:      "payments-api-staging",
					ResourceType:        tc.resourceType,
					ResourceName:        tc.resourceName,
					ResourceDescription: tc.summary,
					ControlAccountID:    "123456789012",
					TargetAccountID:     "123456789012",
					Region:              "ap-northeast-2",
				})
				if err != nil {
					t.Fatalf("Inspect returned error: %v", err)
				}
				if !providerCalled {
					t.Fatalf("expected %s live provider to be attempted before summary fallback", tc.resourceType)
				}
				if !result.Available || result.Source != SourceRolloutSummary {
					t.Fatalf("expected local-friendly summary fallback, got %+v", result)
				}
				if result.TrafficRoutingResource == nil || result.TrafficRoutingResource.InspectionMode != "summary_only" || result.TrafficRoutingResource.TargetGroup != nil {
					t.Fatalf("expected summary-only target-group payload without live fields, got %+v", result.TrafficRoutingResource)
				}
			})
		}
	})

	t.Run("cross account without observer role returns unavailable payload", func(t *testing.T) {
		service := NewService(Options{
			InspectECSService: func(context.Context, ECSRequest) (ECSServiceInspection, error) {
				t.Fatalf("cross-account request should not invoke provider without observer role")
				return ECSServiceInspection{}, nil
			},
		})

		result, err := service.Inspect(context.Background(), Request{
			DeploymentName:   "payments-api-prod",
			ResourceType:     "ecsService",
			ResourceName:     "payments-api-prod-service",
			ControlAccountID: "111111111111",
			TargetAccountID:  "222222222222",
			Region:           "ap-northeast-2",
		})
		if err != nil {
			t.Fatalf("Inspect returned error: %v", err)
		}
		if result.UnavailableReason != "cross_account_not_configured" || result.Available {
			t.Fatalf("unexpected cross-account unavailable payload: %+v", result)
		}
	})

	t.Run("supported resource degrades to unavailable when AWS is unavailable", func(t *testing.T) {
		service := NewService(Options{
			InspectECSService: func(context.Context, ECSRequest) (ECSServiceInspection, error) {
				return ECSServiceInspection{}, &smithy.OperationError{
					ServiceID:     "STS",
					OperationName: "GetCallerIdentity",
					Err:           errors.New("get identity: get credentials: failed to refresh cached credentials, no EC2 IMDS role found"),
				}
			},
		})

		result, err := service.Inspect(context.Background(), Request{
			DeploymentName:   "payments-api-prod",
			ResourceType:     "ecsService",
			ResourceName:     "payments-api-prod-service",
			ControlAccountID: "123456789012",
			TargetAccountID:  "123456789012",
			Region:           "ap-northeast-2",
			ClusterARN:       " arn:aws:ecs:ap-northeast-2:123456789012:cluster/payments ",
		})
		if err != nil {
			t.Fatalf("Inspect returned error: %v", err)
		}
		if result.UnavailableReason != "aws_unavailable" || result.Available || result.ClusterARN != "arn:aws:ecs:ap-northeast-2:123456789012:cluster/payments" {
			t.Fatalf("unexpected AWS unavailable payload: %+v", result)
		}
	})

	t.Run("traffic routing resource falls back to summary when AWS is unavailable", func(t *testing.T) {
		service := NewService(Options{
			InspectTrafficRoutingResource: func(context.Context, TrafficRoutingRequest) (TrafficRoutingInspection, error) {
				return TrafficRoutingInspection{}, &smithy.OperationError{
					ServiceID:     "CodeDeploy",
					OperationName: "GetDeploymentGroup",
					Err:           errors.New("get deployment group: get credentials: failed to refresh cached credentials, no EC2 IMDS role found"),
				}
			},
		})

		result, err := service.Inspect(context.Background(), Request{
			DeploymentName:      "payments-api-prod",
			ResourceType:        "codeDeployDeploymentGroup",
			ResourceName:        "payments-api-prod-codedeploy-group",
			ResourceDescription: "CodeDeploy deployment group Tenet targets for ECS rollout (style: linear)",
			ControlAccountID:    "123456789012",
			TargetAccountID:     "123456789012",
			Region:              "ap-northeast-2",
			ObserverRoleARN:     "arn:aws:iam::123456789012:role/TenetObserverRole",
		})
		if err != nil {
			t.Fatalf("Inspect returned error: %v", err)
		}
		want := Result{
			DeploymentName: "payments-api-prod",
			ResourceType:   "codeDeployDeploymentGroup",
			ResourceName:   "payments-api-prod-codedeploy-group",
			LookupContext:  expectedLookupContext("ap-northeast-2", "123456789012", "123456789012", "arn:aws:iam::123456789012:role/TenetObserverRole"),
			Available:      true,
			Source:         SourceRolloutSummary,
			TrafficRoutingResource: &TrafficRoutingInspection{
				Kind:           "codeDeployDeploymentGroup",
				DisplayName:    "CodeDeploy deployment group",
				InspectionMode: "summary_only",
				Summary:        "CodeDeploy deployment group Tenet targets for ECS rollout (style: linear)",
			},
		}
		if !reflect.DeepEqual(result, want) {
			t.Fatalf("unexpected AWS-unavailable traffic routing fallback payload: %+v", result)
		}
	})

	t.Run("traffic shift plan remains summary only even when live provider is configured", func(t *testing.T) {
		service := NewService(Options{
			InspectTrafficRoutingResource: func(context.Context, TrafficRoutingRequest) (TrafficRoutingInspection, error) {
				t.Fatalf("trafficShiftPlan should not invoke live traffic routing provider")
				return TrafficRoutingInspection{}, nil
			},
		})

		result, err := service.Inspect(context.Background(), Request{
			DeploymentName:      "payments-api-prod",
			ResourceType:        "trafficShiftPlan",
			ResourceName:        "payments-api-prod-weighted-routing",
			ResourceDescription: "Weighted routing plan Tenet follows to shift a canary slice of production traffic from the active target group to the candidate target group before full promotion while respecting warmup 90s",
			ControlAccountID:    "123456789012",
			TargetAccountID:     "222222222222",
			Region:              "ap-northeast-2",
		})
		if err != nil {
			t.Fatalf("Inspect returned error: %v", err)
		}
		if result.Source != SourceRolloutSummary || result.TrafficRoutingResource == nil || result.TrafficRoutingResource.InspectionMode != "summary_only" {
			t.Fatalf("expected summary-only traffic shift plan payload, got %+v", result)
		}
	})

	t.Run("traffic routing live inspection reuses cached result", func(t *testing.T) {
		callCount := 0
		service := NewService(Options{
			InspectTrafficRoutingResource: func(context.Context, TrafficRoutingRequest) (TrafficRoutingInspection, error) {
				callCount++
				return TrafficRoutingInspection{
					Kind:           "productionListener",
					DisplayName:    "Production listener",
					InspectionMode: "live",
					Summary:        "Production listener CodeDeploy shifts during ECS traffic shifting",
					Listener: &ListenerInspection{
						ListenerARN:     "arn:listener/prod",
						LoadBalancerARN: "arn:loadbalancer/payments",
						Port:            443,
						Protocol:        "HTTPS",
					},
				}, nil
			},
		})
		req := Request{
			DeploymentName:      "payments-api-prod",
			ResourceType:        "productionListener",
			ResourceName:        "payments-api-prod-prod-listener",
			ResourceDescription: "Production listener CodeDeploy shifts during ECS traffic shifting",
			ControlAccountID:    "123456789012",
			TargetAccountID:     "123456789012",
			Region:              "ap-northeast-2",
			ObserverRoleARN:     "arn:aws:iam::123456789012:role/TenetObserverRole",
		}

		first, err := service.Inspect(context.Background(), req)
		if err != nil {
			t.Fatalf("first Inspect returned error: %v", err)
		}
		second, err := service.Inspect(context.Background(), req)
		if err != nil {
			t.Fatalf("second Inspect returned error: %v", err)
		}
		if callCount != 1 {
			t.Fatalf("expected one live provider call for repeated lookup, got %d", callCount)
		}
		if !reflect.DeepEqual(first, second) {
			t.Fatalf("expected repeated lookup to return cached payload, first=%+v second=%+v", first, second)
		}
	})

	t.Run("traffic routing live inspection cache key separates region account role type and name", func(t *testing.T) {
		callCount := 0
		service := NewService(Options{
			InspectTrafficRoutingResource: func(context.Context, TrafficRoutingRequest) (TrafficRoutingInspection, error) {
				callCount++
				return TrafficRoutingInspection{
					Kind:           "productionListener",
					DisplayName:    "Production listener",
					InspectionMode: "live",
					Summary:        "summary",
					Listener: &ListenerInspection{
						ListenerARN: "arn:listener/" + string(rune('a'+callCount-1)),
					},
				}, nil
			},
		})
		base := Request{
			DeploymentName:      "payments-api-prod",
			ResourceType:        "productionListener",
			ResourceName:        "payments-api-prod-prod-listener",
			ResourceDescription: "summary",
			ControlAccountID:    "123456789012",
			TargetAccountID:     "123456789012",
			Region:              "ap-northeast-2",
			ObserverRoleARN:     "arn:aws:iam::123456789012:role/TenetObserverRole",
		}
		requests := []Request{
			base,
			base,
			func() Request {
				req := base
				req.Region = "us-west-2"
				return req
			}(),
			func() Request {
				req := base
				req.TargetAccountID = "210987654321"
				return req
			}(),
			func() Request {
				req := base
				req.ObserverRoleARN = "arn:aws:iam::123456789012:role/AnotherObserverRole"
				return req
			}(),
			func() Request {
				req := base
				req.ResourceType = "testListener"
				req.ResourceName = "payments-api-prod-test-listener"
				return req
			}(),
			func() Request {
				req := base
				req.ResourceName = "payments-api-prod-prod-listener-v2"
				return req
			}(),
		}

		for i, req := range requests {
			if _, err := service.Inspect(context.Background(), req); err != nil {
				t.Fatalf("Inspect #%d returned error: %v", i+1, err)
			}
		}
		if callCount != 6 {
			t.Fatalf("expected repeated base lookup plus five key changes to trigger 6 live provider calls, got %d", callCount)
		}
	})

	t.Run("traffic routing live inspection retries after unavailable result because unavailable outcomes are not cached", func(t *testing.T) {
		callCount := 0
		service := NewService(Options{
			InspectTrafficRoutingResource: func(context.Context, TrafficRoutingRequest) (TrafficRoutingInspection, error) {
				callCount++
				if callCount == 1 {
					return TrafficRoutingInspection{}, resourceaws.NewUnavailableError(resourceaws.UnavailableReasonResourceNotFound)
				}
				return TrafficRoutingInspection{
					Kind:           "codeDeployDeploymentGroup",
					DisplayName:    "CodeDeploy deployment group",
					InspectionMode: "live",
					Summary:        "CodeDeploy deployment group Tenet targets for ECS rollout",
					CodeDeployDeploymentGroup: &CodeDeployDeploymentGroupInspection{
						ApplicationName:     "payments-api-prod-codedeploy-app",
						DeploymentGroupName: "payments-api-prod-codedeploy-group",
					},
				}, nil
			},
		})
		req := Request{
			DeploymentName:      "payments-api-prod",
			ResourceType:        "codeDeployDeploymentGroup",
			ResourceName:        "payments-api-prod-codedeploy-group",
			ResourceDescription: "CodeDeploy deployment group Tenet targets for ECS rollout",
			ControlAccountID:    "123456789012",
			TargetAccountID:     "123456789012",
			Region:              "ap-northeast-2",
			ObserverRoleARN:     "arn:aws:iam::123456789012:role/TenetObserverRole",
		}

		first, err := service.Inspect(context.Background(), req)
		if err != nil {
			t.Fatalf("first Inspect returned error: %v", err)
		}
		if first.Available || first.UnavailableReason != resourceaws.UnavailableReasonResourceNotFound {
			t.Fatalf("expected first lookup to remain unavailable, got %+v", first)
		}
		second, err := service.Inspect(context.Background(), req)
		if err != nil {
			t.Fatalf("second Inspect returned error: %v", err)
		}
		if !second.Available || second.Source != SourceTrafficRoutingAWS || second.TrafficRoutingResource == nil || second.TrafficRoutingResource.InspectionMode != "live" {
			t.Fatalf("expected second lookup to retry and return live payload, got %+v", second)
		}
		if callCount != 2 {
			t.Fatalf("expected unavailable result to bypass cache and retry provider, got %d calls", callCount)
		}
	})

	t.Run("traffic routing cached result recomputes summary from current request", func(t *testing.T) {
		callCount := 0
		service := NewService(Options{
			InspectTrafficRoutingResource: func(context.Context, TrafficRoutingRequest) (TrafficRoutingInspection, error) {
				callCount++
				return TrafficRoutingInspection{
					Kind:           "testListener",
					DisplayName:    "Test listener",
					InspectionMode: "live",
					Summary:        "stale summary",
					Listener: &ListenerInspection{
						ListenerARN: "arn:listener/test",
					},
				}, nil
			},
		})
		req := Request{
			DeploymentName:      "payments-api-prod",
			ResourceType:        "testListener",
			ResourceName:        "payments-api-prod-test-listener",
			ResourceDescription: "Test listener CodeDeploy uses to validate the replacement ECS task set before promotion",
			ControlAccountID:    "123456789012",
			TargetAccountID:     "123456789012",
			Region:              "ap-northeast-2",
			ObserverRoleARN:     "arn:aws:iam::123456789012:role/TenetObserverRole",
		}

		first, err := service.Inspect(context.Background(), req)
		if err != nil {
			t.Fatalf("first Inspect returned error: %v", err)
		}
		if first.TrafficRoutingResource == nil || first.TrafficRoutingResource.Summary != "stale summary" {
			t.Fatalf("expected provider summary on first live lookup, got %+v", first)
		}

		req.ResourceDescription = "Updated operator-facing summary for the replacement validation listener"
		second, err := service.Inspect(context.Background(), req)
		if err != nil {
			t.Fatalf("second Inspect returned error: %v", err)
		}
		if second.TrafficRoutingResource == nil || second.TrafficRoutingResource.Summary != req.ResourceDescription {
			t.Fatalf("expected cached lookup to recompute summary from current request, got %+v", second)
		}
		if callCount != 1 {
			t.Fatalf("expected cached summary recomputation to avoid a second provider call, got %d", callCount)
		}
	})

	t.Run("provider failures are returned", func(t *testing.T) {
		providerErr := errors.New("boom: inspection provider internals")
		service := NewService(Options{
			InspectAutoScalingGroup: func(context.Context, AutoScalingGroupRequest) (AutoScalingInspection, error) {
				return AutoScalingInspection{}, providerErr
			},
		})

		result, err := service.Inspect(context.Background(), Request{
			DeploymentName:   "payments-api-prod",
			ResourceType:     "candidateAutoScalingGroup",
			ResourceName:     "payments-api-prod-candidate",
			ControlAccountID: "123456789012",
			TargetAccountID:  "123456789012",
			Region:           "ap-northeast-2",
		})
		if !errors.Is(err, providerErr) {
			t.Fatalf("expected provider error %v, got %v", providerErr, err)
		}
		if !reflect.DeepEqual(result, Result{}) {
			t.Fatalf("expected zero result on provider error, got %+v", result)
		}
	})
}

func TestECSInspectionFromServiceSortsDeploymentsAndEvents(t *testing.T) {
	service := ecstypes.Service{
		ServiceArn:                    stringPtr("arn:service"),
		ServiceName:                   stringPtr("payments-api-prod-service"),
		Status:                        stringPtr("ACTIVE"),
		TaskDefinition:                stringPtr("payments-api:42"),
		DesiredCount:                  2,
		RunningCount:                  2,
		PendingCount:                  0,
		HealthCheckGracePeriodSeconds: int32Ptr(30),
		DeploymentConfiguration: &ecstypes.DeploymentConfiguration{
			MinimumHealthyPercent: int32Ptr(90),
			MaximumPercent:        int32Ptr(150),
		},
		Deployments: []ecstypes.Deployment{
			{Id: stringPtr("secondary"), CreatedAt: timePtr(time.Date(2026, 4, 18, 0, 10, 0, 0, time.UTC))},
			{Id: stringPtr("primary"), CreatedAt: timePtr(time.Date(2026, 4, 18, 0, 11, 0, 0, time.UTC))},
		},
		Events: []ecstypes.ServiceEvent{
			{CreatedAt: timePtr(time.Date(2026, 4, 18, 0, 12, 0, 0, time.UTC)), Message: stringPtr("steady")},
			{CreatedAt: timePtr(time.Date(2026, 4, 18, 0, 10, 0, 0, time.UTC)), Message: stringPtr("started")},
		},
	}

	got := ecsInspectionFromService(service, "arn:cluster", "ghcr.io/devopsartfactory/payments-api:2026-04-18.2", []ECSTaskSummary{{TaskARN: "arn:task/one", TaskID: "one"}}, &ECSTaskSummaryScope{Available: true, EffectiveLimit: maxECSServiceTaskSummaries, ReturnedTaskCount: 1, TruncationStatus: "not_truncated"})
	if len(got.Deployments) != 2 || got.Deployments[0].ID != "primary" {
		t.Fatalf("expected deployments sorted newest-first, got %+v", got.Deployments)
	}
	if len(got.Events) != 2 || got.Events[0].Message != "steady" {
		t.Fatalf("expected events sorted newest-first, got %+v", got.Events)
	}
	if got.TaskDefinitionImage != "ghcr.io/devopsartfactory/payments-api:2026-04-18.2" {
		t.Fatalf("expected task-definition image evidence in ECS inspection payload, got %+v", got)
	}
	if got.MinimumHealthyPercent == nil || *got.MinimumHealthyPercent != 90 {
		t.Fatalf("expected minimum healthy percent in ECS inspection payload, got %+v", got)
	}
	if got.MaximumPercent == nil || *got.MaximumPercent != 150 {
		t.Fatalf("expected maximum percent in ECS inspection payload, got %+v", got)
	}
	if got.HealthCheckGracePeriodSeconds == nil || *got.HealthCheckGracePeriodSeconds != 30 {
		t.Fatalf("expected health check grace period in ECS inspection payload, got %+v", got)
	}
	if len(got.Tasks) != 1 || got.Tasks[0].TaskID != "one" {
		t.Fatalf("expected ECS service task summaries to pass through, got %+v", got.Tasks)
	}
	if got.TasksScope == nil || !got.TasksScope.Available || got.TasksScope.EffectiveLimit != maxECSServiceTaskSummaries || got.TasksScope.ReturnedTaskCount != 1 || got.TasksScope.TruncationStatus != "not_truncated" {
		t.Fatalf("expected ECS service task scope metadata to pass through, got %+v", got.TasksScope)
	}
}

func TestECSServiceTaskSummariesReportsUnavailableScopeWhenTaskBridgeFails(t *testing.T) {
	const (
		clusterARN  = "arn:aws:ecs:ap-northeast-2:123456789012:cluster/worker"
		serviceName = "worker-prod-service"
	)
	client := &fakeECSServiceAPI{listTasksErr: resourceaws.NewUnavailableError(resourceaws.UnavailableReasonAWSUnavailable)}

	got, scope, err := ecsServiceTaskSummaries(context.Background(), client, nil, clusterARN, serviceName, 5)
	if err != nil {
		t.Fatalf("expected task summary bridge failure to be represented in scope, got error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected unavailable task bridge to return no task rows, got %+v", got)
	}
	if scope == nil || scope.Available || scope.RequestedLimit == nil || *scope.RequestedLimit != 5 || scope.EffectiveLimit != 5 || scope.ReturnedTaskCount != 0 || scope.TruncationStatus != "not_applicable" || scope.UnavailableReason != resourceaws.UnavailableReasonAWSUnavailable || scope.Summary == "" {
		t.Fatalf("expected explicit unavailable task-scope metadata, got %+v", scope)
	}
}

func TestECSServiceTaskSummariesDiscoversBoundedTasksWithContainerLogs(t *testing.T) {
	const (
		clusterARN  = "arn:aws:ecs:ap-northeast-2:123456789012:cluster/worker"
		serviceName = "worker-prod-service"
		taskDefARN  = "arn:aws:ecs:ap-northeast-2:123456789012:task-definition/worker:42"
	)
	base := time.Date(2026, 5, 3, 4, 0, 0, 0, time.UTC)
	tasks := map[string]ecstypes.Task{}
	stoppedARNs := make([]string, 0, 11)
	for i := 0; i < 11; i++ {
		taskARN := "arn:aws:ecs:ap-northeast-2:123456789012:task/worker/task-" + strconv.Itoa(i)
		stoppedARNs = append(stoppedARNs, taskARN)
		tasks[taskARN] = ecstypes.Task{
			TaskArn:           stringPtr(taskARN),
			ClusterArn:        stringPtr(clusterARN),
			TaskDefinitionArn: stringPtr(taskDefARN),
			LastStatus:        stringPtr("STOPPED"),
			DesiredStatus:     stringPtr("STOPPED"),
			StoppedAt:         timePtr(base.Add(time.Duration(i) * time.Minute)),
			StoppedReason:     stringPtr("Essential container in task exited"),
			Containers: []ecstypes.Container{
				{Name: stringPtr("sidecar"), Image: stringPtr("collector:latest"), LastStatus: stringPtr("STOPPED")},
				{Name: stringPtr("worker"), Image: stringPtr("worker:latest"), LastStatus: stringPtr("STOPPED"), ExitCode: int32Ptr(1), Reason: stringPtr("Error")},
			},
		}
	}
	runningARN := "arn:aws:ecs:ap-northeast-2:123456789012:task/worker/task-11"
	tasks[runningARN] = ecstypes.Task{
		TaskArn:              stringPtr(runningARN),
		ClusterArn:           stringPtr(clusterARN),
		TaskDefinitionArn:    stringPtr(taskDefARN),
		ContainerInstanceArn: stringPtr("arn:aws:ecs:ap-northeast-2:123456789012:container-instance/worker/ci-running"),
		CapacityProviderName: stringPtr("worker-fargate-spot"),
		AvailabilityZone:     stringPtr("ap-northeast-2c"),
		LastStatus:           stringPtr("RUNNING"),
		DesiredStatus:        stringPtr("RUNNING"),
		LaunchType:           ecstypes.LaunchTypeFargate,
		StartedAt:            timePtr(base.Add(20 * time.Minute)),
		Containers: []ecstypes.Container{
			{Name: stringPtr("sidecar"), Image: stringPtr("collector:latest"), LastStatus: stringPtr("RUNNING")},
			{Name: stringPtr("worker"), Image: stringPtr("worker:latest"), LastStatus: stringPtr("RUNNING")},
		},
	}
	client := &fakeECSServiceAPI{
		taskARNsByDesired: map[ecstypes.DesiredStatus][]string{
			ecstypes.DesiredStatusRunning: {runningARN, stoppedARNs[10]},
			ecstypes.DesiredStatusStopped: stoppedARNs,
		},
		tasksByARN: tasks,
		taskDefinitions: map[string]ecstypes.TaskDefinition{
			taskDefARN: {ContainerDefinitions: []ecstypes.ContainerDefinition{
				{Name: stringPtr("worker"), LogConfiguration: &ecstypes.LogConfiguration{LogDriver: ecstypes.LogDriverAwslogs, Options: map[string]string{"awslogs-group": "/ecs/worker", "awslogs-stream-prefix": "ecs"}}},
			}},
		},
	}

	got, scope, err := ecsServiceTaskSummaries(context.Background(), client, nil, clusterARN, serviceName, 0)
	if err != nil {
		t.Fatalf("ecsServiceTaskSummaries returned error: %v", err)
	}
	if scope == nil || scope.EffectiveLimit != maxECSServiceTaskSummaries || scope.ReturnedTaskCount != maxECSServiceTaskSummaries || scope.TruncationStatus != "truncated" {
		t.Fatalf("expected truncated task-scope metadata, got %+v", scope)
	}
	if len(got) != maxECSServiceTaskSummaries {
		t.Fatalf("expected bounded task summaries, got %d: %+v", len(got), got)
	}
	if got[0].TaskID != "task-11" || got[len(got)-1].TaskID != "task-2" {
		t.Fatalf("expected newest task summaries first and capped to 10, got %+v", got)
	}
	if got[0].ContainerInstanceARN != "arn:aws:ecs:ap-northeast-2:123456789012:container-instance/worker/ci-running" || got[0].CapacityProviderName != "worker-fargate-spot" || got[0].AvailabilityZone != "ap-northeast-2c" {
		t.Fatalf("expected ECS service task sample to preserve placement evidence, got %+v", got[0])
	}
	if !reflect.DeepEqual(client.listTaskCalls, []ecstypes.DesiredStatus{ecstypes.DesiredStatusRunning, ecstypes.DesiredStatusStopped}) {
		t.Fatalf("expected running then stopped task discovery, got %+v", client.listTaskCalls)
	}
	if len(got[0].Containers) != 2 || got[0].Containers[0].Name != "sidecar" || got[0].Containers[1].Name != "worker" {
		t.Fatalf("expected deterministic container summaries, got %+v", got[0].Containers)
	}
	if got[0].Containers[1].AWSLogs == nil || got[0].Containers[1].AWSLogs.Stream != "ecs/worker/task-11" {
		t.Fatalf("expected awslogs metadata from task definition, got %+v", got[0].Containers[1])
	}
}

func TestECSServiceTaskSummariesHonorsRequestedLimit(t *testing.T) {
	const (
		clusterARN  = "arn:aws:ecs:ap-northeast-2:123456789012:cluster/worker"
		serviceName = "worker-prod-service"
	)
	tasks := map[string]ecstypes.Task{}
	taskARNs := make([]string, 0, 4)
	base := time.Date(2026, 4, 18, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 4; i++ {
		taskARN := "arn:aws:ecs:ap-northeast-2:123456789012:task/worker/limited-" + strconv.Itoa(i)
		taskARNs = append(taskARNs, taskARN)
		tasks[taskARN] = ecstypes.Task{TaskArn: stringPtr(taskARN), LastStatus: stringPtr("RUNNING"), StartedAt: timePtr(base.Add(time.Duration(i) * time.Minute))}
	}
	client := &fakeECSServiceAPI{
		taskARNsByDesired: map[ecstypes.DesiredStatus][]string{ecstypes.DesiredStatusRunning: taskARNs},
		tasksByARN:        tasks,
	}

	got, scope, err := ecsServiceTaskSummaries(context.Background(), client, nil, clusterARN, serviceName, 2)
	if err != nil {
		t.Fatalf("ecsServiceTaskSummaries returned error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected requested two task summaries, got %d: %+v", len(got), got)
	}
	if got[0].TaskID != "limited-3" || got[1].TaskID != "limited-2" {
		t.Fatalf("expected newest requested task summaries, got %+v", got)
	}
	if scope == nil || scope.RequestedLimit == nil || *scope.RequestedLimit != 2 || scope.EffectiveLimit != 2 || scope.ReturnedTaskCount != 2 || scope.TruncationStatus != "truncated" {
		t.Fatalf("expected requested task-scope metadata, got %+v", scope)
	}
}

func TestECSServiceTaskSummariesReportsUnknownWhenExactlyAtLimit(t *testing.T) {
	const (
		clusterARN  = "arn:aws:ecs:ap-northeast-2:123456789012:cluster/worker"
		serviceName = "worker-prod-service"
	)
	tasks := map[string]ecstypes.Task{}
	taskARNs := make([]string, 0, maxECSServiceTaskSummaries)
	for i := 0; i < maxECSServiceTaskSummaries; i++ {
		taskARN := "arn:aws:ecs:ap-northeast-2:123456789012:task/worker/exact-" + strconv.Itoa(i)
		taskARNs = append(taskARNs, taskARN)
		tasks[taskARN] = ecstypes.Task{TaskArn: stringPtr(taskARN), LastStatus: stringPtr("RUNNING")}
	}
	client := &fakeECSServiceAPI{
		taskARNsByDesired: map[ecstypes.DesiredStatus][]string{ecstypes.DesiredStatusRunning: taskARNs},
		tasksByARN:        tasks,
	}

	got, scope, err := ecsServiceTaskSummaries(context.Background(), client, nil, clusterARN, serviceName, 0)
	if err != nil {
		t.Fatalf("ecsServiceTaskSummaries returned error: %v", err)
	}
	if len(got) != maxECSServiceTaskSummaries {
		t.Fatalf("expected exactly cap-sized task summaries, got %d", len(got))
	}
	if scope == nil || scope.EffectiveLimit != maxECSServiceTaskSummaries || scope.ReturnedTaskCount != maxECSServiceTaskSummaries || scope.TruncationStatus != "unknown" {
		t.Fatalf("expected conservative cap-reached task-scope metadata, got %+v", scope)
	}
}

func TestECSServiceTaskSummariesOmitLogMetadataWhenTaskDefinitionLookupFails(t *testing.T) {
	const (
		clusterARN  = "arn:aws:ecs:ap-northeast-2:123456789012:cluster/worker"
		serviceName = "worker-prod-service"
		taskDefARN  = "arn:aws:ecs:ap-northeast-2:123456789012:task-definition/worker:42"
		taskARN     = "arn:aws:ecs:ap-northeast-2:123456789012:task/worker/task-1"
	)
	client := &fakeECSServiceAPI{
		taskARNsByDesired: map[ecstypes.DesiredStatus][]string{ecstypes.DesiredStatusRunning: {taskARN}},
		tasksByARN: map[string]ecstypes.Task{taskARN: {
			TaskArn:           stringPtr(taskARN),
			TaskDefinitionArn: stringPtr(taskDefARN),
			LastStatus:        stringPtr("RUNNING"),
			Containers:        []ecstypes.Container{{Name: stringPtr("worker"), Image: stringPtr("worker:latest")}},
		}},
		describeTaskDefinitionErr: errors.New("access denied to task definition"),
	}

	got, scope, err := ecsServiceTaskSummaries(context.Background(), client, nil, clusterARN, serviceName, 0)
	if err != nil {
		t.Fatalf("ecsServiceTaskSummaries returned error: %v", err)
	}
	if scope == nil || scope.EffectiveLimit != maxECSServiceTaskSummaries || scope.ReturnedTaskCount != 1 || scope.TruncationStatus != "not_truncated" {
		t.Fatalf("expected below-cap task-scope metadata, got %+v", scope)
	}
	if len(got) != 1 || len(got[0].Containers) != 1 {
		t.Fatalf("expected task/container summary despite task definition error, got %+v", got)
	}
	if got[0].Containers[0].AWSLogs != nil {
		t.Fatalf("expected missing task definition metadata to omit awslogs, got %+v", got[0].Containers[0])
	}
}

func TestECSTaskInspectionIncludesContainerInstanceEC2HostIdentity(t *testing.T) {
	const (
		clusterARN           = "arn:aws:ecs:ap-northeast-2:123456789012:cluster/worker"
		taskARN              = "arn:aws:ecs:ap-northeast-2:123456789012:task/worker/task-ec2"
		containerInstanceARN = "arn:aws:ecs:ap-northeast-2:123456789012:container-instance/worker/ci-ec2"
	)

	got := ecsTaskInspectionFromTask(ecstypes.Task{
		TaskArn:              stringPtr(taskARN),
		ClusterArn:           stringPtr(clusterARN),
		ContainerInstanceArn: stringPtr(containerInstanceARN),
		LaunchType:           ecstypes.LaunchTypeEc2,
		LastStatus:           stringPtr("RUNNING"),
	}, clusterARN, nil, map[string]string{containerInstanceARN: "i-0abc123def4567890"})

	if got.EC2InstanceID != "i-0abc123def4567890" {
		t.Fatalf("expected direct ECS task inspection to expose EC2 host identity, got %+v", got)
	}
}

func TestECSTaskInspectionEnrichesResolvedEC2HostDetail(t *testing.T) {
	launchTime := time.Date(2026, 5, 5, 7, 30, 0, 0, time.UTC)
	client := &fakeEC2InstanceAPI{
		output: &ec2.DescribeInstancesOutput{
			Reservations: []ec2types.Reservation{{
				Instances: []ec2types.Instance{{
					InstanceId:       stringPtr("i-0abc123def4567890"),
					State:            &ec2types.InstanceState{Name: ec2types.InstanceStateNameRunning},
					InstanceType:     ec2types.InstanceTypeM6iLarge,
					ImageId:          stringPtr("ami-abc123"),
					PrivateIpAddress: stringPtr("10.0.4.12"),
					PublicIpAddress:  stringPtr("198.51.100.12"),
					SubnetId:         stringPtr("subnet-abc123"),
					VpcId:            stringPtr("vpc-abc123"),
					LaunchTime:       &launchTime,
				}},
			}},
		},
	}

	got := enrichECSTaskInspectionFromEC2(context.Background(), client, ECSTaskInspection{
		ECSTaskSummary: ECSTaskSummary{
			TaskARN:       "arn:aws:ecs:ap-northeast-2:123456789012:task/worker/task-ec2",
			TaskID:        "task-ec2",
			EC2InstanceID: " i-0abc123def4567890 ",
		},
		TaskDefinition: "worker:42",
	})

	if !reflect.DeepEqual(client.instanceIDs, []string{"i-0abc123def4567890"}) {
		t.Fatalf("expected ECS task host-detail lookup for resolved EC2 instance ID, got %+v", client.instanceIDs)
	}
	if got.EC2Instance == nil {
		t.Fatalf("expected nested EC2 host detail on ECS task inspection, got %+v", got)
	}
	if got.EC2Instance.InstanceState != "running" || got.EC2Instance.InstanceType != "m6i.large" || got.EC2Instance.ImageID != "ami-abc123" || got.EC2Instance.PrivateIPAddress != "10.0.4.12" || got.EC2Instance.PublicIPAddress != "198.51.100.12" || got.EC2Instance.SubnetID != "subnet-abc123" || got.EC2Instance.VPCID != "vpc-abc123" || got.EC2Instance.LaunchTime != "2026-05-05T07:30:00Z" {
		t.Fatalf("expected EC2 host fields to reuse EC2InstanceDetail semantics, got %+v", got.EC2Instance)
	}
}

func TestECSTaskInspectionOmitsEC2HostDetailOnMissingIdentityOrLookupFailure(t *testing.T) {
	withoutIdentityClient := &fakeEC2InstanceAPI{}
	withoutIdentity := enrichECSTaskInspectionFromEC2(context.Background(), withoutIdentityClient, ECSTaskInspection{
		ECSTaskSummary: ECSTaskSummary{TaskARN: "arn:task/fargate", TaskID: "fargate"},
	})
	if withoutIdentity.EC2Instance != nil || len(withoutIdentityClient.instanceIDs) != 0 {
		t.Fatalf("expected missing ECS-on-EC2 identity to skip optional host lookup, got detail=%+v calls=%+v", withoutIdentity.EC2Instance, withoutIdentityClient.instanceIDs)
	}

	failureClient := &fakeEC2InstanceAPI{err: resourceaws.NewUnavailableError(resourceaws.UnavailableReasonAWSUnavailable)}
	failure := enrichECSTaskInspectionFromEC2(context.Background(), failureClient, ECSTaskInspection{
		ECSTaskSummary: ECSTaskSummary{TaskARN: "arn:task/ec2", TaskID: "ec2", EC2InstanceID: "i-0abc123def4567890"},
	})
	if !reflect.DeepEqual(failureClient.instanceIDs, []string{"i-0abc123def4567890"}) {
		t.Fatalf("expected one best-effort EC2 lookup before graceful omission, got %+v", failureClient.instanceIDs)
	}
	if failure.EC2Instance != nil || failure.EC2InstanceID != "i-0abc123def4567890" {
		t.Fatalf("expected EC2 lookup failure to omit only nested host detail and preserve host identity, got %+v", failure)
	}
}

func TestECSServiceTaskSummariesEnrichContainerInstanceEC2HostIdentity(t *testing.T) {
	const (
		clusterARN           = "arn:aws:ecs:ap-northeast-2:123456789012:cluster/worker"
		serviceName          = "worker-prod-service"
		taskARN              = "arn:aws:ecs:ap-northeast-2:123456789012:task/worker/task-ec2"
		containerInstanceARN = "arn:aws:ecs:ap-northeast-2:123456789012:container-instance/worker/ci-ec2"
	)
	launchTime := time.Date(2026, 5, 5, 7, 30, 0, 0, time.UTC)
	client := &fakeECSServiceAPI{
		taskARNsByDesired: map[ecstypes.DesiredStatus][]string{ecstypes.DesiredStatusRunning: {taskARN}},
		tasksByARN: map[string]ecstypes.Task{taskARN: {
			TaskArn:              stringPtr(taskARN),
			ClusterArn:           stringPtr(clusterARN),
			ContainerInstanceArn: stringPtr(containerInstanceARN),
			LaunchType:           ecstypes.LaunchTypeEc2,
			LastStatus:           stringPtr("RUNNING"),
		}},
		containerInstancesByARN: map[string]ecstypes.ContainerInstance{
			containerInstanceARN: {ContainerInstanceArn: stringPtr(containerInstanceARN), Ec2InstanceId: stringPtr("i-0abc123def4567890")},
		},
	}
	ec2Client := &fakeEC2InstanceAPI{output: &ec2.DescribeInstancesOutput{
		Reservations: []ec2types.Reservation{{
			Instances: []ec2types.Instance{{
				InstanceId:       stringPtr("i-0abc123def4567890"),
				State:            &ec2types.InstanceState{Name: ec2types.InstanceStateNameRunning},
				InstanceType:     ec2types.InstanceTypeM6iLarge,
				ImageId:          stringPtr("ami-abc123"),
				PrivateIpAddress: stringPtr("10.0.4.12"),
				SubnetId:         stringPtr("subnet-abc123"),
				VpcId:            stringPtr("vpc-abc123"),
				LaunchTime:       &launchTime,
			}},
		}},
	}}

	got, scope, err := ecsServiceTaskSummaries(context.Background(), client, ec2Client, clusterARN, serviceName, 0)
	if err != nil {
		t.Fatalf("ecsServiceTaskSummaries returned error: %v", err)
	}
	if scope == nil || scope.ReturnedTaskCount != 1 {
		t.Fatalf("expected one task row in scope, got %+v", scope)
	}
	if len(got) != 1 || got[0].EC2InstanceID != "i-0abc123def4567890" {
		t.Fatalf("expected ECS service task sample to include EC2 host identity, got %+v", got)
	}
	if got[0].EC2Instance == nil || got[0].EC2Instance.InstanceState != "running" || got[0].EC2Instance.InstanceType != "m6i.large" || got[0].EC2Instance.ImageID != "ami-abc123" || got[0].EC2Instance.PrivateIPAddress != "10.0.4.12" || got[0].EC2Instance.SubnetID != "subnet-abc123" || got[0].EC2Instance.VPCID != "vpc-abc123" || got[0].EC2Instance.LaunchTime != "2026-05-05T07:30:00Z" {
		t.Fatalf("expected sampled ECS service task row to reuse EC2InstanceDetail host fields, got %+v", got[0].EC2Instance)
	}
	if !reflect.DeepEqual(client.describeContainerInstanceInput, [][]string{{containerInstanceARN}}) {
		t.Fatalf("expected one bounded DescribeContainerInstances lookup, got %+v", client.describeContainerInstanceInput)
	}
	if !reflect.DeepEqual(ec2Client.instanceIDs, []string{"i-0abc123def4567890"}) {
		t.Fatalf("expected one bounded DescribeInstances lookup for the sampled task host, got %+v", ec2Client.instanceIDs)
	}
}

func TestECSServiceTaskSummariesOmitEC2HostDetailOnMissingIdentityOrLookupFailure(t *testing.T) {
	withoutIdentityClient := &fakeEC2InstanceAPI{}
	withoutIdentity := enrichECSTaskSummariesFromEC2(context.Background(), withoutIdentityClient, []ECSTaskSummary{{TaskARN: "arn:task/fargate", TaskID: "fargate"}})
	if len(withoutIdentity) != 1 || withoutIdentity[0].EC2Instance != nil || len(withoutIdentityClient.instanceIDs) != 0 {
		t.Fatalf("expected missing service-task host identity to skip optional EC2 lookup, got rows=%+v calls=%+v", withoutIdentity, withoutIdentityClient.instanceIDs)
	}

	failureClient := &fakeEC2InstanceAPI{err: resourceaws.NewUnavailableError(resourceaws.UnavailableReasonAWSUnavailable)}
	failure := enrichECSTaskSummariesFromEC2(context.Background(), failureClient, []ECSTaskSummary{{TaskARN: "arn:task/ec2", TaskID: "ec2", EC2InstanceID: " i-0abc123def4567890 "}})
	if !reflect.DeepEqual(failureClient.instanceIDs, []string{"i-0abc123def4567890"}) {
		t.Fatalf("expected one best-effort EC2 lookup before graceful omission, got %+v", failureClient.instanceIDs)
	}
	if len(failure) != 1 || failure[0].EC2Instance != nil || failure[0].EC2InstanceID != "i-0abc123def4567890" {
		t.Fatalf("expected EC2 lookup failure to omit only nested service-task host detail and preserve host identity, got %+v", failure)
	}
}

func TestECSContainerInstanceEC2IDsOmitFargateAndPartialFailures(t *testing.T) {
	const clusterARN = "arn:aws:ecs:ap-northeast-2:123456789012:cluster/worker"
	if got := ecsTaskSummaryFromTask(ecstypes.Task{LaunchType: ecstypes.LaunchTypeFargate}, clusterARN, nil, nil); got.EC2InstanceID != "" {
		t.Fatalf("expected Fargate task without container instance to omit EC2 host identity, got %+v", got)
	}

	client := &fakeECSServiceAPI{describeContainerInstancesErr: resourceaws.NewUnavailableError(resourceaws.UnavailableReasonAWSUnavailable)}
	got := ecsContainerInstanceEC2IDs(context.Background(), client, clusterARN, []string{"arn:aws:ecs:ap-northeast-2:123456789012:container-instance/worker/missing"})
	if len(got) != 0 {
		t.Fatalf("expected DescribeContainerInstances failure to omit optional host identity, got %+v", got)
	}
}

func TestECSTaskDefinitionImageUsesOnlySingleDeterministicImage(t *testing.T) {
	if got := ecsTaskDefinitionImage(&ecstypes.TaskDefinition{
		ContainerDefinitions: []ecstypes.ContainerDefinition{
			{Image: stringPtr("ghcr.io/devopsartfactory/payments-api:2026-04-18.2")},
			{Image: stringPtr("ghcr.io/devopsartfactory/payments-api:2026-04-18.2")},
		},
	}); got != "ghcr.io/devopsartfactory/payments-api:2026-04-18.2" {
		t.Fatalf("expected single deduplicated task-definition image, got %q", got)
	}

	if got := ecsTaskDefinitionImage(&ecstypes.TaskDefinition{
		ContainerDefinitions: []ecstypes.ContainerDefinition{
			{Image: stringPtr("ghcr.io/devopsartfactory/payments-api:2026-04-18.2")},
			{Image: stringPtr("public.ecr.aws/aws-observability/aws-otel-collector:v0.40.0")},
		},
	}); got != "" {
		t.Fatalf("expected multiple live container images to suppress the narrow image contract, got %q", got)
	}
}

func TestAutoScalingInspectionFromGroupSortsFieldsAndMapsLaunchTemplates(t *testing.T) {
	got := autoScalingInspectionFromGroup(asgtypes.AutoScalingGroup{
		AutoScalingGroupARN:  stringPtr("arn:asg"),
		AutoScalingGroupName: stringPtr("payments-api-prod-candidate"),
		LaunchTemplate: &asgtypes.LaunchTemplateSpecification{
			LaunchTemplateId:   stringPtr("lt-abc123"),
			LaunchTemplateName: stringPtr("payments-api-prod-candidate"),
			Version:            stringPtr("$Default"),
		},
		MixedInstancesPolicy: &asgtypes.MixedInstancesPolicy{
			LaunchTemplate: &asgtypes.LaunchTemplate{
				LaunchTemplateSpecification: &asgtypes.LaunchTemplateSpecification{
					LaunchTemplateId:   stringPtr("lt-base123"),
					LaunchTemplateName: stringPtr("payments-api-prod-candidate-base"),
					Version:            stringPtr("7"),
				},
				Overrides: []asgtypes.LaunchTemplateOverrides{
					{
						InstanceType:     stringPtr("c7g.large"),
						WeightedCapacity: stringPtr("2"),
						LaunchTemplateSpecification: &asgtypes.LaunchTemplateSpecification{
							LaunchTemplateId:   stringPtr("lt-arm123"),
							LaunchTemplateName: stringPtr("payments-api-prod-candidate-arm"),
							Version:            stringPtr("3"),
						},
					},
					{
						InstanceType: stringPtr("m6i.large"),
					},
				},
			},
		},
		TargetGroupARNs: []string{
			"arn:tg/b",
			"arn:tg/a",
		},
		Instances: []asgtypes.Instance{
			{InstanceId: stringPtr("i-b")},
			{InstanceId: stringPtr("i-a")},
		},
	})

	if !reflect.DeepEqual(got.TargetGroupARNs, []string{"arn:tg/a", "arn:tg/b"}) {
		t.Fatalf("expected sorted target groups, got %+v", got.TargetGroupARNs)
	}
	if len(got.Instances) != 2 || got.Instances[0].InstanceID != "i-a" {
		t.Fatalf("expected instances sorted by id, got %+v", got.Instances)
	}
	if !reflect.DeepEqual(got.LaunchTemplate, &AutoScalingLaunchTemplateSummary{
		ID:      "lt-abc123",
		Name:    "payments-api-prod-candidate",
		Version: "$Default",
	}) {
		t.Fatalf("expected direct launch template metadata, got %+v", got.LaunchTemplate)
	}
	if got.MixedInstancesPolicy == nil {
		t.Fatalf("expected mixed instances policy metadata, got %+v", got)
	}
	if !reflect.DeepEqual(got.MixedInstancesPolicy.LaunchTemplate, &AutoScalingLaunchTemplateSummary{
		ID:      "lt-base123",
		Name:    "payments-api-prod-candidate-base",
		Version: "7",
	}) {
		t.Fatalf("expected mixed instances base launch template metadata, got %+v", got.MixedInstancesPolicy.LaunchTemplate)
	}
	if !reflect.DeepEqual(got.MixedInstancesPolicy.Overrides, []AutoScalingLaunchTemplateOverride{
		{
			InstanceType:     "c7g.large",
			WeightedCapacity: "2",
			LaunchTemplate: &AutoScalingLaunchTemplateSummary{
				ID:      "lt-arm123",
				Name:    "payments-api-prod-candidate-arm",
				Version: "3",
			},
		},
		{
			InstanceType: "m6i.large",
		},
	}) {
		t.Fatalf("expected mixed instances overrides, got %+v", got.MixedInstancesPolicy.Overrides)
	}
}

func TestAutoScalingObservedLaunchTemplateFromGroupUsesPrimaryLaunchTemplate(t *testing.T) {
	client := &fakeEC2LaunchTemplateAPI{
		output: &ec2.DescribeLaunchTemplateVersionsOutput{
			LaunchTemplateVersions: []ec2types.LaunchTemplateVersion{
				{
					LaunchTemplateData: &ec2types.ResponseLaunchTemplateData{
						ImageId:      stringPtr("ami-abc123"),
						InstanceType: ec2types.InstanceTypeM6iLarge,
						UserData:     stringPtr(base64.StdEncoding.EncodeToString([]byte("export TENET_BOOTSTRAP_SOURCE=file:userdata/payments-api-prod.sh"))),
					},
				},
			},
		},
	}

	got, bootstrapSource, err := autoScalingObservedLaunchTemplateFromGroup(context.Background(), client, asgtypes.AutoScalingGroup{
		LaunchTemplate: &asgtypes.LaunchTemplateSpecification{
			LaunchTemplateId:   stringPtr("lt-abc123"),
			LaunchTemplateName: stringPtr("payments-api-prod-candidate"),
			Version:            stringPtr("$Default"),
		},
	})
	if err != nil {
		t.Fatalf("autoScalingObservedLaunchTemplateFromGroup returned error: %v", err)
	}
	if !reflect.DeepEqual(got, &AutoScalingLaunchTemplateSummary{
		ID:           "lt-abc123",
		Name:         "payments-api-prod-candidate",
		Version:      "$Default",
		ImageID:      "ami-abc123",
		InstanceType: "m6i.large",
	}) {
		t.Fatalf("expected observed launch template summary, got %+v", got)
	}
	if bootstrapSource != "file:userdata/payments-api-prod.sh" {
		t.Fatalf("expected parsed launch template bootstrap source, got %q", bootstrapSource)
	}
	if client.input == nil || aws.ToString(client.input.LaunchTemplateId) != "lt-abc123" || !reflect.DeepEqual(client.input.Versions, []string{"$Default"}) {
		t.Fatalf("unexpected launch template lookup input: %+v", client.input)
	}
}

func TestEnrichSelectedAutoScalingInstanceFromEC2(t *testing.T) {
	launchTime := time.Date(2026, 5, 5, 7, 30, 0, 0, time.UTC)
	client := &fakeEC2InstanceAPI{
		output: &ec2.DescribeInstancesOutput{
			Reservations: []ec2types.Reservation{{
				Instances: []ec2types.Instance{{
					InstanceId:       stringPtr("i-0abc123def4567890"),
					State:            &ec2types.InstanceState{Name: ec2types.InstanceStateNameRunning},
					InstanceType:     ec2types.InstanceTypeM6iLarge,
					ImageId:          stringPtr("ami-abc123"),
					PrivateIpAddress: stringPtr("10.0.1.24"),
					PublicIpAddress:  stringPtr("203.0.113.24"),
					SubnetId:         stringPtr("subnet-abc123"),
					VpcId:            stringPtr("vpc-abc123"),
					LaunchTime:       &launchTime,
				}},
			}},
		},
	}
	detail := AutoScalingInspection{Instances: []AutoScalingGroupInstanceSummary{
		{InstanceID: "i-0other"},
		{InstanceID: "i-0abc123def4567890", AvailabilityZone: "ap-northeast-2a", LifecycleState: "InService"},
	}}

	got := enrichSelectedAutoScalingInstanceFromEC2(context.Background(), client, detail, " i-0abc123def4567890 ")
	if !reflect.DeepEqual(client.instanceIDs, []string{"i-0abc123def4567890"}) {
		t.Fatalf("expected selected EC2 DescribeInstances call, got %+v", client.instanceIDs)
	}
	if got.Instances[0].EC2Instance != nil {
		t.Fatalf("expected omitted EC2 detail on non-selected sample row, got %+v", got.Instances[0])
	}
	selected := got.Instances[1].EC2Instance
	if selected == nil {
		t.Fatalf("expected selected instance EC2 detail, got %+v", got.Instances[1])
	}
	if selected.InstanceState != "running" || selected.InstanceType != "m6i.large" || selected.ImageID != "ami-abc123" || selected.PrivateIPAddress != "10.0.1.24" || selected.PublicIPAddress != "203.0.113.24" || selected.SubnetID != "subnet-abc123" || selected.VPCID != "vpc-abc123" || selected.LaunchTime != "2026-05-05T07:30:00Z" {
		t.Fatalf("unexpected selected EC2 detail: %+v", selected)
	}
}

func TestEnrichSelectedAutoScalingInstanceFromEC2PreservesASGOnFailure(t *testing.T) {
	client := &fakeEC2InstanceAPI{err: resourceaws.NewUnavailableError(resourceaws.UnavailableReasonAWSUnavailable)}
	detail := AutoScalingInspection{Instances: []AutoScalingGroupInstanceSummary{{InstanceID: "i-0abc123def4567890", LifecycleState: "InService"}}}

	got := enrichSelectedAutoScalingInstanceFromEC2(context.Background(), client, detail, "i-0abc123def4567890")
	if !reflect.DeepEqual(got, detail) {
		t.Fatalf("expected ASG membership detail to remain available when EC2 detail lookup fails, got %+v", got)
	}
	if !reflect.DeepEqual(client.instanceIDs, []string{"i-0abc123def4567890"}) {
		t.Fatalf("expected one EC2 lookup attempt before preserving ASG detail, got %+v", client.instanceIDs)
	}
}

func TestEnrichSelectedAutoScalingInstanceFromEC2SkipsOmittedOrMissingSelector(t *testing.T) {
	client := &fakeEC2InstanceAPI{}
	detail := AutoScalingInspection{Instances: []AutoScalingGroupInstanceSummary{{InstanceID: "i-0present123456789"}}}

	if got := enrichSelectedAutoScalingInstanceFromEC2(context.Background(), client, detail, ""); !reflect.DeepEqual(got, detail) {
		t.Fatalf("expected omitted selector to preserve bounded ASG sample, got %+v", got)
	}
	if got := enrichSelectedAutoScalingInstanceFromEC2(context.Background(), client, detail, "i-0missing12345678"); !reflect.DeepEqual(got, detail) {
		t.Fatalf("expected non-member selector to preserve ASG evidence for caller-level unavailable handling, got %+v", got)
	}
	if len(client.instanceIDs) != 0 {
		t.Fatalf("expected omitted/missing selectors to skip EC2 calls, got %+v", client.instanceIDs)
	}
}

func TestExtractLaunchTemplateBootstrapSourceSupportsCommonMarkers(t *testing.T) {
	t.Run("environment assignment", func(t *testing.T) {
		encoded := base64.StdEncoding.EncodeToString([]byte("#!/bin/bash\nexport TENET_BOOTSTRAP_SOURCE=file:userdata/api-prod.sh\n"))
		if got := extractLaunchTemplateBootstrapSource(encoded); got != "file:userdata/api-prod.sh" {
			t.Fatalf("expected environment assignment marker to decode, got %q", got)
		}
	})

	t.Run("comment marker", func(t *testing.T) {
		encoded := base64.StdEncoding.EncodeToString([]byte("# tenet bootstrap source: git:userdata/api-prod.sh\n"))
		if got := extractLaunchTemplateBootstrapSource(encoded); got != "git:userdata/api-prod.sh" {
			t.Fatalf("expected comment marker to decode, got %q", got)
		}
	})

	t.Run("unparseable body stays empty", func(t *testing.T) {
		encoded := base64.StdEncoding.EncodeToString([]byte("#!/bin/bash\necho bootstrapping\n"))
		if got := extractLaunchTemplateBootstrapSource(encoded); got != "" {
			t.Fatalf("expected unmatched launch template user data to stay empty, got %q", got)
		}
	})
}

func TestFindECSServiceReusesCachedClusterHintOnRepeatedLookup(t *testing.T) {
	const (
		region          = "ap-northeast-2"
		targetAccountID = "123456789012"
		serviceName     = "payments-api-prod-service"
		clusterA        = "arn:aws:ecs:ap-northeast-2:123456789012:cluster/a"
		clusterB        = "arn:aws:ecs:ap-northeast-2:123456789012:cluster/b"
	)

	hints := newECSServiceClusterHintCache(4)
	key := newECSServiceClusterHintKey(region, targetAccountID, "", serviceName)
	client := &fakeECSServiceAPI{
		clusterARNs: []string{clusterA, clusterB},
		servicesByCluster: map[string][]ecstypes.Service{
			clusterB: {{ServiceName: stringPtr(serviceName), ServiceArn: stringPtr("arn:service/b")}},
		},
	}

	service, clusterARN, err := findECSService(context.Background(), client, serviceName, "", hints, key)
	if err != nil {
		t.Fatalf("first lookup returned error: %v", err)
	}
	if service == nil || aws.ToString(clusterARN) != clusterB {
		t.Fatalf("expected first lookup to find %s in %s, got service=%+v cluster=%q", serviceName, clusterB, service, aws.ToString(clusterARN))
	}
	if client.listCalls != 1 {
		t.Fatalf("expected first lookup to scan clusters once, got %d", client.listCalls)
	}
	if !reflect.DeepEqual(client.describeCalls, []string{clusterA, clusterB}) {
		t.Fatalf("expected first lookup to describe both clusters, got %+v", client.describeCalls)
	}
	if cachedClusterARN, ok := hints.Get(key); !ok || cachedClusterARN != clusterB {
		t.Fatalf("expected cache to store %s, got %q present=%v", clusterB, cachedClusterARN, ok)
	}

	service, clusterARN, err = findECSService(context.Background(), client, serviceName, "", hints, key)
	if err != nil {
		t.Fatalf("second lookup returned error: %v", err)
	}
	if service == nil || aws.ToString(clusterARN) != clusterB {
		t.Fatalf("expected second lookup to find cached service in %s, got service=%+v cluster=%q", clusterB, service, aws.ToString(clusterARN))
	}
	if client.listCalls != 1 {
		t.Fatalf("expected cached lookup to avoid a second cluster scan, got %d list calls", client.listCalls)
	}
	if !reflect.DeepEqual(client.describeCalls, []string{clusterA, clusterB, clusterB}) {
		t.Fatalf("expected second lookup to reuse cached cluster only, got %+v", client.describeCalls)
	}
}

func TestFindECSServiceUsesSelectedClusterWithoutScanning(t *testing.T) {
	const (
		region          = "ap-northeast-2"
		targetAccountID = "123456789012"
		serviceName     = "payments-api-prod-service"
		selectedCluster = "arn:aws:ecs:ap-northeast-2:123456789012:cluster/selected"
		otherCluster    = "arn:aws:ecs:ap-northeast-2:123456789012:cluster/other"
	)

	hints := newECSServiceClusterHintCache(4)
	key := newECSServiceClusterHintKey(region, targetAccountID, "", serviceName)
	hints.Set(key, otherCluster)
	client := &fakeECSServiceAPI{
		clusterARNs: []string{otherCluster},
		servicesByCluster: map[string][]ecstypes.Service{
			selectedCluster: {{ServiceName: stringPtr(serviceName), ServiceArn: stringPtr("arn:service/selected")}},
			otherCluster:    {{ServiceName: stringPtr(serviceName), ServiceArn: stringPtr("arn:service/other")}},
		},
	}

	service, clusterARN, err := findECSService(context.Background(), client, serviceName, selectedCluster, hints, key)
	if err != nil {
		t.Fatalf("lookup returned error: %v", err)
	}
	if service == nil || aws.ToString(clusterARN) != selectedCluster {
		t.Fatalf("expected selected-cluster lookup to find %s, got service=%+v cluster=%q", selectedCluster, service, aws.ToString(clusterARN))
	}
	if client.listCalls != 0 {
		t.Fatalf("expected selected-cluster lookup to avoid cluster scans, got %d", client.listCalls)
	}
	if !reflect.DeepEqual(client.describeCalls, []string{selectedCluster}) {
		t.Fatalf("expected selected cluster to be described exactly once, got %+v", client.describeCalls)
	}
	if cachedClusterARN, ok := hints.Get(key); !ok || cachedClusterARN != otherCluster {
		t.Fatalf("expected selected-cluster lookup to leave existing hint untouched, got %q present=%v", cachedClusterARN, ok)
	}
}

func TestFindECSServiceClearsStaleClusterHintAndRecovers(t *testing.T) {
	const (
		region          = "ap-northeast-2"
		targetAccountID = "123456789012"
		serviceName     = "payments-api-prod-service"
		staleCluster    = "arn:aws:ecs:ap-northeast-2:123456789012:cluster/stale"
		liveCluster     = "arn:aws:ecs:ap-northeast-2:123456789012:cluster/live"
	)

	hints := newECSServiceClusterHintCache(4)
	key := newECSServiceClusterHintKey(region, targetAccountID, "", serviceName)
	hints.Set(key, staleCluster)
	client := &fakeECSServiceAPI{
		clusterARNs: []string{liveCluster},
		servicesByCluster: map[string][]ecstypes.Service{
			liveCluster: {{ServiceName: stringPtr(serviceName), ServiceArn: stringPtr("arn:service/live")}},
		},
	}

	service, clusterARN, err := findECSService(context.Background(), client, serviceName, "", hints, key)
	if err != nil {
		t.Fatalf("lookup returned error: %v", err)
	}
	if service == nil || aws.ToString(clusterARN) != liveCluster {
		t.Fatalf("expected lookup to recover in %s, got service=%+v cluster=%q", liveCluster, service, aws.ToString(clusterARN))
	}
	if client.listCalls != 1 {
		t.Fatalf("expected stale hint recovery to fall back to one cluster scan, got %d", client.listCalls)
	}
	if !reflect.DeepEqual(client.describeCalls, []string{staleCluster, liveCluster}) {
		t.Fatalf("expected stale cluster miss followed by refreshed lookup, got %+v", client.describeCalls)
	}
	if cachedClusterARN, ok := hints.Get(key); !ok || cachedClusterARN != liveCluster {
		t.Fatalf("expected stale cache entry to refresh to %s, got %q present=%v", liveCluster, cachedClusterARN, ok)
	}
}

type fakeECSServiceAPI struct {
	clusterARNs                    []string
	servicesByCluster              map[string][]ecstypes.Service
	taskARNsByDesired              map[ecstypes.DesiredStatus][]string
	tasksByARN                     map[string]ecstypes.Task
	taskDefinitions                map[string]ecstypes.TaskDefinition
	containerInstancesByARN        map[string]ecstypes.ContainerInstance
	describeTaskDefinitionErr      error
	listTasksErr                   error
	describeTasksErr               error
	describeContainerInstancesErr  error
	listCalls                      int
	describeCalls                  []string
	listTaskCalls                  []ecstypes.DesiredStatus
	describeTaskInputs             [][]string
	describeContainerInstanceInput [][]string
}

type fakeEC2LaunchTemplateAPI struct {
	input  *ec2.DescribeLaunchTemplateVersionsInput
	output *ec2.DescribeLaunchTemplateVersionsOutput
	err    error
}

type fakeEC2InstanceAPI struct {
	instanceIDs []string
	output      *ec2.DescribeInstancesOutput
	err         error
}

func (f *fakeECSServiceAPI) ListClusters(context.Context, *ecs.ListClustersInput, ...func(*ecs.Options)) (*ecs.ListClustersOutput, error) {
	f.listCalls++
	return &ecs.ListClustersOutput{ClusterArns: append([]string(nil), f.clusterARNs...)}, nil
}

func (f *fakeECSServiceAPI) DescribeServices(_ context.Context, input *ecs.DescribeServicesInput, _ ...func(*ecs.Options)) (*ecs.DescribeServicesOutput, error) {
	clusterARN := aws.ToString(input.Cluster)
	f.describeCalls = append(f.describeCalls, clusterARN)
	services := append([]ecstypes.Service(nil), f.servicesByCluster[clusterARN]...)
	return &ecs.DescribeServicesOutput{Services: services}, nil
}

func (f *fakeECSServiceAPI) ListTasks(_ context.Context, input *ecs.ListTasksInput, _ ...func(*ecs.Options)) (*ecs.ListTasksOutput, error) {
	f.listTaskCalls = append(f.listTaskCalls, input.DesiredStatus)
	if f.listTasksErr != nil {
		return nil, f.listTasksErr
	}
	return &ecs.ListTasksOutput{TaskArns: append([]string(nil), f.taskARNsByDesired[input.DesiredStatus]...)}, nil
}

func (f *fakeECSServiceAPI) DescribeTasks(_ context.Context, input *ecs.DescribeTasksInput, _ ...func(*ecs.Options)) (*ecs.DescribeTasksOutput, error) {
	if f.describeTasksErr != nil {
		return nil, f.describeTasksErr
	}
	tasks := make([]ecstypes.Task, 0, len(input.Tasks))
	f.describeTaskInputs = append(f.describeTaskInputs, append([]string(nil), input.Tasks...))
	for _, taskARN := range input.Tasks {
		if task, ok := f.tasksByARN[taskARN]; ok {
			tasks = append(tasks, task)
		}
	}
	return &ecs.DescribeTasksOutput{Tasks: tasks}, nil
}

func (f *fakeECSServiceAPI) DescribeTaskDefinition(_ context.Context, input *ecs.DescribeTaskDefinitionInput, _ ...func(*ecs.Options)) (*ecs.DescribeTaskDefinitionOutput, error) {
	if f.describeTaskDefinitionErr != nil {
		return nil, f.describeTaskDefinitionErr
	}
	definitionARN := aws.ToString(input.TaskDefinition)
	if definition, ok := f.taskDefinitions[definitionARN]; ok {
		copy := definition
		return &ecs.DescribeTaskDefinitionOutput{TaskDefinition: &copy}, nil
	}
	return &ecs.DescribeTaskDefinitionOutput{}, nil
}

func (f *fakeECSServiceAPI) DescribeContainerInstances(_ context.Context, input *ecs.DescribeContainerInstancesInput, _ ...func(*ecs.Options)) (*ecs.DescribeContainerInstancesOutput, error) {
	f.describeContainerInstanceInput = append(f.describeContainerInstanceInput, append([]string(nil), input.ContainerInstances...))
	if f.describeContainerInstancesErr != nil {
		return nil, f.describeContainerInstancesErr
	}
	instances := make([]ecstypes.ContainerInstance, 0, len(input.ContainerInstances))
	for _, arn := range input.ContainerInstances {
		if instance, ok := f.containerInstancesByARN[arn]; ok {
			instances = append(instances, instance)
		}
	}
	return &ecs.DescribeContainerInstancesOutput{ContainerInstances: instances}, nil
}

func (f *fakeEC2LaunchTemplateAPI) DescribeLaunchTemplateVersions(_ context.Context, input *ec2.DescribeLaunchTemplateVersionsInput, _ ...func(*ec2.Options)) (*ec2.DescribeLaunchTemplateVersionsOutput, error) {
	cloned := *input
	cloned.Versions = append([]string(nil), input.Versions...)
	f.input = &cloned
	if f.err != nil {
		return nil, f.err
	}
	if f.output == nil {
		return &ec2.DescribeLaunchTemplateVersionsOutput{}, nil
	}
	return f.output, nil
}

func (f *fakeEC2InstanceAPI) DescribeInstances(_ context.Context, input *ec2.DescribeInstancesInput, _ ...func(*ec2.Options)) (*ec2.DescribeInstancesOutput, error) {
	f.instanceIDs = append([]string(nil), input.InstanceIds...)
	if f.err != nil {
		return nil, f.err
	}
	if f.output == nil {
		return &ec2.DescribeInstancesOutput{}, nil
	}
	return f.output, nil
}

func stringPtr(value string) *string { return &value }

func int32Ptr(value int32) *int32 { return &value }

func timePtr(value time.Time) *time.Time { return &value }
