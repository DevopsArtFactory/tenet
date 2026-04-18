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
	})

	t.Run("unsupported resource type returns unavailable payload", func(t *testing.T) {
		service := NewService(Options{})

		result, err := service.Inspect(context.Background(), Request{
			DeploymentName:   "payments-api-prod",
			ResourceType:     "trafficShiftPlan",
			ResourceName:     "payments-api-prod-weighted-routing",
			ControlAccountID: "123456789012",
			TargetAccountID:  "123456789012",
			Region:           "ap-northeast-2",
		})
		if err != nil {
			t.Fatalf("Inspect returned error: %v", err)
		}
		want := Result{
			DeploymentName:    "payments-api-prod",
			ResourceType:      "trafficShiftPlan",
			ResourceName:      "payments-api-prod-weighted-routing",
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

func TestAutoScalingInspectionFromGroupSortsFields(t *testing.T) {
	got := autoScalingInspectionFromGroup(asgtypes.AutoScalingGroup{
		AutoScalingGroupARN:  stringPtr("arn:asg"),
		AutoScalingGroupName: stringPtr("payments-api-prod-candidate"),
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
