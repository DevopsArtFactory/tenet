package domain

// Account represents a target AWS account Tenet can use for placement.
type Account struct {
	Name           string
	AWSAccountID   string
	AllowedRegions []string
}

// AllowsRegion reports whether the account can operate in the provided region.
func (a Account) AllowsRegion(region string) bool {
	if len(a.AllowedRegions) == 0 {
		return true
	}

	for _, allowed := range a.AllowedRegions {
		if allowed == region {
			return true
		}
	}

	return false
}
