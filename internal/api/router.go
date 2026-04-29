package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/DevopsArtFactory/tenet/internal/applyrequest"
	"github.com/DevopsArtFactory/tenet/internal/auditlog"
	"github.com/DevopsArtFactory/tenet/internal/buildinfo"
	"github.com/DevopsArtFactory/tenet/internal/catalog"
	"github.com/DevopsArtFactory/tenet/internal/controllerrefresh"
	"github.com/DevopsArtFactory/tenet/internal/githubwebhook"
	"github.com/DevopsArtFactory/tenet/internal/planpreview"
	"github.com/DevopsArtFactory/tenet/internal/requestactor"
	"github.com/DevopsArtFactory/tenet/internal/resourceinspection"
	"github.com/DevopsArtFactory/tenet/internal/resourcelogs"
	"github.com/DevopsArtFactory/tenet/internal/statusstore"
)

func requireMethod(w http.ResponseWriter, r *http.Request, method string) bool {
	if r.Method == method {
		return true
	}
	w.Header().Set("Allow", method)
	http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
	return false
}

func respondInternalError(w http.ResponseWriter) {
	http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
}

func respondUnavailableError(w http.ResponseWriter, message string) {
	http.Error(w, message, http.StatusServiceUnavailable)
}

func respondUnauthenticatedError(w http.ResponseWriter, message string) {
	http.Error(w, message, http.StatusUnauthorized)
}

func respondForbiddenError(w http.ResponseWriter, message string) {
	http.Error(w, message, http.StatusForbidden)
}

func isRequestContextError(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

func respondLookupError(w http.ResponseWriter, err error) {
	if errors.Is(err, statusstore.ErrNotFound) || errors.Is(err, planpreview.ErrNotFound) || errors.Is(err, applyrequest.ErrNotFound) {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	respondInternalError(w)
}

func respondReadProviderError(w http.ResponseWriter, err error, unavailableMessage string) {
	if isRequestContextError(err) {
		respondUnavailableError(w, unavailableMessage)
		return
	}
	respondInternalError(w)
}

func respondReadLookupError(w http.ResponseWriter, err error, unavailableMessage string) {
	if errors.Is(err, statusstore.ErrNotFound) || errors.Is(err, planpreview.ErrNotFound) || errors.Is(err, applyrequest.ErrNotFound) {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	if isRequestContextError(err) {
		respondUnavailableError(w, unavailableMessage)
		return
	}
	respondInternalError(w)
}

func respondApplyCreateError(w http.ResponseWriter, err error) {
	if errors.Is(err, applyrequest.ErrPlanNotReady) || errors.Is(err, applyrequest.ErrSourceFingerprintConflict) {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	respondLookupError(w, err)
}

func respondApplyTransitionError(w http.ResponseWriter, err error) {
	if errors.Is(err, applyrequest.ErrInvalidState) {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	respondLookupError(w, err)
}

func parseResourceQuery(w http.ResponseWriter, r *http.Request) (resourceType string, resourceName string, ok bool) {
	resourceType = strings.TrimSpace(r.URL.Query().Get("resourceType"))
	if resourceType == "" {
		http.Error(w, "resourceType is required", http.StatusBadRequest)
		return "", "", false
	}
	resourceName = strings.TrimSpace(r.URL.Query().Get("resourceName"))
	if resourceName == "" {
		http.Error(w, "resourceName is required", http.StatusBadRequest)
		return "", "", false
	}
	return resourceType, resourceName, true
}

func requireAuthenticatedIdentity(w http.ResponseWriter, r *http.Request) bool {
	identity := requestactor.AuthenticatedIdentityFromContext(r.Context())
	if identity.Authenticated {
		return true
	}
	respondUnauthenticatedError(w, "unauthenticated: authenticated identity is required for this operator write route")
	return false
}

func requireOperatorRoleForWrite(w http.ResponseWriter, r *http.Request, enabled bool) bool {
	if !enabled {
		return true
	}
	if !requireAuthenticatedIdentity(w, r) {
		return false
	}
	if requestactor.AuthenticatedRolesFromContext(r.Context()).AllowsOperatorWrite() {
		return true
	}
	respondForbiddenError(w, "forbidden: authenticated operator role is required for this operator write route")
	return false
}

func requireOperatorRoleForRead(w http.ResponseWriter, r *http.Request, enabled bool) bool {
	if !enabled {
		return true
	}
	if requestactor.AuthenticatedRolesFromContext(r.Context()).AllowsOperatorRead() {
		return true
	}
	respondForbiddenError(w, "forbidden: authenticated viewer-capable role is required for this operator read route")
	return false
}

func requireDeploymentReadIdentity(w http.ResponseWriter, r *http.Request, identityEnabled bool, roleEnabled bool) bool {
	if !identityEnabled {
		return true
	}
	identity := requestactor.AuthenticatedIdentityFromContext(r.Context())
	if !identity.Authenticated {
		respondUnauthenticatedError(w, "unauthenticated: authenticated identity is required for this operator read route")
		return false
	}
	return requireOperatorRoleForRead(w, r, roleEnabled)
}

func requireDeploymentReadAccess(w http.ResponseWriter, r *http.Request, enabled bool, deploymentName string) bool {
	if !enabled {
		return true
	}
	if requestactor.AuthorizedDeploymentsFromContext(r.Context()).Allows(deploymentName) {
		return true
	}
	respondForbiddenError(w, "forbidden: operator does not have access to this deployment")
	return false
}

func requireDeploymentReadAuthorization(w http.ResponseWriter, r *http.Request, identityEnabled bool, roleEnabled bool, deploymentName string) bool {
	if !requireDeploymentReadIdentity(w, r, identityEnabled, roleEnabled) {
		return false
	}
	return requireDeploymentReadAccess(w, r, identityEnabled, deploymentName)
}

func requireDeploymentWriteAuthorization(w http.ResponseWriter, r *http.Request, enabled bool, deploymentName string) bool {
	if !enabled {
		return true
	}
	if !requireAuthenticatedIdentity(w, r) {
		return false
	}
	return requireDeploymentReadAccess(w, r, enabled, deploymentName)
}

func filterAuthorizedApplicationOverviews(applications []catalog.ApplicationOverview, authorized requestactor.AuthorizedDeployments, enabled bool) []catalog.ApplicationOverview {
	if !enabled {
		return applications
	}
	filtered := make([]catalog.ApplicationOverview, 0, len(applications))
	for _, application := range applications {
		if authorized.Allows(application.Name) {
			filtered = append(filtered, application)
		}
	}
	return filtered
}

const operatorContextUnmatchedDeploymentLimit = 25

type operatorContextResponse struct {
	TrustedHeaders   operatorContextTrustedHeaders      `json:"trustedHeaders"`
	Identity         requestactor.AuthenticatedIdentity `json:"identity"`
	Roles            operatorContextRoles               `json:"roles"`
	DeploymentAccess operatorContextDeploymentAccess    `json:"deploymentAccess"`
	AccessVerdicts   operatorContextAccessVerdicts      `json:"accessVerdicts"`
}

type operatorContextTrustedHeaders struct {
	Enabled                             bool `json:"enabled"`
	AuthenticatedIdentityRequired       bool `json:"authenticatedIdentityRequired"`
	DeploymentReadAuthorizationEnforced bool `json:"deploymentReadAuthorizationEnforced"`
	OperatorReadRoleEnforced            bool `json:"operatorReadRoleEnforced"`
	OperatorWriteRoleEnforced           bool `json:"operatorWriteRoleEnforced"`
}

type operatorContextRoles struct {
	Names    []string `json:"names,omitempty"`
	CanRead  bool     `json:"canRead"`
	CanWrite bool     `json:"canWrite"`
}

type operatorContextDeploymentAccess struct {
	Enforced  bool                          `json:"enforced"`
	Forwarded bool                          `json:"forwarded"`
	Names     []string                      `json:"names,omitempty"`
	Summary   *operatorContextAccessSummary `json:"summary,omitempty"`
}

type operatorContextAccessSummary struct {
	Available         bool     `json:"available"`
	UnavailableReason string   `json:"unavailableReason,omitempty"`
	ForwardedCount    int      `json:"forwardedCount"`
	MatchedCount      int      `json:"matchedCount"`
	UnmatchedNames    []string `json:"unmatchedNames,omitempty"`
	AccountCount      int      `json:"accountCount,omitempty"`
	ProjectCount      int      `json:"projectCount,omitempty"`
	EnvironmentCount  int      `json:"environmentCount,omitempty"`
}

type operatorContextAccessVerdicts struct {
	OperatorRead          operatorContextAccessVerdict `json:"operatorRead"`
	OperatorWrite         operatorContextAccessVerdict `json:"operatorWrite"`
	DeploymentScopedRead  operatorContextAccessVerdict `json:"deploymentScopedRead"`
	DeploymentScopedWrite operatorContextAccessVerdict `json:"deploymentScopedWrite"`
	CollectionRead        operatorContextAccessVerdict `json:"collectionRead"`
}

type operatorContextAccessVerdict struct {
	Allowed                               bool   `json:"allowed"`
	Reason                                string `json:"reason"`
	RequiresAuthenticatedIdentity         bool   `json:"requiresAuthenticatedIdentity"`
	RequiresViewerCapableRole             bool   `json:"requiresViewerCapableRole,omitempty"`
	RequiresOperatorRole                  bool   `json:"requiresOperatorRole,omitempty"`
	RequiresExactDeploymentAllowlistMatch bool   `json:"requiresExactDeploymentAllowlistMatch,omitempty"`
	FiltersCollectionsByDeploymentAccess  bool   `json:"filtersCollectionsByDeploymentAccess,omitempty"`
	EmptyCollectionLikely                 bool   `json:"emptyCollectionLikely,omitempty"`
}

func buildOperatorContextResponse(r *http.Request, options Options) operatorContextResponse {
	identity := requestactor.AuthenticatedIdentityFromContext(r.Context())
	roles := requestactor.AuthenticatedRolesFromContext(r.Context())
	deployments := requestactor.AuthorizedDeploymentsFromContext(r.Context())
	summary := buildOperatorContextAccessSummary(r.Context(), options, deployments)
	return operatorContextResponse{
		TrustedHeaders: operatorContextTrustedHeaders{
			Enabled:                             options.TrustAuthenticatedHeaders,
			AuthenticatedIdentityRequired:       options.RequireAuthenticatedIdentity,
			DeploymentReadAuthorizationEnforced: options.RequireDeploymentReadAuthorization,
			OperatorReadRoleEnforced:            options.RequireOperatorRoleForRead,
			OperatorWriteRoleEnforced:           options.RequireOperatorRoleForWrite,
		},
		Identity: identity,
		Roles: operatorContextRoles{
			Names:    roles.Names,
			CanRead:  roles.AllowsOperatorRead(),
			CanWrite: roles.AllowsOperatorWrite(),
		},
		DeploymentAccess: operatorContextDeploymentAccess{
			Enforced:  options.RequireDeploymentReadAuthorization,
			Forwarded: len(deployments.Names) > 0,
			Names:     deployments.Names,
			Summary:   summary,
		},
		AccessVerdicts: buildOperatorContextAccessVerdicts(options, identity, roles, deployments, summary),
	}
}

func buildOperatorContextAccessVerdicts(options Options, identity requestactor.AuthenticatedIdentity, roles requestactor.AuthenticatedRoles, deployments requestactor.AuthorizedDeployments, summary *operatorContextAccessSummary) operatorContextAccessVerdicts {
	operatorRead := operatorContextReadVerdict(options, identity, roles)
	operatorWrite := operatorContextWriteVerdict(options, identity, roles)
	return operatorContextAccessVerdicts{
		OperatorRead:          operatorRead,
		OperatorWrite:         operatorWrite,
		DeploymentScopedRead:  operatorContextDeploymentScopedReadVerdict(options, identity, roles, deployments),
		DeploymentScopedWrite: operatorContextDeploymentScopedWriteVerdict(options, identity, roles, deployments, operatorWrite),
		CollectionRead:        operatorContextCollectionReadVerdict(options, identity, roles, deployments, summary),
	}
}

func operatorContextReadVerdict(options Options, identity requestactor.AuthenticatedIdentity, roles requestactor.AuthenticatedRoles) operatorContextAccessVerdict {
	verdict := operatorContextAccessVerdict{
		Allowed:                       true,
		Reason:                        "operator read routes are not enforcing trusted front-door identity for this API instance",
		RequiresAuthenticatedIdentity: options.RequireDeploymentReadAuthorization,
		RequiresViewerCapableRole:     options.RequireDeploymentReadAuthorization && options.RequireOperatorRoleForRead,
	}
	if !options.RequireDeploymentReadAuthorization {
		return verdict
	}
	if !identity.Authenticated {
		verdict.Allowed = false
		verdict.Reason = "authenticated identity is required for guarded operator read routes"
		return verdict
	}
	if options.RequireOperatorRoleForRead && !roles.AllowsOperatorRead() {
		verdict.Allowed = false
		verdict.Reason = "authenticated viewer-capable role is required for guarded operator read routes"
		return verdict
	}
	verdict.Reason = "guarded operator read prerequisites are satisfied for this request"
	return verdict
}

func operatorContextWriteVerdict(options Options, identity requestactor.AuthenticatedIdentity, roles requestactor.AuthenticatedRoles) operatorContextAccessVerdict {
	identityRequired := options.RequireAuthenticatedIdentity || options.RequireOperatorRoleForWrite || options.RequireDeploymentReadAuthorization
	verdict := operatorContextAccessVerdict{
		Allowed:                       true,
		Reason:                        "operator write routes are not enforcing trusted front-door identity for this API instance",
		RequiresAuthenticatedIdentity: identityRequired,
		RequiresOperatorRole:          options.RequireOperatorRoleForWrite,
	}
	if !identityRequired {
		return verdict
	}
	if !identity.Authenticated {
		verdict.Allowed = false
		verdict.Reason = "authenticated identity is required for guarded operator write routes"
		return verdict
	}
	if options.RequireOperatorRoleForWrite && !roles.AllowsOperatorWrite() {
		verdict.Allowed = false
		verdict.Reason = "authenticated operator role is required for guarded operator write routes"
		return verdict
	}
	verdict.Reason = "guarded operator write prerequisites are satisfied for this request"
	return verdict
}

func operatorContextDeploymentScopedReadVerdict(options Options, identity requestactor.AuthenticatedIdentity, roles requestactor.AuthenticatedRoles, deployments requestactor.AuthorizedDeployments) operatorContextAccessVerdict {
	verdict := operatorContextReadVerdict(options, identity, roles)
	verdict.RequiresExactDeploymentAllowlistMatch = options.RequireDeploymentReadAuthorization
	if !options.RequireDeploymentReadAuthorization {
		verdict.Allowed = true
		verdict.Reason = "deployment-scoped reads are not requiring exact deployment allowlist membership for this API instance"
		return verdict
	}
	if !verdict.Allowed {
		return verdict
	}
	if len(deployments.Names) == 0 {
		verdict.Allowed = false
		verdict.Reason = "deployment-scoped reads will return 403 unless X-Tenet-Authorized-Deployments includes the exact deployment name"
		return verdict
	}
	verdict.Reason = "deployment-scoped reads can pass only for deployments named in X-Tenet-Authorized-Deployments"
	return verdict
}

func operatorContextDeploymentScopedWriteVerdict(options Options, identity requestactor.AuthenticatedIdentity, roles requestactor.AuthenticatedRoles, deployments requestactor.AuthorizedDeployments, operatorWrite operatorContextAccessVerdict) operatorContextAccessVerdict {
	verdict := operatorWrite
	verdict.RequiresExactDeploymentAllowlistMatch = options.RequireDeploymentReadAuthorization
	if !verdict.Allowed {
		return verdict
	}
	if !options.RequireDeploymentReadAuthorization {
		verdict.Reason = "deployment-scoped writes are not requiring exact deployment allowlist membership for this API instance"
		return verdict
	}
	if len(deployments.Names) == 0 {
		verdict.Allowed = false
		verdict.Reason = "deployment-scoped writes will return 403 unless X-Tenet-Authorized-Deployments includes the exact deployment name"
		return verdict
	}
	verdict.Reason = "deployment-scoped writes can pass only for deployments named in X-Tenet-Authorized-Deployments"
	return verdict
}

func operatorContextCollectionReadVerdict(options Options, identity requestactor.AuthenticatedIdentity, roles requestactor.AuthenticatedRoles, deployments requestactor.AuthorizedDeployments, summary *operatorContextAccessSummary) operatorContextAccessVerdict {
	verdict := operatorContextReadVerdict(options, identity, roles)
	verdict.FiltersCollectionsByDeploymentAccess = options.RequireDeploymentReadAuthorization
	if !options.RequireDeploymentReadAuthorization {
		verdict.Allowed = true
		verdict.Reason = "collection reads are not filtered by deployment allowlist for this API instance"
		return verdict
	}
	if !verdict.Allowed {
		return verdict
	}
	verdict.Reason = "collection reads are filtered to deployments named in X-Tenet-Authorized-Deployments"
	if len(deployments.Names) == 0 {
		verdict.EmptyCollectionLikely = true
		verdict.Reason = "collection reads will be allowed but return empty collections until X-Tenet-Authorized-Deployments forwards at least one visible deployment"
		return verdict
	}
	if summary != nil && summary.Available && summary.MatchedCount == 0 {
		verdict.EmptyCollectionLikely = true
		verdict.Reason = "collection reads will be allowed but likely empty because the forwarded deployment allowlist does not match catalog deployments"
	}
	return verdict
}

func buildOperatorContextAccessSummary(ctx context.Context, options Options, deployments requestactor.AuthorizedDeployments) *operatorContextAccessSummary {
	if !options.RequireDeploymentReadAuthorization || len(deployments.Names) == 0 {
		return nil
	}
	summary := &operatorContextAccessSummary{ForwardedCount: len(deployments.Names)}
	if options.CatalogPlacements == nil {
		summary.UnavailableReason = "catalog placement provider is unavailable"
		return summary
	}
	placements, err := options.CatalogPlacements(ctx)
	if err != nil {
		summary.UnavailableReason = "catalog placement lookup failed"
		return summary
	}

	forwarded := make(map[string]struct{}, len(deployments.Names))
	for _, name := range deployments.Names {
		forwarded[name] = struct{}{}
	}
	matched := make(map[string]catalog.PlacementView)
	for _, placement := range placements {
		if _, ok := forwarded[placement.DeploymentName]; ok {
			matched[placement.DeploymentName] = placement
		}
	}
	unmatched := make([]string, 0, len(deployments.Names))
	for _, name := range deployments.Names {
		if _, ok := matched[name]; !ok {
			unmatched = append(unmatched, name)
		}
	}

	accounts := map[string]struct{}{}
	projects := map[string]struct{}{}
	environments := map[string]struct{}{}
	for _, placement := range matched {
		if placement.AWSAccountID != "" {
			accounts[placement.AWSAccountID] = struct{}{}
		}
		if placement.AppRef != "" {
			projects[placement.AppRef] = struct{}{}
		}
		if placement.EnvironmentRef != "" {
			environments[placement.EnvironmentRef] = struct{}{}
		}
	}
	sort.Strings(unmatched)
	if len(unmatched) > operatorContextUnmatchedDeploymentLimit {
		unmatched = unmatched[:operatorContextUnmatchedDeploymentLimit]
	}
	summary.Available = true
	summary.MatchedCount = len(matched)
	summary.UnmatchedNames = unmatched
	summary.AccountCount = len(accounts)
	summary.ProjectCount = len(projects)
	summary.EnvironmentCount = len(environments)
	return summary
}

type deploymentOverviewCounts struct {
	deploymentCount  int
	projectCount     int
	environmentCount int
	syncedCount      int
	progressingCount int
	pausedCount      int
	errorCount       int
}

func filterAuthorizedDeploymentOverviews(deployments []catalog.DeploymentOverview, authorized requestactor.AuthorizedDeployments, enabled bool) []catalog.DeploymentOverview {
	if !enabled {
		return deployments
	}
	filtered := make([]catalog.DeploymentOverview, 0, len(deployments))
	for _, deployment := range deployments {
		if authorized.Allows(deployment.Name) {
			filtered = append(filtered, deployment)
		}
	}
	return filtered
}

func filterAuthorizedStatusSnapshots(snapshots []statusstore.Snapshot, authorized requestactor.AuthorizedDeployments, enabled bool) []statusstore.Snapshot {
	if !enabled {
		return snapshots
	}
	filtered := make([]statusstore.Snapshot, 0, len(snapshots))
	for _, snapshot := range snapshots {
		if authorized.Allows(snapshot.DeploymentName) {
			filtered = append(filtered, snapshot)
		}
	}
	return filtered
}

func filterAuthorizedAuditEntries(entries []auditlog.Entry, authorized requestactor.AuthorizedDeployments, enabled bool) []auditlog.Entry {
	if !enabled {
		return entries
	}
	filtered := make([]auditlog.Entry, 0, len(entries))
	for _, entry := range entries {
		if authorized.Allows(entry.DeploymentName) {
			filtered = append(filtered, entry)
		}
	}
	return filtered
}

func summarizeDeploymentOverviews(deployments []catalog.DeploymentOverview) deploymentOverviewCounts {
	counts := deploymentOverviewCounts{
		deploymentCount: len(deployments),
	}
	projectRefs := make(map[string]struct{}, len(deployments))
	environmentRefs := make(map[string]struct{}, len(deployments))
	for _, deployment := range deployments {
		if deployment.AppRef != "" {
			projectRefs[deployment.AppRef] = struct{}{}
		}
		if deployment.EnvironmentRef != "" {
			environmentRefs[deployment.EnvironmentRef] = struct{}{}
		}
		switch deployment.Sync.State {
		case "Synced":
			counts.syncedCount++
		case "Reconciling", "OutOfSync", "Drifted":
			counts.progressingCount++
		case "Paused":
			counts.pausedCount++
		case "Error":
			counts.errorCount++
		}
	}
	counts.projectCount = len(projectRefs)
	counts.environmentCount = len(environmentRefs)
	return counts
}

func filterAuthorizedAccountOverviews(accounts []catalog.AccountOverview, authorized requestactor.AuthorizedDeployments, enabled bool) []catalog.AccountOverview {
	if !enabled {
		return accounts
	}
	filtered := make([]catalog.AccountOverview, 0, len(accounts))
	for _, account := range accounts {
		visibleDeployments := filterAuthorizedDeploymentOverviews(account.Deployments, authorized, enabled)
		if len(visibleDeployments) == 0 {
			continue
		}
		counts := summarizeDeploymentOverviews(visibleDeployments)
		filtered = append(filtered, catalog.AccountOverview{
			Name:             account.Name,
			AWSAccountID:     account.AWSAccountID,
			AllowedRegions:   account.AllowedRegions,
			DeploymentCount:  counts.deploymentCount,
			ProjectCount:     counts.projectCount,
			EnvironmentCount: counts.environmentCount,
			SyncedCount:      counts.syncedCount,
			ProgressingCount: counts.progressingCount,
			PausedCount:      counts.pausedCount,
			ErrorCount:       counts.errorCount,
			Deployments:      visibleDeployments,
		})
	}
	return filtered
}

func filterAuthorizedAccountOverviewDetail(detail catalog.AccountOverviewDetail, authorized requestactor.AuthorizedDeployments, enabled bool) catalog.AccountOverviewDetail {
	if !enabled {
		return detail
	}
	visibleDeployments := filterAuthorizedDeploymentOverviews(detail.Deployments, authorized, enabled)
	counts := summarizeDeploymentOverviews(visibleDeployments)
	return catalog.AccountOverviewDetail{
		Name:             detail.Name,
		AWSAccountID:     detail.AWSAccountID,
		AllowedRegions:   detail.AllowedRegions,
		DeploymentCount:  counts.deploymentCount,
		ProjectCount:     counts.projectCount,
		EnvironmentCount: counts.environmentCount,
		SyncedCount:      counts.syncedCount,
		ProgressingCount: counts.progressingCount,
		PausedCount:      counts.pausedCount,
		ErrorCount:       counts.errorCount,
		Deployments:      visibleDeployments,
	}
}

func filterAuthorizedProjectEnvironmentOverviewDetail(detail catalog.ProjectEnvironmentOverviewDetail, authorized requestactor.AuthorizedDeployments, enabled bool) catalog.ProjectEnvironmentOverviewDetail {
	if !enabled {
		return detail
	}
	visibleDeployments := filterAuthorizedDeploymentOverviews(detail.Deployments, authorized, enabled)
	counts := summarizeDeploymentOverviews(visibleDeployments)
	return catalog.ProjectEnvironmentOverviewDetail{
		AccountName:      detail.AccountName,
		AWSAccountID:     detail.AWSAccountID,
		AllowedRegions:   detail.AllowedRegions,
		AppRef:           detail.AppRef,
		EnvironmentRef:   detail.EnvironmentRef,
		Region:           detail.Region,
		DeploymentCount:  counts.deploymentCount,
		SyncedCount:      counts.syncedCount,
		ProgressingCount: counts.progressingCount,
		PausedCount:      counts.pausedCount,
		ErrorCount:       counts.errorCount,
		Deployments:      visibleDeployments,
	}
}

// Options configures the API router.
type Options struct {
	BuildInfo                            buildinfo.Info
	Logger                               *slog.Logger
	RequestTimeout                       time.Duration
	GitHubWebhookSecret                  string
	TrustAuthenticatedHeaders            bool
	RequireAuthenticatedIdentity         bool
	RequireOperatorRoleForRead           bool
	RequireOperatorRoleForWrite          bool
	RequireDeploymentReadAuthorization   bool
	GitHubWebhookDesiredStateSource      githubwebhook.DesiredStateSourceConfig
	GitHubWebhookRefreshPublisher        controllerrefresh.Publisher
	CatalogSummary                       func(context.Context) (catalog.Summary, error)
	CatalogDeployments                   func(context.Context) ([]catalog.DeploymentView, error)
	CatalogEnvironments                  func(context.Context) ([]catalog.EnvironmentView, error)
	CatalogAccounts                      func(context.Context) ([]catalog.AccountView, error)
	CatalogPlacements                    func(context.Context) ([]catalog.PlacementView, error)
	CatalogTopology                      func(context.Context) (catalog.TopologySnapshot, error)
	OverviewApplications                 func(context.Context) ([]catalog.ApplicationOverview, error)
	OverviewApplication                  func(context.Context, string) (catalog.ApplicationOverviewDetail, error)
	OverviewAccounts                     func(context.Context) ([]catalog.AccountOverview, error)
	OverviewAccount                      func(context.Context, string) (catalog.AccountOverviewDetail, error)
	OverviewProjectEnvironment           func(context.Context, string, string, string) (catalog.ProjectEnvironmentOverviewDetail, error)
	OverviewDeployment                   func(context.Context, string) (catalog.DeploymentOverviewDetail, error)
	OverviewDeploymentEvents             func(context.Context, string) (catalog.DeploymentOverviewEvents, error)
	OverviewDeploymentResourceLogs       func(context.Context, string, string, string) (resourcelogs.Result, error)
	OverviewDeploymentResourceInspection func(context.Context, string, string, string) (resourceinspection.Result, error)
	AuditLogs                            func(context.Context, auditlog.Query) (auditlog.Result, error)
	CreateDeploymentPlanPreview          func(context.Context, string, planpreview.CreateRequest) (planpreview.CreateResult, error)
	PlanPreview                          func(context.Context, string) (planpreview.Record, error)
	CreateApplyRequest                   func(context.Context, string) (applyrequest.CreateResult, error)
	ApplyRequest                         func(context.Context, string) (applyrequest.Record, error)
	TransitionApplyRequest               func(context.Context, string, applyrequest.TransitionRequest) (applyrequest.Record, error)
	DeploymentStatuses                   func(context.Context) ([]statusstore.Snapshot, error)
	DeploymentStatus                     func(context.Context, string) (statusstore.Snapshot, error)
}

// NewRouter builds the API router for the Tenet control plane.
func NewRouter(options Options) http.Handler {
	logger := options.Logger
	if logger == nil {
		logger = slog.Default()
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		serveUI(w)
	})
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/v1/info", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(options.BuildInfo)
	})
	mux.HandleFunc("/v1/operator/context", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(buildOperatorContextResponse(r, options))
	})
	mux.HandleFunc("/api/v1/github/webhook", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodPost) {
			return
		}
		if strings.TrimSpace(options.GitHubWebhookSecret) == "" {
			http.Error(w, "github webhook unavailable", http.StatusServiceUnavailable)
			return
		}

		response, err := githubwebhook.Decode(r.Context(), githubwebhook.Request{
			Secret:             options.GitHubWebhookSecret,
			Header:             r.Header,
			Body:               r.Body,
			DesiredStateSource: options.GitHubWebhookDesiredStateSource,
		})
		if err != nil {
			var validationErr *githubwebhook.ValidationError
			switch {
			case errors.As(err, &validationErr):
				http.Error(w, validationErr.Error(), http.StatusBadRequest)
			case isRequestContextError(err):
				respondUnavailableError(w, "github webhook unavailable")
			default:
				respondInternalError(w)
			}
			return
		}
		response = finalizeGitHubWebhookResponse(r.Context(), logger, response, options.GitHubWebhookRefreshPublisher)

		setRequestResult(r.Context(), response.Status, "")
		w.Header().Set("Content-Type", "application/json")
		if response.Status == githubwebhook.StatusAccepted {
			w.WriteHeader(http.StatusAccepted)
		}
		_ = json.NewEncoder(w).Encode(response)
	})
	mux.HandleFunc("/v1/catalog/summary", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		if options.CatalogSummary == nil {
			http.Error(w, "catalog summary unavailable", http.StatusServiceUnavailable)
			return
		}
		summary, err := options.CatalogSummary(r.Context())
		if err != nil {
			respondReadProviderError(w, err, "catalog summary unavailable")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(summary)
	})
	mux.HandleFunc("/v1/catalog/deployments", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		if options.CatalogDeployments == nil {
			http.Error(w, "catalog deployments unavailable", http.StatusServiceUnavailable)
			return
		}
		deployments, err := options.CatalogDeployments(r.Context())
		if err != nil {
			respondReadProviderError(w, err, "catalog deployments unavailable")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(deployments)
	})
	mux.HandleFunc("/v1/catalog/environments", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		if options.CatalogEnvironments == nil {
			http.Error(w, "catalog environments unavailable", http.StatusServiceUnavailable)
			return
		}
		environments, err := options.CatalogEnvironments(r.Context())
		if err != nil {
			respondReadProviderError(w, err, "catalog environments unavailable")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(environments)
	})
	mux.HandleFunc("/v1/catalog/accounts", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		if options.CatalogAccounts == nil {
			http.Error(w, "catalog accounts unavailable", http.StatusServiceUnavailable)
			return
		}
		accounts, err := options.CatalogAccounts(r.Context())
		if err != nil {
			respondReadProviderError(w, err, "catalog accounts unavailable")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(accounts)
	})
	mux.HandleFunc("/v1/catalog/placements", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		if options.CatalogPlacements == nil {
			http.Error(w, "catalog placements unavailable", http.StatusServiceUnavailable)
			return
		}
		placements, err := options.CatalogPlacements(r.Context())
		if err != nil {
			respondReadProviderError(w, err, "catalog placements unavailable")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(placements)
	})
	mux.HandleFunc("/v1/catalog/topology", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		if options.CatalogTopology == nil {
			http.Error(w, "catalog topology unavailable", http.StatusServiceUnavailable)
			return
		}
		topology, err := options.CatalogTopology(r.Context())
		if err != nil {
			respondReadProviderError(w, err, "catalog topology unavailable")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(topology)
	})
	mux.HandleFunc("/v1/overview/applications", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		if !requireDeploymentReadIdentity(w, r, options.RequireDeploymentReadAuthorization, options.RequireOperatorRoleForRead) {
			return
		}
		if options.OverviewApplications == nil {
			http.Error(w, "overview applications unavailable", http.StatusServiceUnavailable)
			return
		}
		applications, err := options.OverviewApplications(r.Context())
		if err != nil {
			respondReadProviderError(w, err, "overview applications unavailable")
			return
		}
		applications = filterAuthorizedApplicationOverviews(
			applications,
			requestactor.AuthorizedDeploymentsFromContext(r.Context()),
			options.RequireDeploymentReadAuthorization,
		)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(applications)
	})
	mux.HandleFunc("/v1/overview/applications/", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		if options.OverviewApplication == nil {
			http.Error(w, "overview application unavailable", http.StatusServiceUnavailable)
			return
		}
		name := strings.TrimPrefix(r.URL.Path, "/v1/overview/applications/")
		if name == "" || strings.Contains(name, "/") {
			http.Error(w, "invalid deployment name", http.StatusBadRequest)
			return
		}
		if !requireDeploymentReadAuthorization(w, r, options.RequireDeploymentReadAuthorization, options.RequireOperatorRoleForRead, name) {
			return
		}
		detail, err := options.OverviewApplication(r.Context(), name)
		if err != nil {
			respondReadLookupError(w, err, "overview application unavailable")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(detail)
	})
	mux.HandleFunc("/v1/overview/accounts", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		if !requireDeploymentReadIdentity(w, r, options.RequireDeploymentReadAuthorization, options.RequireOperatorRoleForRead) {
			return
		}
		if options.OverviewAccounts == nil {
			http.Error(w, "overview accounts unavailable", http.StatusServiceUnavailable)
			return
		}
		accounts, err := options.OverviewAccounts(r.Context())
		if err != nil {
			respondReadProviderError(w, err, "overview accounts unavailable")
			return
		}
		accounts = filterAuthorizedAccountOverviews(
			accounts,
			requestactor.AuthorizedDeploymentsFromContext(r.Context()),
			options.RequireDeploymentReadAuthorization,
		)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(accounts)
	})
	mux.HandleFunc("/v1/overview/accounts/", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		path := strings.TrimPrefix(r.URL.Path, "/v1/overview/accounts/")
		segments := strings.Split(path, "/")
		switch {
		case len(segments) == 1 && segments[0] != "":
			if options.OverviewAccount == nil {
				http.Error(w, "overview account unavailable", http.StatusServiceUnavailable)
				return
			}
			if !requireDeploymentReadIdentity(w, r, options.RequireDeploymentReadAuthorization, options.RequireOperatorRoleForRead) {
				return
			}
			detail, err := options.OverviewAccount(r.Context(), segments[0])
			if err != nil {
				respondReadLookupError(w, err, "overview account unavailable")
				return
			}
			detail = filterAuthorizedAccountOverviewDetail(
				detail,
				requestactor.AuthorizedDeploymentsFromContext(r.Context()),
				options.RequireDeploymentReadAuthorization,
			)
			if options.RequireDeploymentReadAuthorization && len(detail.Deployments) == 0 {
				respondForbiddenError(w, "forbidden: operator does not have access to this deployment")
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(detail)
		case len(segments) == 5 && segments[0] != "" && segments[1] == "projects" && segments[2] != "" && segments[3] == "environments" && segments[4] != "":
			if options.OverviewProjectEnvironment == nil {
				http.Error(w, "overview project environment unavailable", http.StatusServiceUnavailable)
				return
			}
			if !requireDeploymentReadIdentity(w, r, options.RequireDeploymentReadAuthorization, options.RequireOperatorRoleForRead) {
				return
			}
			detail, err := options.OverviewProjectEnvironment(r.Context(), segments[0], segments[2], segments[4])
			if err != nil {
				respondReadLookupError(w, err, "overview project environment unavailable")
				return
			}
			detail = filterAuthorizedProjectEnvironmentOverviewDetail(
				detail,
				requestactor.AuthorizedDeploymentsFromContext(r.Context()),
				options.RequireDeploymentReadAuthorization,
			)
			if options.RequireDeploymentReadAuthorization && len(detail.Deployments) == 0 {
				respondForbiddenError(w, "forbidden: operator does not have access to this deployment")
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(detail)
		default:
			http.Error(w, "invalid overview account path", http.StatusBadRequest)
		}
	})
	mux.HandleFunc("/v1/overview/deployments/", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		path := strings.TrimPrefix(r.URL.Path, "/v1/overview/deployments/")
		switch {
		case strings.HasSuffix(path, "/events"):
			if options.OverviewDeploymentEvents == nil {
				http.Error(w, "overview deployment events unavailable", http.StatusServiceUnavailable)
				return
			}
			name := strings.TrimSuffix(path, "/events")
			if name == "" || strings.Contains(name, "/") {
				http.Error(w, "invalid deployment name", http.StatusBadRequest)
				return
			}
			if !requireDeploymentReadAuthorization(w, r, options.RequireDeploymentReadAuthorization, options.RequireOperatorRoleForRead, name) {
				return
			}
			events, err := options.OverviewDeploymentEvents(r.Context(), name)
			if err != nil {
				respondReadLookupError(w, err, "overview deployment events unavailable")
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(events)
		case strings.HasSuffix(path, "/resource-logs"):
			if options.OverviewDeployment == nil {
				http.Error(w, "overview deployment unavailable", http.StatusServiceUnavailable)
				return
			}
			if options.OverviewDeploymentResourceLogs == nil {
				http.Error(w, "overview deployment resource logs unavailable", http.StatusServiceUnavailable)
				return
			}
			name := strings.TrimSuffix(path, "/resource-logs")
			if name == "" || strings.Contains(name, "/") {
				http.Error(w, "invalid deployment name", http.StatusBadRequest)
				return
			}
			if !requireDeploymentReadAuthorization(w, r, options.RequireDeploymentReadAuthorization, options.RequireOperatorRoleForRead, name) {
				return
			}
			resourceType, resourceName, ok := parseResourceQuery(w, r)
			if !ok {
				return
			}
			if _, err := options.OverviewDeployment(r.Context(), name); err != nil {
				respondReadLookupError(w, err, "overview deployment unavailable")
				return
			}
			result, err := options.OverviewDeploymentResourceLogs(r.Context(), name, resourceType, resourceName)
			if err != nil {
				respondReadLookupError(w, err, "overview deployment resource logs unavailable")
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(result)
		case strings.HasSuffix(path, "/resource-inspection"):
			if options.OverviewDeployment == nil {
				http.Error(w, "overview deployment unavailable", http.StatusServiceUnavailable)
				return
			}
			if options.OverviewDeploymentResourceInspection == nil {
				http.Error(w, "overview deployment resource inspection unavailable", http.StatusServiceUnavailable)
				return
			}
			name := strings.TrimSuffix(path, "/resource-inspection")
			if name == "" || strings.Contains(name, "/") {
				http.Error(w, "invalid deployment name", http.StatusBadRequest)
				return
			}
			if !requireDeploymentReadAuthorization(w, r, options.RequireDeploymentReadAuthorization, options.RequireOperatorRoleForRead, name) {
				return
			}
			resourceType, resourceName, ok := parseResourceQuery(w, r)
			if !ok {
				return
			}
			if _, err := options.OverviewDeployment(r.Context(), name); err != nil {
				respondReadLookupError(w, err, "overview deployment unavailable")
				return
			}
			result, err := options.OverviewDeploymentResourceInspection(r.Context(), name, resourceType, resourceName)
			if err != nil {
				respondReadLookupError(w, err, "overview deployment resource inspection unavailable")
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(result)
		default:
			if options.OverviewDeployment == nil {
				http.Error(w, "overview deployment unavailable", http.StatusServiceUnavailable)
				return
			}
			name := path
			if name == "" || strings.Contains(name, "/") {
				http.Error(w, "invalid deployment name", http.StatusBadRequest)
				return
			}
			if !requireDeploymentReadAuthorization(w, r, options.RequireDeploymentReadAuthorization, options.RequireOperatorRoleForRead, name) {
				return
			}
			detail, err := options.OverviewDeployment(r.Context(), name)
			if err != nil {
				respondReadLookupError(w, err, "overview deployment unavailable")
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(detail)
		}
	})
	mux.HandleFunc("/v1/audit-logs", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		if options.AuditLogs == nil {
			http.Error(w, "audit logs unavailable", http.StatusServiceUnavailable)
			return
		}
		query := auditlog.Query{}
		if _, ok := r.URL.Query()["deployment"]; ok {
			deploymentName := strings.TrimSpace(r.URL.Query().Get("deployment"))
			if deploymentName == "" || strings.Contains(deploymentName, "/") {
				http.Error(w, "invalid deployment query", http.StatusBadRequest)
				return
			}
			if !requireDeploymentReadAuthorization(w, r, options.RequireDeploymentReadAuthorization, options.RequireOperatorRoleForRead, deploymentName) {
				return
			}
			query.DeploymentName = deploymentName
		}
		if _, ok := r.URL.Query()["action"]; ok {
			action := strings.TrimSpace(r.URL.Query().Get("action"))
			if !auditlog.ValidAction(action) {
				http.Error(w, "invalid action query", http.StatusBadRequest)
				return
			}
			query.Action = auditlog.Action(action)
		}
		if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
			limit, err := strconv.Atoi(raw)
			if err != nil || limit < 1 {
				http.Error(w, "invalid limit query", http.StatusBadRequest)
				return
			}
			query.Limit = limit
		}
		if query.DeploymentName == "" && !requireDeploymentReadIdentity(w, r, options.RequireDeploymentReadAuthorization, options.RequireOperatorRoleForRead) {
			return
		}
		result, err := options.AuditLogs(r.Context(), query)
		if err != nil {
			respondInternalError(w)
			return
		}
		if query.DeploymentName == "" {
			result.Entries = filterAuthorizedAuditEntries(
				result.Entries,
				requestactor.AuthorizedDeploymentsFromContext(r.Context()),
				options.RequireDeploymentReadAuthorization,
			)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(result)
	})
	mux.HandleFunc("/v1/plans/deployments/", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodPost) {
			return
		}
		if options.RequireAuthenticatedIdentity && !requireAuthenticatedIdentity(w, r) {
			return
		}
		if !requireOperatorRoleForWrite(w, r, options.RequireOperatorRoleForWrite) {
			return
		}
		if options.CreateDeploymentPlanPreview == nil {
			http.Error(w, "deployment plan preview unavailable", http.StatusServiceUnavailable)
			return
		}
		name := strings.TrimPrefix(r.URL.Path, "/v1/plans/deployments/")
		if name == "" || strings.Contains(name, "/") {
			http.Error(w, "invalid deployment name", http.StatusBadRequest)
			return
		}
		if !requireDeploymentWriteAuthorization(w, r, options.RequireDeploymentReadAuthorization, name) {
			return
		}
		var req planpreview.CreateRequest
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&req); err != nil && !errors.Is(err, io.EOF) {
			http.Error(w, "invalid plan preview request body", http.StatusBadRequest)
			return
		}
		if err := decoder.Decode(&struct{}{}); err != nil && !errors.Is(err, io.EOF) {
			http.Error(w, "invalid plan preview request body", http.StatusBadRequest)
			return
		}
		result, err := options.CreateDeploymentPlanPreview(r.Context(), name, req)
		if err != nil {
			respondLookupError(w, err)
			return
		}
		w.Header().Set("Location", "/v1/plans/"+result.Record.ID)
		w.Header().Set("Content-Type", "application/json")
		if result.Created {
			w.WriteHeader(http.StatusCreated)
		}
		_ = json.NewEncoder(w).Encode(result.Record)
	})
	mux.HandleFunc("/v1/plans/", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		if options.PlanPreview == nil {
			http.Error(w, "plan preview unavailable", http.StatusServiceUnavailable)
			return
		}
		id := strings.TrimPrefix(r.URL.Path, "/v1/plans/")
		if id == "" || strings.Contains(id, "/") {
			http.Error(w, "invalid plan id", http.StatusBadRequest)
			return
		}
		if !requireDeploymentReadIdentity(w, r, options.RequireDeploymentReadAuthorization, options.RequireOperatorRoleForRead) {
			return
		}
		record, err := options.PlanPreview(r.Context(), id)
		if err != nil {
			respondLookupError(w, err)
			return
		}
		if !requireDeploymentReadAccess(w, r, options.RequireDeploymentReadAuthorization, record.DeploymentName) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(record)
	})
	mux.HandleFunc("/v1/applies/plans/", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodPost) {
			return
		}
		if options.RequireAuthenticatedIdentity && !requireAuthenticatedIdentity(w, r) {
			return
		}
		if !requireOperatorRoleForWrite(w, r, options.RequireOperatorRoleForWrite) {
			return
		}
		if options.CreateApplyRequest == nil {
			http.Error(w, "apply request unavailable", http.StatusServiceUnavailable)
			return
		}
		planID := strings.TrimPrefix(r.URL.Path, "/v1/applies/plans/")
		if planID == "" || strings.Contains(planID, "/") {
			http.Error(w, "invalid plan id", http.StatusBadRequest)
			return
		}
		if options.RequireDeploymentReadAuthorization {
			if options.PlanPreview == nil {
				http.Error(w, "plan preview unavailable", http.StatusServiceUnavailable)
				return
			}
			planRecord, err := options.PlanPreview(r.Context(), planID)
			if err != nil {
				respondLookupError(w, err)
				return
			}
			if !requireDeploymentWriteAuthorization(w, r, true, planRecord.DeploymentName) {
				return
			}
		}
		result, err := options.CreateApplyRequest(r.Context(), planID)
		if err != nil {
			respondApplyCreateError(w, err)
			return
		}
		w.Header().Set("Location", "/v1/applies/"+result.Record.ID)
		w.Header().Set("Content-Type", "application/json")
		if result.Created {
			w.WriteHeader(http.StatusCreated)
		}
		_ = json.NewEncoder(w).Encode(result.Record)
	})
	mux.HandleFunc("/v1/applies/", func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/v1/applies/")
		segments := strings.Split(path, "/")
		switch {
		case len(segments) == 1 && segments[0] != "":
			if !requireMethod(w, r, http.MethodGet) {
				return
			}
			if options.ApplyRequest == nil {
				http.Error(w, "apply request unavailable", http.StatusServiceUnavailable)
				return
			}
			if !requireDeploymentReadIdentity(w, r, options.RequireDeploymentReadAuthorization, options.RequireOperatorRoleForRead) {
				return
			}
			record, err := options.ApplyRequest(r.Context(), segments[0])
			if err != nil {
				respondLookupError(w, err)
				return
			}
			if !requireDeploymentReadAccess(w, r, options.RequireDeploymentReadAuthorization, record.DeploymentName) {
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(record)
		case len(segments) == 2 && segments[0] != "" && segments[1] == "transitions":
			if !requireMethod(w, r, http.MethodPost) {
				return
			}
			if (options.RequireAuthenticatedIdentity || options.RequireDeploymentReadAuthorization) && !requireAuthenticatedIdentity(w, r) {
				return
			}
			if !requireOperatorRoleForWrite(w, r, options.RequireOperatorRoleForWrite) {
				return
			}
			if options.RequireDeploymentReadAuthorization {
				if options.ApplyRequest == nil {
					http.Error(w, "apply request unavailable", http.StatusServiceUnavailable)
					return
				}
				record, err := options.ApplyRequest(r.Context(), segments[0])
				if err != nil {
					respondLookupError(w, err)
					return
				}
				if !requireDeploymentReadAccess(w, r, true, record.DeploymentName) {
					return
				}
			}
			if options.TransitionApplyRequest == nil {
				http.Error(w, "apply request transition unavailable", http.StatusServiceUnavailable)
				return
			}
			var req applyrequest.TransitionRequest
			decoder := json.NewDecoder(r.Body)
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(&req); err != nil {
				http.Error(w, "invalid apply transition request body", http.StatusBadRequest)
				return
			}
			if err := decoder.Decode(&struct{}{}); err != nil && !errors.Is(err, io.EOF) {
				http.Error(w, "invalid apply transition request body", http.StatusBadRequest)
				return
			}
			record, err := options.TransitionApplyRequest(r.Context(), segments[0], req)
			if err != nil {
				respondApplyTransitionError(w, err)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(record)
		default:
			http.Error(w, "invalid apply id", http.StatusBadRequest)
		}
	})
	mux.HandleFunc("/v1/status/deployments", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		if !requireDeploymentReadIdentity(w, r, options.RequireDeploymentReadAuthorization, options.RequireOperatorRoleForRead) {
			return
		}
		if options.DeploymentStatuses == nil {
			http.Error(w, "deployment statuses unavailable", http.StatusServiceUnavailable)
			return
		}
		snapshots, err := options.DeploymentStatuses(r.Context())
		if err != nil {
			respondReadProviderError(w, err, "deployment statuses unavailable")
			return
		}
		snapshots = filterAuthorizedStatusSnapshots(snapshots, requestactor.AuthorizedDeploymentsFromContext(r.Context()), options.RequireDeploymentReadAuthorization)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(snapshots)
	})
	mux.HandleFunc("/v1/status/deployments/", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		if options.DeploymentStatus == nil {
			http.Error(w, "deployment status unavailable", http.StatusServiceUnavailable)
			return
		}
		name := strings.TrimPrefix(r.URL.Path, "/v1/status/deployments/")
		if name == "" || strings.Contains(name, "/") {
			http.Error(w, "invalid deployment name", http.StatusBadRequest)
			return
		}
		if !requireDeploymentReadAuthorization(w, r, options.RequireDeploymentReadAuthorization, options.RequireOperatorRoleForRead, name) {
			return
		}
		snapshot, err := options.DeploymentStatus(r.Context(), name)
		if err != nil {
			respondReadLookupError(w, err, "deployment status unavailable")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(snapshot)
	})

	handler := http.Handler(mux)
	handler = panicRecoveryMiddleware(logger, handler)
	handler = requestTimeoutMiddleware(options.RequestTimeout, handler)
	handler = requestLoggingMiddleware(logger, handler)
	handler = requestActorMiddleware(options.TrustAuthenticatedHeaders || options.RequireAuthenticatedIdentity || options.RequireOperatorRoleForRead || options.RequireDeploymentReadAuthorization, handler)
	handler = requestIDMiddleware(handler)
	return handler
}
