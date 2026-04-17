package manifest

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"math"
	gopath "path"
	"strings"

	"github.com/DevopsArtFactory/tenet/internal/domain"
	"github.com/DevopsArtFactory/tenet/internal/spec"
)

// CatalogSpec is the parser-facing bundle of desired resources.
type CatalogSpec struct {
	Accounts     []spec.AccountSpec
	Environments []spec.EnvironmentSpec
	Deployments  []spec.DeploymentSpec
}

// Catalog is the runtime-facing bundle used by controller components.
type Catalog struct {
	Accounts     []domain.Account
	Environments []domain.Environment
	Deployments  []domain.Deployment
}

// Compile validates a spec catalog and converts it into runtime domain objects.
func Compile(input CatalogSpec) (Catalog, error) {
	compiled := Catalog{
		Accounts:     make([]domain.Account, 0, len(input.Accounts)),
		Environments: make([]domain.Environment, 0, len(input.Environments)),
		Deployments:  make([]domain.Deployment, 0, len(input.Deployments)),
	}

	accountIndex := make(map[string]domain.Account, len(input.Accounts))
	accountIDIndex := make(map[string]string, len(input.Accounts))
	for _, account := range input.Accounts {
		if err := spec.ValidateAccount(account); err != nil {
			return Catalog{}, fmt.Errorf("validate account %q: %w", account.Name, err)
		}
		compiledAccount := domain.Account{
			Name:           account.Name,
			AWSAccountID:   account.AWSAccountID,
			AllowedRegions: append([]string(nil), account.AllowedRegions...),
		}
		if _, exists := accountIndex[account.Name]; exists {
			return Catalog{}, fmt.Errorf("duplicate account %q", account.Name)
		}
		if existingName, exists := accountIDIndex[account.AWSAccountID]; exists {
			return Catalog{}, fmt.Errorf("duplicate AWS account ID %q for accounts %q and %q", account.AWSAccountID, existingName, account.Name)
		}
		compiled.Accounts = append(compiled.Accounts, compiledAccount)
		accountIndex[account.Name] = compiledAccount
		accountIDIndex[account.AWSAccountID] = account.Name
	}

	environmentIndex := make(map[string]domain.Environment, len(input.Environments))
	for _, environment := range input.Environments {
		if err := spec.ValidateEnvironment(environment); err != nil {
			return Catalog{}, fmt.Errorf("validate environment %q: %w", environment.Name, err)
		}
		account, ok := accountIndex[environment.AccountRef]
		if !ok {
			return Catalog{}, fmt.Errorf("environment %q references unknown account %q", environment.Name, environment.AccountRef)
		}
		if !account.AllowsRegion(environment.Region) {
			return Catalog{}, fmt.Errorf("environment %q uses disallowed region %q for account %q", environment.Name, environment.Region, environment.AccountRef)
		}
		compiledEnvironment := domain.Environment{
			Name:       environment.Name,
			AccountRef: environment.AccountRef,
			Region:     environment.Region,
			NetworkRef: environment.NetworkRef,
			PolicyRef:  environment.PolicyRef,
		}
		if _, exists := environmentIndex[environment.Name]; exists {
			return Catalog{}, fmt.Errorf("duplicate environment %q", environment.Name)
		}
		compiled.Environments = append(compiled.Environments, compiledEnvironment)
		environmentIndex[environment.Name] = compiledEnvironment
	}

	deploymentIndex := make(map[string]struct{}, len(input.Deployments))
	for _, deployment := range input.Deployments {
		if err := spec.ValidateDeployment(deployment); err != nil {
			return Catalog{}, fmt.Errorf("validate deployment %q: %w", deployment.Name, err)
		}
		if _, exists := deploymentIndex[deployment.Name]; exists {
			return Catalog{}, fmt.Errorf("duplicate deployment %q", deployment.Name)
		}
		environmentRef := deployment.NormalizedEnvironmentRef()
		if _, ok := environmentIndex[environmentRef]; !ok {
			return Catalog{}, fmt.Errorf("deployment %q references unknown environment %q", deployment.Name, environmentRef)
		}
		strategy := deployment.NormalizedStrategy()
		compiledDeployment := domain.Deployment{
			Name:           deployment.Name,
			AppRef:         deployment.NormalizedAppRef(),
			EnvironmentRef: environmentRef,
			TargetType:     string(deployment.NormalizedTargetType()),
			Strategy:       string(strategy),
			ExecutionMode:  executionMode(deployment.NormalizedTargetType(), strategy),
			DesiredArtifact: domain.DeploymentArtifact{
				Binary: deployment.Binary,
				Image:  deployment.Image,
			},
			UserData:      normalizeUserData(deployment),
			RolloutConfig: normalizeRolloutConfig(deployment),
		}
		environment := environmentIndex[environmentRef]
		account := accountIndex[environment.AccountRef]
		compiledDeployment.Generation = deploymentGeneration(compiledDeployment, deployment, environment, account)
		compiled.Deployments = append(compiled.Deployments, compiledDeployment)
		if len(deployment.Stacks) > 0 {
			stack := deployment.Stacks[0]
			if stack.Account != environment.AccountRef {
				return Catalog{}, fmt.Errorf("deployment %q stack account %q does not match environment account %q", deployment.Name, stack.Account, environment.AccountRef)
			}
			if stack.Regions[0].Region != environment.Region {
				return Catalog{}, fmt.Errorf("deployment %q stack region %q does not match environment region %q", deployment.Name, stack.Regions[0].Region, environment.Region)
			}
		}
		deploymentIndex[deployment.Name] = struct{}{}
	}

	return compiled, nil
}

func deploymentGeneration(deployment domain.Deployment, desired spec.DeploymentSpec, environment domain.Environment, account domain.Account) int64 {
	payload, err := json.Marshal(struct {
		Name            string                         `json:"name"`
		AppRef          string                         `json:"appRef"`
		EnvironmentRef  string                         `json:"environmentRef"`
		Environment     deploymentEnvironmentHash      `json:"environment"`
		Account         deploymentAccountHash          `json:"account"`
		TargetType      string                         `json:"targetType"`
		Strategy        string                         `json:"strategy"`
		ExecutionMode   string                         `json:"executionMode"`
		DesiredArtifact domain.DeploymentArtifact      `json:"desiredArtifact"`
		Traffic         spec.TrafficSpec               `json:"traffic"`
		RolloutConfig   domain.DeploymentRolloutConfig `json:"rolloutConfig"`
		UserData        domain.DeploymentUserData      `json:"userData"`
		Tags            []string                       `json:"tags"`
	}{
		Name:           deployment.Name,
		AppRef:         deployment.AppRef,
		EnvironmentRef: deployment.EnvironmentRef,
		Environment: deploymentEnvironmentHash{
			Name:       environment.Name,
			AccountRef: environment.AccountRef,
			Region:     environment.Region,
			NetworkRef: environment.NetworkRef,
			PolicyRef:  environment.PolicyRef,
		},
		Account: deploymentAccountHash{
			Name:           account.Name,
			AWSAccountID:   account.AWSAccountID,
			AllowedRegions: append([]string(nil), account.AllowedRegions...),
		},
		TargetType:      deployment.TargetType,
		Strategy:        deployment.Strategy,
		ExecutionMode:   deployment.ExecutionMode,
		DesiredArtifact: deployment.DesiredArtifact,
		Traffic:         desired.Traffic,
		RolloutConfig:   deployment.RolloutConfig,
		UserData:        deployment.UserData,
		Tags:            append([]string(nil), desired.Tags...),
	})
	if err != nil {
		panic(fmt.Sprintf("marshal deployment generation payload: %v", err))
	}

	hasher := fnv.New64a()
	_, _ = hasher.Write(payload)

	generation := int64(hasher.Sum64() & math.MaxInt64)
	if generation == 0 {
		return 1
	}
	return generation
}

type deploymentEnvironmentHash struct {
	Name       string `json:"name"`
	AccountRef string `json:"accountRef"`
	Region     string `json:"region"`
	NetworkRef string `json:"networkRef,omitempty"`
	PolicyRef  string `json:"policyRef,omitempty"`
}

type deploymentAccountHash struct {
	Name           string   `json:"name"`
	AWSAccountID   string   `json:"awsAccountId"`
	AllowedRegions []string `json:"allowedRegions,omitempty"`
}

func executionMode(target spec.TargetType, strategy spec.StrategyType) string {
	switch target {
	case spec.TargetTypeEC2:
		switch strategy {
		case spec.StrategyTypeBlueGreen, spec.StrategyTypeCanary, spec.StrategyTypeLinear:
			return "asgWeightedTargetGroups"
		case spec.StrategyTypeRolling:
			return "asgInstanceRefresh"
		}
	case spec.TargetTypeECS:
		switch strategy {
		case spec.StrategyTypeRolling:
			return "ecsRolling"
		case spec.StrategyTypeBlueGreen, spec.StrategyTypeCanary, spec.StrategyTypeLinear:
			return "ecsCodeDeploy"
		}
	}
	return ""
}

func normalizeUserData(deployment spec.DeploymentSpec) domain.DeploymentUserData {
	userData := deployment.UserData
	if userData.Type == "" && userData.Path == "" && len(deployment.Stacks) > 0 {
		userData = deployment.Stacks[0].UserData
	}
	return domain.DeploymentUserData{
		Type: strings.TrimSpace(userData.Type),
		Path: normalizeUserDataPath(userData.Path),
	}
}

func normalizeRolloutConfig(deployment spec.DeploymentSpec) domain.DeploymentRolloutConfig {
	capacity := deployment.NormalizedCapacity()
	config := domain.DeploymentRolloutConfig{
		Strategy: domain.DeploymentRolloutStrategy{
			WarmupSeconds:     deployment.Strategy.WarmupSeconds,
			MinHealthyPercent: deployment.Strategy.MinHealthyPercent,
		},
		Capacity: domain.DeploymentRolloutCapacity{
			Desired: capacity.Desired,
			Min:     capacity.Min,
			Max:     capacity.Max,
		},
		Stacks: make([]domain.DeploymentRolloutStack, 0, len(deployment.Stacks)),
	}
	for _, stack := range deployment.Stacks {
		config.Stacks = append(config.Stacks, domain.DeploymentRolloutStack{
			TerminationDelayRate:       stack.TerminationDelayRate,
			RollingUpdateInstanceCount: stack.RollingUpdateInstanceCount,
		})
	}
	return config
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
