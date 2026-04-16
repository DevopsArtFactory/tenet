package catalog

import (
	"errors"
	"fmt"
	"sort"

	"github.com/DevopsArtFactory/tenet/internal/domain"
	"github.com/DevopsArtFactory/tenet/internal/manifest"
	"github.com/DevopsArtFactory/tenet/internal/placement"
	"github.com/DevopsArtFactory/tenet/internal/statusstore"
)

var errAmbiguousAccountOverviewDetail = errors.New("account overview detail is ambiguous")

// AccountOverview groups deployment state under a single account.
type AccountOverview struct {
	Name             string               `json:"name"`
	AWSAccountID     string               `json:"awsAccountId"`
	AllowedRegions   []string             `json:"allowedRegions,omitempty"`
	DeploymentCount  int                  `json:"deploymentCount"`
	SyncedCount      int                  `json:"syncedCount"`
	ProgressingCount int                  `json:"progressingCount"`
	PausedCount      int                  `json:"pausedCount"`
	ErrorCount       int                  `json:"errorCount"`
	Deployments      []DeploymentOverview `json:"deployments"`
}

// AccountOverviewDetail describes one account's deployment state, keyed by AWS account ID.
type AccountOverviewDetail struct {
	Name             string               `json:"name"`
	AWSAccountID     string               `json:"awsAccountId"`
	AllowedRegions   []string             `json:"allowedRegions,omitempty"`
	DeploymentCount  int                  `json:"deploymentCount"`
	SyncedCount      int                  `json:"syncedCount"`
	ProgressingCount int                  `json:"progressingCount"`
	PausedCount      int                  `json:"pausedCount"`
	ErrorCount       int                  `json:"errorCount"`
	Deployments      []DeploymentOverview `json:"deployments"`
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
	RuntimeSummary     *domain.RuntimeSummary    `json:"runtimeSummary,omitempty"`
	Rollout            *domain.RolloutSummary    `json:"rollout,omitempty"`
	Conditions         []domain.Condition        `json:"conditions,omitempty"`
}

// DeploymentOverviewDetail joins one deployment's desired intent, placement, and observed state.
type DeploymentOverviewDetail struct {
	Name               string                      `json:"name"`
	AppRef             string                      `json:"appRef"`
	EnvironmentRef     string                      `json:"environmentRef"`
	AccountName        string                      `json:"accountName"`
	AWSAccountID       string                      `json:"awsAccountId"`
	AllowedRegions     []string                    `json:"allowedRegions,omitempty"`
	Region             string                      `json:"region"`
	TargetType         string                      `json:"targetType,omitempty"`
	Strategy           string                      `json:"strategy,omitempty"`
	ExecutionMode      string                      `json:"executionMode,omitempty"`
	DesiredArtifact    domain.DeploymentArtifact   `json:"desiredArtifact,omitempty"`
	DeployedArtifact   domain.DeploymentArtifact   `json:"deployedArtifact,omitempty"`
	ObservedGeneration int64                       `json:"observedGeneration,omitempty"`
	Sync               domain.Sync                 `json:"sync"`
	Health             domain.Health               `json:"health"`
	RuntimeSummary     *domain.RuntimeSummary      `json:"runtimeSummary,omitempty"`
	Rollout            *domain.RolloutSummary      `json:"rollout,omitempty"`
	Conditions         []domain.Condition          `json:"conditions,omitempty"`
	History            []domain.StatusHistoryEntry `json:"history,omitempty"`
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
		account.DeploymentCount++
		switch status.Sync.State {
		case domain.SyncStateSynced:
			account.SyncedCount++
		case domain.SyncStateReconciling, domain.SyncStateOutOfSync, domain.SyncStateDrifted:
			account.ProgressingCount++
		case domain.SyncStatePaused:
			account.PausedCount++
		case domain.SyncStateError:
			account.ErrorCount++
		}
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
			RuntimeSummary:     cloneRuntimeSummary(deriveRuntimeSummary(status)),
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

// BuildAccountOverviewDetail returns one account overview by exact AWS account ID.
func BuildAccountOverviewDetail(c manifest.Catalog, snapshots []statusstore.Snapshot, awsAccountID string) (AccountOverviewDetail, error) {
	accounts, err := BuildAccountOverviews(c, snapshots)
	if err != nil {
		return AccountOverviewDetail{}, err
	}
	var (
		matched AccountOverview
		found   bool
	)
	for _, account := range accounts {
		if account.AWSAccountID != awsAccountID {
			continue
		}
		if found {
			return AccountOverviewDetail{}, fmt.Errorf("%w: %s", errAmbiguousAccountOverviewDetail, awsAccountID)
		}
		matched = account
		found = true
	}
	if found {
		return AccountOverviewDetail{
			Name:             matched.Name,
			AWSAccountID:     matched.AWSAccountID,
			AllowedRegions:   append([]string(nil), matched.AllowedRegions...),
			DeploymentCount:  matched.DeploymentCount,
			SyncedCount:      matched.SyncedCount,
			ProgressingCount: matched.ProgressingCount,
			PausedCount:      matched.PausedCount,
			ErrorCount:       matched.ErrorCount,
			Deployments:      append([]DeploymentOverview(nil), matched.Deployments...),
		}, nil
	}
	return AccountOverviewDetail{}, fmt.Errorf("%w: %s", statusstore.ErrNotFound, awsAccountID)
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
		RuntimeSummary:     cloneRuntimeSummary(deriveRuntimeSummary(status)),
		Rollout:            cloneRolloutSummary(status.Rollout),
		Conditions:         append([]domain.Condition(nil), status.Conditions...),
		History:            cloneStatusHistory(status.History),
	}, nil
}

func deriveRuntimeSummary(status domain.DeploymentStatus) *domain.RuntimeSummary {
	if len(status.History) > 0 {
		latest := status.History[len(status.History)-1]
		return &domain.RuntimeSummary{
			Timestamp:          latest.Timestamp,
			Source:             latest.Source,
			Summary:            latest.Summary,
			Changes:            append([]string(nil), latest.Changes...),
			Sync:               latest.Sync,
			Health:             latest.Health,
			DesiredGeneration:  latest.DesiredGeneration,
			ObservedGeneration: latest.ObservedGeneration,
			RolloutPhase:       latest.RolloutPhase,
			CurrentStep:        latest.CurrentStep,
			HistoryCount:       len(status.History),
		}
	}
	if status.Sync == (domain.Sync{}) && status.Health == (domain.Health{}) && status.Rollout == nil {
		return nil
	}

	summary := &domain.RuntimeSummary{
		Sync:               status.Sync,
		Health:             status.Health,
		ObservedGeneration: status.ObservedGeneration,
		HistoryCount:       len(status.History),
	}
	if status.Rollout != nil {
		summary.Summary = status.Rollout.Summary
		summary.DesiredGeneration = status.Rollout.DesiredGeneration
		summary.ObservedGeneration = status.Rollout.ObservedGeneration
		summary.RolloutPhase = status.Rollout.Phase
		summary.CurrentStep = status.Rollout.CurrentStep
	}
	return summary
}

func cloneRuntimeSummary(summary *domain.RuntimeSummary) *domain.RuntimeSummary {
	if summary == nil {
		return nil
	}
	copy := *summary
	copy.Changes = append([]string(nil), summary.Changes...)
	return &copy
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

func cloneStatusHistory(history []domain.StatusHistoryEntry) []domain.StatusHistoryEntry {
	if len(history) == 0 {
		return nil
	}
	cloned := append([]domain.StatusHistoryEntry(nil), history...)
	for i := range cloned {
		cloned[i].Changes = append([]string(nil), history[i].Changes...)
	}
	return cloned
}
