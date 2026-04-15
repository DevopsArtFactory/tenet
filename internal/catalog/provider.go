package catalog

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
