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
	"github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

// All executor clients assume the deployer role, including read-only preflight.
// Observer-client credentials must never be reused for live execution.
func deployerConfig(ctx context.Context, role applyrequest.DeployerRoleReadiness) (aws.Config, error) {
	parsed, err := arn.Parse(role.RoleARN)
	if err != nil || parsed.Service != "iam" || !strings.HasPrefix(parsed.Resource, "role/") || parsed.AccountID != role.AWSAccountID || role.Region == "" || !role.Configured || !role.Ready || !role.CheckedWithAWS {
		return aws.Config{}, fmt.Errorf("verified target-account deployer role is required")
	}
	config, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(role.Region))
	if err != nil {
		return aws.Config{}, err
	}
	config.Credentials = aws.NewCredentialsCache(stscreds.NewAssumeRoleProvider(sts.NewFromConfig(config), role.RoleARN, func(options *stscreds.AssumeRoleOptions) { options.RoleSessionName = "tenet-ecs-executor" }))
	return config, nil
}

func DeployerClient(ctx context.Context, role applyrequest.DeployerRoleReadiness) (*ecs.Client, error) {
	config, err := deployerConfig(ctx, role)
	if err != nil {
		return nil, err
	}
	return ecs.NewFromConfig(config), nil
}
func DeployerALBClient(ctx context.Context, role applyrequest.DeployerRoleReadiness) (*elasticloadbalancingv2.Client, error) {
	config, err := deployerConfig(ctx, role)
	if err != nil {
		return nil, err
	}
	return elasticloadbalancingv2.NewFromConfig(config), nil
}
