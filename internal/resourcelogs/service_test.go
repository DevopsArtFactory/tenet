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
	cwtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
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

	t.Run("ecs task threads container filter into cloudwatch provider", func(t *testing.T) {
		var gotLookup ECSRequest
		service := NewService(Options{
			MaxEntries: 5,
			ListECSLogEvents: func(ctx context.Context, req ECSRequest) ([]CloudWatchEvent, error) {
				gotLookup = req
				return []CloudWatchEvent{{Timestamp: time.Date(2026, 4, 16, 7, 11, 0, 0, time.UTC), Message: "container crashed", Stream: "ecs/api/abc123"}}, nil
			},
		})

		result, err := service.Recent(context.Background(), Request{
			DeploymentName:      "payments-api-prod",
			ResourceType:        "ecsTask",
			ResourceName:        "arn:aws:ecs:ap-northeast-2:123456789012:task/payments/abc123",
			ResourceDescription: "ECS task selected from live service inspection",
			ControlAccountID:    "123456789012",
			TargetAccountID:     "123456789012",
			Region:              "ap-northeast-2",
			ContainerName:       "api",
			Limit:               2,
		})
		if err != nil {
			t.Fatalf("Recent returned error: %v", err)
		}
		if gotLookup.ResourceName != "arn:aws:ecs:ap-northeast-2:123456789012:task/payments/abc123" || gotLookup.ContainerName != "api" || gotLookup.Limit != 2 {
			t.Fatalf("unexpected ECS task log lookup request: %+v", gotLookup)
		}
		if !result.Available || result.Source != SourceCloudWatch || result.ContainerName != "api" || result.ResourceDescription != "ECS task selected from live service inspection" || len(result.Entries) != 1 || result.Entries[0].Stream != "ecs/api/abc123" || result.Entries[0].ContainerName != "api" {
			t.Fatalf("expected available ECS task logs with echoed selector, per-entry container metadata, and description, got %+v", result)
		}
	})

	t.Run("ecs task all-container reads preserve per-entry container source", func(t *testing.T) {
		service := NewService(Options{
			MaxEntries: 5,
			ListECSLogEvents: func(ctx context.Context, req ECSRequest) ([]CloudWatchEvent, error) {
				if req.ContainerName != "" {
					t.Fatalf("expected all-container task lookup without selector, got %q", req.ContainerName)
				}
				return []CloudWatchEvent{
					{Timestamp: time.Date(2026, 4, 16, 7, 12, 0, 0, time.UTC), Message: "worker drained", Stream: "ecs/worker/abc123", ContainerName: "worker"},
					{Timestamp: time.Date(2026, 4, 16, 7, 11, 0, 0, time.UTC), Message: "api handled request", Stream: "ecs/api/abc123", ContainerName: "api"},
				}, nil
			},
		})

		result, err := service.Recent(context.Background(), Request{
			DeploymentName:   "payments-api-prod",
			ResourceType:     "ecsTask",
			ResourceName:     "arn:aws:ecs:ap-northeast-2:123456789012:task/payments/abc123",
			ControlAccountID: "123456789012",
			TargetAccountID:  "123456789012",
			Region:           "ap-northeast-2",
		})
		if err != nil {
			t.Fatalf("Recent returned error: %v", err)
		}
		want := []Entry{
			{Timestamp: "2026-04-16T07:12:00Z", Message: "worker drained", Stream: "ecs/worker/abc123", ContainerName: "worker"},
			{Timestamp: "2026-04-16T07:11:00Z", Message: "api handled request", Stream: "ecs/api/abc123", ContainerName: "api"},
		}
		if !reflect.DeepEqual(result.Entries, want) {
			t.Fatalf("unexpected ECS task entries: got %+v want %+v", result.Entries, want)
		}
		if result.ContainerName != "" {
			t.Fatalf("expected response-level selector to be omitted for all-container read, got %q", result.ContainerName)
		}
	})

	t.Run("threads requested limit into providers and result formatting", func(t *testing.T) {
		var gotECSLimit int
		var gotASGLimit int
		var gotCodeDeployLimit int
		service := NewService(Options{
			MaxEntries: 5,
			ListECSLogEvents: func(ctx context.Context, req ECSRequest) ([]CloudWatchEvent, error) {
				gotECSLimit = req.Limit
				return []CloudWatchEvent{
					{Timestamp: time.Date(2026, 4, 16, 7, 11, 0, 0, time.UTC), Message: "newest"},
					{Timestamp: time.Date(2026, 4, 16, 7, 10, 0, 0, time.UTC), Message: "older"},
				}, nil
			},
			ListASGInstanceConsoles: func(ctx context.Context, req ASGRequest) ([]InstanceConsole, error) {
				gotASGLimit = req.Limit
				return nil, nil
			},
			ListCodeDeployEvents: func(ctx context.Context, req CodeDeployRequest) ([]CodeDeployEvent, error) {
				gotCodeDeployLimit = req.Limit
				return nil, nil
			},
		})

		result, err := service.Recent(context.Background(), Request{
			DeploymentName:   "payments-api-prod",
			ResourceType:     "ecsService",
			ResourceName:     "payments-api-prod-service",
			ControlAccountID: "123456789012",
			TargetAccountID:  "123456789012",
			Region:           "ap-northeast-2",
			Limit:            1,
		})
		if err != nil {
			t.Fatalf("Recent returned error: %v", err)
		}
		if gotECSLimit != 1 {
			t.Fatalf("expected ECS request limit 1, got %d", gotECSLimit)
		}
		if len(result.Entries) != 1 || result.Entries[0].Message != "newest" {
			t.Fatalf("expected requested limit to cap formatted entries, got %+v", result.Entries)
		}

		_, _ = service.Recent(context.Background(), Request{DeploymentName: "payments-api-prod", ResourceType: "candidateAutoScalingGroup", ResourceName: "payments-api-prod-candidate", Limit: 2})
		_, _ = service.Recent(context.Background(), Request{DeploymentName: "payments-api-prod", ResourceType: "codeDeployDeploymentGroup", ResourceName: "payments-api-prod-codedeploy-group", Limit: 3})
		if gotASGLimit != 2 {
			t.Fatalf("expected ASG request limit 2, got %d", gotASGLimit)
		}
		if gotCodeDeployLimit != 3 {
			t.Fatalf("expected CodeDeploy request limit 3, got %d", gotCodeDeployLimit)
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

	t.Run("candidate asg instance selector returns selected console and echoes selector", func(t *testing.T) {
		var gotLookup ASGRequest
		service := NewService(Options{
			MaxEntries: 4,
			ListASGInstanceConsoles: func(ctx context.Context, req ASGRequest) ([]InstanceConsole, error) {
				gotLookup = req
				return []InstanceConsole{
					{InstanceID: "i-older", GeneratedAt: time.Date(2026, 4, 16, 7, 5, 0, 0, time.UTC), Lines: []ConsoleLine{{Timestamp: time.Date(2026, 4, 16, 7, 4, 0, 0, time.UTC), Message: "older bootstrap"}}},
					{InstanceID: "i-selected", GeneratedAt: time.Date(2026, 4, 16, 7, 12, 0, 0, time.UTC), Lines: []ConsoleLine{{Timestamp: time.Date(2026, 4, 16, 7, 11, 30, 0, time.UTC), Message: "selected start"}, {Timestamp: time.Date(2026, 4, 16, 7, 11, 45, 0, time.UTC), Message: "selected complete"}}},
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
			InstanceID:       " i-selected ",
		})
		if err != nil {
			t.Fatalf("Recent returned error: %v", err)
		}
		if gotLookup.InstanceID != "i-selected" {
			t.Fatalf("expected ASG provider to receive trimmed instance selector, got %+v", gotLookup)
		}
		want := Result{
			DeploymentName: "payments-api-prod",
			ResourceType:   "candidateAutoScalingGroup",
			ResourceName:   "payments-api-prod-candidate",
			InstanceID:     "i-selected",
			LookupContext:  expectedLookupContext("ap-northeast-2", "123456789012", "123456789012", ""),
			Available:      true,
			Source:         SourceEC2Console,
			Entries: []Entry{
				{Timestamp: "2026-04-16T07:11:30Z", Message: "selected start"},
				{Timestamp: "2026-04-16T07:11:45Z", Message: "selected complete"},
			},
		}
		if !reflect.DeepEqual(result, want) {
			t.Fatalf("unexpected selected ASG resource logs result: got %+v want %+v", result, want)
		}
	})

	t.Run("candidate asg instance selector returns unavailable when selected instance is absent", func(t *testing.T) {
		service := NewService(Options{
			ListASGInstanceConsoles: func(ctx context.Context, req ASGRequest) ([]InstanceConsole, error) {
				return []InstanceConsole{{InstanceID: "i-present", GeneratedAt: time.Date(2026, 4, 16, 7, 5, 0, 0, time.UTC), Lines: []ConsoleLine{{Timestamp: time.Date(2026, 4, 16, 7, 4, 0, 0, time.UTC), Message: "present bootstrap"}}}}, nil
			},
		})

		result, err := service.Recent(context.Background(), Request{
			DeploymentName:   "payments-api-prod",
			ResourceType:     "candidateAutoScalingGroup",
			ResourceName:     "payments-api-prod-candidate",
			ControlAccountID: "123456789012",
			TargetAccountID:  "123456789012",
			Region:           "ap-northeast-2",
			InstanceID:       "i-missing",
		})
		if err != nil {
			t.Fatalf("Recent returned error: %v", err)
		}
		want := Result{
			DeploymentName:    "payments-api-prod",
			ResourceType:      "candidateAutoScalingGroup",
			ResourceName:      "payments-api-prod-candidate",
			InstanceID:        "i-missing",
			LookupContext:     expectedLookupContext("ap-northeast-2", "123456789012", "123456789012", ""),
			Available:         false,
			UnavailableReason: UnavailableReasonResourceNotFound,
		}
		if !reflect.DeepEqual(result, want) {
			t.Fatalf("unexpected missing selected instance result: got %+v want %+v", result, want)
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
			DeploymentName:      "payments-api-prod",
			ResourceType:        "userDataBootstrapSource",
			ResourceName:        "file:userdata/payments-api-prod.sh",
			BackingResourceType: "candidateAutoScalingGroup",
			BackingResourceName: "payments-api-prod-candidate",
			LookupContext:       expectedLookupContext("ap-northeast-2", "123456789012", "123456789012", ""),
			Available:           true,
			Source:              SourceEC2Console,
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
			DeploymentName:      "payments-api-prod",
			ResourceType:        "userDataBootstrapSource",
			ResourceName:        "file:userdata/payments-api-prod.sh",
			ResourceDescription: "Normalized userdata bootstrap source for candidate EC2 capacity",
			ControlAccountID:    "123456789012",
			TargetAccountID:     "123456789012",
			Region:              "ap-northeast-2",
		})
		if err != nil {
			t.Fatalf("Recent returned error: %v", err)
		}
		want := Result{
			DeploymentName:      "payments-api-prod",
			ResourceType:        "userDataBootstrapSource",
			ResourceName:        "file:userdata/payments-api-prod.sh",
			ResourceDescription: "Normalized userdata bootstrap source for candidate EC2 capacity",
			LookupContext:       expectedLookupContext("ap-northeast-2", "123456789012", "123456789012", ""),
			Available:           false,
			UnavailableReason:   UnavailableReasonNoLogSource,
		}
		if !reflect.DeepEqual(result, want) {
			t.Fatalf("unexpected bootstrap-source unavailable result: %+v", result)
		}
	})

	t.Run("codeDeploy deployment group returns newest rollout history entries", func(t *testing.T) {
		var gotLookup CodeDeployRequest
		service := NewService(Options{
			MaxEntries: 2,
			ListCodeDeployEvents: func(ctx context.Context, req CodeDeployRequest) ([]CodeDeployEvent, error) {
				gotLookup = req
				return []CodeDeployEvent{
					{Timestamp: time.Date(2026, 4, 18, 9, 14, 0, 0, time.UTC), Message: "Status: Failed — deployment did not pass the after-allow-test-traffic hook", DeploymentID: "d-older"},
					{Timestamp: time.Date(2026, 4, 18, 9, 18, 0, 0, time.UTC), Message: "Status: Succeeded — deployment completed for the latest linear rollout", DeploymentID: "d-newest"},
					{Timestamp: time.Date(2026, 4, 18, 9, 16, 0, 0, time.UTC), Message: "Status: InProgress — shifting 50 percent of traffic", DeploymentID: "d-middle"},
				}, nil
			},
		})

		result, err := service.Recent(context.Background(), Request{
			DeploymentName:   "payments-api-staging",
			ResourceType:     "codeDeployDeploymentGroup",
			ResourceName:     "payments-api-staging-codedeploy-group",
			ControlAccountID: "123456789012",
			TargetAccountID:  "123456789012",
			Region:           "ap-northeast-2",
		})
		if err != nil {
			t.Fatalf("Recent returned error: %v", err)
		}
		if !reflect.DeepEqual(gotLookup, CodeDeployRequest{
			DeploymentName:  "payments-api-staging",
			ResourceType:    "codeDeployDeploymentGroup",
			ResourceName:    "payments-api-staging-codedeploy-group",
			Region:          "ap-northeast-2",
			TargetAccountID: "123456789012",
		}) {
			t.Fatalf("unexpected CodeDeploy lookup request: %+v", gotLookup)
		}
		want := Result{
			DeploymentName: "payments-api-staging",
			ResourceType:   "codeDeployDeploymentGroup",
			ResourceName:   "payments-api-staging-codedeploy-group",
			LookupContext:  expectedLookupContext("ap-northeast-2", "123456789012", "123456789012", ""),
			Available:      true,
			Source:         SourceCodeDeploy,
			Entries: []Entry{
				{Timestamp: "2026-04-18T09:18:00Z", Message: "Status: Succeeded — deployment completed for the latest linear rollout", Stream: "d-newest"},
				{Timestamp: "2026-04-18T09:16:00Z", Message: "Status: InProgress — shifting 50 percent of traffic", Stream: "d-middle"},
			},
		}
		if !reflect.DeepEqual(result, want) {
			t.Fatalf("unexpected CodeDeploy resource logs result: %+v", result)
		}
	})

	t.Run("codeDeploy application returns application-wide rollout history entries", func(t *testing.T) {
		var gotLookup CodeDeployRequest
		service := NewService(Options{
			MaxEntries: 1,
			ListCodeDeployEvents: func(ctx context.Context, req CodeDeployRequest) ([]CodeDeployEvent, error) {
				gotLookup = req
				return []CodeDeployEvent{
					{Timestamp: time.Date(2026, 4, 18, 9, 20, 0, 0, time.UTC), Message: "Status: Succeeded — application rollout completed", DeploymentID: "d-application"},
					{Timestamp: time.Date(2026, 4, 18, 9, 18, 0, 0, time.UTC), Message: "Status: InProgress — older deployment group rollout", DeploymentID: "d-older"},
				}, nil
			},
		})

		result, err := service.Recent(context.Background(), Request{
			DeploymentName:   "payments-api-staging",
			ResourceType:     "codeDeployApplication",
			ResourceName:     "payments-api-staging-codedeploy-app",
			ControlAccountID: "123456789012",
			TargetAccountID:  "123456789012",
			Region:           "ap-northeast-2",
		})
		if err != nil {
			t.Fatalf("Recent returned error: %v", err)
		}
		if !reflect.DeepEqual(gotLookup, CodeDeployRequest{
			DeploymentName:  "payments-api-staging",
			ResourceType:    "codeDeployApplication",
			ResourceName:    "payments-api-staging-codedeploy-app",
			Region:          "ap-northeast-2",
			TargetAccountID: "123456789012",
		}) {
			t.Fatalf("unexpected CodeDeploy application lookup request: %+v", gotLookup)
		}
		want := Result{
			DeploymentName: "payments-api-staging",
			ResourceType:   "codeDeployApplication",
			ResourceName:   "payments-api-staging-codedeploy-app",
			LookupContext:  expectedLookupContext("ap-northeast-2", "123456789012", "123456789012", ""),
			Available:      true,
			Source:         SourceCodeDeploy,
			Entries: []Entry{{
				Timestamp: "2026-04-18T09:20:00Z",
				Message:   "Status: Succeeded — application rollout completed",
				Stream:    "d-application",
			}},
		}
		if !reflect.DeepEqual(result, want) {
			t.Fatalf("unexpected CodeDeploy application recent logs result: %+v", result)
		}
	})

	t.Run("codedeploy sibling resources reuse deployment-group history through backing resource context", func(t *testing.T) {
		var gotLookup CodeDeployRequest
		service := NewService(Options{
			MaxEntries: 2,
			ListCodeDeployEvents: func(ctx context.Context, req CodeDeployRequest) ([]CodeDeployEvent, error) {
				gotLookup = req
				return []CodeDeployEvent{
					{Timestamp: time.Date(2026, 4, 18, 9, 18, 0, 0, time.UTC), Message: "Status: Succeeded — deployment completed for the latest linear rollout", DeploymentID: "d-newest"},
				}, nil
			},
		})

		result, err := service.Recent(context.Background(), Request{
			DeploymentName:      "payments-api-staging",
			ResourceType:        "testListener",
			ResourceName:        "payments-api-staging-test-listener",
			BackingResourceType: "codeDeployDeploymentGroup",
			BackingResourceName: "payments-api-staging-codedeploy-group",
			ControlAccountID:    "123456789012",
			TargetAccountID:     "123456789012",
			Region:              "ap-northeast-2",
		})
		if err != nil {
			t.Fatalf("Recent returned error: %v", err)
		}
		if !reflect.DeepEqual(gotLookup, CodeDeployRequest{
			DeploymentName:  "payments-api-staging",
			ResourceType:    "testListener",
			ResourceName:    "payments-api-staging-codedeploy-group",
			Region:          "ap-northeast-2",
			TargetAccountID: "123456789012",
		}) {
			t.Fatalf("unexpected CodeDeploy lookup request for sibling resource: %+v", gotLookup)
		}
		want := Result{
			DeploymentName:      "payments-api-staging",
			ResourceType:        "testListener",
			ResourceName:        "payments-api-staging-test-listener",
			BackingResourceType: "codeDeployDeploymentGroup",
			BackingResourceName: "payments-api-staging-codedeploy-group",
			LookupContext:       expectedLookupContext("ap-northeast-2", "123456789012", "123456789012", ""),
			Available:           true,
			Source:              SourceCodeDeploy,
			Entries: []Entry{{
				Timestamp: "2026-04-18T09:18:00Z",
				Message:   "Status: Succeeded — deployment completed for the latest linear rollout",
				Stream:    "d-newest",
			}},
		}
		if !reflect.DeepEqual(result, want) {
			t.Fatalf("unexpected CodeDeploy sibling recent logs result: %+v", result)
		}
	})

	t.Run("codedeploy sibling resources without backing deployment group return no log source", func(t *testing.T) {
		service := NewService(Options{})

		result, err := service.Recent(context.Background(), Request{
			DeploymentName:   "payments-api-staging",
			ResourceType:     "greenTaskSetTargetGroup",
			ResourceName:     "payments-api-staging-green",
			ControlAccountID: "123456789012",
			TargetAccountID:  "123456789012",
			Region:           "ap-northeast-2",
		})
		if err != nil {
			t.Fatalf("Recent returned error: %v", err)
		}
		want := Result{
			DeploymentName:    "payments-api-staging",
			ResourceType:      "greenTaskSetTargetGroup",
			ResourceName:      "payments-api-staging-green",
			LookupContext:     expectedLookupContext("ap-northeast-2", "123456789012", "123456789012", ""),
			Available:         false,
			UnavailableReason: UnavailableReasonNoLogSource,
		}
		if !reflect.DeepEqual(result, want) {
			t.Fatalf("unexpected missing-backing CodeDeploy sibling result: %+v", result)
		}
	})

	t.Run("codeDeploy deployment group returns empty-but-available payload when no rollout history exists", func(t *testing.T) {
		service := NewService(Options{
			ListCodeDeployEvents: func(ctx context.Context, req CodeDeployRequest) ([]CodeDeployEvent, error) {
				return []CodeDeployEvent{}, nil
			},
		})

		result, err := service.Recent(context.Background(), Request{
			DeploymentName:   "payments-api-staging",
			ResourceType:     "codeDeployDeploymentGroup",
			ResourceName:     "payments-api-staging-codedeploy-group",
			ControlAccountID: "123456789012",
			TargetAccountID:  "123456789012",
			Region:           "ap-northeast-2",
		})
		if err != nil {
			t.Fatalf("Recent returned error: %v", err)
		}
		want := Result{
			DeploymentName: "payments-api-staging",
			ResourceType:   "codeDeployDeploymentGroup",
			ResourceName:   "payments-api-staging-codedeploy-group",
			LookupContext:  expectedLookupContext("ap-northeast-2", "123456789012", "123456789012", ""),
			Available:      true,
			Source:         SourceCodeDeploy,
			Entries:        []Entry{},
		}
		if !reflect.DeepEqual(result, want) {
			t.Fatalf("unexpected empty CodeDeploy result: %+v", result)
		}
	})

	t.Run("codeDeploy deployment group degrades to unavailable when aws access is denied", func(t *testing.T) {
		service := NewService(Options{
			ListCodeDeployEvents: func(ctx context.Context, req CodeDeployRequest) ([]CodeDeployEvent, error) {
				if req.ResourceName != "payments-api-staging-codedeploy-group" {
					t.Fatalf("unexpected CodeDeploy request: %+v", req)
				}
				return nil, &smithy.OperationError{
					ServiceID:     "CodeDeploy",
					OperationName: "ListDeployments",
					Err: &smithy.GenericAPIError{
						Code:    "AccessDeniedException",
						Message: "User is not authorized to perform codedeploy:ListDeployments",
					},
				}
			},
		})

		result, err := service.Recent(context.Background(), Request{
			DeploymentName:   "payments-api-staging",
			ResourceType:     "codeDeployDeploymentGroup",
			ResourceName:     "payments-api-staging-codedeploy-group",
			ControlAccountID: "123456789012",
			TargetAccountID:  "123456789012",
			Region:           "ap-northeast-2",
		})
		if err != nil {
			t.Fatalf("Recent returned error: %v", err)
		}
		want := Result{
			DeploymentName:    "payments-api-staging",
			ResourceType:      "codeDeployDeploymentGroup",
			ResourceName:      "payments-api-staging-codedeploy-group",
			LookupContext:     expectedLookupContext("ap-northeast-2", "123456789012", "123456789012", ""),
			Available:         false,
			UnavailableReason: UnavailableReasonAWSUnavailable,
		}
		if !reflect.DeepEqual(result, want) {
			t.Fatalf("unexpected CodeDeploy AWS unavailable result: %+v", result)
		}
	})

	t.Run("codeDeploy deployment group provider failures are returned", func(t *testing.T) {
		providerErr := errors.New("boom: codedeploy provider internals")
		service := NewService(Options{
			ListCodeDeployEvents: func(ctx context.Context, req CodeDeployRequest) ([]CodeDeployEvent, error) {
				return nil, providerErr
			},
		})

		result, err := service.Recent(context.Background(), Request{
			DeploymentName:   "payments-api-staging",
			ResourceType:     "codeDeployDeploymentGroup",
			ResourceName:     "payments-api-staging-codedeploy-group",
			ControlAccountID: "123456789012",
			TargetAccountID:  "123456789012",
			Region:           "ap-northeast-2",
		})
		if !errors.Is(err, providerErr) {
			t.Fatalf("expected provider error %v, got %v", providerErr, err)
		}
		if !reflect.DeepEqual(result, Result{}) {
			t.Fatalf("expected zero result on CodeDeploy provider failure, got %+v", result)
		}
	})

	t.Run("codeDeploy deployment group without provider hook returns not implemented", func(t *testing.T) {
		service := NewService(Options{})

		result, err := service.Recent(context.Background(), Request{
			DeploymentName:   "payments-api-staging",
			ResourceType:     "codeDeployDeploymentGroup",
			ResourceName:     "payments-api-staging-codedeploy-group",
			ControlAccountID: "123456789012",
			TargetAccountID:  "123456789012",
			Region:           "ap-northeast-2",
		})
		if !errors.Is(err, ErrNotImplemented) {
			t.Fatalf("expected ErrNotImplemented, got %v", err)
		}
		if !reflect.DeepEqual(result, Result{}) {
			t.Fatalf("expected zero result when CodeDeploy provider hook is missing, got %+v", result)
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

	t.Run("weighted traffic-routing resources report no log source", func(t *testing.T) {
		service := NewService(Options{
			ListASGInstanceConsoles: func(ctx context.Context, req ASGRequest) ([]InstanceConsole, error) {
				t.Fatalf("weighted traffic routing resources should not call ASG provider: %+v", req)
				return nil, nil
			},
			ListCodeDeployEvents: func(ctx context.Context, req CodeDeployRequest) ([]CodeDeployEvent, error) {
				t.Fatalf("weighted traffic routing resources should not call CodeDeploy provider: %+v", req)
				return nil, nil
			},
		})

		for _, resource := range []struct {
			resourceType string
			resourceName string
		}{
			{resourceType: "trafficShiftPlan", resourceName: "payments-api-prod-weighted-routing"},
			{resourceType: "productionListenerRule", resourceName: "payments-api-prod-prod-listener"},
			{resourceType: "activeTargetGroup", resourceName: "payments-api-prod-active"},
			{resourceType: "candidateTargetGroup", resourceName: "payments-api-prod-candidate"},
		} {
			result, err := service.Recent(context.Background(), Request{
				DeploymentName:   "payments-api-prod",
				ResourceType:     resource.resourceType,
				ResourceName:     resource.resourceName,
				ControlAccountID: "123456789012",
				TargetAccountID:  "123456789012",
				Region:           "ap-northeast-2",
			})
			if err != nil {
				t.Fatalf("Recent returned error for %s: %v", resource.resourceType, err)
			}
			want := Result{
				DeploymentName:    "payments-api-prod",
				ResourceType:      resource.resourceType,
				ResourceName:      resource.resourceName,
				LookupContext:     expectedLookupContext("ap-northeast-2", "123456789012", "123456789012", ""),
				Available:         false,
				Source:            "",
				Entries:           nil,
				UnavailableReason: UnavailableReasonNoLogSource,
			}
			if !reflect.DeepEqual(result, want) {
				t.Fatalf("unexpected weighted traffic-routing log result for %s: got %+v want %+v", resource.resourceType, result, want)
			}
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
			ResourceType:     "ecsTask",
			ResourceName:     "arn:aws:ecs:ap-northeast-2:222222222222:task/payments/abc123",
			ControlAccountID: "111111111111",
			TargetAccountID:  "222222222222",
			Region:           "ap-northeast-2",
			ContainerName:    "sidecar",
			ObserverRoleARN:  "",
		})
		if err != nil {
			t.Fatalf("Recent returned error: %v", err)
		}
		want := Result{
			DeploymentName:    "payments-api-prod",
			ResourceType:      "ecsTask",
			ResourceName:      "arn:aws:ecs:ap-northeast-2:222222222222:task/payments/abc123",
			ContainerName:     "sidecar",
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
			DeploymentName:      "payments-api-prod",
			ResourceType:        "userDataBootstrapSource",
			ResourceName:        "file:userdata/payments-api-prod.sh",
			BackingResourceType: "candidateAutoScalingGroup",
			BackingResourceName: "payments-api-prod-candidate",
			LookupContext:       expectedLookupContext("ap-northeast-2", "123456789012", "123456789012", ""),
			Available:           false,
			UnavailableReason:   UnavailableReasonAWSUnavailable,
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

func TestFindRecentServiceTasksIncludesStoppedTasks(t *testing.T) {
	base := time.Date(2026, 4, 27, 12, 0, 0, 0, time.UTC)
	client := &fakeECSTaskClient{
		listByStatus: map[ecstypes.DesiredStatus][]string{
			ecstypes.DesiredStatusRunning: {"arn:aws:ecs:region:acct:task/cluster/running-old"},
			ecstypes.DesiredStatusStopped: {"arn:aws:ecs:region:acct:task/cluster/stopped-new"},
		},
		tasksByARN: map[string]ecstypes.Task{
			"arn:aws:ecs:region:acct:task/cluster/running-old": {
				TaskArn:           aws.String("arn:aws:ecs:region:acct:task/cluster/running-old"),
				StartedAt:         aws.Time(base.Add(-10 * time.Minute)),
				TaskDefinitionArn: aws.String("td:1"),
			},
			"arn:aws:ecs:region:acct:task/cluster/stopped-new": {
				TaskArn:           aws.String("arn:aws:ecs:region:acct:task/cluster/stopped-new"),
				StartedAt:         aws.Time(base.Add(-7 * time.Minute)),
				StoppedAt:         aws.Time(base.Add(-1 * time.Minute)),
				TaskDefinitionArn: aws.String("td:1"),
			},
		},
	}

	tasks, err := findRecentServiceTasks(context.Background(), client, "cluster-a", "payments-api-prod-service")
	if err != nil {
		t.Fatalf("findRecentServiceTasks returned error: %v", err)
	}
	if got, want := client.listStatuses, []ecstypes.DesiredStatus{ecstypes.DesiredStatusRunning, ecstypes.DesiredStatusStopped}; !reflect.DeepEqual(got, want) {
		t.Fatalf("expected running then stopped task lookup, got %+v", got)
	}
	if len(tasks) != 2 {
		t.Fatalf("expected running and stopped tasks, got %+v", tasks)
	}
	if got := aws.ToString(tasks[0].TaskArn); got != "arn:aws:ecs:region:acct:task/cluster/stopped-new" {
		t.Fatalf("expected recently stopped task first, got %q", got)
	}
	if got := aws.ToString(tasks[1].TaskArn); got != "arn:aws:ecs:region:acct:task/cluster/running-old" {
		t.Fatalf("expected running task second, got %q", got)
	}
}

func TestFindRecentServiceTasksReturnsNoTasksWhenRunningAndStoppedAreEmpty(t *testing.T) {
	client := &fakeECSTaskClient{listByStatus: map[ecstypes.DesiredStatus][]string{}}

	tasks, err := findRecentServiceTasks(context.Background(), client, "cluster-a", "payments-api-prod-service")
	if err != nil {
		t.Fatalf("findRecentServiceTasks returned error: %v", err)
	}
	if len(tasks) != 0 {
		t.Fatalf("expected no tasks, got %+v", tasks)
	}
	if got, want := client.listStatuses, []ecstypes.DesiredStatus{ecstypes.DesiredStatusRunning, ecstypes.DesiredStatusStopped}; !reflect.DeepEqual(got, want) {
		t.Fatalf("expected running then stopped task lookup, got %+v", got)
	}
}

func TestFetchRecentServiceTaskLogEventsSkipsMissingStoppedTaskStreams(t *testing.T) {
	base := time.Date(2026, 4, 27, 12, 0, 0, 0, time.UTC)
	ecsClient := &fakeECSTaskDefinitionClient{definitionsByARN: map[string]ecstypes.TaskDefinition{
		"td:1": awsLogsTaskDefinition(),
	}}
	logsClient := &fakeCloudWatchLogEventsClient{
		missingStreams: map[string]bool{"ecs/payments-api/app/stopped-missing": true},
		eventsByStream: map[string][]cwtypes.OutputLogEvent{
			"ecs/payments-api/app/stopped-good": {
				{Timestamp: aws.Int64(base.Add(-1 * time.Minute).UnixMilli()), Message: aws.String("stopped task failure")},
			},
		},
	}
	tasks := []ecstypes.Task{
		{
			TaskArn:           aws.String("arn:aws:ecs:region:acct:task/cluster/stopped-missing"),
			TaskDefinitionArn: aws.String("td:1"),
			Containers:        []ecstypes.Container{{Name: aws.String("app")}},
			StoppedAt:         aws.Time(base.Add(-2 * time.Minute)),
		},
		{
			TaskArn:           aws.String("arn:aws:ecs:region:acct:task/cluster/stopped-good"),
			TaskDefinitionArn: aws.String("td:1"),
			Containers:        []ecstypes.Container{{Name: aws.String("app")}},
			StoppedAt:         aws.Time(base.Add(-1 * time.Minute)),
		},
	}

	events, err := fetchRecentServiceTaskLogEvents(context.Background(), ecsClient, logsClient, tasks, 5)
	if err != nil {
		t.Fatalf("fetchRecentServiceTaskLogEvents returned error: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected only the available stopped-task stream, got %+v", events)
	}
	if events[0].Message != "stopped task failure" || events[0].Stream != "ecs/payments-api/app/stopped-good" {
		t.Fatalf("unexpected stopped-task log event: %+v", events[0])
	}
	if got := logsClient.limitsByStream["ecs/payments-api/app/stopped-good"]; got != 5 {
		t.Fatalf("expected CloudWatch log event limit 5, got %d", got)
	}
}

func awsLogsTaskDefinition() ecstypes.TaskDefinition {
	return ecstypes.TaskDefinition{ContainerDefinitions: []ecstypes.ContainerDefinition{{
		Name: aws.String("app"),
		LogConfiguration: &ecstypes.LogConfiguration{
			LogDriver: ecstypes.LogDriverAwslogs,
			Options: map[string]string{
				"awslogs-group":         "/aws/ecs/payments-api",
				"awslogs-stream-prefix": "ecs/payments-api",
			},
		},
	}}}
}

type fakeECSTaskClient struct {
	listByStatus map[ecstypes.DesiredStatus][]string
	tasksByARN   map[string]ecstypes.Task
	listStatuses []ecstypes.DesiredStatus
}

func (f *fakeECSTaskClient) ListTasks(ctx context.Context, input *ecs.ListTasksInput, opts ...func(*ecs.Options)) (*ecs.ListTasksOutput, error) {
	f.listStatuses = append(f.listStatuses, input.DesiredStatus)
	return &ecs.ListTasksOutput{TaskArns: append([]string(nil), f.listByStatus[input.DesiredStatus]...)}, nil
}

func (f *fakeECSTaskClient) DescribeTasks(ctx context.Context, input *ecs.DescribeTasksInput, opts ...func(*ecs.Options)) (*ecs.DescribeTasksOutput, error) {
	out := &ecs.DescribeTasksOutput{Tasks: make([]ecstypes.Task, 0, len(input.Tasks))}
	for _, arn := range input.Tasks {
		if task, ok := f.tasksByARN[arn]; ok {
			out.Tasks = append(out.Tasks, task)
		}
	}
	return out, nil
}

type fakeECSTaskDefinitionClient struct {
	definitionsByARN map[string]ecstypes.TaskDefinition
}

func (f *fakeECSTaskDefinitionClient) DescribeTaskDefinition(ctx context.Context, input *ecs.DescribeTaskDefinitionInput, opts ...func(*ecs.Options)) (*ecs.DescribeTaskDefinitionOutput, error) {
	definition, ok := f.definitionsByARN[aws.ToString(input.TaskDefinition)]
	if !ok {
		return &ecs.DescribeTaskDefinitionOutput{}, nil
	}
	return &ecs.DescribeTaskDefinitionOutput{TaskDefinition: &definition}, nil
}

type fakeCloudWatchLogEventsClient struct {
	eventsByStream map[string][]cwtypes.OutputLogEvent
	missingStreams map[string]bool
	limitsByStream map[string]int32
}

func (f *fakeCloudWatchLogEventsClient) GetLogEvents(ctx context.Context, input *cloudwatchlogs.GetLogEventsInput, opts ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.GetLogEventsOutput, error) {
	streamName := aws.ToString(input.LogStreamName)
	if f.limitsByStream == nil {
		f.limitsByStream = map[string]int32{}
	}
	f.limitsByStream[streamName] = aws.ToInt32(input.Limit)
	if f.missingStreams[streamName] {
		return nil, &cwtypes.ResourceNotFoundException{Message: aws.String("stream not found")}
	}
	return &cloudwatchlogs.GetLogEventsOutput{Events: append([]cwtypes.OutputLogEvent(nil), f.eventsByStream[streamName]...)}, nil
}

func TestCodeDeployListDeploymentsPayloadOmitsEmptyDeploymentGroup(t *testing.T) {
	payload := codeDeployListDeploymentsPayload(" payments-api-staging-codedeploy-app ", " ", "token-1")

	if payload["applicationName"] != "payments-api-staging-codedeploy-app" {
		t.Fatalf("unexpected applicationName: %#v", payload["applicationName"])
	}
	if _, ok := payload["deploymentGroupName"]; ok {
		t.Fatalf("expected application-wide CodeDeploy lookup to omit deploymentGroupName, got %#v", payload)
	}
	if payload["nextToken"] != "token-1" {
		t.Fatalf("unexpected nextToken: %#v", payload["nextToken"])
	}

	groupPayload := codeDeployListDeploymentsPayload("payments-api-staging-codedeploy-app", " payments-api-staging-codedeploy-group ", "")
	if groupPayload["deploymentGroupName"] != "payments-api-staging-codedeploy-group" {
		t.Fatalf("expected deployment-group scoped lookup to include deploymentGroupName, got %#v", groupPayload)
	}
	if _, ok := groupPayload["nextToken"]; ok {
		t.Fatalf("expected empty nextToken to be omitted, got %#v", groupPayload)
	}
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
