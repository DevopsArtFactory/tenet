package resourceinspection

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	asgtypes "github.com/aws/aws-sdk-go-v2/service/autoscaling/types"
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
					ClusterARN:     "arn:aws:ecs:ap-northeast-2:123456789012:cluster/payments",
					ServiceARN:     "arn:aws:ecs:ap-northeast-2:123456789012:service/payments/payments-api-prod-service",
					ServiceName:    "payments-api-prod-service",
					Status:         "ACTIVE",
					LaunchType:     "FARGATE",
					DesiredCount:   2,
					RunningCount:   2,
					PendingCount:   0,
					TaskDefinition: "payments-api:42",
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
		})
		if err != nil {
			t.Fatalf("Inspect returned error: %v", err)
		}
		if !reflect.DeepEqual(gotLookup, ECSRequest{
			DeploymentName:  "payments-api-prod",
			ResourceName:    "payments-api-prod-service",
			Region:          "ap-northeast-2",
			TargetAccountID: "123456789012",
		}) {
			t.Fatalf("unexpected ECS lookup request: %+v", gotLookup)
		}
		want := Result{
			DeploymentName: "payments-api-prod",
			ResourceType:   "ecsService",
			ResourceName:   "payments-api-prod-service",
			LookupContext:  expectedLookupContext("ap-northeast-2", "123456789012", "123456789012", ""),
			Available:      true,
			Source:         SourceECSService,
			ECSService: &ECSServiceInspection{
				ClusterARN:     "arn:aws:ecs:ap-northeast-2:123456789012:cluster/payments",
				ServiceARN:     "arn:aws:ecs:ap-northeast-2:123456789012:service/payments/payments-api-prod-service",
				ServiceName:    "payments-api-prod-service",
				Status:         "ACTIVE",
				LaunchType:     "FARGATE",
				DesiredCount:   2,
				RunningCount:   2,
				PendingCount:   0,
				TaskDefinition: "payments-api:42",
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
						ID:      "lt-abc123",
						Name:    "payments-api-prod-candidate",
						Version: "$Default",
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

	t.Run("traffic routing resources return explicit summary payloads when AWS inspection is disabled", func(t *testing.T) {
		service := NewService(Options{})
		for _, tc := range []struct {
			resourceType string
			resourceName string
			displayName  string
			summary      string
		}{
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
		}
		if !reflect.DeepEqual(calls, []call{
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
		}) {
			t.Fatalf("unexpected traffic routing provider calls: %+v", calls)
		}
	})

	t.Run("unsupported resource type returns unavailable payload", func(t *testing.T) {
		service := NewService(Options{})

		result, err := service.Inspect(context.Background(), Request{
			DeploymentName:   "payments-api-prod",
			ResourceType:     "productionListenerRule",
			ResourceName:     "payments-api-prod-prod-listener",
			ControlAccountID: "123456789012",
			TargetAccountID:  "123456789012",
			Region:           "ap-northeast-2",
		})
		if err != nil {
			t.Fatalf("Inspect returned error: %v", err)
		}
		want := Result{
			DeploymentName:    "payments-api-prod",
			ResourceType:      "productionListenerRule",
			ResourceName:      "payments-api-prod-prod-listener",
			LookupContext:     expectedLookupContext("ap-northeast-2", "123456789012", "123456789012", ""),
			Available:         false,
			UnavailableReason: "unsupported_resource_type",
		}
		if !reflect.DeepEqual(result, want) {
			t.Fatalf("unexpected unsupported inspection result: %+v", result)
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
		})
		if err != nil {
			t.Fatalf("Inspect returned error: %v", err)
		}
		if result.UnavailableReason != "aws_unavailable" || result.Available {
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
		ServiceArn:     stringPtr("arn:service"),
		ServiceName:    stringPtr("payments-api-prod-service"),
		Status:         stringPtr("ACTIVE"),
		TaskDefinition: stringPtr("payments-api:42"),
		DesiredCount:   2,
		RunningCount:   2,
		PendingCount:   0,
		Deployments: []ecstypes.Deployment{
			{Id: stringPtr("secondary"), CreatedAt: timePtr(time.Date(2026, 4, 18, 0, 10, 0, 0, time.UTC))},
			{Id: stringPtr("primary"), CreatedAt: timePtr(time.Date(2026, 4, 18, 0, 11, 0, 0, time.UTC))},
		},
		Events: []ecstypes.ServiceEvent{
			{CreatedAt: timePtr(time.Date(2026, 4, 18, 0, 12, 0, 0, time.UTC)), Message: stringPtr("steady")},
			{CreatedAt: timePtr(time.Date(2026, 4, 18, 0, 10, 0, 0, time.UTC)), Message: stringPtr("started")},
		},
	}

	got := ecsInspectionFromService(service, "arn:cluster")
	if len(got.Deployments) != 2 || got.Deployments[0].ID != "primary" {
		t.Fatalf("expected deployments sorted newest-first, got %+v", got.Deployments)
	}
	if len(got.Events) != 2 || got.Events[0].Message != "steady" {
		t.Fatalf("expected events sorted newest-first, got %+v", got.Events)
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

	service, clusterARN, err := findECSService(context.Background(), client, serviceName, hints, key)
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

	service, clusterARN, err = findECSService(context.Background(), client, serviceName, hints, key)
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

	service, clusterARN, err := findECSService(context.Background(), client, serviceName, hints, key)
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
	clusterARNs       []string
	servicesByCluster map[string][]ecstypes.Service
	listCalls         int
	describeCalls     []string
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

func stringPtr(value string) *string { return &value }

func timePtr(value time.Time) *time.Time { return &value }
