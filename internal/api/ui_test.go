package api

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

func TestEmbeddedUIAvoidsInlineEventHandlers(t *testing.T) {
	body := string(indexHTML)
	inlineHandler := regexp.MustCompile(`(?i)<[a-z][^>]*\son[a-z]+\s*=`)
	if match := inlineHandler.FindString(body); match != "" {
		t.Fatalf("embedded UI must not use inline event handlers; found %q", match)
	}
}

func TestEmbeddedUIMobileAccountHeadersStack(t *testing.T) {
	body := string(indexHTML)
	contract := `.hero, .row-top, .account-head, .deployment-group-head { flex-direction: column; align-items: stretch; }`
	if !strings.Contains(body, contract) {
		t.Fatalf("expected mobile account/group headers to share stacking contract %q", contract)
	}
}

func TestEmbeddedUITopLevelEmptyStatesAreStatusRegions(t *testing.T) {
	body := string(indexHTML)

	contracts := []string{
		`<div class="loading" role="status" aria-live="polite" aria-atomic="true">Loading applications overview…</div>`,
		`const renderApplicationsEmptyState = \(visibleApplications, sourceApplications\) => \{[\s\S]*?const empty = createElement\('div', 'empty'\);[\s\S]*?empty\.setAttribute\('role', 'status'\);[\s\S]*?empty\.setAttribute\('aria-live', 'polite'\);[\s\S]*?empty\.setAttribute\('aria-atomic', 'true'\);[\s\S]*?createElement\('h2', 'account-name', 'Filters hid every application'\)[\s\S]*?createElement\('h2', 'account-name', 'No visible applications'\)`,
		`const renderEmptyState = \(message\) => \{[\s\S]*?const empty = createElement\('div', 'empty', message\);[\s\S]*?empty\.setAttribute\('role', 'status'\);[\s\S]*?empty\.setAttribute\('aria-live', 'polite'\);[\s\S]*?empty\.setAttribute\('aria-atomic', 'true'\);[\s\S]*?document\.getElementById\('accounts'\)\.replaceChildren\(empty\);`,
		`const renderMissingRouteParameterState = \(pageLabel, missingParam\) => \{[\s\S]*?const empty = createElement\('div', 'empty'\);[\s\S]*?empty\.setAttribute\('role', 'status'\);[\s\S]*?empty\.setAttribute\('aria-live', 'polite'\);[\s\S]*?empty\.setAttribute\('aria-atomic', 'true'\);[\s\S]*?createNavLink\(buildPageURL\('applications'\), 'Back to applications', '_self'\)[\s\S]*?\$\{pageLabel\} bookmark is missing required context[\s\S]*?\$\{missingParam\} query parameter`,
	}
	for _, contract := range contracts {
		if !regexp.MustCompile(contract).MatchString(body) {
			t.Fatalf("expected embedded UI empty-state status contract %s", contract)
		}
	}
}

func TestEmbeddedUIResourceLazyPanelStatesAreStatusRegions(t *testing.T) {
	body := string(indexHTML)

	for _, fragment := range []string{
		"const markPoliteStatusNode = (node) => {",
		"node.setAttribute('role', 'status');",
		"node.setAttribute('aria-live', 'polite');",
		"node.setAttribute('aria-atomic', 'true');",
		"const markResourcePanelStatusNode = (node) => {",
	} {
		if !strings.Contains(body, fragment) {
			t.Fatalf("expected embedded UI lazy-panel status helper fragment %q", fragment)
		}
	}

	contracts := []string{
		`const renderResourceLogsState = \(container, text, className = '', beforeNodes = \[\], afterNodes = \[\], actions = \[\]\) => \{[\s\S]*?const stateNode = createElement\('div', 'detail-state', text\);[\s\S]*?markResourcePanelStatusNode\(stateNode\);[\s\S]*?container\.replaceChildren\(heading, \.\.\.beforeNodes, stateNode, \.\.\.afterNodes\);`,
		`const renderResourceInspectionState = \(container, text, className = '', beforeNodes = \[\], afterNodes = \[\], actions = \[\]\) => \{[\s\S]*?const stateNode = createElement\('div', 'detail-state', text\);[\s\S]*?markResourcePanelStatusNode\(stateNode\);[\s\S]*?container\.replaceChildren\(heading, \.\.\.beforeNodes, stateNode, \.\.\.afterNodes\);`,
	}
	for _, contract := range contracts {
		if !regexp.MustCompile(contract).MatchString(body) {
			t.Fatalf("expected embedded UI lazy-panel status contract %s", contract)
		}
	}
}

func TestEmbeddedUICopyControlsAnnounceLabelChangesPolitely(t *testing.T) {
	body := string(indexHTML)

	contracts := []string{
		"const markCopyStatusButton = (button) => {",
		"const status = createElement('span', 'copy-status-label', button.textContent || '');",
		"status.setAttribute('role', 'status');",
		"status.setAttribute('aria-live', 'polite');",
		"status.setAttribute('aria-atomic', 'true');",
		"button.replaceChildren(status);",
		"return status;",
		"const copyReadRouteToClipboard = (routeText) => {\n        return copyTextToClipboard(routeText);",
		"const createCopyReadRouteButton = (routeText, label = 'Copy read route') => {",
		"const button = createElement('button', 'toolbar-button copy-read-route', label);",
		"const status = markCopyStatusButton(button);",
		"button.setAttribute('aria-label', `Copy ${routeText}`);",
		"copyReadRouteToClipboard(routeText)",
		"const createCopyLinkButton = (href, label = 'Copy link', ariaLabel = '') => {",
		"const button = createElement('button', 'toolbar-button copy-link', label);",
		"copyTextToClipboard(href)",
		"status.textContent = 'Copied';",
		"status.textContent = 'Copy failed';",
		"status.textContent = label;",
		"currentPageSectionURL(sectionID),",
		"'Copy section link',",
		"`Copy ${label || 'resource detail'} section link`,",
		"createCopyLinkButton(logAnchorURL, 'Copy log entry link', 'Copy log entry share link')",
	}
	for _, contract := range contracts {
		if !strings.Contains(body, contract) {
			t.Fatalf("expected embedded UI copy-control live status contract %q", contract)
		}
	}
}

func TestEmbeddedUIResourceUnavailableAndEmptyStateReleaseGuard(t *testing.T) {
	script := embeddedUIScriptForTest(t)

	type stateMapping struct {
		name                 string
		requiredFragments    []string
		forbiddenRawFragment string
	}

	releaseGuardedStates := []stateMapping{
		{
			name: "recent logs local or no AWS unavailable state keeps machine fields and friendly copy",
			requiredFragments: []string{
				"if (!result?.available) {",
				"result?.unavailableReason",
				"resourceLogsRequestIDFields(result)",
				"resourceLogsRequestRouteFields(deploymentName, resource, selectedLimit)",
				"resourceLogsResultScopeFields(result?.resultScope)",
				"createResourceLogsLookupContextGrid(result?.lookupContext",
				"Recent logs are unavailable for this resource.",
				"case 'aws_unavailable':",
				"AWS access is currently unavailable for recent logs. Capture the Tenet request ID and lookup path, then verify credentials, region/account placement, observer-role reachability, and log-source permissions.",
			},
		},
		{
			name: "recent logs successful empty state keeps emptyReason machine field behind friendly copy",
			requiredFragments: []string{
				"if (!entries.length) {",
				"result?.emptyReason",
				"resourceLogsResultScopeFields(result?.resultScope)",
				"createResourceLogsLookupContextGrid(result?.lookupContext",
				"No recent logs are available for this resource.",
				"case 'no_recent_events':",
				"Tenet reached the recent-log provider, but no events were found in the selected bounded window.",
				"case 'no_formatted_entries':",
				"Tenet found provider records, but none could be formatted into operator-visible recent-log rows.",
				"No rows matched the current Recent logs window. Increase the Log limit, refresh after deployment activity, or use the lookup context above before escalating.",
			},
			forbiddenRawFragment: "createElement('div', 'detail-condition-meta', result?.emptyReason)",
		},
		{
			name: "live inspection local or no AWS unavailable state keeps machine fields and friendly copy",
			requiredFragments: []string{
				"if (!result?.available) {",
				"result?.unavailableReason",
				"resourceInspectionRequestIDFields(result)",
				"resourceInspectionRequestRouteFields(deploymentName, resource, selectedLimit)",
				"resourceInspectionResultScopeFields(result?.resultScope)",
				"createLookupContextGrid(result?.lookupContext, inspectionFields)",
				"Live inspection is unavailable for this resource.",
				"case 'aws_unavailable':",
				"AWS access is currently unavailable for live inspection. Capture the Tenet request ID and inspection path, then verify credentials, region/account placement, observer-role reachability, and describe permissions.",
			},
		},
		{
			name: "live inspection successful empty details state keeps route metadata without raw codes",
			requiredFragments: []string{
				"const inspectionFields = [",
				"resourceInspectionRequestIDFields(result)",
				"resourceInspectionRequestRouteFields(deploymentName, resource, selectedLimit)",
				"resourceInspectionResultScopeFields(result?.resultScope)",
				"createResourceInspectionLookupContextNodes(result?.lookupContext)",
				"No live inspection details are available for this resource yet.",
			},
			forbiddenRawFragment: "unavailableReason || emptyReason",
		},
	}

	for _, state := range releaseGuardedStates {
		t.Run(state.name, func(t *testing.T) {
			for _, fragment := range state.requiredFragments {
				if !strings.Contains(script, fragment) {
					t.Fatalf("expected embedded UI release guard fragment %q", fragment)
				}
			}
			if state.forbiddenRawFragment != "" && strings.Contains(script, state.forbiddenRawFragment) {
				t.Fatalf("embedded UI should not render raw machine-code fragment %q", state.forbiddenRawFragment)
			}
		})
	}
}

func TestEmbeddedUIRuntimeEventStatesAreStatusRegions(t *testing.T) {
	body := string(indexHTML)

	contracts := []string{
		`const renderDeploymentEventsState = \(section, deploymentName, text, className = '', detailText = '', isLoading = false, limit = currentPageState.eventLimit\) => \{[\s\S]*?const stateNode = createElement\('div', 'detail-state', text\);[\s\S]*?markPoliteStatusNode\(stateNode\);[\s\S]*?section\.replaceChildren\(heading, intro, stateNode\);`,
		`if \(!events\.length\) \{[\s\S]*?const stateNode = createElement\('div', 'detail-state', deploymentEventsEmptyStateText\(result, selectedLimit, selectedSource\)\);[\s\S]*?markPoliteStatusNode\(stateNode\);[\s\S]*?section\.replaceChildren\(heading, intro, \.\.\.\(scopeSummary \? \[scopeSummary\] : \[\]\), \.\.\.\(freshnessMeta \? \[freshnessMeta\] : \[\]\), \.\.\.\(sourceSummaryChips \? \[sourceSummaryChips\] : \[\]\), stateNode\);`,
	}
	for _, contract := range contracts {
		if !regexp.MustCompile(contract).MatchString(body) {
			t.Fatalf("expected runtime-events status-region contract %s", contract)
		}
	}
}

func TestEmbeddedUIDriftEmptyStatesAreStatusRegions(t *testing.T) {
	body := string(indexHTML)

	contract := `if \(!filteredRows\.length\) \{[\s\S]*?const emptyMessage = driftRows\.length[\s\S]*?const stateNode = createElement\('div', 'detail-state', emptyMessage\);[\s\S]*?markPoliteStatusNode\(stateNode\);[\s\S]*?driftTableWrap\.replaceChildren\(stateNode\);[\s\S]*?return;`
	if !regexp.MustCompile(contract).MatchString(body) {
		t.Fatalf("expected drift empty states to reuse polite status-region helper")
	}
}

func TestEmbeddedUIRendersDeploymentEventSourceSummaryChipsSafely(t *testing.T) {
	script := embeddedUIScriptForTest(t)
	for _, want := range []string{
		"const deploymentEventsSourceSummaryEntries = (result) => {",
		"const createDeploymentEventsSourceSummaryChips = (result) => {",
		"deployment-events-source-summary",
		"deployment-events-source-chip",
		"Runtime event source summary",
		"`Source mix · ${source} · ${deploymentEventsCountText(count)}`",
		"const sourceSummaryChips = createDeploymentEventsSourceSummaryChips(result);",
		"...(sourceSummaryChips ? [sourceSummaryChips] : [])",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("expected embedded UI source-summary chip contract %q", want)
		}
	}
}

func TestEmbeddedUIRendersDeploymentEventNavigationSafely(t *testing.T) {
	script := embeddedUIScriptForTest(t)
	for _, want := range []string{
		"const createDeploymentEventsNavigation = (deploymentName, eventRows, limit, source) => {",
		"detail-callout deployment-events-navigation",
		"Runtime event navigation",
		"`Newest visible event · ${textOrDash(newest.event?.timestamp)} · ${newest.summaryText}`",
		"`Oldest visible event · ${textOrDash(oldest.event?.timestamp)} · ${oldest.summaryText}`",
		"createNavLink(runtimeEventAnchorURL(deploymentName, newest.eventID, limit, source), 'Jump to newest event', '_self')",
		"createNavLink(runtimeEventAnchorURL(deploymentName, oldest.eventID, limit, source), 'Jump to oldest visible event', '_self')",
		"const eventRows = events.map((event) => {",
		"const eventNavigation = createDeploymentEventsNavigation(deploymentName, eventRows, selectedLimit, selectedSource);",
		"...(eventNavigation ? [eventNavigation] : [])",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("expected embedded UI event-navigation contract %q", want)
		}
	}
}

func TestEmbeddedUIRendersOperatorContextBannerSafely(t *testing.T) {
	body := string(indexHTML)

	for _, fragment := range []string{
		"id=\"operator-context\"",
		"id=\"operator-context-status\" role=\"status\" aria-live=\"polite\" aria-atomic=\"true\"",
		".operator-context-live { position: absolute;",
		"liveStatus.setAttribute('role', 'status');",
		"liveStatus.setAttribute('aria-live', 'polite');",
		"liveStatus.setAttribute('aria-atomic', 'true');",
		"statusAnnouncement = `Operator context ready: ${summary.textContent}`;",
		"statusAnnouncement = 'Refreshing trusted front-door context…';",
		"statusAnnouncement = `Operator context refresh failed: ${failureMessage}`;",
		"const operatorContextRoute = () => '/v1/operator/context';",
		"const operatorContextReadRoute = () => 'GET /v1/operator/context';",
		"const operatorContextIdentityProvenanceLabel = (context) =>",
		"Identity header sources:",
		"compatibility X-User-*",
		"managed_session_cookie: 'Tenet-managed browser session cookie'",
		"const operatorContextSupportedIdentityHeadersLabel = (context) => {",
		"identityHeaderSupport",
		"Supported identity headers: canonical ${canonicalText}; compatibility aliases ${aliasText}. Canonical X-Tenet-Authenticated-* identity headers are preferred when both canonical and alias values are present.",
		"operatorContextSupportedIdentityHeadersLabel(context)",
		"const operatorContextIdentityLabel = (context) => {",
		"const operatorContextRoleNames = (roles, field) => Array.isArray(roles?.[field]) ? roles[field].filter(Boolean) : [];",
		"const operatorContextRoleLabel = (context) => {",
		"const operatorContextRecognizedRolesLabel = (context) => {",
		"Recognized capability roles:",
		"supported roles are ${supportedText}",
		"const operatorContextUnsupportedRolesLabel = (context) => {",
		"Unsupported forwarded roles ignored for capability checks:",
		"for read capability; forward operator or admin for write capability.",
		"const operatorContextUnsupportedRolesClass = (context) =>",
		"const operatorContextDeploymentAccessLabel = (context) => {",
		"const operatorContextProjectEnvironmentScopesLabel = (context) => {",
		"const operatorContextProjectEnvironmentScopesClass = (context) => {",
		"const operatorContextScopedWriteReminderLabel = (context) => {",
		"const operatorContextScopedWriteReminderClass = (context) =>",
		"Write scope reminder: deployment-scoped writes can pass with an exact X-Tenet-Authorized-Deployments grant or matching X-Tenet-Authorized-Projects plus X-Tenet-Authorized-Environments catalog placement.",
		"Exact deployment grants:",
		"Project/environment scopes:",
		"Scoped reads require both project and environment headers.",
		"const operatorContextDeploymentAccessSummaryLabel = (context) => {",
		"const operatorContextDeploymentAccessSummaryClass = (context) => {",
		"const operatorContextProjectEnvironmentExamplesLabel = (context) => {",
		"const operatorContextProjectEnvironmentExamplesClass = (context) => {",
		"const operatorContextVisibleDeploymentSourceLabel = (deploymentName, example, access) => {",
		"const operatorContextVisibleDeploymentExamplesLabel = (context) => {",
		"Visible deployment examples:",
		"via exact deployment grant + project/environment scope",
		"via project/environment scope",
		"const operatorContextDesiredStateSourceEntries = (context) => {",
		"Desired-state source:",
		"Controller refresh:",
		"const operatorContextControllerRefreshRoute = () => '/v1/controller/refresh';",
		"const operatorContextControllerRefreshWriteRoute = () => 'POST /v1/controller/refresh';",
		"const operatorContextManualControllerRefreshAvailable = (context) => {",
		"const operatorContextManualControllerRefreshResultLabel = (result) => {",
		"const activeDeploymentForControllerRefreshAudit = () => {",
		"const controllerRefreshAuditURL = () => buildPageURL('audit', {",
		"action: 'controller.refresh.requested'",
		"Last refresh requested ·",
		"controllerRefreshAuditHandoffLabel()",
		"let activeEmbeddedReadModelRefresh = null;",
		"const manualControllerRefreshProcessed = (result) => (",
		"const refreshActiveEmbeddedReadModelAfterManualControllerRefresh = (result) => {",
		"return refreshActiveEmbeddedReadModelAfterManualControllerRefresh(result);",
		"Manual controller refresh route:",
		"const operatorContextDesiredStateReadinessLabel = (context) => {",
		"Desired state:",
		"operatorContextDesiredStateReadinessLabel(context)",
		"operatorContextDesiredStateReadinessClass(context)",
		"Webhook match statuses",
		"Webhook ignored reasons",
		"Diagnostic only; no webhook is sent from this page.",
		"operatorContextDesiredStateSourceEntries(context)",
		"Project/environment visibility:",
		"projectEnvironmentExamples",
		"emptyReason",
		"const operatorContextEnforcementModeLabel = (context) => {",
		"const operatorContextEnforcementSummaryLabel = (context) => {",
		"const operatorContextManagedLogin = (context) => context?.authRuntime?.tenetManagedLogin || {};",
		"const operatorContextEnterpriseSSO = (context) => context?.authRuntime?.enterpriseSso || {};",
		"const operatorContextManagedLoginActive = (context = operatorContextState.context) => {",
		"const operatorContextEnterpriseSSOActive = (context = operatorContextState.context) => {",
		"Authentication boundary: Tenet-managed browser login is active for this API instance; guarded read recovery may offer a same-shell sign-in handoff while trusted front-door headers still take precedence when present.",
		"Authentication boundary: enterprise SSO is active for this API instance; guarded read recovery may offer a provider-neutral same-shell sign-in handoff while trusted front-door headers still take precedence when present.",
		"Authentication boundary: this embedded shell consumes trusted front-door headers only; it does not start an in-page GitHub OAuth login, enterprise SSO login, or Tenet-managed browser session from this page unless the optional login/callback/session routes report active.",
		"operatorContextAuthenticationBoundaryLabel(context)",
		"const operatorContextManagedLoginProviderLabel = (provider) => {",
		"google: 'Google Workspace OIDC'",
		"google_oidc: 'Google Workspace OIDC'",
		"okta_saml: 'Okta SAML'",
		"okta_oidc: 'Okta OIDC'",
		"const operatorContextEnterpriseSSOProviderLabel = (provider) => {",
		"google: 'Google Workspace SSO'",
		"google_oidc: 'Google Workspace SSO'",
		"okta_saml: 'Okta SSO'",
		"okta_oidc: 'Okta SSO'",
		"const operatorContextManagedLoginProviderBoundaryLabel = (context) => {",
		"Managed login provider boundary:",
		"Gmail mailbox access is not requested or authorized by this runtime.",
		"Tenet authorization still comes from trusted roles and deployment grants.",
		"const operatorContextManagedLoginStateLabel = (context) => {",
		"Managed login runtime:",
		"Managed login missing settings:",
		"Managed login storage/session:",
		"Enterprise SSO runtime:",
		"Enterprise SSO missing settings:",
		"Enterprise SSO storage/session:",
		"const operatorContextEnterpriseSSOClass = (context) => {",
		"const operatorContextManagedLoginCookieAttributesLabel = (context) => {",
		"Managed login cookie attributes:",
		"Secure override ${managedLogin.sessionCookieSecureOverride ? 'configured on' : 'not configured'}",
		"local HTTP without TLS or forwarded HTTPS evidence is expected for development",
		"configured secure-cookie override",
		"X-Forwarded-Proto=https",
		"Managed login lifecycle:",
		"Managed login provider token boundary:",
		"Managed login logout boundary:",
		"operatorContextManagedLoginClass(context)",
		"const operatorContextEnforcementClass = (context) => {",
		"Enforcement mode:",
		"Enforcement summary:",
		"trusted_front_door_read_write",
		"local unrestricted",
		"observed only",
		"read/write guarded",
		"const operatorContextAccessVerdictExampleRoutesLabel = (verdict) => {",
		"const visibleRoutes = routes.slice(0, 2).join('; ');",
		"const hiddenCount = routes.length - 2;",
		"const operatorContextAccessVerdictRequiredHeadersLabel = (verdict) => {",
		"const headers = Array.isArray(verdict?.requiredTrustedHeaders) ? verdict.requiredTrustedHeaders.filter(Boolean) : [];",
		"return ` Required trusted headers: ${headers.join('; ')}`;",
		"requiredTrustedHeaders",
		"Required trusted headers:",
		"const operatorContextAccessVerdictScopeGrantLabel = (verdict) => verdict?.acceptsProjectEnvironmentScopes",
		"Scope grants: exact deployments or matching project/environment scopes",
		"acceptsProjectEnvironmentScopes",
		"const operatorContextAccessVerdictCollectionFilterLabel = (verdict) => verdict?.filtersCollectionsByDeploymentAccess",
		"Collection scope: filtered to visible deployments",
		"filtersCollectionsByDeploymentAccess",
		"const operatorContextAccessVerdictEmptyCollectionLabel = (verdict) => verdict?.emptyCollectionLikely",
		"Collection result: likely empty",
		"emptyCollectionLikely",
		"const operatorContextAccessVerdictLabel = (label, verdict) => {",
		"const headers = operatorContextAccessVerdictRequiredHeadersLabel(verdict);",
		"const collectionFilter = operatorContextAccessVerdictCollectionFilterLabel(verdict);",
		"const emptyCollection = operatorContextAccessVerdictEmptyCollectionLabel(verdict);",
		"const scopeGrants = operatorContextAccessVerdictScopeGrantLabel(verdict);",
		"return `${label} ${state}${reason}${examples}${headers}${collectionFilter}${emptyCollection}${scopeGrants}`;",
		"const operatorContextAccessVerdictClass = (verdict) => {",
		"const operatorContextAccessVerdictEntries = (context) => {",
		"exampleRoutes",
		"Examples:",
		"+${hiddenCount} more",
		"operator-context-warning",
		"operator-context-denied",
		"Operator reads",
		"Collection reads",
		"Raw catalog reads",
		"verdicts.catalogRead",
		"emptyCollectionLikely",
		"Deployment access summary unavailable for this response",
		"Deployment access summary unavailable:",
		"forwarded match catalog",
		"unmatchedCount",
		"unmatchedNamesTruncated",
		"unmatchedTruncated || unmatchedNames.length > 3",
		"const operatorContextBoundedListLabel = (values, truncated, limit = 3) => {",
		"const operatorContextStaleScopeDiagnosticsLabel = (context) => {",
		"const operatorContextStaleScopeDiagnosticsClass = (context) =>",
		"Project/environment scope warning:",
		"forwardedProjectCount",
		"forwardedEnvironmentCount",
		"unmatchedProjects",
		"unmatchedEnvironments",
		"X-Tenet-Authorized-Projects and X-Tenet-Authorized-Environments",
		"unmatched",
		"const renderOperatorContextBanner = () => {",
		"const handleManualControllerRefresh = () => {",
		"const loadOperatorContextBanner = ({ refresh = false } = {}) => {",
		"loadOperatorContextBanner().catch(() => {});",
		"loadOperatorContextBanner({ refresh: true });",
		"requestManualControllerRefresh()",
		"operatorWriteAuthRequiredMessage(operatorContextControllerRefreshWriteRoute(), 'Refresh controller')",
		"operatorWriteRoleRequiredMessage(operatorContextControllerRefreshWriteRoute(), 'Refresh controller')",
		"operatorWriteForbiddenMessage(operatorContextControllerRefreshWriteRoute(), 'Refresh controller')",
		"Refresh context",
		"Refresh controller",
		"Requesting refresh…",
		"Last checked ${timestamp.toLocaleString()}",
		"lastCheckedAt: new Date()",
		"Refreshing trusted front-door context…",
		"Last refresh failed:",
		"operator-context-details",
		"let operatorContextDetailsOpen = false;",
		"const operatorContextAccessDetailsPanelID = 'operator-context-access-details-panel';",
		"details.open = operatorContextDetailsOpen;",
		"detailsMeta.setAttribute('role', 'group');",
		"summaryToggle.setAttribute('aria-controls', detailsPanelID);",
		"summaryToggle.setAttribute('aria-expanded', operatorContextDetailsOpen ? 'true' : 'false');",
		"Show access details",
		"Hide access details",
		"operatorContextDetailsOpen = Boolean(details.open);",
		"syncAccessDetailsSummary();",
		"details.append(summaryToggle, detailsMeta);",
		"const operatorContextFailureHeading = (error) => operatorReadFailureHeading(error);",
		"const operatorContextFailureMessage = (error) => {",
		"const message = operatorReadFailureMessage(operatorContextReadRoute(), error);",
		"return message && message !== 'unknown error' ? message : 'Tenet could not load trusted front-door context.';",
		"summary.textContent = operatorContextFailureHeading(operatorContextState.error) ||",
		"Checking trusted front-door context…",
		"No trusted operator identity forwarded",
		"Role header source: ${labels[source] || labels.absent}",
		"Supported Tenet roles: ${supported.join(', ')}",
		"const operatorContextSupportedActorHeadersLabel = (context) => {",
		"Supported actor attribution headers: canonical ${canonicalText}; compatibility aliases ${aliasText}. Explicit X-Tenet-Actor-* actor headers win over X-User-* actor aliases; trusted authenticated identity is projected only when explicit actor context is absent; actor attribution records persisted plan/apply/audit metadata and does not authorize routes.",
		"const operatorContextActorProvenanceLabel = (context) => {",
		"Actor header sources: ${entries.join('; ')}",
		"const operatorContextActorAttributionLabel = (context) => {",
		"Actor attribution: ${status}. ${summary}",
		"operatorContextSupportedActorHeadersLabel(context)",
		"operatorContextActorAttributionLabel(context)",
		"const operatorContextSupportedRolesLabel = (context) => {",
		"const operatorContextSupportedRoleHeadersLabel = (context) => {",
		"Supported role headers: ${supportedHeaders.join(', ')} (role forwarding has no X-User-* alias)",
		"operatorContextSupportedRoleHeadersLabel(context)",
		"operatorContextSupportedRolesLabel(context)",
		"const operatorContextSupportedDeploymentAccessHeadersLabel = (context) => {",
		"Supported deployment access headers: ${supportedHeaders.join(', ')}",
		"operatorContextSupportedDeploymentAccessHeadersLabel(context)",
		"const operatorContextRepositoryPolicy = (context) => context?.repositoryPolicy || {};",
		"const operatorContextRepositoryPolicyModeActive = (context = operatorContextState.context) =>",
		"const operatorContextRepositoryPolicySourceLabel = (context) => {",
		"Repository policy source:",
		"const operatorContextRepositoryPolicyStatusLabel = (context) => {",
		"Repository policy status:",
		"Repository policy unavailable:",
		"const operatorContextRepositoryPolicyMatchedSubjectsLabel = (policy, field, label) => {",
		"Repository policy matched users",
		"Repository policy matched groups",
		"const operatorContextRepositoryPolicyGrantsLabel = (context) => {",
		"Repository policy grants:",
		"const operatorContextRepositoryPolicyGroupHeadersLabel = (context) => {",
		"Supported repository policy group headers:",
		"operatorContextRepositoryPolicySourceLabel(context)",
		"operatorContextRepositoryPolicyStatusLabel(context)",
		"operatorContextRepositoryPolicyMatchedSubjectsLabel(policy, 'matchedUsers', 'Repository policy matched users')",
		"operatorContextRepositoryPolicyMatchedSubjectsLabel(policy, 'matchedGroups', 'Repository policy matched groups')",
		"operatorContextRepositoryPolicyGrantsLabel(context)",
		"operatorContextRepositoryPolicyGroupHeadersLabel(context)",
		"canonical X-Tenet-Authenticated-Roles",
		"operatorContextRoleProvenanceClass",
		"No viewer/operator/admin role forwarded",
		"No exact deployment grants or project/environment scopes forwarded",
		"Operator context unavailable",
		"Operator context check failed",
	} {
		if !strings.Contains(body, fragment) {
			t.Fatalf("expected embedded UI to include %q", fragment)
		}
	}

	for _, unsafe := range []string{
		"<section class=\"operator-context\" id=\"operator-context\" aria-live=\"polite\"",
		"operatorContext.innerHTML",
		"operator-context.innerHTML",
		"repositoryPolicy.innerHTML",
		"operatorContextRepositoryPolicy.innerHTML",
		"matchedUsers.innerHTML",
		"matchedGroups.innerHTML",
	} {
		if strings.Contains(body, unsafe) {
			t.Fatalf("embedded operator context banner should avoid unsafe rendering with %q", unsafe)
		}
	}
}

func TestEmbeddedUIIncludesQueuedAppliesMonitorContracts(t *testing.T) {
	body := string(indexHTML)

	for _, fragment := range []string{
		"const validPages = new Set(['applications', 'application', 'account', 'resource', 'drift', 'plan', 'audit', 'applies']);",
		"const appliesMonitorViews = ['queued', 'active', 'stale'];",
		"const normalizeAppliesMonitorView = (view) => appliesMonitorViews.includes(String(view || '').toLowerCase()) ? String(view || '').toLowerCase() : 'queued';",
		"if (normalized === 'stale') return '/v1/applies?status=active&claim=stale';",
		"const activeAppliesReadRoute = () => appliesMonitorReadRoute('active');",
		"const staleAppliesReadRoute = () => appliesMonitorReadRoute('stale');",
		"const fetchAppliesMonitor = (view = 'queued') => fetchEmbeddedReadRouteJSON(appliesMonitorRoutePath(view), {",
		"return 'Applies monitor';",
		"return appliesMonitorReadRoute(pageState?.appliesView || 'queued');",
		"appliesView: normalizeAppliesMonitorView(params.get('appliesView') || params.get('applyView') || ''),",
		"renderQueuedAppliesPage(appliesState);",
		"const appliesMonitorSummaryMetrics = (records) => {",
		"['Lifecycle signals', formatNonZeroCountParts([",
		"['Operation signals', operations.total ? `${terminalOperations}/${operations.total} terminal · ${operations.running} running · ${operations.queued} queued` : 'No stored apply operations yet'],",
		"['Claim signals', formatNonZeroCountParts([",
		"renderOperatorReadFailurePage(embeddedPageTitle(currentPageState.page), buildEmbeddedPageReadRoute(currentPageState), error);",
		"createNavLink(buildPageURL('plan', { deployment: record.deploymentName, applyId: record.id }), 'Open plan/apply workspace', '_self')",
		"['Operation progress', applyOperationProgressSummary(record.operations || []), 'summary']",
		"const applyExecutionReadinessRows = (record, readinessOverride = null) => {",
		"['Execution readiness', applyExecutionReadinessLabel(record, readinessOverride), 'summary']",
		"No backend preflight readiness evidence is attached yet. Use claim/lifecycle metadata and local-safe docs before treating this as ready for live AWS execution.",
		"['Claim status', applyClaimFreshnessSummary(record, { inferLegacyStale: true }), 'summary']",
		"persisted claim expiry metadata is missing; Tenet inferred this legacy claim is stale from claimedAt",
		"Pending executor claim — no local-safe executor has claimed this apply request yet.",
		"heartbeat freshness looks current",
		"heartbeat or lease is stale or expired",
		"const createAppliesMonitorRefreshButton = (state) => createRefreshButton('Refresh applies'",
		"const createAppliesMonitorRecoveryActions = (state = {}) => {",
		"createCopyReadRouteButton(readRoute)",
		"createNavLink(buildPageURL('applications'), 'Applications overview', '_self')",
		"const createAppliesMonitorViewControl = (state) => {",
		"['queued', 'active', 'stale'].forEach((view) => {",
		"view === 'stale' ? 'Stale claims'",
		"Monitor view: ${appliesMonitorViewLabel(state.view || currentPageState.appliesView || 'queued')}",
		"setAppliesMonitorView(view);",
		"syncAppliesMonitorPageState();",
		"Create or reopen a plan/apply workspace from an application when operators expect a local-safe handoff",
		"Active work may already be terminal, or queued records may still be waiting for an executor claim",
		"This is the healthy state for the recovery queue",
		"['Last heartbeat', applyClaimFreshnessValue(record, 'lastHeartbeatAt')]",
		"['Claim expires', applyClaimFreshnessValue(record, 'expiresAt')]",
		"const appliesMonitorExecutionPreviewState = (state, applyId) => state?.executionPreviews?.[applyId]",
		"Use Preview next execution action to read ${route} without leaving this monitor or changing the bookmarkable applies view.",
		"Preview next execution action",
		"Apply execution preview JSON",
		"Copy execution preview read route",
		"Read-only execution preview loaded from ${route}. It summarizes immutable apply/preflight metadata only; it does not mutate AWS or prove executor progress.",
		"appliesState.onPreviewExecution = (applyId) => {",
	} {
		if !strings.Contains(body, fragment) {
			t.Fatalf("expected queued applies monitor contract fragment %q", fragment)
		}
	}

	contracts := []string{
		`const renderQueuedAppliesState = \(text, className = '', actions = \[\], state = \{\}\) => \{[\s\S]*?const stateNode = createElement\('div', 'empty', text\);[\s\S]*?markPoliteStatusNode\(stateNode\);[\s\S]*?document\.getElementById\('accounts'\)\.replaceChildren\(stateNode\);`,
		`const renderQueuedAppliesPage = \(state\) => \{[\s\S]*?renderQueuedAppliesState\(` + "`" + `Loading \$\{appliesMonitorLoadingLabel\(view\)\} from \$\{readRoute\}…` + "`" + `, '', createAppliesMonitorRecoveryActions\(state\)`,
		`const renderQueuedAppliesPage = \(state\) => \{[\s\S]*?renderQueuedAppliesState\(appliesMonitorEmptyText\(view\), '', \[[\s\S]*?createNavLink\(route, ` + "`" + `\$\{appliesMonitorViewLabel\(view\)\} JSON` + "`" + `\)`,
		`const renderQueuedAppliesPage = \(state\) => \{[\s\S]*?renderQueuedAppliesState\(` + "`" + `\$\{appliesMonitorViewLabel\(view\)\} are temporarily unavailable\.\$\{detail\}` + "`" + `, 'error', createAppliesMonitorRecoveryActions\(state\)`,
		`const renderQueuedAppliesPage = \(state\) => \{[\s\S]*?renderQueuedAppliesState\(` + "`" + `Failed to load \$\{view\} applies: \$\{state\.error\?\.message \|\| 'unknown error'\}` + "`" + `, 'error', \[[\s\S]*?\.\.\.createAppliesMonitorRecoveryActions\(state\)`,
	}
	for _, contract := range contracts {
		if !regexp.MustCompile(contract).MatchString(body) {
			t.Fatalf("expected queued applies monitor status contract %s", contract)
		}
	}
}

func TestEmbeddedUILazyLoadsDeploymentDetailsOnExpandOnly(t *testing.T) {
	body := string(indexHTML)

	for _, fragment := range []string{
		"const detailStateByDeployment = new Map();",
		"const deploymentDetailWaitingContainers = (cached, container) => {",
		"const containers = new Set(cached?.containers || []);",
		"if (cached?.container) containers.add(cached.container);",
		"const renderDeploymentDetailContainers = (containers, render) => {",
		"renderDeploymentDetailContainers(current?.containers, (waitingContainer) => renderDeploymentDetail(waitingContainer, detail));",
		"renderDeploymentDetailContainers(current?.containers, (waitingContainer) => renderDetailState(waitingContainer, `Failed to load deployment details: ${error.message}`, 'error'));",
		"const loadDeploymentDetail = (deploymentName, container) => {",
		"const containers = deploymentDetailWaitingContainers(cached, container);",
		"detailStateByDeployment.set(deploymentName, { ...cached, containers });",
		"renderDeploymentDetail(container, cached.detail);",
		"detailPanel.id = `account-deployment-detail-${sectionIDSegment(account.awsAccountId, 'account')}-${sectionIDSegment(deployment.name, 'deployment')}`;",
		"detailPanel.setAttribute('id', detailPanel.id);",
		"toggle.setAttribute('aria-controls', detailPanel.id);",
		"detailPanel.hidden = true;",
		"loadDeploymentDetail(deployment.name, detailPanel).catch(() => {});",
		"const fetchDeploymentDetail = (deploymentName) => fetchEmbeddedReadRouteJSON(`/v1/overview/deployments/${encodeURIComponent(deploymentName)}`, {",
		"unavailable: 'deployment detail unavailable',",
		"failure: 'detail request failed',",
		"renderDetailState(container, 'Loading deployment details…');",
		"detailStateByDeployment.set(deploymentName, { status: 'ready', detail });",
		"detailStateByDeployment.set(deploymentName, { status: 'loading', promise, containers });",
	} {
		if !strings.Contains(body, fragment) {
			t.Fatalf("expected embedded UI to include %q", fragment)
		}
	}

	for _, fragment := range []string{
		"const toggleExpandedState = () => {",
		"const expanded = toggle.getAttribute('aria-expanded') === 'true';",
		"const nextExpanded = !expanded;",
		"toggle.setAttribute('aria-expanded', String(nextExpanded));",
		"toggle.setAttribute('aria-label', `${nextExpanded ? 'Collapse' : 'Expand'} details for ${deployment.name || 'deployment'}`);",
		"rowHint.textContent = nextExpanded ? 'Collapse details' : 'Expand details';",
		"row.classList.toggle('expanded', nextExpanded);",
		"detailPanel.hidden = !nextExpanded;",
		"loadDeploymentDetail(deployment.name, detailPanel).catch(() => {});",
		"toggle.addEventListener('click', toggleExpandedState);",
		"toggle.addEventListener('keydown', (event) => {",
		"if (event.key !== 'Enter' && event.key !== ' ') {",
		"event.preventDefault();",
		"toggleExpandedState();",
	} {
		if !strings.Contains(body, fragment) {
			t.Fatalf("expected embedded UI to include %q", fragment)
		}
	}
}

func TestEmbeddedUIUsesSafeDetailRenderingAndPreservesFalsyMetrics(t *testing.T) {
	body := string(indexHTML)

	for _, fragment := range []string{
		"const metricValue = (value) => value ?? '—';",
		"const generationValue = (value) => Number.isFinite(value) && value > 0 ? String(value) : '—';",
		"const generationsText = (value) => {",
		"const desired = generationValue(value?.desiredGeneration);",
		"const observed = generationValue(value?.observedGeneration);",
		"return `desired ${desired} · observed ${observed}`;",
		"const changeLabels = (changes) => (changes || []).filter((change) => typeof change === 'string' && change);",
		"const changeLabelsText = (changes) => {",
		"const labels = changeLabels(changes);",
		"return labels.length ? labels.join(', ') : '—';",
		"const runtimeArtifactText = (artifactValue) => {",
		"const value = artifact(artifactValue);",
		"return value === '—' ? '' : value;",
		"const runtimeConditionSummary = (condition) => {",
		"const identity = [type, statusText ? ` = ${statusText}` : ''].join('');",
		"const runtimeConditionsText = (conditions, emptyValue = '—') => {",
		"return labels.length ? labels.join('; ') : emptyValue;",
		"replaceChildren",
		"textContent",
		"Sync reason",
		"Account placement",
		"Runtime generations",
		"Runtime source",
		"Runtime changes",
		"Runtime artifact",
		"Runtime conditions",
		"Runtime history",
		"No recent runtime history is available yet.",
		"const history = detail.history || [];",
		"History timestamp",
		"History summary",
		"History sync",
		"History health",
		"History generations",
		"History changes",
		"History source",
		"History artifact",
		"History conditions",
		"History rollout phase",
		"History rollout step",
		"Rollout generations",
		"Rollout execution",
		"const rolloutExecutionResources = (detail) => {",
		"return execution.length ? execution : canonicalRolloutExecutionFallback(detail);",
		"rolloutExecutionResources(detail).filter((item) => normalizeResource(item).id === resource.id)",
		"Execution resource",
		"Execution description",
		"Rollout steps",
		"Conditions",
	} {
		if !strings.Contains(body, fragment) {
			t.Fatalf("expected embedded UI to include %q", fragment)
		}
	}

	for _, unsafe := range []string{
		"details.innerHTML",
		"detailPanel.innerHTML",
		"conditionList.innerHTML",
		"stepsList.innerHTML",
		"historySection.innerHTML",
		"historyList.innerHTML",
	} {
		if strings.Contains(body, unsafe) {
			t.Fatalf("expected embedded UI to avoid unsafe detail rendering %q", unsafe)
		}
	}

	for _, legacy := range []string{
		"detail.rollout.desiredGeneration || 0",
		"detail.rollout.observedGeneration || 0",
		"detail.observedGeneration || 0",
		"Number.isFinite(value) ? String(value) : '—';",
		"String(deployment.observedGeneration || 0)",
		"change?.label",
	} {
		if strings.Contains(body, legacy) {
			t.Fatalf("expected embedded UI to avoid zero-fallback generation rendering %q", legacy)
		}
	}
}

func TestEmbeddedUIRendersRolloutStepStatusMarkersInDetailList(t *testing.T) {
	body := string(indexHTML)

	for _, fragment := range []string{
		"const rolloutStepStatus = (step, index, steps) => {",
		"switch (step.status) {",
		"case 'completed':",
		"case 'current':",
		"case 'pending':",
		"return step.current ? 'current' : 'pending';",
		"return 'completed';",
		"return 'pending';",
		"const rolloutStepStatusLabel = (status) => {",
		"return 'Completed rollout step';",
		"return 'Current rollout step';",
		"return 'Pending rollout step';",
		"const stepStatus = rolloutStepStatus(step, index, steps);",
		"const stepMeta = createElement('span', `detail-step-meta detail-step-meta-${stepStatus}`, rolloutStepStatusLabel(stepStatus));",
	} {
		if !strings.Contains(body, fragment) {
			t.Fatalf("expected embedded UI to include %q", fragment)
		}
	}

	legacyFallbackFragments := []string{
		"const currentIndex = steps.findIndex((candidate) => candidate.current);",
		"if (currentIndex === -1) return step.current ? 'current' : 'pending';",
		"if (step.current) return 'current';",
		"if (index < currentIndex) return 'completed';",
	}
	for _, fragment := range legacyFallbackFragments {
		if !strings.Contains(body, fragment) {
			t.Fatalf("expected embedded UI to include legacy rollout-step fallback %q", fragment)
		}
	}
}

func TestIndexHTMLIncludesRolloutStepDetailRendering(t *testing.T) {
	body := string(indexHTML)

	for _, fragment := range []string{
		"const stepDetail = textOrDash(step.detail);",
		"const stepDetailRow = createElement('span', 'detail-condition-meta', `Step detail · ${stepDetail}`);",
		"if (step.detail) {",
		"item.append(stepDetailRow);",
		"Step detail · ${stepDetail}",
	} {
		if !strings.Contains(body, fragment) {
			t.Fatalf("expected embedded UI to include %q", fragment)
		}
	}

	for _, unsafe := range []string{
		"stepDetailRow.innerHTML",
		"item.innerHTML",
	} {
		if strings.Contains(body, unsafe) {
			t.Fatalf("expected embedded UI to keep rollout step detail rendering safe for %q", unsafe)
		}
	}
}

func TestEmbeddedUIIncludesQuickLinksToDeploymentReadModels(t *testing.T) {
	body := string(indexHTML)

	for _, fragment := range []string{
		"const nonEmptyResourceSections = (detail) => groupResourceSections(detail).filter((group) => Array.isArray(group?.resources) && group.resources.length > 0);",
		"const createNavLink = (href, text, target = '_blank') => {",
		"const createQuickNavChip = (text, ariaLabel, ariaCurrent) => {",
		"if (ariaCurrent) chip.setAttribute('aria-current', ariaCurrent);",
		"link.target = target;",
		"link.rel = target === '_blank' ? 'noreferrer noopener' : '';",
		"link.href = href;",
		"const detailLinks = createElement('div', 'row-links row-links-shell');",
		"createNavLink(buildPageURL('audit', { deployment: deployment.name }), 'Audit log', '_self')",
		"createNavLink(buildPageURL('plan', { deployment: deployment.name }), 'Plan/apply', '_self')",
		"createNavLink(`/v1/overview/deployments/${encodeURIComponent(deployment.name)}`, 'Overview JSON')",
		"createNavLink(`/v1/status/deployments/${encodeURIComponent(deployment.name)}`, 'Status JSON')",
		"detailLinks.append(",
		"row.append(toggle, detailLinks, detailPanel);",
		"Overview JSON",
		"Status JSON",
		"const workspaceLinks = createElement('div', 'row-links row-links-shell');",
		"Drift view",
		"Plan/apply",
		"Audit log",
		"const allResourceGroups = nonEmptyResourceSections(detail);",
		"const linkedResourceSource = applicationLinkedResourceSourceText(detail);",
		"const normalized = { ...normalizeResource(resource), linkedResourceSource };",
		"return [resource?.type, resource?.name, resource?.description, resource?.linkedResourceSource]",
		"Filter by type, name, description, or source",
		"const resourceGroups = filterApplicationResourceSections(allResourceGroups, resourceFilter);",
		"const applicationResourceGroupTotal = (group) => Number.isFinite(group?.totalResources) ? group.totalResources : (group?.resources || []).length;",
		"const applicationResourceGroupCountText = (group, resourceFilter = '') => {",
		"return `${visibleCount} of ${totalCount} linked resources match resource filter \"${resourceFilter}\"`;",
		"const applicationResourceFilteredEmptyMessage = (group, resourceFilter) =>",
		"createElement('div', 'detail-state', applicationResourceFilteredEmptyMessage(group, resourceFilter))",
		"if (!totalResourceCount) {",
		"'No linked resources are currently reported for this application. Use controller runtime events below to inspect the latest rollout signals.'",
		"emptyStateActions.append(createNavLink(buildPageSectionURL('application', 'application-runtime-events', { deployment: deploymentName }), 'Jump to runtime events', '_self'));",
	} {
		if !strings.Contains(body, fragment) {
			t.Fatalf("expected embedded UI to include %q", fragment)
		}
	}
}

func TestEmbeddedUIIncludesResourceDetailQuickNavigation(t *testing.T) {
	body := string(indexHTML)

	for _, fragment := range []string{
		"const createResourceQuickNav = (detail, resource, sectionLinks = []) => {",
		"const strip = createElement('section', 'quick-nav-strip');",
		"createElement('div', 'quick-nav-title', 'Quick navigation')",
		"createElement('div', 'quick-nav-current', `${deploymentName || 'Application'} · ${resource.name}`)",
		"createElement('div', 'quick-nav-label', 'Application')",
		"createElement('div', 'quick-nav-label', 'Current resource')",
		"createElement('div', 'quick-nav-label', 'Current section')",
		"createElement('div', 'quick-nav-label', 'On this page')",
		"createElement('div', 'quick-nav-label', 'Sibling resources')",
		"createNavLink(buildPageURL('applications'), 'Applications overview', '_self')",
		"createNavLink(buildPageURL('application', { deployment: deploymentName }), 'Back to application', '_self')",
		"createNavLink(buildPageURL('drift', { deployment: deploymentName }), 'Drift view', '_self')",
		"createNavLink(buildPageURL('plan', { deployment: deploymentName }), 'Plan/apply', '_self')",
		"createNavLink(buildPageURL('audit', { deployment: deploymentName }), 'Audit log', '_self')",
		"createQuickNavChip(resource.name, `Current resource: ${resource.name}`, 'location')",
		"selectedResourceScopeEntries(resource).forEach(([label, value]) => {",
		"createQuickNavChip(`${label} · ${value}`, `${label}: ${value}`)",
		"...selectedResourceScopeEntries(resource),",
		"selectedResourceScopeText(resource)",
		"const createSelectedResourceContextCallout = (detail, resource) => {",
		"'Selected resource context'",
		"const createCurrentResourceReadCue = (deploymentName) =>",
		"Current resource read: ${resourceDetailReadRoute(deploymentName)}. Loaded before focused Recent logs and Live inspection reads.",
		"workspace-callout-factors resource-read-route resource-current-read-cue",
		"const createResourceLinkedSourceCue = (detail, resource) =>",
		"workspace-callout-factors resource-linked-source-cue",
		"Linked-resource source: ${resource?.linkedResourceSource || applicationLinkedResourceSourceText(detail)}",
		"resourceInspectionReadRoute(deploymentName, resource)",
		"resourceLogsReadRoute(deploymentName, resource)",
		"const selectedResourceEvidenceCalloutText = (supportsInspection, supportsLogs, routeBits) => {",
		"Use the Live inspection and Recent logs handoffs below to refresh focused evidence for this exact resource selection.",
		"Use the Live inspection handoff below to refresh focused evidence for this exact resource selection.",
		"Use the Recent logs handoff below to refresh focused evidence for this exact resource selection.",
		"No focused evidence handoffs are available for this resource type yet. Use the current resource read above for this selection.",
		"const evidenceLinks = createElement('div', 'row-links selected-resource-context-links');",
		"createNavLink(buildPageSectionURL('resource', 'resource-live-inspection', routeParams), 'Jump to live inspection', '_self')",
		"createNavLink(buildPageSectionURL('resource', 'resource-recent-logs', routeParams), 'Jump to recent logs', '_self')",
		"createSelectedResourceContextCallout(detail, resource)",
		"const resourceTroubleshootingSummaryByResourceKey = new Map();",
		"const renderResourceTroubleshootingSummary = (section, detail, resource) => {",
		"Derived from selected deployment/resource context plus the latest Live inspection and Recent logs panel results.",
		"const resourceTroubleshootingFreshnessPart = (label, state) => {",
		"const resourceTroubleshootingFreshness = (states) => [",
		"['Evidence freshness', resourceTroubleshootingFreshness(states)]",
		"const selectedLookupScope = selectedResourceScopeText(resource);",
		"...(selectedLookupScope ? [['Selected lookup scope', selectedLookupScope]] : []),",
		"resourceTroubleshootingState(states.logsState, resourceTroubleshootingLogsLabel)",
		"resourceTroubleshootingState(states.inspectionState, resourceTroubleshootingInspectionLabel)",
		"refreshResourceTroubleshootingSummary(deploymentName, resource);",
		"const activeSectionID = currentHashTargetID();",
		"createQuickNavChip(currentSectionLink.label, `Current section: ${currentSectionLink.label}`, 'location')",
		"createElement('div', 'quick-nav-empty', 'No section is currently targeted.')",
		"sectionLink?.sectionID !== currentSectionLink?.sectionID",
		"createElement('div', 'quick-nav-empty', 'No additional section links are available for this resource.')",
		"const pushResourceSection = (section, sectionID, label) => {",
		"sectionID,",
		"href: currentPageSectionURL(sectionID),",
		"pushResourceSection(historySection, 'resource-runtime-history', 'Runtime history');",
		"pushResourceSection(troubleshootingSection, 'resource-troubleshooting-summary', 'Troubleshooting summary');",
		"pushResourceSection(executionSection, 'resource-rollout-execution', 'Rollout execution');",
		"pushResourceSection(conditionsSection, 'resource-status-conditions', 'Conditions');",
		"pushResourceSection(inspectionSection, 'resource-live-inspection', 'Live inspection');",
		"const ecsServiceEventAnchorID = (event, index = 0) => {",
		"const eventID = String(event?.eventId || '').trim();",
		"if (eventID) return `ecs-service-event-${sectionIDSegment(eventID, 'unknown')}`;",
		"return `ecs-service-event-${sectionIDSegment(eventID, 'unknown')}`;",
		"return `ecs-service-event-${sectionIDSegment(fallback, 'unknown')}`;",
		"item.id = ecsServiceEventAnchorID(event, index);",
		"if (event?.eventId) {",
		"`Event ID · ${String(event.eventId).trim()}`",
		"const eventShareURL = ecsServiceEventShareURL(event, index);",
		"createNavLink(eventShareURL, 'Share ECS service event', '_self')",
		"createCopyLinkButton(eventShareURL, 'Copy ECS service event link', 'Copy ECS service event share link')",
		"ECS service events were not included in this live-inspection payload.",
		"scrollToCurrentHashTarget();",
		"pushResourceSection(logsSection, 'resource-recent-logs', 'Recent logs');",
		".filter((candidate) => candidate.id !== resource.id);",
		"createNavLink(buildPageURL('resource', {",
		"resourceType: sibling.type,",
		"resourceId: sibling.id,",
		"}), sibling.name, '_self')",
		"'No sibling linked resources are available for this application.'",
		"const resourceDetailFetchedAtText = (detail) => {",
		"const withResourceDetailFetchedAt = (detail) => {",
		"const createResourceDetailFreshnessMeta = (resourceState = null, detail = null) => {",
		"'Resource detail read not refreshed yet'",
		"`Resource detail read refreshed · ${fetchedAt}`",
		"'account-topology resource-detail-freshness'",
		"createResourceDetailFreshnessMeta(resourceState, detail)",
		"const quickNav = createResourceQuickNav(detail, resource, resourceSectionLinks);",
		"page.append(quickNav, troubleshootingSection, shell, historySection, executionSection, conditionsSection, inspectionSection, logsSection);",
		"activeResourceQuickNavSync = typeof quickNav.syncCurrentSection === 'function' ? quickNav.syncCurrentSection : null;",
		"syncActiveResourceQuickNav();",
		"scrollToCurrentHashTarget();",
	} {
		if !strings.Contains(body, fragment) {
			t.Fatalf("expected embedded UI to include %q", fragment)
		}
	}
}

func TestEmbeddedUIIncludesDriftSectionNavigation(t *testing.T) {
	body := string(indexHTML)

	for _, fragment := range []string{
		"const buildPageSectionURL = (page, sectionID, extras = {}) => {",
		"return sectionID ? `${baseURL}#${sectionID}` : baseURL;",
		"const currentHashTargetID = () => {",
		"let activeResourceQuickNavSync = null;",
		"const syncActiveResourceQuickNav = () => {",
		"const focusCurrentHashTarget = (target) => {",
		"target.setAttribute('tabindex', '-1');",
		"target.focus({ preventScroll: true });",
		"const scrollToCurrentHashTarget = () => {",
		"target.scrollIntoView({ block: 'start', behavior: 'auto' });",
		"focusCurrentHashTarget(target);",
		"window.addEventListener('hashchange', () => {",
		"if (currentPageState.page !== 'application' && currentPageState.page !== 'account' && currentPageState.page !== 'resource' && currentPageState.page !== 'drift' && currentPageState.page !== 'plan' && currentPageState.page !== 'audit' && currentPageState.page !== 'applies') return;",
		"syncActiveResourceQuickNav();",
		"const createDeploymentQuickNav = (deploymentName, currentWorkspace, linkedResources = null, sectionLinks = [], currentSectionEmptyLabel = '') => {",
		"const trackCurrentSection = Boolean(currentSectionEmptyLabel) && sectionLinks.some((sectionLink) => sectionLink?.sectionID);",
		"createElement('div', 'quick-nav-label', 'Current section')",
		"createQuickNavChip(currentSectionLink.label, `Current section: ${currentSectionLink.label}`, 'location')",
		"createElement('div', 'quick-nav-empty', currentSectionEmptyLabel)",
		"sectionLink?.sectionID !== currentSectionLink?.sectionID",
		"sectionID: 'drift-runtime-signals',",
		"sectionID: 'drift-rollout-execution',",
		"sectionID: 'drift-field-level',",
		"sectionID: 'drift-status-conditions',",
		"createElement('div', 'quick-nav-label', 'On this page')",
		"buildPageSectionURL('drift', 'drift-runtime-signals', { deployment: deploymentName })",
		"buildPageSectionURL('drift', 'drift-rollout-execution', { deployment: deploymentName })",
		"buildPageSectionURL('drift', 'drift-field-level', { deployment: deploymentName })",
		"buildPageSectionURL('drift', 'drift-status-conditions', { deployment: deploymentName })",
		"runtimeSignalsSection.id = 'drift-runtime-signals';",
		"executionSection.id = 'drift-rollout-execution';",
		"driftSection.id = 'drift-field-level';",
		"conditionsSection.id = 'drift-status-conditions';",
		"scrollToCurrentHashTarget();",
	} {
		if !strings.Contains(body, fragment) {
			t.Fatalf("expected embedded UI to include %q", fragment)
		}
	}
}

func TestEmbeddedUIIncludesApplicationSectionNavigation(t *testing.T) {
	body := string(indexHTML)

	for _, fragment := range []string{
		"const createApplicationQuickNav = (detail, sectionLinks = []) => {",
		"createElement('div', 'quick-nav-label', 'Hierarchy')",
		"createNavLink(buildPageURL('applications'), 'Applications overview', '_self')",
		"createElement('div', 'quick-nav-label', 'Current destination')",
		"createQuickNavChip('Application detail', 'Current destination: Application detail', 'page')",
		"createElement('div', 'quick-nav-current', `${deploymentName || 'Deployment'} · Application detail`),",
		"createElement('div', 'quick-nav-label', 'Current section')",
		"const activeSectionID = currentHashTargetID();",
		"createQuickNavChip(currentSectionLink.label, `Current section: ${currentSectionLink.label}`, 'location')",
		"createElement('div', 'quick-nav-empty', 'No section is currently targeted.')",
		"sectionLink?.sectionID !== currentSectionLink?.sectionID",
		"createElement('div', 'quick-nav-empty', 'No additional section links are available for this application.')",
		"const applicationStatusSectionID = 'application-current-status';",
		"const applicationSectionIDs = {",
		"'Workloads': 'application-workloads',",
		"'Traffic routing': 'application-traffic-routing',",
		"'Config and access': 'application-config-access',",
		"sectionID,",
		"pushApplicationSectionLink(groupSection.id, applicationResourceSectionLinkLabel(group, resourceFilter));",
		"applicationStatusSection.id = applicationStatusSectionID;",
		"pushApplicationSectionLink(applicationStatusSectionID, 'Current application status');",
		"eventsSection.id = 'application-runtime-events';",
		"pushApplicationSectionLink(eventsSection.id, 'Controller runtime events');",
		"const quickNav = createApplicationQuickNav(detail, applicationSectionLinks);",
		"activeResourceQuickNavSync = typeof quickNav?.syncCurrentSection === 'function' ? quickNav.syncCurrentSection : null;",
		"syncActiveResourceQuickNav();",
		"scrollToCurrentHashTarget();",
	} {
		if !strings.Contains(body, fragment) {
			t.Fatalf("expected embedded UI to include %q", fragment)
		}
	}
}

func TestEmbeddedUIIncludesAccountQuickNavigation(t *testing.T) {
	body := string(indexHTML)

	for _, fragment := range []string{
		"const createAccountQuickNav = (account, sectionLinks = []) => {",
		"const accountLabel = account?.name || currentPageState.accountId || 'Account';",
		"createElement('div', 'quick-nav-label', 'Hierarchy')",
		"createNavLink(buildPageURL('applications'), 'Applications overview', '_self')",
		"createElement('div', 'quick-nav-label', 'Current destination')",
		"createQuickNavChip('Account detail', 'Current destination: Account detail', 'page')",
		"createElement('div', 'quick-nav-current', `${accountLabel} · Account detail`),",
		"createElement('div', 'quick-nav-label', 'Current section')",
		"createElement('div', 'quick-nav-label', 'On this page')",
		"createQuickNavChip(currentSectionLink.label, `Current section: ${currentSectionLink.label}`, 'location')",
		"createElement('div', 'quick-nav-empty', 'No section is currently targeted.')",
		"createElement('div', 'quick-nav-empty', 'No additional section links are available for this account.')",
		"const sectionIDSegment = (value, fallback) => {",
		"const accountGroupSectionID = (group) => {",
		"return `account-group-${sectionIDSegment(group.appRef, 'project')}-${sectionIDSegment(group.environmentRef, 'environment')}-${sectionIDSegment(group.region, 'region')}`;",
		"const accountSectionLinks = [];",
		"detailSummary.id = 'account-summary';",
		"const pushAccountSectionLink = (sectionID, label) => {",
		"href: buildPageSectionURL('account', sectionID, { accountId: accountID }),",
		"pushAccountSectionLink(detailSummary.id, 'Account summary');",
		"const accountGroupSectionLabel = (group) => {",
		"return region === '—' ? `${app} · ${environment}` : `${app} · ${environment} · ${region}`;",
		"groupSection.id = accountGroupSectionID(group);",
		"pushAccountSectionLink(groupSection.id, accountGroupSectionLabel(group));",
		"No deployments are currently associated with this account.",
		"Use the applications overview to confirm whether filters hide deployments, or open the account JSON payload to inspect the raw catalog response.",
		"const overviewArticle = createAccountOverviewArticle(account, accountSectionLinks);",
		"const quickNav = createAccountQuickNav(account, accountSectionLinks);",
		"const accountDetailSection = createElement('section', 'detail-section');",
		"createDetailSectionHeader('Account detail', createAccountDetailActions(accountState))",
		"activeResourceQuickNavSync = typeof quickNav?.syncCurrentSection === 'function' ? quickNav.syncCurrentSection : null;",
		"root.replaceChildren(...[quickNav, accountDetailSection, overviewArticle].filter(Boolean));",
		"scrollToCurrentHashTarget();",
		"currentPageState.page !== 'account'",
	} {
		if !strings.Contains(body, fragment) {
			t.Fatalf("expected embedded UI to include %q", fragment)
		}
	}
}

func TestEmbeddedUIIncludesPlanSectionNavigation(t *testing.T) {
	body := string(indexHTML)

	for _, fragment := range []string{
		"const createWorkspaceDetailFreshnessMeta = (detail = null, workspaceLabel = 'Workspace') => {",
		"workspace-detail-freshness",
		"${workspaceLabel} read refreshed · ${fetchedAt}",
		"${workspaceLabel} read not refreshed yet",
		"createWorkspaceDetailFreshnessMeta(detail, workspaceTitle)",
		"const planSectionLinks = [];",
		"const pushPlanSection = (section, sectionID, label) => {",
		"section.id = sectionID;",
		"sectionID,",
		"href: buildPageSectionURL('plan', sectionID, {",
		"planId: currentPageState.planId || planState?.planId || planState?.record?.id || '',",
		"applyId: currentPageState.applyId || applyState?.applyId || applyState?.record?.id || '',",
		"pushPlanSection(runtimeSignalsSection, 'plan-runtime-signals', 'Runtime signals');",
		"pushPlanSection(executionSection, 'plan-rollout-execution', 'Rollout execution');",
		"pushPlanSection(approvalSection, 'plan-approval-guidance', activePlanRecord ? 'Plan preview approval guidance' : 'Derived approval guidance');",
		"pushPlanSection(actionsSection, 'plan-preview-actions', 'Plan preview actions');",
		"pushPlanSection(previewSection, 'plan-stored-preview', 'Stored plan preview');",
		"pushPlanSection(errorsSection, 'plan-preview-errors', 'Plan preview errors');",
		"pushPlanSection(applySection, 'plan-apply-actions', 'Apply request actions');",
		"pushPlanSection(recordSection, 'plan-stored-apply', 'Stored apply request');",
		"pushPlanSection(transitionTimelineSection, 'plan-apply-transition-timeline', 'Apply transition timeline');",
		"pushPlanSection(readinessSection, 'plan-apply-readiness', 'Apply execution readiness');",
		"pushPlanSection(executionPreviewSection, 'plan-apply-execution-preview', 'Apply execution preview');",
		"pushPlanSection(recordDetailsSection, 'plan-apply-details', 'Apply request details');",
		"pushPlanSection(applyOperationsSection, 'plan-apply-operations', 'Apply operations');",
		"pushPlanSection(planOperationsSection, 'plan-operations', 'Plan operations');",
		"const operationAnchorID = (prefix, operation) => {",
		"const previewOperations = (record) => mapRecordOperations(record?.operations || [], 'Plan step', 'operation-plan');",
		"anchorID: operationAnchorID('operation-apply', operation),",
		"row.classList.add('operation-anchor-row');",
		"createNavLink(shareURLForOperation(operation), 'Share operation', '_self')",
		"(mode === 'drift' || mode === 'plan') ? 'No section is currently targeted.' : '',",
		"const quickNav = createWorkspaceQuickNav(detail, mode, planSectionLinks);",
		"activeResourceQuickNavSync = typeof quickNav?.syncCurrentSection === 'function' ? quickNav.syncCurrentSection : null;",
		"header.append(head, quickNav, ...sections);",
		"scrollToCurrentHashTarget();",
	} {
		if !strings.Contains(body, fragment) {
			t.Fatalf("expected embedded UI to include %q", fragment)
		}
	}
}

func TestEmbeddedUIIncludesBreadcrumbChromeForDrillDownPages(t *testing.T) {
	body := string(indexHTML)

	for _, fragment := range []string{
		"const createBreadcrumbCurrentChip = (text, ariaLabel) => {",
		"chip.classList.add('breadcrumb-current');",
		"const accountBreadcrumbLabel = (accountName, awsAccountId) => {",
		"return `${accountName} (${awsAccountId})`;",
		"const createBreadcrumbChrome = (items) => {",
		"const trailItems = (items || []).filter((item) => item?.label);",
		"if (trailItems.length < 2) return null;",
		"const trail = createElement('div', 'breadcrumb-trail');",
		"trail.append(createElement('span', 'breadcrumb-separator', '/'));",
		"head.append(createElement('div', 'breadcrumb-title', 'Breadcrumbs'));",
		"trail.append(createNavLink(item.href, item.label, '_self'));",
		"trail.append(createBreadcrumbCurrentChip(item.label, item.ariaLabel || `Current destination: ${item.label}`));",
		"const createDeploymentBreadcrumb = ({ deploymentName = '', awsAccountId = '', accountName = '', currentLabel = '' } = {}) => {",
		"{ label: 'Applications', href: buildPageURL('applications') },",
		"awsAccountId ? { label: accountBreadcrumbLabel(accountName, awsAccountId), href: buildPageURL('account', { accountId: awsAccountId }) } : null,",
		"deploymentName && activeLabel !== deploymentName ? { label: deploymentName, href: buildPageURL('application', { deployment: deploymentName }) } : null,",
		"const breadcrumb = createDeploymentBreadcrumb({",
		"currentLabel: detail.name || currentPageState.deployment || 'Application',",
		"currentLabel: resource.name,",
		"currentLabel: workspaceBreadcrumbLabel,",
		"currentLabel: 'Audit log',",
		"'Breadcrumbs'",
	} {
		if !strings.Contains(body, fragment) {
			t.Fatalf("expected embedded UI to include %q", fragment)
		}
	}
}

func TestEmbeddedUIIncludesApplicationsOverviewPageArchitecture(t *testing.T) {
	body := string(indexHTML)

	for _, fragment := range []string{
		"const parsePageState = () => {",
		"const page = params.get('page') || params.get('view') || 'applications';",
		"const validPages = new Set(['applications', 'application', 'account', 'resource', 'drift', 'plan', 'audit', 'applies']);",
		"page: validPages.has(page) ? page : 'applications',",
		"accountId: params.get('accountId') || '',",
		"healthState: params.get('health') || 'all',",
		"deployment: params.get('deployment') || '',",
		"planId: params.get('planId') || '',",
		"applyId: params.get('applyId') || '',",
		"resourceType: params.get('resourceType') || '',",
		"resourceId: params.get('resourceId') ?? legacyResourceIdentityParam(params),",
		"auditLimit: normalizeAuditLimit(params.get('limit') || ''),",
		"const buildPageURL = (page, extras = {}) => {",
		"if (currentViewState.query) params.set('q', currentViewState.query);",
		"if (currentViewState.syncState && currentViewState.syncState !== 'all') params.set('sync', currentViewState.syncState);",
		"if (currentViewState.healthState && currentViewState.healthState !== 'all') params.set('health', currentViewState.healthState);",
		"if (currentViewState.account && currentViewState.account !== 'all') params.set('account', currentViewState.account);",
		"if (page === 'audit' && currentPageState.page === 'audit') {",
		"const pathname = window.location.pathname || '/';",
		"return query ? `${pathname}?${query}` : pathname;",
		"const renderApplicationsOverview = (applications, focusState = null) => {",
		"Application status overview",
		"Visible applications after filters:",
		"Visible subset last update",
		"Applications read refreshed ·",
		"createApplicationsOverviewFreshnessMeta(applicationsOverviewState)",
		"const createApplicationsOverviewFreshnessMeta = (overviewState = null) => {",
		"'account-meta applications-overview-freshness'",
		"meta.setAttribute('role', 'status');",
		"meta.setAttribute('aria-live', 'polite');",
		"meta.setAttribute('aria-atomic', 'true');",
		"Account detail",
		"Last update",
		"Search applications",
		"Health",
		"All health states",
		"health ${state.healthState}",
		"healthState: validHealthStates.has(state.healthState) ? state.healthState : 'all',",
		"const matchesHealth = state.healthState === 'all' || healthLabel(deployment.health) === state.healthState;",
		"return matchesAccount && matchesState && matchesHealth && matchesQuery;",
		"currentViewState = { query: '', syncState: 'all', healthState: 'all', account: 'all' };",
		"Open audit log",
		"Applications JSON",
		"/v1/overview/applications",
		"Open account",
		"More actions for ${deployment.name || 'deployment'}",
		"row-primary-action",
		"row-secondary-actions",
		"secondaryActions.setAttribute('role', 'group');",
		"secondaryActions.setAttribute('aria-label', `Secondary actions for ${deployment.name || 'deployment'}`);",
		"application-row-core",
		"['Project', textOrDash(deployment.appRef)]",
		"['Last update', runtimeSummaryTimestamp(deployment.runtimeSummary)]",
		"row-match-context",
		"row-runtime-signal",
		"const latestRuntimeChangeParts = (deployment) => {",
		"const hasRuntimeEventsLinkEvidence = (deployment) => {",
		"const runtimeEventCueFromDeployments = (deployments = []) => {",
		"const createRuntimeEventCue = (deployments = [], scopeLabel = 'Account') => {",
		"Runtime events retained",
		"No recent runtime events",
		"runtime-event-cue-empty",
		"runtime-trend-cue",
		"runtime-trend-strip",
		"const runtimeTrendCueFromDeployments = (deployments = []) => {",
		"const createRuntimeTrendCue = (deployments = [], scopeLabel = 'Account') => {",
		"Runtime trend quiet",
		"No retained runtime history yet",
		"createRuntimeTrendCue(account.deployments || [], 'Account')",
		"createRuntimeTrendCue(group.deployments || [], 'Group')",
		"`Latest event · ${cue.deploymentName}",
		"createRuntimeEventCue(account.deployments || [], 'Account')",
		"createRuntimeEventCue(group.deployments || [], 'Group')",
		"container.append(createElement('strong', '', 'Latest runtime change'));",
		"secondaryLinks.append(createNavLink(buildPageSectionURL('application', 'application-runtime-events', { deployment: deployment.name }), 'Open runtime events', '_self'));",
		"const createApplicationMatchContext = (deployment, query) => {",
		"Matched ${label}: ${value}",
		"createNavLink(buildPageURL('account', { accountId: deployment.awsAccountId }), 'Open account', '_self')",
		"Open application",
		"createNavLink(buildPageURL('application', { deployment: deployment.name }), 'Open application', '_self')",
		"Drift view",
		"createNavLink(buildPageURL('drift', { deployment: deployment.name }), 'Drift view', '_self')",
		"const renderAccountDetailPage = (account, accountState = null) => {",
		"createAccountDetailActions(accountState).forEach((action) => {",
		"toolbar.replaceChildren(toolbarActions);",
		"Account deployment overview",
		"Current account read: ${accountDetailReadRoute(account.awsAccountId)}",
		"account-detail-read-route",
		"Account JSON",
		"Copy account read route",
		"accountLinks.append(createCopyReadRouteButton(accountDetailReadRoute(account.awsAccountId), 'Copy account read route'));",
		"Group JSON",
		"Copy group read route",
		"accountGroupDetailReadRoute(account.awsAccountId, group)",
		"Open application",
		"detailLinks.append(createNavLink(buildPageSectionURL('application', 'application-runtime-events', { deployment: deployment.name }), 'Open runtime events', '_self'));",
		"Back to applications",
		"const groupResourceSections = (detail) => {",
		"const stableResourceIdentity = (resource) => {",
		"const normalizeResource = (resource) => {",
		"const resourceId = stableResourceIdentity({ type, name, description });",
		"Workloads",
		"Traffic routing",
		"Config and access",
		"createNavLink(buildPageURL('resource', { deployment: detail.name, resourceType: resource.type, resourceId: resource.id }), 'Open resource', '_self')",
		"const resourceSupportsLiveInspection = (resource) => {",
		"'codeDeployApplication',\n          'codeDeployDeploymentGroup'",
		"'trafficShiftPlan',\n          'productionListenerRule',\n          'activeTargetGroup',\n          'candidateTargetGroup'",
		"const trafficRoutingIdentifierTokens = (value) => {",
		"const rolloutResourceDetailURL = (detail, resourceTypes, identifiers) => {",
		"const createRolloutResourceDetailLink = (detail, resourceTypes, identifiers) => {",
		"return href ? createNavLink(href, 'Open resource detail', '_self') : null;",
		"const resourceSupportsRecentLogs = (resource) => {",
		"'Evidence cues'",
		"createNavLink(buildPageSectionURL('resource', 'resource-live-inspection', { deployment: deploymentName, resourceType: resource.type, resourceId: resource.id }), 'Live evidence available', '_self')",
		"createNavLink(buildPageSectionURL('resource', 'resource-recent-logs', { deployment: deploymentName, resourceType: resource.type, resourceId: resource.id }), 'Recent logs supported', '_self')",
		"const renderApplicationDetailPage = (detail, applicationState = null, focusState = null) => {",
		"const applicationSectionIDs = {",
		"const createApplicationQuickNav = (detail, sectionLinks = []) => {",
		"const deploymentName = detail.name || currentPageState.deployment || '';",
		"const eventsDeploymentName = deploymentName;",
		"Application resources",
		"eventsSection.id = 'application-runtime-events';",
		"pushApplicationSectionLink(eventsSection.id, 'Controller runtime events');",
		"const quickNav = createApplicationQuickNav(detail, applicationSectionLinks);",
		"createElement('div', 'quick-nav-label', 'Current section')",
		"createElement('div', 'quick-nav-empty', 'No section is currently targeted.')",
		"createElement('div', 'quick-nav-empty', 'No additional section links are available for this application.')",
		"Open drift, plan/apply, or audit log from here before the final resource page.",
		"const eventsSection = createDetailSection('Controller runtime events');",
		"renderDeploymentEventsSection(eventsSection, eventsDeploymentName).catch(() => {});",
		"activeResourceQuickNavSync = typeof quickNav?.syncCurrentSection === 'function' ? quickNav.syncCurrentSection : null;",
		"syncActiveResourceQuickNav();",
		"scrollToCurrentHashTarget();",
		"backLink.target = '_self';",
		"const renderResourceDetailPage = (detail, resourceState = null) => {",
		"const resourceDetailReadRoute = (deploymentName) => `GET /v1/overview/deployments/${deploymentName || '{deploymentName}'}`;",
		"const createResourceDetailActions = (resourceState = null, detail = null, resource = null) => [",
		"createNavLink(`/v1/overview/deployments/${encodeURIComponent(detail?.name || resourceState?.detail?.name || currentPageState.deployment)}`, 'Resource JSON')",
		"Current resource read: ${resourceDetailReadRoute(deploymentName)}. Loaded before focused Recent logs and Live inspection reads.",
		"Linked-resource source: ${resource?.linkedResourceSource || applicationLinkedResourceSourceText(detail)}",
		"Refresh resource detail",
		"Refreshing resource detail from GET /v1/overview/deployments/{deploymentName}…",
		"Current resource detail remains visible below.",
		"Failed to refresh resource detail:",
		"renderResourceDetailPage(resourceState.detail, resourceState);",
		"return fetchDeploymentDetail(deploymentName)",
		"Resource detail",
		"Back to application",
		"Quick navigation",
		"Sibling resources",
		"const currentPageSectionURL = (sectionID) => {",
		"return `${pathname}${search}#${sectionID}`;",
		"const createCopySectionLinkButton = (sectionID, label) => {",
		"Copy section link",
		"const decorateResourceDetailSection = (section, sectionID",
		"createNavLink(href, 'Section link', '_self')",
		"section.dataset.resourceDetailSection = 'true';",
		"decorateResourceDetailSection(section, sectionID, label);",
		"href: currentPageSectionURL(sectionID),",
	} {
		if !strings.Contains(body, fragment) {
			t.Fatalf("expected embedded UI to include %q", fragment)
		}
	}
}

func TestEmbeddedUIIncludesApplicationDetailRoute(t *testing.T) {
	body := string(indexHTML)

	for _, fragment := range []string{
		"const sortAuditEntries = (entries) => (entries || [])",
		"const auditActorStateLabel = (actor) => {",
		"if (actor?.available === true) return 'Recorded';",
		"if (actor?.available === false) return 'Unavailable';",
		"return 'Unknown';",
		"const auditActorReasonText = (actor) => {",
		"const details = [actor.name, actor.email, actor.id].filter((value) => typeof value === 'string' && value);",
		"return details.length ? details.join(' · ') : 'Actor identity was recorded for this request.';",
		"'Actor identity is unavailable for this record.'",
		"'Actor identity is unknown for this record.'",
		"const auditEntryAnchorID = (entry) => {",
		"const backendID = auditAnchorSlug(entry?.entryId);",
		"if (backendID && backendID !== 'unknown') return `audit-entry-${backendID}`;",
		"if (entry?.applyId) return `audit-entry-apply-${auditAnchorSlug(entry.applyId)}`;",
		"const uniqueAuditEntryAnchorID = (entry, seenAnchors) => {",
		"const auditEntryAnchorURL = (entry, anchorID = auditEntryAnchorID(entry)) => buildPageSectionURL('audit', anchorID, {",
		"const auditEntryQuickNavLabel = (entry) => {",
		"const normalizeAuditLimit = (value) => {",
		"{ value: 'plan.created', label: 'Plan preview created (plan.created)' },",
		"{ value: 'apply.requested', label: 'Apply requested (apply.requested)' },",
		"{ value: 'apply.transitioned', label: 'Apply lifecycle transitioned (apply.transitioned)' },",
		"{ value: 'controller.refresh.requested', label: 'Controller refresh requested (controller.refresh.requested)' },",
		"const auditActionLabel = (value) => {",
		"const auditLimitPresets = [1, 2, 5, 10, 20, 50];",
		"const auditLimitSelectOptions = (limit) => {",
		"const nextAuditLimitPreset = (limit) => {",
		"const fetchEmbeddedReadRouteJSON = (route, labels) => fetch(route)",
		"const requestManualControllerRefresh = () => fetch(operatorContextControllerRefreshRoute(), { method: 'POST' })",
		"controller refresh request failed",
		"const requestId = responseRequestID(response);",
		"return response.json().then((value) => attachResponseRequestID(value, requestId));",
		"throw requestError(response.status === 503 ? 'unavailable' : 'fetch-error', message, response.status, requestId);",
		"const operatorReadFailureMessageWithRequestID = (message, error) => {",
		"return `${message} Tenet request ID · ${requestId}`;",
		"const applicationsOverviewReadRoute = () => 'GET /v1/overview/applications';",
		"const createApplicationsOverviewActions = (overviewState = null) => [",
		"createNavLink('/v1/overview/applications', 'Applications JSON'),",
		"const applicationsOverviewFetchedAtText = (overviewState = null) => {",
		"const applicationsOverviewFreshnessLabel = (overviewState = null) => {",
		"const createApplicationsOverviewFreshnessMeta = (overviewState = null) => {",
		"Applications read not refreshed yet",
		"Applications read refreshed ·",
		"lastFetchedAt: new Date()",
		"'Refresh applications overview'",
		"Refresh here after fixing inputs",
		"Open Operator context details",
		"const applicationsOverviewRefreshState = (overviewState = null) => {",
		"'Refreshing applications overview from GET /v1/overview/applications…'",
		"Current applications overview remains visible below.",
		"Failed to refresh applications overview:",
		"const fetchAccountDetail = (accountID) => fetchEmbeddedReadRouteJSON(`/v1/overview/accounts/${encodeURIComponent(accountID)}`, {",
		"unavailable: 'account detail unavailable',",
		"failure: 'account detail request failed',",
		"const accountDetailReadRoute = (accountID) => `GET /v1/overview/accounts/${accountID || '{awsAccountId}'}`;",
		"const accountDetailFetchedAtText = (account) => {",
		"const withAccountDetailFetchedAt = (account) => {",
		"const createAccountDetailFreshnessMeta = (accountState = null, account = null) => {",
		"Account detail read not refreshed yet",
		"Account detail read refreshed ·",
		"const createAccountDetailActions = (accountState = null) => [",
		"'Refresh account detail'",
		"const accountDetailRefreshState = (accountState = null) => {",
		"'Refreshing account detail from GET /v1/overview/accounts/{awsAccountId}…'",
		"Current account detail remains visible below.",
		"Failed to refresh account detail:",
		"const fetchApplicationDetail = (deploymentName) => fetchEmbeddedReadRouteJSON(`/v1/overview/applications/${encodeURIComponent(deploymentName)}`, {",
		"unavailable: 'application detail unavailable',",
		"failure: 'application detail request failed',",
		"const applicationDetailReadRoute = (deploymentName) => `GET /v1/overview/applications/${deploymentName || '{deploymentName}'}`;",
		"const applicationStatusSignal = (detail) => {",
		"const applicationRuntimeEventSummary = (detail) => {",
		"const applicationManagedDriftCounts = (detail) => {",
		"const applicationManagedDriftSummaryText = (detail) => {",
		"No managed drift reported",
		"const createApplicationStatusSummary = (detail, resourceGroups = []) => {",
		"Current application status",
		"Application rollout context:",
		"['Managed drift', applicationManagedDriftSummaryText(detail)],",
		"['Runtime events', applicationRuntimeEventSummary(detail)],",
		"const applicationStatusSection = createApplicationStatusSummary(detail, allResourceGroups);",
		"applicationStatusSection.id = applicationStatusSectionID;",
		"page.append(applicationStatusSection);",
		"pushApplicationSectionLink(applicationStatusSectionID, 'Current application status');",
		"const createApplicationDetailActions = (applicationState = null) => [",
		"createNavLink(`/v1/overview/applications/${encodeURIComponent(applicationState?.detail?.name || currentPageState.deployment)}`, 'Application JSON')",
		"'Refresh application detail'",
		"const applicationDetailRefreshState = (applicationState = null) => {",
		"'Refreshing application detail from GET /v1/overview/applications/{deploymentName}…'",
		"Current application detail remains visible below.",
		"Failed to refresh application detail:",
		"resourceFilter: params.get('resourceFilter') || '',",
		"const applicationResourceFilterMatches = (resource, query) => {",
		"const syncApplicationResourceFilterQueryParam = (value) => {",
		"const createClearApplicationResourceFilterButton = (detail, applicationState = null) => {",
		"createInput('Filter linked resources', currentPageState.resourceFilter, 'Filter by type, name, description, or source',",
		"createElement('div', 'account-topology resource-filter-summary')",
		"const applicationResourceGroupPillText = (group, resourceFilter = '') => {",
		"const applicationLinkedResourceSourceText = (detail) => {",
		"Linked-resource source",
		"Controller-authored rollout execution",
		"Fallback rollout inventory",
		"No ${group.title} resources match resource filter \"${resourceFilter}\". Clear the resource filter to show all ${group.title} resources.",
		"Clear resource filter",
		"const resourceDetailReadRoute = (deploymentName) => `GET /v1/overview/deployments/${deploymentName || '{deploymentName}'}`;",
		"const createResourceDetailActions = (resourceState = null, detail = null, resource = null) => [",
		"createNavLink(`/v1/overview/deployments/${encodeURIComponent(detail?.name || resourceState?.detail?.name || currentPageState.deployment)}`, 'Resource JSON')",
		"'Refresh resource detail'",
		"const resourceDetailRefreshState = (resourceState = null) => {",
		"'Refreshing resource detail from GET /v1/overview/deployments/{deploymentName}…'",
		"Current resource detail remains visible below.",
		"Failed to refresh resource detail:",
		"const syncAuditPageState = ({ clearAuditHash = false } = {}) => {",
		"const hash = clearAuditHash && currentHash.startsWith('#audit-') ? '' : currentHash;",
		"syncAuditPageState({ clearAuditHash: true });",
		"const clearAuditControl = (control) => {",
		"case 'deployment':",
		"case 'action':",
		"case 'limit':",
		"const buildAuditLogsRoute = ({ deploymentName = '', action = '', limit = 0 } = {}) => {",
		"const params = new URLSearchParams();",
		"if (deploymentName) params.set('deployment', deploymentName);",
		"if (action) params.set('action', action);",
		"if (limit > 0) params.set('limit', String(limit));",
		"return params.toString() ? `/v1/audit-logs?${params.toString()}` : '/v1/audit-logs';",
		"const fetchAuditLogs = ({ deploymentName = '', action = '', limit = 0 } = {}) => {",
		"const route = buildAuditLogsRoute({ deploymentName, action, limit });",
		"return fetchEmbeddedReadRouteJSON(route, {",
		"unavailable: 'audit log API unavailable',",
		"failure: 'audit log request failed',",
		"const createAuditLogsIntro = () => {",
		"Current audit read: ${buildEmbeddedPageReadRoute(currentPageState)}",
		"const auditEntryCountLabel = (count) => `${count} entr${count === 1 ? 'y' : 'ies'}`;",
		"const createAuditResultScopeSummary = (result, entries) => {",
		"const scope = result?.resultScope;",
		"['Returned entries', auditEntryCountLabel(returnedCount)],",
		"['Matching entries', auditEntryCountLabel(totalCount)],",
		"['Router limit', limit > 0 ? auditEntryCountLabel(limit) : 'No cap'],",
		"['Window status', auditResultScopeStatusText(scope)],",
		"Applied scope · ${filters.join(' · ')}",
		"audit-result-scope-strip",
		"const createAuditTimelineSummary = (entries) => {",
		"['Visible audit entries', auditEntryCountLabel(visibleEntries.length)],",
		"['Plan previews', String(planCount)],",
		"['Apply requests', String(applyCount)],",
		"['Apply transitions', String(transitionCount)],",
		"['Controller refreshes', String(controllerRefreshCount)],",
		"['Actor recorded', String(actorRecordedCount)],",
		"['Actor unavailable/unknown', String(actorUnavailableCount)],",
		"createAuditTimelineSummary(entries),",
		"const createAuditTimelineNavigation = (auditEntryAnchors) => {",
		"detail-callout audit-timeline-navigation",
		"Audit timeline navigation",
		"`Newest visible entry · ${textOrDash(newest.entry?.timestamp)} · ${auditEntryNavigationSummary(newest.entry)}`",
		"`Oldest visible entry · ${textOrDash(oldest.entry?.timestamp)} · ${auditEntryNavigationSummary(oldest.entry)}`",
		"createNavLink(auditEntryAnchorURL(newest.entry, newest.anchorID), 'Jump to newest entry', '_self')",
		"createNavLink(auditEntryAnchorURL(oldest.entry, oldest.anchorID), 'Jump to oldest visible entry', '_self')",
		"const timelineNavigation = createAuditTimelineNavigation(auditEntryAnchors);",
		"...(timelineNavigation ? [timelineNavigation] : [])",
		"const sourceLabels = [];",
		"if (entry.source?.recordRoute) sourceLabels.push(`Source record · ${entry.source.recordRoute}`);",
		"if (entry.source?.sourceRoute) sourceLabels.push(`Source detail · ${entry.source.sourceRoute}`);",
		"if (entry.metadata?.previousStatus || entry.metadata?.newStatus) metadataLabels.push(`Transition · ${textOrDash(entry.metadata.previousStatus)} → ${textOrDash(entry.metadata.newStatus)}`);",
		"if (Number.isFinite(entry.metadata?.step) && entry.metadata.step > 0) metadataLabels.push(`Transition step · ${entry.metadata.step}`);",
		"if (Number.isFinite(entry.metadata?.currentStep) && entry.metadata.currentStep > 0) metadataLabels.push(`Current step · ${entry.metadata.currentStep}`);",
		"Completed steps · ${Number.isFinite(entry.metadata?.completedSteps) ? entry.metadata.completedSteps : '—'} of ${Number.isFinite(entry.metadata?.totalSteps) ? entry.metadata.totalSteps : '—'}",
		"sourceLabels.join(' · ')",
		"createNavLink(entry.source.recordRoute, 'Open source record')",
		"createNavLink(entry.source.sourceRoute, 'Open source detail')",
		"Inspect audit sources",
		"const auditEntryAnchors = entries.map((entry) => ({",
		"pushAuditSectionLink(anchor.anchorID, anchor.label);",
		"row.setAttribute('id', rowAnchorID);",
		"createNavLink(auditEntryAnchorURL(entry, rowAnchorID), 'Share audit entry', '_self')",
		"Share or continue in app",
		"const syncAuditQueryParams = (params, pageState) => {",
		"const renderAuditPageChrome = () => {",
		"const deploymentName = auditDeploymentName();",
		"? createNavLink(buildPageURL('application', { deployment: deploymentName }), 'Back to application', '_self')",
		": createNavLink(buildPageURL('applications'), 'Back to applications', '_self')",
		"createSelect('Action', currentPageState.auditAction, auditActionOptions, (event) => {",
		"{ value: 'apply.transitioned', label: 'Apply lifecycle transitioned (apply.transitioned)' },",
		"{ value: 'controller.refresh.requested', label: 'Controller refresh requested (controller.refresh.requested)' },",
		"createSelect('Limit', String(currentPageState.auditLimit || ''), auditLimitSelectOptions(currentPageState.auditLimit), (event) => {",
		"Audit filters: none",
		"Audit filters: ${activeFilters.join(' · ')}",
		"const createAuditFilterChip = ({ label, clearLabel, clearControl }) => {",
		"const createAuditFilterChips = () => {",
		"chips.setAttribute('aria-label', 'Active audit controls');",
		"label: `Deployment: ${currentPageState.deployment}`",
		"clearLabel: `Clear deployment filter ${currentPageState.deployment}`",
		"label: `Action: ${auditActionLabel(currentPageState.auditAction)}`",
		"clearLabel: `Clear action filter ${auditActionLabel(currentPageState.auditAction)}`",
		"label: `Limit: latest ${currentPageState.auditLimit}`",
		"clearLabel: `Clear limit cap latest ${currentPageState.auditLimit}`",
		"const createDeploymentQuickNav = (deploymentName, currentWorkspace, linkedResources = null, sectionLinks = [], currentSectionEmptyLabel = '') => {",
		"const createAuditQuickNav = (sectionLinks = []) => createDeploymentQuickNav(",
		"'No section is currently targeted.',",
		"const deploymentContextSectionID = 'audit-deployment-context';",
		"deploymentCallout.setAttribute('id', deploymentContextSectionID);",
		"const timelineSectionID = 'audit-timeline';",
		"timelineSection.setAttribute('id', timelineSectionID);",
		"buildPageSectionURL('audit', sectionID, {",
		"const renderAuditLogsState = (text, className = '', detailText = '', afterNodes = []) => {",
		"const quickNav = createAuditQuickNav(auditSectionLinks);",
		"activeResourceQuickNavSync = typeof quickNav?.syncCurrentSection === 'function' ? quickNav.syncCurrentSection : null;",
		"scrollToCurrentHashTarget();",
		"Loading audit log…",
		"Audit log is currently unavailable.",
		"No audit activity has been persisted yet.",
		"No audit activity has been persisted yet for deployment ${currentPageState.deployment}. Create a plan preview or apply request from the plan/apply workspace to add immutable audit rows for this deployment.",
		"const auditFilterLabels = () => {",
		"labels.push(`action ${auditActionLabel(currentPageState.auditAction)}`);",
		"const auditLimitLabel = () => currentPageState.auditLimit > 0 ? `Limit cap: latest ${currentPageState.auditLimit}` : 'Limit cap: all entries';",
		"const auditLimitCapHintText = (entries, result = null) => {",
		"Showing the latest ${limit}-entry capped audit window. Older matching audit entries may be hidden; increase Limit or clear audit controls before concluding no earlier activity exists.",
		"const createAuditLimitCapActions = (result = null) => {",
		"Expand audit window to ${nextLimit} entries",
		"Clear audit limit cap",
		"setAuditLimitFromTimeline(nextLimit)",
		"audit-limit-cap-hint",
		"const auditControlLabels = () => {",
		"const auditEmptyStateText = () => {",
		"No audit entries are visible with the current limit cap (${auditLimitLabel()}). Increase the limit or clear audit controls to return to the full audit timeline.",
		"No audit entries match the current filters (${filters.join(' · ')}${hasLimit ? `; ${auditLimitLabel()}` : ''}). ${currentPageState.deployment && (currentPageState.auditAction || hasLimit) ? 'Clear action/limit to stay scoped to this deployment, or clear audit controls to remove the deployment scope.' : 'Clear audit controls to return to the full audit timeline.'}",
		"'Clear action/limit'",
		"'Clear audit controls'",
		"Open drift view",
		"Open plan/apply workspace",
		"const contextLinks = createElement('div', 'row-links');",
		"createNavLink(buildPageURL('application', { deployment: deploymentName }), 'Open application drill-down', '_self')",
		"createNavLink(buildPageURL('plan', { deployment: deploymentName }), 'Open plan/apply workspace', '_self')",
		"createNavLink(buildPageURL('application', { deployment: entry.deploymentName }), 'Open application drill-down', '_self')",
		"createNavLink(buildPageURL('drift', { deployment: entry.deploymentName }), 'Open drift view', '_self')",
		"createNavLink(buildPageURL('plan', {",
		"const createAuditLogsActions = (isLoading = false) => {",
		"const auditLogsRoute = buildAuditLogsRoute({",
		"createCopyReadRouteButton(`GET ${auditLogsRoute}`, 'Copy audit read route')",
		"'Refresh audit log'",
		"createNavLink(auditLogsRoute, 'Audit JSON')",
		"'Audit JSON'",
		"const auditLogsRefreshStateText = (auditState) => {",
		"const auditLogsFetchedAtText = (result) => {",
		"const withAuditLogsFetchedAt = (result) => {",
		"const createAuditLogsFreshnessMeta = (result) => {",
		"Last refreshed · ${fetchedAt}",
		"meta.setAttribute('role', 'status');",
		"meta.setAttribute('aria-live', 'polite');",
		"meta.setAttribute('aria-atomic', 'true');",
		"const loadAuditLogsPage = (options = {}) => {",
		"deploymentName: currentPageState.deployment,",
		"action: currentPageState.auditAction,",
		"limit: currentPageState.auditLimit,",
		"const defaultDeploymentEventsLimit = 10;",
		"const normalizeDeploymentEventsLimit = (value) => {",
		"const normalizeDeploymentEventsSource = (value) => {",
		"eventLimit: normalizeDeploymentEventsLimit(params.get('eventLimit') || ''),",
		"eventSource: normalizeDeploymentEventsSource(params.get('eventSource') || ''),",
		"if (key === 'eventLimit') {",
		"if (key === 'eventSource') {",
		"const buildDeploymentEventsReadRoute = (deploymentName, { limit = defaultDeploymentEventsLimit, source = currentPageState.eventSource } = {}) => {",
		"params.set('limit', String(normalizedLimit));",
		"if (normalizedSource) params.set('source', normalizedSource);",
		"const deploymentEventsReadRoute = (deploymentName, limit = currentPageState.eventLimit, source = currentPageState.eventSource) => `GET ${buildDeploymentEventsReadRoute(deploymentName, { limit, source })}`;",
		"const deploymentEventsCacheKey = (deploymentName, limit = currentPageState.eventLimit, source = currentPageState.eventSource) => `${deploymentName || ''}::limit=${normalizeDeploymentEventsLimit(limit)}::source=${normalizeDeploymentEventsSource(source) || 'all'}`;",
		"const fetchDeploymentEvents = (deploymentName, limit = currentPageState.eventLimit, source = currentPageState.eventSource) => fetchEmbeddedReadRouteJSON(buildDeploymentEventsReadRoute(deploymentName, { limit, source }), {",
		"unavailable: 'deployment events unavailable',",
		"failure: 'deployment events request failed',",
		"const createDeploymentEventsIntro = (limit = currentPageState.eventLimit, source = currentPageState.eventSource) => createElement('div', 'detail-section-copy', `Controller-derived runtime events from ${deploymentEventsReadRoute('{deploymentName}', limit, source)}.",
		"use Event window to change how much controller history this panel reads without leaving the application drill-down",
		"const createDeploymentEventsLimitControl = (section, deploymentName, isLoading = false) => createSelect(",
		"'Event window'",
		"const createDeploymentEventsSourceControl = (section, deploymentName, isLoading = false) => createSelect(",
		"'Event source'",
		"const syncApplicationEventSourcePageState = (deploymentName, source) => {",
		"const createDeploymentEventsActions = (section, deploymentName, isLoading = false, result = null) => {",
		"'Refresh runtime events'",
		"() => renderDeploymentEventsSection(section, deploymentName, { force: true }).catch(() => {}),",
		"createCopyReadRouteButton(",
		"deploymentEventsReadRoute(deploymentName, currentPageState.eventLimit, currentPageState.eventSource)",
		"'Copy runtime events route'",
		"const sortTimelineEvents = (events) => (events || [])",
		"const renderDeploymentEventsState = (section, deploymentName, text, className = '', detailText = '', isLoading = false, limit = currentPageState.eventLimit) => {",
		"createDetailSectionHeader('Controller runtime events', createDeploymentEventsActions(section, deploymentName, isLoading))",
		"const deploymentEventsManagedDriftCounts = (result) => {",
		"const eventRows = (result?.events || []).flatMap((event) => Array.isArray(event?.managedDrift) ? event.managedDrift : []);",
		"const deploymentEventsHasManagedDrift = (result) => deploymentEventsManagedDriftCounts(result).total > 0;",
		"const deploymentEventsRetentionScope = (result) => {",
		"const deploymentEventsResultScopeFields = (scope, retention = null) => {",
		"if (!scope && !retention) return [];",
		"['Requested window', scope.requestedLimit == null ? 'All available events' : deploymentEventsScopeLimitText(scope.requestedLimit)],",
		"['Returned events', deploymentEventsCountText(scope.returnedEventCount)],",
		"['Total retained events', deploymentEventsCountText(scope.totalEventCount ?? retention?.retainedEventCount)],",
		"['Retention limit', deploymentEventsScopeLimitText(retention.limit)]",
		"['Retention retained', deploymentEventsRetainedCountText(retention.retainedEventCount)]",
		"['Retention status', deploymentEventsRetentionStatusText(retention.status)]",
		"if (scope?.newestEventTimestamp) fields.push(['Newest event', scope.newestEventTimestamp]);",
		"if (scope?.oldestEventTimestamp) fields.push(['Oldest event', scope.oldestEventTimestamp]);",
		"if (scope) fields.push(['Window status', deploymentEventsTruncationStatusText(scope.truncationStatus)]);",
		"Query-limited response; increase Event window to inspect older returned history",
		"Controller retention cap reached; older runtime history may have been dropped",
		"if (source) {",
		"return `No controller runtime events match source filter \"${source}\" in the latest ${selectedLimit}-event window. Clear Event source or refresh before treating controller history as absent.`;",
		"No controller runtime events are retained for this application yet.",
		"const createDeploymentEventsResultScopeSummary = (result) => {",
		"const withDeploymentEventsFetchedAt = (result) => {",
		"const createDeploymentEventsFreshnessMeta = (result) => {",
		"deployment-events-freshness",
		"Last refreshed · ${fetchedAt}",
		"const shortRevision = (value) => {",
		"const controllerRefreshFields = (event) => {",
		"if (refresh.repository) fields.push(`repo ${refresh.repository}`);",
		"if (revision) fields.push(`revision ${revision}`);",
		"const createControllerRefreshCue = (event) => {",
		"GitHub refresh evidence · ${fields.join(' · ')}",
		"const runtimeEventAnchorBase = (event) => {",
		"const backendID = auditAnchorSlug(event?.eventId);",
		"if (backendID && backendID !== 'unknown') return `runtime-event-${backendID}`;",
		"const uniqueRuntimeEventAnchorID = (event, seenAnchors) => {",
		"const runtimeEventAnchorURL = (deploymentName, eventID, limit = currentPageState.eventLimit, source = currentPageState.eventSource) => buildPageSectionURL('application', eventID, {",
		"const managedDriftAnchorBase = (row) => {",
		"const backendID = auditAnchorSlug(row?.driftId);",
		"return `drift-row-${backendID}`;",
		"const uniqueManagedDriftAnchorID = (row, seenAnchors) => {",
		"const managedDriftAnchorURL = (deploymentName, rowID) => buildPageSectionURL('drift', rowID, {",
		"actions.push(createNavLink(buildPageSectionURL('drift', 'drift-field-level', { deployment: deploymentName }), 'Open field-level drift', '_self'));",
		"const deploymentEventsManagedDriftSummary = (result) => {",
		"return `Managed drift summary · ${counts.total} total · ${counts.high} high · ${counts.medium} medium · ${counts.low} low`;",
		"const renderDeploymentEventsPayload = (section, deploymentName, result, limit = currentPageState.eventLimit) => {",
		"const seenEventAnchors = new Map();",
		"item.setAttribute('id', eventID);",
		"createNavLink(runtimeEventAnchorURL(deploymentName, eventID, selectedLimit, selectedSource), 'Share event', '_self')",
		"No controller runtime events are retained for this application yet.",
		"Controller runtime events are currently unavailable.",
		"Loading controller runtime events…",
		"Failed to load controller runtime events: ${error.message}",
		"const force = Boolean(options?.force);",
		"Rollout phase · ${event.rolloutPhase}",
		"Rollout step · ${event.currentStep}",
		"Event details · ${changeDetailsText(event.changeDetails)}",
		"const buildPlanPreviewRoute = (planId) => `/v1/plans/${encodeURIComponent(planId)}`;",
		"const fetchPlanPreview = (planId) => fetch(buildPlanPreviewRoute(planId))",
		"const buildApplyRequestRoute = (applyId) => `/v1/applies/${encodeURIComponent(applyId)}`;",
		"const fetchApplyRequest = (applyId) => fetch(buildApplyRequestRoute(applyId))",
		"const cloneJSONValue = (value) => {",
		"const buildPlanSourceFingerprintPayload = (detail) => {",
		"const context = effectiveRolloutContext(detail);",
		"if (context.executionMode) payload.executionMode = context.executionMode;",
		"const execution = planSourceExecutionResources(detail);",
		"const derivePlanSourceFingerprint = async (detail) => {",
		"const createPlanPreview = (deploymentName, ifMatch = '') => fetch(`/v1/plans/deployments/${encodeURIComponent(deploymentName)}`, {",
		"const createApplyRequest = (planId) => fetch(`/v1/applies/plans/${encodeURIComponent(planId)}`, {",
		"method: 'POST',",
		"headers: { 'Content-Type': 'application/json' },",
		"body: ifMatch ? JSON.stringify({ ifMatch }) : '{}',",
		"const applicationDetail = currentPageState.page === 'application' ? fetchApplicationDetail(currentPageState.deployment) : fetchDeploymentDetail(currentPageState.deployment);",
		"const applicationDetailFetchedAtText = (detail) => {",
		"const withApplicationDetailFetchedAt = (detail) => {",
		"const createApplicationDetailFreshnessMeta = (applicationState = null, detail = null) => {",
		"Application detail read refreshed · ${fetchedAt}",
		"Application detail read not refreshed yet",
		"Current application read: ${applicationDetailReadRoute(deploymentName)}",
		"meta.setAttribute('role', 'status');",
		"meta.setAttribute('aria-live', 'polite');",
		"meta.setAttribute('aria-atomic', 'true');",
		"createApplicationDetailFreshnessMeta(applicationState, detail)",
		"}).then(withApplicationDetailFetchedAt);",
		"applicationDetail",
		"} else if (currentPageState.page === 'audit') {",
		"loadAuditLogsPage().catch(() => {});",
		"if (currentPageState.page === 'application') {",
		"const renderApplicationWorkspace = () => {",
		"renderApplicationDetailPage(applicationState.detail, applicationState);",
		"if (currentPageState.page === 'resource') {",
		"renderResourceDetailPage(resourceState.detail, resourceState);",
		"if (currentPageState.page === 'drift') {",
		"const renderDriftWorkspace = () => {",
		"renderDeploymentWorkspace(currentPageState.page, workspaceState.detail, null, null, workspaceState);",
		"if (currentPageState.page === 'plan') {",
		"renderDeploymentWorkspace('plan', workspaceDetail, {",
		"syncPlanPageURL(record.id, '');",
		"fetchDeploymentDetail(deploymentName)",
	} {
		if !strings.Contains(body, fragment) {
			t.Fatalf("expected embedded UI to include %q", fragment)
		}
	}
}

func TestEmbeddedUIIncludesOperatorReadAuthGuidanceContract(t *testing.T) {
	body := string(indexHTML)

	for _, fragment := range []string{
		"const operatorReadAuthRequiredState = (error) => error?.kind === 'fetch-error' && Number(error?.status) === 401;",
		"const operatorReadForbiddenState = (error) => error?.kind === 'fetch-error' && Number(error?.status) === 403;",
		"const operatorReadAuthRequiredMessage = (route, context = operatorContextState.context) => {",
		"${signInRuntime.message}; use the sign-in handoff below to authenticate through ${signInRuntime.loginRoute} and return to this same bookmarkable URL.",
		"Tenet does not start a built-in GitHub OAuth login, enterprise SSO login, or Tenet-managed browser session from this page; reopen this same bookmarkable URL through the trusted front door",
		"const operatorReadRoleRequiredMessage = (route) => `Tenet rejected ${route} after recognizing the caller's identity because this operator read route requires a trusted forwarded viewer/operator/admin role. Open Operator context details (#operator-context-access-details-panel) and check roles.provenance.source, roles.canRead, and the forwarded X-Tenet-Authenticated-Roles value; ask the front-door/RBAC admin to forward viewer, operator, or admin, then retry this same bookmarkable page.`;",
		"const operatorReadPolicyForbiddenMessage = (route) => `Tenet rejected ${route} after recognizing the caller's identity, but this operator is not authorized by the repository RBAC policy for that route or deployment.",
		"compare repositoryPolicy.summary, matchedUsers, matchedGroups, repositoryPolicy.roles, repositoryPolicy.deploymentAccess, repositoryPolicy.projectEnvironmentScopes, and the policy source for this route",
		"rbac/policy.json, rbac/policy.yaml, or rbac/policy.yml",
		"const operatorReadForbiddenMessage = (route) => `Tenet rejected ${route} after recognizing the caller's identity, but this operator is not authorized for that route or deployment. Open Operator context details (#operator-context-access-details-panel) and compare deploymentAccess.summary, access verdicts, stale/unmatched deployment/project/environment grants, and the required trusted headers for this route; ask an admin to grant the matching deployment, project, or environment scope, then retry this same bookmarkable page.`;",
		"operatorContextRepositoryPolicyModeActive() ? operatorReadPolicyForbiddenMessage(route) : operatorReadForbiddenMessage(route);",
		"const operatorReadStateAuthRequired = (state) => state?.request === 'load' && state?.status === 'fetch-error' && operatorReadAuthRequiredState(state?.error);",
		"const operatorReadStateForbidden = (state) => state?.request === 'load' && state?.status === 'fetch-error' && operatorReadForbiddenState(state?.error);",
		"const operatorPlanPreviewReadRoute = (planId) => `GET /v1/plans/${planId || 'planId'}`;",
		"const operatorApplyRequestReadRoute = (applyId) => `GET /v1/applies/${applyId || 'applyId'}`;",
		"if (operatorReadStateHeading(planState)) {",
		"return `${operatorReadFailureMessage(operatorPlanPreviewReadRoute(activePlanId), planState.error)} Deployment-derived context is still shown below and Apply Plan stays disabled until a persisted ready preview is available.`;",
		"return `${operatorReadFailureMessage(operatorApplyRequestReadRoute(activeApplyID), applyState.error)} Deployment and plan-derived context are still shown below.`;",
		"const operatorFetchErrorPillLabel = (state) => {",
		"if (operatorReadStateForbidden(state) || operatorWriteForbiddenState(state)) return 'Access denied';",
		"const embeddedPageTitle = (page) => {",
		"return 'Applications overview';",
		"return 'Application detail';",
		"return 'Resource detail';",
		"return 'Drift viewer';",
		"const browserPageTitle = (page) => {",
		"return 'Drift view';",
		"return 'Plan/apply';",
		"const setEmbeddedBrowserTitle = (...parts) => {",
		"document.title = [...titleParts, 'Tenet'].join(' · ');",
		"const setBrowserTitleFromPageState = (pageState = currentPageState) => {",
		"setEmbeddedBrowserTitle(resource.name || pageState.resourceType, pageState.deployment, pageLabel);",
		"const buildEmbeddedPageReadRoute = (pageState = currentPageState) => {",
		"return 'GET /v1/overview/applications';",
		"return `GET /v1/overview/applications/${deploymentName}`;",
		"return `GET /v1/overview/deployments/${deploymentName}`;",
		"return `GET ${buildAuditLogsRoute({",
		"const currentBookmarkableURL = (hashOverride) => {",
		"const operatorContextSignInRuntime = (context = operatorContextState.context) => {",
		"message: `${providerLabel} is active for this API instance`",
		"const operatorContextManagedLoginSignInURL = (context = operatorContextState.context) => {",
		"const operatorContextSignInLabel = (context = operatorContextState.context) => {",
		"const operatorContextEnterpriseSSOLogoutActive = (context = operatorContextState.context) => {",
		"const operatorContextSignOutRuntime = (context = operatorContextState.context) => {",
		"operatorContextEnterpriseSSOLogoutActive(context)",
		"logoutRoute: safeManagedLoginRoute(enterpriseSso.logoutRoute)",
		"const operatorContextSignOutURL = (context = operatorContextState.context) => {",
		"const operatorContextSignOutLabel = (context = operatorContextState.context) => {",
		"Sign out of ${runtime.label} session",
		"const operatorContextSignOutHandoffLabel = (context = operatorContextState.context) => {",
		"does not sign out of upstream ${runtime.label}, trusted front-door sessions, or revoke provider tokens.",
		"Tenet-managed sign-out handoff: clears only the Tenet session cookie and returns to this bookmarkable page; it does not sign out of upstream provider or trusted front-door sessions, and it does not revoke provider tokens.",
		"Sign out of Tenet-managed browser session",
		"params.set('returnUrl', currentBookmarkableURL());",
		"return runtime?.label ? `Sign in with ${runtime.label}` : 'Sign in';",
		"const createOperatorReadFailureActions = (error) => {",
		"const createOperatorWriteAuthRequiredActions = (state) => {",
		"operator-write-recovery-actions",
		"if (!operatorWriteAuthRequiredState(state)) return null;",
		"actionBar.append(createNavLink(managedSignInURL, operatorContextSignInLabel(), '_self'));",
		"if (operatorReadAuthRequiredState(error)) {",
		"Retry current page after front-door sign-in",
		"if (operatorReadForbiddenState(error)) {",
		"Open Operator context details",
		"Retry current page after access update",
		"currentBookmarkableURL(operatorContextAccessDetailsPanelID)",
		"const renderGenericReadFailurePage = (pageLabel, route, error) => {",
		"`Failed to load ${pageLabel.toLowerCase()}`",
		"`Tenet could not load ${route}: ${message}`",
		"`Tenet request ID · ${requestId}`",
		"Retry current page",
		"const renderOperatorReadFailurePage = (pageLabel, route, error) => {",
		"renderGenericReadFailurePage(pageLabel, route, error);",
		"createElement('h2', 'account-name', heading)",
		"createElement('div', 'account-meta', operatorReadFailureMessage(route, error))",
		"createOperatorReadFailureActions(error)",
		"const renderOperatorReadFailurePanel = (container, title, actions, route, error) => {",
		"createElement('div', 'detail-state error', heading)",
		"createElement('div', 'detail-condition-meta', operatorReadFailureMessage(route, error))",
		"const deploymentEventsReadRoute = (deploymentName, limit = currentPageState.eventLimit, source = currentPageState.eventSource) => `GET ${buildDeploymentEventsReadRoute(deploymentName, { limit, source })}`;",
		"createDeploymentEventsActions(section, deploymentName),",
		"deploymentEventsReadRoute(deploymentName, limit, source),",
		"createDeploymentEventsActions(container, deploymentName),",
		"renderAuditLogsState(operatorReadFailureHeading(error), 'error', operatorReadFailureMessage(buildEmbeddedPageReadRoute(currentPageState), error), [createOperatorReadFailureActions(error)]);",
		"renderOperatorReadFailurePage(embeddedPageTitle(currentPageState.page), buildEmbeddedPageReadRoute(currentPageState), error);",
		"const fetchResourcePanelJSON = (route, failureLabel) => Promise.resolve()",
		".then(() => fetch(route))",
		"const resourcePanelRequestIDFields = (result) => {",
		"const createResourcePanelRequestIDMeta = (error) => {",
		"const resourceInspectionRequestIDFields = resourcePanelRequestIDFields;",
		"const createResourceInspectionRequestIDMeta = createResourcePanelRequestIDMeta;",
		"...resourceInspectionRequestIDFields(result),",
		"const requestIDMeta = createResourceInspectionRequestIDMeta(error);",
		"requestIDMeta ? [requestIDMeta] : []",
		"if (response.status === 401 || response.status === 403) {",
		"throw requestError('fetch-error', message, response.status, requestId);",
		"const buildResourceLogsRoute = (deploymentName, resource, limit = 0) => {",
		"params.set('containerName', containerName);",
		"const resourceLogsReadRoute = (deploymentName, resource, limit = 0) => `GET ${buildResourceLogsRoute(deploymentName, resource, limit)}`;",
		"const resourceLogsRequestRouteFields = (deploymentName, resource, limit = 0) => {",
		"return [['Recent logs request', resourceLogsReadRoute(deploymentName, resource, limit)]];",
		"const buildResourceInspectionRoute = (deploymentName, resource, limit = 0) => {",
		"const resourceInspectionReadRoute = (deploymentName, resource, limit = 0) => `GET ${buildResourceInspectionRoute(deploymentName, resource, limit)}`;",
		"if (renderOperatorReadFailurePanel(",
		"'Recent logs',",
		"resourceLogsReadRoute(deploymentName, resource, target?.limit ?? selectedLimit),",
		"'Live inspection',",
		"resourceInspectionReadRoute(deploymentName, resource, targetLimit),",
	} {
		if !strings.Contains(body, fragment) {
			t.Fatalf("expected embedded UI to include %q", fragment)
		}
	}
}

func TestIndexHTMLIncludesECSRollingFallbackBranch(t *testing.T) {
	body := string(indexHTML)

	for _, fragment := range []string{
		"const ecsRollingExecutionDescription = (detail) => {",
		"const qualifiers = rolloutStrategyQualifiers(detail.rolloutConfig?.strategy);",
		"description: ecsRollingExecutionDescription(detail)",
	} {
		if !strings.Contains(body, fragment) {
			t.Fatalf("expected embedded UI to include %q", fragment)
		}
	}
}

func TestIndexHTMLLocksSynthesizedRolloutFallbackContract(t *testing.T) {
	body := string(indexHTML)

	for _, fragment := range []string{
		"const stableResourceIdentity = (resource) => {",
		"const canonicalUserDataBootstrapSource = (userData) => {",
		"const weightedTargetGroupCandidateExecutionDescription = (detail) => {",
		"const weightedTargetGroupTrafficShiftDescription = (detail) => {",
		"const codeDeployExecutionDescription = (detail, base) => {",
		"const instanceRefreshBatchSize = (detail) => {",
		"const canonicalRolloutExecutionFallback = (detail) => {",
		"const batchSize = instanceRefreshBatchSize(detail);",
		"Service Auto Scaling Group Tenet refreshes in place with rolling-update batch size ${batchSize}",
		"const resourceId = stableResourceIdentity({ type, name, description });",
		"createNavLink(buildPageURL('resource', { deployment: detail.name, resourceType: resource.type, resourceId: resource.id }), 'Open resource', '_self')",
		"Open resource",
		"Workloads",
		"Traffic routing",
		"Config and access",
	} {
		if !strings.Contains(body, fragment) {
			t.Fatalf("expected embedded UI to include synthesized rollout fallback contract fragment %q", fragment)
		}
	}

	type fallbackExpectation struct {
		resourceID   string
		nameFragment string
		description  string
	}

	for _, expectation := range []fallbackExpectation{
		{resourceID: "ecsService", nameFragment: "-service", description: "codeDeployExecutionDescription(detail, 'ECS service Tenet updates during CodeDeploy traffic shifting')"},
		{resourceID: "ecsService", nameFragment: "-service", description: "ecsRollingExecutionDescription(detail)"},
		{resourceID: "codeDeployApplication", nameFragment: "-codedeploy-app", description: "codeDeployExecutionDescription(detail, 'CodeDeploy application coordinating ECS traffic shifting')"},
		{resourceID: "codeDeployDeploymentGroup", nameFragment: "-codedeploy-group", description: "codeDeployExecutionDescription(detail, 'CodeDeploy deployment group Tenet targets for ECS rollout')"},
		{resourceID: "productionListener", nameFragment: "-prod-listener", description: "codeDeployExecutionDescription(detail, 'Production listener CodeDeploy shifts during ECS traffic shifting')"},
		{resourceID: "testListener", nameFragment: "-test-listener", description: "codeDeployExecutionDescription(detail, 'Test listener CodeDeploy uses to validate the replacement ECS task set before promotion')"},
		{resourceID: "blueTaskSetTargetGroup", nameFragment: "-blue", description: "codeDeployExecutionDescription(detail, 'Blue task set target group CodeDeploy treats as the live ECS traffic destination')"},
		{resourceID: "greenTaskSetTargetGroup", nameFragment: "-green", description: "codeDeployExecutionDescription(detail, 'Green task set target group CodeDeploy warms before shifting ECS traffic')"},
		{resourceID: "candidateAutoScalingGroup", nameFragment: "-candidate", description: "weightedTargetGroupCandidateExecutionDescription(detail)"},
		{resourceID: "activeTargetGroup", nameFragment: "-active", description: "Currently active target group receiving production traffic"},
		{resourceID: "candidateTargetGroup", nameFragment: "-candidate", description: "Candidate target group Tenet will register and warm before traffic shifting"},
		{resourceID: "productionListenerRule", nameFragment: "-prod-listener", description: "Production listener rule operators inspect while Tenet updates weighted forwarding during rollout"},
		{resourceID: "trafficShiftPlan", nameFragment: "-weighted-routing", description: "weightedTargetGroupTrafficShiftDescription(detail)"},
		{resourceID: "userDataBootstrapSource", nameFragment: "canonicalUserDataBootstrapSource(detail.userData)", description: "Normalized userdata bootstrap source for candidate EC2 capacity"},
		{resourceID: "userDataBootstrapSource", nameFragment: "canonicalUserDataBootstrapSource(detail.userData)", description: "Normalized userdata bootstrap source for refreshed EC2 instances"},
	} {
		contract := regexp.MustCompile(`(?s)` + regexp.QuoteMeta(expectation.resourceID) + `.*?` + regexp.QuoteMeta(expectation.nameFragment) + `.*?` + regexp.QuoteMeta(expectation.description))
		if !contract.MatchString(body) {
			t.Fatalf("expected embedded UI to lock synthesized rollout fallback resource %q with label %q and description %q", expectation.resourceID, expectation.nameFragment, expectation.description)
		}
	}

	if !strings.Contains(body, "createNavLink(buildPageURL('resource', { deployment: detail.name, resourceType: resource.type, resourceId: resource.id }), 'Open resource', '_self')") {
		t.Fatal("expected embedded UI to keep resource drill-down links keyed by stable resourceId identities")
	}
}

func TestIndexHTMLIncludesTolerantResourceBookmarkResolutionContract(t *testing.T) {
	body := string(indexHTML)

	for _, fragment := range []string{
		"const legacyResourceIdentityParam = (params) => {",
		"const type = (params.get('resourceType') || '').trim();",
		"const legacyName = (params.get('resourceName') || '').trim();",
		"if (!legacyName) return '';",
		"return type ? [type, legacyName].join('::') : legacyName;",
		"const parseResourceIdentity = (resourceId) => {",
		"const [type = '', name = '', ...descriptionParts] = String(resourceId || '').split('::');",
		"description: descriptionParts.join('::'),",
		"const bootstrapSourceAliasIdentity = (value) => {",
		"const resourceBookmarkAliasMatchesResolvedResource = (bookmarkedResource, resource) => {",
		"if (bookmarkedResource.type !== 'userDataBootstrapSource') return false;",
		"const bookmarkedAlias = bootstrapSourceAliasIdentity(bookmarkedResource.name);",
		"const resolvedAlias = bootstrapSourceAliasIdentity(resource.name);",
		"const bookmarkedResource = parseResourceIdentity(currentPageState.resourceId);",
		"const exactMatch = resources.find((resource) => {",
		"return resource.id === currentPageState.resourceId && resource.type === currentPageState.resourceType;",
		"const aliasMatch = resources.find((resource) => resourceBookmarkAliasMatchesResolvedResource(bookmarkedResource, resource));",
		"const tolerantMatch = resources.find((resource) => {",
		"if (bookmarkedResource.type && resource.type !== bookmarkedResource.type) {",
		"if (bookmarkedResource.name && resource.name !== bookmarkedResource.name) {",
		"return Boolean(bookmarkedResource.type || bookmarkedResource.name);",
		"const resolved = exactMatch || aliasMatch || tolerantMatch || resources.find((resource) => {",
		"type: bookmarkedResource.type || currentPageState.resourceType || 'resource',",
		"name: bookmarkedResource.name || bookmarkedResource.type || currentPageState.resourceType || 'Resource',",
		"description: bookmarkedResource.description || 'No linked resource metadata is available for this selection.',",
		"const createCurrentResourceRouteState = (resource) => {",
		"const openedViaAlias = resourceBookmarkAliasMatchesResolvedResource(bookmarkedResource, resource);",
		"routeResourceId: openedViaAlias ? bookmarkedResource.id : resource.id,",
		"routeResourceType: currentPageState.resourceType || bookmarkedResource.type || resource.type,",
		"Opened via bookmarked resource alias. Requested resource id ${resourceRouteState.routeResourceId}; resolved canonical resource id ${resource.id}.",
	} {
		if !strings.Contains(body, fragment) {
			t.Fatalf("expected embedded UI to include tolerant resource bookmark resolution fragment %q", fragment)
		}
	}

	for _, legacy := range []string{
		"name: currentPageState.resourceId || currentPageState.resourceType || 'Resource'",
	} {
		if strings.Contains(body, legacy) {
			t.Fatalf("expected embedded UI to drop brittle resource fallback fragment %q", legacy)
		}
	}
}

func TestEmbeddedUIIncludesDeploymentWorkspaceViewModes(t *testing.T) {
	body := string(indexHTML)

	for _, fragment := range []string{
		"const renderDeploymentWorkspace = (mode, detail, planState = null, applyState = null, workspaceState = null) => {",
		"const deploymentWorkspaceRefreshStateText = (mode, workspaceState) => {",
		"return `Refreshing ${workspaceLabel} from GET /v1/overview/deployments/{deploymentName}…`;",
		"return `Failed to refresh ${workspaceLabel}: ${workspaceState.error?.message || 'unknown error'}`;",
		"const deploymentDetailJSONRoute = (deploymentName) => `/v1/overview/deployments/${encodeURIComponent(deploymentName || '')}`;",
		"const deploymentDetailReadRoute = (deploymentName) => `GET /v1/overview/deployments/${deploymentName || '{deploymentName}'}`;",
		"const workspaceTitle = mode === 'drift' ? 'Drift viewer' : 'Plan and apply';",
		"const deploymentName = detail.name || currentPageState.deployment || '';",
		"createElement('div', 'account-topology workspace-read-route', `Current workspace read: ${deploymentDetailReadRoute(deploymentName)}.`)",
		"const runtimeSignalsSection = createDetailSection('Latest runtime signals');",
		"['Runtime source', metricValue(detail.runtimeSummary?.source)]",
		"['Runtime timestamp', metricValue(detail.runtimeSummary?.timestamp)]",
		"['Runtime history', Number.isFinite(detail.runtimeSummary?.historyCount) ? String(detail.runtimeSummary.historyCount) : '—']",
		"detail.runtimeSummary?.changes || []",
		"No runtime change signals are currently reported.",
		"const executionSection = createDetailSection('Rollout execution');",
		"rolloutExecutionResources(detail).filter((item) => normalizeResource(item).id === resource.id)",
		"No rollout execution resources are currently reported.",
		"const backLink = createNavLink(buildPageURL('application', { deployment: deploymentName }), 'Back to application');",
		"backLink.target = '_self';",
		"backLink.rel = '';",
		"toolbarActions.append(createNavLink(deploymentDetailJSONRoute(deploymentName), 'Workspace JSON'));",
		"const createDeploymentQuickNav = (deploymentName, currentWorkspace, linkedResources = null, sectionLinks = [], currentSectionEmptyLabel = '') => {",
		"const createWorkspaceQuickNav = (detail, mode, sectionLinks = null) => {",
		"const currentWorkspace = mode === 'drift' ? 'Drift view' : 'Plan/apply';",
		"const linkedResources = (mode === 'drift' || mode === 'plan') ? groupResourceSections(detail).flatMap((group) => group.resources || []) : null;",
		"const resolvedSectionLinks = mode === 'drift'",
		"buildPageSectionURL('drift', 'drift-runtime-signals', { deployment: deploymentName })",
		"buildPageSectionURL('drift', 'drift-rollout-execution', { deployment: deploymentName })",
		"buildPageSectionURL('drift', 'drift-field-level', { deployment: deploymentName })",
		"buildPageSectionURL('drift', 'drift-status-conditions', { deployment: deploymentName })",
		"Array.isArray(sectionLinks) ? sectionLinks : [];",
		"(mode === 'drift' || mode === 'plan') ? 'No section is currently targeted.' : '',",
		"return createDeploymentQuickNav(",
		"createElement('div', 'quick-nav-label', 'Hierarchy')",
		"createElement('div', 'quick-nav-label', 'Current destination')",
		"createElement('div', 'quick-nav-label', 'Deployment workspaces')",
		"createElement('div', 'quick-nav-label', 'Current section')",
		"createElement('div', 'quick-nav-label', 'On this page')",
		"createElement('div', 'quick-nav-label', 'Linked resources')",
		"createElement('div', 'quick-nav-current', `${deploymentName || 'Deployment'} · ${currentWorkspace}`)",
		"createNavLink(buildPageURL('applications'), 'Applications overview', '_self')",
		"createNavLink(buildPageURL('application', { deployment: deploymentName }), 'Back to application', '_self')",
		"createQuickNavChip(currentWorkspace, `Current destination: ${currentWorkspace}`, 'page')",
		"if (currentWorkspace !== 'Drift view') {",
		"createNavLink(buildPageURL('drift', { deployment: deploymentName }), 'Drift view', '_self')",
		"if (currentWorkspace !== 'Plan/apply') {",
		"createNavLink(buildPageURL('plan', { deployment: deploymentName }), 'Plan/apply', '_self')",
		"if (currentWorkspace !== 'Audit log') {",
		"createNavLink(buildPageURL('audit', { deployment: deploymentName }), 'Audit log', '_self')",
		"remainingSectionLinks.forEach((sectionLink) => {",
		"sectionNavLinks.append(createNavLink(sectionLink.href, sectionLink.label, '_self'));",
		"linkedResources.forEach((resource) => {",
		"resourceType: resource.type,",
		"resourceId: resource.id,",
		"}), resource.name, '_self'));",
		"'No linked resources are available in this workspace yet.'",
		"const quickNav = createWorkspaceQuickNav(detail, mode, planSectionLinks);",
		"activeResourceQuickNavSync = typeof quickNav?.syncCurrentSection === 'function' ? quickNav.syncCurrentSection : null;",
		"header.append(head, quickNav, ...sections);",
		"const emptyTitle = mode === 'drift' ? 'Field-level drift' : 'Plan operations';",
		"const driftSeverityRank = (severity) => {",
		"const driftSeverityCounts = (rows) => rows.reduce((counts, row) => {",
		"const driftRowLabel = (count) => `${count} drift row${count === 1 ? '' : 's'}`;",
		"const driftSummaryCopy = (totalRows, visibleRows) => {",
		"const driftValueText = (value) => value == null || value === '' ? '—' : String(value);",
		"const normalizeManagedDriftResourceRef = (resourceRef) => {",
		"const normalizeManagedDriftRow = (row) => {",
		"const driftID = typeof row.driftId === 'string' ? row.driftId.trim() : '';",
		"driftId: driftID,",
		"const managedDeploymentDriftRows = (detail) => {",
		"if (!Array.isArray(detail?.managedDrift)) return null;",
		"const deriveFallbackDeploymentDriftRows = (detail) => {",
		"const deploymentDriftRows = (detail) => {",
		"const managedRows = managedDeploymentDriftRows(detail);",
		"if (managedRows) return managedRows;",
		"const deploymentDriftSource = (detail) => {",
		"badge: 'Controller-authored managedDrift',",
		"summary: 'Current source: controller-authored managedDrift rows.',",
		"factors: 'managedDrift is present in the deployment detail payload, so fallback read-model heuristics stay suppressed for this deployment.',",
		"badge: 'Derived fallback heuristics',",
		"summary: 'Current source: derived fallback read-model heuristics.',",
		"factors: 'managedDrift is absent in the deployment detail payload, so Tenet is deriving field-level drift from artifact, generation, runtime-context, and unhealthy-condition read-model signals.',",
		"const rolloutPhaseForStepKind = (stepKind) => {",
		"const effectiveRolloutContext = (detail) => ({",
		"detail.runtimeSummary?.executionMode || detail.executionMode || ''",
		"const derivePlanOperations = (detail) => {",
		"const derivePlanRisk = (detail) => {",
		"const driftCondition = condition.type === 'DriftDetected' && condition.status === true;",
		"const unhealthyCondition = condition.type === 'Healthy' && condition.status === false;",
		"if (driftCondition || unhealthyCondition) {",
		"field: 'artifact',",
		"field: 'generation',",
		"field: 'runtimeContext.executionMode',",
		"field: `condition.${condition.type || 'Unknown'}`",
		"const driftRows = deploymentDriftRows(detail);",
		"const driftSource = deploymentDriftSource(detail);",
		"createRefreshButton(",
		"'Refresh drift workspace',",
		"() => workspaceState?.onRefresh?.().catch(() => {}),",
		"const driftRefreshState = deploymentWorkspaceRefreshStateText(mode, workspaceState);",
		"workspaceState?.status === 'error' ? 'detail-state error' : 'detail-state'",
		"const defaultDriftFilters = { query: '', severity: 'all', sort: 'field' };",
		"const normalizeDriftFilters = (filters = {}) => {",
		"const syncDriftPageState = (filters = {}) => {",
		"params.set('driftQuery', normalized.query);",
		"params.set('driftSeverity', normalized.severity);",
		"params.set('driftSort', normalized.sort);",
		"const hasActiveDriftFilters = () => (",
		"const initialDriftFilters = normalizeDriftFilters(workspaceState?.filters || {});",
		"let driftQuery = initialDriftFilters.query;",
		"let driftSeverity = initialDriftFilters.severity;",
		"let driftSort = initialDriftFilters.sort;",
		"const syncDriftFilters = () => {",
		"workspaceState.filters = normalized;",
		"const clearDriftFilters = () => {",
		"createRefreshButton(\n            'Clear drift filters',\n            !hasActiveDriftFilters(),\n            clearDriftFilters,\n          )",
		"const describeClearDriftFiltersAction = (activeFilters) => {",
		"description: `Clear active drift filters: ${activeFilters.join(', ')}`",
		"applyClearDriftFiltersCopy(clearDriftFiltersButton, activeDriftFilters);",
		"const emptyClearButton = createRefreshButton('Clear drift filters', false, clearDriftFilters);",
		"applyClearDriftFiltersCopy(emptyClearButton, activeDriftFilters);",
		"driftTableWrap.replaceChildren(stateNode, emptyActions);",
		"'Drift filters: defaults'",
		"createInput('Filter drift rows', driftQuery, 'Filter by field, value, or condition context', (event) => {",
		"syncDriftFilters();",
		"createMetric('Drift summary', driftSummaryCopy(driftRows.length, filteredRows.length), 'summary')",
		"createMetric('Total rows', String(driftRows.length))",
		"createMetric('Visible rows', String(filteredRows.length))",
		"createMetric('Visible high', String(filteredCounts.high))",
		"createMetric('Visible medium', String(filteredCounts.medium))",
		"createMetric('Visible low', String(filteredCounts.low))",
		"createSelect('Severity', driftSeverity, [",
		"createSelect('Sort by', driftSort, [",
		"const driftAnchorIDs = new Map();",
		"const seenAllDriftAnchors = new Map();",
		"uniqueManagedDriftAnchorID(row, seenAllDriftAnchors)",
		"tableRow.setAttribute('id', rowID);",
		"Share drift row",
		"managedDriftAnchorURL(detail.name, rowID)",
		"Field path",
		"Desired",
		"Actual",
		"Severity",
		"Evidence",
		"Open resource evidence",
		"resolveManagedDriftResourceRef(detail, row.resourceRef)",
		"No field-level drift rows are currently reported.",
		"Filters currently hide all ${driftRowLabel(totalRows)}.",
		"Showing ${driftRowLabel(visibleRows)} out of ${driftRowLabel(totalRows)} after filters.",
		"No field-level drift rows match the current filters. Adjust the query or severity filters to inspect reported drift.",
		"No controller-authored field-level drift was reported for this deployment.",
		"No derived fallback field-level drift was detected from the deployment read model.",
		"Active drift source",
		"Current source: controller-authored managedDrift rows.",
		"Current source: derived fallback read-model heuristics.",
		"const requestError = (kind, message, status = 0, requestId = '') => {",
		"const responseErrorMessage = (response, fallback) => {",
		"const buildPlanPreviewRoute = (planId) => `/v1/plans/${encodeURIComponent(planId)}`;",
		"const fetchPlanPreview = (planId) => fetch(buildPlanPreviewRoute(planId))",
		"const buildApplyRequestRoute = (applyId) => `/v1/applies/${encodeURIComponent(applyId)}`;",
		"const fetchApplyRequest = (applyId) => fetch(buildApplyRequestRoute(applyId))",
		"const createPlanPreview = (deploymentName, ifMatch = '') => fetch(`/v1/plans/deployments/${encodeURIComponent(deploymentName)}`, {",
		"const createApplyRequest = (planId) => fetch(`/v1/applies/plans/${encodeURIComponent(planId)}`, {",
		"body: ifMatch ? JSON.stringify({ ifMatch }) : '{}',",
		"const recordOperationKind = (operation, index, fallbackPrefix) => {",
		"return kind || `${fallbackPrefix} ${index + 1}`;",
		"const operationAnchorID = (prefix, operation) => {",
		"const mapRecordOperations = (operations, fallbackPrefix, anchorPrefix = '') => (operations || []).map((operation, index) => ({",
		"const previewOperations = (record) => mapRecordOperations(record?.operations || [], 'Plan step', 'operation-plan');",
		"const applyOperations = (record) => (record?.operations || []).map((operation, index) => {",
		"anchorID: operationAnchorID('operation-apply', operation),",
		"status: applyOperationStatusClassName(operation?.status),",
		"statusLabel: applyOperationStatusLabel(operation?.status),",
		"lifecycleStatus: normalizedStatus,",
		"const createOperationsSection = ({ title, intro, operations, emptyMessage, shareURLForOperation = null }) => {",
		"section.append(markPoliteStatusNode(createElement('div', 'detail-state', emptyMessage)));",
		"createNavLink(shareURLForOperation(operation), 'Share operation', '_self')",
		"const planPreviewCanApply = (planState) => {",
		"const operatorWriteAuthRequiredState = (state) => state?.request === 'create' && state?.status === 'fetch-error' && Number(state?.error?.status) === 401;",
		"const operatorWriteForbiddenState = (state) => state?.request === 'create' && state?.status === 'fetch-error' && Number(state?.error?.status) === 403;",
		"const operatorWriteRoleRequiredState = (state) => operatorWriteForbiddenState(state) && String(state?.error?.message || '').includes('authenticated operator role is required for this operator write route');",
		"const operatorWriteAuthRequiredMessage = (route, action, context = operatorContextState.context) => {",
		"${signInRuntime.message}; sign in through ${signInRuntime.loginRoute}, then return to this same bookmarkable workspace and retry ${action}.",
		"Trusted front-door X-Tenet-Authenticated-* identity headers still take precedence when present, and Tenet will not send the write until authenticated identity is available.",
		"Tenet does not start a built-in GitHub OAuth login, enterprise SSO login, or Tenet-managed browser session from this page; retry ${action} for this same bookmarkable workspace through the trusted front door",
		"const operatorWriteRoleRequiredMessage = (route, action) => `Tenet rejected ${route} after recognizing the caller's identity because this operator write route requires a trusted forwarded operator/admin role. Retry ${action} through the trusted front door so it can forward X-Tenet-Authenticated-Roles for an operator-capable session.`;",
		"const operatorWriteForbiddenMessage = (route, action) => `Tenet rejected ${route} after recognizing the caller's identity, but this operator is not authorized to ${action.toLowerCase()} for that deployment through this route. Ask a front-door/RBAC admin to authorize this deployment through either X-Tenet-Authorized-Deployments or matching X-Tenet-Authorized-Projects plus X-Tenet-Authorized-Environments catalog placement, then retry ${action}. Partial, stale, or unavailable project/environment placement still fails closed, so use the Operator context banner to confirm the forwarded scopes match this deployment.`;",
		"const operatorWriteStateHeading = (state) => {",
		"const planPreviewStateMessage = (planState, detail) => {",
		"Create Plan will POST /v1/plans/deployments/${deploymentName}, store an immutable server-authored preview, and update this workspace URL with a shareable planId.",
		"operatorWriteAuthRequiredMessage(route, 'Create Plan')",
		"operatorWriteRoleRequiredMessage(route, 'Create Plan')",
		"operatorWriteForbiddenMessage(route, 'Create Plan')",
		"Apply Plan waits for a loaded persisted ready preview with a valid planId.",
		"Loading immutable plan preview ${activePlanId} from GET /v1/plans/${activePlanId}.",
		"const approvalSection = createDetailSection(activePlanRecord ? 'Plan preview approval guidance' : 'Derived approval guidance');",
		"Plan preview risk level",
		"Risk factors · ${riskReasons.join('; ')}",
		"Approval required · ${activePlanRecord.approval?.required ? 'Yes' : 'No'}",
		"Create Plan",
		"Apply Plan",
		"Refresh plan preview",
		"Refresh apply request",
		"Refresh plan workspace",
		"const workspaceLabel = mode === 'plan' ? 'plan workspace' : 'drift workspace';",
		"return `Refreshing ${workspaceLabel} from GET /v1/overview/deployments/{deploymentName}…`;",
		"return `Failed to refresh ${workspaceLabel}: ${workspaceState.error?.message || 'unknown error'}`;",
		"renderDeploymentWorkspace('plan', workspaceDetail,",
		"workspaceState.onRefresh = () => {",
		"workspaceDetail = withResourceDetailFetchedAt(nextDetail);",
		"Plan preview actions",
		"createOperatorWriteAuthRequiredActions(planState)",
		"Apply request actions",
		"Apply next safe action",
		"const applyNextSafeActionCue = (record, readinessOverride = null) => {",
		"Recover stale executor claim",
		"Wait for executor claim",
		"Monitor queued handoff",
		"Monitor executor progress",
		"Review local-safe disabled result",
		"pushPlanSection(createApplyNextSafeActionSection(activeApplyRecord, activeApplyReadiness), 'plan-apply-next-action', 'Apply next safe action');",
		"const applyRecordStateMessage = (applyState, detail, planState) => {",
		"Live execution is intentionally disabled by default, so failed immutable apply records are expected and useful for operator review.",
		"Future immutable lifecycle states render here as queued, running, succeeded, cancelled, or failed.",
		"const planActionHint = createElement(",
		"Create Plan reviews the current deployment detail immediately; Apply Plan and record refresh controls activate only after a persisted ready preview or apply request is loaded.",
		"Apply Plan activates only when a loaded persisted ready plan preview with a valid planId is present.",
		"Loading immutable apply request ${activeApplyID} from GET /v1/applies/${activeApplyID}.",
		"operatorWriteAuthRequiredMessage(route, 'Apply Plan')",
		"createOperatorWriteAuthRequiredActions(applyState)",
		"operatorWriteRoleRequiredMessage(route, 'Apply Plan')",
		"operatorWriteForbiddenMessage(route, 'Apply Plan')",
		"Authenticated operator required",
		"Operator role required",
		"Operator access denied",
		"Auth required",
		"Role required",
		"Access denied",
		"const workspaceState = {",
		"filters: {",
		"const renderDriftWorkspace = () => {",
		"workspaceState.onRefresh = () => {",
		"workspaceState.status = 'loading';",
		"return fetchDeploymentDetail(deploymentName)",
		"workspaceState.status = 'error';",
		"renderDriftWorkspace();",
		"const normalizeApplyRecordStatus = (status) => {",
		"const normalizeApplyOperationStatus = (status) => {",
		"const applyLifecycleTimestampValue = (lifecycle, field) => {",
		"const applyLifecycleStepValue = (lifecycle, field) => Number.isFinite(lifecycle?.[field]) && lifecycle[field] > 0",
		"const hasApplyLifecycleMetadata = (record) => {",
		"const applyClaimWorkerValue = (record) => {",
		"const applyClaimTimestampValue = (record) => {",
		"const applyClaimFreshnessValue = (record, field) => {",
		"const activeApplyClaimStatuses = ['pending', 'queued', 'running', 'in_progress', 'in-progress'];",
		"const hasApplyClaimCoreMetadata = (record) => applyClaimWorkerValue(record) !== '—' || applyClaimTimestampValue(record) !== '—';",
		"const applyClaimFreshnessRows = (record) => {",
		"Freshness metadata unavailable for this active apply claim.",
		"const hasApplyClaimMetadata = (record) => hasApplyClaimCoreMetadata(record) || hasApplyClaimFreshnessMetadata(record);",
		"const hasPendingApplyClaim = (record) => {",
		"return isActiveApplyRecord(record);",
		"const applyLifecycleProgressText = (record) => {",
		"Current step ${currentStep} of ${totalSteps}",
		"Completed ${completedSteps} of ${totalSteps}",
		"Lifecycle metadata is not available yet",
		"const createApplyTransitionTimelineSection = (record) => {",
		"Apply transition timeline",
		"Immutable apply lifecycle transitions are read-only executor/audit evidence.",
		"No lifecycle transitions have been recorded for this immutable apply yet.",
		"Actor unavailable — ${reason}",
		"Apply request queued",
		"Apply request running",
		"Apply request succeeded",
		"Apply request cancelled",
		"Apply request failed",
		"Apply request unavailable",
		"Apply request fetch error",
		"Stored apply request",
		"Apply execution readiness",
		"Next safe action",
		"Next action detail",
		"This failed record is the expected disabled-by-default local-safe outcome; review details, then create a fresh plan/apply only when execution should be retried.",
		"Live AWS mutation is disabled for this local-safe apply record; this is expected metadata-only evidence, not an AWS readiness failure.",
		"AWS credentials or provider access are unavailable in this local-safe environment.",
		"No deployer role is configured for this target account and region.",
		"Tenet could not prove the configured deployer role is assumable from this environment.",
		"Execution-readiness copy prefers the standalone non-mutating preflight read route when it is loaded, then falls back to immutable apply-record evidence.",
		"Use Refresh apply preflight to read ${route}.",
		"Apply preflight JSON",
		"Apply execution preview JSON",
		"Copy apply preflight read route",
		"Copy execution preview read route",
		"const buildApplyExecutionPreviewRoute = (applyId) => `/v1/applies/${encodeURIComponent(applyId)}/execution-preview`;",
		"Read-only execution preview loaded from ${route}. It is derived from immutable apply/preflight metadata and does not perform AWS mutation or claim executor progress.",
		"This preview surfaces the ordered executor handoff inventory Tenet would derive from immutable apply/preflight metadata. It is read-only operator evidence, not a live executor claim and not AWS mutation.",
		"Refresh execution preview",
		"Apply execution preview inventory",
		"The ordered preview inventory is copied from the stored apply/preflight read model so operators can see each likely AWS-facing handoff without running it.",
		"No read-only execution preview inventory is loaded yet. Refresh execution preview after loading an apply record to inspect the executor handoff sequence without mutating AWS.",
		"Apply preflight operation handoffs",
		"Apply preflight next action",
		"const applyPreflightNextActionSummary = (readiness) => {",
		"Blocked by",
		"Local-safe handoff ready",
		"Preflight reports no blocked operations. The queued local-safe handoff is ready to be claimed, but Tenet is not claiming live AWS mutation succeeded.",
		"createApplyPreflightNextActionSummary(activeApplyReadiness || activeApplyRecord.executionReadiness)",
		"Standalone apply preflight rows include read-only troubleshooting handoffs",
		"Live inspection",
		"Recent logs",
		"deploymentDetailRoute",
		"resourceInspectionRoute",
		"resourceLogsRoute",
		"Apply lifecycle summary",
		"Apply lifecycle metadata",
		"Executor claim",
		"Claimed by a local-safe executor worker; no live AWS mutation is implied by this handoff.",
		"Pending local-safe executor claim; no worker has claimed this active apply request yet.",
		"Executor worker",
		"Claimed at",
		"Claim freshness",
		"Freshness reason",
		"Last heartbeat",
		"Claim expires",
		"Lifecycle updated",
		"Lifecycle started",
		"Lifecycle finished",
		"Current lifecycle step",
		"Completed / total steps",
		"Queued operations",
		"Running operations",
		"Succeeded operations",
		"Failed operations",
		"Cancelled operations",
		"Queued apply operation",
		"Running apply operation",
		"Succeeded apply operation",
		"Failed apply operation",
		"Cancelled apply operation",
		"createNavLink(buildPageURL('plan', { deployment: detail.name || currentPageState.deployment, planId: activeApplyRecord.planId || activePlanRecord?.id || currentPageState.planId, applyId: activeApplyRecord.id }), 'Open bookmarked apply', '_self')",
		"createNavLink(buildApplyRequestRoute(activeApplyRecord.id), 'Apply JSON')",
		"Apply operations",
		"Immutable apply request loaded from GET /v1/applies/${activeApplyRecord.id}. The ordered execution sequence below comes from the persisted apply record, so operators can review intended execution order without depending on mutable plan-preview or deployment-derived context.",
		"No additive apply operations are stored for this request.",
		"Apply request details",
		"Stored plan preview",
		"createNavLink(buildPageURL('plan', { deployment: detail.name || currentPageState.deployment, planId: activePlanRecord.id, applyId: activeApplyRecord?.id || applyState?.applyId || '' }), 'Open bookmarked workspace', '_self')",
		"createNavLink(buildPlanPreviewRoute(activePlanRecord.id), 'Plan JSON')",
		"Plan preview errors",
		"Server-authored plan preview loaded from GET /v1/plans/${activePlanRecord.id}.",
		"No immutable plan preview has been created yet. The table below is derived from rollout steps, rollout execution resources, and the latest runtime summary in the deployment detail read model so operators can inspect likely changes before creating a server-authored preview.",
		"Operation",
		"Phase",
		"Target",
		"Operator note",
		"No derived plan operations are currently available.",
		"No server-authored plan operations are stored for this preview.",
		"const fallbackIfMatch = planState.record?.source?.fingerprint || '';",
		"const ifMatch = workspaceDetail.sourceFingerprint || await derivePlanSourceFingerprint(workspaceDetail).catch(() => '') || fallbackIfMatch;",
		"createNavLink(buildPageURL('audit', { deployment: deploymentName }), 'Audit log', '_self')",
		"let applyState = {",
		"applyId: currentPageState.applyId,",
		"const syncPlanPageURL = (planId, applyId = '') => {",
		"const loadPlanPreviewRecord = (planId) => {",
		"const loadApplyRequestRecord = (applyId) => {",
		"onReloadPlan: (planId) => loadPlanPreviewRecord(planId),",
		"onReloadApply: (applyId) => loadApplyRequestRecord(applyId),",
		"applyId: applyId || '',",
		"createApplyRequest(planID)",
		"syncPlanPageURL(record.planId || planID, record.id);",
		"loadPlanPreviewRecord(currentPageState.planId).catch(() => {});",
		"loadApplyRequestRecord(currentPageState.applyId).catch(() => {});",
		"if (currentPageState.page === 'application') {",
		"const renderApplicationWorkspace = () => {",
		"renderApplicationDetailPage(applicationState.detail, applicationState);",
		"if (currentPageState.page === 'resource') {",
		"renderResourceDetailPage(resourceState.detail, resourceState);",
		"if (currentPageState.page === 'drift') {",
		"const renderDriftWorkspace = () => {",
		"renderDeploymentWorkspace(currentPageState.page, workspaceState.detail, null, null, workspaceState);",
		"if (currentPageState.page === 'plan') {",
		"renderDeploymentWorkspace('plan', workspaceDetail, {",
		"fetchDeploymentDetail(deploymentName)",
	} {
		if !strings.Contains(body, fragment) {
			t.Fatalf("expected embedded UI to include %q", fragment)
		}
	}
}

func TestEmbeddedUIIncludesOverviewStatusInFetchErrors(t *testing.T) {
	body := string(indexHTML)

	for _, fragment := range []string{
		"const fetchApplicationsOverview = () => fetchEmbeddedReadRouteJSON('/v1/overview/applications', {",
		"unavailable: 'applications overview unavailable',",
		"failure: 'applications overview request failed',",
		"const fetchAccountDetail = (accountID) => fetchEmbeddedReadRouteJSON(`/v1/overview/accounts/${encodeURIComponent(accountID)}`, {",
		"unavailable: 'account detail unavailable',",
		"failure: 'account detail request failed',",
		"const accountDetailReadRoute = (accountID) => `GET /v1/overview/accounts/${accountID || '{awsAccountId}'}`;",
		"const accountDetailFetchedAtText = (account) => {",
		"const withAccountDetailFetchedAt = (account) => {",
		"const createAccountDetailFreshnessMeta = (accountState = null, account = null) => {",
		"Account detail read not refreshed yet",
		"Account detail read refreshed ·",
		"const createAccountDetailActions = (accountState = null) => [",
		"const accountDetailRefreshState = (accountState = null) => {",
		"const labels = changeLabels(runtimeSummary?.changes);",
		"['Latest runtime', runtimeSummaryText(deployment.runtimeSummary), 'summary']",
		"['Latest changes', changeLabelsText(deployment.runtimeSummary?.changes)]",
		"['Runtime timestamp', runtimeSummaryTimestamp(deployment.runtimeSummary)]",
		"fetchApplicationsOverview()",
		"fetchAccountDetail(currentPageState.accountId)",
		"renderOperatorReadFailurePage(embeddedPageTitle(currentPageState.page), buildEmbeddedPageReadRoute(currentPageState), error);",
		"renderGenericReadFailurePage(pageLabel, route, error);",
		"Tenet could not load ${route}: ${message}",
	} {
		if !strings.Contains(body, fragment) {
			t.Fatalf("expected embedded UI to include %q", fragment)
		}
	}
}

func TestEmbeddedUIRendersAccountStatusCountPills(t *testing.T) {
	body := string(indexHTML)

	for _, fragment := range []string{
		"const deriveAccountCounts = (account) => {",
		"const deployments = account.deployments || [];",
		"const counts = {",
		"deploymentCount: deployments.length,",
		"syncedCount: 0,",
		"progressingCount: 0,",
		"pausedCount: 0,",
		"errorCount: 0,",
		"counts.syncedCount += 1;",
		"counts.progressingCount += 1;",
		"counts.pausedCount += 1;",
		"counts.errorCount += 1;",
		"const fallbackCounts = deriveAccountCounts(account);",
		"[account.deploymentCount ?? fallbackCounts.deploymentCount, 'deployments', 'pill target']",
		"[account.syncedCount ?? fallbackCounts.syncedCount, 'synced', 'pill synced']",
		"[account.progressingCount ?? fallbackCounts.progressingCount, 'progressing', 'pill progress']",
		"[account.pausedCount ?? fallbackCounts.pausedCount, 'paused', 'pill paused']",
		"[account.errorCount ?? fallbackCounts.errorCount, 'error', 'pill error']",
		"accountCountPills.forEach(([value, label, className]) => {",
	} {
		if !strings.Contains(body, fragment) {
			t.Fatalf("expected embedded UI to include %q", fragment)
		}
	}

	fallbackContract := regexp.MustCompile(`const deriveAccountCounts = \(account\) => \{[\s\S]*?const deployments = account\.deployments \|\| \[\];[\s\S]*?deployments\.forEach\(\(deployment\) => \{[\s\S]*?switch \(deployment\.sync\?\.state\) \{[\s\S]*?case 'Synced':[\s\S]*?counts\.syncedCount \+= 1;[\s\S]*?case 'Reconciling':[\s\S]*?case 'OutOfSync':[\s\S]*?case 'Drifted':[\s\S]*?counts\.progressingCount \+= 1;[\s\S]*?case 'Paused':[\s\S]*?counts\.pausedCount \+= 1;[\s\S]*?case 'Error':[\s\S]*?counts\.errorCount \+= 1;[\s\S]*?default:[\s\S]*?\}[\s\S]*?\};`)
	if !fallbackContract.MatchString(body) {
		t.Fatalf("expected embedded UI to derive missing account status counts from deployment sync state")
	}
}

func TestEmbeddedUIGroupsDeploymentsByProjectAndEnvironment(t *testing.T) {
	body := string(indexHTML)

	for _, fragment := range []string{
		"const deriveProjectEnvironmentGroups = (account) => {",
		"const groups = new Map();",
		"const key = [deployment.appRef || '', deployment.environmentRef || '', deployment.region || ''].join('\\u0000');",
		"projectCount: projects.size,",
		"environmentCount: environments.size,",
		"const deriveGroupCounts = (group) => {",
		"const deployments = group.deployments || [];",
		"deploymentCount: deployments.length,",
		"syncedCount: 0,",
		"progressingCount: 0,",
		"pausedCount: 0,",
		"errorCount: 0,",
		"counts.syncedCount += 1;",
		"counts.progressingCount += 1;",
		"counts.pausedCount += 1;",
		"counts.errorCount += 1;",
		"const derivedTopology = deriveProjectEnvironmentGroups(account);",
		"const topology = {",
		"projectCount: account.projectCount ?? derivedTopology.projectCount,",
		"environmentCount: account.environmentCount ?? derivedTopology.environmentCount,",
		"groups: derivedTopology.groups,",
		"createElement('div', 'account-topology', `${topology.projectCount} projects · ${topology.environmentCount} environments`)",
		"createElement('h3', 'deployment-group-title', `${metricValue(group.appRef)} · ${metricValue(group.environmentRef)}`)",
		"createElement('div', 'deployment-group-meta', `Project ${metricValue(group.appRef)} · Environment ${metricValue(group.environmentRef)} · Region ${metricValue(group.region)}`)",
		"const groupCount = group.deployments.length;",
		"groupPills.append(createElement('span', 'group-pill', `${groupCount} deployment${groupCount === 1 ? '' : 's'}`));",
		"const groupCounts = deriveGroupCounts(group);",
		"[groupCounts.syncedCount, 'synced', 'pill synced']",
		"[groupCounts.progressingCount, 'progressing', 'pill progress']",
		"[groupCounts.pausedCount, 'paused', 'pill paused']",
		"[groupCounts.errorCount, 'error', 'pill error']",
		"if (value > 0) {",
		"groupPills.append(createElement('span', className, `${value} ${label}`));",
		"const groupRows = createElement('div', 'group-rows');",
		"groupSection.append(groupHead, groupRows);",
	} {
		if !strings.Contains(body, fragment) {
			t.Fatalf("expected embedded UI to include %q", fragment)
		}
	}

	fallbackContract := regexp.MustCompile(`const deriveGroupCounts = \(group\) => \{[\s\S]*?const deployments = group\.deployments \|\| \[\];[\s\S]*?deployments\.forEach\(\(deployment\) => \{[\s\S]*?switch \(deployment\.sync\?\.state\) \{[\s\S]*?case 'Synced':[\s\S]*?counts\.syncedCount \+= 1;[\s\S]*?case 'Reconciling':[\s\S]*?case 'OutOfSync':[\s\S]*?case 'Drifted':[\s\S]*?counts\.progressingCount \+= 1;[\s\S]*?case 'Paused':[\s\S]*?counts\.pausedCount \+= 1;[\s\S]*?case 'Error':[\s\S]*?counts\.errorCount \+= 1;[\s\S]*?default:[\s\S]*?\}[\s\S]*?\};`)
	if !fallbackContract.MatchString(body) {
		t.Fatalf("expected embedded UI to derive missing group status counts from deployment sync state")
	}
}

func TestUIOverviewCardsRenderRuntimeSummaryContract(t *testing.T) {
	body := string(indexHTML)
	for _, fragment := range []string{
		"const runtimeSummaryTimestamp = (runtimeSummary) => runtimeSummary?.timestamp || 'Latest runtime status';",
		"const latestRuntimeTimestamp = (applications) => applications",
		"deployment.runtimeSummary?.timestamp",
		"const generationValue = (value) => Number.isFinite(value) && value > 0 ? String(value) : '—';",
		"const generationsText = (value) => {",
		"const runtimeArtifactText = (artifactValue) => {",
		"const runtimeConditionsText = (conditions, emptyValue = '—') => {",
		"runtimeArtifactText(runtimeSummary.deployedArtifact),",
		"runtimeConditionsText(runtimeSummary.conditions, ''),",
		"['Runtime generations', generationsText(deployment.runtimeSummary)]",
		"['Observed generation', generationValue(deployment.observedGeneration)]",
		"['Rollout generations', detail.rollout ? generationsText(detail.rollout) : `desired — · observed ${generationValue(detail.observedGeneration)}`]",
		"const runtimeSummaryText = (runtimeSummary) => {",
		"runtimeSummary.source,",
		"['Latest runtime', runtimeSummaryText(deployment.runtimeSummary), 'summary']",
		"['Runtime timestamp', runtimeSummaryTimestamp(deployment.runtimeSummary)]",
		"['Runtime generations', generationsText(deployment.runtimeSummary)]",
	} {
		if !strings.Contains(body, fragment) {
			t.Fatalf("expected embedded UI to include %q", fragment)
		}
	}
}

func TestEmbeddedUIProvidesFilterControlsAndURLSyncedViewState(t *testing.T) {
	body := string(indexHTML)

	for _, fragment := range []string{
		"<section class=\"toolbar\" id=\"toolbar\"></section>",
		"const runtimeSummaryTimestamp = (runtimeSummary) => runtimeSummary?.timestamp || 'Latest runtime status';",
		"const healthClass = (overall) => {",
		"case 'Healthy':",
		"case 'Progressing':",
		"case 'Unhealthy':",
		"const runtimeSummaryText = (runtimeSummary) => {",
		"runtimeSummary.summary,",
		"runtimeSummary.sync?.state,",
		"runtimeSummary.sync?.reason,",
		"runtimeSummary.health?.overall,",
		"runtimeSummary.health?.summary,",
		"runtimeArtifactText(runtimeSummary.deployedArtifact),",
		"runtimeConditionsText(runtimeSummary.conditions, ''),",
		"runtimeSummary.source,",
		"runtimeSummary.rolloutPhase,",
		"runtimeSummary.currentStep,",
		"Number.isFinite(runtimeSummary.historyCount) ? `history ${runtimeSummary.historyCount}` : '',",
		"const resolveManagedDriftResourceRef = (detail, resourceRef) => {",
		"resource.type === resourceRef.resourceType && resource.name === resourceRef.resourceName",
		"const createSelect = (labelText, value, options, onChange, selectOptions = {}) => {",
		"const createInput = (labelText, value, placeholder, onInput) => {",
		"const parseViewState = () => {",
		"const normalizeViewState = (applications, state) => {",
		"const syncViewState = (state) => {",
		"const applicationRuntimeMatchCandidates = (deployment) => {",
		"...changeDetailLabels(runtimeSummary.changeDetails).map((detail) => ['runtime', detail]),",
		"const applicationVisibleSearchText = (deployment) => [",
		"const createApplicationMatchContext = (deployment, query) => {",
		"['Project', textOrDash(deployment.appRef)]",
		"['Last update', runtimeSummaryTimestamp(deployment.runtimeSummary)]",
		"const applyViewState = (applications, state) => {",
		"const searchNeedle = state.query.trim().toLowerCase();",
		"deployment.accountName,",
		"deployment.awsAccountId,",
		"const matchesAccount = state.account === 'all' || deployment.awsAccountId === state.account;",
		"runtimeSummaryText(deployment.runtimeSummary),",
		"runtimeSummaryTimestamp(deployment.runtimeSummary),",
		"deployment.rollout?.summary,",
		"plannedSteps(deployment.rollout),",
		"`${healthLabel(deployment.health)}${deployment.health?.summary ? ` ${deployment.health.summary}` : ''}`,",
		"const matchesState = state.syncState === 'all' || deployment.sync?.state === state.syncState;",
		"const query = params.toString();",
		"const hash = window.location.hash || '';",
		"const nextURL = query ? `${window.location.pathname}?${query}${hash}` : `${window.location.pathname}${hash}`;",
		"history.replaceState({}, '', nextURL);",
		"value: application.awsAccountId,",
		"renderToolbar(allApplications, currentViewState);",
		"currentViewState = normalizeViewState(applications, parseViewState());",
		"renderApplicationsOverview(applications);",
		"const applicationsFilterLabels = (applications, state) => {",
		"const resetApplicationsFilters = (focusState = null) => {",
		"const renderApplicationsEmptyState = (visibleApplications, sourceApplications) => {",
		"Filters hid every application",
		"GET /v1/overview/applications returned no applications for this operator view.",
		"Check catalog loading, runtime status snapshots, or trusted front-door authorization grants if deployments were expected.",
		"Open Operator context details",
		"currentBookmarkableURL(operatorContextAccessDetailsPanelID)",
		"const applicationsClearFiltersLabel = (activeFilters) => {",
		"return `Clear ${count} ${count === 1 ? 'filter' : 'filters'}`;",
		"const applicationsClearFiltersAriaLabel = (activeFilters) => {",
		"return `Clear active application filters: ${filters.join(', ')}`;",
		"const clearLabel = applicationsClearFiltersAriaLabel(active);",
		"clearButton.setAttribute('aria-label', clearLabel);",
		"clearButton.setAttribute('title', clearLabel);",
		".toolbar-button:hover:not(:disabled)",
		".toolbar-button:disabled",
		"Clear filters",
	} {
		if !strings.Contains(body, fragment) {
			t.Fatalf("expected embedded UI to include %q", fragment)
		}
	}

	urlStateContract := regexp.MustCompile(`const parseViewState = \(\) => \{[\s\S]*?new URLSearchParams\(window\.location\.search\)[\s\S]*?query: params\.get\('q'\) \|\| ''[\s\S]*?syncState: params\.get\('sync'\) \|\| 'all'[\s\S]*?account: params\.get\('account'\) \|\| 'all'[\s\S]*?\};[\s\S]*?const normalizeViewState = \(applications, state\) => \{[\s\S]*?const validSyncStates = new Set\([\s\S]*?'Synced'[\s\S]*?'Error'[\s\S]*?\);[\s\S]*?const validAccounts = new Set\(\['all', \.\.\.applications\.map\(\(application\) => application\.awsAccountId\)\.filter\(Boolean\)\]\);[\s\S]*?query: \(state\.query \|\| ''\)\.trim\(\),[\s\S]*?account: validAccounts\.has\(state\.account\) \? state\.account : 'all'[\s\S]*?\};[\s\S]*?const syncViewState = \(state\) => \{[\s\S]*?params\.set\('q', state\.query\);[\s\S]*?params\.set\('sync', state\.syncState\);[\s\S]*?params\.set\('account', state\.account\);[\s\S]*?history\.replaceState\(\{\}, '', nextURL\);`)
	if !urlStateContract.MatchString(body) {
		t.Fatalf("expected embedded UI to round-trip validated filter state through URL query params")
	}
}

func TestEmbeddedUIPreservesToolbarFocusAndRestoresCaretAcrossFilterRerenders(t *testing.T) {
	body := string(indexHTML)

	if strings.Count(body, "const captureToolbarFocus = () => {") != 1 {
		t.Fatalf("expected embedded UI to declare captureToolbarFocus exactly once")
	}
	if strings.Count(body, "const restoreToolbarFocus = (focusState) => {") != 1 {
		t.Fatalf("expected embedded UI to declare restoreToolbarFocus exactly once")
	}

	for _, fragment := range []string{
		"select.dataset.toolbarField = labelText;",
		"input.dataset.toolbarField = labelText;",
		"const captureToolbarFocus = () => {",
		"const restoreToolbarFocus = (focusState) => {",
		"selectionStart: typeof active.selectionStart === 'number' ? active.selectionStart : null,",
		"selectionEnd: typeof active.selectionEnd === 'number' ? active.selectionEnd : null,",
		"const selector = `[data-toolbar-field=\"${focusState.field}\"]`;",
		"const node = document.querySelector(selector);",
		"node.focus();",
		"if (node instanceof HTMLInputElement && focusState.selectionStart !== null && focusState.selectionEnd !== null) {",
		"node.setSelectionRange(focusState.selectionStart, focusState.selectionEnd);",
		"const focusState = captureToolbarFocus();",
		"renderApplicationsOverview(allApplications, focusState);",
		"restoreToolbarFocus(focusState);",
		"const applicationResourceFilterFocusState = () => ({ field: 'Filter linked resources', selectionStart: 0, selectionEnd: 0 });",
		"renderApplicationDetailPage(detail, applicationState, applicationResourceFilterFocusState());",
	} {
		if !strings.Contains(body, fragment) {
			t.Fatalf("expected embedded UI to include %q", fragment)
		}
	}
}

func TestEmbeddedUIResourceDetailRecentLogsContract(t *testing.T) {
	body := string(indexHTML)

	for _, fragment := range []string{
		"const resourceLogStateByKey = new Map();",
		"const resourceLogCacheKey = (deploymentName, resource) => [deploymentName || '', resource.type || '', resource.name || '', resource.id || '', resource.containerName || '', resource.instanceId || '', resource.clusterArn || ''].join('::');",
		"const supportsECSClusterScope = (resource) => ['ecsService', 'ecsTask'].includes(String(resource?.type || '').trim());",
		"const resourceLogLimitByResourceKey = new Map();",
		"const normalizeResourceLogsLimit = (value) => {",
		"const resourceLogsLimitOptions = [",
		"resourceLogLimit: normalizeResourceLogsLimit(params.get('resourceLogLimit') || '')",
		"params.set('resourceLogLimit', String(limit));",
		"syncResourcePanelLimitQueryParams(deploymentName, resource);",
		"{ value: '', label: 'All available logs' }",
		"{ value: '5', label: '5 entries' }",
		"{ value: '10', label: '10 entries' }",
		"{ value: '25', label: '25 entries' }",
		"const resourceLogsLimitText = (limit) => {",
		"Log entry cap · ${resourceLogsLimitText(limit)}",
		"const resourceLogsResultScopeFields = (scope) => {",
		"['Requested cap', scope.requestedLimit == null ? 'All available entries' : resourceLogsScopeLimitText(scope.requestedLimit)]",
		"['Effective cap', resourceLogsScopeLimitText(scope.effectiveLimit)]",
		"['Returned entries', resourceLogsCountText(scope.returnedEntryCount)]",
		"['Newest entry', textOrDash(scope.newestEntryTimestamp)]",
		"['Oldest entry', textOrDash(scope.oldestEntryTimestamp)]",
		"['Scope status', resourceLogsTruncationStatusText(scope.truncationStatus)]",
		"const resourceInspectionResultScopeFields = (scope) => {",
		"['Sample type', resourceInspectionSampleTypeText(scope.sampleType)]",
		"['Requested sample cap', scope.requestedLimit == null ? 'All available items' : resourceInspectionResultScopeLimitText(scope.requestedLimit)]",
		"['Effective sample cap', resourceInspectionResultScopeLimitText(scope.effectiveLimit)]",
		"['Returned sample items', resourceInspectionResultScopeCountText(scope.returnedItemCount)]",
		"Capped response; more rows may exist",
		"At cap; provider totals unknown",
		"const fetchResourceLogs = (deploymentName, resource, limit = 0) => {",
		"const buildResourceLogsRoute = (deploymentName, resource, limit = 0) => {",
		"resourceType: resource?.type || '',",
		"resourceName: resource?.name || '',",
		"params.set('instanceId', instanceId);",
		"params.set('clusterArn', clusterArn);",
		"params.set('limit', String(normalizedLimit));",
		"return `/v1/overview/deployments/${encodeURIComponent(deploymentName)}/resource-logs?${params.toString()}`;",
		"const resourceLogsReadRoute = (deploymentName, resource, limit = 0) => `GET ${buildResourceLogsRoute(deploymentName, resource, limit)}`;",
		"const resourceLogsRequestRouteFields = (deploymentName, resource, limit = 0) => {",
		"return [['Recent logs request', resourceLogsReadRoute(deploymentName, resource, limit)]];",
		"const resourceLogAnchorBase = (entry) => {",
		"const backendID = auditAnchorSlug(entry?.entryId);",
		"return `resource-log-${backendID}`;",
		"return parts.length ? `resource-log-${parts.join('-')}` : 'resource-log-unknown';",
		"const uniqueResourceLogAnchorID = (entry, seenAnchors) => {",
		"const resourceLogAnchorURL = (deploymentName, resource, logID, limit = 0) => buildPageSectionURL('resource', logID, {",
		"containerName: String(currentPageState.containerName || resource?.containerName || '').trim(),",
		"instanceId: String(currentPageState.instanceId || resource?.instanceId || '').trim(),",
		"clusterArn: supportsECSClusterScope(resource) ? String(currentPageState.clusterArn || resource?.clusterArn || '').trim() : '',",
		"resourceLogLimit: limit,",
		"return fetchResourcePanelJSON(buildResourceLogsRoute(deploymentName, resource, limit), 'resource logs request failed');",
		"const fetchResourcePanelJSON = (route, failureLabel) => Promise.resolve()",
		".then(() => fetch(route))",
		"const responseRequestID = (response) => {",
		"const attachResponseRequestID = (value, requestId) => {",
		"tenetRequestId: requestId",
		"error.requestId = requestId;",
		"const createDetailSectionHeader = (title, actions = []) => {",
		"const createRefreshButton = (label, disabled, onRefresh) => {",
		"const createOperatorReadFailureRequestIDMeta = (error) => {",
		"const requestId = String(error?.requestId || '').trim();",
		"...(requestIdMeta ? [requestIdMeta] : []),",
		"const renderResourceLogsState = (container, text, className = '', beforeNodes = [], afterNodes = [], actions = []) => {",
		"container.replaceChildren(heading, ...beforeNodes, stateNode, ...afterNodes);",
		"const resourceLogsUnavailableReasonText = (reason) => {",
		"case 'unsupported_resource_type':",
		"return 'Recent logs are not supported for this resource type.';",
		"case 'cross_account_not_configured':",
		"Ask the platform owner to configure the observer role for this target account, then retry Recent logs after the account wiring is updated.",
		"case 'no_log_source':",
		"return 'This resource is a valid drill-down target, but Tenet has no independent recent-log stream for it yet.';",
		"Check whether the service is scaled down or still deploying, then retry Recent logs after a task is running.",
		"Check ASG desired capacity and instance health, then retry Recent logs after a current instance exists.",
		"Refresh resource detail to confirm the deployment still points at this resource, then retry Recent logs.",
		"case 'aws_unavailable':",
		"Capture the Tenet request ID and lookup path, then verify credentials, region/account placement, observer-role reachability, and log-source permissions.",
		"return reason ? 'Tenet returned an unavailable recent-log state. Use the lookup context above to continue provider troubleshooting.' : '';",
		"const resourceLogsEmptyReasonText = (reason) => {",
		"case 'no_recent_events':",
		"return 'Tenet reached the recent-log provider, but no events were found in the selected bounded window.';",
		"case 'no_formatted_entries':",
		"return 'Tenet found provider records, but none could be formatted into operator-visible recent-log rows.';",
		"No rows matched the current Recent logs window. Increase the Log limit, refresh after deployment activity, or use the lookup context above before escalating.",
		"const lookupContextText = (value) => value == null || value === '' ? '—' : String(value);",
		"const lookupContextBoolText = (value) => typeof value === 'boolean' ? boolLabel(value) : '—';",
		"const lookupContextReturned = (lookupContext) => lookupContext && typeof lookupContext === 'object' && !Array.isArray(lookupContext);",
		"const resourceLogsLookupContextFields = (lookupContext) => lookupContextReturned(lookupContext) ? [",
		"['Lookup region', lookupContextText(lookupContext.region)]",
		"['Control account ID', lookupContextText(lookupContext.controlAccountId)]",
		"['Target account ID', lookupContextText(lookupContext.targetAccountId)]",
		"['Cross-account lookup', lookupContextBoolText(lookupContext.crossAccount)]",
		"['Observer role configured', lookupContextBoolText(lookupContext.observerRoleConfigured)]",
		"const lookupContextPathText = (lookupContext, prefix = 'Lookup path ·') => {",
		"return `${prefix} ${parts.length ? parts.join(' · ') : 'lookup context unavailable'}`;",
		"const resourceLogsLookupPathText = (lookupContext) => lookupContextPathText(lookupContext, 'Lookup path ·');",
		"const resourceInspectionLookupPathText = (lookupContext) => lookupContextPathText(lookupContext, 'Inspection path ·');",
		"const createResourceLogsLookupPathSummary = (lookupContext) => createElement('p', 'detail-section-copy', resourceLogsLookupPathText(lookupContext));",
		"const createResourceInspectionLookupPathSummary = (lookupContext) => createElement('p', 'detail-section-copy', resourceInspectionLookupPathText(lookupContext));",
		"const createProviderLookupContextLabel = () => createElement('div', 'detail-condition-meta', 'Provider lookup context');",
		"const createResourceLogsLookupContextCard = (lookupContext) => {",
		"createElement('div', 'recent-log-lookup-context-card'",
		"createElement('div', 'recent-log-lookup-context-label', 'Provider lookup context')",
		"createElement('div', 'recent-log-lookup-context-summary', resourceLogsLookupPathText(lookupContext))",
		"createElement('span', 'recent-log-lookup-context-chip'",
		"const createResourceLogsLookupContextNodes = (lookupContext) => lookupContextReturned(lookupContext)",
		"? [createResourceLogsLookupContextCard(lookupContext)]",
		"const createResourceInspectionLookupContextNodes = (lookupContext) => lookupContextReturned(lookupContext)",
		"? [createResourceInspectionLookupPathSummary(lookupContext), createProviderLookupContextLabel()]",
		"const resourcePanelFetchedAtText = (result) => {",
		"const withResourcePanelFetchedAt = (result) => {",
		"const resourcePanelRequestIDFields = (result) => {",
		"['Tenet request ID', requestId]",
		"const fetchedAtText = resourcePanelFetchedAtText(result)",
		"['Last refreshed', fetchedAtText]",
		"const createResourcePanelRequestIDMeta = (error) => {",
		"`Tenet request ID · ${requestId}`",
		"const resourceLogsRequestIDFields = resourcePanelRequestIDFields;",
		"const createResourceLogsRequestIDMeta = createResourcePanelRequestIDMeta;",
		"const resourceLogsBackingResourceFields = (result) => {",
		"fields.push(['Resource description', lookupContextText(result.resourceDescription)]);",
		"fields.push(['Selected container', lookupContextText(result.containerName)]);",
		"if (result?.backingResourceType) {",
		"fields.push(['Backing resource type', lookupContextText(result.backingResourceType)]);",
		"if (result?.backingResourceName) {",
		"fields.push(['Backing resource name', lookupContextText(result.backingResourceName)]);",
		"const backingResourceFields = resourceLogsBackingResourceFields(result);",
		"const createResourceLogsLookupContextGrid = (lookupContext, extraFields = []) => {",
		"const createResourceLogsActions = (section, deploymentName, resource, isLoading = false, detail = null) => {",
		"const readRoute = deploymentName && resource?.type && resource?.name ? resourceLogsReadRoute(deploymentName, resource, selectedLimit) : '';",
		"createResourceLogsLimitControl(section, deploymentName, resource, isLoading, detail)",
		"'Log limit'",
		"{ disabled: isLoading || !deploymentName || !resource?.type || !resource?.name }",
		"'Refresh recent logs'",
		"renderResourceLogsSection(section, deploymentName, resource, { force: true, detail }).catch(() => {})",
		"createCopyReadRouteButton(readRoute, 'Copy recent logs route')",
		"[...extraFields, ...resourceLogsLookupContextFields(lookupContext)].forEach(([label, value]) => {",
		"const createResourceLogsBackingResourceHandoffSection = (detail, result) => {",
		"Open the resolved ${handoff.resource.type} drill-down Tenet used for this recent-log lookup.",
		"const lookupContextNodes = createResourceLogsLookupContextNodes(result?.lookupContext);",
		"const handoffSection = createResourceLogsBackingResourceHandoffSection(detail, result);",
		"const beforeNodes = (metadata) => handoffSection ? [capNote, ...lookupContextNodes, metadata, handoffSection] : [capNote, ...lookupContextNodes, metadata];",
		"const scopeFields = resourceLogsResultScopeFields(result?.resultScope);",
		"const requestRouteFields = resourceLogsRequestRouteFields(deploymentName, resource, selectedLimit);",
		"[...resourceLogsRequestIDFields(result), ...requestRouteFields, ...backingResourceFields, ...scopeFields]",
		"...resourceLogsRequestIDFields(result),",
		"...requestRouteFields,",
		"...scopeFields,",
		"const unavailableReason = resourceLogsUnavailableReasonText(result?.unavailableReason);",
		"const metadata = createResourceLogsLookupContextGrid(result?.lookupContext, [...resourceLogsRequestIDFields(result), ...requestRouteFields, ...backingResourceFields, ...scopeFields]);",
		"const details = unavailableReason",
		"renderResourceLogsState(section, 'Recent logs are unavailable for this resource.', '', beforeNodes(metadata), details, actions);",
		"const emptyReason = resourceLogsEmptyReasonText(result?.emptyReason);",
		"const details = emptyReason",
		"renderResourceLogsState(section, 'No recent logs are available for this resource.', '', beforeNodes(metadata), details, actions);",
		"...backingResourceFields,",
		"const renderResourceLogsSection = (section, deploymentName, resource, options = {}) => {",
		"const force = Boolean(options?.force);",
		"const detail = options?.detail || null;",
		"const baseLogsKey = resourceLogCacheKey(deploymentName, resource);",
		"const selectedLimit = normalizeResourceLogsLimit(options?.limit ?? resourceLogLimitByResourceKey.get(baseLogsKey) ?? 0);",
		"const logsKey = resourceLogRequestCacheKey(deploymentName, resource, selectedLimit);",
		"if (!force && cached?.status === 'ready') {",
		"const pendingContext = () => [createResourceLogsLimitNote(selectedLimit), createResourceLogsRequestContext(deploymentName, resource, selectedLimit)];",
		"renderResourceLogsState(section, 'Loading recent logs…', '', pendingContext(), [], loadingActions);",
		"[createResourceLogsLimitNote(targetLimit), createResourceLogsRequestContext(deploymentName, resource, targetLimit)],",
		"resourceLogsReadRoute(deploymentName, resource, target?.limit ?? selectedLimit),",
		"renderResourceLogsState(",
		"`Failed to load recent logs: ${error.message}`",
		"createResourceLogsActions(container, deploymentName, resource, false, target?.detail || detail),",
		"createDetailSection('Recent logs');",
		"Recent logs",
		"Refresh recent logs",
		"Lookup path ·",
		"same-account lookup",
		"observer role not configured",
		"Recent logs request",
		"Logs source",
		"Tenet request ID",
		"Last refreshed",
		"Backing resource type",
		"Backing resource name",
		"Lookup region",
		"Control account ID",
		"Target account ID",
		"Cross-account lookup",
		"Observer role configured",
		"Log stream",
		"Log group",
		"Log container",
		"Log instance",
		"Log timestamp",
		"Share log entry",
		"Copy log entry link",
		"Copy log entry share link",
		"const createCopyLinkButton = (href, label = 'Copy link', ariaLabel = '') => {",
		"return createCopyLinkButton(",
		"logItem.setAttribute('id', logID);",
		"const logAnchorURL = resourceLogAnchorURL(deploymentName, resource, logID, selectedLimit);",
		"createNavLink(logAnchorURL, 'Share log entry', '_self')",
		"createCopyLinkButton(logAnchorURL, 'Copy log entry link', 'Copy log entry share link')",
		"Resource description",
		"Selected container",
		"Selected instance",
		"Selected cluster",
		"replaceChildren",
		"textContent",
	} {
		if !strings.Contains(body, fragment) {
			t.Fatalf("expected embedded UI to include %q", fragment)
		}
	}

	for _, unsafe := range []string{
		"logsSection.innerHTML",
		"logList.innerHTML",
		"logItem.innerHTML",
		"message.innerHTML",
	} {
		if strings.Contains(body, unsafe) {
			t.Fatalf("expected embedded UI to avoid unsafe recent-log rendering %q", unsafe)
		}
	}
}

func TestEmbeddedUIResourceDetailLiveInspectionContract(t *testing.T) {
	body := string(indexHTML)

	for _, fragment := range []string{
		"const resourceInspectionLimitByResourceKey = new Map();",
		"const resourceInspectionStateByKey = new Map();",
		"const resourceInspectionCacheKey = (deploymentName, resource) => resourceLogCacheKey(deploymentName, resource);",
		"const buildResourceInspectionRoute = (deploymentName, resource, limit = 0) => {",
		"return `/v1/overview/deployments/${encodeURIComponent(deploymentName)}/resource-inspection?${params.toString()}`;",
		"const resourceInspectionReadRoute = (deploymentName, resource, limit = 0) => `GET ${buildResourceInspectionRoute(deploymentName, resource, limit)}`;",
		"const fetchResourceInspection = (deploymentName, resource, limit = 0) => {",
		"return fetchResourcePanelJSON(buildResourceInspectionRoute(deploymentName, resource, limit), 'resource inspection request failed');",
		"const supportsBoundedResourceInspection = (resource) => ['ecsService', 'candidateAutoScalingGroup', 'serviceAutoScalingGroup'].includes(String(resource?.type || '').trim());",
		"const resourceInspectionLimitLabel = (resource) => String(resource?.type || '').trim() === 'ecsService' ? 'Task sample limit' : 'ASG instance sample limit';",
		"const resourceInspectionLimitOptions = (resource) => String(resource?.type || '').trim() === 'ecsService' ? resourceInspectionTaskLimitOptions : resourceInspectionInstanceLimitOptions;",
		"{ value: '10', label: '10 instances (default)' }",
		"{ value: '25', label: '25 instances' }",
		"const resourceInspectionDeploymentName = (detailOrDeploymentName) => typeof detailOrDeploymentName === 'string'",
		"const createResourceInspectionLimitControl = (section, detail, resource, isLoading = false) => {",
		"resourceInspectionLimitLabel(resource)",
		"if (!supportsBoundedResourceInspection(resource)) return null;",
		"renderResourceInspectionSection(section, detail, resource, { force: true, limit: nextLimit }).catch(() => {});",
		"const createResourceInspectionActions = (section, detail, resource, isLoading = false) => {",
		"const selectedLimit = selectedResourceInspectionLimit(deploymentName, resource);",
		"const readRoute = deploymentName && resource?.type && resource?.name ? resourceInspectionReadRoute(deploymentName, resource, selectedLimit) : '';",
		"'Refresh live inspection'",
		"renderResourceInspectionSection(section, detail, resource, { force: true }).catch(() => {})",
		"createCopyReadRouteButton(readRoute, 'Copy live inspection route')",
		"const renderResourceInspectionState = (container, text, className = '', beforeNodes = [], afterNodes = [], actions = []) => {",
		"const resourceInspectionUnavailableReasonText = (reason) => {",
		"return 'Live inspection is not supported for this resource type.';",
		"Ask the platform owner to configure the observer role for this target account, then retry Live inspection after the account wiring is updated.",
		"Check desired count, deployment progress, and stopped-task events, then retry Live inspection after a task is running.",
		"Check ASG desired capacity and instance health, then retry Live inspection after a current instance exists.",
		"Refresh resource detail to confirm the deployment still points at this resource, then retry Live inspection.",
		"Capture the Tenet request ID and inspection path, then verify credentials, region/account placement, observer-role reachability, and describe permissions.",
		"Tenet returned an unavailable live-inspection state. Use the lookup context above to continue provider troubleshooting.",
		"const resourceLogsLookupContextFields = (lookupContext) => lookupContextReturned(lookupContext) ? [",
		"const lookupContextFields = (lookupContext) => resourceLogsLookupContextFields(lookupContext);",
		"Provider lookup context",
		"Inspection path ·",
		"cross-account lookup",
		"const createLookupContextGrid = (lookupContext, extraFields = []) => {",
		"const createResourceLogsLookupContextGrid = (lookupContext, extraFields = []) => {",
		"[...extraFields, ...resourceLogsLookupContextFields(lookupContext)].forEach(([label, value]) => {",
		"const countText = (value) => {",
		"const percentText = (value) => {",
		"return `${value}%`;",
		"const durationSecondsText = (value) => {",
		"return `${value} seconds`;",
		"const renderECSServiceInspection = (section, deploymentName, resource, result) => {",
		"Live ECS service inspection from GET /v1/overview/deployments/{deploymentName}/resource-inspection.",
		"...createResourceInspectionLookupContextNodes(result?.lookupContext)",
		"Inspection source",
		"Service name",
		"Service status",
		"Launch type",
		"Desired count",
		"Running count",
		"Pending count",
		"Minimum healthy percent",
		"Maximum percent",
		"Health check grace period",
		"const ecsDeploymentCircuitBreakerText = (circuitBreaker) => {",
		"const ecsDeploymentAlarmConfigurationText = (alarmConfiguration) => {",
		"const deploymentConfigurationFields = [];",
		"Deployment controller",
		"Deployment circuit breaker",
		"Deployment alarms",
		"Task definition",
		"Cluster ARN",
		"Service ARN",
		"Service deployments",
		"const ecsServiceDeploymentAnchorID = (deployment, index = 0) => {",
		"const ecsServiceDeploymentShareURL = (deployment, index = 0) => currentPageSectionURL(ecsServiceDeploymentAnchorID(deployment, index));",
		"Share ECS deployment",
		"const ecsTaskScopeUnavailableReasonText = (reason) => {",
		"return 'AWS access is currently unavailable for ECS task sampling.';",
		"const isECSTaskScopeUnavailable = (scope) => scope && typeof scope === 'object' && scope.available === false;",
		"const createECSTaskScopeSummary = (scope) => {",
		"Task sample unavailable · ${reasonText} · requested ${countText(scope.requestedLimit)} · effective ${countText(effectiveLimit)}",
		"Task sample scope · returned ${countText(returnedTaskCount)} task(s) from effective limit ${countText(effectiveLimit)} · ${truncationStatus}",
		"const createECSServiceEventScopeSummary = (scope) => {",
		"Event window scope · ${parts.join(' · ')}${summary ? ` · ${summary}` : ''}",
		"const taskSampleUnavailable = isECSTaskScopeUnavailable(service?.tasksScope);",
		"Task sampling is unavailable for this service; service inspection remains available.",
		"[createECSTaskScopeSummary(service?.tasksScope)]",
		"Recent ECS tasks",
		"Open task live inspection",
		"Open task recent logs",
		"Open container recent logs",
		"const appendECSTaskPlacementRows = (meta, task) => {",
		"Container instance",
		"EC2 instance",
		"Capacity provider",
		"Availability Zone",
		"appendECSTaskPlacementRows(meta, taskWithScope);",
		"const ecsContainerNetworkBindingText = (binding) => {",
		"const appendECSTaskContainerNetworkBindingRows = (meta, container, name) => {",
		"Container ${name} network binding · ${bindingText}",
		"appendECSTaskContainerNetworkBindingRows(containerSection, container, name);",
		"createECSTaskContainerLogHandoff",
		"containerName: params.get('containerName') || ''",
		"clusterArn: params.get('clusterArn') || ''",
		"routeContainerName: String(currentPageState.containerName || resource.containerName || '').trim()",
		"routeInstanceId: String(currentPageState.instanceId || resource.instanceId || '').trim()",
		"routeClusterArn: supportsECSClusterScope(resource) ? String(currentPageState.clusterArn || resource.clusterArn || '').trim() : ''",
		"const ecsTaskResourceURLParams = (taskResource) => ({",
		"...(taskResource.clusterArn ? { clusterArn: taskResource.clusterArn } : {})",
		"...ecsTaskResourceURLParams(taskResource),",
		"Clear selected container",
		"const clearSelectedContainerRoute = (detail, resource) => {",
		"containerName: '',",
		"Clear selected cluster",
		"const clearSelectedClusterRoute = (detail, resource) => {",
		"clusterArn: '',",
		"const supportsASGInstanceScope = (resource) => ['candidateAutoScalingGroup', 'serviceAutoScalingGroup', 'userDataBootstrapSource'].includes(String(resource?.type || '').trim());",
		"Clear selected instance",
		"const clearSelectedInstanceRoute = (detail, resource) => {",
		"instanceId: '',",
		"createClearSelectedInstanceButton(detail, resource, resourceState)",
		"renderResourceDetailPage(detail, resourceState);",
		"currentPageSectionURL(sectionID)",
		"selectedResourceScopeText(resource)",
		"selectedResourceScopeEntries(resource)",
		"buildPageSectionURL('resource', 'resource-recent-logs'",
		"Container ${name} awslogs",
		"Deployment status",
		"Rollout state",
		"Desired/running/pending",
		"Rollout reason",
		"Recent ECS service events",
		"[createECSServiceEventScopeSummary(service?.eventsScope)]",
		"Event timestamp",
		"const renderECSTaskInspection = (section, deploymentName, resource, result) => {",
		"Live ECS task inspection from GET /v1/overview/deployments/{deploymentName}/resource-inspection.",
		"EC2 host details",
		"Optional host evidence resolved from the inspected ECS task.",
		"EC2 state",
		"EC2 instance type",
		"AMI image ID",
		"Private IP",
		"Public IP",
		"Subnet",
		"VPC",
		"Launch time",
		"Task containers",
		"appendECSTaskContainerNetworkBindingRows(meta, container, textOrDash(container?.name || 'container'));",
		"Container awslogs",
		"No container evidence is available for this ECS task.",
		"const autoScalingObservedLaunchTemplate = (group) => {",
		"const autoScalingInstanceScopeStatusText = (value) => {",
		"const createAutoScalingInstanceScopeSummary = (scope) => {",
		"ASG instance sample unavailable · ${reasonText} · requested ${countText(scope.requestedLimit)} · effective ${countText(effectiveLimit)}",
		"const summary = String(scope.summary || '').trim();",
		"summary ? `ASG instance sample scope · ${summary}` : `ASG instance sample scope · requested ${countText(scope.requestedLimit)} · effective ${countText(effectiveLimit)} · returned ${countText(returnedInstanceCount)} instance(s) · ${truncationStatus}`",
		"ASG instance sampling is unavailable for this group; group inspection remains available.",
		"const autoScalingGroupSupportsInstanceHandoff = (resource) => ['candidateAutoScalingGroup', 'serviceAutoScalingGroup'].includes(String(resource?.type || '').trim());",
		"const autoScalingGroupRecentLogsHandoff = (deploymentName, resource) => {",
		"Open EC2 console logs",
		"const createAutoScalingInstanceHandoffLinks = (deploymentName, resource, instance) => {",
		"instanceId: params.get('instanceId') || ''",
		"Inspect this instance",
		"Open instance logs",
		"const appendAutoScalingEC2InstanceDetail = (item, instance) => {",
		"const ec2Instance = instance?.ec2Instance;",
		"EC2 state",
		"EC2 instance type",
		"AMI image ID",
		"Private IP",
		"Launch time",
		"const appendAutoScalingEC2InstanceStatusDetail = (item, instance) => {",
		"const status = instance?.ec2InstanceStatus;",
		"EC2 instance status check",
		"EC2 system status check",
		"Attached EBS status check",
		"EC2 scheduled event ${index + 1}",
		"const autoScalingActivityAnchorID = (activity, index = 0) =>",
		"const autoScalingActivityShareURL = (deploymentName, resource, activity, index = 0) =>",
		"const createAutoScalingActivityListItem = (deploymentName, resource, activity, index = 0) => {",
		"Share scaling activity",
		"Activity ID",
		"Status code",
		"Progress",
		"Start time",
		"End time",
		"Cause",
		"Description",
		"Status message",
		"buildPageSectionURL('resource', 'resource-recent-logs'",
		"const renderAutoScalingGroupInspection = (section, deploymentName, resource, result) => {",
		"Live Auto Scaling Group inspection from GET /v1/overview/deployments/{deploymentName}/resource-inspection.",
		"Auto Scaling Group name",
		"Desired capacity",
		"Min size",
		"Max size",
		"Default cooldown",
		"Health check type",
		"Health check grace period",
		"Launch template",
		"Mixed instances launch template",
		"Observed image ID",
		"Observed instance type",
		"Auto Scaling Group ARN",
		"Target group ARNs",
		"Mixed instances overrides",
		"Recent scaling activities",
		"No scaling activities are reported in the bounded recent activity window for this Auto Scaling Group.",
		"...(scalingActivitiesSection ? [scalingActivitiesSection] : []),",
		"Instances",
		"Protected from scale-in",
		"Weighted capacity",
		"const actions = [",
		"...createResourceInspectionActions(section, deploymentName, resource),",
		"autoScalingGroupRecentLogsHandoff(deploymentName, resource),",
		"const bootstrapSourceObservationText = (detail) => {",
		"No explicit bootstrap-source marker was found in the observed launch template user data.",
		"Observed launch template bootstrap source differs from the normalized desired bootstrap source.",
		"const resolveBackingResourceDrillDown = (detail, inspectionDetail) => {",
		"const backingResource = rolloutExecutionResources(detail)",
		".map((resource) => normalizeResource(resource))",
		".find((candidate) => candidate.type === backingResourceType && candidate.name === backingResourceName);",
		"resourceId: backingResource.id,",
		"const createBackingResourceHandoffSection = (detail, inspectionDetail) => {",
		"Open ${handoff.resource.name} drill-down",
		"Resolved backing resource",
		"Open the resolved ${handoff.resource.type} drill-down Tenet used for this bootstrap-source inspection.",
		"const renderUserDataBootstrapSourceInspection = (section, detail, resource, result) => {",
		"Live bootstrap-source inspection from GET /v1/overview/deployments/{deploymentName}/resource-inspection.",
		"Desired bootstrap source",
		"Observed bootstrap source",
		"Backing resource type",
		"Backing resource name",
		"Observed launch template",
		"Observed image ID",
		"Observed instance type",
		"...(handoffSection ? [handoffSection] : []),",
		"Bootstrap source observation",
		"const trafficRoutingInspectionModeText = (value) => {",
		"return 'Rollout summary only';",
		"return 'Live AWS inspection';",
		"const trafficRoutingTargetGroupDetail = (trafficRouting) => {",
		"trafficRouting.targetGroup || trafficRouting.targetGroupInspection || trafficRouting.loadBalancerTargetGroup",
		"const targetHealthCountsText = (counts) => {",
		"const createTrafficRoutingTargetGroupSection = (detail, trafficRouting) => {",
		"Target group live details",
		"Target group name",
		"Target health counts",
		"Registered target health",
		"const listenerRuleArrayText = (values) =>",
		"const createCodeDeployDeploymentGroupSection = (detail, trafficRouting) => {",
		"CodeDeploy deployment group status",
		"Bounded live deployment-group evidence for the CodeDeploy traffic-routing resource selected in this drill-down.",
		"deploymentGroup.latestDeployment || deploymentGroup.latestCodeDeployDeployment",
		"const codeDeployDeploymentStatusClass = (status) => {",
		"const codeDeployDeploymentOperatorCue = (status) => {",
		"Latest CodeDeploy deployment",
		"Latest deployment · ${textOrDash(latestStatus)}",
		"Rollout is still active. Watch the CodeDeploy deployment before promoting or troubleshooting traffic.",
		"Rollout needs attention. Inspect the CodeDeploy deployment message and target-group health before retrying.",
		"Deployment ID",
		"Deployment status",
		"Create time",
		"Complete time",
		"Status message",
		"Rollback diagnostics",
		"Rollback deployment ID",
		"Rollback trigger deployment ID",
		"Rollback message",
		"Error diagnostics",
		"Error code",
		"Error message",
		"Alarm diagnostics",
		"Alarm status",
		"Alarm polling enabled",
		"Ignore alarm polling failures",
		"Alarms",
		"Deployment overview",
		"CodeDeploy instance or target lifecycle counts for the latest deployment.",
		"Pending",
		"In progress",
		"Succeeded",
		"Failed",
		"Skipped",
		"Ready",
		"CodeDeploy ECS service bindings",
		"Latest CodeDeploy deployment",
		"CodeDeploy target groups",
		"const createTrafficRoutingListenerRuleSection = (detail, trafficRouting) => {",
		"Listener rule live details",
		"Rule ARN",
		"Default rule",
		"Listener rule conditions",
		"Listener rule actions",
		"weight ${textOrDash(weight)}",
		"const renderECSTaskInspection = (section, deploymentName, resource, result) => {",
		"const renderTrafficRoutingInspection = (section, detail, resource, result) => {",
		"Read-only traffic-routing inspection from GET /v1/overview/deployments/{deploymentName}/resource-inspection.",
		"Traffic-routing status",
		"['Resource', trafficRoutingDisplayName(trafficRouting)]",
		"['Inspection mode', trafficRoutingInspectionModeText(trafficRouting?.inspectionMode)]",
		"['Source', metricValue(result?.source)]",
		"['Summary', textOrDash(trafficRouting?.summary)]",
		"const targetGroup = trafficRoutingTargetGroupDetail(trafficRouting);",
		"const targetGroupHealthCueNode = createTargetGroupHealthCue(targetGroupHealthCounts);",
		"Target health · ${cue.label}",
		"At least one target is unhealthy. Inspect registered target health, deployment events, and service logs before shifting more traffic.",
		"Targets are still registering or draining. Wait for target health to settle before treating traffic routing as stable.",
		"Some targets are unused or unavailable. Confirm listener routing and target registration before relying on this group.",
		"All reported targets are healthy. Continue rollout checks with deployment status and recent logs.",
		"const targetGroupHealthCounts = targetGroup?.targetHealthCounts || targetGroup?.healthCounts;",
		"const targetGroupHealth = targetHealthCountsText(targetGroupHealthCounts);",
		"if (targetGroupHealth && targetGroupHealth !== '—') {",
		"statusRows.push(['Target group health', targetGroupHealth]);",
		"`${label} · ${value}`",
		"Resource kind",
		"Display name",
		"Inspection mode",
		"Rollout summary",
		"const renderResourceInspectionPayload = (section, detail, resource, result) => {",
		"renderResourceInspectionState(section, 'Live inspection is unavailable for this resource.', '', [...createResourceInspectionLookupContextNodes(result?.lookupContext), metadata], details, actions);",
		"renderResourceInspectionState(section, 'No live inspection details are available for this resource yet.', '', [...createResourceInspectionLookupContextNodes(result?.lookupContext), metadata], [], actions);",
		"const renderResourceInspectionSection = (section, detail, resource, options = {}) => {",
		"const selectedLimit = supportsBoundedResourceInspection(resource) ? normalizeResourceInspectionTaskLimit",
		"const inspectionKey = resourceInspectionRequestCacheKey(deploymentName, resource, selectedLimit);",
		"const force = Boolean(options?.force);",
		"const baseInspectionKey = resourceInspectionCacheKey(deploymentName, resource);",
		"if (!force && cached?.status === 'ready') {",
		"const pendingContext = () => [createResourceInspectionRequestContext(deploymentName, resource, selectedLimit)];",
		"renderResourceInspectionState(section, 'Loading live inspection…', '', pendingContext(), [], loadingActions);",
		"[createResourceInspectionRequestContext(deploymentName, resource, targetLimit)],",
		"resourceInspectionReadRoute(deploymentName, resource, targetLimit),",
		"renderResourceInspectionState(",
		"`Failed to load live inspection: ${error.message}`",
		"createResourceInspectionActions(container, targetDetail, resource),",
		"createDetailSection('Live inspection');",
		"renderResourceInspectionSection(inspectionSection, detail, resource).catch(() => {});",
		"Live inspection",
		"Refresh live inspection",
	} {
		if !strings.Contains(body, fragment) {
			t.Fatalf("expected embedded UI to include %q", fragment)
		}
	}

	for _, unsafe := range []string{
		"inspectionSection.innerHTML",
		"renderResourceInspectionState.innerHTML",
		"metadata.innerHTML",
		"item.innerHTML",
		"networkBinding.innerHTML",
	} {
		if strings.Contains(body, unsafe) {
			t.Fatalf("expected embedded UI to avoid unsafe live-inspection rendering %q", unsafe)
		}
	}
}

func TestEmbeddedUIRendersECSServiceDeploymentRolloutRowsSafely(t *testing.T) {
	body := string(indexHTML)

	for _, fragment := range []string{
		"const ecsServiceDeploymentAnchorID = (deployment, index = 0) => {",
		"if (deploymentID) return `ecs-deployment-${sectionIDSegment(deploymentID, 'unknown')}`;",
		"`row-${index + 1}`",
		"const ecsServiceDeploymentShareURL = (deployment, index = 0) => currentPageSectionURL(ecsServiceDeploymentAnchorID(deployment, index));",
		"const createECSServiceDeploymentListItem = (deployment, index = 0) => {",
		"item.id = ecsServiceDeploymentAnchorID(deployment, index);",
		"item.classList.add('operation-anchor-row');",
		"const cueNodes = createElement('div', 'pills');",
		"`Deployment · ${textOrDash(deployment.id || deployment.taskDefinition || deployment.status)}`",
		"createElement('span', 'pill synced', 'Primary deployment')",
		"createElement('span', 'pill progress', 'Active deployment')",
		"['Deployment ID', deployment.id]",
		"['Deployment status', deployment.status]",
		"['Rollout state', deployment.rolloutState]",
		"['Rollout reason', deployment.rolloutStateReason]",
		"['Desired/running/pending', `${countText(deployment.desiredCount)} / ${countText(deployment.runningCount)} / ${countText(deployment.pendingCount)}`]",
		"['Task definition', deployment.taskDefinition]",
		"['Created at', deployment.createdAt]",
		"['Updated at', deployment.updatedAt]",
		"createNavLink(ecsServiceDeploymentShareURL(deployment, index), 'Share ECS deployment', '_self')",
		"createInspectionListSection(",
		"'Service deployments'",
		"(deployment, index) => createECSServiceDeploymentListItem(deployment, index),",
	} {
		if !strings.Contains(body, fragment) {
			t.Fatalf("expected embedded UI ECS service deployment rollout contract %q", fragment)
		}
	}

	for _, unsafe := range []string{
		"deployment.rolloutStateReason.innerHTML",
		"rolloutStateReason.innerHTML",
		"createECSServiceDeploymentListItem.innerHTML",
	} {
		if strings.Contains(body, unsafe) {
			t.Fatalf("expected deployment rollout rows to avoid unsafe rendering %q", unsafe)
		}
	}
}

func TestSmokeScriptsLockECSServiceDeploymentRolloutUILabels(t *testing.T) {
	labels := []string{
		"Service deployments",
		"Deployment ID",
		"Deployment status",
		"Rollout state",
		"Rollout reason",
		"Desired/running/pending",
		"Task definition",
		"Created at",
		"Updated at",
		"Primary deployment",
		"Active deployment",
	}

	for _, script := range []string{"smoke-local.sh", "smoke-docker.sh"} {
		body, err := os.ReadFile("../../scripts/" + script)
		if err != nil {
			t.Fatalf("read %s: %v", script, err)
		}

		text := string(body)
		for _, label := range labels {
			if !strings.Contains(text, label) {
				t.Fatalf("expected %s to lock ECS service deployment UI label %q", script, label)
			}
		}
	}
}

func TestSmokeScriptsLockApplicationsOverviewInitialLoadingCopy(t *testing.T) {
	for _, script := range []string{"smoke-local.sh", "smoke-docker.sh"} {
		body, err := os.ReadFile("../../scripts/" + script)
		if err != nil {
			t.Fatalf("read %s: %v", script, err)
		}
		if !strings.Contains(string(body), "Loading applications overview…") {
			t.Fatalf("expected %s to lock applications-overview initial loading copy", script)
		}
	}
}

func TestSmokeScriptsLockApplicationLinkedResourceSourceCopy(t *testing.T) {
	fragments := []string{
		"const applicationLinkedResourceSourceText = (detail) => {",
		"Linked-resource source",
		"Controller-authored rollout execution",
		"Fallback rollout inventory",
		"rollout.execution[] is missing or empty, so Tenet is showing canonical fallback resources for this execution mode.",
		"const applicationResourceFilterMatches = (resource, query) => {",
		"createInput('Filter linked resources', currentPageState.resourceFilter, 'Filter by type, name, description, or source',",
	}

	for _, script := range []string{"smoke-local.sh", "smoke-docker.sh"} {
		body, err := os.ReadFile("../../scripts/" + script)
		if err != nil {
			t.Fatalf("read %s: %v", script, err)
		}
		text := string(body)
		for _, fragment := range fragments {
			if !strings.Contains(text, fragment) {
				t.Fatalf("expected %s to lock application linked-resource source/filter fragment %q", script, fragment)
			}
		}
	}
}

func TestEmbeddedUIIncludesRuntimeAndHistoryChangeDetailsRendering(t *testing.T) {
	body := string(indexHTML)

	for _, fragment := range []string{
		"const changeDetailLabels = (changeDetails) => (changeDetails || []).filter((detail) => typeof detail === 'string' && detail);",
		"const changeDetailsText = (changeDetails) => {",
		"const labels = changeDetailLabels(changeDetails);",
		"return labels.length ? labels.join('; ') : '—';",
		"['Runtime change details', changeDetailsText(detail.runtimeSummary?.changeDetails)],",
		"History change details",
		"changeDetailLabels(entry.changeDetails).length",
		"changeDetailsText(entry.changeDetails)",
	} {
		if !strings.Contains(body, fragment) {
			t.Fatalf("expected embedded UI to include %q", fragment)
		}
	}
}

func TestCISmokeWorkflowSyntaxChecksFastLaneScripts(t *testing.T) {
	body, err := os.ReadFile("../../.github/workflows/ci-smoke.yml")
	if err != nil {
		t.Fatalf("read ci-smoke workflow: %v", err)
	}

	text := string(body)
	fastStart := strings.Index(text, "  fast-smoke:")
	dockerStart := strings.Index(text, "  docker-smoke:")
	if fastStart < 0 || dockerStart < 0 || dockerStart <= fastStart {
		t.Fatalf("expected fast-smoke and docker-smoke jobs in workflow")
	}
	fastJob := text[fastStart:dockerStart]
	for _, fragment := range []string{
		"run: bash -n scripts/smoke-local.sh",
		"run: bash -n scripts/smoke-docker.sh",
		"run: bash -n scripts/ci-smoke-runner-evidence.sh",
		"run: make smoke-local",
	} {
		if !strings.Contains(fastJob, fragment) {
			t.Fatalf("expected fast-smoke job to include %q", fragment)
		}
	}
	if strings.Contains(fastJob, "make smoke-docker") {
		t.Fatalf("fast-smoke job must not run full docker smoke")
	}

	dockerJob := text[dockerStart:]
	for _, fragment := range []string{
		"needs: docker-smoke-scope",
		"if: needs.docker-smoke-scope.outputs.should-run == 'true'",
		"run: bash -n scripts/smoke-docker.sh",
		"run: make smoke-docker",
	} {
		if !strings.Contains(dockerJob, fragment) {
			t.Fatalf("expected docker-smoke job to include %q", fragment)
		}
	}

	scopeStart := strings.Index(text, "  docker-smoke-scope:")
	if scopeStart < 0 || scopeStart <= fastStart || scopeStart >= dockerStart {
		t.Fatalf("expected docker-smoke-scope job between fast-smoke and docker-smoke")
	}
	scopeJob := text[scopeStart:dockerStart]
	for _, fragment := range []string{
		"Docker smoke scope",
		"fetch-depth: 0",
		"git diff --name-only \"$BASE_SHA\" \"$HEAD_SHA\"",
		".github/workflows/ci-smoke.yml",
		"scripts/smoke-docker.sh",
		"scripts/docker-up.sh",
		"compose.yaml",
		"examples/local-repo/*|examples/local-repo/**",
		"examples/local-status/*|examples/local-status/**",
		"cmd/tenet-api/*|cmd/tenet-api/**",
		"cmd/tenet-controller/*|cmd/tenet-controller/**",
		"internal/api/*|internal/api/**",
		"internal/catalog/*|internal/catalog/**",
		"internal/domain/*|internal/domain/**",
		"internal/placement/*|internal/placement/**",
		"internal/githubwebhook/*|internal/githubwebhook/**",
		"internal/authprovider/*|internal/authprovider/**",
		"internal/authsaml/*|internal/authsaml/**",
		"internal/authsso/*|internal/authsso/**",
		"internal/authidentity/*|internal/authidentity/**",
		"internal/authstate/*|internal/authstate/**",
		"internal/requestactor/*|internal/requestactor/**",
		"internal/rbacpolicy/*|internal/rbacpolicy/**",
		"internal/statusstore/*|internal/statusstore/**",
		"internal/resourceaws/*|internal/resourceaws/**",
		"internal/resourcelogs/*|internal/resourcelogs/**",
		"internal/resourceinspection/*|internal/resourceinspection/**",
		"internal/planpreview/*|internal/planpreview/**",
		"internal/applyrequest/*|internal/applyrequest/**",
		"internal/auditlog/*|internal/auditlog/**",
		"Docker smoke required because this PR touches a Compose/front-door/auth/API/controller, local desired-state/status fixture, catalog/domain/placement status read model, RBAC policy diagnostics, GitHub webhook, resource evidence/logs/inspection, or plan/apply/audit persistence smoke surface.",
		"Docker smoke skipped because this PR does not touch a Compose/front-door/auth/API/controller, local desired-state/status fixture, catalog/domain/placement status read model, RBAC policy diagnostics, GitHub webhook, resource evidence/logs/inspection, or plan/apply/audit persistence smoke surface.",
	} {
		if !strings.Contains(scopeJob, fragment) {
			t.Fatalf("expected docker-smoke-scope job to include %q", fragment)
		}
	}
}

func TestCISmokeWorkflowDockerScopeCoversFixtureEvidenceChanges(t *testing.T) {
	body, err := os.ReadFile("../../.github/workflows/ci-smoke.yml")
	if err != nil {
		t.Fatalf("read ci-smoke workflow: %v", err)
	}

	patterns := dockerSmokeScopePatterns(t, string(body))
	for _, changedPath := range []string{
		"examples/local-repo/deployments/payments-api-prod.json",
		"examples/local-status/deployments.json",
	} {
		if !dockerSmokeScopeMatches(patterns, changedPath) {
			t.Fatalf("expected fixture-only change %s to trigger docker smoke scope; patterns=%v", changedPath, patterns)
		}
	}
	if dockerSmokeScopeMatches(patterns, "docs/features/overview-dashboard.md") {
		t.Fatalf("expected unrelated docs-only paths to skip docker smoke scope")
	}
}

func TestCISmokeWorkflowDockerScopeCoversStatusReadModelChanges(t *testing.T) {
	body, err := os.ReadFile("../../.github/workflows/ci-smoke.yml")
	if err != nil {
		t.Fatalf("read ci-smoke workflow: %v", err)
	}

	patterns := dockerSmokeScopePatterns(t, string(body))
	for _, changedPath := range []string{
		"internal/domain/status.go",
		"internal/catalog/overview.go",
		"internal/placement/resolver.go",
	} {
		if !dockerSmokeScopeMatches(patterns, changedPath) {
			t.Fatalf("expected %s to trigger docker smoke scope; patterns=%v", changedPath, patterns)
		}
	}
	if dockerSmokeScopeMatches(patterns, "docs/features/overview-dashboard.md") {
		t.Fatalf("expected unrelated docs-only paths to skip docker smoke scope")
	}
}

func TestCISmokeWorkflowDockerScopeCoversGitHubWebhookChanges(t *testing.T) {
	body, err := os.ReadFile("../../.github/workflows/ci-smoke.yml")
	if err != nil {
		t.Fatalf("read ci-smoke workflow: %v", err)
	}

	patterns := dockerSmokeScopePatterns(t, string(body))
	if !dockerSmokeScopeMatches(patterns, "internal/githubwebhook/webhook.go") {
		t.Fatalf("expected internal/githubwebhook/webhook.go to trigger docker smoke scope; patterns=%v", patterns)
	}
	if dockerSmokeScopeMatches(patterns, "docs/features/overview-dashboard.md") {
		t.Fatalf("expected unrelated docs-only paths to skip docker smoke scope")
	}
}

func TestCISmokeWorkflowDockerScopeCoversRBACPolicyChanges(t *testing.T) {
	body, err := os.ReadFile("../../.github/workflows/ci-smoke.yml")
	if err != nil {
		t.Fatalf("read ci-smoke workflow: %v", err)
	}

	patterns := dockerSmokeScopePatterns(t, string(body))
	for _, changedPath := range []string{
		"internal/rbacpolicy/policy.go",
		"internal/rbacpolicy/file.go",
		"internal/rbacpolicy/testdata/policy.json",
	} {
		if !dockerSmokeScopeMatches(patterns, changedPath) {
			t.Fatalf("expected %s to trigger docker smoke scope; patterns=%v", changedPath, patterns)
		}
	}
	if dockerSmokeScopeMatches(patterns, "docs/features/overview-dashboard.md") {
		t.Fatalf("expected unrelated docs-only paths to skip docker smoke scope")
	}
}

func TestCISmokeWorkflowDockerScopeCoversSharedResourceAWSChanges(t *testing.T) {
	body, err := os.ReadFile("../../.github/workflows/ci-smoke.yml")
	if err != nil {
		t.Fatalf("read ci-smoke workflow: %v", err)
	}

	patterns := dockerSmokeScopePatterns(t, string(body))
	if !dockerSmokeScopeMatches(patterns, "internal/resourceaws/access.go") {
		t.Fatalf("expected internal/resourceaws/access.go to trigger docker smoke scope; patterns=%v", patterns)
	}
	if dockerSmokeScopeMatches(patterns, "docs/features/resource-recent-logs.md") {
		t.Fatalf("expected unrelated docs-only paths to skip docker smoke scope")
	}
}

func TestCISmokeWorkflowDockerScopeCoversManagedLoginRuntimeChanges(t *testing.T) {
	body, err := os.ReadFile("../../.github/workflows/ci-smoke.yml")
	if err != nil {
		t.Fatalf("read ci-smoke workflow: %v", err)
	}

	patterns := dockerSmokeScopePatterns(t, string(body))
	for _, changedPath := range []string{
		"internal/authprovider/google_oidc.go",
		"internal/authsaml/assertion.go",
		"internal/authsso/runtime.go",
		"internal/authidentity/file.go",
		"internal/authstate/file.go",
	} {
		if !dockerSmokeScopeMatches(patterns, changedPath) {
			t.Fatalf("expected %s to trigger docker smoke scope; patterns=%v", changedPath, patterns)
		}
	}
}

func dockerSmokeScopePatterns(t *testing.T, workflow string) []string {
	t.Helper()

	start := strings.Index(workflow, "case \"$changed_path\" in")
	if start < 0 {
		t.Fatalf("expected docker smoke scope case statement")
	}
	block := workflow[start:]
	endMarker := ")\n                should_run=true"
	end := strings.Index(block, endMarker)
	if end < 0 {
		t.Fatalf("expected docker smoke scope case body to set should_run")
	}
	block = block[:end]
	block = strings.TrimPrefix(block, "case \"$changed_path\" in")
	block = strings.NewReplacer("\\\n", "", "\n", "", "\t", "", " ", "").Replace(block)
	block = strings.Trim(block, "|")
	if block == "" {
		t.Fatalf("expected docker smoke scope patterns")
	}
	return strings.Split(block, "|")
}

func dockerSmokeScopeMatches(patterns []string, changedPath string) bool {
	for _, pattern := range patterns {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			continue
		}
		matchPattern := "^" + regexp.QuoteMeta(pattern) + "$"
		matchPattern = strings.ReplaceAll(matchPattern, "\\*", ".*")
		if regexp.MustCompile(matchPattern).MatchString(changedPath) {
			return true
		}
	}
	return false
}

func TestSmokeScriptsLockBoundedASGResourceInspectionReleaseGuard(t *testing.T) {
	for _, script := range []string{"smoke-local.sh", "smoke-docker.sh"} {
		body, err := os.ReadFile("../../scripts/" + script)
		if err != nil {
			t.Fatalf("read %s: %v", script, err)
		}
		text := string(body)
		for _, fragment := range []string{
			"resource-inspection?resourceType=candidateAutoScalingGroup&resourceName=payments-api-prod-candidate&limit=2",
			"resource-inspection?resourceType=candidateAutoScalingGroup&resourceName=payments-api-prod-candidate&limit=0",
			"bounded_candidate_resource_inspection_path=$(write_payload bounded-candidate-resource-inspection.json \"$bounded_candidate_resource_inspection\")",
			"assert_asg_resource_inspection(bounded_candidate_resource_inspection, expect_limit=2)",
			"requestedLimit",
			"truncationStatus",
			"invalid_resource_inspection_limit.strip()",
		} {
			if !strings.Contains(text, fragment) {
				t.Fatalf("expected %s to lock bounded ASG inspection smoke fragment %q", script, fragment)
			}
		}
	}
}

func TestSmokeScriptsLockControllerRefreshFilesystemNoopReleaseGuard(t *testing.T) {
	for _, script := range []string{"smoke-local.sh", "smoke-docker.sh"} {
		body, err := os.ReadFile("../../scripts/" + script)
		if err != nil {
			t.Fatalf("read %s: %v", script, err)
		}
		text := string(body)
		for _, fragment := range []string{
			"/v1/controller/refresh",
			"/v1/controller/refresh/status",
			"CONTROLLER_REFRESH_STATUS",
			"CONTROLLER_REFRESH_READ_STATUS",
			"assert controller_refresh[",
			"assert controller_refresh_read[",
			"filesystem_noop",
			"latestReceipt",
			"controllerRefresh",
			"desiredStateSource",
			"const operatorContextControllerRefreshWriteRoute = () => 'POST /v1/controller/refresh';",
			"const requestManualControllerRefresh = () => fetch(operatorContextControllerRefreshRoute(), { method: 'POST' })",
			"Refresh controller",
		} {
			if !strings.Contains(text, fragment) {
				t.Fatalf("expected %s to lock controller-refresh filesystem-noop smoke fragment %q", script, fragment)
			}
		}
	}
}

func TestSmokeScriptsLockDeploymentEventsInvalidLimitReleaseGuard(t *testing.T) {
	for _, script := range []string{"smoke-local.sh", "smoke-docker.sh"} {
		body, err := os.ReadFile("../../scripts/" + script)
		if err != nil {
			t.Fatalf("read %s: %v", script, err)
		}
		text := string(body)
		for _, fragment := range []string{
			"/v1/overview/deployments/payments-api-prod/events?limit=0",
			"INVALID_DEPLOYMENT_EVENTS_LIMIT_STATUS",
			"invalid_deployment_events_limit.strip() == ",
			"invalid limit query",
		} {
			if !strings.Contains(text, fragment) {
				t.Fatalf("expected %s to lock invalid deployment-events limit smoke fragment %q", script, fragment)
			}
		}
	}
}

func TestSmokeScriptsPairCodeDeployDiagnosticEmbeddedShellLabels(t *testing.T) {
	labels := []string{
		"Rollback diagnostics",
		"Rollback deployment ID",
		"Rollback trigger deployment ID",
		"Rollback message",
		"Error diagnostics",
		"Error code",
		"Error message",
		"Alarm diagnostics",
		"Alarm status",
		"Alarm polling enabled",
		"Ignore alarm polling failures",
		"Alarms",
		"Deployment overview",
		"CodeDeploy instance or target lifecycle counts for the latest deployment.",
	}

	for _, script := range []string{"smoke-local.sh", "smoke-docker.sh"} {
		body, err := os.ReadFile("../../scripts/" + script)
		if err != nil {
			t.Fatalf("read %s: %v", script, err)
		}
		text := string(body)
		for _, label := range labels {
			if !strings.Contains(text, label) {
				t.Fatalf("expected %s to lock CodeDeploy diagnostic embedded-shell label %q", script, label)
			}
		}
	}
}

func TestSmokeScriptsAssertGuardedResourceEvidenceRequestIDHeaders(t *testing.T) {
	for _, script := range []string{"smoke-local.sh", "smoke-docker.sh"} {
		body, err := os.ReadFile("../../scripts/" + script)
		if err != nil {
			t.Fatalf("read %s: %v", script, err)
		}

		text := string(body)
		for _, fragment := range []string{
			"curl_status_body_request_id",
			"AUTH_UNAUTHENTICATED_RESOURCE_LOGS_REQUEST_ID",
			"AUTH_UNAUTHENTICATED_RESOURCE_INSPECTION_REQUEST_ID",
			"AUTH_FORBIDDEN_RESOURCE_LOGS_REQUEST_ID",
			"AUTH_FORBIDDEN_RESOURCE_INSPECTION_REQUEST_ID",
			"guarded resource evidence reads must include Request-Id",
		} {
			if !strings.Contains(text, fragment) {
				t.Fatalf("expected %s to lock guarded resource Request-Id smoke fragment %q", script, fragment)
			}
		}

		if script == "smoke-docker.sh" {
			for _, fragment := range []string{
				"worker_resource_inspection_request_id=$(assert_request_id_header",
				"WORKER_RESOURCE_INSPECTION_REQUEST_ID",
				"/v1/overview/deployments/worker-prod/resource-inspection",
			} {
				if !strings.Contains(text, fragment) {
					t.Fatalf("expected %s to correlate live-inspection Request-Id fragment %q", script, fragment)
				}
			}
		}
	}
}

func TestSmokeScriptsAssertRecentLogEmptyReasonReleaseGuard(t *testing.T) {
	for _, script := range []string{"smoke-local.sh", "smoke-docker.sh"} {
		body, err := os.ReadFile("../../scripts/" + script)
		if err != nil {
			t.Fatalf("read %s: %v", script, err)
		}

		text := string(body)
		for _, fragment := range []string{
			"def assert_recent_log_empty_reason_contract(label, payload):",
			"no_recent_events",
			"no_formatted_entries",
			"emptyReason",
			"assert_recent_log_empty_reason_contract(label, payload)",
			"const resourceLogsEmptyReasonText = (reason) => {",
			"Tenet reached the recent-log provider, but no events were found in the selected bounded window.",
			"Tenet found provider records, but none could be formatted into operator-visible recent-log rows.",
		} {
			if !strings.Contains(text, fragment) {
				t.Fatalf("expected %s to lock recent-log empty-reason release guard fragment %q", script, fragment)
			}
		}
	}
}

func TestSmokeDockerScriptLocksOperatorContextRoleDiagnostics(t *testing.T) {
	body, err := os.ReadFile("../../scripts/smoke-docker.sh")
	if err != nil {
		t.Fatalf("read smoke-docker.sh: %v", err)
	}

	text := string(body)
	for _, fragment := range []string{
		"-H \"X-Tenet-Authenticated-Roles: viewer, auditor\"",
		"operator_context_unsupported_roles_path=$(write_payload operator-context-unsupported-roles.json \"$operator_context_unsupported_roles\")",
		"canonical_identity_headers = [\"X-Tenet-Authenticated-Id\", \"X-Tenet-Authenticated-Name\", \"X-Tenet-Authenticated-Email\"]",
		"alias_identity_headers = [\"X-User-ID\", \"X-User-Name\", \"X-User-Email\"]",
		"canonical_actor_headers = [\"X-Tenet-Actor-Id\", \"X-Tenet-Actor-Name\", \"X-Tenet-Actor-Email\"]",
		"alias_actor_headers = [\"X-User-ID\", \"X-User-Name\", \"X-User-Email\"]",
		"assert identity_header_support.get(\"canonical\") == canonical_identity_headers, payload",
		"assert identity_header_support.get(\"aliases\") == alias_identity_headers, payload",
		"assert actor_header_support.get(\"canonical\") == canonical_actor_headers, payload",
		"assert actor_header_support.get(\"aliases\") == alias_actor_headers, payload",
		"assert tenet_managed_login.get(\"sessionCookiePath\") == \"/\", payload",
		"assert tenet_managed_login.get(\"sessionCookieHttpOnly\") is True, payload",
		"assert tenet_managed_login.get(\"sessionCookieSameSite\") == \"Lax\", payload",
		"assert tenet_managed_login.get(\"sessionCookieSecureOverride\") is False, payload",
		"assert tenet_managed_login.get(\"sessionCookieSecureForRequest\") is False, payload",
		"operator_context_actor_precedence_path=$(write_payload operator-context-actor-precedence.json \"$operator_context_actor_precedence\")",
		"assert operator_context_actor_precedence[\"actor\"][\"id\"] == \"explicit-789\", operator_context_actor_precedence",
		"assert operator_context_alias[\"actorProvenance\"] == {\"idSource\": \"alias\", \"nameSource\": \"alias\", \"emailSource\": \"alias\"}, operator_context_alias",
		"assert operator_context_unsupported_roles[\"roles\"][\"recognizedNames\"] == [\"viewer\"], operator_context_unsupported_roles",
		"assert operator_context_unsupported_roles[\"roles\"][\"unrecognizedNames\"] == [\"auditor\"], operator_context_unsupported_roles",
		"operatorContextSupportedRolesLabel(context)",
		"const operatorContextSupportedIdentityHeadersLabel = (context) => {",
		"Supported identity headers: canonical ${canonicalText}; compatibility aliases ${aliasText}. Canonical X-Tenet-Authenticated-* identity headers are preferred when both canonical and alias values are present.",
		"operatorContextSupportedIdentityHeadersLabel(context)",
		"const operatorContextSupportedRoleHeadersLabel = (context) => {",
		"Supported role headers: ${supportedHeaders.join(', ')} (role forwarding has no X-User-* alias)",
		"operatorContextSupportedRoleHeadersLabel(context)",
		"const operatorContextSupportedDeploymentAccessHeadersLabel = (context) => {",
		"Supported deployment access headers: ${supportedHeaders.join(', ')}",
		"operatorContextSupportedDeploymentAccessHeadersLabel(context)",
		"const operatorContextDesiredStateSourceEntries = (context) => {",
		"Desired-state source:",
		"Controller refresh:",
		"const operatorContextDesiredStateReadinessLabel = (context) => {",
		"Desired state:",
		"operatorContextDesiredStateReadinessLabel(context)",
		"operatorContextDesiredStateReadinessClass(context)",
		"Webhook match statuses",
		"Webhook ignored reasons",
		"Diagnostic only; no webhook is sent from this page.",
		"operatorContextDesiredStateSourceEntries(context)",
		"canonical X-Tenet-Authenticated-Roles",
		"\"Unsupported forwarded roles ignored for capability checks:\"",
	} {
		if !strings.Contains(text, fragment) {
			t.Fatalf("expected smoke-docker.sh to lock operator-context role diagnostic fragment %q", fragment)
		}
	}
}

func TestSmokeDockerScriptLocksFreshnessMarkers(t *testing.T) {
	body, err := os.ReadFile("../../scripts/smoke-docker.sh")
	if err != nil {
		t.Fatalf("read smoke-docker.sh: %v", err)
	}

	text := string(body)
	for _, fragment := range []string{
		"\"applications-overview-freshness\",",
		"\"workspace-detail-freshness\",",
		"\"deployment-events-freshness\",",
	} {
		if !strings.Contains(text, fragment) {
			t.Fatalf("expected smoke-docker.sh to lock freshness marker fragment %q", fragment)
		}
	}
}

func TestSmokeLocalScriptLocksOperatorContextRoleDiagnostics(t *testing.T) {
	body, err := os.ReadFile("../../scripts/smoke-local.sh")
	if err != nil {
		t.Fatalf("read smoke-local.sh: %v", err)
	}

	text := string(body)
	for _, fragment := range []string{
		"assert \"const operatorContextSupportedIdentityHeadersLabel = (context) => {\" in ui, ui",
		"assert \"Supported identity headers: canonical ${canonicalText}; compatibility aliases ${aliasText}. Canonical X-Tenet-Authenticated-* identity headers are preferred when both canonical and alias values are present.\" in ui, ui",
		"assert 'operatorContextSupportedIdentityHeadersLabel(context)' in ui, ui",
		"assert \"const operatorContextSupportedRoleHeadersLabel = (context) => {\" in ui, ui",
		"assert \"Supported role headers: ${supportedHeaders.join(', ')} (role forwarding has no X-User-* alias)\" in ui, ui",
		"assert 'operatorContextSupportedRoleHeadersLabel(context)' in ui, ui",
		"canonical_identity_headers = ['X-Tenet-Authenticated-Id', 'X-Tenet-Authenticated-Name', 'X-Tenet-Authenticated-Email']",
		"alias_identity_headers = ['X-User-ID', 'X-User-Name', 'X-User-Email']",
		"canonical_actor_headers = ['X-Tenet-Actor-Id', 'X-Tenet-Actor-Name', 'X-Tenet-Actor-Email']",
		"alias_actor_headers = ['X-User-ID', 'X-User-Name', 'X-User-Email']",
		"assert identity_header_support.get('canonical') == canonical_identity_headers, payload",
		"assert identity_header_support.get('aliases') == alias_identity_headers, payload",
		"assert actor_header_support.get('canonical') == canonical_actor_headers, payload",
		"assert actor_header_support.get('aliases') == alias_actor_headers, payload",
		"assert tenet_managed_login.get('sessionCookiePath') == '/', payload",
		"assert tenet_managed_login.get('sessionCookieHttpOnly') is True, payload",
		"assert tenet_managed_login.get('sessionCookieSameSite') == 'Lax', payload",
		"assert tenet_managed_login.get('sessionCookieSecureOverride') is False, payload",
		"assert tenet_managed_login.get('sessionCookieSecureForRequest') is False, payload",
		"operator_context_actor_precedence_path=$(write_payload operator-context-actor-precedence.json \"$operator_context_actor_precedence\")",
		"assert operator_context_actor_precedence['actor']['id'] == 'explicit-789', operator_context_actor_precedence",
		"assert operator_context_alias['actorProvenance'] == {'idSource': 'alias', 'nameSource': 'alias', 'emailSource': 'alias'}, operator_context_alias",
		"assert \"const operatorContextSupportedDeploymentAccessHeadersLabel = (context) => {\" in ui, ui",
		"assert \"Supported deployment access headers: ${supportedHeaders.join(', ')}\" in ui, ui",
		"assert 'operatorContextSupportedDeploymentAccessHeadersLabel(context)' in ui, ui",
		"assert \"const operatorContextDesiredStateReadinessLabel = (context) => {\" in ui, ui",
		"assert \"Desired state:\" in ui, ui",
		"assert \"operatorContextDesiredStateReadinessLabel(context)\" in ui, ui",
		"assert \"operatorContextDesiredStateReadinessClass(context)\" in ui, ui",
		"assert 'canonical X-Tenet-Authenticated-Roles' in ui, ui",
	} {
		if !strings.Contains(text, fragment) {
			t.Fatalf("expected smoke-local.sh to lock operator-context role diagnostic fragment %q", fragment)
		}
	}
}

func TestSmokeLocalScriptAssertsRecentLogsInEmbeddedHTML(t *testing.T) {
	body, err := os.ReadFile("../../scripts/smoke-local.sh")
	if err != nil {
		t.Fatalf("read smoke-local.sh: %v", err)
	}

	text := string(body)
	if !strings.Contains(text, "assert 'Recent logs' in ui, ui") {
		t.Fatalf("expected smoke-local.sh to assert embedded html contains Recent logs")
	}
	if !strings.Contains(text, "assert 'Recent logs request' in ui, ui") {
		t.Fatalf("expected smoke-local.sh to assert embedded html contains Recent logs request")
	}
	if !strings.Contains(text, "assert 'Live inspection request' in ui, ui") {
		t.Fatalf("expected smoke-local.sh to assert embedded html contains Live inspection request")
	}
	if !strings.Contains(text, "assert 'Refresh recent logs' in ui, ui") && !strings.Contains(text, "assert \"Refresh recent logs\" in ui, ui") {
		t.Fatalf("expected smoke-local.sh to assert embedded html contains Refresh recent logs")
	}
	if !strings.Contains(text, "assert 'recent-log-lookup-context-card' in ui, ui") {
		t.Fatalf("expected smoke-local.sh to assert embedded html contains recent-log lookup context card")
	}
	if !strings.Contains(text, "No rows matched the current Recent logs window. Increase the Log limit, refresh after deployment activity, or use the lookup context above before escalating.") {
		t.Fatalf("expected smoke-local.sh to assert actionable recent-log empty-state guidance")
	}
	for _, fragment := range []string{
		"assert \"const resourceLogsLimitOptions = [\" in ui, ui",
		"assert \"Log entry cap · ${resourceLogsLimitText(limit)}\" in ui, ui",
		"assert \"'Log limit'\" in ui, ui",
		"assert \"params.set('limit', String(normalizedLimit));\" in ui, ui",
		"assert \"const markResourcePanelStatusNode = (node) => {\" in ui, ui",
		"assert \"markResourcePanelStatusNode(stateNode);\" in ui, ui",
		"assert \"node.setAttribute('role', 'status');\" in ui, ui",
		"assert \"node.setAttribute('aria-live', 'polite');\" in ui, ui",
		"assert \"node.setAttribute('aria-atomic', 'true');\" in ui, ui",
	} {
		if !strings.Contains(text, fragment) {
			t.Fatalf("expected smoke-local.sh to assert recent-log limit shell fragment %q", fragment)
		}
	}
	for _, fragment := range []string{
		`invalid_resource_log_instance_status=$(curl -sS -o "$INVALID_RESOURCE_LOG_INSTANCE_BODY"`,
		`resource-logs?resourceType=userDataBootstrapSource&resourceName=git%3Auserdata%2Fpayments-api-prod.sh&instanceId=i-0abc123def4567890`,
		`resource-logs?resourceType=userDataBootstrapSource&resourceName=git%3Auserdata%2Fpayments-api-prod.sh&instanceId=i-`,
		`assert selected_bootstrap_resource_logs.get('instanceId') == 'i-0abc123def4567890'`,
		`assert os.environ['INVALID_RESOURCE_LOG_INSTANCE_STATUS'] == '400'`,
		`assert invalid_resource_log_instance.strip() == 'invalid instanceId query'`,
		`selected_bootstrap_resource_inspection=$(curl -fsS`,
		`resource-inspection?resourceType=userDataBootstrapSource&resourceName=git%3Auserdata%2Fpayments-api-prod.sh&instanceId=i-0abc123def4567890`,
		`selected_candidate_resource_inspection=$(curl -fsS`,
		`assert selected_bootstrap_resource_inspection.get('instanceId') == 'i-0abc123def4567890'`,
		`selected_instance = payload.get('selectedInstance')`,
		`resource-inspection?resourceType=candidateAutoScalingGroup&resourceName=payments-api-prod-candidate&instanceId=i-0abc123def4567890`,
		`assert selected_candidate_resource_inspection.get('instanceId') == 'i-0abc123def4567890'`,
		`assert os.environ['INVALID_RESOURCE_INSPECTION_INSTANCE_STATUS'] == '400'`,
		`assert invalid_resource_inspection_instance.strip() == 'invalid instanceId query'`,
		`invalid_resource_inspection_limit_status=$(curl -sS -o "$INVALID_RESOURCE_INSPECTION_LIMIT_BODY"`,
		`resource-inspection?resourceType=candidateAutoScalingGroup&resourceName=payments-api-prod-candidate&limit=0`,
		`assert os.environ['INVALID_RESOURCE_INSPECTION_LIMIT_STATUS'] == '400'`,
		`assert invalid_resource_inspection_limit.strip() == 'invalid limit query'`,
	} {
		if !strings.Contains(text, fragment) {
			t.Fatalf("expected smoke-local.sh to assert ASG selector/limit fragment %q", fragment)
		}
	}

	if !strings.Contains(text, "assert 'Backing resource type' in ui, ui") {
		t.Fatalf("expected smoke-local.sh to assert embedded html contains Backing resource type")
	}
	if !strings.Contains(text, "assert 'Backing resource name' in ui, ui") {
		t.Fatalf("expected smoke-local.sh to assert embedded html contains Backing resource name")
	}
	if !strings.Contains(text, "assert 'Live inspection' in ui, ui") {
		t.Fatalf("expected smoke-local.sh to assert embedded html contains Live inspection")
	}
	if !strings.Contains(text, "assert 'Refresh live inspection' in ui, ui") {
		t.Fatalf("expected smoke-local.sh to assert embedded html contains Refresh live inspection")
	}
	for _, fragment := range []string{
		`assert 'Container instance' in ui, ui`,
		`assert 'EC2 instance' in ui, ui`,
		`assert 'Capacity provider' in ui, ui`,
		`assert 'Availability Zone' in ui, ui`,
		`assert 'EC2 host details' in ui, ui`,
		`assert 'Optional host evidence resolved from the inspected ECS task.' in ui, ui`,
		`assert 'EC2 state' in ui, ui`,
		`assert 'EC2 instance type' in ui, ui`,
		`assert 'AMI image ID' in ui, ui`,
		`assert 'Private IP' in ui, ui`,
		`assert 'Public IP' in ui, ui`,
		`assert 'Launch time' in ui, ui`,
		`assert "const ecsTaskScopeUnavailableReasonText = (reason) => {" in ui, ui`,
		`assert "Task sample unavailable · ${reasonText} · requested ${countText(scope.requestedLimit)} · effective ${countText(effectiveLimit)}" in ui, ui`,
		`assert "Task sampling is unavailable for this service; service inspection remains available." in ui, ui`,
		`assert "const createECSTaskScopeSummary = (scope) => {" in ui, ui`,
		`assert "Task sample scope · returned ${countText(returnedTaskCount)} task(s) from effective limit ${countText(effectiveLimit)} · ${truncationStatus}" in ui, ui`,
		`assert "[createECSTaskScopeSummary(service?.tasksScope)]" in ui, ui`,
	} {
		if !strings.Contains(text, fragment) {
			t.Fatalf("expected smoke-local.sh to assert ECS task sample scope shell fragment %q", fragment)
		}
	}
	if !strings.Contains(text, "assert 'Lookup path ·' in ui, ui") {
		t.Fatalf("expected smoke-local.sh to assert embedded html contains Lookup path summary")
	}
	if !strings.Contains(text, "assert 'Lookup region' in ui, ui") {
		t.Fatalf("expected smoke-local.sh to assert embedded html contains Lookup region")
	}
	for _, fragment := range []string{
		`assert_recent_log_empty_reason_contract(label, payload)`,
		`assert "const resourceLogsEmptyReasonText = (reason) => {" in ui, ui`,
		`assert 'Tenet reached the recent-log provider, but no events were found in the selected bounded window.' in ui, ui`,
		`assert 'Tenet found provider records, but none could be formatted into operator-visible recent-log rows.' in ui, ui`,
	} {
		if !strings.Contains(text, fragment) {
			t.Fatalf("expected smoke-local.sh to assert recent-log empty-reason shell fragment %q", fragment)
		}
	}
	if !strings.Contains(text, "assert 'Observer role configured' in ui, ui") {
		t.Fatalf("expected smoke-local.sh to assert embedded html contains Observer role configured")
	}
	if !strings.Contains(text, "assert 'Controller runtime events' in ui, ui") {
		t.Fatalf("expected smoke-local.sh to assert embedded html contains controller runtime events")
	}
	if !strings.Contains(text, "assert 'Refresh runtime events' in ui, ui") {
		t.Fatalf("expected smoke-local.sh to assert embedded html contains Refresh runtime events")
	}
	if !strings.Contains(text, "assert 'Copy runtime events route' in ui, ui") || !strings.Contains(text, "assert \"deploymentEventsReadRoute(deploymentName, currentPageState.eventLimit, currentPageState.eventSource)\" in ui, ui") {
		t.Fatalf("expected smoke-local.sh to assert runtime events copy-route handoff")
	}
	if !strings.Contains(text, "assert 'Refresh drift workspace' in ui, ui") {
		t.Fatalf("expected smoke-local.sh to assert embedded html contains Refresh drift workspace")
	}
	if !strings.Contains(text, "assert 'Refresh applications overview' in ui, ui") {
		t.Fatalf("expected smoke-local.sh to assert embedded html contains Refresh applications overview")
	}
	if !strings.Contains(text, "assert 'Applications JSON' in ui, ui") {
		t.Fatalf("expected smoke-local.sh to assert embedded html contains applications overview JSON handoff")
	}
	if !strings.Contains(text, "assert \"createNavLink('/v1/overview/applications', 'Applications JSON')\" in ui, ui") {
		t.Fatalf("expected smoke-local.sh to assert applications overview JSON route contract")
	}
	if !strings.Contains(text, "assert 'Copy read route' in ui, ui") {
		t.Fatalf("expected smoke-local.sh to assert embedded html contains copy read route controls")
	}
	if !strings.Contains(text, "assert \"createCopyReadRouteButton(applicationsOverviewReadRoute())\" in ui, ui") {
		t.Fatalf("expected smoke-local.sh to assert applications overview copy route contract")
	}
	if !strings.Contains(text, "assert \"return copyTextToClipboard(routeText);\" in ui, ui") {
		t.Fatalf("expected smoke-local.sh to assert copy read route helper delegates to shared clipboard helper")
	}
	if !strings.Contains(text, "assert \"navigator.clipboard.writeText(text)\" in ui, ui") {
		t.Fatalf("expected smoke-local.sh to assert shared clipboard write contract")
	}
	if !strings.Contains(text, "assert 'Workspace JSON' in ui, ui") {
		t.Fatalf("expected smoke-local.sh to assert embedded html contains workspace JSON handoff")
	}
	if !strings.Contains(text, "assert \"createNavLink(deploymentDetailJSONRoute(deploymentName), 'Workspace JSON')\" in ui, ui") {
		t.Fatalf("expected smoke-local.sh to assert workspace JSON route contract")
	}
	if !strings.Contains(text, "assert \"container.append(createElement('strong', '', 'Latest runtime change'));\" in ui, ui") {
		t.Fatalf("expected smoke-local.sh to assert embedded html contains compact latest runtime change label")
	}
	if !strings.Contains(text, "assert \"['Project', textOrDash(deployment.appRef)]\" in ui, ui") {
		t.Fatalf("expected smoke-local.sh to assert embedded html contains row-local Project field")
	}
	if !strings.Contains(text, "assert \"['Last update', runtimeSummaryTimestamp(deployment.runtimeSummary)]\" in ui, ui") {
		t.Fatalf("expected smoke-local.sh to assert embedded html contains row-local Last update field")
	}
	if !strings.Contains(text, "assert \"const latestRuntimeChangeParts = (deployment) => {\" in ui, ui") {
		t.Fatalf("expected smoke-local.sh to assert embedded html contains latest runtime change helper contract")
	}
	if !strings.Contains(text, "application-runtime-events") {
		t.Fatalf("expected smoke-local.sh to assert embedded html contains runtime-events handoff anchor")
	}
	if !strings.Contains(text, "assert 'Refreshing applications overview from GET /v1/overview/applications…' in ui, ui") {
		t.Fatalf("expected smoke-local.sh to assert embedded html contains applications overview refresh loading copy")
	}
	if !strings.Contains(text, "assert 'Failed to refresh applications overview:' in ui, ui") {
		t.Fatalf("expected smoke-local.sh to assert embedded html contains applications overview refresh error copy")
	}
	if !strings.Contains(text, "assert 'Refresh account detail' in account_ui, account_ui") {
		t.Fatalf("expected smoke-local.sh to assert embedded html contains Refresh account detail")
	}
	if !strings.Contains(text, "assert 'Copy account read route' in account_ui, account_ui") || !strings.Contains(text, "assert 'Copy group read route' in account_ui, account_ui") {
		t.Fatalf("expected smoke-local.sh to assert account page copyable read-route controls")
	}
	if !strings.Contains(text, "assert \"accountLinks.append(createCopyReadRouteButton(accountDetailReadRoute(account.awsAccountId), 'Copy account read route'));\" in account_ui, account_ui") {
		t.Fatalf("expected smoke-local.sh to assert account read-route copy control placement")
	}
	if !strings.Contains(text, "assert \"groupLinks.append(createCopyReadRouteButton(accountGroupDetailReadRoute(account.awsAccountId, group), 'Copy group read route'));\" in account_ui, account_ui") {
		t.Fatalf("expected smoke-local.sh to assert group read-route copy control placement")
	}
	if !strings.Contains(text, "assert \"const accountGroupDetailReadRoute = (accountID, group) => {\" in account_ui, account_ui") {
		t.Fatalf("expected smoke-local.sh to assert group read-route helper contract")
	}
	if !strings.Contains(text, "assert \"createAccountDetailActions(accountState).forEach((action) => {\" in account_ui, account_ui") {
		t.Fatalf("expected smoke-local.sh to assert account refresh toolbar action wiring")
	}
	if !strings.Contains(text, "assert \"toolbar.replaceChildren(toolbarActions);\" in account_ui, account_ui") {
		t.Fatalf("expected smoke-local.sh to assert account toolbar action container")
	}
	if !strings.Contains(text, "assert 'Refresh application detail' in application_ui, application_ui") {
		t.Fatalf("expected smoke-local.sh to assert embedded html contains Refresh application detail")
	}
	if !strings.Contains(text, "assert 'Application JSON' in application_ui, application_ui") {
		t.Fatalf("expected smoke-local.sh to assert embedded html contains application detail JSON handoff")
	}
	if !strings.Contains(text, "assert \"createNavLink(`/v1/overview/applications/${encodeURIComponent(applicationState?.detail?.name || currentPageState.deployment)}`, 'Application JSON')\" in application_ui, application_ui") {
		t.Fatalf("expected smoke-local.sh to assert application detail JSON route contract")
	}
	if !strings.Contains(text, "assert \"createCopyReadRouteButton(applicationDetailReadRoute(applicationState?.detail?.name || currentPageState.deployment))\" in application_ui, application_ui") {
		t.Fatalf("expected smoke-local.sh to assert application detail copy route contract")
	}
	if !strings.Contains(text, "assert 'Grouped linked resources for the selected application. Open drift, plan/apply, or audit log from here before the final resource page.' in application_ui, application_ui") {
		t.Fatalf("expected smoke-local.sh to assert embedded html contains complete application workspace handoff copy")
	}
	if !strings.Contains(text, "assert 'Refresh resource detail' in body, body") {
		t.Fatalf("expected smoke-local.sh to assert embedded html contains Refresh resource detail")
	}
	if !strings.Contains(text, "assert 'Resource JSON' in body, body") {
		t.Fatalf("expected smoke-local.sh to assert embedded html contains resource detail JSON handoff")
	}
	if !strings.Contains(text, "assert \"createNavLink(`/v1/overview/deployments/${encodeURIComponent(detail?.name || resourceState?.detail?.name || currentPageState.deployment)}`, 'Resource JSON')\" in body, body") {
		t.Fatalf("expected smoke-local.sh to assert resource detail JSON route contract")
	}
	if !strings.Contains(text, "assert \"createCopyReadRouteButton(resourceDetailReadRoute(detail?.name || resourceState?.detail?.name || currentPageState.deployment))\" in body, body") {
		t.Fatalf("expected smoke-local.sh to assert resource detail copy route contract")
	}
	if !strings.Contains(text, "assert \"workspace-callout-factors resource-read-route resource-current-read-cue\" in body, body") {
		t.Fatalf("expected smoke-local.sh to assert resource detail read route body cue")
	}
	if !strings.Contains(text, "assert \"const selectedResourceEvidenceCalloutText = (supportsInspection, supportsLogs, routeBits) => {\" in body, body") {
		t.Fatalf("expected smoke-local.sh to assert capability-aware selected-resource evidence copy")
	}
	if !strings.Contains(text, "assert 'Refreshing resource detail from GET /v1/overview/deployments/{deploymentName}…' in body, body") {
		t.Fatalf("expected smoke-local.sh to assert embedded html contains resource detail refresh loading copy")
	}
	if !strings.Contains(text, "assert 'Current resource detail remains visible below.' in body, body") {
		t.Fatalf("expected smoke-local.sh to assert embedded html contains resource detail refresh preserved-detail copy")
	}
	if !strings.Contains(text, "assert 'Failed to refresh resource detail:' in body, body") {
		t.Fatalf("expected smoke-local.sh to assert embedded html contains resource detail refresh error copy")
	}
	if !strings.Contains(text, "assert 'Clear drift filters' in ui, ui") {
		t.Fatalf("expected smoke-local.sh to assert embedded html contains Clear drift filters")
	}
	if !strings.Contains(text, "assert \"params.set('driftQuery', normalized.query);\" in ui, ui") || !strings.Contains(text, "assert \"params.set('driftSeverity', normalized.severity);\" in ui, ui") || !strings.Contains(text, "assert \"params.set('driftSort', normalized.sort);\" in ui, ui") {
		t.Fatalf("expected smoke-local.sh to assert embedded html documents shareable drift filter URL params")
	}
	if !strings.Contains(text, "assert 'Linked resources' in ui, ui") {
		t.Fatalf("expected smoke-local.sh to assert embedded html contains drift linked-resources copy")
	}
	if !strings.Contains(text, "assert 'No linked resources are available in this workspace yet.' in ui, ui") {
		t.Fatalf("expected smoke-local.sh to assert embedded html contains workspace linked-resources empty-state copy")
	}
	if !strings.Contains(text, "assert \"const workspaceLabel = mode === 'plan' ? 'plan workspace' : 'drift workspace';\" in ui, ui") {
		t.Fatalf("expected smoke-local.sh to assert embedded html contains shared workspace refresh label logic")
	}
	if !strings.Contains(text, "assert 'Refreshing ${workspaceLabel} from GET /v1/overview/deployments/{deploymentName}…' in ui, ui") {
		t.Fatalf("expected smoke-local.sh to assert embedded html contains workspace refresh loading copy")
	}
	if !strings.Contains(text, "assert 'Failed to refresh ${workspaceLabel}:' in ui, ui") {
		t.Fatalf("expected smoke-local.sh to assert embedded html contains workspace refresh error copy")
	}
	if !strings.Contains(text, "assert 'Refreshing account detail from GET /v1/overview/accounts/{awsAccountId}…' in account_ui, account_ui") {
		t.Fatalf("expected smoke-local.sh to assert embedded html contains account refresh loading copy")
	}
	if !strings.Contains(text, "assert 'Refreshing application detail from GET /v1/overview/applications/{deploymentName}…' in application_ui, application_ui") {
		t.Fatalf("expected smoke-local.sh to assert embedded html contains application refresh loading copy")
	}
	if !strings.Contains(text, "assert 'Failed to refresh account detail:' in account_ui, account_ui") {
		t.Fatalf("expected smoke-local.sh to assert embedded html contains account refresh error copy")
	}
	if !strings.Contains(text, "assert 'Failed to refresh application detail:' in application_ui, application_ui") {
		t.Fatalf("expected smoke-local.sh to assert embedded html contains application refresh error copy")
	}
	if !strings.Contains(text, "application_ui=$(curl -fsS \"http://$HOST:$PORT/?page=application&deployment=payments-api-prod\")") {
		t.Fatalf("expected smoke-local.sh to fetch bookmarkable application page")
	}
	if !strings.Contains(text, "account_ui=$(curl -fsS \"http://$HOST:$PORT/?page=account&accountId=123456789012\")") {
		t.Fatalf("expected smoke-local.sh to fetch bookmarkable account page")
	}
	if !strings.Contains(text, "resource_ui=$(curl -fsS \"http://$HOST:$PORT/?page=resource&deployment=payments-api-prod&resourceType=trafficShiftPlan&resourceId=trafficShiftPlan%3A%3Apayments-api-prod-weighted-routing%3A%3AWeighted+routing+plan\")") {
		t.Fatalf("expected smoke-local.sh to fetch bookmarkable resource page")
	}
	if !strings.Contains(text, "ecs_resource_ui=$(curl -fsS \"http://$HOST:$PORT/?page=resource&deployment=worker-prod&resourceType=ecsService&resourceId=ecsService%3A%3Aworker-prod-service%3A%3AECS+service+Tenet+updates+with+native+rolling+deployment\")") {
		t.Fatalf("expected smoke-local.sh to fetch bookmarkable ecs resource page")
	}
	if !strings.Contains(text, "bootstrap_resource_ui=$(curl -fsS \"http://$HOST:$PORT/?page=resource&deployment=payments-api-prod&resourceType=userDataBootstrapSource&resourceId=userDataBootstrapSource%3A%3Afile%3Auserdata%2Fpayments-api-prod.sh%3A%3ANormalized+userdata+bootstrap+source+for+candidate+EC2+capacity\")") {
		t.Fatalf("expected smoke-local.sh to fetch bookmarkable bootstrap resource page")
	}
	if !strings.Contains(text, "codedeploy_application_resource_ui=$(curl -fsS \"http://$HOST:$PORT/?page=resource&deployment=payments-api-staging&resourceType=codeDeployApplication&resourceId=codeDeployApplication%3A%3Apayments-api-staging-codedeploy-app%3A%3ACodeDeploy+application+coordinating+ECS+traffic+shifting\")") {
		t.Fatalf("expected smoke-local.sh to fetch bookmarkable CodeDeploy application resource page")
	}
	if !strings.Contains(text, "blue_task_set_target_group_resource_ui=$(curl -fsS \"http://$HOST:$PORT/?page=resource&deployment=payments-api-staging&resourceType=blueTaskSetTargetGroup&resourceId=blueTaskSetTargetGroup%3A%3Apayments-api-staging-blue%3A%3ABlue+task+set+target+group+CodeDeploy+treats+as+the+live+ECS+traffic+destination\")") {
		t.Fatalf("expected smoke-local.sh to fetch bookmarkable CodeDeploy blue task-set target-group resource page")
	}
	if !strings.Contains(text, "production_listener_rule_resource_ui=$(curl -fsS \"http://$HOST:$PORT/?page=resource&deployment=payments-api-prod&resourceType=productionListenerRule&resourceId=productionListenerRule%3A%3Apayments-api-prod-prod-listener%3A%3AProduction+listener+rule+operators+inspect+while+Tenet+updates+weighted+forwarding+during+rollout\")") {
		t.Fatalf("expected smoke-local.sh to fetch bookmarkable production listener-rule resource page")
	}
	if !strings.Contains(text, "codedeploy_application_resource_ui_path=$(write_payload codedeploy-application-resource-ui.html \"$codedeploy_application_resource_ui\")") || !strings.Contains(text, "blue_task_set_target_group_resource_ui_path=$(write_payload blue-task-set-target-group-resource-ui.html \"$blue_task_set_target_group_resource_ui\")") || !strings.Contains(text, "production_listener_rule_resource_ui_path=$(write_payload production-listener-rule-resource-ui.html \"$production_listener_rule_resource_ui\")") {
		t.Fatalf("expected smoke-local.sh to keep traffic-routing resource shells file-backed")
	}
	if !strings.Contains(text, "bootstrap_resource_logs=$(curl -fsS \"http://$HOST:$PORT/v1/overview/deployments/payments-api-prod/resource-logs?resourceType=userDataBootstrapSource&resourceName=git%3Auserdata%2Fpayments-api-prod.sh\")") {
		t.Fatalf("expected smoke-local.sh to fetch bootstrap-source recent logs payload")
	}
	if !strings.Contains(text, "active_target_group_resource_logs=$(curl -fsS \"http://$HOST:$PORT/v1/overview/deployments/payments-api-prod/resource-logs?resourceType=activeTargetGroup&resourceName=payments-api-prod-active\")") || !strings.Contains(text, "candidate_target_group_resource_logs=$(curl -fsS \"http://$HOST:$PORT/v1/overview/deployments/payments-api-prod/resource-logs?resourceType=candidateTargetGroup&resourceName=payments-api-prod-candidate\")") {
		t.Fatalf("expected smoke-local.sh to fetch weighted target-group recent logs payloads")
	}
	if !strings.Contains(text, "bootstrap_resource_inspection=$(curl -fsS \"http://$HOST:$PORT/v1/overview/deployments/payments-api-prod/resource-inspection?resourceType=userDataBootstrapSource&resourceName=git%3Auserdata%2Fpayments-api-prod.sh\")") {
		t.Fatalf("expected smoke-local.sh to fetch bootstrap-source inspection payload")
	}
	if !strings.Contains(text, "assert \"const fetchApplicationDetail = (deploymentName) => fetchEmbeddedReadRouteJSON(`/v1/overview/applications/${encodeURIComponent(deploymentName)}`, {\" in application_ui, application_ui") {
		t.Fatalf("expected smoke-local.sh to assert application page fetch route contract")
	}
	if !strings.Contains(text, "assert \"const fetchApplicationsOverview = () => fetchEmbeddedReadRouteJSON('/v1/overview/applications', {\" in ui, ui") {
		t.Fatalf("expected smoke-local.sh to assert applications landing page fetch route contract")
	}
	if !strings.Contains(text, "assert \"fetchApplicationsOverview()\" in ui, ui") {
		t.Fatalf("expected smoke-local.sh to assert landing-page boot loader uses fetchApplicationsOverview")
	}
	if !strings.Contains(text, "assert_fragments(\"account_ui\", account_ui, operator_read_auth_fragments + (") {
		t.Fatalf("expected smoke-local.sh to lock account page operator-read auth guidance")
	}
	if !strings.Contains(text, "assert \"fetchAccountDetail(currentPageState.accountId)\" in account_ui, account_ui") {
		t.Fatalf("expected smoke-local.sh to assert account-page boot loader uses fetchAccountDetail")
	}
	if !strings.Contains(text, "assert 'Quick navigation' in account_ui, account_ui") {
		t.Fatalf("expected smoke-local.sh to assert account page quick navigation copy")
	}
	if !strings.Contains(text, "assert 'Applications overview' in account_ui, account_ui") {
		t.Fatalf("expected smoke-local.sh to assert account page overview quick-nav copy")
	}
	if !strings.Contains(text, "assert 'Current destination' in account_ui, account_ui") {
		t.Fatalf("expected smoke-local.sh to assert account page current-destination quick-nav copy")
	}
	if !strings.Contains(text, "assert 'Current section' in account_ui, account_ui") {
		t.Fatalf("expected smoke-local.sh to assert account page current-section quick-nav copy")
	}
	if !strings.Contains(text, "assert 'No section is currently targeted.' in account_ui, account_ui") {
		t.Fatalf("expected smoke-local.sh to assert account page empty current-section copy")
	}
	if !strings.Contains(text, "assert 'On this page' in account_ui, account_ui") {
		t.Fatalf("expected smoke-local.sh to assert account page section quick-nav copy")
	}
	if !strings.Contains(text, "assert \"const createAccountQuickNav = (account, sectionLinks = []) => {\" in account_ui, account_ui") {
		t.Fatalf("expected smoke-local.sh to assert account page quick-nav helper contract")
	}
	if !strings.Contains(text, "assert \"createNavLink(buildPageURL('applications'), 'Applications overview', '_self')\" in account_ui, account_ui") {
		t.Fatalf("expected smoke-local.sh to assert account page overview return-link contract")
	}
	if !strings.Contains(text, "assert \"createQuickNavChip('Account detail', 'Current destination: Account detail', 'page')\" in account_ui, account_ui") {
		t.Fatalf("expected smoke-local.sh to assert account page current-destination chip contract")
	}
	if !strings.Contains(text, "assert \"createQuickNavChip(currentSectionLink.label, `Current section: ${currentSectionLink.label}`, 'location')\" in account_ui, account_ui") {
		t.Fatalf("expected smoke-local.sh to assert account page current-section chip contract")
	}
	if !strings.Contains(text, "assert \"detailSummary.id = 'account-summary';\" in account_ui, account_ui") {
		t.Fatalf("expected smoke-local.sh to assert account summary anchor contract")
	}
	if !strings.Contains(text, "assert \"groupSection.id = accountGroupSectionID(group);\" in account_ui, account_ui") {
		t.Fatalf("expected smoke-local.sh to assert account group anchor contract")
	}
	if !strings.Contains(text, "assert \"href: buildPageSectionURL('account', sectionID, { accountId: accountID }),\" in account_ui, account_ui") {
		t.Fatalf("expected smoke-local.sh to assert account section link builder contract")
	}
	if !strings.Contains(text, "for label, body in ((\"resource_ui\", resource_ui), (\"ecs_resource_ui\", ecs_resource_ui), (\"bootstrap_resource_ui\", bootstrap_resource_ui), (\"codedeploy_application_resource_ui\", codedeploy_application_resource_ui), (\"blue_task_set_target_group_resource_ui\", blue_task_set_target_group_resource_ui), (\"production_listener_rule_resource_ui\", production_listener_rule_resource_ui)):") {
		t.Fatalf("expected smoke-local.sh to assert all bookmarkable resource pages")
	}
	if !strings.Contains(text, "\"return 'Account detail';\",") || !strings.Contains(text, "\"return `GET /v1/overview/accounts/${accountID}`;\",") {
		t.Fatalf("expected smoke-local.sh to assert account-page operator read auth labels")
	}
	if !strings.Contains(text, "assert \"return 'Applications overview';\" in ui, ui") || !strings.Contains(text, "assert \"return 'GET /v1/overview/applications';\" in ui, ui") {
		t.Fatalf("expected smoke-local.sh to assert landing-page operator read auth labels")
	}
	if !strings.Contains(text, "assert \"const setEmbeddedBrowserTitle = (...parts) => {\" in ui, ui") || !strings.Contains(text, "assert \"document.title = [...titleParts, 'Tenet'].join(' · ');\" in ui, ui") {
		t.Fatalf("expected smoke-local.sh to assert embedded browser-title contract")
	}
	if !strings.Contains(text, "assert \"createNavLink(buildPageURL('resource', { deployment: detail.name, resourceType: resource.type, resourceId: resource.id }), 'Open resource', '_self')\" in application_ui, application_ui") {
		t.Fatalf("expected smoke-local.sh to assert application page resource link contract")
	}
	if !strings.Contains(text, "assert \"const resourceSupportsLiveInspection = (resource) => {\" in application_ui, application_ui") {
		t.Fatalf("expected smoke-local.sh to assert application page live inspection support helper")
	}
	if !strings.Contains(text, "assert \"const resourceSupportsRecentLogs = (resource) => {\" in application_ui, application_ui") || !strings.Contains(text, "assert \"'codeDeployApplication',\\n          'codeDeployDeploymentGroup'\" in application_ui, application_ui") || !strings.Contains(text, "assert \"'trafficShiftPlan',\\n          'productionListenerRule',\\n          'activeTargetGroup',\\n          'candidateTargetGroup'\" in application_ui, application_ui") {
		t.Fatalf("expected smoke-local.sh to assert application page recent logs support helper and route-stable resource types")
	}
	if !strings.Contains(text, "assert \"createNavLink(buildPageSectionURL('resource', 'resource-live-inspection', { deployment: deploymentName, resourceType: resource.type, resourceId: resource.id }), 'Live evidence available', '_self')\" in application_ui, application_ui") {
		t.Fatalf("expected smoke-local.sh to assert application page live inspection handoff contract")
	}
	if !strings.Contains(text, "assert \"createNavLink(buildPageSectionURL('resource', 'resource-recent-logs', { deployment: deploymentName, resourceType: resource.type, resourceId: resource.id }), 'Recent logs supported', '_self')\" in application_ui, application_ui") {
		t.Fatalf("expected smoke-local.sh to assert application page recent logs handoff contract")
	}
	if !strings.Contains(text, "assert \"const rolloutResourceDetailURL = (detail, resourceTypes, identifiers) => {\" in application_ui, application_ui") || !strings.Contains(text, "assert \"return href ? createNavLink(href, 'Open resource detail', '_self') : null;\" in application_ui, application_ui") {
		t.Fatalf("expected smoke-local.sh to assert traffic-routing resource-detail handoff contract")
	}
	if !strings.Contains(text, "assert 'Evidence cues' in application_ui, application_ui") || !strings.Contains(text, "assert 'Live evidence available' in application_ui, application_ui") || !strings.Contains(text, "assert 'Recent logs supported' in application_ui, application_ui") {
		t.Fatalf("expected smoke-local.sh to assert application page evidence handoff copy")
	}
	if !strings.Contains(text, "assert \"const applicationResourceSectionLinkLabel = (group, resourceFilter = '') => {\" in application_ui, application_ui") {
		t.Fatalf("expected smoke-local.sh to assert application page filters empty linked-resource groups")
	}
	if !strings.Contains(text, "assert 'No linked resources are currently reported for this application. Use controller runtime events below to inspect the latest rollout signals.' in application_ui, application_ui") {
		t.Fatalf("expected smoke-local.sh to assert application page empty-state copy")
	}
	if !strings.Contains(text, "assert 'Jump to runtime events' in application_ui, application_ui") || !strings.Contains(text, "assert \"emptyStateActions.append(createNavLink(buildPageSectionURL('application', 'application-runtime-events', { deployment: deploymentName }), 'Jump to runtime events', '_self'));\" in application_ui, application_ui") {
		t.Fatalf("expected smoke-local.sh to assert application page direct runtime-events handoff contract")
	}
	if !strings.Contains(text, "assert 'Quick navigation' in application_ui, application_ui") {
		t.Fatalf("expected smoke-local.sh to assert application page quick navigation copy")
	}
	if !strings.Contains(text, "assert 'Applications overview' in application_ui, application_ui") {
		t.Fatalf("expected smoke-local.sh to assert application page overview quick-nav copy")
	}
	if !strings.Contains(text, "assert 'Current destination' in application_ui, application_ui") {
		t.Fatalf("expected smoke-local.sh to assert application page current-destination quick-nav copy")
	}
	if !strings.Contains(text, "assert 'On this page' in application_ui, application_ui") {
		t.Fatalf("expected smoke-local.sh to assert application page section navigation copy")
	}
	if !strings.Contains(text, "assert \"const createApplicationQuickNav = (detail, sectionLinks = []) => {\" in application_ui, application_ui") {
		t.Fatalf("expected smoke-local.sh to assert application page quick-nav helper contract")
	}
	if !strings.Contains(text, "assert \"createNavLink(buildPageURL('applications'), 'Applications overview', '_self')\" in application_ui, application_ui") {
		t.Fatalf("expected smoke-local.sh to assert application page overview return-link contract")
	}
	if !strings.Contains(text, "assert \"createQuickNavChip('Application detail', 'Current destination: Application detail', 'page')\" in application_ui, application_ui") {
		t.Fatalf("expected smoke-local.sh to assert application page current-destination chip contract")
	}
	if !strings.Contains(text, "assert \"eventsSection.id = 'application-runtime-events';\" in application_ui, application_ui") {
		t.Fatalf("expected smoke-local.sh to assert application page runtime-events anchor contract")
	}
	if !strings.Contains(text, "for body in (resource_ui, ecs_resource_ui, bootstrap_resource_ui, codedeploy_application_resource_ui, blue_task_set_target_group_resource_ui, production_listener_rule_resource_ui):") {
		t.Fatalf("expected smoke-local.sh to iterate over all resource page payloads")
	}
	if !strings.Contains(text, "assert \"const bookmarkedResource = parseResourceIdentity(currentPageState.resourceId);\" in body, body") {
		t.Fatalf("expected smoke-local.sh to assert bookmarkable resource page contracts for both resource pages")
	}
	if !strings.Contains(text, "assert 'Opened via bookmarked resource alias.' in body, body") {
		t.Fatalf("expected smoke-local.sh to assert alias-aware resource page copy")
	}
	if !strings.Contains(text, "assert \"createNavLink(buildPageURL('application', { deployment: deploymentName }), 'Back to application', '_self')\" in body, body") {
		t.Fatalf("expected smoke-local.sh to assert resource page back-link contract for both resource pages")
	}
	if !strings.Contains(text, "assert \"const resolveBackingResourceDrillDown = (detail, inspectionDetail) => {\" in body, body") {
		t.Fatalf("expected smoke-local.sh to assert resource page backing-resource resolver contract")
	}
	if !strings.Contains(text, "assert \"const createBackingResourceHandoffSection = (detail, inspectionDetail) => {\" in body, body") {
		t.Fatalf("expected smoke-local.sh to assert resource page backing-resource handoff contract")
	}
	if !strings.Contains(text, "assert 'Resolved backing resource' in body, body") {
		t.Fatalf("expected smoke-local.sh to assert embedded html contains backing-resource handoff copy")
	}
	if !strings.Contains(text, "assert 'Current section' in body, body") {
		t.Fatalf("expected smoke-local.sh to assert resource page current-section copy")
	}
	if !strings.Contains(text, "assert \"createQuickNavChip(currentSectionLink.label, `Current section: ${currentSectionLink.label}`, 'location')\" in ui, ui") {
		t.Fatalf("expected smoke-local.sh to assert resource page current-section chip contract")
	}
	if !strings.Contains(text, "assert 'Audit timeline' in audit_ui, audit_ui") {
		t.Fatalf("expected smoke-local.sh to assert embedded html contains audit timeline copy")
	}
	if !strings.Contains(text, "assert 'Current audit read: ${buildEmbeddedPageReadRoute(currentPageState)}' in audit_ui, audit_ui") {
		t.Fatalf("expected smoke-local.sh to assert embedded html contains current audit read route copy")
	}
	if !strings.Contains(text, "assert 'Refresh audit log' in audit_ui, audit_ui") {
		t.Fatalf("expected smoke-local.sh to assert embedded html contains Refresh audit log")
	}
	if !strings.Contains(text, "assert 'Refreshing audit log from GET /v1/audit-logs…' in audit_ui, audit_ui") {
		t.Fatalf("expected smoke-local.sh to assert embedded html contains audit refresh loading copy")
	}
	if !strings.Contains(text, "assert 'Failed to refresh audit log:' in audit_ui, audit_ui") {
		t.Fatalf("expected smoke-local.sh to assert embedded html contains audit refresh error copy")
	}
	if !strings.Contains(text, "assert 'audit-freshness' in audit_ui, audit_ui") || !strings.Contains(text, "assert 'Last refreshed · ${fetchedAt}' in audit_ui, audit_ui") {
		t.Fatalf("expected smoke-local.sh to assert embedded html contains audit freshness copy")
	}
	for _, fragment := range []string{
		"json_string_field_from_path() {",
		"response_path=$(write_payload wait-for-apply-status.json \"$response\")",
		"current_status=$(json_string_field_from_path \"$response_path\" status)",
		"assert_request_id_header() {",
		"assert headers.get(\"request-id\"), headers",
		"assert_request_id_header \"http://$HOST:$PORT/healthz\"",
		"assert_request_id_header \"http://$HOST:$PORT/favicon.ico\"",
		"assert_request_id_header \"http://$HOST:$PORT/v1/catalog/summary\"",
		"assert_request_id_header \"http://$HOST:$PORT/v1/overview/deployments/payments-api-staging/resource-logs?resourceType=codeDeployDeploymentGroup&resourceName=payments-api-staging-codedeploy-group\"",
		"assert_request_id_header \"http://$HOST:$PORT/\"",
		"TENET_GITHUB_WEBHOOK_SECRET=\"$WEBHOOK_SECRET\"",
		"webhook_push=$(curl -fsS -X POST \\",
		"http://$HOST:$PORT/api/v1/github/webhook",
		"deployment_events_fixture=$(curl -fsS \"http://$HOST:$PORT/v1/overview/deployments/payments-api-prod/events\")",
		"resource_logs=$(curl -fsS \"http://$HOST:$PORT/v1/overview/deployments/payments-api-staging/resource-logs?resourceType=codeDeployDeploymentGroup&resourceName=payments-api-staging-codedeploy-group\")",
		"worker_resource_logs=$(curl -fsS \"http://$HOST:$PORT/v1/overview/deployments/worker-prod/resource-logs?resourceType=ecsService&resourceName=worker-prod-service\")",
		"ecs_task_resource_logs=$(curl -fsS \"http://$HOST:$PORT/v1/overview/deployments/worker-prod/resource-logs?resourceType=ecsTask&resourceName=$ecs_task_resource_name&containerName=worker&clusterArn=arn%3Aaws%3Aecs%3Aap-northeast-2%3A123456789012%3Acluster%2Fworker-prod&limit=1\")",
		"ecs_task_resource_inspection=$(curl -fsS \"http://$HOST:$PORT/v1/overview/deployments/worker-prod/resource-inspection?resourceType=ecsTask&resourceName=$ecs_task_resource_name&clusterArn=arn%3Aaws%3Aecs%3Aap-northeast-2%3A123456789012%3Acluster%2Fworker-prod\")",
		"assert deployment_events_fixture['deploymentName'] == 'payments-api-prod', deployment_events_fixture",
		"assert isinstance(deployment_events_fixture['events'], list), deployment_events_fixture",
		"assert len(deployment_events_fixture['events']) == 0, deployment_events_fixture",
		"assert deployment_events_fixture.get('managedDriftSummary') == {'total': 0, 'high': 0, 'medium': 0, 'low': 0}, deployment_events_fixture",
		"fixture_events_default_scope = deployment_events_fixture.get('resultScope') or {}",
		"assert 'requestedLimit' not in fixture_events_default_scope, deployment_events_fixture",
		"assert fixture_events_default_scope.get('effectiveLimit') == 0, deployment_events_fixture",
		"assert fixture_events_default_scope.get('returnedEventCount') == len(deployment_events_fixture['events']), deployment_events_fixture",
		"assert fixture_events_default_scope.get('returnedCount') == fixture_events_default_scope.get('returnedEventCount'), deployment_events_fixture",
		"assert fixture_events_default_scope.get('totalEventCount') == len(deployment_events_fixture['events']), deployment_events_fixture",
		"assert fixture_events_default_scope.get('truncationStatus') == 'not_truncated', deployment_events_fixture",
		"assert fixture_events_default_scope.get('truncated') is False, deployment_events_fixture",
		"assert resource_logs['deploymentName'] == 'payments-api-staging', resource_logs",
		"assert resource_logs['resourceType'] == 'codeDeployDeploymentGroup', resource_logs",
		"assert resource_logs['resourceName'] == 'payments-api-staging-codedeploy-group', resource_logs",
		"assert isinstance(resource_logs['available'], bool), resource_logs",
		"assert isinstance(resource_logs.get('lookupContext'), dict), resource_logs",
		"assert isinstance(resource_logs['lookupContext'].get('region'), str) and resource_logs['lookupContext']['region'], resource_logs",
		"assert isinstance(resource_logs['lookupContext'].get('controlAccountId'), str) and resource_logs['lookupContext']['controlAccountId'], resource_logs",
		"assert isinstance(resource_logs['lookupContext'].get('targetAccountId'), str) and resource_logs['lookupContext']['targetAccountId'], resource_logs",
		"assert isinstance(resource_logs['lookupContext'].get('crossAccount'), bool), resource_logs",
		"assert isinstance(resource_logs['lookupContext'].get('observerRoleConfigured'), bool), resource_logs",
		"if resource_logs['available']:",
		"assert resource_logs.get('source') == 'codedeploy', resource_logs",
		"assert isinstance(resource_logs.get('entries'), list), resource_logs",
		"assert resource_logs.get('unavailableReason') in {'aws_unavailable', 'resource_not_found'}, resource_logs",
		"assert worker_resource_logs['deploymentName'] == 'worker-prod', worker_resource_logs",
		"assert worker_resource_logs['resourceType'] == 'ecsService', worker_resource_logs",
		"assert worker_resource_logs['resourceName'] == 'worker-prod-service', worker_resource_logs",
		"assert isinstance(worker_resource_logs['available'], bool), worker_resource_logs",
		"assert isinstance(worker_resource_logs.get('lookupContext'), dict), worker_resource_logs",
		"assert isinstance(worker_resource_logs['lookupContext'].get('region'), str) and worker_resource_logs['lookupContext']['region'], worker_resource_logs",
		"assert isinstance(worker_resource_logs['lookupContext'].get('controlAccountId'), str) and worker_resource_logs['lookupContext']['controlAccountId'], worker_resource_logs",
		"assert isinstance(worker_resource_logs['lookupContext'].get('targetAccountId'), str) and worker_resource_logs['lookupContext']['targetAccountId'], worker_resource_logs",
		"assert isinstance(worker_resource_logs['lookupContext'].get('crossAccount'), bool), worker_resource_logs",
		"assert isinstance(worker_resource_logs['lookupContext'].get('observerRoleConfigured'), bool), worker_resource_logs",
		"assert worker_resource_logs.get('entries') is None or isinstance(worker_resource_logs.get('entries'), list), worker_resource_logs",
		"if worker_resource_logs['available']:",
		"assert isinstance(worker_resource_logs.get('source'), str), worker_resource_logs",
		"assert isinstance(worker_resource_logs.get('unavailableReason'), str) and worker_resource_logs['unavailableReason'], worker_resource_logs",
		"assert ecs_task_resource_logs['resourceType'] == 'ecsTask', ecs_task_resource_logs",
		"assert ecs_task_resource_logs['resourceName'] == 'arn:aws:ecs:ap-northeast-2:123456789012:task/worker/abc123', ecs_task_resource_logs",
		"assert ecs_task_scope.get('requestedLimit') == 1, ecs_task_resource_logs",
		"assert ecs_task_scope.get('effectiveLimit') == 1, ecs_task_resource_logs",
		"assert ecs_task_resource_inspection['resourceType'] == 'ecsTask', ecs_task_resource_inspection",
		"assert ecs_task_resource_inspection['resourceName'] == 'arn:aws:ecs:ap-northeast-2:123456789012:task/worker/abc123', ecs_task_resource_inspection",
		"assert isinstance(ecs_task_resource_inspection.get('lookupContext'), dict), ecs_task_resource_inspection",
		"assert isinstance(worker_resource_inspection['ecsService'].get('serviceName'), str) and worker_resource_inspection['ecsService']['serviceName'], worker_resource_inspection",
		"minimum_healthy_percent = worker_resource_inspection['ecsService'].get('minimumHealthyPercent')",
		"maximum_percent = worker_resource_inspection['ecsService'].get('maximumPercent')",
		"health_check_grace_period_seconds = worker_resource_inspection['ecsService'].get('healthCheckGracePeriodSeconds')",
		"assert isinstance(worker_resource_inspection['ecsService'].get('deployments', []), list), worker_resource_inspection",
		"assert isinstance(worker_resource_inspection['ecsService'].get('events', []), list), worker_resource_inspection",
		"assert_ecs_service_event_ids('worker_resource_inspection', worker_resource_inspection)",
		"assert_ecs_service_event_ids('selected_worker_resource_inspection', selected_worker_resource_inspection)",
		"assert isinstance(asg.get('autoScalingGroupName'), str) and asg['autoScalingGroupName'], payload",
		"assert isinstance(asg.get('instances', []), list), payload",
		"assert \"Controller-derived runtime events from ${deploymentEventsReadRoute('{deploymentName}', limit, source)}. Tenet shows the selected bounded window, newest first; use Event window to change how much controller history this panel reads without leaving the application drill-down. This is not audit/auth history.\" in ui, ui",
		"assert 'Event source' in ui, ui",
		"assert 'Refresh runtime events' in ui, ui",
		"assert 'Copy runtime events route' in ui, ui",
		"assert \"deploymentEventsReadRoute(deploymentName, currentPageState.eventLimit, currentPageState.eventSource)\" in ui, ui",
		"assert 'Loading controller runtime events…' in ui, ui",
		"assert 'No controller runtime events are retained for this application yet.' in ui, ui",
		"assert \"const createDeploymentEventsFreshnessMeta = (result) => {\" in ui, ui",
		"assert \"deployment-events-freshness\" in ui, ui",
		"assert 'Last refreshed · ${fetchedAt}' in ui, ui",
		"assert 'Controller runtime events are currently unavailable.' in ui, ui",
		"assert \"const runtimeEventAnchorBase = (event) => {\" in ui, ui",
		"assert \"const backendID = auditAnchorSlug(event?.eventId);\" in ui, ui",
		"assert \"if (backendID && backendID !== 'unknown') return `runtime-event-${backendID}`;\" in ui, ui",
		"assert \"const uniqueRuntimeEventAnchorID = (event, seenAnchors) => {\" in ui, ui",
		"assert \"createNavLink(runtimeEventAnchorURL(deploymentName, eventID, selectedLimit, selectedSource), 'Share event', '_self')\" in ui, ui",
		"assert \"const deploymentEventsReadRoute = (deploymentName, limit = currentPageState.eventLimit, source = currentPageState.eventSource) => `GET ${buildDeploymentEventsReadRoute(deploymentName, { limit, source })}`;\" in ui, ui",
		"assert \"createDeploymentEventsActions(section, deploymentName),\" in ui, ui",
		"assert \"createDeploymentEventsActions(container, deploymentName),\" in ui, ui",
		"assert \"deploymentEventsReadRoute(deploymentName, limit, source),\" in ui, ui",
		"assert 'Loading live inspection…' in ui, ui",
		"assert 'Live inspection is unavailable for this resource.' in ui, ui",
		"assert 'No live inspection details are available for this resource yet.' in ui, ui",
		"assert 'Service deployments' in ui, ui",
		"assert 'Auto Scaling Group name' in ui, ui",
		"assert 'Open EC2 console logs' in ui, ui",
		"assert \"const autoScalingGroupRecentLogsHandoff = (deploymentName, resource) => {\" in ui, ui",
		"assert \"const autoScalingGroupSupportsInstanceHandoff = (resource) => ['candidateAutoScalingGroup', 'serviceAutoScalingGroup'].includes(String(resource?.type || '').trim());\" in ui, ui",
		"assert 'Inspect this instance' in ui, ui",
		"assert 'Open instance logs' in ui, ui",
		"assert \"buildPageSectionURL('resource', 'resource-recent-logs'\" in ui, ui",
		"assert 'Launch template' in ui, ui",
		"assert 'Target group ARNs' in ui, ui",
		"assert 'Mixed instances overrides' in ui, ui",
		"audit_ui=$(curl -fsS \"http://$HOST:$PORT/?page=audit&deployment=payments-api-prod&action=apply.requested&limit=1\")",
		"summary_path=$(write_payload summary.json \"$summary\")",
		"AUDIT_UI_PATH=\"$audit_ui_path\"",
		"audit_ui = read_text('AUDIT_UI_PATH')",
		"def read_json(path_env):",
		"def read_text(path_env):",
		"assert 'Open audit log' in ui, ui",
		"assert 'Deployment audit timeline' in audit_ui, audit_ui",
		"assert 'Active deployment context' in audit_ui, audit_ui",
		"assert 'Quick navigation' in audit_ui, audit_ui",
		"assert 'Deployment workspaces' in audit_ui, audit_ui",
		"assert 'Current section' in audit_ui, audit_ui",
		"assert 'No section is currently targeted.' in audit_ui, audit_ui",
		"assert 'On this page' in audit_ui, audit_ui",
		"assert \"const createAuditQuickNav = (sectionLinks = []) => createDeploymentQuickNav(\" in audit_ui, audit_ui",
		"assert \"const deploymentContextSectionID = 'audit-deployment-context';\" in audit_ui, audit_ui",
		"assert \"deploymentCallout.setAttribute('id', deploymentContextSectionID);\" in audit_ui, audit_ui",
		"assert \"const timelineSectionID = 'audit-timeline';\" in audit_ui, audit_ui",
		"assert \"timelineSection.setAttribute('id', timelineSectionID);\" in audit_ui, audit_ui",
		"assert \"buildPageSectionURL('audit', sectionID, {\" in audit_ui, audit_ui",
		"assert \"createElement('div', 'quick-nav-current', `${deploymentName || 'Deployment'} · ${currentWorkspace}`)\" in audit_ui, audit_ui",
		"assert 'Immutable plan preview and apply-request activity for deployment ${deploymentName} rendered from GET /v1/audit-logs.' in audit_ui, audit_ui",
		"assert \"const auditEntryAnchorID = (entry) => {\" in audit_ui, audit_ui",
		"assert \"const uniqueAuditEntryAnchorID = (entry, seenAnchors) => {\" in audit_ui, audit_ui",
		"assert \"const auditEntryQuickNavLabel = (entry) => {\" in audit_ui, audit_ui",
		"assert \"const auditEntryAnchors = entries.map((entry) => ({\" in audit_ui, audit_ui",
		"assert \"pushAuditSectionLink(anchor.anchorID, anchor.label);\" in audit_ui, audit_ui",
		"assert \"row.setAttribute('id', rowAnchorID);\" in audit_ui, audit_ui",
		"assert 'Audit timeline navigation' in audit_ui, audit_ui",
		"assert 'Newest visible entry' in audit_ui, audit_ui",
		"assert 'Oldest visible entry' in audit_ui, audit_ui",
		"assert \"createNavLink(auditEntryAnchorURL(newest.entry, newest.anchorID), 'Jump to newest entry', '_self')\" in audit_ui, audit_ui",
		"assert \"createNavLink(auditEntryAnchorURL(oldest.entry, oldest.anchorID), 'Jump to oldest visible entry', '_self')\" in audit_ui, audit_ui",
		"assert \"createNavLink(auditEntryAnchorURL(entry, rowAnchorID), 'Share audit entry', '_self')\" in audit_ui, audit_ui",
		"assert 'Share or continue in app' in audit_ui, audit_ui",
		"assert 'No audit activity has been persisted yet.' in audit_ui, audit_ui",
		"assert 'No audit activity has been persisted yet for deployment ${currentPageState.deployment}. Create a plan preview or apply request from the plan/apply workspace to add immutable audit rows for this deployment.' in audit_ui, audit_ui",
		"assert \"const auditFilterLabels = () => {\" in audit_ui, audit_ui",
		"assert \"const auditEmptyStateText = () => {\" in audit_ui, audit_ui",
		"assert \"No audit entries match the current filters (${filters.join(' · ')}${hasLimit ? `; ${auditLimitLabel()}` : ''}). ${currentPageState.deployment && (currentPageState.auditAction || hasLimit) ? 'Clear action/limit to stay scoped to this deployment, or clear audit controls to remove the deployment scope.' : 'Clear audit controls to return to the full audit timeline.'}\" in audit_ui, audit_ui",
		"assert \"'Clear action/limit'\" in audit_ui, audit_ui",
		"assert \"'Clear audit controls'\" in audit_ui, audit_ui",
		"assert 'Loading audit log…' in audit_ui, audit_ui",
		"assert 'Audit log is currently unavailable.' in audit_ui, audit_ui",
		"assert 'Refresh audit log' in audit_ui, audit_ui",
		"assert 'Copy audit read route' in audit_ui, audit_ui",
		"assert \"const auditLogsRoute = buildAuditLogsRoute({\" in audit_ui, audit_ui",
		"assert \"createCopyReadRouteButton(`GET ${auditLogsRoute}`, 'Copy audit read route')\" in audit_ui, audit_ui",
		"assert \"createNavLink(auditLogsRoute, 'Audit JSON')\" in audit_ui, audit_ui",
		"assert 'Refreshing audit log from GET /v1/audit-logs…' in audit_ui, audit_ui",
		"assert 'Failed to refresh audit log:' in audit_ui, audit_ui",
		"assert 'audit-freshness' in audit_ui, audit_ui",
		"assert 'const auditLogsFetchedAtText = (result) => {' in audit_ui, audit_ui",
		"assert 'const withAuditLogsFetchedAt = (result) => {' in audit_ui, audit_ui",
		"assert 'Last refreshed · ${fetchedAt}' in audit_ui, audit_ui",
		"assert \"const createAuditLogsActions = (isLoading = false) => {\" in audit_ui, audit_ui",
		"assert \"const auditLogsRefreshStateText = (auditState) => {\" in audit_ui, audit_ui",
		"assert \"const fetchAuditLogs = ({ deploymentName = '', action = '', limit = 0 } = {}) => {\" in audit_ui, audit_ui",
		"assert \"if (action) params.set('action', action);\" in audit_ui, audit_ui",
		"assert \"const createAuditQuickNav = (sectionLinks = []) => createDeploymentQuickNav(\" in audit_ui, audit_ui",
		"assert \"createSelect('Action', currentPageState.auditAction, auditActionOptions, (event) => {\" in audit_ui, audit_ui",
		"assert \"const normalizeAuditLimit = (value) => {\" in audit_ui, audit_ui",
		"assert \"const auditLimitSelectOptions = (limit) => {\" in audit_ui, audit_ui",
		"assert \"const syncAuditQueryParams = (params, pageState) => {\" in audit_ui, audit_ui",
		"assert \"const syncAuditPageState = ({ clearAuditHash = false } = {}) => {\" in audit_ui, audit_ui",
		"assert \"const hash = clearAuditHash && currentHash.startsWith('#audit-') ? '' : currentHash;\" in audit_ui, audit_ui",
		"assert \"syncAuditPageState({ clearAuditHash: true });\" in audit_ui, audit_ui",
		"assert \"createSelect('Limit', String(currentPageState.auditLimit || ''), auditLimitSelectOptions(currentPageState.auditLimit), (event) => {\" in audit_ui, audit_ui",
		"assert 'Open drift view' in audit_ui, audit_ui",
		"assert \"const defaultDeploymentEventsLimit = 10;\" in ui, ui",
		"assert \"const buildDeploymentEventsReadRoute = (deploymentName, { limit = defaultDeploymentEventsLimit, source = currentPageState.eventSource } = {}) => {\" in ui, ui",
		"assert \"if (normalizedSource) params.set('source', normalizedSource);\" in ui, ui",
		"assert \"const fetchDeploymentEvents = (deploymentName, limit = currentPageState.eventLimit, source = currentPageState.eventSource) => fetchEmbeddedReadRouteJSON(buildDeploymentEventsReadRoute(deploymentName, { limit, source }), {\" in ui, ui",
		"audit_logs_action_filtered=$(curl -fsS \"http://$HOST:$PORT/v1/audit-logs?action=apply.requested\")",
		"audit_logs_filtered=$(curl -fsS \"http://$HOST:$PORT/v1/audit-logs?deployment=payments-api-prod&limit=1\")",
		"audit_logs_empty=$(curl -fsS \"http://$HOST:$PORT/v1/audit-logs?deployment=worker-prod\")",
		"actor_plan_preview=$(curl -fsS -X POST \\",
		"X-Tenet-Actor-Id: explicit-456",
		"X-Tenet-Actor-Name: Alex Explicit",
		"X-User-ID: operator-123",
		"actor_audit_logs=$(curl -fsS \"http://$HOST:$PORT/v1/audit-logs?deployment=payments-api-staging&action=apply.requested&limit=1\")",
		"limited_worker_resource_logs=$(curl -fsS \"http://$HOST:$PORT/v1/overview/deployments/worker-prod/resource-logs?resourceType=ecsService&resourceName=worker-prod-service&limit=1\")",
		"ecs_task_resource_logs=$(curl -fsS \"http://$HOST:$PORT/v1/overview/deployments/worker-prod/resource-logs?resourceType=ecsTask&resourceName=$ecs_task_resource_name&containerName=worker&clusterArn=arn%3Aaws%3Aecs%3Aap-northeast-2%3A123456789012%3Acluster%2Fworker-prod&limit=1\")",
		"ecs_task_resource_inspection=$(curl -fsS \"http://$HOST:$PORT/v1/overview/deployments/worker-prod/resource-inspection?resourceType=ecsTask&resourceName=$ecs_task_resource_name&clusterArn=arn%3Aaws%3Aecs%3Aap-northeast-2%3A123456789012%3Acluster%2Fworker-prod\")",
		"invalid_resource_log_limit_status=$(curl -sS -o \"$INVALID_RESOURCE_LOG_LIMIT_BODY\" -w \"%{http_code}\" \"http://$HOST:$PORT/v1/overview/deployments/worker-prod/resource-logs?resourceType=ecsService&resourceName=worker-prod-service&limit=0\")",
		"invalid_resource_inspection_limit_status=$(curl -sS -o \"$INVALID_RESOURCE_INSPECTION_LIMIT_BODY\" -w \"%{http_code}\" \"http://$HOST:$PORT/v1/overview/deployments/payments-api-prod/resource-inspection?resourceType=candidateAutoScalingGroup&resourceName=payments-api-prod-candidate&limit=0\")",
		"ECS_TASK_RESOURCE_LOGS_PATH=\"$ecs_task_resource_logs_path\"",
		"ECS_TASK_RESOURCE_INSPECTION_PATH=\"$ecs_task_resource_inspection_path\"",
		"LIMITED_WORKER_RESOURCE_LOGS_PATH=\"$limited_worker_resource_logs_path\"",
		"INVALID_RESOURCE_LOG_LIMIT_STATUS=\"$invalid_resource_log_limit_status\"",
		"limited_worker_resource_logs = read_json('LIMITED_WORKER_RESOURCE_LOGS_PATH')",
		"ecs_task_resource_logs = read_json('ECS_TASK_RESOURCE_LOGS_PATH')",
		"ecs_task_resource_inspection = read_json('ECS_TASK_RESOURCE_INSPECTION_PATH')",
		"assert len(limited_worker_resource_logs.get('entries') or []) <= 1, limited_worker_resource_logs",
		"assert invalid_resource_log_limit.strip() == 'invalid limit query', invalid_resource_log_limit",
		"operator_context_stale_scopes=$(curl -fsS \\",
		"X-Tenet-Authorized-Projects: payments-api, archived-payments",
		"X-Tenet-Authorized-Environments: staging-apne2, stale-env",
		"operator_context_stale_scopes_path=$(write_payload operator-context-stale-scopes.json \"$operator_context_stale_scopes\")",
		"OPERATOR_CONTEXT_STALE_SCOPES_PATH=\"$operator_context_stale_scopes_path\"",
		"operator_context_stale_scopes = read_json('OPERATOR_CONTEXT_STALE_SCOPES_PATH')",
		"assert stale_scopes_summary['forwardedProjectCount'] == 2, operator_context_stale_scopes",
		"assert stale_scopes_summary['forwardedEnvironmentCount'] == 2, operator_context_stale_scopes",
		"assert stale_scopes_summary['unmatchedProjects'] == ['archived-payments'], operator_context_stale_scopes",
		"assert stale_scopes_summary['unmatchedEnvironments'] == ['stale-env'], operator_context_stale_scopes",
		"invalid_audit_action_status=$(curl -sS -o \"$INVALID_AUDIT_ACTION_BODY\" -w \"%{http_code}\" \"http://$HOST:$PORT/v1/audit-logs?action=invalid.action\")",
		"AUDIT_LOGS_ACTION_FILTERED_PATH=\"$audit_logs_action_filtered_path\"",
		"AUDIT_LOGS_FILTERED_PATH=\"$audit_logs_filtered_path\"",
		"AUDIT_LOGS_EMPTY_PATH=\"$audit_logs_empty_path\"",
		"ACTOR_PLAN_PREVIEW_PATH=\"$actor_plan_preview_path\"",
		"ACTOR_PLAN_PREVIEW_GET_PATH=\"$actor_plan_preview_get_path\"",
		"ACTOR_APPLY_RECORD_PATH=\"$actor_apply_record_path\"",
		"ACTOR_APPLY_RECORD_GET_PATH=\"$actor_apply_record_get_path\"",
		"INVALID_APPLY_TRANSITION_STATUS=\"$invalid_apply_transition_status\"",
		"UNKNOWN_APPLY_TRANSITION_STATUS=\"$unknown_apply_transition_status\"",
		"TERMINAL_APPLY_TRANSITION_STATUS=\"$terminal_apply_transition_status\"",
		"ACTOR_AUDIT_LOGS_PATH=\"$actor_audit_logs_path\"",
		"audit_logs_action_filtered = read_json('AUDIT_LOGS_ACTION_FILTERED_PATH')",
		"audit_logs_filtered = read_json('AUDIT_LOGS_FILTERED_PATH')",
		"audit_logs_empty = read_json('AUDIT_LOGS_EMPTY_PATH')",
		"actor_plan_preview = read_json('ACTOR_PLAN_PREVIEW_PATH')",
		"actor_plan_preview_get = read_json('ACTOR_PLAN_PREVIEW_GET_PATH')",
		"actor_apply_record = read_json('ACTOR_APPLY_RECORD_PATH')",
		"actor_apply_record_get = read_json('ACTOR_APPLY_RECORD_GET_PATH')",
		"invalid_apply_transition = read_text('INVALID_APPLY_TRANSITION_PATH')",
		"unknown_apply_transition = read_text('UNKNOWN_APPLY_TRANSITION_PATH')",
		"terminal_apply_transition = read_text('TERMINAL_APPLY_TRANSITION_PATH')",
		"actor_audit_logs = read_json('ACTOR_AUDIT_LOGS_PATH')",
		"INVALID_AUDIT_ACTION_BODY_PATH=\"$INVALID_AUDIT_ACTION_BODY\"",
		"\"http://$HOST:$PORT/v1/applies/$apply_id/transitions\"",
		"\"http://$HOST:$PORT/v1/applies/apply_smoke_missing/transitions\"",
		"assert invalid_apply_transition.strip() == 'invalid apply transition request body', invalid_apply_transition",
		"assert 'apply request not found' in unknown_apply_transition, unknown_apply_transition",
		"assert f\"{apply_record['id']} is already terminal with status failed\" in terminal_apply_transition, terminal_apply_transition",
		"assert len(audit_logs_action_filtered['entries']) >= 2, audit_logs_action_filtered",
		"assert actor_plan_preview['actor']['id'] == 'explicit-456', actor_plan_preview",
		"assert actor_plan_preview['actor']['name'] == 'Alex Explicit', actor_plan_preview",
		"assert actor_apply_record['actor']['id'] == 'explicit-456', actor_apply_record",
		"assert actor_apply_record['actor']['name'] == 'Alex Explicit', actor_apply_record",
		"assert actor_audit_logs['entries'][0]['actor']['available'] is True, actor_audit_logs",
		"assert actor_audit_logs['entries'][0]['actor']['id'] == 'explicit-456', actor_audit_logs",
		"assert actor_audit_logs['entries'][0]['actor']['name'] == 'Alex Explicit', actor_audit_logs",
		"assert 'no authenticated request context' in entry['actor']['reason'], entry",
		"assert len(audit_logs_filtered['entries']) == 1, audit_logs_filtered",
		"assert audit_logs_empty['entries'] == [], audit_logs_empty",
		"assert os.environ['INVALID_AUDIT_ACTION_STATUS'] == '400', os.environ['INVALID_AUDIT_ACTION_STATUS']",
	} {
		if !strings.Contains(text, fragment) {
			t.Fatalf("expected smoke-local.sh to include %q", fragment)
		}
	}
	if !strings.Contains(text, "assert isinstance(runtime_summary.get('changeDetails', []), list), overview") {
		t.Fatalf("expected smoke-local.sh to verify overview runtimeSummary.changeDetails shape")
	}
	if !strings.Contains(text, "assert webhook_push['status'] == 'accepted', webhook_push") || !strings.Contains(text, "assert webhook_deleted['status'] == 'ignored', webhook_deleted") || !strings.Contains(text, "assert webhook_deleted['deleted'] is True, webhook_deleted") || !strings.Contains(text, "assert webhook_ping['status'] == 'ignored', webhook_ping") || !strings.Contains(text, "assert 'github webhook signature mismatch' in invalid_webhook, invalid_webhook") {
		t.Fatalf("expected smoke-local.sh to verify github webhook accepted/ignored/error contracts")
	}
	if !strings.Contains(text, "assert isinstance(deployment_runtime_summary.get('changeDetails', []), list), deployment_detail") {
		t.Fatalf("expected smoke-local.sh to verify deployment detail runtimeSummary.changeDetails shape")
	}
	if !strings.Contains(text, "managed_drift = deployment_detail.get('managedDrift')") || !strings.Contains(text, "if managed_drift is not None:") || !strings.Contains(text, "assert isinstance(managed_drift, list), deployment_detail") {
		t.Fatalf("expected smoke-local.sh to verify deployment detail managedDrift shape when present")
	}
	if !strings.Contains(text, "controller_events=$(curl -fsS \"http://$HOST:$PORT/v1/overview/deployments/payments-api-prod/events\"") {
		t.Fatalf("expected smoke-local.sh to hit deployment events after controller starts")
	}
	if !strings.Contains(text, "if deployment_history:") || !strings.Contains(text, "assert isinstance(deployment_history[0].get('changeDetails', []), list), deployment_detail") {
		t.Fatalf("expected smoke-local.sh to verify deployment detail history changeDetails shape")
	}
	if !strings.Contains(text, "assert events_payload['deploymentName'] == 'payments-api-prod', events_payload") || !strings.Contains(text, "summary.get('total') == len(event_managed_drift)") || !strings.Contains(text, "assert event.get('source') == 'controller', events_payload") {
		t.Fatalf("expected smoke-local.sh to verify controller-populated deployment events payload")
	}
	if !strings.Contains(text, "assert 'Runtime change details' in ui, ui") {
		t.Fatalf("expected smoke-local.sh to assert embedded html contains Runtime change details")
	}
	if !strings.Contains(text, "assert 'Current source: controller-authored managedDrift rows.' in ui, ui") || !strings.Contains(text, "assert 'Current source: derived fallback read-model heuristics.' in ui, ui") {
		t.Fatalf("expected smoke-local.sh to assert embedded html contains drift-source callout copy")
	}
	if !strings.Contains(text, "assert \"const linkedResources = (mode === 'drift' || mode === 'plan') ? groupResourceSections(detail).flatMap((group) => group.resources || []) : null;\" in ui, ui") {
		t.Fatalf("expected smoke-local.sh to assert drift workspace resource-link contract")
	}
	for _, fragment := range []string{
		"assert 'On this page' in ui, ui",
		"assert 'Evidence' in ui, ui",
		"assert 'Open resource evidence' in ui, ui",
		"assert \"resolveManagedDriftResourceRef(detail, row.resourceRef)\" in ui, ui",
		"assert 'Drift summary' in ui, ui",
		"assert 'Total rows' in ui, ui",
		"assert 'Visible rows' in ui, ui",
		"assert 'Visible high' in ui, ui",
		"assert 'No field-level drift rows match the current filters. Adjust the query or severity filters to inspect reported drift.' in ui, ui",
		"assert \"const buildPageSectionURL = (page, sectionID, extras = {}) => {\" in ui, ui",
		"assert \"const scrollToCurrentHashTarget = () => {\" in ui, ui",
		"assert \"target.scrollIntoView({ block: 'start', behavior: 'auto' });\" in ui, ui",
		"assert \"buildPageSectionURL('drift', 'drift-runtime-signals', { deployment: deploymentName })\" in ui, ui",
		"assert \"buildPageSectionURL('drift', 'drift-rollout-execution', { deployment: deploymentName })\" in ui, ui",
		"assert \"buildPageSectionURL('drift', 'drift-field-level', { deployment: deploymentName })\" in ui, ui",
		"assert \"buildPageSectionURL('drift', 'drift-status-conditions', { deployment: deploymentName })\" in ui, ui",
		"assert \"runtimeSignalsSection.id = 'drift-runtime-signals';\" in ui, ui",
		"assert \"executionSection.id = 'drift-rollout-execution';\" in ui, ui",
		"assert \"driftSection.id = 'drift-field-level';\" in ui, ui",
		"assert \"conditionsSection.id = 'drift-status-conditions';\" in ui, ui",
	} {
		if !strings.Contains(text, fragment) {
			t.Fatalf("expected smoke-local.sh to include %q", fragment)
		}
	}
	for _, fragment := range []string{
		"assert 'Plan preview actions' in ui, ui",
		"assert 'Apply request actions' in ui, ui",
		"assert 'Plan preview approval guidance' in ui, ui",
		"assert \"const pushPlanSection = (section, sectionID, label) => {\" in ui, ui",
		"assert \"pushPlanSection(runtimeSignalsSection, 'plan-runtime-signals', 'Runtime signals');\" in ui, ui",
		"assert \"pushPlanSection(executionSection, 'plan-rollout-execution', 'Rollout execution');\" in ui, ui",
		"assert \"pushPlanSection(actionsSection, 'plan-preview-actions', 'Plan preview actions');\" in ui, ui",
		"assert \"pushPlanSection(applySection, 'plan-apply-actions', 'Apply request actions');\" in ui, ui",
		"assert \"pushPlanSection(planOperationsSection, 'plan-operations', 'Plan operations');\" in ui, ui",
		"assert \"buildPageSectionURL('plan', sectionID, {\" in ui, ui",
		"assert \"planId: currentPageState.planId || planState?.planId || planState?.record?.id || '',\" in ui, ui",
		"assert \"applyId: currentPageState.applyId || applyState?.applyId || applyState?.record?.id || '',\" in ui, ui",
		"assert 'Create Plan will POST /v1/plans/deployments/${deploymentName}, store an immutable server-authored preview, and update this workspace URL with a shareable planId.' in ui, ui",
		"assert 'Authenticated operator required' in ui, ui",
		"assert 'const operatorWriteAuthRequiredMessage = (route, action, context = operatorContextState.context) => {' in ui, ui",
		"assert '${signInRuntime.message}; sign in through ${signInRuntime.loginRoute}, then return to this same bookmarkable workspace and retry ${action}.' in ui, ui",
		"assert 'Trusted front-door X-Tenet-Authenticated-* identity headers still take precedence when present, and Tenet will not send the write until authenticated identity is available.' in ui, ui",
		"assert 'Tenet does not start a built-in GitHub OAuth login, enterprise SSO login, or Tenet-managed browser session from this page; retry ${action} for this same bookmarkable workspace through the trusted front door' in ui, ui",
		"assert 'const operatorWriteForbiddenMessage = (route, action) => `Tenet rejected ${route} after recognizing the caller\\x27s identity, but this operator is not authorized to ${action.toLowerCase()} for that deployment through this route. Ask a front-door/RBAC admin to authorize this deployment through either X-Tenet-Authorized-Deployments or matching X-Tenet-Authorized-Projects plus X-Tenet-Authorized-Environments catalog placement, then retry ${action}. Partial, stale, or unavailable project/environment placement still fails closed, so use the Operator context banner to confirm the forwarded scopes match this deployment.`;' in ui, ui",
		"assert 'Operator access denied' in ui, ui",
		"assert 'reviewed plan is stale (${response.status})' in ui, ui",
		"assert \"throw requestError(response.status === 409 ? 'stale-plan' : response.status === 503 ? 'unavailable' : 'fetch-error', message, response.status);\" in ui, ui",
		"assert 'Reviewed plan is stale' in ui, ui",
		"assert 'The reviewed plan preview ${activePlanID || \\'planId\\'} is stale. Create a fresh plan preview before retrying Apply Plan. Deployment and plan-derived context are still shown below.' in ui, ui",
		"assert \"applyState?.status === 'stale-plan' ? 'Stale plan'\" in ui, ui",
		"assert 'const buildPlanPreviewRoute = (planId) => `/v1/plans/${encodeURIComponent(planId)}`;' in ui, ui",
		"assert 'const buildApplyRequestRoute = (applyId) => `/v1/applies/${encodeURIComponent(applyId)}`;' in ui, ui",
		"assert 'Refresh plan preview' in ui, ui",
		"assert 'Refresh apply request' in ui, ui",
		"assert 'const loadPlanPreviewRecord = (planId) => {' in ui, ui",
		"assert 'const loadApplyRequestRecord = (applyId) => {' in ui, ui",
		"assert \"createNavLink(buildPageURL('plan', { deployment: detail.name || currentPageState.deployment, planId: activePlanRecord.id, applyId: activeApplyRecord?.id || applyState?.applyId || '' }), 'Open bookmarked workspace', '_self')\" in ui, ui",
		"assert \"createNavLink(buildPlanPreviewRoute(activePlanRecord.id), 'Plan JSON')\" in ui, ui",
		"assert \"createNavLink(buildPageURL('plan', { deployment: detail.name || currentPageState.deployment, planId: activeApplyRecord.planId || activePlanRecord?.id || currentPageState.planId, applyId: activeApplyRecord.id }), 'Open bookmarked apply', '_self')\" in ui, ui",
		"assert \"createNavLink(buildApplyRequestRoute(activeApplyRecord.id), 'Apply JSON')\" in ui, ui",
		"assert 'Server-authored plan preview loaded from GET /v1/plans/${activePlanRecord.id}.' in ui, ui",
		"assert 'Future immutable lifecycle states render here as queued, running, succeeded, cancelled, or failed.' in ui, ui",
		"assert 'Apply Plan activates only when a loaded persisted ready plan preview with a valid planId is present.' in ui, ui",
		"assert 'Live execution is intentionally disabled by default, so failed immutable apply records are expected and useful for operator review.' in ui, ui",
		"assert 'Apply lifecycle summary' in apply_ui, apply_ui",
		"assert 'Apply lifecycle metadata' in apply_ui, apply_ui",
		"assert 'Executor claim' in ui, ui",
		"assert 'Executor worker' in ui, ui",
		"assert 'Claimed at' in ui, ui",
		"assert 'Claim freshness' in ui, ui",
		"assert 'Freshness reason' in ui, ui",
		"assert 'Last heartbeat' in ui, ui",
		"assert 'Claim expires' in ui, ui",
		"assert \"const applyClaimFreshnessValue = (record, field) => {\" in ui, ui",
		"assert \"const activeApplyClaimStatuses = ['pending', 'queued', 'running', 'in_progress', 'in-progress'];\" in ui, ui",
		"assert \"const hasApplyClaimCoreMetadata = (record) => applyClaimWorkerValue(record) !== '—' || applyClaimTimestampValue(record) !== '—';\" in ui, ui",
		"assert \"const applyClaimFreshnessRows = (record) => {\" in ui, ui",
		"assert 'Freshness metadata unavailable for this active apply claim.' in ui, ui",
		"assert \"const hasApplyClaimMetadata = (record) => hasApplyClaimCoreMetadata(record) || hasApplyClaimFreshnessMetadata(record);\" in ui, ui",
		"assert 'Pending local-safe executor claim; no worker has claimed this active apply request yet.' in ui, ui",
		"assert \"const hasPendingApplyClaim = (record) => {\" in ui, ui",
		"assert \"return isActiveApplyRecord(record);\" in ui, ui",
		"assert 'Lifecycle updated' in apply_ui, apply_ui",
		"assert 'Completed / total steps' in apply_ui, apply_ui",
		"assert 'Queued apply operation' in apply_ui, apply_ui",
		"assert \"params.get('planId') || ''\" in ui, ui",
		"assert \"params.get('applyId') || ''\" in ui, ui",
		"plan_ui=$(curl -fsS \"http://$HOST:$PORT/?page=plan&deployment=payments-api-prod&planId=$plan_id\")",
		"apply_ui=$(curl -fsS \"http://$HOST:$PORT/?page=plan&deployment=payments-api-prod&planId=$plan_id&applyId=$apply_id\")",
		"assert 'planId' in plan_ui, plan_ui",
		"assert 'applyId' in apply_ui, apply_ui",
		"assert 'Apply operations' in apply_ui, apply_ui",
		"assert '/v1/applies/' in apply_ui, apply_ui",
		"assert 'The ordered execution sequence below comes from the persisted apply record' in apply_ui, apply_ui",
	} {
		if !strings.Contains(text, fragment) {
			t.Fatalf("expected smoke-local.sh to include %q", fragment)
		}
	}
	for _, fragment := range []string{
		"DEPLOYMENT_DETAIL_JSON=\"$deployment_detail\"",
		"PLAN_PREVIEW_JSON=\"$plan_preview\"",
		"APPLY_RECORD_JSON=\"$apply_record\"",
		"FRONTDOOR_PLAN_PREVIEW_JSON=\"$frontdoor_plan_preview\"",
		"CONTROLLER_DETAIL_JSON=\"$controller_detail\"",
	} {
		if strings.Contains(text, fragment) {
			t.Fatalf("expected smoke-local.sh to avoid env-blob JSON parsing fragment %q", fragment)
		}
	}
}

func TestResourceLogsResultScopeReleaseGuardContracts(t *testing.T) {
	statusVocabulary := []string{"truncated", "not_truncated", "unknown", "not_applicable"}
	fieldFragments := []string{"requestedLimit", "effectiveLimit", "returnedEntryCount", "returnedCount", "newestEntryTimestamp", "oldestEntryTimestamp", "truncationStatus", "truncated"}

	docs := map[string][]string{
		"../../docs/architecture/tenet-v1-operator-contracts.md": {
			"bounded `resource-logs?limit=1` provider cap plus router fallback truncation/resultScope metadata",
			"Operators can add an optional positive `limit` query parameter",
			"Successful responses include additive `resultScope` metadata",
			"Tenet does not issue extra provider calls solely to prove exact totals or provider-wide time ranges beyond the bounded read",
			"Tenet threads that cap into providers that can limit natively",
			"router-level truncation to `entries[]` as a safety fallback",
			"Local and compose smoke prove the provider-cap/router-fallback contract shape without requiring live CloudWatch rows",
			"operator troubleshooting examples that distinguish bounded successful reads, AWS-unavailable supported-resource lookups, and route-stable no-log-source resources",
			"`ecsTask` for an exact task ARN, with optional `containerName={container}` to narrow the bounded CloudWatch Logs read to one awslogs-backed container; responses echo that selector as response-level `containerName` when supplied",
			"successful ECS task `entries[]` rows may carry additive `containerName` metadata for the resolved per-row log source",
			"optional per-row CloudWatch `logGroup` source identity when provided",
			"with optional `instanceId={i-...}` to narrow the read to a specific current ASG member while preserving the default newest-instance behavior when omitted",
			"Recent-log payloads also echo the selected rollout execution description as additive `resourceDescription`",
			"Local and compose smoke lock this alongside the ECS service and ECS task selected-cluster `clusterArn` echo, ECS task `containerName` selector, ASG console `entries[].instanceId` metadata, and selected ASG live-inspection `instanceId` echo",
			"Local and compose smoke now also release-guard successful empty recent-log reads by requiring `emptyReason` (`no_recent_events` or `no_formatted_entries`) only when `available=true` and `entries=[]`",
			"Known `unavailableReason` values render friendly operator next-step copy for missing cross-account observer-role wiring, missing ECS tasks, missing ASG instances, stale resource selections, and AWS/provider access failures",
			"capture the HTTP `Request-Id` response header next to the `lookupContext` object before escalating",
			"the request ID anchors Tenet's structured API logs while the lookup context explains which region, control account, target account, and observer-role path the provider attempted",
		},
		"../../docs/features/resource-recent-logs.md": {
			"Both local and compose smoke coverage lock a representative `ecsService` bounded read (`limit=1`), an ECS task/container/cluster read (`resourceType=ecsTask&containerName=worker&clusterArn={ecsClusterArn}&limit=1`), plus the invalid `limit=0` rejection",
			"`containerName` is optional and only meaningful for `resourceType=ecsTask`",
			"echoes the trimmed selector as response-level `containerName`, including explicit unavailable payloads",
			"`instanceId` is optional and only meaningful for Auto Scaling Group-backed EC2 console-log reads",
			"echoes the trimmed selector as response-level `instanceId`, including explicit unavailable payloads",
			"Successful EC2 console rows also carry additive `entries[].instanceId` metadata for the source ASG member",
			"Successful rows include `entries[].instanceId` so default ASG reads and selected-instance reads both identify which EC2 member produced each console line",
			"`available=false` / `unavailableReason=resource_not_found` instead of reading arbitrary instance console output",
			"Successful ECS task log rows may also carry per-entry `entries[].containerName`",
			"optional `logGroup` for CloudWatch-backed ECS rows when Tenet knows the resolved log group",
			"release guards intentionally lock the response-level selector separately from per-entry/source metadata and separately from rollout `resourceDescription`",
			"optional `resourceDescription` when Tenet resolved a rollout execution description",
			"echoes it as `resourceDescription` on both available and unavailable payloads",
			"`ecsTask` reads recent CloudWatch log events for the exact ECS task ARN or task identifier",
			"Local and smoke environments may still return explicit unavailable states such as `aws_unavailable`, `resource_not_found`, `cross_account_not_configured`, or `no_log_source`",
			"shape-focused: it verifies the `resultScope` fields and conservative status vocabulary without requiring live AWS totals",
			"assert that `available=true` / `entries=[]` payloads carry `emptyReason` (`no_recent_events` or `no_formatted_entries`)",
			"populated successful payloads omit `emptyReason`, and unavailable payloads keep using explicit `unavailableReason`",
			"not a pagination cursor or full log-total guarantee",
			"Tenet does not issue extra AWS calls just to prove total rows or provider-wide time ranges beyond the bounded read",
			"Operator troubleshooting examples",
			"Bounded successful read: `available=true`",
			"Supported resource, AWS lookup unavailable: `available=false`, `unavailableReason=aws_unavailable`",
			"Operators should capture the HTTP `Request-Id` response header together with the `lookupContext` object before escalating",
			"that pair gives reviewers the exact Tenet request log correlation key plus the attempted region/account/observer-role path",
			"maps known reasons to friendly next-step copy for missing cross-account observer-role wiring, missing ECS tasks, missing ASG instances, stale resource selections, and temporary AWS/provider access failures",
			"Route-stable resource with no independent stream: `available=false`, `unavailableReason=no_log_source`",
			"unavailable/no-log-source responses use `returnedCount=0`, compatibility `returnedEntryCount=0`, `truncated=false`, and `truncationStatus=not_applicable`",
			"the UI also recognizes the compatibility aliases `capped` and `at_cap_unknown_total`",
		},
		"../../docs/architecture/tenet-v1-local-runtime.md": {
			"bounded `resource-logs?limit=1` provider cap plus router fallback truncation/resultScope metadata",
			"ASG console `entries[].instanceId` metadata",
			"weighted target-group `no_log_source` recent-log payloads",
			"the `candidateAutoScalingGroup` bounded `resource-inspection?limit=2` sample with `autoScalingGroup.instancesScope` fields when available plus explicit unavailable semantics without AWS access",
		},
	}
	for path, fragments := range docs {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		text := string(body)
		for _, fragment := range fragments {
			if !strings.Contains(text, fragment) {
				t.Fatalf("expected %s to include %q", path, fragment)
			}
		}
		for _, fragment := range fieldFragments {
			if !strings.Contains(text, fragment) {
				t.Fatalf("expected %s to document resultScope field %q", path, fragment)
			}
		}
	}

	overviewDoc, err := os.ReadFile("../../docs/features/overview-dashboard.md")
	if err != nil {
		t.Fatalf("read overview dashboard feature doc: %v", err)
	}
	if !strings.Contains(string(overviewDoc), "operator troubleshooting examples for bounded successful reads, AWS-unavailable supported-resource lookups, and route-stable no-log-source resources") {
		t.Fatalf("expected overview dashboard feature doc to point operators at the recent-log troubleshooting examples")
	}

	featureDoc, err := os.ReadFile("../../docs/features/resource-recent-logs.md")
	if err != nil {
		t.Fatalf("read resource recent-log feature doc: %v", err)
	}
	for _, status := range statusVocabulary {
		if !strings.Contains(string(featureDoc), "`"+status+"`") {
			t.Fatalf("expected resource recent-log feature doc to include truncation status %q", status)
		}
	}

	smokeScripts := map[string][]string{
		"../../scripts/smoke-local.sh": {
			"limited_worker_resource_logs=$(curl -fsS \"http://$HOST:$PORT/v1/overview/deployments/worker-prod/resource-logs?resourceType=ecsService&resourceName=worker-prod-service&limit=1\")",
			"ecs_task_resource_logs=$(curl -fsS \"http://$HOST:$PORT/v1/overview/deployments/worker-prod/resource-logs?resourceType=ecsTask&resourceName=$ecs_task_resource_name&containerName=worker&clusterArn=arn%3Aaws%3Aecs%3Aap-northeast-2%3A123456789012%3Acluster%2Fworker-prod&limit=1\")",
			"ecs_task_resource_inspection=$(curl -fsS \"http://$HOST:$PORT/v1/overview/deployments/worker-prod/resource-inspection?resourceType=ecsTask&resourceName=$ecs_task_resource_name&clusterArn=arn%3Aaws%3Aecs%3Aap-northeast-2%3A123456789012%3Acluster%2Fworker-prod\")",
			"invalid_resource_log_limit_status=$(curl -sS -o \"$INVALID_RESOURCE_LOG_LIMIT_BODY\" -w \"%{http_code}\" \"http://$HOST:$PORT/v1/overview/deployments/worker-prod/resource-logs?resourceType=ecsService&resourceName=worker-prod-service&limit=0\")",
			"invalid_resource_inspection_limit_status=$(curl -sS -o \"$INVALID_RESOURCE_INSPECTION_LIMIT_BODY\" -w \"%{http_code}\" \"http://$HOST:$PORT/v1/overview/deployments/payments-api-prod/resource-inspection?resourceType=candidateAutoScalingGroup&resourceName=payments-api-prod-candidate&limit=0\")",
			"assert limited_scope.get('requestedLimit') == 1",
			"assert limited_scope.get('effectiveLimit') == 1",
			"assert limited_scope.get('returnedEntryCount') == len(limited_worker_resource_logs.get('entries') or [])",
			"assert limited_scope.get('returnedCount') == len(limited_worker_resource_logs.get('entries') or [])",
			"assert limited_scope.get('truncationStatus') in ('truncated', 'not_truncated', 'unknown', 'not_applicable')",
			"assert limited_scope.get('truncated') == (limited_scope.get('truncationStatus') == 'truncated')",
			"assert ecs_task_resource_logs.get('resourceDescription') == 'ECS task selected from live service inspection'",
			"assert ecs_task_resource_inspection.get('resourceDescription') == 'ECS task selected from live service inspection'",
			"assert ecs_task_resource_logs.get('containerName') == 'worker'",
			"assert ecs_task_scope.get('requestedLimit') == 1",
			"assert ecs_task_resource_inspection['resourceType'] == 'ecsTask'",
			"assert_ecs_service_event_ids('worker_resource_inspection', worker_resource_inspection)",
			"assert_ecs_service_event_ids('selected_worker_resource_inspection', selected_worker_resource_inspection)",
			"assert no_log_source.get('truncationStatus') == 'not_applicable'",
			"assert 'recent-log-lookup-context-card' in ui, ui",
		},
		"../../scripts/smoke-docker.sh": {
			"limited_worker_resource_logs=$(curl -fsS \"$API_BASE_URL/v1/overview/deployments/worker-prod/resource-logs?resourceType=ecsService&resourceName=worker-prod-service&limit=1\")",
			"ecs_task_resource_logs=$(curl -fsS \"$API_BASE_URL/v1/overview/deployments/worker-prod/resource-logs?resourceType=ecsTask&resourceName=$ecs_task_resource_name&containerName=worker&clusterArn=arn%3Aaws%3Aecs%3Aap-northeast-2%3A123456789012%3Acluster%2Fworker-prod&limit=1\")",
			"ecs_task_resource_inspection=$(curl -fsS \"$API_BASE_URL/v1/overview/deployments/worker-prod/resource-inspection?resourceType=ecsTask&resourceName=$ecs_task_resource_name&clusterArn=arn%3Aaws%3Aecs%3Aap-northeast-2%3A123456789012%3Acluster%2Fworker-prod\")",
			"invalid_resource_log_limit_status=$(curl -sS -o \"$INVALID_RESOURCE_LOG_LIMIT_BODY\" -w \"%{http_code}\" \"$API_BASE_URL/v1/overview/deployments/worker-prod/resource-logs?resourceType=ecsService&resourceName=worker-prod-service&limit=0\")",
			"invalid_resource_inspection_limit_status=$(curl -sS -o \"$INVALID_RESOURCE_INSPECTION_LIMIT_BODY\" -w \"%{http_code}\" \"$API_BASE_URL/v1/overview/deployments/payments-api-prod/resource-inspection?resourceType=candidateAutoScalingGroup&resourceName=payments-api-prod-candidate&limit=0\")",
			"assert limited_scope.get(\"requestedLimit\") == 1",
			"assert limited_scope.get(\"effectiveLimit\") == 1",
			"assert limited_scope.get(\"returnedEntryCount\") == len(limited_worker_resource_logs.get(\"entries\") or [])",
			"assert limited_scope.get(\"returnedCount\") == len(limited_worker_resource_logs.get(\"entries\") or [])",
			"assert limited_scope.get(\"truncationStatus\") in (\"truncated\", \"not_truncated\", \"unknown\", \"not_applicable\")",
			"assert limited_scope.get(\"truncated\") == (limited_scope.get(\"truncationStatus\") == \"truncated\")",
			"assert ecs_task_resource_logs.get(\"resourceDescription\") == \"ECS task selected from live service inspection\"",
			"assert ecs_task_resource_inspection.get(\"resourceDescription\") == \"ECS task selected from live service inspection\"",
			"assert ecs_task_resource_logs.get(\"containerName\") == \"worker\"",
			"assert_ecs_service_event_ids(\"worker_resource_inspection\", worker_resource_inspection)",
			"assert_ecs_service_event_ids(\"selected_worker_resource_inspection\", selected_worker_resource_inspection)",
			"assert ecs_task_scope.get(\"requestedLimit\") == 1",
			"assert ecs_task_resource_inspection[\"resourceType\"] == \"ecsTask\"",
			"assert no_log_source.get(\"truncationStatus\") == \"not_applicable\"",
			"\"const markResourcePanelStatusNode = (node) => {\"",
			"\"markResourcePanelStatusNode(stateNode);\"",
			"\"node.setAttribute('role', 'status');\"",
			"\"node.setAttribute('aria-live', 'polite');\"",
			"\"node.setAttribute('aria-atomic', 'true');\"",
			"\"recent-log-lookup-context-card\"",
			"\"audit-freshness\"",
			"\"Last refreshed · ${fetchedAt}\"",
			"\"meta.setAttribute('role', 'status');\"",
			"\"meta.setAttribute('aria-live', 'polite');\"",
			"\"meta.setAttribute('aria-atomic', 'true');\"",
		},
	}
	for path, fragments := range smokeScripts {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		text := string(body)
		for _, fragment := range fragments {
			if !strings.Contains(text, fragment) {
				t.Fatalf("expected %s to include %q", path, fragment)
			}
		}
	}
}

func TestResourceInspectionFeatureGuideDocumentsSelectorsAndScope(t *testing.T) {
	fieldFragments := []string{"clusterArn", "limit", "instanceId", "lookupContext", "unavailableReason", "resultScope"}
	guidePath := "../../docs/features/resource-inspection.md"
	guideBody, err := os.ReadFile(guidePath)
	if err != nil {
		t.Fatalf("read resource inspection feature guide: %v", err)
	}
	guideText := string(guideBody)

	for _, fragment := range []string{
		"# Resource Inspection",
		"`GET /v1/overview/deployments/{deploymentName}/resource-inspection?resourceType={type}&resourceName={name}[&clusterArn={ecsClusterArn}][&limit={positiveInteger}][&instanceId={ec2InstanceId}]`",
		"`clusterArn` is optional and only meaningful for `resourceType=ecsService` and `resourceType=ecsTask`",
		"`limit` is optional and applies only to bounded ECS service task samples and direct Auto Scaling Group instance samples",
		"including `userDataBootstrapSource`, ignore the query",
		"`instanceId` is optional and only meaningful for Auto Scaling Group-backed inspection reads",
		"including `userDataBootstrapSource` after it resolves a backing ASG",
		"Like recent logs, every resource-inspection response includes `lookupContext`",
		"Unavailable responses keep the selected route explicit with `available=false`, `unavailableReason`, `lookupContext`, and any accepted selector echo such as `clusterArn` or `instanceId`",
		"`resultScope` is additive metadata for bounded live-inspection samples, not a pagination cursor or provider-wide total guarantee",
		"`resultScope.sampleType`",
		"`resultScope.returnedItemCount`",
		"`resultScope.returnedCount`",
		"`resultScope.truncationStatus`",
		"`resultScope.truncated`",
		"`not_truncated`, `truncated`, `unknown`, or `not_applicable`",
		"`ecsService.eventsScope`, an additive response-window summary",
		"`ecsService.events[].eventId`",
		"When ECS service events are present",
		"`effectiveLimit`, `returnedEventCount`, `truncationStatus`, `truncated`, optional",
		"the timestamps identify the newest and oldest events retained",
		"`Share ECS service event` links to a same-page row anchor for one ECS service event",
		"`Copy ECS service event link` copies the same stable handoff URL without navigating away",
		"eventId-backed anchors remain stable across refreshes",
		"Operators should capture the HTTP `Request-Id` response header together with the `lookupContext` object before escalating",
		"Runtime code remains read-only; this guide documents the shipped live-inspection route contract.",
	} {
		if !strings.Contains(guideText, fragment) {
			t.Fatalf("expected resource inspection feature guide to include %q", fragment)
		}
	}
	for _, fragment := range fieldFragments {
		if !strings.Contains(guideText, fragment) {
			t.Fatalf("expected resource inspection feature guide to keep selector/result wording %q discoverable", fragment)
		}
	}

	files := map[string][]string{
		"../../README.md": {
			"`docs/features/resource-inspection.md` documents the focused live resource inspection route, selectors, lookup context, unavailable states, and detailed field contracts.",
			"Resource drill-down Recent logs and Live inspection panels summarize lookup context, bounded reads, request correlation, and unavailable states without duplicating the detailed resource field contract.",
			"Use `Share ECS service event` or the adjacent `Copy ECS service event link` from a Live inspection event row to hand off one ECS service event without changing the resource-page selector state.",
		},
		"../../docs/architecture/tenet-v1-overview.md": {
			"`docs/features/resource-inspection.md`",
		},
		"../../docs/features/overview-dashboard.md": {
			"`Share ECS service event` or `Copy ECS service event link` on the Live inspection event row",
			"preserves the current resource page query state and changes only the hash",
		},
		"../../docs/architecture/tenet-v1-overview-ui.md": {
			"See `docs/features/resource-inspection.md` for the focused selector, lookup-context, unavailable-state, and `resultScope` contract.",
		},
		"../../docs/architecture/tenet-v1-operator-contracts.md": {
			"See `docs/features/resource-inspection.md` for the focused resource-inspection guide covering `clusterArn`, `limit`, `instanceId`, `lookupContext`, `unavailableReason`, and `resultScope`.",
		},
	}
	for path, fragments := range files {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		text := string(body)
		for _, fragment := range fragments {
			if !strings.Contains(text, fragment) {
				t.Fatalf("expected %s to include %q", path, fragment)
			}
		}
	}
}

func TestDocsDescribeTrustedFrontDoorAuthAndSmokeScope(t *testing.T) {
	files := map[string][]string{
		"../../docs/architecture/tenet-v1-operator-contracts.md": {
			"### Trusted front-door auth",
			"Front-door auth remains additive and opt-in.",
			"The API can now also run explicitly configured Tenet-managed GitHub OAuth, Google Workspace OIDC, or Okta OIDC browser login",
			"`GET /auth/login/github`, `GET /auth/login/google`, or `GET /auth/login/okta` creates one-time callback state",
			"the matching callback route exchanges the code, persists the normalized identity, and issues a signed `tenet_session` cookie",
			"`HttpOnly`, `SameSite=Lax`",
			"`TENET_AUTH_SESSION_COOKIE_SECURE` override",
			"local HTTP development keeps the cookie non-`Secure`",
			"the internal `authidentity` package provides a small JSON-backed identity-store contract for normalized provider identities",
			"the internal `authstate` package provides a JSON-backed OAuth callback state store with expiry and one-time consume semantics for future CSRF/state validation",
			"When the runtime variables are complete",
			"`tenet-api` wires the GitHub provider plus JSON-backed state/identity stores",
			"reports `authRuntime.tenetManagedLogin.active=true` from `GET /v1/operator/context`",
			"The runtime stays disabled when any setting is absent",
			"The shipped managed-login runtime supports GitHub OAuth, Google Workspace OIDC, and Okta OIDC browser login today",
			"Okta SAML remains diagnostic-only and inactive",
			"Okta SAML activation and repository-backed provider group authorization",
			"`GET /v1/operator/context.authRuntime.enterpriseSso` is the provider-neutral diagnostic surface",
			"The legacy `/api/v1/auth/*` route family remains unregistered.",
			"Reviewer verification for managed-login work should include `go test ./internal/api ./cmd/tenet-api`",
			"Local and Docker smoke still cover the default disabled runtime unless those OAuth environment variables are explicitly supplied.",
			"The release guard is deliberately split: smoke proves a default API does not advertise active managed login",
			"focused `internal/api` router/session tests and the Node-backed embedded UI harness use local fakes to prove the configured GitHub, Google, and Okta OIDC login/callback/session/logout paths",
			"Tenet-managed GitHub, Google Workspace OIDC, and Okta OIDC sessions are signed cookies minted only by their configured callback routes; `/auth/logout` only expires that Tenet-managed cookie",
			"The configured fake path also locks that `GET /auth/logout` links use `/auth/logout?returnUrl=...`, stay in the same shell, and explicitly say they clear only the Tenet-managed session cookie",
			"TENET_API_TRUST_AUTHENTICATED_HEADERS",
			"TENET_API_ENFORCE_AUTHENTICATED_IDENTITY",
			"TENET_API_ENFORCE_OPERATOR_ROLE_AUTHORIZATION",
			"TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION",
			"strips any client-supplied `X-Tenet-Authenticated-*`, `X-Tenet-Authenticated-Roles`, `X-Tenet-Authorized-Deployments`, `X-Tenet-Authorized-Projects`, `X-Tenet-Authorized-Environments`, and trusted-alias `X-User-*` headers before forwarding trusted values",
			"Do not expose a header-trusting `tenet-api` instance directly to untrusted clients",
			"Tenet currently has two intentionally separate identity header families",
			"`X-Tenet-Authenticated-*`, `X-Tenet-Authenticated-Roles`, `X-Tenet-Authorized-Deployments`, `X-Tenet-Authorized-Projects`, `X-Tenet-Authorized-Environments` | Authenticating front door only | Request-scoped route authorization",
			"`X-Tenet-Actor-*` plus compatible `X-User-*` aliases | Local proxy, smoke, or trusted integration creating immutable records | Persisted actor metadata",
			"Tenet does not treat raw browser-supplied `X-User-*` headers as a login session",
			"The same trusted front-door read contract covers `GET /v1/overview/accounts/{awsAccountId}`",
			"`GET /v1/applies?status=active&claim=stale`",
			"`GET /v1/applies?status=running`",
			"additive `identityProvenance` source metadata",
			"roles.provenance.source",
			"whether the request used canonical `X-Tenet-Authenticated-*` fields, compatibility `X-User-*` aliases, or the Tenet-managed `managed_session_cookie` source",
			"the canonical supported role-header list from `roles.supportedHeaderNames` (`X-Tenet-Authenticated-Roles`, with no `X-User-*` role alias)",
			"The authentication-boundary reminder distinguishes the default trusted-front-door path from an explicitly active Tenet-managed browser login",
			"`Sign in with Tenet-managed login` handoff that preserves the current bookmarkable path/query/hash as `returnUrl`",
			"When placement data is available, `deploymentAccess.summary` reports both `exactDeploymentMatchedCount` and `projectEnvironmentMatchedCount`",
			"deterministic `projectEnvironmentExamples`",
			"If a front door forwards stale project/environment scopes",
			"Those fields are troubleshooting evidence only: stale project/environment scope diagnostics do not authorize deployment-scoped routes, mint a session, or replace the guarded route checks.",
			"That summary is diagnostic-only: it does not mint sessions, replace OAuth/RBAC, or authorize routes by itself",
			"`GET /v1/overview/deployments/{deploymentName}/resource-logs`, `GET /v1/overview/deployments/{deploymentName}/resource-inspection`",
			"### Smoke verification scope",
			"Use `make smoke-local` for the fastest fixture-backed API/UI regression check when you only need direct-process validation without Docker or Redis.",
			"Local and Docker smoke cover the active-vs-queued/running apply collection split, stale-claim refinement, and terminal exact subsets: `GET /v1/applies?status=active` returns queued plus running records, `GET /v1/applies?status=active&claim=stale` returns running records whose claim lease is stale or expired, `GET /v1/applies?status=running` returns only claimed/in-flight running records, a claimed heartbeat record carries freshness fields, `GET /v1/applies?status=queued` stays the queued-only subset, and `GET /v1/applies?status=succeeded` / `GET /v1/applies?status=cancelled` return only matching terminal records.",
			"The request-actor header contract by sending conflicting `X-Tenet-Actor-*` and `X-User-*` identities together",
			"Bookmarkable `/?page=application&deployment=payments-api-prod`, `/?page=account&accountId=123456789012`, the existing traffic-shift resource route, one representative ECS resource route, representative CodeDeploy `codeDeployApplication` / `blueTaskSetTargetGroup` resource routes, and the weighted `productionListenerRule` resource route",
			"Trusted front-door auth by restarting `tenet-api`",
			"gates guarded application, raw-status collection, account collection, unscoped audit collection, raw-status detail, account detail, grouped-account, immutable plan/apply, deployment-scoped audit reads, plus the exact `GET /v1/overview/deployments/{deploymentName}/resource-logs` and `GET /v1/overview/deployments/{deploymentName}/resource-inspection` drill-down routes.",
			"the unscoped audit collection still returns `{\"entries\":[]}` when no audit rows are visible",
			"Use `make smoke-docker` when you need to validate the containerized local stack itself.",
			"API health, representative endpoints, deployment-events/resource-logs/audit-log JSON shapes, including the compose-backed `codeDeployApplication` recent-log envelope, `testListener` backing-resource metadata, weighted `activeTargetGroup` / `candidateTargetGroup` `no_log_source` recent-log payloads, ECS task drill-down `resource-logs?resourceType=ecsTask&containerName=worker&clusterArn={ecsClusterArn}&limit=1` plus paired live-inspection envelope coverage, bounded `resource-logs?limit=1` provider cap plus router fallback truncation/resultScope metadata",
			"The full `GET /v1/audit-logs` collection with expected `plan.created`, `apply.requested`, and lifecycle `apply.transitioned` action support present",
			"The executor-facing `POST /v1/applies/{applyId}/claim`, `POST /v1/applies/{applyId}/heartbeat`, and `POST /v1/applies/{applyId}/transitions` boundaries on a smoke-created terminal apply record: `400` malformed JSON where applicable, `404` unknown apply, and `409` invalid terminal claim/heartbeat/move with operator-facing text.",
			"The same actor-header precedence check used by direct-process smoke, including `actorAttribution.precedence`, plus the default disabled immutable apply contract.",
			"The Docker smoke also mirrors local smoke's active-vs-queued collection guard by file-backing queued apply records, claiming and heartbeating one record, asserting `GET /v1/applies?status=active` returns queued/running records with heartbeat freshness fields on the running claim, asserting `GET /v1/applies?status=queued` remains the queued-only subset, and asserting `GET /v1/applies?status=succeeded` / `GET /v1/applies?status=cancelled` stay exact terminal subsets.",
			"Trusted front-door auth by restarting the Compose `api` service",
			"controller-authored `managedDrift[]` evidence in `GET /v1/overview/deployments/payments-api-prod` with stable `driftId` / `field` / `desired` / `actual` / `severity` rows, additive detail-level `managedDriftSummary` severity counts, validates `resourceRef` shape when backend-authored metadata is present, preserves optional `source` evidence metadata when known, and keeps the local-friendly no-resourceRef/no-source fallback explicit while aligned `runtimeSummary.managedDrift` remains present",
			"survive the Compose `api` service restart",
			"`TENET_AWS_ENABLE_APPLY=true make docker-up`",
			"representative live-inspection read plus correlated bookmarkable shell reads for the application drill-down, account drill-down, drift workspace, deployment-scoped audit workspace, and bookmarkable plan/apply workspace",
			"CodeDeploy application, task-set target-group, and weighted production listener-rule resource shells",
			"bookmarkable application-detail status summary shell",
			"The raw embedded shell/docs contract also keeps the labels `Plan preview actions`, `Apply request actions`, `Controller runtime events`, `Runtime event navigation`, `Recent logs`, `Resolved backing resource`, `recent-log-lookup-context-card`, `Lookup path ·`, `Lookup region`, `Observer role configured`, and the runtime-events `Open field-level drift` handoff present",
			"resource is a valid drill-down target but has no independent recent-log stream yet",
			"temp files via the shared `write_payload` helper",
			"The compose-backed smoke now covers shipped resource drill-down shells directly",
			"Both smoke lanes now cover the shipped CodeDeploy application, task-set target-group, and weighted `productionListenerRule` bookmarkable resource shells directly",
			"`aws_unavailable` and `no_log_source` as part of the expected local no-AWS contract",
			"valid drill-down target but has no independent recent-log stream yet",
			"Linux CI runners",
			"syntax-checks `scripts/smoke-local.sh`, `scripts/smoke-docker.sh`, and the runner-start evidence helper `scripts/ci-smoke-runner-evidence.sh` with `bash -n`, then runs `make smoke-local`",
			"on pull requests, a cheap `docker-smoke-scope` job first checks changed paths",
			"runs the full `make smoke-docker` container stack only when the PR touches Compose/front-door/auth/API/controller, local desired-state/status fixtures, catalog/domain/placement status read-model contracts, GitHub webhook, RBAC policy diagnostics, resource evidence/logs/inspection, or plan/apply/audit persistence smoke surfaces",
			"fixture inputs (`examples/local-repo/**`, `examples/local-status/**`)",
			"Fixture-only desired-state/status PRs can change release-gated resource evidence, placement, recent-log, managed-drift, and embedded smoke behavior",
			"Runner-start failures usually show no job steps or downloadable repo logs and may include GitHub account-level annotations such as recent account payments failing or the spending limit needing an increase.",
			"use local evidence from `go test ./...`, `make smoke-local`, and `make smoke-docker` for code-level confidence",
			"Embedded UI behavior harnesses in `internal/api/ui_behavior_test.go` execute extracted JavaScript with the local `node` binary",
			"including the current local Node.js 25 runtime",
			"the Go test diagnostic prints the Node.js version and the temporary harness script path",
			"go test ./internal/api -run 'TestResourceDetailPageWithNodeHarness|TestAuditTimelinePageWithNodeHarness' -count=1",
			"See `docs/runbooks/ci-smoke-runner-start.md` and `scripts/ci-smoke-runner-evidence.sh <run-id>` for the exact `gh run view` plus Actions jobs API evidence checklist, rerun-target selection, post-recovery release-health checklist, recovery steps, and closeout comment template for issue #850; use `scripts/ci-smoke-runner-evidence.sh --summary <run-id>`",
			"The post-recovery checklist requires non-empty `steps[]`, a real runner assignment, fast-lane evidence that `go test ./...`, smoke-script syntax checks, and `make smoke-local` were reached",
			"classify any remaining failures as normal actionable CI failures rather than continuing to label them as the billing/spending-limit blocker",
			"Unrelated docs-only PRs keep the cheap fast lane without the Docker stack",
			"`internal/catalog/**`",
			"`internal/domain/**`",
			"`internal/placement/**`",
			"`internal/resourceaws/**`",
			"reserved Okta SAML/enterprise runtime paths `internal/authsaml/**` and `internal/authsso/**`",
			"make smoke-docker",
		},
		"../../docs/architecture/tenet-v1-local-runtime.md": {
			"## Trusted front-door auth",
			"Front-door auth remains additive and opt-in.",
			"The local runtime can now also run an explicitly configured Tenet-managed GitHub OAuth, Google Workspace OIDC, or Okta OIDC browser login",
			"the internal `authidentity` package contains a small JSON-backed normalized identity-store contract",
			"the internal `authstate` package contains a JSON-backed OAuth callback state store with expiry and one-time consume semantics for future CSRF/state validation",
			"the internal `authstate` package contains a JSON-backed OAuth callback state store with expiry and one-time consume semantics",

			"When all runtime settings are present",
			"`tenet-api` loads the selected provider plus JSON-backed state/identity stores",
			"reports `GET /v1/operator/context.authRuntime.tenetManagedLogin.active=true`",
			"When any setting is absent the runtime stays disabled",
			"The active managed-login runtime supports GitHub OAuth, Google Workspace OIDC, and Okta OIDC today",
			"Okta SAML provider selection remains diagnostic-only, while provider-token refresh/revocation, upstream provider/front-door logout, and long-lived token lifecycle management remain future work",

			"non-secret `requiredSettings` and `missingSettings` arrays",
			"The legacy `/api/v1/auth/*` route family remains unregistered.",
			"Reviewer verification for managed-login changes should include `go test ./internal/api ./cmd/tenet-api`",
			"Local and Docker smoke intentionally keep the default disabled runtime unless those OAuth environment variables are explicitly supplied.",
			"Treat managed-login release coverage as two complementary checks: smoke asserts the out-of-box `authRuntime.tenetManagedLogin.active=false` / trusted-front-door recovery path",
			"focused router/session plus Node-backed embedded UI tests use local fake provider/context data to prove the configured active login, logout cookie clearing, expanded Operator context sign-out handoff, and sign-in handoff path without live GitHub, Google, or Okta credentials",
			"The configured fake path also locks that the sign-out handoff uses `/auth/logout?returnUrl=...`, stays in the same shell, and tells operators it clears only the Tenet-managed session cookie",
			"TENET_API_TRUST_AUTHENTICATED_HEADERS",
			"TENET_API_ENFORCE_AUTHENTICATED_IDENTITY",
			"TENET_API_ENFORCE_OPERATOR_ROLE_AUTHORIZATION",
			"TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION",
			"strips any client-supplied `X-Tenet-Authenticated-*`, `X-Tenet-Authenticated-Roles`, `X-Tenet-Authorized-Deployments`, `X-Tenet-Authorized-Projects`, `X-Tenet-Authorized-Environments`, and trusted-alias `X-User-*` headers before forwarding trusted values",
			"Do not expose a header-trusting `tenet-api` instance directly to untrusted clients",
			"Tenet currently has two intentionally separate identity header families",
			"`X-Tenet-Authenticated-*`, `X-Tenet-Authenticated-Roles`, `X-Tenet-Authorized-Deployments`, `X-Tenet-Authorized-Projects`, `X-Tenet-Authorized-Environments` | Authenticating front door only | Request-scoped route authorization",
			"`X-Tenet-Actor-*` plus compatible `X-User-*` aliases | Local proxy, smoke, or trusted integration creating immutable records | Persisted actor metadata",
			"Tenet does not treat raw browser-supplied `X-User-*` headers as a login session",
			"The same trusted front-door read contract also covers `GET /v1/overview/accounts/{awsAccountId}`",
			"`GET /v1/applies?status=active&claim=stale`",
			"additive `identityProvenance` source metadata",
			"roles.provenance.source",
			"compatibility `X-User-*` aliases accepted by this Tenet build",
			"only changes embedded `401` recovery into a `_self` managed sign-in handoff when `active=true` plus a same-origin `loginRoute` is present",
			"disabled/inactive managed-login deployments tell the operator to reopen the same bookmarkable URL through the trusted front door",
			"active Tenet-managed login deployments can offer `_self` `Sign in with Tenet-managed login` with the current URL preserved as `returnUrl`",
			"`deploymentAccess.summary` additively reports how many forwarded deployment names or project/environment scopes matched catalog deployments",
			"`projectEnvironmentExamples` shows which visible project/environment groups",
			"including the region, AWS account, deployment count, and bounded example deployments for each diagnostic group",
			"When forwarded project/environment scopes are stale",
			"remain troubleshooting evidence only. They explain likely empty collections or scoped deployment-route denials, but they do not mint a session or replace the guarded route checks.",
			"it does not mint sessions, perform OAuth, authorize routes, or replace the guarded route checks",
			"`GET /v1/overview/deployments/{deploymentName}/resource-logs`, `GET /v1/overview/deployments/{deploymentName}/resource-inspection`",
			"Use `make smoke-local` for the fastest fixture-backed regression check when you only need direct-process validation.",
			"Local and Docker smoke cover the active-vs-queued/running apply collection split, stale-claim refinement, and terminal exact subsets: `GET /v1/applies?status=active` returns queued plus running records, `GET /v1/applies?status=active&claim=stale` returns running records whose claim lease is stale or expired, `GET /v1/applies?status=running` returns only claimed/in-flight running records, a claimed heartbeat record carries freshness fields, `GET /v1/applies?status=queued` stays the queued-only subset, and `GET /v1/applies?status=succeeded` / `GET /v1/applies?status=cancelled` return only matching terminal records.",
			"Additive `X-User-*` alias support plus explicit `X-Tenet-Actor-*` precedence by sending both header families with conflicting identities",
			"Bookmarkable `/?page=application&deployment=payments-api-prod`, `/?page=account&accountId=123456789012`, the existing traffic-shift resource route, one representative ECS resource route, representative CodeDeploy `codeDeployApplication` / `blueTaskSetTargetGroup` resource routes, and the weighted `productionListenerRule` resource route",
			"Trusted front-door auth by restarting `tenet-api`",
			"gates guarded application, raw-status collection, account collection, unscoped audit collection, raw-status detail, account detail, grouped-account, immutable plan/apply, deployment-scoped audit reads, plus the exact `GET /v1/overview/deployments/{deploymentName}/resource-logs` and `GET /v1/overview/deployments/{deploymentName}/resource-inspection` drill-down routes.",
			"trusted `X-Tenet-Authenticated-Roles: viewer` deployment detail/status/events and immutable plan/apply reads succeed through matching `X-Tenet-Authorized-Projects` plus `X-Tenet-Authorized-Environments` scopes even when `X-Tenet-Authorized-Deployments` does not name the deployment",
			"stale project/environment scopes return empty guarded collections",
			"The bookmarkable `/?page=plan` workspace, but only behind a trusted front door that strips inbound trust headers, including `X-User-*` aliases, before forwarding its own values",
			"### Docker stack smoke check",
			"make smoke-docker",
			"`docker compose config` plus the `api`, `controller`, and `redis` services present in `compose.yaml`",
			"The full `GET /v1/audit-logs` collection, exact-action filtering with `GET /v1/audit-logs?action=apply.requested&limit=2`, lifecycle-transition action support for `apply.transitioned`, and `400 Bad Request` on `action=invalid.action`",
			"controller-authored `managedDrift[]` evidence in `GET /v1/overview/deployments/payments-api-prod` with stable `driftId` / `field` / `desired` / `actual` / `severity` rows, additive detail-level `managedDriftSummary` severity counts, validates `resourceRef` shape when backend-authored metadata is present, preserves optional `source` evidence metadata when known, and keeps the local-friendly no-resourceRef/no-source fallback explicit while aligned `runtimeSummary.managedDrift` remains present",
			"The same direct-process actor coverage by sending both header families with conflicting identities on a `payments-api-staging` plan/apply flow",
			"The Docker smoke also mirrors local smoke's active-vs-queued collection guard by file-backing queued apply records, claiming and heartbeating one record, asserting `GET /v1/applies?status=active` returns queued/running records with heartbeat freshness fields on the running claim, asserting `GET /v1/applies?status=queued` remains the queued-only subset, and asserting `GET /v1/applies?status=succeeded` / `GET /v1/applies?status=cancelled` stay exact terminal subsets.",
			"Bookmarkable application, account, drift, audit, plan workspace, and representative resource URLs, checking raw embedded application/account/resource/drift/audit/plan shell HTML/script fragments instead of runtime DOM text",
			"bookmarkable application-detail status summary shell",
			"Drift page source/summary/table copy, audit filter copy such as `Audit filters: action Apply requested (apply.requested) · Limit cap: latest 2` plus lifecycle transition filter support for `Apply lifecycle transitioned (apply.transitioned)`, audit row `Source record · ...` / `Source detail · ...` metadata plus `Open source record` / `Open source detail` raw JSON links, `Share audit entry` self-links for row-level handoffs, the audit control recovery affordance, and correlated `http_request` logs",
			"Trusted front-door auth by restarting the Compose `api` service",
			"authenticated but unauthorized transitions fail with `403`",
			"survive the Compose `api` service restart",
			"`TENET_AWS_ENABLE_APPLY=true make docker-up`",
			"correlated structured `http_request` logs with stable fields such as `request_id`, `method`, `path`, `status`, `duration_ms`, and `result` for a representative live-inspection read, the bookmarkable application drill-down shell request, bookmarkable account drill-down shell request, bookmarkable drift workspace request, the deployment-scoped audit workspace request, and the bookmarkable plan/apply workspace shell request",
			"`Plan preview actions`, `Apply request actions`, `Controller runtime events`, `Recent logs`, `Resolved backing resource`, `recent-log-lookup-context-card`, `Lookup path ·`, `Lookup region`, `Observer role configured`, and the runtime-events `Open field-level drift` handoff",
			"temp files through the shared `write_payload` helper",
			"bookmarkable bootstrap-source resource page plus the paired `userDataBootstrapSource` `resource-logs` and `resource-inspection` routes, the `codeDeployApplication` recent-log route, the `testListener` recent-log backing-resource metadata, weighted target-group `no_log_source` recent-log payloads, the selected `userDataBootstrapSource` recent-log probe with `instanceId=i-0abc123def4567890` plus echoed selector/backing-ASG metadata on explicit unavailable payloads, the selected `userDataBootstrapSource` live-inspection probe with `instanceId=i-0abc123def4567890` plus echoed selector/backing-ASG metadata when available, the selected ASG live-inspection probe `resource-inspection?resourceType=candidateAutoScalingGroup&resourceName=payments-api-prod-candidate&instanceId=i-0abc123def4567890` with echoed selector/lookup context/typed ASG evidence, malformed `instanceId=i-` `400 Bad Request` / `invalid instanceId query` validation on both selected route families before provider lookup, the `candidateAutoScalingGroup` bounded `resource-inspection?limit=2` sample with `autoScalingGroup.instancesScope` fields when available plus explicit unavailable semantics without AWS access",
			"bookmarkable CodeDeploy application, `blueTaskSetTargetGroup`, and `productionListenerRule` resource pages and the local-friendly traffic-routing inspection fallback",
			"CodeDeploy deployment-group status cues such as CodeDeploy application",
			"without moving that noise onto the applications overview",
			"When live target-group/listener rows match exactly one normalized rollout resource, they add `_self` `Open resource detail` handoffs back into the bookmarkable resource drill-down",
			"`aws_unavailable` / `no_log_source` unavailable payloads as expected local-runtime outcomes, including route-stable weighted target-group resources that have no independent recent-log stream",
			"The same raw-shell checks separately lock the ECS service `tasksScope` renderer and `Task sample scope` copy",
			"without requiring live AWS task inventory in local or compose runs",
			"Linux CI runners with tighter environment-size limits",
			"If GitHub reports a CI Smoke failure before any repository step starts, classify it as a runner-start failure rather than a Tenet smoke result.",
			"no job steps, no downloadable repo logs",
			"failed payments or a spending limit that needs to be increased",
			"The focused runbook in `docs/runbooks/ci-smoke-runner-start.md` and helper `scripts/ci-smoke-runner-evidence.sh <run-id>` show the exact `gh run view` plus Actions jobs API commands, the `runner_id=0` / `steps=[]` evidence pattern, rerun-target selection, post-recovery release-health checklist, and closeout comment template for issue #850; `scripts/ci-smoke-runner-evidence.sh --summary <run-id>`",
			"For the first recovered `main` run, require non-empty `steps[]`, real runner assignment, fast-lane evidence that `go test ./...`, smoke-script syntax checks, and `make smoke-local` were reached",
			"remaining red checks are normal actionable CI failures: inspect logs, reproduce locally, and link the repo-side follow-up",
			"rely on local `go test ./...`, `make smoke-local`, and `make smoke-docker` output for code-level reproduction",
			"syntax-checks `scripts/smoke-local.sh`, `scripts/smoke-docker.sh`, and the runner-start evidence helper `scripts/ci-smoke-runner-evidence.sh` with `bash -n`, then runs `make smoke-local`",
			"Pull requests first pass through a cheap `docker-smoke-scope` path filter",
			"only PRs that touch Compose/front-door/auth/API/controller, local desired-state/status fixtures, catalog/domain/placement status read-model contracts, GitHub webhook, RBAC policy diagnostics, resource evidence/logs/inspection, or plan/apply/audit persistence smoke surfaces",
			"fixture inputs (`examples/local-repo/**`, `examples/local-status/**`)",
			"Fixture-only desired-state/status PRs can change release-gated resource evidence, placement, recent-log, managed-drift, and embedded smoke behavior",
			"`internal/githubwebhook/**`",
			"`internal/catalog/**`",
			"`internal/domain/**`",
			"`internal/placement/**`",
			"`internal/resourceaws/**`",
			"reserved Okta SAML/enterprise runtime paths `internal/authsaml/**` and `internal/authsso/**`",
			"unrelated docs-only PRs on the cheap fast lane",
			"`docker compose down --remove-orphans` even on failure",
		},
		"../../docs/architecture/tenet-v1-overview-ui.md": {
			"diagnostic `deploymentAccess.summary` JSON when Tenet can compare forwarded deployment allowlists or project/environment scopes with catalog placements",
			"project/environment evidence",
			"the summary is not an authorization source and the UI must not make client-side access decisions from it",
			"`/?page=plan&deployment={deploymentName}&planId={planId}` workspace keeps the URL shareable/bookmarkable",
			"`POST /v1/applies/{applyId}/transitions`",
			"when `POST /v1/plans/deployments/{deploymentName}`, `POST /v1/applies/plans/{planId}`, or `POST /v1/applies/{applyId}/transitions` returns `401 Unauthorized` under auth enforcement, the embedded workspace explains that authenticated identity is required on operator write routes and keeps recovery mode-aware instead of showing generic fetch-error copy: default trusted-front-door deployments tell operators to retry the same bookmarkable plan/apply action through the front door, while an active same-origin Tenet-managed GitHub login diagnostic points operators at the configured sign-in handoff and tells them to return to that same workspace before retrying the write",
			"either an exact trusted `X-Tenet-Authorized-Deployments` grant for the requested deployment, persisted preview deployment, or persisted apply deployment",
			"trusted front door",
			"an authentication-boundary reminder that keeps trusted front-door recovery as the default while noting active Tenet-managed browser login when the optional login/callback/session/logout routes report active",
			"managed-login runtime diagnostics from `authRuntime.tenetManagedLogin`",
			"`Retry current page after front-door sign-in`",
			"`Open Operator context details`",
			"`Retry current page after access update`",
			"bookmarkable `/?page=resource&deployment=payments-api-prod&resourceType=userDataBootstrapSource&resourceId=userDataBootstrapSource::file:userdata/payments-api-prod.sh::Normalized userdata bootstrap source for candidate EC2 capacity` page",
			"bookmarkable `/?page=resource&deployment=payments-api-staging&resourceType=codeDeployApplication&resourceId=codeDeployApplication::payments-api-staging-codedeploy-app::CodeDeploy application coordinating ECS traffic shifting` page",
			"bookmarkable `/?page=resource&deployment=payments-api-staging&resourceType=blueTaskSetTargetGroup&resourceId=blueTaskSetTargetGroup::payments-api-staging-blue::Blue task set target group CodeDeploy treats as the live ECS traffic destination` page",
			"bookmarkable `/?page=resource&deployment=payments-api-prod&resourceType=productionListenerRule&resourceId=productionListenerRule::payments-api-prod-prod-listener::Production listener rule operators inspect while Tenet updates weighted forwarding during rollout` page",
			"summary-only traffic-routing fallbacks and explicit unavailable reasons",
			"live target-group and listener rows add `_self` `Open resource detail` handoffs only when their identifiers resolve to exactly one normalized `rolloutExecutionResources()` resource",
		},
		"../../docs/features/overview-dashboard.md": {
			"diagnostic identity header provenance (`identityProvenance.idSource` / `nameSource` / `emailSource`)",
			"Node-backed behavior tests in `internal/api/ui_behavior_test.go`; harness failures include the local Node.js version and temporary script path",
			"current local verification covers Node.js 25 with `-count=1` cache-bypass runs",
			"authentication boundary (trusted front-door recovery by default; active Tenet-managed browser login when optional login/callback/session/logout routes report active)",
			"managed-login runtime state from `authRuntime.tenetManagedLogin`",
			"roles.provenance.source",
			"the canonical role header list from `roles.supportedHeaderNames` in expanded `Show access details` / `#operator-context-access-details-panel` (`X-Tenet-Authenticated-Roles`; no `X-User-*` role alias)",
			"trusted identity source wiring (`canonical` for `X-Tenet-Authenticated-*`, `alias` for compatibility `X-User-*`, `managed_session_cookie` for Tenet-managed session cookies rendered as `Tenet-managed browser session cookie`, `absent` for missing fields)",
			"Release-guard coverage intentionally does not require live GitHub, Google, or Okta credentials",
			"provider-token refresh/revocation and long-lived token lifecycle management remain future work rather than embedded-dashboard behavior operators should expect today",
			"the Node harness injects an active managed-login diagnostic to verify the `_self` sign-in link preserves the current bookmarkable path/query/hash as `returnUrl`",
			"when `TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION=true` is enabled, `GET /v1/overview/applications` requires trusted `X-Tenet-Authenticated-*` identity",
			"When `TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION=true` is enabled, the same narrow read gate also applies to `GET /v1/overview/deployments/{deploymentName}/events`, `GET /v1/overview/deployments/{deploymentName}/resource-logs`, and `GET /v1/overview/deployments/{deploymentName}/resource-inspection`",
			"Recognized-identity read `403` cards instead offer `Open Operator context details` plus `Retry current page after access update`",
			"diagnostic `deploymentAccess.summary` JSON that reports matched visible deployments from exact names or project/environment scopes plus total unmatched forwarded deployment-name counts",
			"deterministic project/environment examples",
			"project/environment examples with region, AWS account, deployment count, and bounded example deployment names",
			"not a client-side authorization decision",
			"when `TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION=true` is enabled, stored `GET /v1/plans/{planId}` reloads reuse the same trusted `X-Tenet-Authenticated-*` identity",
			"when `TENET_API_ENFORCE_OPERATOR_ROLE_AUTHORIZATION=true` is enabled, `POST /v1/plans/deployments/{deploymentName}`",
			"unavailableReason=no_log_source` render friendly copy that the target has no independent recent-log stream yet",
			"codeDeployDeploymentGroup` live payloads render only at the resource-detail Live inspection depth as CodeDeploy deployment-group status cues",
			"ECS service bindings, latest CodeDeploy deployment summary, listener ARNs, and target groups",
			"rollback, error, alarm, or deployment-overview diagnostics",
			"scannable status pill, operator next-action cue",
			"live `trafficRoutingResource.listenerRule` fields are intentionally AWS-only",
			"when `TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION=true` is enabled, stored `GET /v1/applies/{applyId}` reloads use the same trusted `X-Tenet-Authenticated-*` identity",
		},
		"../../docs/architecture/tenet-v1-audit-log-api.md": {
			"TENET_API_TRUST_AUTHENTICATED_HEADERS",
			"TENET_API_ENFORCE_AUTHENTICATED_IDENTITY",
			"TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION",
			"Unscoped `GET /v1/audit-logs` now follows the same collection-style contract as the guarded overview/status collections under read enforcement",
		},
	}

	for path, fragments := range files {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		text := string(body)
		for _, fragment := range fragments {
			if !strings.Contains(text, fragment) {
				t.Fatalf("expected %s to include %q", path, fragment)
			}
		}
	}
	dockerSmokeBody, err := os.ReadFile("../../scripts/smoke-docker.sh")
	if err != nil {
		t.Fatalf("read ../../scripts/smoke-docker.sh: %v", err)
	}
	dockerSmoke := string(dockerSmokeBody)
	for _, fragment := range []string{
		"DEPLOYMENT_DETAIL_JSON=\"$deployment_detail\"",
		"PLAN_PREVIEW_JSON=\"$plan_preview\"",
		"APPLY_RECORD_JSON=\"$apply_record\"",
		"QUEUED_PLAN_PREVIEW_JSON=\"$queued_plan_preview\"",
		"QUEUED_APPLY_RECORD_JSON=\"$queued_apply_record\"",
		"fetch(`/v1/overview/deployments/${encodeURIComponent(deploymentName)}/resource-logs?${params.toString()}`)",
	} {
		if strings.Contains(dockerSmoke, fragment) {
			t.Fatalf("expected smoke-docker.sh to avoid stale or env-blob fragment %q", fragment)
		}
	}
	for _, fragment := range []string{
		"operator_context_stale_scopes=$(curl -fsS \\",
		"X-Tenet-Authorized-Projects: payments-api, archived-payments",
		"X-Tenet-Authorized-Environments: staging-apne2, stale-env",
		"operator_context_stale_scopes_path=$(write_payload operator-context-stale-scopes.json \"$operator_context_stale_scopes\")",
		"OPERATOR_CONTEXT_STALE_SCOPES_PATH=\"$operator_context_stale_scopes_path\"",
		"operator_context_stale_scopes = json.load(fh)",
		"assert stale_scopes_summary[\"forwardedProjectCount\"] == 2, operator_context_stale_scopes",
		"assert stale_scopes_summary[\"forwardedEnvironmentCount\"] == 2, operator_context_stale_scopes",
		"assert stale_scopes_summary[\"unmatchedProjects\"] == [\"archived-payments\"], operator_context_stale_scopes",
		"assert stale_scopes_summary[\"unmatchedEnvironments\"] == [\"stale-env\"], operator_context_stale_scopes",
	} {
		if !strings.Contains(dockerSmoke, fragment) {
			t.Fatalf("expected smoke-docker.sh to validate stale project/environment scope diagnostics fragment %q", fragment)
		}
	}
	for _, fragment := range []string{
		"resource_refs = [row.get(\"resourceRef\") for row in managed_drift if isinstance(row.get(\"resourceRef\"), dict)]",
		"for ref in resource_refs:",
		"assert isinstance(ref.get(\"resourceType\"), str) and ref[\"resourceType\"]",
		"assert isinstance(ref.get(\"resourceName\"), str) and ref[\"resourceName\"]",
	} {
		if !strings.Contains(dockerSmoke, fragment) {
			t.Fatalf("expected smoke-docker.sh to validate managed drift resourceRef fragment %q", fragment)
		}
	}
	for _, fragment := range []string{
		"assert_fragments(\"application_ui\", application_ui, (",
		"\"const applicationStatusSignal = (detail) => {\",",
		"\"const applicationRuntimeEventSummary = (detail) => {\",",
		"\"const applicationManagedDriftCounts = (detail) => {\",",
		"\"const applicationManagedDriftSummaryText = (detail) => {\",",
		"\"No managed drift reported\",",
		"\"const createApplicationStatusSummary = (detail, resourceGroups = []) => {\",",
		"\"Current application status\",",
		"\"Application rollout context:\",",
		"\"['Managed drift', applicationManagedDriftSummaryText(detail)],\",",
		"\"['Runtime events', applicationRuntimeEventSummary(detail)],\",",
		"\"const applicationStatusSectionID = 'application-current-status';\",",
		"\"applicationStatusSection.id = applicationStatusSectionID;\",",
		"\"pushApplicationSectionLink(applicationStatusSectionID, 'Current application status');\",",
		"\"Quick navigation\",",
		"\"Applications overview\",",
		"\"Current destination\",",
		"\"createNavLink(buildPageURL('applications'), 'Applications overview', '_self')\",",
		"\"createQuickNavChip('Application detail', 'Current destination: Application detail', 'page')\",",
	} {
		if !strings.Contains(dockerSmoke, fragment) {
			t.Fatalf("expected smoke-docker.sh to lock bookmarkable application quick-nav fragment %q", fragment)
		}
	}
	for _, fragment := range []string{
		"assert_fragments(\"account_ui\", account_ui, (",
		"\"Quick navigation\",",
		"\"Applications overview\",",
		"\"Current destination\",",
		"\"Current section\",",
		"\"No section is currently targeted.\",",
		"\"On this page\",",
		"\"createNavLink(buildPageURL('applications'), 'Applications overview', '_self')\",",
		"\"createQuickNavChip('Account detail', 'Current destination: Account detail', 'page')\",",
		"\"Current account read: ${accountDetailReadRoute(account.awsAccountId)}\",",
		"\"account-detail-read-route\",",
		"\"Copy account read route\",",
		"\"Copy group read route\",",
		"\"accountLinks.append(createCopyReadRouteButton(accountDetailReadRoute(account.awsAccountId), 'Copy account read route'));\",",
		"\"groupLinks.append(createCopyReadRouteButton(accountGroupDetailReadRoute(account.awsAccountId, group), 'Copy group read route'));\",",
		"\"const accountGroupDetailReadRoute = (accountID, group) => {\",",
		"\"createQuickNavChip(currentSectionLink.label, `Current section: ${currentSectionLink.label}`, 'location')\",",
		"\"target.focus({ preventScroll: true });\",",
		"\"focusCurrentHashTarget(target);\",",
		"\"detailSummary.id = 'account-summary';\",",
		"\"const accountGroupSectionLabel = (group) => {\",",
		"\"return region === '—' ? `${app} · ${environment}` : `${app} · ${environment} · ${region}`;\",",
		"\"groupSection.id = accountGroupSectionID(group);\",",
		"\"href: buildPageSectionURL('account', sectionID, { accountId: accountID }),\",",
	} {
		if !strings.Contains(dockerSmoke, fragment) {
			t.Fatalf("expected smoke-docker.sh to lock bookmarkable account quick-nav fragment %q", fragment)
		}
	}
	for _, fragment := range []string{
		"for body in (resource_ui, ecs_resource_ui, bootstrap_resource_ui, codedeploy_application_resource_ui, blue_task_set_target_group_resource_ui, production_listener_rule_resource_ui):",
		"\"const renderResourceDetailPage = (detail, resourceState = null) => {\",",
		"\"const resourceDetailReadRoute = (deploymentName) => `GET /v1/overview/deployments/${deploymentName || '{deploymentName}'}`;\",",
		"\"const createResourceDetailActions = (resourceState = null, detail = null, resource = null) => [\",",
		"\"Current resource read: ${resourceDetailReadRoute(deploymentName)}. Loaded before focused Recent logs and Live inspection reads.\",",
		"\"workspace-callout-factors resource-read-route resource-current-read-cue\",",
		"\"Refresh resource detail\",",
		"\"Refreshing resource detail from GET /v1/overview/deployments/{deploymentName}…\",",
		"\"Current resource detail remains visible below.\",",
		"\"Failed to refresh resource detail:\",",
		"\"renderResourceDetailPage(resourceState.detail, resourceState);\",",
		"\"return fetchDeploymentDetail(deploymentName)\",",
		"assert_fragments(\"resource_ui\", body, operator_read_auth_fragments + (",
		"\"const createResourceLogsActions = (section, deploymentName, resource, isLoading = false, detail = null) => {\",",
		"\"const selectedLimit = selectedResourceLogsLimit(deploymentName, resource);\",",
		"\"const readRoute = deploymentName && resource?.type && resource?.name ? resourceLogsReadRoute(deploymentName, resource, selectedLimit) : '';\",",
		"\"return [\",",
		"\"createResourceLogsLimitControl(section, deploymentName, resource, isLoading, detail)\",",
		"\"'Log limit'\",",
		"\"'Refresh recent logs'\",",
		"\"const buildResourceLogsRoute = (deploymentName, resource, limit = 0) => {\",",
		"\"const resourceLogsReadRoute = (deploymentName, resource, limit = 0) => `GET ${buildResourceLogsRoute(deploymentName, resource, limit)}`;\",",
		"\"renderResourceLogsState(section, 'Loading recent logs…', '', pendingContext(), [], loadingActions);\",",
		"\"Log entry cap · ${resourceLogsLimitText(limit)}\",",
		"\"params.set('limit', String(normalizedLimit));\",",
		"\"`Failed to load recent logs: ${error.message}`\",",
		"\"'Recent logs'\",",
		"\"'Recent logs request'\",",
		"\"resourceLogsReadRoute(deploymentName, resource, target?.limit ?? selectedLimit)\",",
		"\"const createResourceInspectionActions = (section, detail, resource, isLoading = false) => {\",",
		"\"const deploymentName = resourceInspectionDeploymentName(detail);\",",
		"\"const selectedLimit = selectedResourceInspectionLimit(deploymentName, resource);\",",
		"\"const readRoute = deploymentName && resource?.type && resource?.name ? resourceInspectionReadRoute(deploymentName, resource, selectedLimit) : '';\",",
		"\"return [\",",
		"\"createResourceInspectionLimitControl(section, detail, resource, isLoading)\",",
		"\"'Task sample limit'\",",
		"\"'Refresh live inspection'\",",
		"\"const buildResourceInspectionRoute = (deploymentName, resource, limit = 0) => {\",",
		"\"const resourceInspectionReadRoute = (deploymentName, resource, limit = 0) => `GET ${buildResourceInspectionRoute(deploymentName, resource, limit)}`;\",",
		"\"const createECSTaskScopeSummary = (scope) => {\",",
		"\"Task sample scope · returned ${countText(returnedTaskCount)} task(s) from effective limit ${countText(effectiveLimit)} · ${truncationStatus}\",",
		"\"[createECSTaskScopeSummary(service?.tasksScope)]\",",
		"\"renderResourceInspectionState(section, 'Loading live inspection…', '', pendingContext(), [], loadingActions);\",",
		"\"`Failed to load live inspection: ${error.message}`\",",
		"\"'Live inspection'\",",
		"\"'Live inspection request'\",",
		"\"resourceInspectionReadRoute(deploymentName, resource, targetLimit)\",",
	} {
		if !strings.Contains(dockerSmoke, fragment) {
			t.Fatalf("expected smoke-docker.sh to lock resource refresh fragment %q", fragment)
		}
	}
}

func TestDocsDescribeCISmokeRunnerStartRunbook(t *testing.T) {
	files := map[string][]string{
		"../../docs/architecture/tenet-v1-operator-contracts.md": {
			"docs/runbooks/ci-smoke-runner-start.md",
			"scripts/ci-smoke-runner-evidence.sh <run-id>",
			"scripts/ci-smoke-runner-evidence.sh --summary <run-id>",
			"exact `gh run view` plus Actions jobs API evidence checklist, rerun-target selection, post-recovery release-health checklist, recovery steps, and closeout comment template for issue #850",
			"run URL, head SHA, runner assignment, missing-log, Docker-skip, and billing/spending-limit evidence needs to be pasted as concise Markdown",
			"The post-recovery checklist requires non-empty `steps[]`, a real runner assignment",
			"A rerun is not recovery evidence until the current run or replacement run shows real runner assignment",
		},
		"../../docs/architecture/tenet-v1-local-runtime.md": {
			"docs/runbooks/ci-smoke-runner-start.md",
			"scripts/ci-smoke-runner-evidence.sh <run-id>",
			"scripts/ci-smoke-runner-evidence.sh --summary <run-id>",
			"`runner_id=0` / `steps=[]` evidence pattern",
			"rerun-target selection, post-recovery release-health checklist, and closeout comment template for issue #850",
			"concise Markdown for PR or issue comments",
			"remaining red checks are normal actionable CI failures",
			"Do not count a rerun as recovered while it still reports `runner_id=0`, `steps=[]`, or lacks downloadable/actionable repository logs.",
		},
		"../../README.md": {
			"scripts/ci-smoke-runner-evidence.sh --summary <run-id>",
			"The detailed runner-start evidence bundle remains available by omitting",
			"`docs/runbooks/ci-smoke-runner-start.md`",
		},
		"../../docs/features/resource-inspection.md": {
			"latest-deployment, rollback, error, alarm",
		},
		"../../docs/runbooks/ci-smoke-runner-start.md": {
			"# CI Smoke runner-start failures",
			"scripts/ci-smoke-runner-evidence.sh <run-id>",
			"scripts/ci-smoke-runner-evidence.sh --summary <run-id>",
			"`--summary` and `--markdown` are aliases.",
			"run URL and head SHA",
			"Docker smoke skipped note",
			"missing failed-log note",
			"The helper below prints the same evidence bundle",
			"prints a compact `Runner-start verdict` section before the raw evidence",
			"likely pre-runner blocker when `Fast smoke` or `Docker smoke scope` has `runner_id=0` or an empty `runner_name`, and `steps=[]`",
			"treats missing downloadable failed logs as a runner-start signal to record rather than as a script failure",
			"If you need to collect each piece manually or paste individual commands into a PR comment",
			"`runner_id=0`",
			"`steps=[]`",
			"gh run view <run-id> --json databaseId,name,status,conclusion,headSha,url",
			"gh api \"repos/:owner/:repo/actions/runs/<run-id>/jobs\"",
			"runner_id,runner_name,steps",
			"gh run view <run-id> --log-failed",
			"Capture the check-run annotations too",
			"gh api --paginate \"repos/:owner/:repo/commits/${SHA}/check-runs\"",
			"select(.name == \"Fast smoke\" or .name == \"Docker smoke scope\" or .name == \"Docker smoke\")",
			"gh api --paginate \"repos/:owner/:repo/check-runs/${id}/annotations\"",
			"spending limit needs to be increased",
			"No Tenet command ran.",
			"classify the incident as external GitHub Actions runner availability",
			"Choose the rerun target that proves the signal you are trying to restore:",
			"gh run list --workflow \"CI Smoke\" --branch main --limit 5",
			"Do not use an old PR rerun as evidence that the `main` release signal recovered.",
			"For a still-open affected PR, rerun that PR head run/check",
			"Re-run the selected failed workflow run:",
			"gh run rerun <run-id> --failed",
			"gh run watch <run-id> --exit-status",
			"## Avoid false recovery signals",
			"A rerun request alone is not recovery evidence.",
			"either passing checks or normal downloadable/actionable repository logs from failed commands",
			"gh workflow run \"CI Smoke\" --ref main",
			"Confirm the fast lane reaches `go test ./...`, smoke script syntax checks, and `make smoke-local`.",
			"## Post-recovery release-health checklist",
			"first `main` run that starts after the account-level runner blocker is fixed",
			"Confirm `Fast smoke` and `Docker smoke scope` have non-empty `steps[]` plus a real runner assignment.",
			"Do not require the Docker lane on unrelated PRs just to prove runner recovery; keep the existing CI cost profile intact.",
			"If any command fails after steps are non-empty, classify it as a normal actionable CI failure",
			"Leave one concrete #850 comment with the run URL, the non-empty `steps[]` evidence",
			"confirm the Docker lane reaches `make smoke-docker` or produces normal actionable logs",
			"## Closeout comment template",
			"Runner availability recovered for CI Smoke run <run-url>.",
			"non-empty runner assignments and steps[]",
			"Outcome: <passed | failed with actionable repo-side failure>",
			"Follow-up: <none | opened issue/PR #... for the new repo-side failure>",
			"Only close the runner-start blocker when the recovered run proves GitHub assigned runners and reached repository commands.",
		},
		"../../scripts/ci-smoke-runner-evidence.sh": {
			"Usage: scripts/ci-smoke-runner-evidence.sh [--summary|--markdown] <run-id>",
			"Use --summary or --markdown to print a concise Markdown block for PR/issue comments.",
			"set -euo pipefail",
			"SUMMARY_MODE=false",
			"print_markdown_summary()",
			"### CI Smoke runner-start evidence",
			"| Scope | Status | Conclusion | runner_id | runner_name | steps |",
			"Docker smoke",
			"Missing failed logs",
			"section \"Runner-start verdict\"",
			"Likely pre-runner blocker",
			"billing/spending-limit annotation",
			"gh api --paginate \"repos/:owner/:repo/commits/${SHA}/check-runs\"",
			"gh api --paginate \"repos/:owner/:repo/check-runs/${id}/annotations\"",
			"gh run view \"$RUN_ID\" --json databaseId,name,status,conclusion,headSha,url",
			"gh api \"repos/:owner/:repo/actions/runs/${RUN_ID}/jobs\"",
			"runner_id,runner_name,steps",
			"gh run view \"$RUN_ID\" --log-failed",
			"did not return downloadable failed logs",
			"gh api --paginate \"repos/:owner/:repo/commits/${SHA}/check-runs\"",
			"select(.name == \"Fast smoke\" or .name == \"Docker smoke scope\" or .name == \"Docker smoke\")",
			"gh api --paginate \"repos/:owner/:repo/check-runs/${id}/annotations\"",
		},
	}

	for path, fragments := range files {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		text := string(body)
		for _, fragment := range fragments {
			if !strings.Contains(text, fragment) {
				t.Fatalf("expected %s to include %q", path, fragment)
			}
		}
	}
}

func TestOverviewDocsDescribeECSTaskScopeReleaseGuard(t *testing.T) {
	files := map[string][]string{
		"../../docs/architecture/tenet-v1-operator-contracts.md": {
			"operators can request a positive bounded sample with `GET /v1/overview/deployments/{deploymentName}/resource-inspection?resourceType=ecsService&resourceName={serviceName}&limit={n}`",
			"optional nested `ec2Instance` host detail (`instanceState`, `instanceType`, `imageId`, `privateIpAddress`, `publicIpAddress`, `subnetId`, `vpcId`, `launchTime`) when the resolved EC2 instance identity and best-effort `DescribeInstances` lookup succeed",
			"sibling `tasksScope` reports `available`, optional `requestedLimit`, `effectiveLimit`, `returnedTaskCount`, `truncationStatus`",
			"Interpret the scope fields literally: `available=true` with `returnedTaskCount=0` is a complete empty task sample",
			"`available=false` / `truncationStatus=not_applicable` means the service inspection succeeded but the optional task bridge could not be sampled",
			"exact task inspection can also include optional nested `ecsTask.ec2Instance` host details",
			"Fargate tasks, missing host identity, missing EC2 records, and EC2 lookup failures keep task inspection `available=true` and omit only the nested host object",
		},
		"../../docs/features/overview-dashboard.md": {
			"`resource-inspection?resourceType=ecsService&resourceName={serviceName}&limit={n}` accepts an optional positive task-sample cap",
			"`ecsService.tasks[]` rows can also add optional container identity evidence (`imageDigest`, `runtimeId`) plus optional nested `ec2Instance` host details using the same EC2 detail shape",
			"local and compose smoke also lock the embedded shell helper/copy that renders `ecsService.tasksScope` as either the normal `Task sample scope` note or the distinct `Task sample unavailable` warning",
			"without needing live AWS task inventory",
			"conservative `truncationStatus` semantics (`not_truncated` = complete bounded result, `truncated` = more matching tasks existed, `unknown` = Tenet reached the cap without provider proof either way, `not_applicable` = task bridge unavailable)",
			"That separates true empty successful samples (`available=true`, `returnedTaskCount=0`) from partial/unavailable task-bridge evidence",
			"exact `ecsTask` inspection keeps existing container identity and placement fields such as `ec2InstanceId` and can add optional nested `ecsTask.ec2Instance` host details",
		},
		"../../docs/architecture/tenet-v1-overview-ui.md": {
			"`resource-inspection?resourceType=ecsService&resourceName={serviceName}&limit={n}` accepts an optional positive task-sample cap",
			"sampled task's resolved EC2 instance ID can be described",
			"sibling `tasksScope` keeps the row cap and sampling state self-describing with `available`, optional `requestedLimit`, `effectiveLimit`, `returnedTaskCount`",
			"`not_applicable` when the optional task bridge is unavailable",
			"Empty successful samples remain `available=true` with `returnedTaskCount=0`",
			"Exact `ecsTask` inspection keeps those container identity and placement fields and can add optional nested `ecsTask.ec2Instance` host facts",
			"optional ECS-on-EC2 host details render in a separate `EC2 host details` block",
			"Fargate/no-AWS/no-host cases stay quiet instead of showing empty placeholders",
		},
	}

	for path, fragments := range files {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		text := string(body)
		for _, fragment := range fragments {
			if !strings.Contains(text, fragment) {
				t.Fatalf("expected %s to include %q", path, fragment)
			}
		}
	}
}

func TestOverviewDocsDescribeAccountSectionNavigation(t *testing.T) {
	files := map[string][]string{
		"../../docs/architecture/tenet-v1-operator-contracts.md": {
			"`Current section`",
			"`On this page`",
			"`No section is currently targeted.`",
			"`Account summary`",
		},
		"../../docs/features/overview-dashboard.md": {
			"`Current section`",
			"`On this page`",
			"`No section is currently targeted.`",
			"`Account summary`",
		},
	}

	for path, fragments := range files {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		text := string(body)
		for _, fragment := range fragments {
			if !strings.Contains(text, fragment) {
				t.Fatalf("expected %s to include %q", path, fragment)
			}
		}
	}
}

func TestOverviewDocsDescribeAccountDetailRefresh(t *testing.T) {
	files := map[string][]string{
		"../../docs/architecture/tenet-v1-operator-contracts.md": {
			"toolbar-level `Refresh account detail`",
			"re-runs only `GET /v1/overview/accounts/{awsAccountId}`",
			"leaves the stale account content visible with explicit loading/error copy",
		},
		"../../docs/features/overview-dashboard.md": {
			"toolbar-level `Refresh account detail`",
			"re-runs only `GET /v1/overview/accounts/{awsAccountId}`",
			"keyboard-accessible toggle wired to the controlled lazy-loaded detail panel with `aria-controls`",
			"refresh-time `401` / `403` responses reuse that same trusted-front-door / access-denied guidance instead of generic fetch-error copy",
		},
	}

	for path, fragments := range files {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		text := string(body)
		for _, fragment := range fragments {
			if !strings.Contains(text, fragment) {
				t.Fatalf("expected %s to include %q", path, fragment)
			}
		}
	}
}

func TestOverviewDocsDescribeAuditFilterReset(t *testing.T) {
	files := map[string][]string{
		"../../docs/architecture/tenet-v1-operator-contracts.md": {
			"`Copy audit read route`",
			"`Audit timeline navigation`",
			"`Jump to newest entry`",
			"`Jump to oldest visible entry`",
			"`Clear action/limit`",
			"`Clear audit controls`",
			"clearable audit toolbar chips",
			"removing stale `#audit-*` section anchors",
		},
		"../../docs/features/overview-dashboard.md": {
			"`Copy audit read route`",
			"`Audit timeline navigation`",
			"`Jump to newest entry`",
			"`Jump to oldest visible entry`",
			"`Clear action/limit`",
			"`Clear audit controls`",
			"clearable toolbar chips",
			"remove stale `#audit-*` section anchors",
		},
		"../../docs/architecture/tenet-v1-overview-ui.md": {
			"`Copy audit read route`",
			"`Audit timeline navigation`",
			"`Jump to newest entry`",
			"`Jump to oldest visible entry`",
			"`Clear action/limit`",
			"`Clear audit controls`",
			"clearable toolbar chips",
			"`Clear audit controls` recovery path",
		},
	}

	for path, fragments := range files {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		text := string(body)
		for _, fragment := range fragments {
			if !strings.Contains(text, fragment) {
				t.Fatalf("expected %s to include %q", path, fragment)
			}
		}
	}
}

func TestOverviewDocsDescribeResourceLazyPanelRefreshAndAuthGuidance(t *testing.T) {
	files := map[string][]string{
		"../../docs/architecture/tenet-v1-operator-contracts.md": {
			"`Refresh live inspection` control",
			"`Refresh recent logs` control",
			"re-run only that panel-local backend route in place",
			"leaves the sibling panel and the rest of the resource page visible",
			"replaces generic fetch-error copy with the same trusted-front-door / access-denied guidance used on other embedded read paths",
			"`Open EC2 console logs` handoff",
			"generic unsupported `autoScalingGroup` resources intentionally omit those handoffs",
		},
		"../../docs/features/overview-dashboard.md": {
			"`Refresh recent logs` and `Refresh live inspection` controls",
			"re-run only that panel's backend route in place",
			"leaves the sibling panel and the rest of the resource page visible",
			"replaces generic fetch-error copy with the same trusted-front-door / access-denied guidance used on other embedded read paths",
			"`Open EC2 console logs` handoff",
			"unsupported generic `autoScalingGroup` resources intentionally omit those handoffs",
		},
		"../../docs/architecture/tenet-v1-overview-ui.md": {
			"`Open EC2 console logs` handoff",
			"unsupported or unavailable resources return `available=false`",
		},
	}

	for path, fragments := range files {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		text := string(body)
		for _, fragment := range fragments {
			if !strings.Contains(text, fragment) {
				t.Fatalf("expected %s to include %q", path, fragment)
			}
		}
	}
}

func TestDockerSmokeDocsDescribeResourceRefreshCoverage(t *testing.T) {
	files := map[string][]string{
		"../../docs/architecture/tenet-v1-operator-contracts.md": {
			"compose-backed shell coverage now also locks the bookmarkable resource page's top-level `Refresh resource detail` and `Resource JSON` shell (`Resource JSON` handoff builder, `GET /v1/overview/deployments/{deploymentName}` route-builder text, plus the preserved-detail loading/error copy) alongside the existing `Refresh recent logs` and `Refresh live inspection` lazy-panel route-builder/loading/error/auth-guidance hooks",
		},
		"../../docs/features/overview-dashboard.md": {
			"`make smoke-docker`, including raw shell-contract checks for the root applications overview refresh hooks",
			"the bookmarkable resource page's top-level JSON/refresh shell (`Resource JSON`, its `/v1/overview/deployments/{deploymentName}` handoff builder, `Refresh resource detail`, the `GET /v1/overview/deployments/{deploymentName}` route-builder text, and the preserved-detail loading/error copy)",
			"the drift workspace `Open resource evidence` handoff shell plus optional `managedDrift[].resourceRef` payload-shape validation when live AWS evidence is present",
			"the bookmarkable resource page's lazy-panel refresh hooks (`Refresh recent logs`, `Refresh live inspection`, their route-builder helpers, and the loading/error/auth-guidance copy)",
		},
	}

	for path, fragments := range files {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		text := string(body)
		for _, fragment := range fragments {
			if !strings.Contains(text, fragment) {
				t.Fatalf("expected %s to include %q", path, fragment)
			}
		}
	}
}

func TestDockerSmokeDocsDescribeEmbeddedReadAuthGuidanceCoverage(t *testing.T) {
	files := map[string][]string{
		"../../docs/architecture/tenet-v1-operator-contracts.md": {
			"The same bookmarkable application, account, traffic-shift resource, ECS resource, drift, audit, and plan routes locked by direct-process smoke",
			"the bookmarkable application shell still exposes the `_self` `Applications overview` return path, the non-click `Application detail` destination chip, and the bookmarkable application-detail status summary shell",
			"raw embedded application/account/resource/drift/audit/plan shell HTML/script fragments instead of runtime DOM text",
			"the shipped operator read-auth guidance hooks for initial `401` / `403` failures remain present",
		},
		"../../docs/architecture/tenet-v1-local-runtime.md": {
			"Bookmarkable application, account, drift, audit, plan workspace, and representative resource URLs, checking raw embedded application/account/resource/drift/audit/plan shell HTML/script fragments instead of runtime DOM text",
			"the bookmarkable application shell still exposes the `_self` `Applications overview` return path plus the non-click `Application detail` destination chip, and the same smoke now explicitly locks the bookmarkable application-detail status summary shell",
			"the shipped operator read-auth guidance hooks for initial `401` / `403` failures remain present",
		},
	}

	for path, fragments := range files {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		text := string(body)
		for _, fragment := range fragments {
			if !strings.Contains(text, fragment) {
				t.Fatalf("expected %s to include %q", path, fragment)
			}
		}
	}
}

func TestSmokeDockerLocksResourceJSONHandoff(t *testing.T) {
	body, err := os.ReadFile("../../scripts/smoke-docker.sh")
	if err != nil {
		t.Fatalf("read smoke-docker.sh: %v", err)
	}
	text := string(body)
	for _, fragment := range []string{
		"\"Resource JSON\"",
		"\"createNavLink(`/v1/overview/deployments/${encodeURIComponent(detail?.name || resourceState?.detail?.name || currentPageState.deployment)}`, 'Resource JSON')\"",
	} {
		if !strings.Contains(text, fragment) {
			t.Fatalf("expected smoke-docker.sh to lock resource detail JSON handoff fragment %q", fragment)
		}
	}
}

func TestSmokeDockerAssertsInvalidResourceInspectionLimit(t *testing.T) {
	body, err := os.ReadFile("../../scripts/smoke-docker.sh")
	if err != nil {
		t.Fatalf("read smoke-docker.sh: %v", err)
	}

	text := string(body)
	for _, fragment := range []string{
		`invalid_resource_inspection_limit_status=$(curl -sS -o "$INVALID_RESOURCE_INSPECTION_LIMIT_BODY"`,
		`resource-inspection?resourceType=candidateAutoScalingGroup&resourceName=payments-api-prod-candidate&limit=0`,
		`assert os.environ["INVALID_RESOURCE_INSPECTION_LIMIT_STATUS"] == "400"`,
		`assert invalid_resource_inspection_limit.strip() == "invalid limit query"`,
	} {
		if !strings.Contains(text, fragment) {
			t.Fatalf("expected smoke-docker.sh to assert invalid ASG resource-inspection limit fragment %q", fragment)
		}
	}
}
