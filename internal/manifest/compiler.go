package manifest

import (
	"fmt"
	"hash/fnv"
	"math"

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
		compiled.Accounts = append(compiled.Accounts, compiledAccount)
		accountIndex[account.Name] = compiledAccount
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
		}
		compiledDeployment.Generation = deploymentGeneration(compiledDeployment)
		compiled.Deployments = append(compiled.Deployments, compiledDeployment)
		if len(deployment.Stacks) > 0 {
			stack := deployment.Stacks[0]
			environment := environmentIndex[environmentRef]
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

func deploymentGeneration(deployment domain.Deployment) int64 {
	hasher := fnv.New64a()
	_, _ = hasher.Write([]byte(deployment.Name))
	_, _ = hasher.Write([]byte{0})
	_, _ = hasher.Write([]byte(deployment.AppRef))
	_, _ = hasher.Write([]byte{0})
	_, _ = hasher.Write([]byte(deployment.EnvironmentRef))
	_, _ = hasher.Write([]byte{0})
	_, _ = hasher.Write([]byte(deployment.TargetType))
	_, _ = hasher.Write([]byte{0})
	_, _ = hasher.Write([]byte(deployment.Strategy))
	_, _ = hasher.Write([]byte{0})
	_, _ = hasher.Write([]byte(deployment.ExecutionMode))
	_, _ = hasher.Write([]byte{0})
	_, _ = hasher.Write([]byte(deployment.DesiredArtifact.Binary))
	_, _ = hasher.Write([]byte{0})
	_, _ = hasher.Write([]byte(deployment.DesiredArtifact.Image))

	generation := int64(hasher.Sum64() & math.MaxInt64)
	if generation == 0 {
		return 1
	}
	return generation
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
