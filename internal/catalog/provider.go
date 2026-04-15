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
