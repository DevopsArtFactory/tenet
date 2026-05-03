package rbacpolicy

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/DevopsArtFactory/tenet/internal/manifestcodec"
)

var ErrNotConfigured = errors.New("repository RBAC policy is not configured")

var policyFileCandidates = []string{"policy.json", "policy.yaml", "policy.yml"}

// LoadFromRepository reads rbac/policy.{json,yaml,yml} from a desired-state repository.
// A missing policy file is not fatal; callers can report configured=false while preserving
// existing trusted-front-door behavior.
func LoadFromRepository(root string) (Policy, error) {
	for _, name := range policyFileCandidates {
		path := filepath.Join(root, "rbac", name)
		content, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return Policy{}, fmt.Errorf("read RBAC policy %s: %w", path, err)
		}
		var policy Policy
		if err := manifestcodec.Decode(name, content, &policy); err != nil {
			return Policy{}, fmt.Errorf("decode RBAC policy %s: %w", path, err)
		}
		policy = Normalize(policy)
		if err := Validate(policy); err != nil {
			return Policy{}, err
		}
		return policy, nil
	}
	return Policy{}, ErrNotConfigured
}
