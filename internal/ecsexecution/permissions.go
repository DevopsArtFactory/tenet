package ecsexecution

import (
	"context"
	"fmt"
	"strings"

	"github.com/DevopsArtFactory/tenet/internal/applyrequest"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/arn"
)

func passRoleChecks(taskRole, executionRole *string, request applyrequest.ECSOperationRequest) ([]applyrequest.OperationExecutionCapabilityCheck, error) {
	base, _ := arn.Parse(request.Intent.BaseTaskDefinitionARN)
	seen := map[string]bool{}
	var checks []applyrequest.OperationExecutionCapabilityCheck
	for _, role := range []*string{taskRole, executionRole} {
		value := aws.ToString(role)
		if value == "" || seen[value] {
			continue
		}
		parsed, err := arn.Parse(value)
		if err != nil || parsed.Service != "iam" || parsed.AccountID != request.Intent.AWSAccountID || parsed.Partition != base.Partition || !strings.HasPrefix(parsed.Resource, "role/") {
			return nil, fmt.Errorf("inherited task roles must be concrete IAM role ARNs in the reviewed account")
		}
		seen[value] = true
		checks = append(checks, applyrequest.OperationExecutionCapabilityCheck{Action: "iam:PassRole", ResourceType: "iamRole", ResourceName: value, RequiredRole: request.Role.RoleARN})
	}
	return checks, nil
}

func authorizeMutation(ctx context.Context, request applyrequest.ECSOperationRequest) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if request.BeforeMutation == nil {
		return fmt.Errorf("executor mutation ownership guard is required")
	}
	return request.BeforeMutation(ctx)
}
