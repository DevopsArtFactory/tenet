package catalog

import (
	"sort"

	"github.com/DevopsArtFactory/tenet/internal/manifest"
)

// AccountView is a UI-friendly account listing entry.
type AccountView struct {
	Name           string   `json:"name"`
	AWSAccountID   string   `json:"awsAccountId"`
	AllowedRegions []string `json:"allowedRegions,omitempty"`
}

// ListAccounts projects a compiled catalog into a stable account listing.
func ListAccounts(c manifest.Catalog) []AccountView {
	accounts := make([]AccountView, 0, len(c.Accounts))
	for _, account := range c.Accounts {
		accounts = append(accounts, AccountView{
			Name:           account.Name,
			AWSAccountID:   account.AWSAccountID,
			AllowedRegions: append([]string(nil), account.AllowedRegions...),
		})
	}
	sort.Slice(accounts, func(i, j int) bool {
		return accounts[i].Name < accounts[j].Name
	})
	return accounts
}
