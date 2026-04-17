package spec

import (
	"fmt"
	gopath "path"
	"regexp"
	"strings"
)

var awsAccountIDPattern = regexp.MustCompile(`^\d{12}$`)

// ValidateAccount checks the minimum correctness of a target-account declaration.
func ValidateAccount(account AccountSpec) error {
	if account.Name == "" {
		return fmt.Errorf("account name is required")
	}
	if !awsAccountIDPattern.MatchString(account.AWSAccountID) {
		return fmt.Errorf("aws account id must be 12 digits")
	}
	if account.RoleARN == "" {
		return fmt.Errorf("role ARN is required")
	}
	return nil
}

// ValidateEnvironment checks that an environment can be resolved to a placement.
func ValidateEnvironment(environment EnvironmentSpec) error {
	if environment.Name == "" {
		return fmt.Errorf("environment name is required")
	}
	if environment.AccountRef == "" {
		return fmt.Errorf("environment accountRef is required")
	}
	if environment.Region == "" {
		return fmt.Errorf("environment region is required")
	}
	return nil
}

// ValidateDeployment checks the smallest useful deployment constraints.
func ValidateDeployment(deployment DeploymentSpec) error {
	if deployment.Name == "" {
		return fmt.Errorf("deployment name is required")
	}
	if deployment.NormalizedAppRef() == "" {
		return fmt.Errorf("deployment appRef is required")
	}
	if deployment.NormalizedEnvironmentRef() == "" {
		return fmt.Errorf("deployment environmentRef is required")
	}

	targetType := deployment.NormalizedTargetType()
	switch targetType {
	case TargetTypeEC2, TargetTypeECS:
	default:
		return fmt.Errorf("unsupported target type %q", targetType)
	}

	strategyType := deployment.NormalizedStrategy()
	switch strategyType {
	case StrategyTypeBlueGreen, StrategyTypeRolling, StrategyTypeCanary, StrategyTypeLinear:
	default:
		return fmt.Errorf("unsupported strategy type %q", strategyType)
	}

	capacity := deployment.NormalizedCapacity()
	if capacity.Min <= 0 || capacity.Max <= 0 || capacity.Desired <= 0 {
		return fmt.Errorf("capacity values must be positive")
	}
	if capacity.Min > capacity.Max {
		return fmt.Errorf("capacity min cannot exceed max")
	}
	if capacity.Desired < capacity.Min || capacity.Desired > capacity.Max {
		return fmt.Errorf("capacity desired must be within min/max")
	}

	if deployment.Traffic.Port != 0 && (deployment.Traffic.Port < 1 || deployment.Traffic.Port > 65535) {
		return fmt.Errorf("traffic port must be between 1 and 65535")
	}
	if deployment.Strategy.MinHealthyPercent < 0 || deployment.Strategy.MinHealthyPercent > 100 {
		return fmt.Errorf("minHealthyPercent must be between 0 and 100")
	}
	if deployment.Strategy.WarmupSeconds < 0 {
		return fmt.Errorf("warmupSeconds cannot be negative")
	}
	if len(deployment.Stacks) > 0 {
		if len(deployment.Stacks) != 1 {
			return fmt.Errorf("multiple deployment stacks are not supported yet")
		}
		first := deployment.Stacks[0]
		if first.Account == "" {
			return fmt.Errorf("deployment stack account is required")
		}
		if first.Env == "" {
			return fmt.Errorf("deployment stack env is required")
		}
		if len(first.Regions) == 0 {
			return fmt.Errorf("deployment stack must declare at least one region")
		}
		if len(first.Regions) != 1 {
			return fmt.Errorf("multiple deployment stack regions are not supported yet")
		}
		if first.Regions[0].Region == "" {
			return fmt.Errorf("deployment stack region is required")
		}
	}
	if deployment.Image != "" && targetType != TargetTypeECS {
		return fmt.Errorf("image artifact requires ecs target")
	}
	if deployment.Binary != "" && targetType != TargetTypeEC2 {
		return fmt.Errorf("binary artifact requires ec2 target")
	}
	if targetType == TargetTypeEC2 {
		userData := effectiveUserData(deployment)
		if strings.TrimSpace(userData.Type) == "" || normalizeUserDataPath(userData.Path) == "" {
			return fmt.Errorf("EC2 deployments require userdata bootstrap metadata for immutable capacity and rolling replacement capacity: both userdata type and path are required")
		}
	}

	return nil
}

func effectiveUserData(deployment DeploymentSpec) UserDataSpec {
	userData := deployment.UserData
	if strings.TrimSpace(userData.Type) == "" && strings.TrimSpace(userData.Path) == "" && len(deployment.Stacks) > 0 {
		return deployment.Stacks[0].UserData
	}
	return userData
}

func normalizeUserDataPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	cleaned := gopath.Clean(strings.ReplaceAll(path, "\\", "/"))
	for strings.HasPrefix(cleaned, "./") {
		cleaned = strings.TrimPrefix(cleaned, "./")
	}
	if cleaned == "." {
		return ""
	}
	return cleaned
}
