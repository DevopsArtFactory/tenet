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
	ProjectCount     int                  `json:"projectCount"`
	EnvironmentCount int                  `json:"environmentCount"`
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
	ProjectCount     int                  `json:"projectCount"`
	EnvironmentCount int                  `json:"environmentCount"`
	SyncedCount      int                  `json:"syncedCount"`
	ProgressingCount int                  `json:"progressingCount"`
	PausedCount      int                  `json:"pausedCount"`
	ErrorCount       int                  `json:"errorCount"`
	Deployments      []DeploymentOverview `json:"deployments"`
}

// ProjectEnvironmentOverviewDetail describes one grouped project/environment slice under an account.
type ProjectEnvironmentOverviewDetail struct {
	AccountName      string               `json:"accountName"`
	AWSAccountID     string               `json:"awsAccountId"`
	AllowedRegions   []string             `json:"allowedRegions,omitempty"`
	AppRef           string               `json:"appRef"`
	EnvironmentRef   string               `json:"environmentRef"`
	Region           string               `json:"region,omitempty"`
	DeploymentCount  int                  `json:"deploymentCount"`
	SyncedCount      int                  `json:"syncedCount"`
	ProgressingCount int                  `json:"progressingCount"`
	PausedCount      int                  `json:"pausedCount"`
	ErrorCount       int                  `json:"errorCount"`
	Deployments      []DeploymentOverview `json:"deployments"`
}

type accountOverviewAccumulator struct {
	overview        AccountOverview
	projectRefs     map[string]struct{}
	environmentRefs map[string]struct{}
}

// ApplicationOverview flattens the account overview read model into one application row per deployment.
type ApplicationOverview struct {
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
	Name               string                         `json:"name"`
	AppRef             string                         `json:"appRef"`
	EnvironmentRef     string                         `json:"environmentRef"`
	AccountName        string                         `json:"accountName"`
	AWSAccountID       string                         `json:"awsAccountId"`
	AllowedRegions     []string                       `json:"allowedRegions,omitempty"`
	Region             string                         `json:"region"`
	TargetType         string                         `json:"targetType,omitempty"`
	Strategy           string                         `json:"strategy,omitempty"`
	ExecutionMode      string                         `json:"executionMode,omitempty"`
	DesiredArtifact    domain.DeploymentArtifact      `json:"desiredArtifact,omitempty"`
	UserData           domain.DeploymentUserData      `json:"userData,omitempty"`
	RolloutConfig      domain.DeploymentRolloutConfig `json:"rolloutConfig,omitempty"`
	DeployedArtifact   domain.DeploymentArtifact      `json:"deployedArtifact,omitempty"`
	ObservedGeneration int64                          `json:"observedGeneration,omitempty"`
	Sync               domain.Sync                    `json:"sync"`
	Health             domain.Health                  `json:"health"`
	RuntimeSummary     *domain.RuntimeSummary         `json:"runtimeSummary,omitempty"`
	Rollout            *domain.RolloutSummary         `json:"rollout,omitempty"`
	Conditions         []domain.Condition             `json:"conditions,omitempty"`
	History            []domain.StatusHistoryEntry    `json:"history,omitempty"`
}

// DeploymentOverviewEvents exposes controller-derived runtime history for one deployment.
type DeploymentOverviewEvents struct {
	DeploymentName string                      `json:"deploymentName"`
	Events         []domain.StatusHistoryEntry `json:"events"`
}

// ApplicationOverviewDetail makes the application page contract explicit while preserving deployment detail semantics.
type ApplicationOverviewDetail struct {
	Name               string                         `json:"name"`
	AppRef             string                         `json:"appRef"`
	EnvironmentRef     string                         `json:"environmentRef"`
	AccountName        string                         `json:"accountName"`
	AWSAccountID       string                         `json:"awsAccountId"`
	AllowedRegions     []string                       `json:"allowedRegions,omitempty"`
	Region             string                         `json:"region"`
	TargetType         string                         `json:"targetType,omitempty"`
	Strategy           string                         `json:"strategy,omitempty"`
	ExecutionMode      string                         `json:"executionMode,omitempty"`
	DesiredArtifact    domain.DeploymentArtifact      `json:"desiredArtifact,omitempty"`
	UserData           domain.DeploymentUserData      `json:"userData,omitempty"`
	RolloutConfig      domain.DeploymentRolloutConfig `json:"rolloutConfig,omitempty"`
	DeployedArtifact   domain.DeploymentArtifact      `json:"deployedArtifact,omitempty"`
	ObservedGeneration int64                          `json:"observedGeneration,omitempty"`
	Sync               domain.Sync                    `json:"sync"`
	Health             domain.Health                  `json:"health"`
	RuntimeSummary     *domain.RuntimeSummary         `json:"runtimeSummary,omitempty"`
	Rollout            *domain.RolloutSummary         `json:"rollout,omitempty"`
	Conditions         []domain.Condition             `json:"conditions,omitempty"`
	History            []domain.StatusHistoryEntry    `json:"history,omitempty"`
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

	accountMap := make(map[string]*accountOverviewAccumulator, len(c.Accounts))
	for _, account := range c.Accounts {
		accountMap[account.Name] = &accountOverviewAccumulator{
			overview: AccountOverview{
				Name:           account.Name,
				AWSAccountID:   account.AWSAccountID,
				AllowedRegions: append([]string(nil), account.AllowedRegions...),
				Deployments:    []DeploymentOverview{},
			},
			projectRefs:     make(map[string]struct{}),
			environmentRefs: make(map[string]struct{}),
		}
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
		account.overview.DeploymentCount++
		account.projectRefs[deployment.AppRef] = struct{}{}
		account.environmentRefs[deployment.EnvironmentRef] = struct{}{}
		switch status.Sync.State {
		case domain.SyncStateSynced:
			account.overview.SyncedCount++
		case domain.SyncStateReconciling, domain.SyncStateOutOfSync, domain.SyncStateDrifted:
			account.overview.ProgressingCount++
		case domain.SyncStatePaused:
			account.overview.PausedCount++
		case domain.SyncStateError:
			account.overview.ErrorCount++
		}
		account.overview.Deployments = append(account.overview.Deployments, DeploymentOverview{
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
		account.overview.ProjectCount = len(account.projectRefs)
		account.overview.EnvironmentCount = len(account.environmentRefs)
		sort.Slice(account.overview.Deployments, func(i, j int) bool {
			return account.overview.Deployments[i].Name < account.overview.Deployments[j].Name
		})
		accounts = append(accounts, account.overview)
	}

	sort.Slice(accounts, func(i, j int) bool {
		return accounts[i].Name < accounts[j].Name
	})
	return accounts, nil
}

// BuildApplicationOverviews flattens account overviews into one application row per deployment.
func BuildApplicationOverviews(c manifest.Catalog, snapshots []statusstore.Snapshot) ([]ApplicationOverview, error) {
	accounts, err := BuildAccountOverviews(c, snapshots)
	if err != nil {
		return nil, err
	}

	applications := make([]ApplicationOverview, 0)
	for _, account := range accounts {
		for _, deployment := range account.Deployments {
			applications = append(applications, ApplicationOverview{
				Name:               deployment.Name,
				AppRef:             deployment.AppRef,
				EnvironmentRef:     deployment.EnvironmentRef,
				AccountName:        deployment.AccountName,
				AWSAccountID:       deployment.AWSAccountID,
				Region:             deployment.Region,
				TargetType:         deployment.TargetType,
				Strategy:           deployment.Strategy,
				ExecutionMode:      deployment.ExecutionMode,
				DesiredArtifact:    deployment.DesiredArtifact,
				DeployedArtifact:   deployment.DeployedArtifact,
				ObservedGeneration: deployment.ObservedGeneration,
				Sync:               deployment.Sync,
				Health:             deployment.Health,
				RuntimeSummary:     cloneRuntimeSummary(deployment.RuntimeSummary),
				Rollout:            cloneRolloutSummary(deployment.Rollout),
				Conditions:         append([]domain.Condition(nil), deployment.Conditions...),
			})
		}
	}

	sort.Slice(applications, func(i, j int) bool {
		return applications[i].Name < applications[j].Name
	})
	return applications, nil
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
		deployments := make([]DeploymentOverview, 0, len(matched.Deployments))
		for _, deployment := range matched.Deployments {
			deployments = append(deployments, cloneDeploymentOverview(deployment))
		}
		return AccountOverviewDetail{
			Name:             matched.Name,
			AWSAccountID:     matched.AWSAccountID,
			AllowedRegions:   append([]string(nil), matched.AllowedRegions...),
			DeploymentCount:  matched.DeploymentCount,
			ProjectCount:     matched.ProjectCount,
			EnvironmentCount: matched.EnvironmentCount,
			SyncedCount:      matched.SyncedCount,
			ProgressingCount: matched.ProgressingCount,
			PausedCount:      matched.PausedCount,
			ErrorCount:       matched.ErrorCount,
			Deployments:      deployments,
		}, nil
	}
	return AccountOverviewDetail{}, fmt.Errorf("%w: %s", statusstore.ErrNotFound, awsAccountID)
}

// BuildProjectEnvironmentOverviewDetail returns one grouped project/environment overview under an exact AWS account ID.
func BuildProjectEnvironmentOverviewDetail(c manifest.Catalog, snapshots []statusstore.Snapshot, awsAccountID, appRef, environmentRef string) (ProjectEnvironmentOverviewDetail, error) {
	accountDetail, err := BuildAccountOverviewDetail(c, snapshots, awsAccountID)
	if err != nil {
		return ProjectEnvironmentOverviewDetail{}, err
	}

	detail := ProjectEnvironmentOverviewDetail{
		AccountName:    accountDetail.Name,
		AWSAccountID:   accountDetail.AWSAccountID,
		AllowedRegions: append([]string(nil), accountDetail.AllowedRegions...),
		AppRef:         appRef,
		EnvironmentRef: environmentRef,
		Deployments:    make([]DeploymentOverview, 0, len(accountDetail.Deployments)),
	}

	for _, environment := range c.Environments {
		if environment.Name == environmentRef {
			detail.Region = environment.Region
			break
		}
	}

	for _, deployment := range accountDetail.Deployments {
		if deployment.AppRef != appRef || deployment.EnvironmentRef != environmentRef {
			continue
		}
		if detail.Region == "" {
			detail.Region = deployment.Region
		}
		detail.Deployments = append(detail.Deployments, cloneDeploymentOverview(deployment))
		detail.DeploymentCount++
		switch deployment.Sync.State {
		case domain.SyncStateSynced:
			detail.SyncedCount++
		case domain.SyncStateReconciling, domain.SyncStateOutOfSync, domain.SyncStateDrifted:
			detail.ProgressingCount++
		case domain.SyncStatePaused:
			detail.PausedCount++
		case domain.SyncStateError:
			detail.ErrorCount++
		}
	}

	if len(detail.Deployments) == 0 {
		return ProjectEnvironmentOverviewDetail{}, fmt.Errorf("%w: %s/%s/%s", statusstore.ErrNotFound, awsAccountID, appRef, environmentRef)
	}

	sort.Slice(detail.Deployments, func(i, j int) bool {
		return detail.Deployments[i].Name < detail.Deployments[j].Name
	})

	return detail, nil
}

// BuildApplicationOverviewDetail makes the application-page detail contract explicit while preserving deployment detail semantics.
func BuildApplicationOverviewDetail(c manifest.Catalog, snapshots []statusstore.Snapshot, deploymentName string) (ApplicationOverviewDetail, error) {
	detail, err := BuildDeploymentOverviewDetail(c, snapshots, deploymentName)
	if err != nil {
		return ApplicationOverviewDetail{}, err
	}

	return ApplicationOverviewDetail{
		Name:               detail.Name,
		AppRef:             detail.AppRef,
		EnvironmentRef:     detail.EnvironmentRef,
		AccountName:        detail.AccountName,
		AWSAccountID:       detail.AWSAccountID,
		AllowedRegions:     append([]string(nil), detail.AllowedRegions...),
		Region:             detail.Region,
		TargetType:         detail.TargetType,
		Strategy:           detail.Strategy,
		ExecutionMode:      detail.ExecutionMode,
		DesiredArtifact:    detail.DesiredArtifact,
		UserData:           detail.UserData,
		RolloutConfig:      cloneDeploymentRolloutConfig(detail.RolloutConfig),
		DeployedArtifact:   detail.DeployedArtifact,
		ObservedGeneration: detail.ObservedGeneration,
		Sync:               detail.Sync,
		Health:             detail.Health,
		RuntimeSummary:     cloneRuntimeSummary(detail.RuntimeSummary),
		Rollout:            cloneRolloutSummary(detail.Rollout),
		Conditions:         append([]domain.Condition(nil), detail.Conditions...),
		History:            cloneStatusHistory(detail.History),
	}, nil
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
		UserData:           deployment.UserData,
		RolloutConfig:      cloneDeploymentRolloutConfig(deployment.RolloutConfig),
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

// BuildDeploymentOverviewEvents returns the controller-derived runtime history timeline for one deployment.
func BuildDeploymentOverviewEvents(c manifest.Catalog, snapshots []statusstore.Snapshot, deploymentName string) (DeploymentOverviewEvents, error) {
	detail, err := BuildDeploymentOverviewDetail(c, snapshots, deploymentName)
	if err != nil {
		return DeploymentOverviewEvents{}, err
	}

	events := detail.History
	if len(events) == 0 {
		events = []domain.StatusHistoryEntry{}
	}

	return DeploymentOverviewEvents{
		DeploymentName: detail.Name,
		Events:         events,
	}, nil
}

func deriveRuntimeSummary(status domain.DeploymentStatus) *domain.RuntimeSummary {
	if len(status.History) > 0 {
		latest := status.History[len(status.History)-1]
		summary := &domain.RuntimeSummary{
			Timestamp:          latest.Timestamp,
			Source:             latest.Source,
			Summary:            latest.Summary,
			Changes:            append([]string(nil), latest.Changes...),
			ChangeDetails:      append([]string(nil), latest.ChangeDetails...),
			DeployedArtifact:   latest.DeployedArtifact,
			Conditions:         append([]domain.Condition(nil), latest.Conditions...),
			Sync:               latest.Sync,
			Health:             latest.Health,
			DesiredGeneration:  latest.DesiredGeneration,
			ObservedGeneration: latest.ObservedGeneration,
			RolloutPhase:       latest.RolloutPhase,
			CurrentStep:        latest.CurrentStep,
			TargetType:         latest.TargetType,
			Strategy:           latest.Strategy,
			ExecutionMode:      latest.ExecutionMode,
			HistoryCount:       len(status.History),
		}
		if summary.DeployedArtifact == (domain.DeploymentArtifact{}) {
			summary.DeployedArtifact = status.DeployedArtifact
		}
		if len(summary.Conditions) == 0 && len(status.Conditions) > 0 {
			summary.Conditions = append([]domain.Condition(nil), status.Conditions...)
		}
		if status.Rollout != nil {
			if summary.TargetType == "" {
				summary.TargetType = status.Rollout.TargetType
			}
			if summary.Strategy == "" {
				summary.Strategy = status.Rollout.Strategy
			}
			if summary.ExecutionMode == "" {
				summary.ExecutionMode = status.Rollout.ExecutionMode
			}
		}
		return summary
	}
	if status.Sync == (domain.Sync{}) && status.Health == (domain.Health{}) && status.Rollout == nil && status.DeployedArtifact == (domain.DeploymentArtifact{}) && len(status.Conditions) == 0 {
		return nil
	}

	summary := &domain.RuntimeSummary{
		DeployedArtifact:   status.DeployedArtifact,
		Conditions:         append([]domain.Condition(nil), status.Conditions...),
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
		summary.TargetType = status.Rollout.TargetType
		summary.Strategy = status.Rollout.Strategy
		summary.ExecutionMode = status.Rollout.ExecutionMode
	}
	return summary
}

func cloneRuntimeSummary(summary *domain.RuntimeSummary) *domain.RuntimeSummary {
	if summary == nil {
		return nil
	}
	copy := *summary
	copy.Changes = append([]string(nil), summary.Changes...)
	copy.ChangeDetails = append([]string(nil), summary.ChangeDetails...)
	copy.Conditions = append([]domain.Condition(nil), summary.Conditions...)
	return &copy
}

func cloneDeploymentOverview(deployment DeploymentOverview) DeploymentOverview {
	copy := deployment
	copy.RuntimeSummary = cloneRuntimeSummary(deployment.RuntimeSummary)
	copy.Rollout = cloneRolloutSummary(deployment.Rollout)
	copy.Conditions = append([]domain.Condition(nil), deployment.Conditions...)
	return copy
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

func cloneDeploymentRolloutConfig(config domain.DeploymentRolloutConfig) domain.DeploymentRolloutConfig {
	copy := config
	copy.Stacks = append([]domain.DeploymentRolloutStack(nil), config.Stacks...)
	return copy
}

func cloneStatusHistory(history []domain.StatusHistoryEntry) []domain.StatusHistoryEntry {
	if len(history) == 0 {
		return nil
	}
	cloned := append([]domain.StatusHistoryEntry(nil), history...)
	for i := range cloned {
		cloned[i].Changes = append([]string(nil), history[i].Changes...)
		cloned[i].ChangeDetails = append([]string(nil), history[i].ChangeDetails...)
		cloned[i].Conditions = append([]domain.Condition(nil), history[i].Conditions...)
	}
	return cloned
}
