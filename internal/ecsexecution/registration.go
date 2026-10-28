// Package ecsexecution implements reviewed ECS operations with injected AWS clients.
package ecsexecution

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/DevopsArtFactory/tenet/internal/applyrequest"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/arn"
	awsmiddleware "github.com/aws/aws-sdk-go-v2/aws/middleware"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/aws-sdk-go-v2/service/ecs/types"
	"github.com/aws/smithy-go"
)

const intentTag = "tenet:task-definition-intent"
const requestTag = "tenet:request-id"

type RegistrationClient interface {
	DescribeTaskDefinition(context.Context, *ecs.DescribeTaskDefinitionInput, ...func(*ecs.Options)) (*ecs.DescribeTaskDefinitionOutput, error)
	ListTaskDefinitions(context.Context, *ecs.ListTaskDefinitionsInput, ...func(*ecs.Options)) (*ecs.ListTaskDefinitionsOutput, error)
	RegisterTaskDefinition(context.Context, *ecs.RegisterTaskDefinitionInput, ...func(*ecs.Options)) (*ecs.RegisterTaskDefinitionOutput, error)
}

// Registration only registers/reuses revisions. Its client cannot update services.
// Clients must assume the supplied deployer role, including same-account calls.
type Registration struct {
	AllowAdvanced bool

	ClientFor func(context.Context, applyrequest.DeployerRoleReadiness) (RegistrationClient, error)
	// CheckCapabilities checks concrete ECS actions and inherited iam:PassRole ARNs.
	// It is an additional fail-closed gate; simulation never guarantees AWS acceptance.
	CheckCapabilities func(context.Context, applyrequest.DeployerRoleReadiness, []applyrequest.OperationExecutionCapabilityCheck) error
}

func (r Registration) Preflight(ctx context.Context, request applyrequest.ECSOperationRequest) error {
	_, _, _, err := r.prepare(ctx, request)
	return err
}

func (r Registration) prepare(ctx context.Context, request applyrequest.ECSOperationRequest) (RegistrationClient, *ecs.RegisterTaskDefinitionInput, string, error) {
	if r.ClientFor == nil || r.CheckCapabilities == nil {
		return nil, nil, "", fmt.Errorf("registration client and concrete capability checker are required")
	}
	e := request.Intent
	baseARN, err := arn.Parse(e.BaseTaskDefinitionARN)
	roleARN, roleErr := arn.Parse(request.Role.RoleARN)
	revision := ""
	family := ""
	if err == nil && strings.HasPrefix(baseARN.Resource, "task-definition/") {
		family, revision, _ = strings.Cut(strings.TrimPrefix(baseARN.Resource, "task-definition/"), ":")
	}
	revisionNumber, revisionErr := strconv.ParseInt(revision, 10, 32)
	if err != nil || roleErr != nil || revisionErr != nil || revisionNumber <= 0 || family == "" || baseARN.Service != "ecs" || baseARN.AccountID != e.AWSAccountID || baseARN.Region != e.Region || roleARN.Service != "iam" || !strings.HasPrefix(roleARN.Resource, "role/") || roleARN.AccountID != e.AWSAccountID || roleARN.Partition != baseARN.Partition || !request.Role.Configured || !request.Role.Ready || !request.Role.CheckedWithAWS || request.Role.AWSAccountID != e.AWSAccountID || request.Role.Region != e.Region || e.DeploymentController != "ECS" || (e.DeploymentStrategy != "ROLLING" && !r.AllowAdvanced) || e.ContainerName == "" || e.Image == "" || request.RequestID == "" {
		return nil, nil, "", fmt.Errorf("registration requires reviewed native Rolling intent, pinned base revision and matching deployer role")
	}
	if e.DeploymentStrategy != "ROLLING" {
		if err := validateAdvancedIntent(e); err != nil {
			return nil, nil, "", err
		}
	}
	client, err := r.ClientFor(ctx, request.Role)
	if err != nil {
		return nil, nil, "", err
	}
	if client == nil {
		return nil, nil, "", fmt.Errorf("registration client is unavailable")
	}
	observed, err := client.DescribeTaskDefinition(ctx, &ecs.DescribeTaskDefinitionInput{TaskDefinition: aws.String(e.BaseTaskDefinitionARN), Include: []types.TaskDefinitionField{types.TaskDefinitionFieldTags}})
	if err != nil {
		return nil, nil, "", err
	}
	if observed == nil || observed.TaskDefinition == nil || aws.ToString(observed.TaskDefinition.TaskDefinitionArn) != e.BaseTaskDefinitionARN || aws.ToString(observed.TaskDefinition.Family) != family || observed.TaskDefinition.Revision != int32(revisionNumber) {
		return nil, nil, "", fmt.Errorf("AWS did not return the reviewed pinned task definition")
	}
	input, err := registrationInput(observed.TaskDefinition, observed.Tags)
	if err != nil {
		return nil, nil, "", err
	}
	matches := 0
	for i := range input.ContainerDefinitions {
		if aws.ToString(input.ContainerDefinitions[i].Name) == e.ContainerName {
			matches++
			input.ContainerDefinitions[i].Image = aws.String(e.Image)
		}
	}
	if matches != 1 {
		return nil, nil, "", fmt.Errorf("reviewed container must match exactly one base task-definition container")
	}
	familyARN := fmt.Sprintf("arn:%s:ecs:%s:%s:task-definition/%s:*", baseARN.Partition, e.Region, e.AWSAccountID, family)
	checks := []applyrequest.OperationExecutionCapabilityCheck{
		{Action: "ecs:DescribeTaskDefinition", ResourceType: "taskDefinition", ResourceName: e.BaseTaskDefinitionARN, RequiredRole: request.Role.RoleARN},
		{Action: "ecs:DescribeTaskDefinition", ResourceType: "taskDefinition", ResourceName: familyARN, RequiredRole: request.Role.RoleARN},
		{Action: "ecs:ListTaskDefinitions", ResourceName: "*", RequiredRole: request.Role.RoleARN},
		{Action: "ecs:RegisterTaskDefinition", ResourceType: "taskDefinition", ResourceName: familyARN, RequiredRole: request.Role.RoleARN},
		{Action: "ecs:TagResource", ResourceType: "taskDefinition", ResourceName: familyARN, RequiredRole: request.Role.RoleARN},
	}
	if request.Recovery != nil {
		checks = checks[:1]
	}
	roleChecks, err := passRoleChecks(input.TaskRoleArn, input.ExecutionRoleArn, request)
	if err != nil {
		return nil, nil, "", err
	}
	checks = append(checks, roleChecks...)

	if err := r.CheckCapabilities(ctx, request.Role, checks); err != nil {
		return nil, nil, "", err
	}
	fingerprint, err := inputFingerprint(input)
	return client, input, fingerprint, err
}

// Copy only registration fields; response-only metadata cannot reach AWS input.
// JSON cloning keeps all nested container/runtime settings independent of SDK results.
func registrationInput(definition *types.TaskDefinition, tags []types.Tag) (*ecs.RegisterTaskDefinitionInput, error) {
	content, err := json.Marshal(definition)
	if err != nil {
		return nil, err
	}
	var d types.TaskDefinition
	if err := json.Unmarshal(content, &d); err != nil {
		return nil, err
	}
	input := &ecs.RegisterTaskDefinitionInput{
		Family: d.Family, ContainerDefinitions: d.ContainerDefinitions, Cpu: d.Cpu, Memory: d.Memory,
		TaskRoleArn: d.TaskRoleArn, ExecutionRoleArn: d.ExecutionRoleArn, NetworkMode: d.NetworkMode,
		Volumes: d.Volumes, PlacementConstraints: d.PlacementConstraints, RequiresCompatibilities: d.RequiresCompatibilities,
		PidMode: d.PidMode, IpcMode: d.IpcMode, ProxyConfiguration: d.ProxyConfiguration,
		InferenceAccelerators: d.InferenceAccelerators, RuntimePlatform: d.RuntimePlatform,
		EphemeralStorage: d.EphemeralStorage, EnableFaultInjection: d.EnableFaultInjection,
	}
	seen := map[string]bool{}
	for _, tag := range tags {
		key := aws.ToString(tag.Key)
		if strings.HasPrefix(strings.ToLower(key), "aws:") || key == intentTag || key == requestTag {
			continue
		}
		if key == "" || seen[key] {
			return nil, fmt.Errorf("base task definition contains invalid or duplicate tags")
		}
		seen[key] = true
		input.Tags = append(input.Tags, types.Tag{Key: aws.String(key), Value: aws.String(aws.ToString(tag.Value))})
	}
	if len(input.Tags) > 48 {
		return nil, fmt.Errorf("registration needs two available task-definition tags for recovery identity")
	}
	sort.Slice(input.Tags, func(i, j int) bool { return aws.ToString(input.Tags[i].Key) < aws.ToString(input.Tags[j].Key) })
	return input, nil
}

func inputFingerprint(input *ecs.RegisterTaskDefinitionInput) (string, error) {
	content, err := json.Marshal(input)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:]), nil
}

func (r Registration) Execute(ctx context.Context, request applyrequest.ECSOperationRequest) (applyrequest.ECSOperationResult, error) {
	client, input, fingerprint, err := r.prepare(ctx, request)
	if err != nil {
		return applyrequest.ECSOperationResult{}, beforeMutationError(err)
	}
	found, err := findRevision(ctx, client, request, input, fingerprint)
	if err != nil {
		return applyrequest.ECSOperationResult{}, beforeMutationError(err)
	}
	if found != nil {
		return *found, nil
	}
	input.Tags = append(input.Tags, types.Tag{Key: aws.String(intentTag), Value: aws.String(fingerprint)}, types.Tag{Key: aws.String(requestTag), Value: aws.String(request.RequestID)})
	if err := authorizeMutation(ctx, request); err != nil {
		return applyrequest.ECSOperationResult{}, beforeMutationError(err)
	}
	output, err := client.RegisterTaskDefinition(ctx, input, func(options *ecs.Options) { options.Retryer = aws.NopRetryer{}; options.RetryMaxAttempts = 1 })
	if err != nil {
		return applyrequest.ECSOperationResult{}, ecsMutationError(err)
	}
	if output == nil || output.TaskDefinition == nil {
		return applyrequest.ECSOperationResult{}, fmt.Errorf("registration response omitted task-definition identity; reconcile before retry")
	}
	result, err := revisionResult(output.TaskDefinition, request, input, fingerprint)
	if err != nil {
		return applyrequest.ECSOperationResult{}, err
	}
	result.AWSRequestID, _ = awsmiddleware.GetRequestIDMetadata(output.ResultMetadata)
	return result, nil
}

func (r Registration) Reconcile(ctx context.Context, request applyrequest.ECSOperationRequest) (applyrequest.ECSReconciliation, error) {
	client, input, fingerprint, err := r.prepare(ctx, request)
	if err != nil {
		return applyrequest.ECSReconciliation{}, err
	}
	found, err := findRevision(ctx, client, request, input, fingerprint)
	if err != nil {
		return applyrequest.ECSReconciliation{}, err
	}
	if found != nil {
		return applyrequest.ECSReconciliation{Complete: true, Result: *found}, nil
	}
	// A paginated list cannot prove an ambiguous registration never happened:
	// ECS visibility can lag. Retain ownership and wait rather than register again.
	return applyrequest.ECSReconciliation{}, nil
}

func findRevision(ctx context.Context, client RegistrationClient, request applyrequest.ECSOperationRequest, input *ecs.RegisterTaskDefinitionInput, fingerprint string) (*applyrequest.ECSOperationResult, error) {
	pinned, err := client.DescribeTaskDefinition(ctx, &ecs.DescribeTaskDefinitionInput{TaskDefinition: aws.String(request.Intent.BaseTaskDefinitionARN), Include: []types.TaskDefinitionField{types.TaskDefinitionFieldTags}})
	if err != nil {
		return nil, err
	}
	if pinned == nil || pinned.TaskDefinition == nil || aws.ToString(pinned.TaskDefinition.TaskDefinitionArn) != request.Intent.BaseTaskDefinitionARN {
		return nil, fmt.Errorf("pinned base evidence is unavailable during revision discovery")
	}
	if pinned.TaskDefinition.Status == types.TaskDefinitionStatusActive {
		candidate, err := registrationInput(pinned.TaskDefinition, pinned.Tags)
		if err != nil {
			return nil, err
		}
		candidateFingerprint, err := inputFingerprint(candidate)
		if err != nil {
			return nil, err
		}
		if candidateFingerprint == fingerprint {
			result, err := revisionResult(pinned.TaskDefinition, request, input, fingerprint)
			if err != nil {
				return nil, err
			}
			return &result, nil
		}
	}

	if request.Recovery != nil {
		return nil, fmt.Errorf("rollback requires the exact previous ACTIVE task definition; registration is prohibited")
	}
	base, _ := arn.Parse(request.Intent.BaseTaskDefinitionARN)
	_, baseVersion, _ := strings.Cut(strings.TrimPrefix(base.Resource, "task-definition/"), ":")
	baseRevision, _ := strconv.ParseInt(baseVersion, 10, 32)
	readPreviously := false
	var token *string
	seen := map[string]bool{}
	for {
		output, err := client.ListTaskDefinitions(ctx, &ecs.ListTaskDefinitionsInput{FamilyPrefix: input.Family, Status: types.TaskDefinitionStatusActive, Sort: types.SortOrderDesc, MaxResults: aws.Int32(100), NextToken: token})
		if err != nil {
			return nil, err
		}
		if output == nil {
			return nil, fmt.Errorf("task-definition listing returned no response")
		}
		for _, candidate := range output.TaskDefinitionArns {
			if !sameFamily(candidate, request, aws.ToString(input.Family)) {
				continue
			}
			parsed, _ := arn.Parse(candidate)
			_, version, _ := strings.Cut(strings.TrimPrefix(parsed.Resource, "task-definition/"), ":")
			revision, _ := strconv.ParseInt(version, 10, 32)
			// Our registration cannot predate its pinned base. With DESC ordering,
			// reaching the base ends the relevant history without describing it.
			if revision <= baseRevision {
				return nil, nil
			}
			if readPreviously {
				timer := time.NewTimer(200 * time.Millisecond)
				select {
				case <-ctx.Done():
					timer.Stop()
					return nil, ctx.Err()
				case <-timer.C:
				}
			}
			readPreviously = true
			observed, err := client.DescribeTaskDefinition(ctx, &ecs.DescribeTaskDefinitionInput{TaskDefinition: aws.String(candidate), Include: []types.TaskDefinitionField{types.TaskDefinitionFieldTags}})
			if err != nil {
				return nil, err
			}
			if observed == nil || observed.TaskDefinition == nil || aws.ToString(observed.TaskDefinition.TaskDefinitionArn) != candidate {
				return nil, fmt.Errorf("listed task-definition revision is unavailable")
			}
			taggedIntent, taggedRequest := "", ""
			for _, tag := range observed.Tags {
				switch aws.ToString(tag.Key) {
				case intentTag:
					taggedIntent = aws.ToString(tag.Value)
				case requestTag:
					taggedRequest = aws.ToString(tag.Value)
				}
			}
			if taggedIntent != fingerprint && taggedRequest != request.RequestID {
				continue
			}
			candidateInput, err := registrationInput(observed.TaskDefinition, observed.Tags)
			if err != nil {
				return nil, err
			}
			candidateFingerprint, err := inputFingerprint(candidateInput)
			if err != nil {
				return nil, err
			}
			if candidateFingerprint != fingerprint {
				if taggedRequest == request.RequestID {
					return nil, fmt.Errorf("task-definition request identity conflicts with its content")
				}
				continue
			}
			result, err := revisionResult(observed.TaskDefinition, request, input, fingerprint)
			if err != nil {
				return nil, err
			}
			return &result, nil
		}
		token = output.NextToken
		if aws.ToString(token) == "" {
			return nil, nil
		}
		if seen[*token] {
			return nil, fmt.Errorf("task-definition pagination repeated a token")
		}
		seen[*token] = true
	}
}

func sameFamily(value string, request applyrequest.ECSOperationRequest, family string) bool {
	parsed, err := arn.Parse(value)
	base, _ := arn.Parse(request.Intent.BaseTaskDefinitionARN)
	if err != nil || parsed.Service != "ecs" || parsed.Partition != base.Partition || parsed.AccountID != request.Intent.AWSAccountID || parsed.Region != request.Intent.Region {
		return false
	}
	name, revision, ok := strings.Cut(strings.TrimPrefix(parsed.Resource, "task-definition/"), ":")
	number, parseErr := strconv.ParseInt(revision, 10, 32)
	return strings.HasPrefix(parsed.Resource, "task-definition/") && ok && name == family && parseErr == nil && number > 0
}

func revisionResult(definition *types.TaskDefinition, request applyrequest.ECSOperationRequest, input *ecs.RegisterTaskDefinitionInput, fingerprint string) (applyrequest.ECSOperationResult, error) {
	value := aws.ToString(definition.TaskDefinitionArn)
	if !sameFamily(value, request, aws.ToString(input.Family)) || definition.Status != types.TaskDefinitionStatusActive || definition.Revision <= 0 || !strings.HasSuffix(value, ":"+strconv.Itoa(int(definition.Revision))) {
		return applyrequest.ECSOperationResult{}, fmt.Errorf("registration revision identity/status did not match reviewed placement; reconcile before retry")
	}
	return applyrequest.ECSOperationResult{TaskDefinitionARN: value, TaskDefinitionRevision: definition.Revision, TaskDefinitionIntent: fingerprint}, nil
}

func ecsMutationError(err error) error {
	var api smithy.APIError
	if errors.As(err, &api) {
		switch api.ErrorCode() {
		case "ThrottlingException", "TooManyRequestsException", "RequestLimitExceeded":
			return &applyrequest.ECSCallError{Err: err, Rejected: true, Retryable: true}
		case "AccessDenied", "AccessDeniedException", "UnauthorizedException", "ClientException", "InvalidParameterException", "UnsupportedFeatureException":
			return &applyrequest.ECSCallError{Err: err, Rejected: true}
		}
	}
	return err
}

// Reads failed before the mutation was sent, so retrying them cannot duplicate
// the mutation. Known authorization/validation rejections remain terminal.
func beforeMutationError(err error) error {
	classified := ecsMutationError(err)
	var known *applyrequest.ECSCallError
	if errors.As(classified, &known) {
		return classified
	}
	return &applyrequest.ECSCallError{Err: err, Rejected: true, Retryable: true}
}
