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
	"github.com/DevopsArtFactory/tenet/internal/authidentity"
	"github.com/DevopsArtFactory/tenet/internal/authprovider"
	"github.com/DevopsArtFactory/tenet/internal/authstate"
	"github.com/DevopsArtFactory/tenet/internal/buildinfo"
	"github.com/DevopsArtFactory/tenet/internal/catalog"
	"github.com/DevopsArtFactory/tenet/internal/controllerrefresh"
	"github.com/DevopsArtFactory/tenet/internal/domain"
	"github.com/DevopsArtFactory/tenet/internal/githubwebhook"
	"github.com/DevopsArtFactory/tenet/internal/planpreview"
	"github.com/DevopsArtFactory/tenet/internal/rbacpolicy"
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

func validEC2InstanceID(instanceID string) bool {
	instanceID = strings.TrimSpace(instanceID)
	if len(instanceID) < 3 || !strings.HasPrefix(instanceID, "i-") {
		return false
	}
	for _, r := range instanceID[2:] {
		if (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F') {
			continue
		}
		return false
	}
	return true
}

func validECSClusterARN(clusterARN string) bool {
	clusterARN = strings.TrimSpace(clusterARN)
	parts := strings.SplitN(clusterARN, ":", 6)
	if len(parts) != 6 {
		return false
	}
	if parts[0] != "arn" || parts[1] == "" || parts[2] != "ecs" || parts[3] == "" || parts[4] == "" {
		return false
	}
	return strings.HasPrefix(parts[5], "cluster/") && strings.TrimSpace(strings.TrimPrefix(parts[5], "cluster/")) != ""
}

func parseOptionalECSClusterARN(w http.ResponseWriter, r *http.Request) (string, bool) {
	clusterARN := strings.TrimSpace(r.URL.Query().Get("clusterArn"))
	if clusterARN == "" {
		return "", true
	}
	if !validECSClusterARN(clusterARN) {
		http.Error(w, "invalid clusterArn query", http.StatusBadRequest)
		return "", false
	}
	return clusterARN, true
}

func parseOptionalPositiveLimit(w http.ResponseWriter, r *http.Request) (limit int, ok bool) {
	raw := strings.TrimSpace(r.URL.Query().Get("limit"))
	if raw == "" {
		return 0, true
	}
	limit, err := strconv.Atoi(raw)
	if err != nil || limit < 1 {
		http.Error(w, "invalid limit query", http.StatusBadRequest)
		return 0, false
	}
	return limit, true
}

func applyAuditLogLimit(result auditlog.Result, query auditlog.Query) auditlog.Result {
	totalEntryCount := len(result.Entries)
	if query.Limit > 0 && query.Limit < len(result.Entries) {
		result.Entries = append([]auditlog.Entry(nil), result.Entries[:query.Limit]...)
	}
	result.ResultScope = auditlog.BuildResultScope(query, totalEntryCount, len(result.Entries))
	return result
}

func applyDeploymentEventsLimit(result catalog.DeploymentOverviewEvents, limit int) catalog.DeploymentOverviewEvents {
	if limit <= 0 {
		return result
	}

	effectiveLimit := limit
	requested := limit
	truncationStatus := resourcelogs.ScopeTruncationStatusNotTruncated
	if len(result.Events) > limit {
		result.Events = append([]domain.StatusHistoryEntry(nil), result.Events[:limit]...)
		result.ManagedDriftSummary = summarizeDeploymentEventsManagedDrift(result.Events)
		truncationStatus = resourcelogs.ScopeTruncationStatusTruncated
	} else if len(result.Events) == limit {
		truncationStatus = resourcelogs.ScopeTruncationStatusUnknown
	}
	result.ResultScope = &catalog.DeploymentEventsResultScope{
		RequestedLimit:     &requested,
		EffectiveLimit:     effectiveLimit,
		ReturnedEventCount: len(result.Events),
		TruncationStatus:   truncationStatus,
	}
	return result
}

func summarizeDeploymentEventsManagedDrift(events []domain.StatusHistoryEntry) catalog.ManagedDriftSummary {
	summary := catalog.ManagedDriftSummary{}
	for _, event := range events {
		for _, item := range event.ManagedDrift {
			summary.Total++
			switch item.Severity {
			case domain.ManagedDriftSeverityHigh:
				summary.High++
			case domain.ManagedDriftSeverityMedium:
				summary.Medium++
			case domain.ManagedDriftSeverityLow:
				summary.Low++
			}
		}
	}
	return summary
}

func applyResourceLogLimit(result resourcelogs.Result, limit int) resourcelogs.Result {
	effectiveLimit := resourcelogs.DefaultMaxEntries
	var requestedLimit *int
	if limit > 0 {
		effectiveLimit = limit
		requested := limit
		requestedLimit = &requested
	}

	truncationStatus := resourcelogs.ScopeTruncationStatusNotApplicable
	if result.Available {
		truncationStatus = resourcelogs.ScopeTruncationStatusNotTruncated
		if effectiveLimit > 0 && len(result.Entries) > effectiveLimit {
			result.Entries = append([]resourcelogs.Entry(nil), result.Entries[:effectiveLimit]...)
			truncationStatus = resourcelogs.ScopeTruncationStatusTruncated
		} else if effectiveLimit > 0 && len(result.Entries) == effectiveLimit {
			truncationStatus = resourcelogs.ScopeTruncationStatusUnknown
		}
	}

	result.ResultScope = &resourcelogs.ResultScope{
		RequestedLimit:     requestedLimit,
		EffectiveLimit:     effectiveLimit,
		ReturnedEntryCount: len(result.Entries),
		TruncationStatus:   truncationStatus,
	}
	return result
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
	if requestHasDeploymentAccess(r.Context(), Options{}, deploymentName, requestactor.AuthorizedDeploymentsFromContext(r.Context()), requestactor.AuthorizedProjectEnvironmentScopesFromContext(r.Context()), nil) {
		return true
	}
	respondForbiddenError(w, "forbidden: operator does not have access to this deployment")
	return false
}

func requireDeploymentReadAccessWithOptions(w http.ResponseWriter, r *http.Request, options Options, deploymentName string) bool {
	if !options.RequireDeploymentReadAuthorization {
		return true
	}
	if requestHasDeploymentAccess(r.Context(), options, deploymentName, requestactor.AuthorizedDeploymentsFromContext(r.Context()), requestactor.AuthorizedProjectEnvironmentScopesFromContext(r.Context()), nil) {
		return true
	}
	respondForbiddenError(w, "forbidden: operator does not have access to this deployment")
	return false
}

func requireDeploymentReadAuthorization(w http.ResponseWriter, r *http.Request, options Options, deploymentName string) bool {
	if !requireDeploymentReadIdentity(w, r, options.RequireDeploymentReadAuthorization, options.RequireOperatorRoleForRead) {
		return false
	}
	return requireDeploymentReadAccessWithOptions(w, r, options, deploymentName)
}

func requireDeploymentWriteAuthorization(w http.ResponseWriter, r *http.Request, options Options, deploymentName string) bool {
	if !options.RequireDeploymentReadAuthorization {
		return true
	}
	if !requireAuthenticatedIdentity(w, r) {
		return false
	}
	return requireDeploymentReadAccessWithOptions(w, r, options, deploymentName)
}

func requestHasDeploymentAccess(ctx context.Context, options Options, deploymentName string, deployments requestactor.AuthorizedDeployments, scopes requestactor.AuthorizedProjectEnvironmentScopes, placements []catalog.PlacementView) bool {
	if deployments.Allows(deploymentName) {
		return true
	}
	if len(scopes.Projects) == 0 || len(scopes.Environments) == 0 {
		return false
	}
	if placements == nil && options.CatalogPlacements != nil {
		var err error
		placements, err = options.CatalogPlacements(ctx)
		if err != nil {
			return false
		}
	}
	if len(placements) == 0 {
		return false
	}
	projects := stringSet(scopes.Projects)
	environments := stringSet(scopes.Environments)
	for _, placement := range placements {
		if placement.DeploymentName == deploymentName && placement.AppRef != "" && placement.EnvironmentRef != "" {
			_, projectOK := projects[placement.AppRef]
			_, environmentOK := environments[placement.EnvironmentRef]
			return projectOK && environmentOK
		}
	}
	return false
}

func authorizedDeploymentNameSet(ctx context.Context, options Options, deployments requestactor.AuthorizedDeployments, scopes requestactor.AuthorizedProjectEnvironmentScopes) map[string]struct{} {
	visible := make(map[string]struct{}, len(deployments.Names))
	for _, name := range deployments.Names {
		visible[name] = struct{}{}
	}
	if len(scopes.Projects) == 0 || len(scopes.Environments) == 0 || options.CatalogPlacements == nil {
		return visible
	}
	placements, err := options.CatalogPlacements(ctx)
	if err != nil {
		return visible
	}
	projects := stringSet(scopes.Projects)
	environments := stringSet(scopes.Environments)
	for _, placement := range placements {
		if placement.DeploymentName == "" || placement.AppRef == "" || placement.EnvironmentRef == "" {
			continue
		}
		if _, ok := projects[placement.AppRef]; !ok {
			continue
		}
		if _, ok := environments[placement.EnvironmentRef]; !ok {
			continue
		}
		visible[placement.DeploymentName] = struct{}{}
	}
	return visible
}

func stringSet(values []string) map[string]struct{} {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		set[value] = struct{}{}
	}
	return set
}

func filterAuthorizedApplicationOverviews(applications []catalog.ApplicationOverview, visible map[string]struct{}, enabled bool) []catalog.ApplicationOverview {
	if !enabled {
		return applications
	}
	filtered := make([]catalog.ApplicationOverview, 0, len(applications))
	for _, application := range applications {
		if _, ok := visible[application.Name]; ok {
			filtered = append(filtered, application)
		}
	}
	return filtered
}

const (
	operatorContextUnmatchedDeploymentLimit                 = 25
	operatorContextProjectEnvironmentExampleLimit           = 10
	operatorContextVisibleDeploymentExampleLimit            = 10
	operatorContextProjectEnvironmentExampleDeploymentLimit = 5
)

var (
	operatorContextOperatorReadExampleRoutes = []string{
		"GET /v1/overview/applications",
		"GET /v1/status/deployments",
		"GET /v1/catalog/deployments",
		"GET /v1/audit-logs",
	}
	operatorContextOperatorWriteExampleRoutes = []string{
		"POST /v1/plans/deployments/{deploymentName}",
		"POST /v1/applies/plans/{planId}",
		"POST /v1/applies/{applyId}/transitions",
	}
	operatorContextDeploymentScopedReadExampleRoutes = []string{
		"GET /v1/overview/deployments/{deploymentName}",
		"GET /v1/status/deployments/{deploymentName}",
		"GET /v1/audit-logs?deployment={deploymentName}",
	}
	operatorContextDeploymentScopedWriteExampleRoutes = []string{
		"POST /v1/plans/deployments/{deploymentName}",
		"POST /v1/applies/plans/{planId}",
		"POST /v1/applies/{applyId}/transitions",
	}
	operatorContextCollectionReadExampleRoutes = []string{
		"GET /v1/overview/applications",
		"GET /v1/status/deployments",
		"GET /v1/catalog/deployments",
		"GET /v1/overview/accounts",
		"GET /v1/audit-logs",
	}
	operatorContextCatalogReadExampleRoutes = []string{
		"GET /v1/catalog/deployments",
		"GET /v1/catalog/accounts",
		"GET /v1/catalog/placements",
		"GET /v1/catalog/topology",
		"GET /v1/catalog/summary",
	}
	operatorContextAuthenticatedIdentityHeaders = []string{
		"X-Tenet-Authenticated-Id",
		"X-User-ID",
	}
	operatorContextViewerRoleHeaders = []string{
		"X-Tenet-Authenticated-Roles",
	}
	operatorContextOperatorRoleHeaders = []string{
		"X-Tenet-Authenticated-Roles",
	}
	operatorContextDeploymentAllowlistHeaders = []string{
		"X-Tenet-Authorized-Deployments",
	}
	operatorContextProjectEnvironmentScopeHeaders = []string{
		"X-Tenet-Authorized-Projects",
		"X-Tenet-Authorized-Environments",
	}
)

type operatorContextResponse struct {
	TrustedHeaders        operatorContextTrustedHeaders                `json:"trustedHeaders"`
	AuthRuntime           AuthRuntimeDiagnostic                        `json:"authRuntime"`
	Enforcement           operatorContextEnforcement                   `json:"enforcement"`
	Identity              requestactor.AuthenticatedIdentity           `json:"identity"`
	IdentityHeaderSupport operatorContextIdentityHeaderSupport         `json:"identityHeaderSupport"`
	IdentityProvenance    requestactor.AuthenticatedIdentityProvenance `json:"identityProvenance"`
	Actor                 requestactor.Actor                           `json:"actor"`
	ActorHeaderSupport    operatorContextIdentityHeaderSupport         `json:"actorHeaderSupport"`
	ActorProvenance       requestactor.ActorProvenance                 `json:"actorProvenance"`
	ActorAttribution      requestactor.ActorAttribution                `json:"actorAttribution"`
	Roles                 operatorContextRoles                         `json:"roles"`
	DeploymentAccess      operatorContextDeploymentAccess              `json:"deploymentAccess"`
	AccessVerdicts        operatorContextAccessVerdicts                `json:"accessVerdicts"`
	RepositoryPolicy      operatorContextRepositoryPolicy              `json:"repositoryPolicy"`
	DesiredStateSource    operatorContextDesiredStateSource            `json:"desiredStateSource"`
}

// AuthRuntimeDiagnostic describes optional Tenet-managed authentication runtime capability.
// It intentionally reports only non-secret configuration/capability state for operator diagnostics.
type AuthRuntimeDiagnostic struct {
	TenetManagedLogin TenetManagedLoginDiagnostic `json:"tenetManagedLogin"`
	EnterpriseSSO     EnterpriseSSODiagnostic     `json:"enterpriseSso"`
}

// EnterpriseSSODiagnostic reports provider-neutral enterprise SSO configuration
// state without exposing client secrets, raw tokens, SAML assertions, or private
// provider metadata. Active remains false until a provider's login/callback
// runtime is implemented and wired for this API process.
type EnterpriseSSODiagnostic struct {
	Configured              bool     `json:"configured"`
	Active                  bool     `json:"active"`
	Provider                string   `json:"provider,omitempty"`
	RequiredSettings        []string `json:"requiredSettings,omitempty"`
	MissingSettings         []string `json:"missingSettings,omitempty"`
	InvalidSettings         []string `json:"invalidSettings,omitempty"`
	LoginRoute              string   `json:"loginRoute,omitempty"`
	CallbackRoute           string   `json:"callbackRoute,omitempty"`
	LogoutRoute             string   `json:"logoutRoute,omitempty"`
	StateStoreConfigured    bool     `json:"stateStoreConfigured"`
	IdentityStoreConfigured bool     `json:"identityStoreConfigured"`
	SessionSecretConfigured bool     `json:"sessionSecretConfigured"`
	UnavailableReason       string   `json:"unavailableReason,omitempty"`
}

// TenetManagedLoginDiagnostic reports whether opt-in Tenet-managed browser login is configured.
// Active is true only when the current API process has complete non-secret runtime
// settings, provider/session wiring, and no missing settings for the login routes.
type TenetManagedLoginDiagnostic struct {
	Configured                       bool     `json:"configured"`
	Active                           bool     `json:"active"`
	Provider                         string   `json:"provider"`
	RequiredSettings                 []string `json:"requiredSettings"`
	MissingSettings                  []string `json:"missingSettings"`
	LoginRoute                       string   `json:"loginRoute,omitempty"`
	CallbackRoute                    string   `json:"callbackRoute,omitempty"`
	LogoutRoute                      string   `json:"logoutRoute,omitempty"`
	SessionCookieName                string   `json:"sessionCookieName,omitempty"`
	SessionCookiePath                string   `json:"sessionCookiePath"`
	SessionCookieHTTPOnly            bool     `json:"sessionCookieHttpOnly"`
	SessionCookieSameSite            string   `json:"sessionCookieSameSite"`
	SessionCookieSecureOverride      bool     `json:"sessionCookieSecureOverride"`
	SessionCookieSecureForRequest    bool     `json:"sessionCookieSecureForRequest"`
	SessionCookieSecureSources       []string `json:"sessionCookieSecureSources,omitempty"`
	OAuthStateTTLSeconds             int64    `json:"oauthStateTtlSeconds"`
	SessionTTLSeconds                int64    `json:"sessionTtlSeconds"`
	SessionExpiryEnforced            bool     `json:"sessionExpiryEnforced"`
	ProviderAccessTokenStored        bool     `json:"providerAccessTokenStored"`
	ProviderAccessTokenReturned      bool     `json:"providerAccessTokenReturned"`
	ProviderTokenRefreshSupported    bool     `json:"providerTokenRefreshSupported"`
	ProviderTokenRevocationSupported bool     `json:"providerTokenRevocationSupported"`
	LogoutClearsSessionCookie        bool     `json:"logoutClearsSessionCookie"`
	LogoutRevokesProviderToken       bool     `json:"logoutRevokesProviderToken"`
	StateStoreConfigured             bool     `json:"stateStoreConfigured"`
	IdentityStoreConfigured          bool     `json:"identityStoreConfigured"`
	SessionSecretConfigured          bool     `json:"sessionSecretConfigured"`
	UnavailableReason                string   `json:"unavailableReason,omitempty"`
}

type operatorContextDesiredStateSource struct {
	Configured                   bool                             `json:"configured"`
	Kind                         string                           `json:"kind"`
	Filesystem                   *operatorContextFilesystemSource `json:"filesystem,omitempty"`
	GitHub                       *operatorContextGitHubSource     `json:"github,omitempty"`
	ControllerRefresh            operatorContextControllerRefresh `json:"controllerRefresh"`
	WebhookMatchStatuses         []string                         `json:"webhookMatchStatuses"`
	WebhookProcessedFalseReasons []string                         `json:"webhookProcessedFalseReasons"`
}

type operatorContextFilesystemSource struct {
	Mode string `json:"mode"`
}

type operatorContextGitHubSource struct {
	Repository string `json:"repository,omitempty"`
	Ref        string `json:"ref,omitempty"`
	Path       string `json:"path,omitempty"`
	BaseURL    string `json:"baseUrl,omitempty"`
}

type operatorContextControllerRefresh struct {
	Configured     bool   `json:"configured"`
	Available      bool   `json:"available"`
	Backend        string `json:"backend,omitempty"`
	Channel        string `json:"channel,omitempty"`
	Status         string `json:"status"`
	Reason         string `json:"reason"`
	DiagnosticOnly bool   `json:"diagnosticOnly"`
}

type operatorContextTrustedHeaders struct {
	Enabled                             bool `json:"enabled"`
	AuthenticatedIdentityRequired       bool `json:"authenticatedIdentityRequired"`
	DeploymentReadAuthorizationEnforced bool `json:"deploymentReadAuthorizationEnforced"`
	OperatorReadRoleEnforced            bool `json:"operatorReadRoleEnforced"`
	OperatorWriteRoleEnforced           bool `json:"operatorWriteRoleEnforced"`
}

type operatorContextIdentityHeaderSupport struct {
	Canonical []string `json:"canonical"`
	Aliases   []string `json:"aliases"`
}

type operatorContextEnforcement struct {
	Mode                                   string `json:"mode"`
	Summary                                string `json:"summary"`
	TrustedHeadersEnabled                  bool   `json:"trustedHeadersEnabled"`
	AuthenticatedIdentityRequired          bool   `json:"authenticatedIdentityRequired"`
	DeploymentReadAuthorizationEnforced    bool   `json:"deploymentReadAuthorizationEnforced"`
	OperatorReadRoleEnforced               bool   `json:"operatorReadRoleEnforced"`
	OperatorWriteRoleEnforced              bool   `json:"operatorWriteRoleEnforced"`
	ReadRoutesGuarded                      bool   `json:"readRoutesGuarded"`
	WriteRoutesGuarded                     bool   `json:"writeRoutesGuarded"`
	DeploymentScopedRoutesRequireAllowlist bool   `json:"deploymentScopedRoutesRequireAllowlist"`
	CollectionReadsFiltered                bool   `json:"collectionReadsFiltered"`
}

type operatorContextRoles struct {
	Names                []string                                  `json:"names,omitempty"`
	CanRead              bool                                      `json:"canRead"`
	CanWrite             bool                                      `json:"canWrite"`
	SupportedNames       []string                                  `json:"supportedNames"`
	SupportedHeaderNames []string                                  `json:"supportedHeaderNames"`
	RecognizedNames      []string                                  `json:"recognizedNames"`
	UnrecognizedNames    []string                                  `json:"unrecognizedNames"`
	Provenance           requestactor.AuthenticatedRolesProvenance `json:"provenance"`
}

type operatorContextDeploymentAccess struct {
	Enforced             bool                          `json:"enforced"`
	Forwarded            bool                          `json:"forwarded"`
	SupportedHeaderNames []string                      `json:"supportedHeaderNames"`
	Names                []string                      `json:"names,omitempty"`
	Scopes               operatorContextAccessScopes   `json:"scopes"`
	Summary              *operatorContextAccessSummary `json:"summary,omitempty"`
}

type operatorContextAccessScopes struct {
	Forwarded    bool     `json:"forwarded"`
	Projects     []string `json:"projects,omitempty"`
	Environments []string `json:"environments,omitempty"`
}

type operatorContextAccessSummary struct {
	Available                      bool                                       `json:"available"`
	UnavailableReason              string                                     `json:"unavailableReason,omitempty"`
	EmptyReason                    string                                     `json:"emptyReason,omitempty"`
	ForwardedCount                 int                                        `json:"forwardedCount"`
	ForwardedProjectCount          int                                        `json:"forwardedProjectCount,omitempty"`
	ForwardedEnvironmentCount      int                                        `json:"forwardedEnvironmentCount,omitempty"`
	MatchedCount                   int                                        `json:"matchedCount"`
	ExactDeploymentMatchedCount    int                                        `json:"exactDeploymentMatchedCount,omitempty"`
	ProjectEnvironmentMatchedCount int                                        `json:"projectEnvironmentMatchedCount,omitempty"`
	UnmatchedCount                 int                                        `json:"unmatchedCount"`
	UnmatchedNames                 []string                                   `json:"unmatchedNames,omitempty"`
	UnmatchedNamesTruncated        bool                                       `json:"unmatchedNamesTruncated,omitempty"`
	UnmatchedProjectCount          int                                        `json:"unmatchedProjectCount,omitempty"`
	UnmatchedProjects              []string                                   `json:"unmatchedProjects,omitempty"`
	UnmatchedProjectsTruncated     bool                                       `json:"unmatchedProjectsTruncated,omitempty"`
	UnmatchedEnvironmentCount      int                                        `json:"unmatchedEnvironmentCount,omitempty"`
	UnmatchedEnvironments          []string                                   `json:"unmatchedEnvironments,omitempty"`
	UnmatchedEnvironmentsTruncated bool                                       `json:"unmatchedEnvironmentsTruncated,omitempty"`
	AccountCount                   int                                        `json:"accountCount,omitempty"`
	ProjectCount                   int                                        `json:"projectCount,omitempty"`
	EnvironmentCount               int                                        `json:"environmentCount,omitempty"`
	VisibleDeploymentExamples      []operatorContextVisibleDeploymentExample  `json:"visibleDeploymentExamples,omitempty"`
	ProjectEnvironmentExamples     []operatorContextProjectEnvironmentExample `json:"projectEnvironmentExamples,omitempty"`
}

type operatorContextVisibleDeploymentExample struct {
	DeploymentName string   `json:"deploymentName"`
	AppRef         string   `json:"appRef,omitempty"`
	EnvironmentRef string   `json:"environmentRef,omitempty"`
	Region         string   `json:"region,omitempty"`
	AWSAccountID   string   `json:"awsAccountId,omitempty"`
	MatchSources   []string `json:"matchSources"`
}

type operatorContextProjectEnvironmentExample struct {
	AppRef             string   `json:"appRef"`
	EnvironmentRef     string   `json:"environmentRef"`
	Region             string   `json:"region,omitempty"`
	AWSAccountID       string   `json:"awsAccountId,omitempty"`
	DeploymentCount    int      `json:"deploymentCount"`
	ExampleDeployments []string `json:"exampleDeployments,omitempty"`
}

type operatorContextAccessVerdicts struct {
	OperatorRead          operatorContextAccessVerdict `json:"operatorRead"`
	OperatorWrite         operatorContextAccessVerdict `json:"operatorWrite"`
	DeploymentScopedRead  operatorContextAccessVerdict `json:"deploymentScopedRead"`
	DeploymentScopedWrite operatorContextAccessVerdict `json:"deploymentScopedWrite"`
	CollectionRead        operatorContextAccessVerdict `json:"collectionRead"`
	CatalogRead           operatorContextAccessVerdict `json:"catalogRead"`
}

type operatorContextRepositoryPolicy struct {
	rbacpolicy.Evaluation
	SupportedGroupHeaderNames []string `json:"supportedGroupHeaderNames"`
	DiagnosticOnly            bool     `json:"diagnosticOnly"`
}

type operatorContextAccessVerdict struct {
	Allowed                               bool     `json:"allowed"`
	Reason                                string   `json:"reason"`
	ExampleRoutes                         []string `json:"exampleRoutes"`
	RequiredTrustedHeaders                []string `json:"requiredTrustedHeaders,omitempty"`
	AcceptedRoleNames                     []string `json:"acceptedRoleNames,omitempty"`
	RequiresAuthenticatedIdentity         bool     `json:"requiresAuthenticatedIdentity"`
	RequiresViewerCapableRole             bool     `json:"requiresViewerCapableRole,omitempty"`
	RequiresOperatorRole                  bool     `json:"requiresOperatorRole,omitempty"`
	RequiresExactDeploymentAllowlistMatch bool     `json:"requiresExactDeploymentAllowlistMatch,omitempty"`
	AcceptsProjectEnvironmentScopes       bool     `json:"acceptsProjectEnvironmentScopes,omitempty"`
	FiltersCollectionsByDeploymentAccess  bool     `json:"filtersCollectionsByDeploymentAccess,omitempty"`
	EmptyCollectionLikely                 bool     `json:"emptyCollectionLikely,omitempty"`
}

func buildOperatorContextResponse(r *http.Request, options Options) operatorContextResponse {
	identity := requestactor.AuthenticatedIdentityFromContext(r.Context())
	actor := requestactor.FromContext(r.Context())
	actorHeaders := requestactor.FromHeaders(r.Header)
	actorProvenance := requestactor.ActorProvenanceFromHeaders(r.Header)
	identityProvenance := requestactor.AuthenticatedIdentityProvenanceFromContext(r.Context())
	roles := requestactor.AuthenticatedRolesFromContext(r.Context())
	rolesProvenance := requestactor.NormalizeAuthenticatedRolesProvenance(requestactor.AuthenticatedRolesProvenance{})
	if options.TrustAuthenticatedHeaders {
		rolesProvenance = requestactor.AuthenticatedRolesProvenanceFromHeaders(r.Header)
	}
	deployments := requestactor.AuthorizedDeploymentsFromContext(r.Context())
	scopes := requestactor.AuthorizedProjectEnvironmentScopesFromContext(r.Context())
	summary := buildOperatorContextAccessSummary(r.Context(), options, deployments, scopes)
	repositoryPolicy := buildOperatorContextRepositoryPolicy(r.Context(), options, identity, requestactor.AuthenticatedGroupsFromContext(r.Context()))
	return operatorContextResponse{
		TrustedHeaders: operatorContextTrustedHeaders{
			Enabled:                             options.TrustAuthenticatedHeaders,
			AuthenticatedIdentityRequired:       options.RequireAuthenticatedIdentity,
			DeploymentReadAuthorizationEnforced: options.RequireDeploymentReadAuthorization,
			OperatorReadRoleEnforced:            options.RequireOperatorRoleForRead,
			OperatorWriteRoleEnforced:           options.RequireOperatorRoleForWrite,
		},
		AuthRuntime: buildAuthRuntimeDiagnosticForRequest(r, options),
		Enforcement: buildOperatorContextEnforcement(options),
		Identity:    identity,
		IdentityHeaderSupport: operatorContextIdentityHeaderSupport{
			Canonical: requestactor.SupportedAuthenticatedIdentityHeaderNames(),
			Aliases:   requestactor.SupportedAuthenticatedIdentityAliasHeaderNames(),
		},
		IdentityProvenance: identityProvenance,
		Actor:              actor,
		ActorHeaderSupport: operatorContextIdentityHeaderSupport{
			Canonical: requestactor.SupportedActorHeaderNames(),
			Aliases:   requestactor.SupportedActorAliasHeaderNames(),
		},
		ActorProvenance:  actorProvenance,
		ActorAttribution: requestactor.Attribution(actor, actorHeaders, identity),
		Roles: operatorContextRoles{
			Names:                roles.Names,
			CanRead:              roles.AllowsOperatorRead(),
			CanWrite:             roles.AllowsOperatorWrite(),
			SupportedNames:       requestactor.SupportedOperatorRoleNames(),
			SupportedHeaderNames: requestactor.SupportedAuthenticatedRoleHeaderNames(),
			RecognizedNames:      roles.RecognizedNames(),
			UnrecognizedNames:    roles.UnrecognizedNames(),
			Provenance:           rolesProvenance,
		},
		DeploymentAccess: operatorContextDeploymentAccess{
			Enforced:             options.RequireDeploymentReadAuthorization,
			Forwarded:            len(deployments.Names) > 0 || len(scopes.Projects) > 0 || len(scopes.Environments) > 0,
			SupportedHeaderNames: requestactor.SupportedDeploymentAccessHeaderNames(),
			Names:                deployments.Names,
			Scopes: operatorContextAccessScopes{
				Forwarded:    len(scopes.Projects) > 0 || len(scopes.Environments) > 0,
				Projects:     scopes.Projects,
				Environments: scopes.Environments,
			},
			Summary: summary,
		},
		AccessVerdicts:     buildOperatorContextAccessVerdicts(options, identity, roles, deployments, scopes, summary),
		RepositoryPolicy:   repositoryPolicy,
		DesiredStateSource: buildOperatorContextDesiredStateSource(options),
	}
}

func buildOperatorContextDesiredStateSource(options Options) operatorContextDesiredStateSource {
	source := options.GitHubWebhookDesiredStateSource
	kind := strings.TrimSpace(source.Kind)
	diagnostic := operatorContextDesiredStateSource{
		Configured: kind != githubwebhook.DesiredStateSourceKindNone,
		Kind:       kind,
		ControllerRefresh: operatorContextControllerRefresh{
			Configured:     false,
			Available:      false,
			Status:         "not_configured",
			Reason:         "controller refresh handoff is not configured for this API instance",
			DiagnosticOnly: true,
		},
		WebhookMatchStatuses: []string{
			githubwebhook.DesiredStateSourceMatchStatusMatched,
			githubwebhook.DesiredStateSourceMatchStatusRepositoryMismatch,
			githubwebhook.DesiredStateSourceMatchStatusRefMismatch,
			githubwebhook.DesiredStateSourceMatchStatusPathUntouched,
			githubwebhook.DesiredStateSourceMatchStatusFilesystemSource,
			githubwebhook.DesiredStateSourceMatchStatusNotConfigured,
		},
		WebhookProcessedFalseReasons: []string{
			"deleted_ref_ignored",
			githubwebhook.DesiredStateSourceMatchStatusRepositoryMismatch,
			githubwebhook.DesiredStateSourceMatchStatusRefMismatch,
			githubwebhook.DesiredStateSourceMatchStatusPathUntouched,
			githubwebhook.DesiredStateSourceMatchStatusFilesystemSource,
			"controller_refresh_unavailable",
		},
	}

	switch kind {
	case githubwebhook.DesiredStateSourceKindFilesystem:
		diagnostic.Filesystem = &operatorContextFilesystemSource{Mode: "local_repository"}
		diagnostic.ControllerRefresh.Status = "filesystem_noop"
		diagnostic.ControllerRefresh.Reason = "desired-state source uses the filesystem catalog, so GitHub webhook controller-refresh handoff is a no-op in this mode"
	case githubwebhook.DesiredStateSourceKindGitHub:
		diagnostic.GitHub = &operatorContextGitHubSource{
			Repository: strings.TrimSpace(source.Repository),
			Ref:        strings.TrimSpace(source.Ref),
			Path:       strings.TrimSpace(source.Path),
			BaseURL:    strings.TrimSpace(source.BaseURL),
		}
		diagnostic.ControllerRefresh.Status = "redis_unavailable"
		diagnostic.ControllerRefresh.Reason = "desired-state source uses GitHub, but Redis controller-refresh handoff is not configured for this API instance"
		if provider, ok := options.GitHubWebhookRefreshPublisher.(interface {
			Diagnostic() controllerrefresh.Diagnostic
		}); ok {
			refresh := provider.Diagnostic()
			diagnostic.ControllerRefresh.Configured = refresh.Configured
			diagnostic.ControllerRefresh.Available = refresh.Available
			diagnostic.ControllerRefresh.Backend = refresh.Backend
			diagnostic.ControllerRefresh.Channel = refresh.Channel
			if refresh.Available {
				diagnostic.ControllerRefresh.Status = "available"
				diagnostic.ControllerRefresh.Reason = "GitHub desired-state source can publish accepted matching push webhooks to the Redis controller-refresh handoff"
			}
		} else if options.GitHubWebhookRefreshPublisher != nil {
			diagnostic.ControllerRefresh.Configured = true
			diagnostic.ControllerRefresh.Available = true
			diagnostic.ControllerRefresh.Status = "available"
			diagnostic.ControllerRefresh.Reason = "GitHub desired-state source has a configured controller-refresh handoff publisher"
		}
	case githubwebhook.DesiredStateSourceKindNone:
		diagnostic.ControllerRefresh.Status = "source_not_configured"
		diagnostic.ControllerRefresh.Reason = "desired-state source is not configured, so webhook refresh relevance is unavailable"
	default:
		diagnostic.Configured = false
		diagnostic.Kind = githubwebhook.DesiredStateSourceKindNone
		diagnostic.ControllerRefresh.Status = "source_not_configured"
		diagnostic.ControllerRefresh.Reason = "desired-state source kind is unsupported, so webhook refresh relevance is unavailable"
	}
	return diagnostic
}

func buildOperatorContextRepositoryPolicy(ctx context.Context, options Options, identity requestactor.AuthenticatedIdentity, groups requestactor.AuthenticatedGroups) operatorContextRepositoryPolicy {
	evaluation := rbacpolicy.Evaluation{
		Configured:        false,
		Available:         false,
		Summary:           "repository RBAC policy evaluation is not configured for this API instance",
		UnavailableReason: "repository RBAC policy provider is not configured",
	}
	if options.RepositoryPolicy != nil {
		if resolved, err := options.RepositoryPolicy(ctx, identity, groups); err != nil {
			evaluation = resolved
			if evaluation.Summary == "" {
				evaluation.Summary = "repository RBAC policy evaluation failed"
			}
			if evaluation.UnavailableReason == "" {
				evaluation.UnavailableReason = err.Error()
			}
		} else {
			evaluation = resolved
		}
	}
	return operatorContextRepositoryPolicy{
		Evaluation:                evaluation,
		SupportedGroupHeaderNames: requestactor.SupportedAuthenticatedGroupHeaderNames(),
		DiagnosticOnly:            true,
	}
}

func managedLoginRoutesForProvider(provider string) (string, string) {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case authprovider.GoogleProviderName, authprovider.GoogleOIDCProviderAlias:
		return "/auth/login/google", "/auth/callback/google"
	case authprovider.OktaOIDCProviderName:
		return "/auth/login/okta", "/auth/callback/okta/oidc"
	case authprovider.GitHubProviderName, "":
		return "/auth/login/github", "/auth/callback/github"
	default:
		return "/auth/login/" + strings.ToLower(strings.TrimSpace(provider)), "/auth/callback/" + strings.ToLower(strings.TrimSpace(provider))
	}
}

func canonicalManagedLoginProvider(provider string) string {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if provider == authprovider.GoogleOIDCProviderAlias {
		return authprovider.GoogleProviderName
	}
	return provider
}

func buildAuthRuntimeDiagnosticForRequest(r *http.Request, options Options) AuthRuntimeDiagnostic {
	diagnostic := options.AuthRuntime
	managedLogin := diagnostic.TenetManagedLogin
	authSession := authSessionConfigFromOptions(options)
	managedLogin.SessionCookieSecureOverride = authSession.CookieSecure
	managedLogin.SessionCookiePath = defaultAuthSessionCookiePath
	managedLogin.SessionCookieHTTPOnly = true
	managedLogin.SessionCookieSameSite = defaultAuthSessionCookieSameSite
	managedLogin.OAuthStateTTLSeconds = int64(authSession.stateTTL().Seconds())
	managedLogin.SessionTTLSeconds = int64(authSession.sessionTTL().Seconds())
	managedLogin.SessionExpiryEnforced = true
	managedLogin.ProviderAccessTokenStored = false
	managedLogin.ProviderAccessTokenReturned = false
	managedLogin.ProviderTokenRefreshSupported = false
	managedLogin.ProviderTokenRevocationSupported = false
	managedLogin.LogoutClearsSessionCookie = authSession.active() || managedLogin.Active
	managedLogin.LogoutRevokesProviderToken = false
	if authSession.active() || managedLogin.Configured {
		secureSources := authSession.secureCookieSources(r)
		managedLogin.SessionCookieSecureForRequest = len(secureSources) > 0
		managedLogin.SessionCookieSecureSources = secureSources
	}
	diagnostic.TenetManagedLogin = managedLogin
	enterpriseSSO := diagnostic.EnterpriseSSO
	enterpriseSSO.Provider = canonicalManagedLoginProvider(enterpriseSSO.Provider)
	if enterpriseSSO.Provider != "" {
		if enterpriseSSO.MissingSettings == nil {
			enterpriseSSO.MissingSettings = []string{}
		}
		enterpriseSSO.Active = authSession.active() && authSession.Provider.Name() == enterpriseSSO.Provider && len(enterpriseSSO.MissingSettings) == 0 && len(enterpriseSSO.InvalidSettings) == 0
		if enterpriseSSO.Active {
			enterpriseSSO.Configured = true
			enterpriseSSO.UnavailableReason = ""
		} else if enterpriseSSO.UnavailableReason == "" {
			enterpriseSSO.UnavailableReason = "enterprise SSO provider diagnostics are configured, but provider login/callback runtime is not active for this API instance"
		}
		diagnostic.EnterpriseSSO = enterpriseSSO
	}
	return normalizeAuthRuntimeDiagnostic(diagnostic)
}

func normalizeAuthRuntimeDiagnostic(diagnostic AuthRuntimeDiagnostic) AuthRuntimeDiagnostic {
	managedLogin := diagnostic.TenetManagedLogin
	if managedLogin.OAuthStateTTLSeconds <= 0 {
		managedLogin.OAuthStateTTLSeconds = int64(defaultAuthStateTTL.Seconds())
	}
	if managedLogin.SessionTTLSeconds <= 0 {
		managedLogin.SessionTTLSeconds = int64(defaultAuthSessionTTL.Seconds())
	}
	if strings.TrimSpace(managedLogin.SessionCookiePath) == "" {
		managedLogin.SessionCookiePath = defaultAuthSessionCookiePath
	}
	managedLogin.SessionCookieHTTPOnly = true
	if strings.TrimSpace(managedLogin.SessionCookieSameSite) == "" {
		managedLogin.SessionCookieSameSite = defaultAuthSessionCookieSameSite
	}
	managedLogin.SessionExpiryEnforced = true
	managedLogin.ProviderAccessTokenStored = false
	managedLogin.ProviderAccessTokenReturned = false
	managedLogin.ProviderTokenRefreshSupported = false
	managedLogin.ProviderTokenRevocationSupported = false
	managedLogin.LogoutRevokesProviderToken = false
	if len(managedLogin.RequiredSettings) == 0 {
		managedLogin.RequiredSettings = RequiredTenetManagedLoginSettings()
	}
	enterpriseSSO := diagnostic.EnterpriseSSO
	enterpriseSSO.Provider = canonicalManagedLoginProvider(enterpriseSSO.Provider)
	if enterpriseSSO.Provider != "" {
		if enterpriseSSO.MissingSettings == nil {
			enterpriseSSO.MissingSettings = []string{}
		}
		enterpriseSSO.Configured = enterpriseSSO.Configured && len(enterpriseSSO.MissingSettings) == 0 && len(enterpriseSSO.InvalidSettings) == 0
		enterpriseSSO.Active = enterpriseSSO.Active && enterpriseSSO.Configured
		if enterpriseSSO.Configured && strings.TrimSpace(enterpriseSSO.LogoutRoute) == "" {
			enterpriseSSO.LogoutRoute = "/auth/logout"
		}
		if enterpriseSSO.Active {
			enterpriseSSO.UnavailableReason = ""
		} else if enterpriseSSO.UnavailableReason == "" {
			enterpriseSSO.UnavailableReason = "enterprise SSO provider diagnostics are configured, but provider login/callback runtime is not active for this API instance"
		}
	}

	if managedLogin.Configured {
		managedLogin.Active = managedLogin.Active && managedLogin.Provider != "" && len(managedLogin.MissingSettings) == 0
		if managedLogin.MissingSettings == nil {
			managedLogin.MissingSettings = []string{}
		}
		managedLogin.Provider = canonicalManagedLoginProvider(managedLogin.Provider)
		if managedLogin.Provider == "" {
			managedLogin.Provider = authprovider.GitHubProviderName
		}
		loginRoute, callbackRoute := managedLoginRoutesForProvider(managedLogin.Provider)
		if managedLogin.LoginRoute == "" {
			managedLogin.LoginRoute = loginRoute
		}
		if managedLogin.CallbackRoute == "" {
			managedLogin.CallbackRoute = callbackRoute
		}
		if managedLogin.LogoutRoute == "" && managedLogin.Active {
			managedLogin.LogoutRoute = "/auth/logout"
		}
		if managedLogin.SessionCookieName == "" {
			managedLogin.SessionCookieName = "tenet_session"
		}
		if managedLogin.Active {
			managedLogin.UnavailableReason = ""
		} else if managedLogin.UnavailableReason == "" {
			managedLogin.UnavailableReason = "tenet-managed login configuration is present, but login/callback/session routes are not active for this API instance"
		}
		return AuthRuntimeDiagnostic{TenetManagedLogin: managedLogin, EnterpriseSSO: enterpriseSSO}
	}
	managedLogin.Configured = false
	if len(managedLogin.MissingSettings) == 0 {
		managedLogin.MissingSettings = RequiredTenetManagedLoginSettings()
	}
	if managedLogin.UnavailableReason == "" {
		managedLogin.UnavailableReason = "tenet-managed login is disabled because OAuth runtime configuration is incomplete or absent"
	}
	return AuthRuntimeDiagnostic{TenetManagedLogin: managedLogin, EnterpriseSSO: enterpriseSSO}
}

// RequiredTenetManagedLoginSettings returns the stable non-secret runtime settings
// needed before Tenet-managed GitHub login can become active for this API process.
func RequiredTenetManagedLoginSettings() []string {
	return []string{
		"TENET_AUTH_PROVIDER",
		"TENET_GITHUB_OAUTH_CLIENT_ID",
		"TENET_GITHUB_OAUTH_CLIENT_SECRET",
		"TENET_GITHUB_OAUTH_REDIRECT_URL",
		"TENET_AUTH_STATE_STORE_PATH",
		"TENET_AUTH_IDENTITY_STORE_PATH",
		"TENET_AUTH_SESSION_SECRET",
	}
}

// DefaultTenetManagedLoginOAuthStateTTLSeconds returns the public diagnostic TTL
// for one-time OAuth callback state records created by Tenet-managed login.
func DefaultTenetManagedLoginOAuthStateTTLSeconds() int64 {
	return int64(defaultAuthStateTTL.Seconds())
}

// DefaultTenetManagedLoginSessionTTLSeconds returns the public diagnostic TTL
// for signed Tenet-managed browser session cookies.
func DefaultTenetManagedLoginSessionTTLSeconds() int64 {
	return int64(defaultAuthSessionTTL.Seconds())
}

func buildOperatorContextEnforcement(options Options) operatorContextEnforcement {
	readRoutesGuarded := options.RequireDeploymentReadAuthorization
	writeRoutesGuarded := options.RequireAuthenticatedIdentity || options.RequireOperatorRoleForWrite || options.RequireDeploymentReadAuthorization
	enforcement := operatorContextEnforcement{
		Mode:                                   "local_unrestricted",
		Summary:                                "trusted front-door headers are not enabled; Tenet is not enforcing authenticated operator gates for this API instance",
		TrustedHeadersEnabled:                  options.TrustAuthenticatedHeaders,
		AuthenticatedIdentityRequired:          options.RequireAuthenticatedIdentity,
		DeploymentReadAuthorizationEnforced:    options.RequireDeploymentReadAuthorization,
		OperatorReadRoleEnforced:               options.RequireOperatorRoleForRead,
		OperatorWriteRoleEnforced:              options.RequireOperatorRoleForWrite,
		ReadRoutesGuarded:                      readRoutesGuarded,
		WriteRoutesGuarded:                     writeRoutesGuarded,
		DeploymentScopedRoutesRequireAllowlist: options.RequireDeploymentReadAuthorization,
		CollectionReadsFiltered:                options.RequireDeploymentReadAuthorization,
	}
	if !options.TrustAuthenticatedHeaders {
		return enforcement
	}
	if !readRoutesGuarded && !writeRoutesGuarded {
		enforcement.Mode = "trusted_headers_observed_only"
		enforcement.Summary = "trusted front-door headers are accepted for diagnostics and actor projection, but guarded operator route enforcement is disabled"
		return enforcement
	}
	if readRoutesGuarded && writeRoutesGuarded {
		enforcement.Mode = "trusted_front_door_read_write"
		enforcement.Summary = "trusted front-door identity, roles, and deployment allowlists are enforcing guarded operator read/write routes"
		return enforcement
	}
	if readRoutesGuarded {
		enforcement.Mode = "trusted_front_door_read_only"
		enforcement.Summary = "trusted front-door identity, viewer-capable roles, and deployment allowlists are enforcing guarded operator read routes"
		return enforcement
	}
	enforcement.Mode = "trusted_front_door_write_only"
	enforcement.Summary = "trusted front-door identity and operator roles are enforcing guarded operator write routes"
	return enforcement
}

func buildOperatorContextAccessVerdicts(options Options, identity requestactor.AuthenticatedIdentity, roles requestactor.AuthenticatedRoles, deployments requestactor.AuthorizedDeployments, scopes requestactor.AuthorizedProjectEnvironmentScopes, summary *operatorContextAccessSummary) operatorContextAccessVerdicts {
	operatorRead := operatorContextReadVerdict(options, identity, roles)
	operatorWrite := operatorContextWriteVerdict(options, identity, roles)
	return operatorContextAccessVerdicts{
		OperatorRead:          operatorRead,
		OperatorWrite:         operatorWrite,
		DeploymentScopedRead:  operatorContextDeploymentScopedReadVerdict(options, identity, roles, deployments, scopes, summary),
		DeploymentScopedWrite: operatorContextDeploymentScopedWriteVerdict(options, identity, roles, deployments, scopes, summary, operatorWrite),
		CollectionRead:        operatorContextCollectionReadVerdict(options, identity, roles, deployments, scopes, summary),
		CatalogRead:           operatorContextCatalogReadVerdict(options, identity, roles, deployments, scopes, summary),
	}
}

func operatorContextReadVerdict(options Options, identity requestactor.AuthenticatedIdentity, roles requestactor.AuthenticatedRoles) operatorContextAccessVerdict {
	verdict := operatorContextAccessVerdict{
		Allowed:                       true,
		Reason:                        "operator read routes are not enforcing trusted front-door identity for this API instance",
		ExampleRoutes:                 cloneOperatorContextExampleRoutes(operatorContextOperatorReadExampleRoutes),
		AcceptedRoleNames:             requestactor.AcceptedOperatorReadRoleNames(),
		RequiresAuthenticatedIdentity: options.RequireDeploymentReadAuthorization,
		RequiresViewerCapableRole:     options.RequireDeploymentReadAuthorization && options.RequireOperatorRoleForRead,
	}
	if verdict.RequiresAuthenticatedIdentity {
		verdict.RequiredTrustedHeaders = cloneOperatorContextRequiredHeaders(operatorContextAuthenticatedIdentityHeaders)
	}
	if verdict.RequiresViewerCapableRole {
		verdict.RequiredTrustedHeaders = appendOperatorContextRequiredHeaders(verdict.RequiredTrustedHeaders, operatorContextViewerRoleHeaders)
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
		ExampleRoutes:                 cloneOperatorContextExampleRoutes(operatorContextOperatorWriteExampleRoutes),
		AcceptedRoleNames:             requestactor.AcceptedOperatorWriteRoleNames(),
		RequiresAuthenticatedIdentity: identityRequired,
		RequiresOperatorRole:          options.RequireOperatorRoleForWrite,
	}
	if verdict.RequiresAuthenticatedIdentity {
		verdict.RequiredTrustedHeaders = cloneOperatorContextRequiredHeaders(operatorContextAuthenticatedIdentityHeaders)
	}
	if verdict.RequiresOperatorRole {
		verdict.RequiredTrustedHeaders = appendOperatorContextRequiredHeaders(verdict.RequiredTrustedHeaders, operatorContextOperatorRoleHeaders)
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

func operatorContextDeploymentScopedReadVerdict(options Options, identity requestactor.AuthenticatedIdentity, roles requestactor.AuthenticatedRoles, deployments requestactor.AuthorizedDeployments, scopes requestactor.AuthorizedProjectEnvironmentScopes, summary *operatorContextAccessSummary) operatorContextAccessVerdict {
	verdict := operatorContextReadVerdict(options, identity, roles)
	verdict.ExampleRoutes = cloneOperatorContextExampleRoutes(operatorContextDeploymentScopedReadExampleRoutes)
	verdict.RequiresExactDeploymentAllowlistMatch = options.RequireDeploymentReadAuthorization
	verdict.AcceptsProjectEnvironmentScopes = options.RequireDeploymentReadAuthorization
	if verdict.RequiresExactDeploymentAllowlistMatch {
		verdict.RequiredTrustedHeaders = appendOperatorContextRequiredHeaders(verdict.RequiredTrustedHeaders, operatorContextDeploymentAllowlistHeaders, operatorContextProjectEnvironmentScopeHeaders)
	}
	if !options.RequireDeploymentReadAuthorization {
		verdict.Allowed = true
		verdict.Reason = "deployment-scoped reads are not requiring exact deployment allowlist membership for this API instance"
		return verdict
	}
	if !verdict.Allowed {
		return verdict
	}
	hasExactGrant := len(deployments.Names) > 0
	hasProjectScope := len(scopes.Projects) > 0
	hasEnvironmentScope := len(scopes.Environments) > 0
	if !hasExactGrant && !hasProjectScope && !hasEnvironmentScope {
		verdict.Allowed = false
		verdict.Reason = "deployment-scoped reads will return 403 unless X-Tenet-Authorized-Deployments names the deployment or project/environment scopes match catalog placement"
		return verdict
	}
	if !hasExactGrant && (hasProjectScope != hasEnvironmentScope) {
		verdict.Allowed = false
		verdict.Reason = "deployment-scoped reads need both X-Tenet-Authorized-Projects and X-Tenet-Authorized-Environments when no exact deployment allowlist is forwarded"
		return verdict
	}
	if !hasExactGrant && hasProjectScope && hasEnvironmentScope {
		if summary == nil || !summary.Available {
			verdict.Allowed = false
			verdict.Reason = "deployment-scoped reads using only project/environment scopes will fail safely until catalog placement data is available"
			return verdict
		}
		if summary.MatchedCount == 0 {
			verdict.Allowed = false
			verdict.Reason = "deployment-scoped reads using only project/environment scopes will return 403 because those scopes do not match catalog deployments"
			return verdict
		}
	}
	verdict.Reason = "deployment-scoped reads can pass for deployments named in X-Tenet-Authorized-Deployments or matched by X-Tenet-Authorized-Projects plus X-Tenet-Authorized-Environments"
	return verdict
}

func operatorContextDeploymentScopedWriteVerdict(options Options, identity requestactor.AuthenticatedIdentity, roles requestactor.AuthenticatedRoles, deployments requestactor.AuthorizedDeployments, scopes requestactor.AuthorizedProjectEnvironmentScopes, summary *operatorContextAccessSummary, operatorWrite operatorContextAccessVerdict) operatorContextAccessVerdict {
	verdict := operatorWrite
	verdict.ExampleRoutes = cloneOperatorContextExampleRoutes(operatorContextDeploymentScopedWriteExampleRoutes)
	verdict.RequiresExactDeploymentAllowlistMatch = options.RequireDeploymentReadAuthorization
	verdict.AcceptsProjectEnvironmentScopes = options.RequireDeploymentReadAuthorization
	if verdict.RequiresExactDeploymentAllowlistMatch {
		verdict.RequiredTrustedHeaders = appendOperatorContextRequiredHeaders(verdict.RequiredTrustedHeaders, operatorContextDeploymentAllowlistHeaders, operatorContextProjectEnvironmentScopeHeaders)
	}
	if !verdict.Allowed {
		return verdict
	}
	if !options.RequireDeploymentReadAuthorization {
		verdict.Reason = "deployment-scoped writes are not requiring deployment allowlist or project/environment scope membership for this API instance"
		return verdict
	}
	hasExactGrant := len(deployments.Names) > 0
	hasProjectScope := len(scopes.Projects) > 0
	hasEnvironmentScope := len(scopes.Environments) > 0
	if !hasExactGrant && !hasProjectScope && !hasEnvironmentScope {
		verdict.Allowed = false
		verdict.Reason = "deployment-scoped writes will return 403 unless X-Tenet-Authorized-Deployments names the deployment or project/environment scopes match catalog placement"
		return verdict
	}
	if !hasExactGrant && (hasProjectScope != hasEnvironmentScope) {
		verdict.Allowed = false
		verdict.Reason = "deployment-scoped writes need both X-Tenet-Authorized-Projects and X-Tenet-Authorized-Environments when no exact deployment allowlist is forwarded"
		return verdict
	}
	if !hasExactGrant && hasProjectScope && hasEnvironmentScope {
		if summary == nil || !summary.Available {
			verdict.Allowed = false
			verdict.Reason = "deployment-scoped writes using only project/environment scopes will fail safely until catalog placement data is available"
			return verdict
		}
		if summary.MatchedCount == 0 {
			verdict.Allowed = false
			verdict.Reason = "deployment-scoped writes using only project/environment scopes will return 403 because those scopes do not match catalog deployments"
			return verdict
		}
	}
	verdict.Reason = "deployment-scoped writes can pass for deployments named in X-Tenet-Authorized-Deployments or matched by X-Tenet-Authorized-Projects plus X-Tenet-Authorized-Environments"
	return verdict
}

func operatorContextCollectionReadVerdict(options Options, identity requestactor.AuthenticatedIdentity, roles requestactor.AuthenticatedRoles, deployments requestactor.AuthorizedDeployments, scopes requestactor.AuthorizedProjectEnvironmentScopes, summary *operatorContextAccessSummary) operatorContextAccessVerdict {
	verdict := operatorContextReadVerdict(options, identity, roles)
	verdict.ExampleRoutes = cloneOperatorContextExampleRoutes(operatorContextCollectionReadExampleRoutes)
	verdict.FiltersCollectionsByDeploymentAccess = options.RequireDeploymentReadAuthorization
	if verdict.FiltersCollectionsByDeploymentAccess {
		verdict.RequiredTrustedHeaders = appendOperatorContextRequiredHeaders(verdict.RequiredTrustedHeaders, operatorContextDeploymentAllowlistHeaders, operatorContextProjectEnvironmentScopeHeaders)
	}
	if !options.RequireDeploymentReadAuthorization {
		verdict.Allowed = true
		verdict.Reason = "collection reads are not filtered by deployment allowlist for this API instance"
		return verdict
	}
	if !verdict.Allowed {
		return verdict
	}
	verdict.Reason = "collection reads are filtered to the union of exact deployment grants and project/environment-scoped catalog matches"
	if len(deployments.Names) == 0 && len(scopes.Projects) == 0 && len(scopes.Environments) == 0 {
		verdict.EmptyCollectionLikely = true
		verdict.Reason = "collection reads will be allowed but return empty collections until deployment or project/environment scopes forward at least one visible deployment"
		return verdict
	}
	if len(deployments.Names) == 0 && len(scopes.Projects) > 0 && len(scopes.Environments) > 0 && summary != nil && !summary.Available {
		verdict.EmptyCollectionLikely = true
		verdict.Reason = "collection reads will be allowed but likely empty because catalog placement data is unavailable to match the forwarded project/environment scopes"
		return verdict
	}
	if summary != nil && summary.Available && summary.MatchedCount == 0 {
		verdict.EmptyCollectionLikely = true
		verdict.Reason = "collection reads will be allowed but likely empty because the forwarded deployment grants or project/environment scopes do not match catalog deployments"
	}
	return verdict
}

func operatorContextCatalogReadVerdict(options Options, identity requestactor.AuthenticatedIdentity, roles requestactor.AuthenticatedRoles, deployments requestactor.AuthorizedDeployments, scopes requestactor.AuthorizedProjectEnvironmentScopes, summary *operatorContextAccessSummary) operatorContextAccessVerdict {
	verdict := operatorContextCollectionReadVerdict(options, identity, roles, deployments, scopes, summary)
	verdict.ExampleRoutes = cloneOperatorContextExampleRoutes(operatorContextCatalogReadExampleRoutes)
	if !options.RequireDeploymentReadAuthorization {
		verdict.Reason = "raw catalog reads are not filtered by deployment allowlist for this API instance"
		return verdict
	}
	if !verdict.Allowed {
		return verdict
	}
	if verdict.EmptyCollectionLikely {
		return verdict
	}
	verdict.Reason = "raw catalog reads are filtered to deployment-visible desired-state metadata before returning catalog collections or summaries"
	return verdict
}

func cloneOperatorContextExampleRoutes(routes []string) []string {
	return append([]string(nil), routes...)
}

func cloneOperatorContextRequiredHeaders(headers []string) []string {
	return append([]string(nil), headers...)
}

func appendOperatorContextRequiredHeaders(headers []string, additional ...[]string) []string {
	for _, set := range additional {
		headers = append(headers, set...)
	}
	return headers
}

func buildOperatorContextAccessSummary(ctx context.Context, options Options, deployments requestactor.AuthorizedDeployments, scopes requestactor.AuthorizedProjectEnvironmentScopes) *operatorContextAccessSummary {
	if !options.RequireDeploymentReadAuthorization || (len(deployments.Names) == 0 && len(scopes.Projects) == 0 && len(scopes.Environments) == 0) {
		return nil
	}
	summary := &operatorContextAccessSummary{
		ForwardedCount:            len(deployments.Names),
		ForwardedProjectCount:     len(scopes.Projects),
		ForwardedEnvironmentCount: len(scopes.Environments),
	}
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
	projectsScope := stringSet(scopes.Projects)
	environmentsScope := stringSet(scopes.Environments)
	hasProjectEnvironmentScope := len(projectsScope) > 0 && len(environmentsScope) > 0
	matched := make(map[string]catalog.PlacementView)
	matchSources := make(map[string][]string)
	for _, placement := range placements {
		if placement.DeploymentName == "" {
			continue
		}
		_, exactMatch := forwarded[placement.DeploymentName]
		projectEnvironmentMatch := false
		if hasProjectEnvironmentScope && placement.AppRef != "" && placement.EnvironmentRef != "" {
			if _, ok := projectsScope[placement.AppRef]; ok {
				if _, ok := environmentsScope[placement.EnvironmentRef]; ok {
					projectEnvironmentMatch = true
				}
			}
		}
		if !exactMatch && !projectEnvironmentMatch {
			continue
		}
		matched[placement.DeploymentName] = placement
		if exactMatch {
			matchSources[placement.DeploymentName] = append(matchSources[placement.DeploymentName], "exactDeployment")
		}
		if projectEnvironmentMatch {
			matchSources[placement.DeploymentName] = append(matchSources[placement.DeploymentName], "projectEnvironment")
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
	summary.UnmatchedCount = len(unmatched)
	if len(unmatched) > operatorContextUnmatchedDeploymentLimit {
		summary.UnmatchedNamesTruncated = true
		unmatched = unmatched[:operatorContextUnmatchedDeploymentLimit]
	}
	if hasProjectEnvironmentScope {
		matchedProjects := make(map[string]struct{}, len(projects))
		matchedEnvironments := make(map[string]struct{}, len(environments))
		for project := range projects {
			matchedProjects[project] = struct{}{}
		}
		for environment := range environments {
			matchedEnvironments[environment] = struct{}{}
		}
		summary.ForwardedProjectCount = len(scopes.Projects)
		summary.ForwardedEnvironmentCount = len(scopes.Environments)
		summary.UnmatchedProjects, summary.UnmatchedProjectCount, summary.UnmatchedProjectsTruncated = boundedOperatorContextUnmatchedValues(scopes.Projects, matchedProjects, operatorContextUnmatchedDeploymentLimit)
		summary.UnmatchedEnvironments, summary.UnmatchedEnvironmentCount, summary.UnmatchedEnvironmentsTruncated = boundedOperatorContextUnmatchedValues(scopes.Environments, matchedEnvironments, operatorContextUnmatchedDeploymentLimit)
	}
	summary.Available = true
	summary.MatchedCount = len(matched)
	for _, sources := range matchSources {
		for _, source := range sources {
			switch source {
			case "exactDeployment":
				summary.ExactDeploymentMatchedCount++
			case "projectEnvironment":
				summary.ProjectEnvironmentMatchedCount++
			}
		}
	}
	summary.UnmatchedNames = unmatched
	summary.AccountCount = len(accounts)
	summary.ProjectCount = len(projects)
	summary.EnvironmentCount = len(environments)
	summary.VisibleDeploymentExamples = buildOperatorContextVisibleDeploymentExamples(matched, matchSources)
	summary.ProjectEnvironmentExamples = buildOperatorContextProjectEnvironmentExamples(matched)
	if len(matched) == 0 {
		summary.EmptyReason = "no forwarded deployment names or project/environment scopes match catalog placements"
	}
	return summary
}

func boundedOperatorContextUnmatchedValues(forwarded []string, matched map[string]struct{}, limit int) ([]string, int, bool) {
	unmatched := make([]string, 0, len(forwarded))
	for _, value := range forwarded {
		if _, ok := matched[value]; ok {
			continue
		}
		unmatched = append(unmatched, value)
	}
	sort.Strings(unmatched)
	count := len(unmatched)
	truncated := false
	if len(unmatched) > limit {
		truncated = true
		unmatched = unmatched[:limit]
	}
	return unmatched, count, truncated
}

func buildOperatorContextVisibleDeploymentExamples(matched map[string]catalog.PlacementView, matchSources map[string][]string) []operatorContextVisibleDeploymentExample {
	deploymentNames := make([]string, 0, len(matched))
	for deploymentName := range matched {
		deploymentNames = append(deploymentNames, deploymentName)
	}
	sort.Strings(deploymentNames)
	if len(deploymentNames) > operatorContextVisibleDeploymentExampleLimit {
		deploymentNames = deploymentNames[:operatorContextVisibleDeploymentExampleLimit]
	}
	examples := make([]operatorContextVisibleDeploymentExample, 0, len(deploymentNames))
	for _, deploymentName := range deploymentNames {
		placement := matched[deploymentName]
		examples = append(examples, operatorContextVisibleDeploymentExample{
			DeploymentName: deploymentName,
			AppRef:         placement.AppRef,
			EnvironmentRef: placement.EnvironmentRef,
			Region:         placement.Region,
			AWSAccountID:   placement.AWSAccountID,
			MatchSources:   append([]string(nil), matchSources[deploymentName]...),
		})
	}
	return examples
}

func buildOperatorContextProjectEnvironmentExamples(matched map[string]catalog.PlacementView) []operatorContextProjectEnvironmentExample {
	type groupKey struct {
		appRef         string
		environmentRef string
		region         string
		awsAccountID   string
	}
	groups := make(map[groupKey][]string)
	for deploymentName, placement := range matched {
		key := groupKey{
			appRef:         placement.AppRef,
			environmentRef: placement.EnvironmentRef,
			region:         placement.Region,
			awsAccountID:   placement.AWSAccountID,
		}
		groups[key] = append(groups[key], deploymentName)
	}
	keys := make([]groupKey, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
		sort.Strings(groups[key])
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].appRef != keys[j].appRef {
			return keys[i].appRef < keys[j].appRef
		}
		if keys[i].environmentRef != keys[j].environmentRef {
			return keys[i].environmentRef < keys[j].environmentRef
		}
		if keys[i].region != keys[j].region {
			return keys[i].region < keys[j].region
		}
		return keys[i].awsAccountID < keys[j].awsAccountID
	})
	if len(keys) > operatorContextProjectEnvironmentExampleLimit {
		keys = keys[:operatorContextProjectEnvironmentExampleLimit]
	}
	examples := make([]operatorContextProjectEnvironmentExample, 0, len(keys))
	for _, key := range keys {
		deployments := groups[key]
		exampleDeployments := deployments
		if len(exampleDeployments) > operatorContextProjectEnvironmentExampleDeploymentLimit {
			exampleDeployments = exampleDeployments[:operatorContextProjectEnvironmentExampleDeploymentLimit]
		}
		examples = append(examples, operatorContextProjectEnvironmentExample{
			AppRef:             key.appRef,
			EnvironmentRef:     key.environmentRef,
			Region:             key.region,
			AWSAccountID:       key.awsAccountID,
			DeploymentCount:    len(deployments),
			ExampleDeployments: append([]string(nil), exampleDeployments...),
		})
	}
	return examples
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

func filterAuthorizedDeploymentOverviews(deployments []catalog.DeploymentOverview, visible map[string]struct{}, enabled bool) []catalog.DeploymentOverview {
	if !enabled {
		return deployments
	}
	filtered := make([]catalog.DeploymentOverview, 0, len(deployments))
	for _, deployment := range deployments {
		if _, ok := visible[deployment.Name]; ok {
			filtered = append(filtered, deployment)
		}
	}
	return filtered
}

func filterAuthorizedCatalogDeployments(deployments []catalog.DeploymentView, visible map[string]struct{}, enabled bool) []catalog.DeploymentView {
	if !enabled {
		return deployments
	}
	filtered := make([]catalog.DeploymentView, 0, len(deployments))
	for _, deployment := range deployments {
		if _, ok := visible[deployment.Name]; ok {
			filtered = append(filtered, deployment)
		}
	}
	return filtered
}

func filterAuthorizedCatalogPlacements(placements []catalog.PlacementView, visible map[string]struct{}, enabled bool) []catalog.PlacementView {
	if !enabled {
		return placements
	}
	filtered := make([]catalog.PlacementView, 0, len(placements))
	for _, placement := range placements {
		if _, ok := visible[placement.DeploymentName]; ok {
			filtered = append(filtered, placement)
		}
	}
	return filtered
}

func filterAuthorizedCatalogAccounts(accounts []catalog.AccountView, placements []catalog.PlacementView, visible map[string]struct{}, enabled bool) []catalog.AccountView {
	if !enabled {
		return accounts
	}
	visibleAccounts := make(map[string]struct{})
	for _, placement := range filterAuthorizedCatalogPlacements(placements, visible, enabled) {
		if placement.AccountName != "" {
			visibleAccounts[placement.AccountName] = struct{}{}
		}
		if placement.AWSAccountID != "" {
			visibleAccounts[placement.AWSAccountID] = struct{}{}
		}
	}
	filtered := make([]catalog.AccountView, 0, len(accounts))
	for _, account := range accounts {
		if _, ok := visibleAccounts[account.Name]; ok {
			filtered = append(filtered, account)
			continue
		}
		if _, ok := visibleAccounts[account.AWSAccountID]; ok {
			filtered = append(filtered, account)
		}
	}
	return filtered
}

func filterAuthorizedCatalogEnvironments(environments []catalog.EnvironmentView, placements []catalog.PlacementView, visible map[string]struct{}, enabled bool) []catalog.EnvironmentView {
	if !enabled {
		return environments
	}
	visibleEnvironments := make(map[string]struct{})
	for _, placement := range filterAuthorizedCatalogPlacements(placements, visible, enabled) {
		if placement.EnvironmentRef != "" {
			visibleEnvironments[placement.EnvironmentRef] = struct{}{}
		}
	}
	filtered := make([]catalog.EnvironmentView, 0, len(environments))
	for _, environment := range environments {
		if _, ok := visibleEnvironments[environment.Name]; ok {
			filtered = append(filtered, environment)
		}
	}
	return filtered
}

func summarizeCatalogTopology(topology catalog.TopologySnapshot) catalog.Summary {
	return catalog.Summary{
		AccountCount:     len(topology.Accounts),
		EnvironmentCount: len(topology.Environments),
		DeploymentCount:  len(topology.Deployments),
	}
}

func filterAuthorizedCatalogTopology(topology catalog.TopologySnapshot, visible map[string]struct{}, enabled bool) catalog.TopologySnapshot {
	if !enabled {
		return topology
	}
	topology.Deployments = filterAuthorizedCatalogDeployments(topology.Deployments, visible, enabled)
	topology.Placements = filterAuthorizedCatalogPlacements(topology.Placements, visible, enabled)
	topology.Accounts = filterAuthorizedCatalogAccounts(topology.Accounts, topology.Placements, visible, enabled)
	topology.Environments = filterAuthorizedCatalogEnvironments(topology.Environments, topology.Placements, visible, enabled)
	topology.Summary = summarizeCatalogTopology(topology)
	return topology
}

func filteredCatalogPlacementsForAuthorization(ctx context.Context, options Options, visible map[string]struct{}) ([]catalog.PlacementView, bool) {
	if !options.RequireDeploymentReadAuthorization {
		return nil, false
	}
	if options.CatalogPlacements == nil {
		return nil, true
	}
	placements, err := options.CatalogPlacements(ctx)
	if err != nil {
		return nil, true
	}
	return filterAuthorizedCatalogPlacements(placements, visible, true), true
}

func filterAuthorizedStatusSnapshots(snapshots []statusstore.Snapshot, visible map[string]struct{}, enabled bool) []statusstore.Snapshot {
	if !enabled {
		return snapshots
	}
	filtered := make([]statusstore.Snapshot, 0, len(snapshots))
	for _, snapshot := range snapshots {
		if _, ok := visible[snapshot.DeploymentName]; ok {
			filtered = append(filtered, snapshot)
		}
	}
	return filtered
}

func assignManagedDriftIDsToSnapshots(snapshots []statusstore.Snapshot) []statusstore.Snapshot {
	if len(snapshots) == 0 {
		return snapshots
	}
	assigned := make([]statusstore.Snapshot, len(snapshots))
	copy(assigned, snapshots)
	for i := range assigned {
		assigned[i].Status = assignManagedDriftIDsToStatus(assigned[i].Status)
	}
	return assigned
}

func assignManagedDriftIDsToStatus(status domain.DeploymentStatus) domain.DeploymentStatus {
	status.ManagedDrift = domain.AssignManagedDriftIDs(status.ManagedDrift)
	if len(status.History) > 0 {
		status.History = append([]domain.StatusHistoryEntry(nil), status.History...)
		for i := range status.History {
			status.History[i].ManagedDrift = domain.AssignManagedDriftIDs(status.History[i].ManagedDrift)
		}
	}
	return status
}

func filterAuthorizedAuditEntries(entries []auditlog.Entry, visible map[string]struct{}, enabled bool) []auditlog.Entry {
	if !enabled {
		return entries
	}
	filtered := make([]auditlog.Entry, 0, len(entries))
	for _, entry := range entries {
		if _, ok := visible[entry.DeploymentName]; ok {
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

func filterAuthorizedAccountOverviews(accounts []catalog.AccountOverview, visible map[string]struct{}, enabled bool) []catalog.AccountOverview {
	if !enabled {
		return accounts
	}
	filtered := make([]catalog.AccountOverview, 0, len(accounts))
	for _, account := range accounts {
		visibleDeployments := filterAuthorizedDeploymentOverviews(account.Deployments, visible, enabled)
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

func filterAuthorizedAccountOverviewDetail(detail catalog.AccountOverviewDetail, visible map[string]struct{}, enabled bool) catalog.AccountOverviewDetail {
	if !enabled {
		return detail
	}
	visibleDeployments := filterAuthorizedDeploymentOverviews(detail.Deployments, visible, enabled)
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

func filterAuthorizedProjectEnvironmentOverviewDetail(detail catalog.ProjectEnvironmentOverviewDetail, visible map[string]struct{}, enabled bool) catalog.ProjectEnvironmentOverviewDetail {
	if !enabled {
		return detail
	}
	visibleDeployments := filterAuthorizedDeploymentOverviews(detail.Deployments, visible, enabled)
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
	AuthRuntime                          AuthRuntimeDiagnostic
	AuthProvider                         authprovider.Provider
	AuthStateStore                       authstate.Store
	AuthIdentityStore                    authidentity.Store
	AuthSessionSecret                    string
	AuthSessionCookieName                string
	AuthSessionCookieSecure              bool
	TrustAuthenticatedHeaders            bool
	RequireAuthenticatedIdentity         bool
	RequireOperatorRoleForRead           bool
	RequireOperatorRoleForWrite          bool
	RequireDeploymentReadAuthorization   bool
	RepositoryPolicy                     func(context.Context, requestactor.AuthenticatedIdentity, requestactor.AuthenticatedGroups) (rbacpolicy.Evaluation, error)
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
	OverviewDeploymentResourceLogs       func(context.Context, string, string, string, int) (resourcelogs.Result, error)
	OverviewDeploymentResourceInspection func(context.Context, string, string, string, int) (resourceinspection.Result, error)
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

	authSession := authSessionConfigFromOptions(options)
	options.AuthRuntime.TenetManagedLogin.Active = authSession.active()
	if authSession.active() {
		options.AuthRuntime.TenetManagedLogin.Configured = true
		providerName := authSession.Provider.Name()
		loginRoute, callbackRoute := managedLoginRoutesForProvider(providerName)
		options.AuthRuntime.TenetManagedLogin.Provider = providerName
		options.AuthRuntime.TenetManagedLogin.LoginRoute = loginRoute
		options.AuthRuntime.TenetManagedLogin.CallbackRoute = callbackRoute
		options.AuthRuntime.TenetManagedLogin.LogoutRoute = "/auth/logout"
		options.AuthRuntime.TenetManagedLogin.SessionCookieName = authSession.cookieName()
		options.AuthRuntime.TenetManagedLogin.MissingSettings = []string{}
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
	mux.HandleFunc("/auth/login/github", func(w http.ResponseWriter, r *http.Request) {
		handleGitHubLogin(w, r, authSession)
	})
	mux.HandleFunc("/auth/callback/github", func(w http.ResponseWriter, r *http.Request) {
		handleGitHubCallback(w, r, authSession)
	})
	mux.HandleFunc("/auth/login/google", func(w http.ResponseWriter, r *http.Request) {
		handleGoogleLogin(w, r, authSession)
	})
	mux.HandleFunc("/auth/callback/google", func(w http.ResponseWriter, r *http.Request) {
		handleGoogleCallback(w, r, authSession)
	})
	mux.HandleFunc("/auth/login/okta", func(w http.ResponseWriter, r *http.Request) {
		handleOktaOIDCLogin(w, r, authSession)
	})
	mux.HandleFunc("/auth/callback/okta/oidc", func(w http.ResponseWriter, r *http.Request) {
		handleOktaOIDCCallback(w, r, authSession)
	})
	if authSession.active() {
		mux.HandleFunc("/auth/logout", func(w http.ResponseWriter, r *http.Request) {
			handleManagedLoginLogout(w, r, authSession)
		})
	}
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
		if !requireDeploymentReadIdentity(w, r, options.RequireDeploymentReadAuthorization, options.RequireOperatorRoleForRead) {
			return
		}
		if options.RequireDeploymentReadAuthorization {
			if options.CatalogTopology == nil {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(catalog.Summary{})
				return
			}
			topology, err := options.CatalogTopology(r.Context())
			if err != nil {
				respondReadProviderError(w, err, "catalog summary unavailable")
				return
			}
			topology = filterAuthorizedCatalogTopology(
				topology,
				authorizedDeploymentNameSet(r.Context(), options, requestactor.AuthorizedDeploymentsFromContext(r.Context()), requestactor.AuthorizedProjectEnvironmentScopesFromContext(r.Context())),
				true,
			)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(topology.Summary)
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
		if !requireDeploymentReadIdentity(w, r, options.RequireDeploymentReadAuthorization, options.RequireOperatorRoleForRead) {
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
		deployments = filterAuthorizedCatalogDeployments(
			deployments,
			authorizedDeploymentNameSet(r.Context(), options, requestactor.AuthorizedDeploymentsFromContext(r.Context()), requestactor.AuthorizedProjectEnvironmentScopesFromContext(r.Context())),
			options.RequireDeploymentReadAuthorization,
		)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(deployments)
	})
	mux.HandleFunc("/v1/catalog/environments", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		if !requireDeploymentReadIdentity(w, r, options.RequireDeploymentReadAuthorization, options.RequireOperatorRoleForRead) {
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
		visible := authorizedDeploymentNameSet(r.Context(), options, requestactor.AuthorizedDeploymentsFromContext(r.Context()), requestactor.AuthorizedProjectEnvironmentScopesFromContext(r.Context()))
		if placements, ok := filteredCatalogPlacementsForAuthorization(r.Context(), options, visible); ok {
			environments = filterAuthorizedCatalogEnvironments(environments, placements, visible, true)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(environments)
	})
	mux.HandleFunc("/v1/catalog/accounts", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		if !requireDeploymentReadIdentity(w, r, options.RequireDeploymentReadAuthorization, options.RequireOperatorRoleForRead) {
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
		visible := authorizedDeploymentNameSet(r.Context(), options, requestactor.AuthorizedDeploymentsFromContext(r.Context()), requestactor.AuthorizedProjectEnvironmentScopesFromContext(r.Context()))
		if placements, ok := filteredCatalogPlacementsForAuthorization(r.Context(), options, visible); ok {
			accounts = filterAuthorizedCatalogAccounts(accounts, placements, visible, true)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(accounts)
	})
	mux.HandleFunc("/v1/catalog/placements", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		if !requireDeploymentReadIdentity(w, r, options.RequireDeploymentReadAuthorization, options.RequireOperatorRoleForRead) {
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
		placements = filterAuthorizedCatalogPlacements(
			placements,
			authorizedDeploymentNameSet(r.Context(), options, requestactor.AuthorizedDeploymentsFromContext(r.Context()), requestactor.AuthorizedProjectEnvironmentScopesFromContext(r.Context())),
			options.RequireDeploymentReadAuthorization,
		)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(placements)
	})
	mux.HandleFunc("/v1/catalog/topology", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		if !requireDeploymentReadIdentity(w, r, options.RequireDeploymentReadAuthorization, options.RequireOperatorRoleForRead) {
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
		topology = filterAuthorizedCatalogTopology(
			topology,
			authorizedDeploymentNameSet(r.Context(), options, requestactor.AuthorizedDeploymentsFromContext(r.Context()), requestactor.AuthorizedProjectEnvironmentScopesFromContext(r.Context())),
			options.RequireDeploymentReadAuthorization,
		)
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
			authorizedDeploymentNameSet(r.Context(), options, requestactor.AuthorizedDeploymentsFromContext(r.Context()), requestactor.AuthorizedProjectEnvironmentScopesFromContext(r.Context())),
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
		if !requireDeploymentReadAuthorization(w, r, options, name) {
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
			authorizedDeploymentNameSet(r.Context(), options, requestactor.AuthorizedDeploymentsFromContext(r.Context()), requestactor.AuthorizedProjectEnvironmentScopesFromContext(r.Context())),
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
				authorizedDeploymentNameSet(r.Context(), options, requestactor.AuthorizedDeploymentsFromContext(r.Context()), requestactor.AuthorizedProjectEnvironmentScopesFromContext(r.Context())),
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
				authorizedDeploymentNameSet(r.Context(), options, requestactor.AuthorizedDeploymentsFromContext(r.Context()), requestactor.AuthorizedProjectEnvironmentScopesFromContext(r.Context())),
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
			if !requireDeploymentReadAuthorization(w, r, options, name) {
				return
			}
			eventLimit, ok := parseOptionalPositiveLimit(w, r)
			if !ok {
				return
			}
			events, err := options.OverviewDeploymentEvents(r.Context(), name)
			if err != nil {
				respondReadLookupError(w, err, "overview deployment events unavailable")
				return
			}
			events = applyDeploymentEventsLimit(events, eventLimit)
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
			if !requireDeploymentReadAuthorization(w, r, options, name) {
				return
			}
			resourceType, resourceName, ok := parseResourceQuery(w, r)
			if !ok {
				return
			}
			resourceLimit, ok := parseOptionalPositiveLimit(w, r)
			if !ok {
				return
			}
			ctx := r.Context()
			containerName := strings.TrimSpace(r.URL.Query().Get("containerName"))
			if containerName != "" {
				ctx = resourcelogs.ContextWithContainerName(ctx, containerName)
			}
			instanceID := strings.TrimSpace(r.URL.Query().Get("instanceId"))
			if instanceID != "" {
				if !validEC2InstanceID(instanceID) {
					http.Error(w, "invalid instanceId query", http.StatusBadRequest)
					return
				}
				ctx = resourcelogs.ContextWithInstanceID(ctx, instanceID)
			}
			clusterARN := ""
			if parsedClusterARN, ok := parseOptionalECSClusterARN(w, r); !ok {
				return
			} else if resourceType == "ecsService" || resourceType == "ecsTask" {
				clusterARN = parsedClusterARN
				if clusterARN != "" {
					ctx = resourcelogs.ContextWithClusterARN(ctx, clusterARN)
				}
			}
			if _, err := options.OverviewDeployment(r.Context(), name); err != nil {
				respondReadLookupError(w, err, "overview deployment unavailable")
				return
			}
			result, err := options.OverviewDeploymentResourceLogs(ctx, name, resourceType, resourceName, resourceLimit)
			if err != nil {
				respondReadLookupError(w, err, "overview deployment resource logs unavailable")
				return
			}
			result = applyResourceLogLimit(result, resourceLimit)
			result.Entries = resourcelogs.AssignEntryIDs(resourcelogs.Request{
				DeploymentName:      name,
				ResourceType:        resourceType,
				ResourceName:        resourceName,
				ResourceDescription: result.ResourceDescription,
				BackingResourceType: result.BackingResourceType,
				BackingResourceName: result.BackingResourceName,
				ControlAccountID:    result.LookupContext.ControlAccountID,
				TargetAccountID:     result.LookupContext.TargetAccountID,
				Region:              result.LookupContext.Region,
				ContainerName:       containerName,
				InstanceID:          instanceID,
				ClusterARN:          clusterARN,
			}, result.Source, result.Entries)
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
			if !requireDeploymentReadAuthorization(w, r, options, name) {
				return
			}
			resourceType, resourceName, ok := parseResourceQuery(w, r)
			if !ok {
				return
			}
			resourceLimit := 0
			if resourceType == "ecsService" || resourceType == "candidateAutoScalingGroup" || resourceType == "serviceAutoScalingGroup" {
				var limitOK bool
				resourceLimit, limitOK = parseOptionalPositiveLimit(w, r)
				if !limitOK {
					return
				}
			}
			ctx := r.Context()
			if instanceID := strings.TrimSpace(r.URL.Query().Get("instanceId")); instanceID != "" {
				if !validEC2InstanceID(instanceID) {
					http.Error(w, "invalid instanceId query", http.StatusBadRequest)
					return
				}
				ctx = resourceinspection.ContextWithInstanceID(ctx, instanceID)
			}
			if clusterARN, ok := parseOptionalECSClusterARN(w, r); !ok {
				return
			} else if (resourceType == "ecsService" || resourceType == "ecsTask") && clusterARN != "" {
				ctx = resourceinspection.ContextWithClusterARN(ctx, clusterARN)
			}
			if _, err := options.OverviewDeployment(r.Context(), name); err != nil {
				respondReadLookupError(w, err, "overview deployment unavailable")
				return
			}
			result, err := options.OverviewDeploymentResourceInspection(ctx, name, resourceType, resourceName, resourceLimit)
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
			if !requireDeploymentReadAuthorization(w, r, options, name) {
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
			if !requireDeploymentReadAuthorization(w, r, options, deploymentName) {
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
		providerQuery := query
		if query.DeploymentName == "" && options.RequireDeploymentReadAuthorization {
			providerQuery.Limit = 0
		}
		result, err := options.AuditLogs(r.Context(), providerQuery)
		if err != nil {
			respondInternalError(w)
			return
		}
		if query.DeploymentName == "" {
			result.Entries = filterAuthorizedAuditEntries(
				result.Entries,
				authorizedDeploymentNameSet(r.Context(), options, requestactor.AuthorizedDeploymentsFromContext(r.Context()), requestactor.AuthorizedProjectEnvironmentScopesFromContext(r.Context())),
				options.RequireDeploymentReadAuthorization,
			)
			if options.RequireDeploymentReadAuthorization {
				result = applyAuditLogLimit(result, query)
			}
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
		if !requireDeploymentWriteAuthorization(w, r, options, name) {
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
		if !requireDeploymentReadAccessWithOptions(w, r, options, record.DeploymentName) {
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
			if !requireDeploymentWriteAuthorization(w, r, options, planRecord.DeploymentName) {
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
			if !requireDeploymentReadAccessWithOptions(w, r, options, record.DeploymentName) {
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
				if !requireDeploymentWriteAuthorization(w, r, options, record.DeploymentName) {
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
		snapshots = filterAuthorizedStatusSnapshots(snapshots, authorizedDeploymentNameSet(r.Context(), options, requestactor.AuthorizedDeploymentsFromContext(r.Context()), requestactor.AuthorizedProjectEnvironmentScopesFromContext(r.Context())), options.RequireDeploymentReadAuthorization)
		snapshots = assignManagedDriftIDsToSnapshots(snapshots)
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
		if !requireDeploymentReadAuthorization(w, r, options, name) {
			return
		}
		snapshot, err := options.DeploymentStatus(r.Context(), name)
		if err != nil {
			respondReadLookupError(w, err, "deployment status unavailable")
			return
		}
		snapshot.Status = assignManagedDriftIDsToStatus(snapshot.Status)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(snapshot)
	})

	handler := http.Handler(mux)
	handler = panicRecoveryMiddleware(logger, handler)
	handler = requestTimeoutMiddleware(options.RequestTimeout, handler)
	handler = requestLoggingMiddleware(logger, handler)
	handler = requestActorMiddleware(options.TrustAuthenticatedHeaders || options.RequireAuthenticatedIdentity || options.RequireOperatorRoleForRead || options.RequireOperatorRoleForWrite || options.RequireDeploymentReadAuthorization, authSession, handler)
	handler = requestIDMiddleware(handler)
	return handler
}
