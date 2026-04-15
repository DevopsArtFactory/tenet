package placement

import (
	"fmt"

	"github.com/DevopsArtFactory/tenet/internal/domain"
)

// Placement is the resolved execution context for a deployment.
type Placement struct {
	AccountName string
	AccountID   string
	Region      string
	NetworkRef  string
}

// Resolver resolves account and environment references into an execution placement.
type Resolver struct{}

// Resolve returns the concrete placement for a deployment.
func (Resolver) Resolve(accounts []domain.Account, environments []domain.Environment, deployment domain.Deployment) (Placement, error) {
	environment, ok := findEnvironment(environments, deployment.EnvironmentRef)
	if !ok {
		return Placement{}, fmt.Errorf("environment %q not found", deployment.EnvironmentRef)
	}

	account, ok := findAccount(accounts, environment.AccountRef)
	if !ok {
		return Placement{}, fmt.Errorf("account %q not found", environment.AccountRef)
	}

	if !account.AllowsRegion(environment.Region) {
		return Placement{}, fmt.Errorf("region %q is not allowed for account %q", environment.Region, account.Name)
	}

	return Placement{
		AccountName: account.Name,
		AccountID:   account.AWSAccountID,
		Region:      environment.Region,
		NetworkRef:  environment.NetworkRef,
	}, nil
}

func findEnvironment(environments []domain.Environment, name string) (domain.Environment, bool) {
	for _, environment := range environments {
		if environment.Name == name {
			return environment, true
		}
	}

	return domain.Environment{}, false
}

func findAccount(accounts []domain.Account, name string) (domain.Account, bool) {
	for _, account := range accounts {
		if account.Name == name {
			return account, true
		}
	}

	return domain.Account{}, false
}
