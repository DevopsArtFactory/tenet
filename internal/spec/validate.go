package spec

import (
	"fmt"
	"regexp"
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
	if deployment.AppRef == "" {
		return fmt.Errorf("deployment appRef is required")
	}
	if deployment.EnvironmentRef == "" {
		return fmt.Errorf("deployment environmentRef is required")
	}

	switch deployment.Target.Type {
	case TargetTypeEC2, TargetTypeECS:
	default:
		return fmt.Errorf("unsupported target type %q", deployment.Target.Type)
	}

	switch deployment.Strategy.Type {
	case StrategyTypeBlueGreen, StrategyTypeRolling:
	default:
		return fmt.Errorf("unsupported strategy type %q", deployment.Strategy.Type)
	}

	if deployment.Traffic.Port <= 0 || deployment.Traffic.Port > 65535 {
		return fmt.Errorf("traffic port must be between 1 and 65535")
	}
	if deployment.Capacity.Min <= 0 || deployment.Capacity.Max <= 0 || deployment.Capacity.Desired <= 0 {
		return fmt.Errorf("capacity values must be positive")
	}
	if deployment.Capacity.Min > deployment.Capacity.Max {
		return fmt.Errorf("capacity min cannot exceed max")
	}
	if deployment.Capacity.Desired < deployment.Capacity.Min || deployment.Capacity.Desired > deployment.Capacity.Max {
		return fmt.Errorf("capacity desired must be within min/max")
	}
	if deployment.Strategy.MinHealthyPercent < 0 || deployment.Strategy.MinHealthyPercent > 100 {
		return fmt.Errorf("minHealthyPercent must be between 0 and 100")
	}
	if deployment.Strategy.WarmupSeconds < 0 {
		return fmt.Errorf("warmupSeconds cannot be negative")
	}

	return nil
}
