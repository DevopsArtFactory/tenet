package manifest

import (
	"fmt"

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
		if _, ok := environmentIndex[deployment.EnvironmentRef]; !ok {
			return Catalog{}, fmt.Errorf("deployment %q references unknown environment %q", deployment.Name, deployment.EnvironmentRef)
		}
		compiled.Deployments = append(compiled.Deployments, domain.Deployment{
			Name:           deployment.Name,
			AppRef:         deployment.AppRef,
			EnvironmentRef: deployment.EnvironmentRef,
		})
		deploymentIndex[deployment.Name] = struct{}{}
	}

	return compiled, nil
}
