package ecsexecution

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/DevopsArtFactory/tenet/internal/applyrequest"
	"github.com/DevopsArtFactory/tenet/internal/domain"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/aws-sdk-go-v2/service/ecs/types"
	"github.com/aws/smithy-go"
)

var _ applyrequest.ECSOperationHandler = Registration{}

type registrationFake struct {
	definitions   map[string]*ecs.DescribeTaskDefinitionOutput
	revisions     []string
	writes        int
	input         *ecs.RegisterTaskDefinitionInput
	responseError error
	readError     error
	hidden        bool
	pages         bool
	lists         int
	singleAttempt bool
}

func (f *registrationFake) DescribeTaskDefinition(_ context.Context, in *ecs.DescribeTaskDefinitionInput, _ ...func(*ecs.Options)) (*ecs.DescribeTaskDefinitionOutput, error) {
	if f.readError != nil {
		return nil, f.readError
	}
	result := f.definitions[aws.ToString(in.TaskDefinition)]
	if result == nil {
		return nil, errors.New("missing fake task definition")
	}
	return result, nil
}
func (f *registrationFake) ListTaskDefinitions(_ context.Context, in *ecs.ListTaskDefinitionsInput, _ ...func(*ecs.Options)) (*ecs.ListTaskDefinitionsOutput, error) {
	f.lists++
	if f.hidden {
		return &ecs.ListTaskDefinitionsOutput{}, nil
	}
	if f.pages && in.NextToken == nil {
		return &ecs.ListTaskDefinitionsOutput{NextToken: aws.String("second-page"), TaskDefinitionArns: []string{"arn:aws:ecs:ap-northeast-2:123456789012:task-definition/worker-prefix:99"}}, nil
	}
	return &ecs.ListTaskDefinitionsOutput{TaskDefinitionArns: f.revisions}, nil
}
func (f *registrationFake) RegisterTaskDefinition(_ context.Context, in *ecs.RegisterTaskDefinitionInput, options ...func(*ecs.Options)) (*ecs.RegisterTaskDefinitionOutput, error) {
	f.writes++
	var configured ecs.Options
	for _, option := range options {
		option(&configured)
	}
	f.singleAttempt = configured.RetryMaxAttempts == 1 && configured.Retryer.MaxAttempts() == 1
	content, _ := json.Marshal(in)
	var copied ecs.RegisterTaskDefinitionInput
	_ = json.Unmarshal(content, &copied)
	f.input = &copied
	var definition types.TaskDefinition
	_ = json.Unmarshal(content, &definition)
	definition.Revision = int32(17 + f.writes)
	definition.TaskDefinitionArn = aws.String(fmt.Sprintf("arn:aws:ecs:ap-northeast-2:123456789012:task-definition/worker:%d", definition.Revision))
	definition.Status = types.TaskDefinitionStatusActive
	f.revisions = append([]string{aws.ToString(definition.TaskDefinitionArn)}, f.revisions...)
	f.definitions[aws.ToString(definition.TaskDefinitionArn)] = &ecs.DescribeTaskDefinitionOutput{TaskDefinition: &definition, Tags: copied.Tags}
	if f.responseError != nil {
		return nil, f.responseError
	}
	return &ecs.RegisterTaskDefinitionOutput{TaskDefinition: &definition, Tags: copied.Tags}, nil
}

func registrationFixture() (*registrationFake, Registration, applyrequest.ECSOperationRequest) {
	base := "arn:aws:ecs:ap-northeast-2:123456789012:task-definition/worker:17"
	role := applyrequest.DeployerRoleReadiness{Configured: true, Ready: true, CheckedWithAWS: true, AWSAccountID: "123456789012", Region: "ap-northeast-2", RoleARN: "arn:aws:iam::123456789012:role/tenet-deployer"}
	request := applyrequest.ECSOperationRequest{BeforeMutation: func(context.Context) error { return nil }, ApplyID: "apply-a", OperationID: "register", RequestID: "tenet-request-a", Role: role, Intent: domain.ECSDeploymentIntent{AWSAccountID: role.AWSAccountID, Region: role.Region, DeploymentController: "ECS", DeploymentStrategy: "ROLLING", BaseTaskDefinitionARN: base, ContainerName: "worker", Image: "registry.example.com/worker@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}}
	definition := &types.TaskDefinition{TaskDefinitionArn: aws.String(base), Family: aws.String("worker"), Revision: 17, Status: types.TaskDefinitionStatusActive, Cpu: aws.String("256"), Memory: aws.String("512"), NetworkMode: types.NetworkModeAwsvpc, RequiresCompatibilities: []types.Compatibility{types.CompatibilityFargate}, TaskRoleArn: aws.String("arn:aws:iam::123456789012:role/task"), ExecutionRoleArn: aws.String("arn:aws:iam::123456789012:role/execution"), RuntimePlatform: &types.RuntimePlatform{CpuArchitecture: types.CPUArchitectureX8664, OperatingSystemFamily: types.OSFamilyLinux}, EphemeralStorage: &types.EphemeralStorage{SizeInGiB: 30}, ContainerDefinitions: []types.ContainerDefinition{
		{Name: aws.String("worker"), Image: aws.String("registry.example.com/worker:old"), Essential: aws.Bool(true), Environment: []types.KeyValuePair{{Name: aws.String("UNCHANGED"), Value: aws.String("fixture-value")}}, Secrets: []types.Secret{{Name: aws.String("DB_PASSWORD"), ValueFrom: aws.String("arn:aws:ssm:ap-northeast-2:123456789012:parameter/password")}}, LogConfiguration: &types.LogConfiguration{LogDriver: types.LogDriverAwslogs, Options: map[string]string{"awslogs-group": "worker", "awslogs-region": "ap-northeast-2"}}},
		{Name: aws.String("sidecar"), Image: aws.String("registry.example.com/sidecar:1"), Essential: aws.Bool(false)},
	}}
	fake := &registrationFake{definitions: map[string]*ecs.DescribeTaskDefinitionOutput{base: {TaskDefinition: definition, Tags: []types.Tag{{Key: aws.String("team"), Value: aws.String("payments")}, {Key: aws.String("aws:reserved"), Value: aws.String("ignored")}}}}}
	handler := Registration{ClientFor: func(context.Context, applyrequest.DeployerRoleReadiness) (RegistrationClient, error) {
		return fake, nil
	}, CheckCapabilities: func(context.Context, applyrequest.DeployerRoleReadiness, []applyrequest.OperationExecutionCapabilityCheck) error {
		return nil
	}}
	return fake, handler, request
}

func TestRegistrationPreservesSettingsAndReusesEquivalentRevision(t *testing.T) {
	fake, handler, request := registrationFixture()
	base := fake.definitions[request.Intent.BaseTaskDefinitionARN].TaskDefinition
	before, _ := json.Marshal(base)
	var checks []applyrequest.OperationExecutionCapabilityCheck
	handler.CheckCapabilities = func(_ context.Context, role applyrequest.DeployerRoleReadiness, input []applyrequest.OperationExecutionCapabilityCheck) error {
		if role.RoleARN != request.Role.RoleARN {
			t.Fatal("wrong deployer role")
		}
		checks = input
		return nil
	}
	if err := handler.Preflight(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if fake.writes != 0 {
		t.Fatal("preflight mutated AWS")
	}
	result, err := handler.Execute(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if fake.writes != 1 || !fake.singleAttempt || result.TaskDefinitionRevision != 18 || result.TaskDefinitionIntent == "" {
		t.Fatalf("missing registered identity/retry controls: %+v", result)
	}
	input := fake.input
	if aws.ToString(input.ContainerDefinitions[0].Image) != request.Intent.Image || !reflect.DeepEqual(input.ContainerDefinitions[1], base.ContainerDefinitions[1]) || !reflect.DeepEqual(input.ContainerDefinitions[0].Secrets, base.ContainerDefinitions[0].Secrets) || !reflect.DeepEqual(input.ContainerDefinitions[0].LogConfiguration, base.ContainerDefinitions[0].LogConfiguration) || !reflect.DeepEqual(input.ContainerDefinitions[0].Environment, base.ContainerDefinitions[0].Environment) || !reflect.DeepEqual(input.RuntimePlatform, base.RuntimePlatform) || !reflect.DeepEqual(input.EphemeralStorage, base.EphemeralStorage) || aws.ToString(input.ExecutionRoleArn) != aws.ToString(base.ExecutionRoleArn) || aws.ToString(input.TaskRoleArn) != aws.ToString(base.TaskRoleArn) || input.NetworkMode != base.NetworkMode {
		t.Fatal("registration lost inherited configuration")
	}
	after, _ := json.Marshal(base)
	if string(before) != string(after) {
		t.Fatal("registration modified cached base response")
	}
	passRoles := map[string]bool{}
	for _, check := range checks {
		if check.Action == "iam:PassRole" {
			passRoles[check.ResourceName] = true
		}
	}
	if len(passRoles) != 2 || !passRoles[aws.ToString(base.TaskRoleArn)] || !passRoles[aws.ToString(base.ExecutionRoleArn)] {
		t.Fatalf("unscoped or missing PassRole checks: %+v", checks)
	}
	for _, tag := range input.Tags {
		if aws.ToString(tag.Key) == "aws:reserved" {
			t.Fatal("attempted to write AWS reserved tag")
		}
	}
	fake.pages = true
	again, err := handler.Execute(context.Background(), request)
	if err != nil || again.TaskDefinitionARN != result.TaskDefinitionARN || fake.writes != 1 {
		t.Fatalf("same request duplicated revision: %+v %v", again, err)
	}
	request.RequestID = "another-apply-same-intent"
	equivalent, err := handler.Execute(context.Background(), request)
	if err != nil || equivalent.TaskDefinitionARN != result.TaskDefinitionARN || fake.writes != 1 {
		t.Fatalf("equivalent intent was not reused: %v", err)
	}
	request.Intent.Image = "registry.example.com/worker:changed"
	changed, err := handler.Execute(context.Background(), request)
	if err != nil || fake.writes != 2 || changed.TaskDefinitionARN == result.TaskDefinitionARN || changed.TaskDefinitionIntent == result.TaskDefinitionIntent {
		t.Fatalf("changed intent was lost: %+v %v", changed, err)
	}
}

func TestRegistrationReconcilesInterruptedResponsesAndWaitsForVisibility(t *testing.T) {
	fake, handler, request := registrationFixture()
	fake.responseError = errors.New("lost registration response")
	if _, err := handler.Execute(context.Background(), request); err == nil {
		t.Fatal("expected ambiguous response")
	}
	fake.hidden = true
	unresolved, err := handler.Reconcile(context.Background(), request)
	if err != nil || unresolved.Complete || unresolved.Absent || fake.writes != 1 {
		t.Fatal("missing result incorrectly proved absence")
	}
	fake.hidden = false
	recovered, err := handler.Reconcile(context.Background(), request)
	if err != nil || !recovered.Complete || recovered.Result.TaskDefinitionRevision != 18 || fake.writes != 1 {
		t.Fatalf("interrupted registration was replayed or lost: %+v %v", recovered, err)
	}
}

func TestRegistrationRejectsInvalidOrDeniedInputsWithoutWriting(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*registrationFake, *Registration, *applyrequest.ECSOperationRequest)
	}{
		{"missing container", func(_ *registrationFake, _ *Registration, r *applyrequest.ECSOperationRequest) {
			r.Intent.ContainerName = "missing"
		}},
		{"duplicate container", func(f *registrationFake, _ *Registration, r *applyrequest.ECSOperationRequest) {
			d := f.definitions[r.Intent.BaseTaskDefinitionARN].TaskDefinition
			d.ContainerDefinitions = append(d.ContainerDefinitions, d.ContainerDefinitions[0])
		}},
		{"unpinned base", func(_ *registrationFake, _ *Registration, r *applyrequest.ECSOperationRequest) {
			r.Intent.BaseTaskDefinitionARN = "worker"
		}},
		{"cross account task role", func(f *registrationFake, _ *Registration, r *applyrequest.ECSOperationRequest) {
			f.definitions[r.Intent.BaseTaskDefinitionARN].TaskDefinition.TaskRoleArn = aws.String("arn:aws:iam::999999999999:role/wrong")
		}},
		{"denied PassRole", func(_ *registrationFake, h *Registration, _ *applyrequest.ECSOperationRequest) {
			h.CheckCapabilities = func(context.Context, applyrequest.DeployerRoleReadiness, []applyrequest.OperationExecutionCapabilityCheck) error {
				return errors.New("PassRole denied")
			}
		}},
		{"missing capability checker", func(_ *registrationFake, h *Registration, _ *applyrequest.ECSOperationRequest) {
			h.CheckCapabilities = nil
		}},
		{"wrong deployer account", func(_ *registrationFake, _ *Registration, r *applyrequest.ECSOperationRequest) {
			r.Role.RoleARN = "arn:aws:iam::999999999999:role/wrong"
		}},
		{"tags exhausted", func(f *registrationFake, _ *Registration, r *applyrequest.ECSOperationRequest) {
			for i := 0; i < 49; i++ {
				f.definitions[r.Intent.BaseTaskDefinitionARN].Tags = append(f.definitions[r.Intent.BaseTaskDefinitionARN].Tags, types.Tag{Key: aws.String(fmt.Sprintf("tag%d", i)), Value: aws.String("value")})
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, h, r := registrationFixture()
			tc.change(f, &h, &r)
			if _, err := h.Execute(context.Background(), r); err == nil || f.writes != 0 {
				t.Fatal("invalid registration reached AWS mutation")
			}
		})
	}
}

func TestRegistrationChecksContentRatherThanTrustingRecoveryTags(t *testing.T) {
	fake, handler, request := registrationFixture()
	result, err := handler.Execute(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	fake.definitions[result.TaskDefinitionARN].TaskDefinition.ContainerDefinitions[0].Image = aws.String("registry.example.com/wrong:1")
	if _, err := handler.Reconcile(context.Background(), request); err == nil {
		t.Fatal("trusted copied tags over actual registered content")
	}
	if fake.writes != 1 {
		t.Fatal("reconciliation registered a replacement")
	}
}

func TestRegistrationErrorClassification(t *testing.T) {
	for _, tc := range []struct {
		code                string
		rejected, retryable bool
	}{{"AccessDeniedException", true, false}, {"InvalidParameterException", true, false}, {"ThrottlingException", true, true}, {"ServerException", false, false}} {
		classified := ecsMutationError(&smithy.GenericAPIError{Code: tc.code, Message: tc.code})
		var call *applyrequest.ECSCallError
		if errors.As(classified, &call) != tc.rejected || (call != nil && call.Retryable != tc.retryable) {
			t.Fatalf("bad classification for %s", tc.code)
		}
	}
}

func TestRegistrationReadFailureBeforeMutationIsSafeToRetry(t *testing.T) {
	for _, denied := range []bool{false, true} {
		fake, handler, request := registrationFixture()
		fake.readError = errors.New("temporary read connection failure")
		if denied {
			fake.readError = &smithy.GenericAPIError{Code: "AccessDeniedException", Message: "denied"}
		}
		_, err := handler.Execute(context.Background(), request)
		var classified *applyrequest.ECSCallError
		if !errors.As(err, &classified) || !classified.Rejected || classified.Retryable == denied || fake.writes != 0 {
			t.Fatalf("incorrect pre-mutation failure classification: %v", err)
		}
	}
}

func TestRegistrationDoesNotDescribeRevisionsOlderThanPinnedBase(t *testing.T) {
	fake, handler, request := registrationFixture()
	fake.revisions = []string{request.Intent.BaseTaskDefinitionARN, "arn:aws:ecs:ap-northeast-2:123456789012:task-definition/worker:16"}
	// Revision 16 intentionally has no fake Describe response. It predates the
	// pinned base, so it cannot belong to this request and must never be read.
	if _, err := handler.Execute(context.Background(), request); err != nil || fake.writes != 1 {
		t.Fatalf("scanned irrelevant historical revisions: %v", err)
	}
}

func TestRegistrationHistoryReadWaitIsCancellable(t *testing.T) {
	fake, handler, request := registrationFixture()
	result, err := handler.Execute(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	original := fake.definitions[result.TaskDefinitionARN]
	copied := *original.TaskDefinition
	copied.TaskDefinitionArn = aws.String("arn:aws:ecs:ap-northeast-2:123456789012:task-definition/worker:19")
	copied.Revision = 19
	fake.definitions[*copied.TaskDefinitionArn] = &ecs.DescribeTaskDefinitionOutput{TaskDefinition: &copied, Tags: original.Tags}
	fake.revisions = append([]string{*copied.TaskDefinitionArn}, fake.revisions...)
	request.RequestID = "different-request"
	request.Intent.Image = "registry.example.com/worker:different"
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if _, err := handler.Execute(ctx, request); !errors.Is(err, context.DeadlineExceeded) || fake.writes != 1 {
		t.Fatalf("cancelled history lookup reached registration: %v", err)
	}
}

func TestRegistrationReusesActivePinnedBaseThatAlreadyMatches(t *testing.T) {
	fake, handler, request := registrationFixture()
	request.Intent.Image = aws.ToString(fake.definitions[request.Intent.BaseTaskDefinitionARN].TaskDefinition.ContainerDefinitions[0].Image)
	result, err := handler.Execute(context.Background(), request)
	if err != nil || result.TaskDefinitionARN != request.Intent.BaseTaskDefinitionARN || result.TaskDefinitionRevision != 17 || fake.writes != 0 {
		t.Fatalf("matching reviewed base created a needless revision: %+v %v", result, err)
	}
}

func TestRegistrationRequiresFinalMutationGuardAfterDiscovery(t *testing.T) {
	for _, missing := range []bool{true, false} {
		fake, handler, request := registrationFixture()
		request.BeforeMutation = nil
		if !missing {
			request.BeforeMutation = func(context.Context) error { return errors.New("ownership revoked") }
		}
		if _, err := handler.Execute(context.Background(), request); err == nil || fake.writes != 0 {
			t.Fatalf("unguarded registration: writes=%d err=%v", fake.writes, err)
		}
	}
}
