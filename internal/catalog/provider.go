package catalog

import "github.com/DevopsArtFactory/tenet/internal/statusstore"

// RepositorySummaryProvider builds a summary function backed by a repository root.
func RepositorySummaryProvider(root string) func() (Summary, error) {
	return func() (Summary, error) {
		compiled, err := LoadFromRepository(root)
		if err != nil {
			return Summary{}, err
		}
		return Summarize(compiled), nil
	}
}

// RepositoryDeploymentProvider builds a deployment listing function backed by a repository root.
func RepositoryDeploymentProvider(root string) func() ([]DeploymentView, error) {
	return func() ([]DeploymentView, error) {
		compiled, err := LoadFromRepository(root)
		if err != nil {
			return nil, err
		}
		return ListDeployments(compiled), nil
	}
}

// RepositoryEnvironmentProvider builds an environment listing function backed by a repository root.
func RepositoryEnvironmentProvider(root string) func() ([]EnvironmentView, error) {
	return func() ([]EnvironmentView, error) {
		compiled, err := LoadFromRepository(root)
		if err != nil {
			return nil, err
		}
		return ListEnvironments(compiled), nil
	}
}

// RepositoryAccountProvider builds an account listing function backed by a repository root.
func RepositoryAccountProvider(root string) func() ([]AccountView, error) {
	return func() ([]AccountView, error) {
		compiled, err := LoadFromRepository(root)
		if err != nil {
			return nil, err
		}
		return ListAccounts(compiled), nil
	}
}

// RepositoryPlacementProvider builds a placement listing function backed by a repository root.
func RepositoryPlacementProvider(root string) func() ([]PlacementView, error) {
	return func() ([]PlacementView, error) {
		compiled, err := LoadFromRepository(root)
		if err != nil {
			return nil, err
		}
		return ListPlacements(compiled)
	}
}

// RepositoryTopologyProvider builds a topology snapshot function backed by a repository root.
func RepositoryTopologyProvider(root string) func() (TopologySnapshot, error) {
	return func() (TopologySnapshot, error) {
		compiled, err := LoadFromRepository(root)
		if err != nil {
			return TopologySnapshot{}, err
		}
		return BuildTopology(compiled)
	}
}

// RepositoryApplicationOverviewDetailProvider builds an application detail function backed by a repository root.
func RepositoryApplicationOverviewDetailProvider(root string, snapshots func() ([]statusstore.Snapshot, error)) func(string) (ApplicationOverviewDetail, error) {
	return func(deploymentName string) (ApplicationOverviewDetail, error) {
		compiled, err := LoadFromRepository(root)
		if err != nil {
			return ApplicationOverviewDetail{}, err
		}
		currentSnapshots, err := snapshots()
		if err != nil {
			return ApplicationOverviewDetail{}, err
		}
		return BuildApplicationOverviewDetail(compiled, currentSnapshots, deploymentName)
	}
}

// RepositoryAccountOverviewDetailProvider builds an account detail function backed by a repository root.
func RepositoryAccountOverviewDetailProvider(root string, snapshots func() ([]statusstore.Snapshot, error)) func(string) (AccountOverviewDetail, error) {
	return func(awsAccountID string) (AccountOverviewDetail, error) {
		compiled, err := LoadFromRepository(root)
		if err != nil {
			return AccountOverviewDetail{}, err
		}
		currentSnapshots, err := snapshots()
		if err != nil {
			return AccountOverviewDetail{}, err
		}
		return BuildAccountOverviewDetail(compiled, currentSnapshots, awsAccountID)
	}
}

// RepositoryProjectEnvironmentOverviewDetailProvider builds a grouped project/environment detail function backed by a repository root.
func RepositoryProjectEnvironmentOverviewDetailProvider(root string, snapshots func() ([]statusstore.Snapshot, error)) func(string, string, string) (ProjectEnvironmentOverviewDetail, error) {
	return func(awsAccountID, appRef, environmentRef string) (ProjectEnvironmentOverviewDetail, error) {
		compiled, err := LoadFromRepository(root)
		if err != nil {
			return ProjectEnvironmentOverviewDetail{}, err
		}
		currentSnapshots, err := snapshots()
		if err != nil {
			return ProjectEnvironmentOverviewDetail{}, err
		}
		return BuildProjectEnvironmentOverviewDetail(compiled, currentSnapshots, awsAccountID, appRef, environmentRef)
	}
}
