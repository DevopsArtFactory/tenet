package resourcelogs

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/autoscaling"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
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
			LookupContext:  expectedLookupContext("ap-northeast-2", "123456789012", "123456789012", ""),
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
			LookupContext:  expectedLookupContext("ap-northeast-2", "123456789012", "123456789012", ""),
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
			LookupContext:  expectedLookupContext("ap-northeast-2", "123456789012", "123456789012", ""),
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
			LookupContext:  expectedLookupContext("ap-northeast-2", "123456789012", "123456789012", ""),
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
			LookupContext:  expectedLookupContext("ap-northeast-2", "123456789012", "123456789012", ""),
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
			LookupContext:  expectedLookupContext("ap-northeast-2", "123456789012", "123456789012", ""),
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

	t.Run("bootstrap source reuses backing asg console lookup while preserving bootstrap resource identity", func(t *testing.T) {
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
			t.Fatalf("Recent returned error: %v", err)
		}
		if !reflect.DeepEqual(gotLookup, ASGRequest{
			DeploymentName:  "payments-api-prod",
			ResourceName:    "payments-api-prod-candidate",
			Region:          "ap-northeast-2",
			TargetAccountID: "123456789012",
		}) {
			t.Fatalf("unexpected bootstrap-source ASG lookup request: %+v", gotLookup)
		}
		want := Result{
			DeploymentName: "payments-api-prod",
			ResourceType:   "userDataBootstrapSource",
			ResourceName:   "file:userdata/payments-api-prod.sh",
			LookupContext:  expectedLookupContext("ap-northeast-2", "123456789012", "123456789012", ""),
			Available:      true,
			Source:         SourceEC2Console,
			Entries: []Entry{
				{Timestamp: "2026-04-16T07:11:30Z", Message: "refresh start"},
				{Timestamp: "2026-04-16T07:11:45Z", Message: "refresh complete"},
			},
		}
		if !reflect.DeepEqual(result, want) {
			t.Fatalf("unexpected bootstrap-source result: %+v", result)
		}
	})

	t.Run("bootstrap source without backing resource returns no log source unavailable payload", func(t *testing.T) {
		service := NewService(Options{
			ListASGInstanceConsoles: func(ctx context.Context, req ASGRequest) ([]InstanceConsole, error) {
				t.Fatalf("bootstrap-source request should not call ASG provider without backing resource")
				return nil, nil
			},
		})

		result, err := service.Recent(context.Background(), Request{
			DeploymentName:   "payments-api-prod",
			ResourceType:     "userDataBootstrapSource",
			ResourceName:     "file:userdata/payments-api-prod.sh",
			ControlAccountID: "123456789012",
			TargetAccountID:  "123456789012",
			Region:           "ap-northeast-2",
		})
		if err != nil {
			t.Fatalf("Recent returned error: %v", err)
		}
		want := Result{
			DeploymentName:    "payments-api-prod",
			ResourceType:      "userDataBootstrapSource",
			ResourceName:      "file:userdata/payments-api-prod.sh",
			LookupContext:     expectedLookupContext("ap-northeast-2", "123456789012", "123456789012", ""),
			Available:         false,
			UnavailableReason: UnavailableReasonNoLogSource,
		}
		if !reflect.DeepEqual(result, want) {
			t.Fatalf("unexpected bootstrap-source unavailable result: %+v", result)
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
			LookupContext:     expectedLookupContext("ap-northeast-2", "123456789012", "123456789012", ""),
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
			LookupContext:     expectedLookupContext("ap-northeast-2", "111111111111", "222222222222", ""),
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
			LookupContext:     expectedLookupContext("ap-northeast-2", "123456789012", "123456789012", ""),
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
			LookupContext:     expectedLookupContext("ap-northeast-2", "123456789012", "123456789012", ""),
			Available:         false,
			UnavailableReason: UnavailableReasonAWSUnavailable,
		}
		if !reflect.DeepEqual(result, want) {
			t.Fatalf("unexpected AWS unavailable result: %+v", result)
		}
	})

	t.Run("bootstrap source degrades to unavailable when backing asg aws access is denied", func(t *testing.T) {
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
			t.Fatalf("Recent returned error: %v", err)
		}
		want := Result{
			DeploymentName:    "payments-api-prod",
			ResourceType:      "userDataBootstrapSource",
			ResourceName:      "file:userdata/payments-api-prod.sh",
			LookupContext:     expectedLookupContext("ap-northeast-2", "123456789012", "123456789012", ""),
			Available:         false,
			UnavailableReason: UnavailableReasonAWSUnavailable,
		}
		if !reflect.DeepEqual(result, want) {
			t.Fatalf("unexpected bootstrap-source AWS unavailable result: %+v", result)
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
			LookupContext:     expectedLookupContext("ap-northeast-2", "123456789012", "123456789012", ""),
			Available:         false,
			UnavailableReason: UnavailableReasonUnsupportedResourceType,
		}
		if !reflect.DeepEqual(result, want) {
			t.Fatalf("unexpected generic asg unavailable result: %+v", result)
		}
	})
}

func TestServiceAWSClientCache(t *testing.T) {
	t.Run("reuses config and typed clients for identical aws lookup keys", func(t *testing.T) {
		service := NewService(Options{})
		loads := 0
		ecsClients := 0
		logsClients := 0
		asgClients := 0
		ec2Clients := 0
		service.awsCache = newAWSClientCache(awsClientCacheDeps{
			LoadConfig: func(ctx context.Context, region, targetAccountID, observerRoleARN string) (aws.Config, error) {
				loads++
				if region != "ap-northeast-2" || targetAccountID != "123456789012" || observerRoleARN != "arn:aws:iam::123456789012:role/observer" {
					t.Fatalf("unexpected cache lookup key: region=%q target=%q observer=%q", region, targetAccountID, observerRoleARN)
				}
				return aws.Config{Region: region}, nil
			},
			NewECSClient: func(cfg aws.Config) *ecs.Client {
				ecsClients++
				return &ecs.Client{}
			},
			NewLogsClient: func(cfg aws.Config) *cloudwatchlogs.Client {
				logsClients++
				return &cloudwatchlogs.Client{}
			},
			NewASGClient: func(cfg aws.Config) *autoscaling.Client {
				asgClients++
				return &autoscaling.Client{}
			},
			NewEC2Client: func(cfg aws.Config) *ec2.Client {
				ec2Clients++
				return &ec2.Client{}
			},
		})

		first, err := service.awsClients(context.Background(), "ap-northeast-2", "123456789012", " arn:aws:iam::123456789012:role/observer ")
		if err != nil {
			t.Fatalf("first awsClients returned error: %v", err)
		}
		second, err := service.awsClients(context.Background(), "ap-northeast-2", "123456789012", "arn:aws:iam::123456789012:role/observer")
		if err != nil {
			t.Fatalf("second awsClients returned error: %v", err)
		}

		if loads != 1 || ecsClients != 1 || logsClients != 1 || asgClients != 1 || ec2Clients != 1 {
			t.Fatalf("expected single config/client construction, got loads=%d ecs=%d logs=%d asg=%d ec2=%d", loads, ecsClients, logsClients, asgClients, ec2Clients)
		}
		if first.ECS != second.ECS || first.Logs != second.Logs || first.ASG != second.ASG || first.EC2 != second.EC2 {
			t.Fatalf("expected cached aws clients to be reused across identical lookups")
		}
	})

	t.Run("builds separate clients for different aws lookup keys", func(t *testing.T) {
		service := NewService(Options{})
		loads := 0
		ecsClients := 0
		service.awsCache = newAWSClientCache(awsClientCacheDeps{
			LoadConfig: func(ctx context.Context, region, targetAccountID, observerRoleARN string) (aws.Config, error) {
				loads++
				return aws.Config{Region: region}, nil
			},
			NewECSClient: func(cfg aws.Config) *ecs.Client {
				ecsClients++
				return &ecs.Client{}
			},
			NewLogsClient: func(cfg aws.Config) *cloudwatchlogs.Client { return &cloudwatchlogs.Client{} },
			NewASGClient:  func(cfg aws.Config) *autoscaling.Client { return &autoscaling.Client{} },
			NewEC2Client:  func(cfg aws.Config) *ec2.Client { return &ec2.Client{} },
		})

		first, err := service.awsClients(context.Background(), "ap-northeast-2", "123456789012", "arn:aws:iam::123456789012:role/observer")
		if err != nil {
			t.Fatalf("first awsClients returned error: %v", err)
		}
		second, err := service.awsClients(context.Background(), "us-west-2", "123456789012", "arn:aws:iam::123456789012:role/observer")
		if err != nil {
			t.Fatalf("second awsClients returned error: %v", err)
		}

		if loads != 2 || ecsClients != 2 {
			t.Fatalf("expected unique lookup keys to rebuild config and clients, got loads=%d ecs=%d", loads, ecsClients)
		}
		if first.ECS == second.ECS {
			t.Fatalf("expected different lookup keys to keep separate aws clients")
		}
	})

	t.Run("retries failed cache entries on the next lookup", func(t *testing.T) {
		service := NewService(Options{})
		loads := 0
		service.awsCache = newAWSClientCache(awsClientCacheDeps{
			LoadConfig: func(ctx context.Context, region, targetAccountID, observerRoleARN string) (aws.Config, error) {
				loads++
				if loads == 1 {
					return aws.Config{}, errors.New("boom")
				}
				return aws.Config{Region: region}, nil
			},
			NewECSClient:  func(cfg aws.Config) *ecs.Client { return &ecs.Client{} },
			NewLogsClient: func(cfg aws.Config) *cloudwatchlogs.Client { return &cloudwatchlogs.Client{} },
			NewASGClient:  func(cfg aws.Config) *autoscaling.Client { return &autoscaling.Client{} },
			NewEC2Client:  func(cfg aws.Config) *ec2.Client { return &ec2.Client{} },
		})

		if _, err := service.awsClients(context.Background(), "ap-northeast-2", "123456789012", "arn:aws:iam::123456789012:role/observer"); err == nil || err.Error() != "boom" {
			t.Fatalf("expected first awsClients call to surface load failure, got %v", err)
		}
		if _, err := service.awsClients(context.Background(), "ap-northeast-2", "123456789012", "arn:aws:iam::123456789012:role/observer"); err != nil {
			t.Fatalf("expected second awsClients call to retry after failure, got %v", err)
		}
		if loads != 2 {
			t.Fatalf("expected failed cache entry to be retried, got loads=%d", loads)
		}
	})
}
