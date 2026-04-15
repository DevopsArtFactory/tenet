package catalog

import (
	"fmt"
	"sort"

	"github.com/DevopsArtFactory/tenet/internal/domain"
	"github.com/DevopsArtFactory/tenet/internal/manifest"
	"github.com/DevopsArtFactory/tenet/internal/placement"
	"github.com/DevopsArtFactory/tenet/internal/statusstore"
)

// AccountOverview groups deployment state under a single account.
type AccountOverview struct {
	Name           string               `json:"name"`
	AWSAccountID   string               `json:"awsAccountId"`
	AllowedRegions []string             `json:"allowedRegions,omitempty"`
	Deployments    []DeploymentOverview `json:"deployments"`
}

// DeploymentOverview joins desired deployment intent with current runtime status.
type DeploymentOverview struct {
	Name               string                    `json:"name"`
	AppRef             string                    `json:"appRef"`
	EnvironmentRef     string                    `json:"environmentRef"`
	AccountName        string                    `json:"accountName"`
	AWSAccountID       string                    `json:"awsAccountId"`
	Region             string                    `json:"region"`
	TargetType         string                    `json:"targetType,omitempty"`
	Strategy           string                    `json:"strategy,omitempty"`
	ExecutionMode      string                    `json:"executionMode,omitempty"`
	DesiredArtifact    domain.DeploymentArtifact `json:"desiredArtifact,omitempty"`
	DeployedArtifact   domain.DeploymentArtifact `json:"deployedArtifact,omitempty"`
	ObservedGeneration int64                     `json:"observedGeneration,omitempty"`
	Sync               domain.Sync               `json:"sync"`
	Health             domain.Health             `json:"health"`
	Rollout            *domain.RolloutSummary    `json:"rollout,omitempty"`
	Conditions         []domain.Condition        `json:"conditions,omitempty"`
}

// DeploymentOverviewDetail joins one deployment's desired intent, placement, and observed state.
type DeploymentOverviewDetail struct {
	Name               string                    `json:"name"`
	AppRef             string                    `json:"appRef"`
	EnvironmentRef     string                    `json:"environmentRef"`
	AccountName        string                    `json:"accountName"`
	AWSAccountID       string                    `json:"awsAccountId"`
	AllowedRegions     []string                  `json:"allowedRegions,omitempty"`
	Region             string                    `json:"region"`
	TargetType         string                    `json:"targetType,omitempty"`
	Strategy           string                    `json:"strategy,omitempty"`
	ExecutionMode      string                    `json:"executionMode,omitempty"`
	DesiredArtifact    domain.DeploymentArtifact `json:"desiredArtifact,omitempty"`
	DeployedArtifact   domain.DeploymentArtifact `json:"deployedArtifact,omitempty"`
	ObservedGeneration int64                     `json:"observedGeneration,omitempty"`
	Sync               domain.Sync               `json:"sync"`
	Health             domain.Health             `json:"health"`
	Rollout            *domain.RolloutSummary    `json:"rollout,omitempty"`
	Conditions         []domain.Condition        `json:"conditions,omitempty"`
}

// BuildAccountOverviews groups placement-aware deployment state by account.
func BuildAccountOverviews(c manifest.Catalog, snapshots []statusstore.Snapshot) ([]AccountOverview, error) {
	placementViews, err := ListPlacements(c)
	if err != nil {
		return nil, err
	}

	statusByDeployment := make(map[string]domain.DeploymentStatus, len(snapshots))
	for _, snapshot := range snapshots {
		statusByDeployment[snapshot.DeploymentName] = snapshot.Status
	}

	deploymentByName := make(map[string]domain.Deployment, len(c.Deployments))
	for _, deployment := range c.Deployments {
		deploymentByName[deployment.Name] = deployment
	}

	accountMap := make(map[string]*AccountOverview, len(c.Accounts))
	for _, account := range c.Accounts {
		accountCopy := AccountOverview{
			Name:           account.Name,
			AWSAccountID:   account.AWSAccountID,
			AllowedRegions: append([]string(nil), account.AllowedRegions...),
			Deployments:    []DeploymentOverview{},
		}
		accountMap[account.Name] = &accountCopy
	}

	for _, placement := range placementViews {
		account := accountMap[placement.AccountName]
		if account == nil {
			continue
		}
		deployment, ok := deploymentByName[placement.DeploymentName]
		if !ok {
			continue
		}
		status := statusByDeployment[placement.DeploymentName]
		account.Deployments = append(account.Deployments, DeploymentOverview{
			Name:               deployment.Name,
			AppRef:             deployment.AppRef,
			EnvironmentRef:     deployment.EnvironmentRef,
			AccountName:        placement.AccountName,
			AWSAccountID:       placement.AWSAccountID,
			Region:             placement.Region,
			TargetType:         deployment.TargetType,
			Strategy:           deployment.Strategy,
			ExecutionMode:      deployment.ExecutionMode,
			DesiredArtifact:    deployment.DesiredArtifact,
			DeployedArtifact:   status.DeployedArtifact,
			ObservedGeneration: status.ObservedGeneration,
			Sync:               status.Sync,
			Health:             status.Health,
			Rollout:            cloneRolloutSummary(status.Rollout),
			Conditions:         append([]domain.Condition(nil), status.Conditions...),
		})
	}

	accounts := make([]AccountOverview, 0, len(accountMap))
	for _, account := range accountMap {
		sort.Slice(account.Deployments, func(i, j int) bool {
			return account.Deployments[i].Name < account.Deployments[j].Name
		})
		accounts = append(accounts, *account)
	}

	sort.Slice(accounts, func(i, j int) bool {
		return accounts[i].Name < accounts[j].Name
	})
	return accounts, nil
}

// BuildDeploymentOverviewDetail joins one deployment's placement-aware desired and observed state.
func BuildDeploymentOverviewDetail(c manifest.Catalog, snapshots []statusstore.Snapshot, deploymentName string) (DeploymentOverviewDetail, error) {
	statusByDeployment := make(map[string]domain.DeploymentStatus, len(snapshots))
	for _, snapshot := range snapshots {
		statusByDeployment[snapshot.DeploymentName] = snapshot.Status
	}

	deploymentByName := make(map[string]domain.Deployment, len(c.Deployments))
	for _, deployment := range c.Deployments {
		deploymentByName[deployment.Name] = deployment
	}

	deployment, ok := deploymentByName[deploymentName]
	if !ok {
		return DeploymentOverviewDetail{}, fmt.Errorf("%w: %s", statusstore.ErrNotFound, deploymentName)
	}

	resolver := placement.Resolver{}
	resolved, err := resolver.Resolve(c.Accounts, c.Environments, deployment)
	if err != nil {
		return DeploymentOverviewDetail{}, err
	}

	accountByName := make(map[string]domain.Account, len(c.Accounts))
	for _, account := range c.Accounts {
		accountByName[account.Name] = account
	}
	account := accountByName[resolved.AccountName]
	status := statusByDeployment[deploymentName]

	return DeploymentOverviewDetail{
		Name:               deployment.Name,
		AppRef:             deployment.AppRef,
		EnvironmentRef:     deployment.EnvironmentRef,
		AccountName:        resolved.AccountName,
		AWSAccountID:       resolved.AccountID,
		AllowedRegions:     append([]string(nil), account.AllowedRegions...),
		Region:             resolved.Region,
		TargetType:         deployment.TargetType,
		Strategy:           deployment.Strategy,
		ExecutionMode:      deployment.ExecutionMode,
		DesiredArtifact:    deployment.DesiredArtifact,
		DeployedArtifact:   status.DeployedArtifact,
		ObservedGeneration: status.ObservedGeneration,
		Sync:               status.Sync,
		Health:             status.Health,
		Rollout:            cloneRolloutSummary(status.Rollout),
		Conditions:         append([]domain.Condition(nil), status.Conditions...),
	}, nil
}

func cloneRolloutSummary(summary *domain.RolloutSummary) *domain.RolloutSummary {
	if summary == nil {
		return nil
	}
	copy := *summary
	copy.Steps = append([]domain.RolloutStepSummary(nil), summary.Steps...)
	copy.Execution = append([]domain.RolloutExecutionDetail(nil), summary.Execution...)
	return &copy
}
