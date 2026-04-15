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
