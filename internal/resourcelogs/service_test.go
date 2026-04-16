package resourcelogs

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/aws/smithy-go"
)

func TestServiceRecent(t *testing.T) {
	t.Run("ecs service returns newest cloudwatch log events", func(t *testing.T) {
		var gotLookup ECSRequest
		service := NewService(Options{
			MaxEntries: 3,
			ListECSLogEvents: func(ctx context.Context, req ECSRequest) ([]CloudWatchEvent, error) {
				gotLookup = req
				return []CloudWatchEvent{
					{Timestamp: time.Date(2026, 4, 16, 7, 8, 0, 0, time.UTC), Message: "older event", Stream: "ecs/payments-api/old-task"},
					{Timestamp: time.Date(2026, 4, 16, 7, 11, 0, 0, time.UTC), Message: "newest event", Stream: "ecs/payments-api/new-task"},
					{Timestamp: time.Date(2026, 4, 16, 7, 9, 0, 0, time.UTC), Message: "middle event"},
					{Timestamp: time.Date(2026, 4, 16, 7, 10, 0, 0, time.UTC), Message: "recent event", Stream: "ecs/payments-api/new-task"},
				}, nil
			},
		})

		result, err := service.Recent(context.Background(), Request{
			DeploymentName:   "payments-api-prod",
			ResourceType:     "ecsService",
			ResourceName:     "payments-api-prod-service",
			ControlAccountID: "123456789012",
			TargetAccountID:  "123456789012",
			Region:           "ap-northeast-2",
		})
		if err != nil {
			t.Fatalf("Recent returned error: %v", err)
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
			Available:      true,
			Source:         SourceCloudWatch,
			Entries: []Entry{
				{Timestamp: "2026-04-16T07:11:00Z", Message: "newest event", Stream: "ecs/payments-api/new-task"},
				{Timestamp: "2026-04-16T07:10:00Z", Message: "recent event", Stream: "ecs/payments-api/new-task"},
				{Timestamp: "2026-04-16T07:09:00Z", Message: "middle event"},
			},
			UnavailableReason: "",
		}
		if !reflect.DeepEqual(result, want) {
			t.Fatalf("unexpected resource logs result: %+v", result)
		}
	})

	t.Run("ecs service preserves source order for equal-timestamp cloudwatch events", func(t *testing.T) {
		service := NewService(Options{
			MaxEntries: 3,
			ListECSLogEvents: func(ctx context.Context, req ECSRequest) ([]CloudWatchEvent, error) {
				return []CloudWatchEvent{
					{Timestamp: time.Date(2026, 4, 16, 7, 11, 0, 0, time.UTC), Message: "first event", Stream: "ecs/payments-api/task-a"},
					{Timestamp: time.Date(2026, 4, 16, 7, 11, 0, 0, time.UTC), Message: "second event", Stream: "ecs/payments-api/task-b"},
					{Timestamp: time.Date(2026, 4, 16, 7, 10, 0, 0, time.UTC), Message: "older event", Stream: "ecs/payments-api/task-c"},
				}, nil
			},
		})

		result, err := service.Recent(context.Background(), Request{
			DeploymentName:   "payments-api-prod",
			ResourceType:     "ecsService",
			ResourceName:     "payments-api-prod-service",
			ControlAccountID: "123456789012",
			TargetAccountID:  "123456789012",
			Region:           "ap-northeast-2",
		})
		if err != nil {
			t.Fatalf("Recent returned error: %v", err)
		}
		want := []Entry{
			{Timestamp: "2026-04-16T07:11:00Z", Message: "first event", Stream: "ecs/payments-api/task-a"},
			{Timestamp: "2026-04-16T07:11:00Z", Message: "second event", Stream: "ecs/payments-api/task-b"},
			{Timestamp: "2026-04-16T07:10:00Z", Message: "older event", Stream: "ecs/payments-api/task-c"},
		}
		if !reflect.DeepEqual(result.Entries, want) {
			t.Fatalf("unexpected cloudwatch entry order: %+v", result.Entries)
		}
	})

	t.Run("ecs service returns empty-but-available payload when no formatted log entries remain", func(t *testing.T) {
		service := NewService(Options{
			MaxEntries: 3,
			ListECSLogEvents: func(ctx context.Context, req ECSRequest) ([]CloudWatchEvent, error) {
				return []CloudWatchEvent{{Timestamp: time.Date(2026, 4, 16, 7, 11, 0, 0, time.UTC), Message: "   ", Stream: "ecs/payments-api/new-task"}}, nil
			},
		})

		result, err := service.Recent(context.Background(), Request{
			DeploymentName:   "payments-api-prod",
			ResourceType:     "ecsService",
			ResourceName:     "payments-api-prod-service",
			ControlAccountID: "123456789012",
			TargetAccountID:  "123456789012",
			Region:           "ap-northeast-2",
		})
		if err != nil {
			t.Fatalf("Recent returned error: %v", err)
		}
		want := Result{
			DeploymentName: "payments-api-prod",
			ResourceType:   "ecsService",
			ResourceName:   "payments-api-prod-service",
			Available:      true,
			Source:         SourceCloudWatch,
			Entries:        []Entry{},
		}
		if !reflect.DeepEqual(result, want) {
			t.Fatalf("unexpected empty ECS resource logs result: %+v", result)
		}
	})

	t.Run("candidate asg returns newest instance console bootstrap output and preserves equal-timestamp line order", func(t *testing.T) {
		var gotLookup ASGRequest
		service := NewService(Options{
			MaxEntries: 3,
			ListASGInstanceConsoles: func(ctx context.Context, req ASGRequest) ([]InstanceConsole, error) {
				gotLookup = req
				return []InstanceConsole{
					{
						InstanceID:  "i-older",
						GeneratedAt: time.Date(2026, 4, 16, 7, 5, 0, 0, time.UTC),
						Lines:       []ConsoleLine{{Timestamp: time.Date(2026, 4, 16, 7, 4, 0, 0, time.UTC), Message: "older bootstrap"}},
					},
					{
						InstanceID:  "i-newest",
						GeneratedAt: time.Date(2026, 4, 16, 7, 12, 0, 0, time.UTC),
						Lines: []ConsoleLine{
							{Timestamp: time.Date(2026, 4, 16, 7, 11, 45, 0, time.UTC), Message: "bootstrap step one"},
							{Timestamp: time.Date(2026, 4, 16, 7, 11, 45, 0, time.UTC), Message: "bootstrap complete"},
							{Timestamp: time.Date(2026, 4, 16, 7, 11, 30, 0, time.UTC), Message: "bootstrap start"},
						},
					},
				}, nil
			},
		})

		result, err := service.Recent(context.Background(), Request{
			DeploymentName:   "payments-api-prod",
			ResourceType:     "candidateAutoScalingGroup",
			ResourceName:     "payments-api-prod-candidate",
			ControlAccountID: "123456789012",
			TargetAccountID:  "123456789012",
			Region:           "ap-northeast-2",
		})
		if err != nil {
			t.Fatalf("Recent returned error: %v", err)
		}
		if !reflect.DeepEqual(gotLookup, ASGRequest{
			DeploymentName:  "payments-api-prod",
			ResourceName:    "payments-api-prod-candidate",
			Region:          "ap-northeast-2",
			TargetAccountID: "123456789012",
		}) {
			t.Fatalf("unexpected ASG lookup request: %+v", gotLookup)
		}
		want := Result{
			DeploymentName: "payments-api-prod",
			ResourceType:   "candidateAutoScalingGroup",
			ResourceName:   "payments-api-prod-candidate",
			Available:      true,
			Source:         SourceEC2Console,
			Entries: []Entry{
				{Timestamp: "2026-04-16T07:11:45Z", Message: "bootstrap step one"},
				{Timestamp: "2026-04-16T07:11:45Z", Message: "bootstrap complete"},
				{Timestamp: "2026-04-16T07:11:30Z", Message: "bootstrap start"},
			},
			UnavailableReason: "",
		}
		if !reflect.DeepEqual(result, want) {
			t.Fatalf("unexpected resource logs result: %+v", result)
		}
	})

	t.Run("candidate asg uses tail of newest non-empty console lines while preserving displayed order", func(t *testing.T) {
		service := NewService(Options{
			MaxEntries: 3,
			ListASGInstanceConsoles: func(ctx context.Context, req ASGRequest) ([]InstanceConsole, error) {
				return []InstanceConsole{
					{
						InstanceID:  "i-older",
						GeneratedAt: time.Date(2026, 4, 16, 7, 5, 0, 0, time.UTC),
						Lines:       []ConsoleLine{{Timestamp: time.Date(2026, 4, 16, 7, 4, 0, 0, time.UTC), Message: "older bootstrap"}},
					},
					{
						InstanceID:  "i-newest",
						GeneratedAt: time.Date(2026, 4, 16, 7, 12, 0, 0, time.UTC),
						Lines: []ConsoleLine{
							{Timestamp: time.Date(2026, 4, 16, 7, 11, 45, 0, time.UTC), Message: "bootstrap preface"},
							{Timestamp: time.Date(2026, 4, 16, 7, 11, 45, 0, time.UTC), Message: "   "},
							{Timestamp: time.Date(2026, 4, 16, 7, 11, 45, 0, time.UTC), Message: "download config"},
							{Timestamp: time.Date(2026, 4, 16, 7, 11, 45, 0, time.UTC), Message: "start services"},
							{Timestamp: time.Date(2026, 4, 16, 7, 11, 45, 0, time.UTC), Message: "bootstrap complete"},
						},
					},
				}, nil
			},
		})

		result, err := service.Recent(context.Background(), Request{
			DeploymentName:   "payments-api-prod",
			ResourceType:     "candidateAutoScalingGroup",
			ResourceName:     "payments-api-prod-candidate",
			ControlAccountID: "123456789012",
			TargetAccountID:  "123456789012",
			Region:           "ap-northeast-2",
		})
		if err != nil {
			t.Fatalf("Recent returned error: %v", err)
		}
		want := Result{
			DeploymentName: "payments-api-prod",
			ResourceType:   "candidateAutoScalingGroup",
			ResourceName:   "payments-api-prod-candidate",
			Available:      true,
			Source:         SourceEC2Console,
			Entries: []Entry{
				{Timestamp: "2026-04-16T07:11:45Z", Message: "download config"},
				{Timestamp: "2026-04-16T07:11:45Z", Message: "start services"},
				{Timestamp: "2026-04-16T07:11:45Z", Message: "bootstrap complete"},
			},
		}
		if !reflect.DeepEqual(result, want) {
			t.Fatalf("unexpected resource logs result: %+v", result)
		}
	})

	t.Run("candidate asg returns empty-but-available payload when newest console has no formatted lines", func(t *testing.T) {
		service := NewService(Options{
			MaxEntries: 3,
			ListASGInstanceConsoles: func(ctx context.Context, req ASGRequest) ([]InstanceConsole, error) {
				return []InstanceConsole{{
					InstanceID:  "i-newest",
					GeneratedAt: time.Date(2026, 4, 16, 7, 12, 0, 0, time.UTC),
					Lines:       []ConsoleLine{{Timestamp: time.Date(2026, 4, 16, 7, 11, 45, 0, time.UTC), Message: "   "}},
				}}, nil
			},
		})

		result, err := service.Recent(context.Background(), Request{
			DeploymentName:   "payments-api-prod",
			ResourceType:     "candidateAutoScalingGroup",
			ResourceName:     "payments-api-prod-candidate",
			ControlAccountID: "123456789012",
			TargetAccountID:  "123456789012",
			Region:           "ap-northeast-2",
		})
		if err != nil {
			t.Fatalf("Recent returned error: %v", err)
		}
		want := Result{
			DeploymentName: "payments-api-prod",
			ResourceType:   "candidateAutoScalingGroup",
			ResourceName:   "payments-api-prod-candidate",
			Available:      true,
			Source:         SourceEC2Console,
			Entries:        []Entry{},
		}
		if !reflect.DeepEqual(result, want) {
			t.Fatalf("unexpected empty ASG resource logs result: %+v", result)
		}
	})

	t.Run("service asg reuses EC2 console lookup", func(t *testing.T) {
		var gotLookup ASGRequest
		service := NewService(Options{
			MaxEntries: 2,
			ListASGInstanceConsoles: func(ctx context.Context, req ASGRequest) ([]InstanceConsole, error) {
				gotLookup = req
				return []InstanceConsole{{
					InstanceID:  "i-refresh",
					GeneratedAt: time.Date(2026, 4, 16, 7, 12, 0, 0, time.UTC),
					Lines: []ConsoleLine{
						{Timestamp: time.Date(2026, 4, 16, 7, 11, 30, 0, time.UTC), Message: "refresh start"},
						{Timestamp: time.Date(2026, 4, 16, 7, 11, 45, 0, time.UTC), Message: "refresh complete"},
					},
				}}, nil
			},
		})

		result, err := service.Recent(context.Background(), Request{
			DeploymentName:   "payments-api-prod",
			ResourceType:     "serviceAutoScalingGroup",
			ResourceName:     "payments-api-prod-service",
			ControlAccountID: "123456789012",
			TargetAccountID:  "123456789012",
			Region:           "ap-northeast-2",
		})
		if err != nil {
			t.Fatalf("Recent returned error: %v", err)
		}
		if !reflect.DeepEqual(gotLookup, ASGRequest{
			DeploymentName:  "payments-api-prod",
			ResourceName:    "payments-api-prod-service",
			Region:          "ap-northeast-2",
			TargetAccountID: "123456789012",
		}) {
			t.Fatalf("unexpected ASG lookup request: %+v", gotLookup)
		}
		want := Result{
			DeploymentName: "payments-api-prod",
			ResourceType:   "serviceAutoScalingGroup",
			ResourceName:   "payments-api-prod-service",
			Available:      true,
			Source:         SourceEC2Console,
			Entries: []Entry{
				{Timestamp: "2026-04-16T07:11:30Z", Message: "refresh start"},
				{Timestamp: "2026-04-16T07:11:45Z", Message: "refresh complete"},
			},
		}
		if !reflect.DeepEqual(result, want) {
			t.Fatalf("unexpected service ASG result: %+v", result)
		}
	})

	t.Run("unsupported resource type returns unavailable payload", func(t *testing.T) {
		service := NewService(Options{})

		result, err := service.Recent(context.Background(), Request{
			DeploymentName:   "payments-api-prod",
			ResourceType:     "loadBalancer",
			ResourceName:     "payments-api-prod-alb",
			ControlAccountID: "123456789012",
			TargetAccountID:  "123456789012",
			Region:           "ap-northeast-2",
		})
		if err != nil {
			t.Fatalf("Recent returned error: %v", err)
		}
		want := Result{
			DeploymentName:    "payments-api-prod",
			ResourceType:      "loadBalancer",
			ResourceName:      "payments-api-prod-alb",
			Available:         false,
			Source:            "",
			Entries:           nil,
			UnavailableReason: UnavailableReasonUnsupportedResourceType,
		}
		if !reflect.DeepEqual(result, want) {
			t.Fatalf("unexpected unavailable result: %+v", result)
		}
	})

	t.Run("cross account without observer role returns unavailable payload", func(t *testing.T) {
		service := NewService(Options{
			ListECSLogEvents: func(ctx context.Context, req ECSRequest) ([]CloudWatchEvent, error) {
				t.Fatalf("cross-account request should not call ECS provider without observer role")
				return nil, nil
			},
		})

		result, err := service.Recent(context.Background(), Request{
			DeploymentName:   "payments-api-prod",
			ResourceType:     "ecsService",
			ResourceName:     "payments-api-prod-service",
			ControlAccountID: "111111111111",
			TargetAccountID:  "222222222222",
			Region:           "ap-northeast-2",
			ObserverRoleARN:  "",
		})
		if err != nil {
			t.Fatalf("Recent returned error: %v", err)
		}
		want := Result{
			DeploymentName:    "payments-api-prod",
			ResourceType:      "ecsService",
			ResourceName:      "payments-api-prod-service",
			Available:         false,
			Source:            "",
			Entries:           nil,
			UnavailableReason: UnavailableReasonCrossAccountNotConfigured,
		}
		if !reflect.DeepEqual(result, want) {
			t.Fatalf("unexpected cross-account unavailable result: %+v", result)
		}
	})

	t.Run("ecs service degrades to unavailable when aws credentials are missing", func(t *testing.T) {
		service := NewService(Options{
			ListECSLogEvents: func(ctx context.Context, req ECSRequest) ([]CloudWatchEvent, error) {
				if req.ResourceName != "payments-api-prod-service" {
					t.Fatalf("unexpected ECS request: %+v", req)
				}
				return nil, &smithy.OperationError{
					ServiceID:     "STS",
					OperationName: "GetCallerIdentity",
					Err:           errors.New("get identity: get credentials: failed to refresh cached credentials, no EC2 IMDS role found"),
				}
			},
		})

		result, err := service.Recent(context.Background(), Request{
			DeploymentName:   "payments-api-prod",
			ResourceType:     "ecsService",
			ResourceName:     "payments-api-prod-service",
			ControlAccountID: "123456789012",
			TargetAccountID:  "123456789012",
			Region:           "ap-northeast-2",
		})
		if err != nil {
			t.Fatalf("Recent returned error: %v", err)
		}
		want := Result{
			DeploymentName:    "payments-api-prod",
			ResourceType:      "ecsService",
			ResourceName:      "payments-api-prod-service",
			Available:         false,
			UnavailableReason: UnavailableReasonAWSUnavailable,
		}
		if !reflect.DeepEqual(result, want) {
			t.Fatalf("unexpected AWS unavailable result: %+v", result)
		}
	})

	t.Run("asg degrades to unavailable when aws access is denied", func(t *testing.T) {
		service := NewService(Options{
			ListASGInstanceConsoles: func(ctx context.Context, req ASGRequest) ([]InstanceConsole, error) {
				if req.ResourceName != "payments-api-prod-candidate" {
					t.Fatalf("unexpected ASG request: %+v", req)
				}
				return nil, &smithy.OperationError{
					ServiceID:     "AutoScaling",
					OperationName: "DescribeAutoScalingGroups",
					Err: &smithy.GenericAPIError{
						Code:    "AccessDeniedException",
						Message: "User is not authorized to perform autoscaling:DescribeAutoScalingGroups",
					},
				}
			},
		})

		result, err := service.Recent(context.Background(), Request{
			DeploymentName:   "payments-api-prod",
			ResourceType:     "candidateAutoScalingGroup",
			ResourceName:     "payments-api-prod-candidate",
			ControlAccountID: "123456789012",
			TargetAccountID:  "123456789012",
			Region:           "ap-northeast-2",
		})
		if err != nil {
			t.Fatalf("Recent returned error: %v", err)
		}
		want := Result{
			DeploymentName:    "payments-api-prod",
			ResourceType:      "candidateAutoScalingGroup",
			ResourceName:      "payments-api-prod-candidate",
			Available:         false,
			UnavailableReason: UnavailableReasonAWSUnavailable,
		}
		if !reflect.DeepEqual(result, want) {
			t.Fatalf("unexpected AWS unavailable result: %+v", result)
		}
	})

	t.Run("ecs service provider failures are returned", func(t *testing.T) {
		providerErr := errors.New("boom: ecs provider internals")
		service := NewService(Options{
			ListECSLogEvents: func(ctx context.Context, req ECSRequest) ([]CloudWatchEvent, error) {
				if req.ResourceName != "payments-api-prod-service" {
					t.Fatalf("unexpected ECS request: %+v", req)
				}
				return nil, providerErr
			},
		})

		result, err := service.Recent(context.Background(), Request{
			DeploymentName:   "payments-api-prod",
			ResourceType:     "ecsService",
			ResourceName:     "payments-api-prod-service",
			ControlAccountID: "123456789012",
			TargetAccountID:  "123456789012",
			Region:           "ap-northeast-2",
		})
		if !errors.Is(err, providerErr) {
			t.Fatalf("expected provider error %v, got %v", providerErr, err)
		}
		if !reflect.DeepEqual(result, Result{}) {
			t.Fatalf("expected zero result on provider failure, got %+v", result)
		}
	})

	t.Run("ecs service without provider hook returns not implemented", func(t *testing.T) {
		service := NewService(Options{})

		result, err := service.Recent(context.Background(), Request{
			DeploymentName:   "payments-api-prod",
			ResourceType:     "ecsService",
			ResourceName:     "payments-api-prod-service",
			ControlAccountID: "123456789012",
			TargetAccountID:  "123456789012",
			Region:           "ap-northeast-2",
		})
		if !errors.Is(err, ErrNotImplemented) {
			t.Fatalf("expected ErrNotImplemented, got %v", err)
		}
		if !reflect.DeepEqual(result, Result{}) {
			t.Fatalf("expected zero result when provider hook is missing, got %+v", result)
		}
	})

	t.Run("asg provider failures are returned", func(t *testing.T) {
		providerErr := errors.New("boom: asg provider internals")
		service := NewService(Options{
			ListASGInstanceConsoles: func(ctx context.Context, req ASGRequest) ([]InstanceConsole, error) {
				if req.ResourceName != "payments-api-prod-candidate" {
					t.Fatalf("unexpected ASG request: %+v", req)
				}
				return nil, providerErr
			},
		})

		result, err := service.Recent(context.Background(), Request{
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
			t.Fatalf("expected zero result on provider failure, got %+v", result)
		}
	})

	t.Run("asg without provider hook returns not implemented", func(t *testing.T) {
		service := NewService(Options{})

		result, err := service.Recent(context.Background(), Request{
			DeploymentName:   "payments-api-prod",
			ResourceType:     "candidateAutoScalingGroup",
			ResourceName:     "payments-api-prod-candidate",
			ControlAccountID: "123456789012",
			TargetAccountID:  "123456789012",
			Region:           "ap-northeast-2",
		})
		if !errors.Is(err, ErrNotImplemented) {
			t.Fatalf("expected ErrNotImplemented, got %v", err)
		}
		if !reflect.DeepEqual(result, Result{}) {
			t.Fatalf("expected zero result when provider hook is missing, got %+v", result)
		}
	})

	t.Run("generic autoScalingGroup remains unsupported", func(t *testing.T) {
		service := NewService(Options{
			ListASGInstanceConsoles: func(ctx context.Context, req ASGRequest) ([]InstanceConsole, error) {
				t.Fatalf("generic autoScalingGroup should stay unsupported")
				return nil, nil
			},
		})

		result, err := service.Recent(context.Background(), Request{
			DeploymentName:   "payments-api-prod",
			ResourceType:     "autoScalingGroup",
			ResourceName:     "payments-api-prod-candidate",
			ControlAccountID: "123456789012",
			TargetAccountID:  "123456789012",
			Region:           "ap-northeast-2",
		})
		if err != nil {
			t.Fatalf("Recent returned error: %v", err)
		}
		want := Result{
			DeploymentName:    "payments-api-prod",
			ResourceType:      "autoScalingGroup",
			ResourceName:      "payments-api-prod-candidate",
			Available:         false,
			UnavailableReason: UnavailableReasonUnsupportedResourceType,
		}
		if !reflect.DeepEqual(result, want) {
			t.Fatalf("unexpected generic asg unavailable result: %+v", result)
		}
	})
}
