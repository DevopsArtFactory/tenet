package spec

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws/arn"
)

// ECSDeploymentSpec binds reviewed intent to an existing ECS service and base revision.
type ECSDeploymentSpec struct {
	Advanced              *ECSAdvancedSpec `json:"advanced,omitempty" yaml:"advanced,omitempty"`
	SyncPolicy            *ECSSyncPolicy   `json:"syncPolicy,omitempty" yaml:"syncPolicy,omitempty"`
	RollbackOnFailure     bool             `json:"rollbackOnFailure,omitempty" yaml:"rollbackOnFailure,omitempty"`
	DeploymentController  string           `json:"deploymentController" yaml:"deploymentController"`
	ClusterARN            string           `json:"clusterArn" yaml:"clusterArn"`
	ServiceARN            string           `json:"serviceArn" yaml:"serviceArn"`
	BaseTaskDefinitionARN string           `json:"baseTaskDefinitionArn" yaml:"baseTaskDefinitionArn"`
	ContainerName         string           `json:"containerName" yaml:"containerName"`
	MinimumHealthyPercent *int             `json:"minimumHealthyPercent,omitempty" yaml:"minimumHealthyPercent,omitempty"`
	MaximumPercent        *int             `json:"maximumPercent,omitempty" yaml:"maximumPercent,omitempty"`
}

var ecsClusterPattern = regexp.MustCompile(`^cluster/[a-zA-Z0-9_-]{1,255}$`)
var ecsServicePattern = regexp.MustCompile(`^service/[a-zA-Z0-9_-]{1,255}/[a-zA-Z0-9_-]{1,255}$`)
var ecsTaskDefinitionPattern = regexp.MustCompile(`^task-definition/[a-zA-Z0-9_-]{1,255}:[1-9][0-9]*$`)
var ecsContainerPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,255}$`)
var ecsImagePattern = regexp.MustCompile(`^[a-z0-9]+(?:[._-]+[a-z0-9]+)*(?::[0-9]+)?(?:/[a-z0-9]+(?:[._-]+[a-z0-9]+)*)*(?::[a-zA-Z0-9_][a-zA-Z0-9_.-]{0,127})?(?:@sha256:[a-fA-F0-9]{64})?$`)

func validateECSDeployment(d DeploymentSpec) error {
	e := d.ECS
	if e == nil {
		return nil
	}
	if d.NormalizedTargetType() != TargetTypeECS {
		return fmt.Errorf("ecs execution inputs require an ecs target")
	}
	if e.RollbackOnFailure && d.NormalizedStrategy() != StrategyTypeRolling {
		return fmt.Errorf("ecs.rollbackOnFailure currently supports Rolling only")
	}
	if e.SyncPolicy != nil {
		mode := strings.TrimSpace(e.SyncPolicy.Mode)
		if mode != "" && mode != "manual" && mode != "automatic" {
			return fmt.Errorf("ecs.syncPolicy.mode must be manual or automatic")
		}
		if e.SyncPolicy.SelfHeal && d.NormalizedStrategy() != StrategyTypeRolling {
			return fmt.Errorf("ECS self-heal currently supports Rolling only")
		}
		if mode == "automatic" && d.NormalizedStrategy() != StrategyTypeRolling && e.Advanced == nil {
			return fmt.Errorf("automatic advanced ECS sync requires reviewed ecs.advanced configuration")
		}
		if e.SyncPolicy.SelfHeal && mode != "automatic" {
			return fmt.Errorf("ecs.syncPolicy.selfHeal requires automatic mode")
		}
	}
	if e.DeploymentController != "ECS" {
		return fmt.Errorf("ecs.deploymentController must be ECS; legacy CodeDeploy manifests must omit ecs execution inputs")
	}
	cluster, err := arn.Parse(e.ClusterARN)
	if err != nil || !validECSARN(cluster) || !ecsClusterPattern.MatchString(cluster.Resource) {
		return fmt.Errorf("ecs.clusterArn must be a full ECS cluster ARN")
	}
	service, err := arn.Parse(e.ServiceARN)
	if err != nil || !validECSARN(service) || !ecsServicePattern.MatchString(service.Resource) {
		return fmt.Errorf("ecs.serviceArn must be a full cluster-qualified ECS service ARN")
	}
	task, err := arn.Parse(e.BaseTaskDefinitionARN)
	if err != nil || !validECSARN(task) || !ecsTaskDefinitionPattern.MatchString(task.Resource) {
		return fmt.Errorf("ecs.baseTaskDefinitionArn must pin a positive task-definition revision")
	}
	for _, ref := range []arn.ARN{service, task} {
		if ref.Partition != cluster.Partition || ref.AccountID != cluster.AccountID || ref.Region != cluster.Region {
			return fmt.Errorf("ecs ARNs must use the same partition, account and region")
		}
	}
	if strings.Split(service.Resource, "/")[1] != strings.TrimPrefix(cluster.Resource, "cluster/") {
		return fmt.Errorf("ecs.serviceArn must belong to ecs.clusterArn")
	}
	if !ecsContainerPattern.MatchString(e.ContainerName) {
		return fmt.Errorf("ecs.containerName must be an explicit valid container name")
	}
	if !ecsImagePattern.MatchString(d.Image) {
		return fmt.Errorf("ecs image must be a container image reference; digests must use sha256 with 64 hex characters")
	}
	revision := task.Resource[strings.LastIndex(task.Resource, ":")+1:]
	if _, err := strconv.ParseInt(revision, 10, 32); err != nil {
		return fmt.Errorf("ecs base task-definition revision exceeds the supported range")
	}
	count := d.NormalizedCapacity().Desired
	if count > math.MaxInt32 {
		return fmt.Errorf("ecs desired count exceeds the supported range")
	}
	min, max := ECSRollingPercentages(d)
	if min < 0 || min > 100 || max < 100 || max > 200 || max <= min {
		return fmt.Errorf("ecs rolling percentages must allow progress: minimum 0..100, maximum 100..200 and maximum greater than minimum")
	}
	if d.NormalizedStrategy() == StrategyTypeRolling && int64(count)*int64(max)/100 == int64(count) && (int64(count)*int64(min)+99)/100 == int64(count) {
		return fmt.Errorf("ecs rolling percentages cannot start or stop a task at the desired count")
	}
	if d.Strategy.MinHealthyPercent != 0 && d.Strategy.MinHealthyPercent != min {
		return fmt.Errorf("strategy.minHealthyPercent conflicts with ecs.minimumHealthyPercent")
	}
	return validateECSAdvanced(d)
}

func validECSARN(ref arn.ARN) bool {
	return ref.Partition != "" && ref.Service == "ecs" && ref.Region != "" && awsAccountIDPattern.MatchString(ref.AccountID)
}

// ECSRollingPercentages makes defaults and explicit zero values unambiguous in plans.
func ECSRollingPercentages(d DeploymentSpec) (int, int) {
	min, max := 100, 200
	if d.Strategy.MinHealthyPercent != 0 {
		min = d.Strategy.MinHealthyPercent
	}
	if d.ECS != nil {
		if d.ECS.MinimumHealthyPercent != nil {
			min = *d.ECS.MinimumHealthyPercent
		}
		if d.ECS.MaximumPercent != nil {
			max = *d.ECS.MaximumPercent
		}
	}
	return min, max
}

// ValidateECSPlacement prevents manifest ARNs from escaping the selected environment.
func ValidateECSPlacement(d DeploymentSpec, accountID, region string) error {
	if d.ECS == nil {
		return nil
	}
	cluster, err := arn.Parse(d.ECS.ClusterARN)
	if err != nil || cluster.AccountID != accountID || cluster.Region != region {
		return fmt.Errorf("ecs target must match the environment account and region")
	}
	return nil
}

type ECSSyncPolicy struct {
	Mode     string `json:"mode,omitempty" yaml:"mode,omitempty"`
	SelfHeal bool   `json:"selfHeal,omitempty" yaml:"selfHeal,omitempty"`
}
