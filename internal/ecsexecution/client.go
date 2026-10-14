package ecsexecution

import (
	"context"
	"fmt"
	"strings"

	"github.com/DevopsArtFactory/tenet/internal/applyrequest"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/arn"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

// DeployerClient always assumes the configured role, even in the control account.
// Observer-client credentials must never be reused for live mutations.
func DeployerClient(ctx context.Context, role applyrequest.DeployerRoleReadiness) (RegistrationClient, error) {
	parsed, err := arn.Parse(role.RoleARN)
	if err != nil || parsed.Service != "iam" || !strings.HasPrefix(parsed.Resource, "role/") || parsed.AccountID != role.AWSAccountID || role.Region == "" || !role.Configured || !role.Ready || !role.CheckedWithAWS {
		return nil, fmt.Errorf("verified target-account deployer role is required")
	}
	config, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(role.Region))
	if err != nil {
		return nil, err
	}
	config.Credentials = aws.NewCredentialsCache(stscreds.NewAssumeRoleProvider(sts.NewFromConfig(config), role.RoleARN, func(options *stscreds.AssumeRoleOptions) { options.RoleSessionName = "tenet-ecs-executor" }))
	return ecs.NewFromConfig(config), nil
}
