package rbacpolicy

import (
	"fmt"
	"sort"
	"strings"

	"github.com/DevopsArtFactory/tenet/internal/requestactor"
)

const (
	VersionV1        = "v1"
	SourceRepository = "repository"
)

// Policy is the repository-backed RBAC grant document. It is intentionally
// additive groundwork: current route enforcement still uses trusted front-door
// headers until a later slice explicitly opts into policy-backed enforcement.
type Policy struct {
	Version string       `json:"version" yaml:"version"`
	Users   []Subject    `json:"users,omitempty" yaml:"users,omitempty"`
	Groups  []GroupGrant `json:"groups,omitempty" yaml:"groups,omitempty"`
}

// Subject grants roles and deployment scopes directly to one authenticated user.
type Subject struct {
	ID           string   `json:"id,omitempty" yaml:"id,omitempty"`
	Email        string   `json:"email,omitempty" yaml:"email,omitempty"`
	Roles        []string `json:"roles,omitempty" yaml:"roles,omitempty"`
	Deployments  []string `json:"deployments,omitempty" yaml:"deployments,omitempty"`
	Projects     []string `json:"projects,omitempty" yaml:"projects,omitempty"`
	Environments []string `json:"environments,omitempty" yaml:"environments,omitempty"`
}

// GroupGrant grants roles and deployment scopes to one trusted authenticated group claim.
type GroupGrant struct {
	Name         string   `json:"name" yaml:"name"`
	Roles        []string `json:"roles,omitempty" yaml:"roles,omitempty"`
	Deployments  []string `json:"deployments,omitempty" yaml:"deployments,omitempty"`
	Projects     []string `json:"projects,omitempty" yaml:"projects,omitempty"`
	Environments []string `json:"environments,omitempty" yaml:"environments,omitempty"`
}

// Evaluation is the operator-facing read model produced by evaluating one
// authenticated request against a repository policy.
type Evaluation struct {
	Configured               bool                                            `json:"configured"`
	Available                bool                                            `json:"available"`
	Source                   string                                          `json:"source,omitempty"`
	Version                  string                                          `json:"version,omitempty"`
	MatchedUsers             []MatchedSubject                                `json:"matchedUsers,omitempty"`
	MatchedGroups            []MatchedSubject                                `json:"matchedGroups,omitempty"`
	Roles                    requestactor.AuthenticatedRoles                 `json:"roles"`
	DeploymentAccess         requestactor.AuthorizedDeployments              `json:"deploymentAccess"`
	ProjectEnvironmentScopes requestactor.AuthorizedProjectEnvironmentScopes `json:"projectEnvironmentScopes"`
	CanRead                  bool                                            `json:"canRead"`
	CanWrite                 bool                                            `json:"canWrite"`
	Summary                  string                                          `json:"summary"`
	UnavailableReason        string                                          `json:"unavailableReason,omitempty"`
}

// MatchedSubject names a policy subject that contributed grants.
type MatchedSubject struct {
	Kind  string `json:"kind"`
	Name  string `json:"name"`
	Match string `json:"match"`
}

func Evaluate(policy Policy, identity requestactor.AuthenticatedIdentity, groups requestactor.AuthenticatedGroups) (Evaluation, error) {
	policy = Normalize(policy)
	if policy.Version == "" {
		return Evaluation{Configured: false, Available: false, Summary: "no repository RBAC policy is configured", UnavailableReason: "repository RBAC policy file was not found"}, nil
	}
	if err := Validate(policy); err != nil {
		return Evaluation{Configured: true, Available: false, Source: SourceRepository, Version: policy.Version, Summary: "repository RBAC policy is invalid", UnavailableReason: err.Error()}, err
	}
	identity = requestactor.NormalizeAuthenticatedIdentity(identity)
	groups = requestactor.NormalizeAuthenticatedGroups(groups)
	result := Evaluation{Configured: true, Available: true, Source: SourceRepository, Version: policy.Version}
	if !identity.Authenticated {
		result.Summary = "repository RBAC policy is loaded but no authenticated identity was available to evaluate"
		result.UnavailableReason = identity.Reason
		return result, nil
	}
	roles := []string{}
	deployments := []string{}
	projects := []string{}
	environments := []string{}
	for _, user := range policy.Users {
		if user.ID != "" && identity.ID != "" && user.ID == identity.ID {
			result.MatchedUsers = append(result.MatchedUsers, MatchedSubject{Kind: "user", Name: subjectName(user), Match: "id"})
			roles = append(roles, user.Roles...)
			deployments = append(deployments, user.Deployments...)
			projects = append(projects, user.Projects...)
			environments = append(environments, user.Environments...)
			continue
		}
		if user.Email != "" && identity.Email != "" && strings.EqualFold(user.Email, identity.Email) {
			result.MatchedUsers = append(result.MatchedUsers, MatchedSubject{Kind: "user", Name: subjectName(user), Match: "email"})
			roles = append(roles, user.Roles...)
			deployments = append(deployments, user.Deployments...)
			projects = append(projects, user.Projects...)
			environments = append(environments, user.Environments...)
		}
	}
	groupSet := make(map[string]struct{}, len(groups.Names))
	for _, group := range groups.Names {
		groupSet[group] = struct{}{}
	}
	for _, group := range policy.Groups {
		if _, ok := groupSet[group.Name]; !ok {
			continue
		}
		result.MatchedGroups = append(result.MatchedGroups, MatchedSubject{Kind: "group", Name: group.Name, Match: "name"})
		roles = append(roles, group.Roles...)
		deployments = append(deployments, group.Deployments...)
		projects = append(projects, group.Projects...)
		environments = append(environments, group.Environments...)
	}
	result.Roles = requestactor.NormalizeAuthenticatedRoles(requestactor.AuthenticatedRoles{Names: roles})
	result.DeploymentAccess = requestactor.NormalizeAuthorizedDeployments(requestactor.AuthorizedDeployments{Names: deployments})
	result.ProjectEnvironmentScopes = requestactor.NormalizeAuthorizedProjectEnvironmentScopes(requestactor.AuthorizedProjectEnvironmentScopes{Projects: projects, Environments: environments})
	result.CanRead = result.Roles.AllowsOperatorRead()
	result.CanWrite = result.Roles.AllowsOperatorWrite()
	result.Summary = summary(result)
	return result, nil
}

func Validate(policy Policy) error {
	policy = Normalize(policy)
	if policy.Version == "" {
		return nil
	}
	if policy.Version != VersionV1 {
		return fmt.Errorf("unsupported RBAC policy version %q", policy.Version)
	}
	for i, user := range policy.Users {
		if user.ID == "" && user.Email == "" {
			return fmt.Errorf("users[%d] must set id or email", i)
		}
		if err := validateRoles(user.Roles, fmt.Sprintf("users[%d].roles", i)); err != nil {
			return err
		}
	}
	seenGroups := map[string]struct{}{}
	for i, group := range policy.Groups {
		if group.Name == "" {
			return fmt.Errorf("groups[%d].name is required", i)
		}
		if _, exists := seenGroups[group.Name]; exists {
			return fmt.Errorf("duplicate group %q", group.Name)
		}
		seenGroups[group.Name] = struct{}{}
		if err := validateRoles(group.Roles, fmt.Sprintf("groups[%d].roles", i)); err != nil {
			return err
		}
	}
	return nil
}

func Normalize(policy Policy) Policy {
	policy.Version = strings.TrimSpace(policy.Version)
	for i := range policy.Users {
		policy.Users[i].ID = strings.TrimSpace(policy.Users[i].ID)
		policy.Users[i].Email = strings.TrimSpace(policy.Users[i].Email)
		policy.Users[i].Roles = normalize(policy.Users[i].Roles)
		policy.Users[i].Deployments = normalize(policy.Users[i].Deployments)
		policy.Users[i].Projects = normalize(policy.Users[i].Projects)
		policy.Users[i].Environments = normalize(policy.Users[i].Environments)
	}
	for i := range policy.Groups {
		policy.Groups[i].Name = strings.TrimSpace(policy.Groups[i].Name)
		policy.Groups[i].Roles = normalize(policy.Groups[i].Roles)
		policy.Groups[i].Deployments = normalize(policy.Groups[i].Deployments)
		policy.Groups[i].Projects = normalize(policy.Groups[i].Projects)
		policy.Groups[i].Environments = normalize(policy.Groups[i].Environments)
	}
	return policy
}

func validateRoles(roles []string, path string) error {
	allowed := map[string]struct{}{requestactor.RoleViewer: {}, requestactor.RoleOperator: {}, requestactor.RoleAdmin: {}}
	for _, role := range roles {
		if _, ok := allowed[role]; !ok {
			return fmt.Errorf("%s contains unsupported role %q", path, role)
		}
	}
	return nil
}

func normalize(values []string) []string {
	seen := map[string]struct{}{}
	out := []string{}
	for _, raw := range values {
		value := strings.TrimSpace(raw)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	if len(out) == 0 {
		return nil
	}
	sort.Strings(out)
	return out
}

func subjectName(user Subject) string {
	if user.ID != "" {
		return user.ID
	}
	return user.Email
}

func summary(e Evaluation) string {
	matches := len(e.MatchedUsers) + len(e.MatchedGroups)
	if matches == 0 {
		return "repository RBAC policy matched no user or group grants for this authenticated identity"
	}
	return fmt.Sprintf("repository RBAC policy matched %d subject grant(s): roles=%d deployments=%d projects=%d environments=%d", matches, len(e.Roles.Names), len(e.DeploymentAccess.Names), len(e.ProjectEnvironmentScopes.Projects), len(e.ProjectEnvironmentScopes.Environments))
}
