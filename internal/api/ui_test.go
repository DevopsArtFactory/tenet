package api

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

func TestEmbeddedUILazyLoadsDeploymentDetailsOnExpandOnly(t *testing.T) {
	body := string(indexHTML)

	for _, fragment := range []string{
		"const detailStateByDeployment = new Map();",
		"const loadDeploymentDetail = (deploymentName, container) => {",
		"detailStateByDeployment.set(deploymentName, { ...cached, container });",
		"const current = detailStateByDeployment.get(deploymentName);",
		"if (current?.container) {",
		"renderDeploymentDetail(current.container, detail);",
		"renderDetailState(current.container, `Failed to load deployment details: ${error.message}`, 'error');",
		"renderDeploymentDetail(container, cached.detail);",
		"detailPanel.hidden = true;",
		"loadDeploymentDetail(deployment.name, detailPanel).catch(() => {});",
		"fetch(`/v1/overview/deployments/${encodeURIComponent(deploymentName)}`)",
		"if (!response.ok) throw new Error(`detail request failed (${response.status})`);",
		"renderDetailState(container, 'Loading deployment details…');",
		"detailStateByDeployment.set(deploymentName, { status: 'ready', detail });",
		"detailStateByDeployment.set(deploymentName, { status: 'loading', promise, container });",
		"renderDetailState(current.container, `Failed to load deployment details: ${error.message}`, 'error');",
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
		"const generationValue = (value) => Number.isFinite(value) ? String(value) : '—';",
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
		"const createNavLink = (href, text, target = '_blank') => {",
		"link.target = target;",
		"link.rel = target === '_blank' ? 'noreferrer noopener' : '';",
		"link.href = href;",
		"const detailLinks = createElement('div', 'row-links row-links-shell');",
		"createNavLink(`/v1/overview/deployments/${encodeURIComponent(deployment.name)}`, 'Overview JSON')",
		"createNavLink(`/v1/status/deployments/${encodeURIComponent(deployment.name)}`, 'Status JSON')",
		"detailLinks.append(",
		"row.append(toggle, detailLinks, detailPanel);",
		"Overview JSON",
		"Status JSON",
		"const workspaceLinks = createElement('div', 'row-links row-links-shell');",
		"createNavLink(buildPageURL('drift', { deployment: detail.name }), 'Drift view', '_self')",
		"createNavLink(buildPageURL('plan', { deployment: detail.name }), 'Plan/apply', '_self')",
		"createNavLink(buildPageURL('audit'), 'Audit log', '_self')",
		"Drift view",
		"Plan/apply",
		"Audit log",
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
		"const validPages = new Set(['applications', 'application', 'account', 'resource', 'drift', 'plan', 'audit']);",
		"const limit = Number.parseInt(params.get('limit') || '', 10);",
		"page: validPages.has(page) ? page : 'applications',",
		"accountId: params.get('accountId') || '',",
		"deployment: params.get('deployment') || '',",
		"planId: params.get('planId') || '',",
		"applyId: params.get('applyId') || '',",
		"resourceType: params.get('resourceType') || '',",
		"resourceId: params.get('resourceId') || '',",
		"auditLimit: Number.isInteger(limit) && limit > 0 ? limit : 0,",
		"const buildPageURL = (page, extras = {}) => {",
		"if (currentViewState.query) params.set('q', currentViewState.query);",
		"if (currentViewState.syncState && currentViewState.syncState !== 'all') params.set('sync', currentViewState.syncState);",
		"if (currentViewState.account && currentViewState.account !== 'all') params.set('account', currentViewState.account);",
		"const pathname = window.location.pathname || '/';",
		"return query ? `${pathname}?${query}` : pathname;",
		"const renderApplicationsOverview = (applications, focusState = null) => {",
		"Application status overview",
		"Last update",
		"Search applications",
		"Open audit log",
		"Open account",
		"createNavLink(buildPageURL('account', { accountId: deployment.awsAccountId }), 'Open account', '_self')",
		"Open application",
		"createNavLink(buildPageURL('application', { deployment: deployment.name }), 'Open application', '_self')",
		"const renderAccountDetailPage = (account) => {",
		"Account deployment overview",
		"Account JSON",
		"Group JSON",
		"Open application",
		"Back to applications",
		"const groupResourceSections = (detail) => {",
		"const stableResourceIdentity = (resource) => {",
		"const normalizeResource = (resource) => {",
		"const resourceId = stableResourceIdentity({ type, name, description });",
		"Workloads",
		"Traffic routing",
		"Config and access",
		"createNavLink(buildPageURL('resource', { deployment: detail.name, resourceType: resource.type, resourceId: resource.id }), 'Open resource', '_self')",
		"const renderApplicationDetailPage = (detail) => {",
		"const eventsDeploymentName = detail.name || currentPageState.deployment || '';",
		"Application resources",
		"Open drift or plan/apply from here before the final resource page.",
		"const eventsSection = createDetailSection('Controller runtime events');",
		"renderDeploymentEventsSection(eventsSection, eventsDeploymentName).catch(() => {});",
		"backLink.target = '_self';",
		"const renderResourceDetailPage = (detail) => {",
		"Resource detail",
		"Back to application",
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
		"if (actor?.available === false) return 'Unavailable';",
		"return 'Unknown';",
		"const auditActorReasonText = (actor) => {",
		"'Actor identity is unavailable for this record.'",
		"'Actor identity is unknown for this record.'",
		"const fetchApplicationDetail = (deploymentName) => fetch(`/v1/overview/applications/${encodeURIComponent(deploymentName)}`)",
		"if (!response.ok) throw new Error(`application detail request failed (${response.status})`);",
		"const fetchAuditLogs = ({ deploymentName = '', action = '', limit = 0 } = {}) => {",
		"const params = new URLSearchParams();",
		"if (deploymentName) params.set('deployment', deploymentName);",
		"if (action) params.set('action', action);",
		"if (limit > 0) params.set('limit', String(limit));",
		"const route = params.toString() ? `/v1/audit-logs?${params.toString()}` : '/v1/audit-logs';",
		"return fetch(route)",
		"audit log API unavailable (${response.status})",
		"audit log request failed (${response.status})",
		"const createAuditLogsIntro = () => createElement('div', 'detail-section-copy', 'Immutable operator audit log from GET /v1/audit-logs. Newest entries first. Actor identity remains explicit when unavailable or unknown.');",
		"const renderAuditPageChrome = () => {",
		"createSelect('Action', currentPageState.auditAction, auditActionOptions, (event) => {",
		"Audit filters: none",
		"Audit filters: ${activeFilters.join(' · ')}",
		"const renderAuditLogsState = (text, className = '', detailText = '') => {",
		"Loading audit log…",
		"Audit log is currently unavailable.",
		"No audit activity has been persisted yet.",
		"Open plan/apply workspace",
		"createNavLink(buildPageURL('application', { deployment: entry.deploymentName }), 'Open application drill-down', '_self')",
		"createNavLink(buildPageURL('plan', {",
		"const loadAuditLogsPage = () => {",
		"deploymentName: currentPageState.deployment,",
		"action: currentPageState.auditAction,",
		"limit: currentPageState.auditLimit,",
		"const fetchDeploymentEvents = (deploymentName) => fetch(`/v1/overview/deployments/${encodeURIComponent(deploymentName)}/events`)",
		"deployment events unavailable (${response.status})",
		"deployment events request failed (${response.status})",
		"const createDeploymentEventsIntro = () => createElement('div', 'detail-section-copy', 'Controller-derived runtime events from GET /v1/overview/deployments/{deploymentName}/events. Newest entries first. This is not audit/auth history.');",
		"const sortTimelineEvents = (events) => (events || [])",
		"const renderDeploymentEventsState = (section, text, className = '', detailText = '') => {",
		"const renderDeploymentEventsPayload = (section, result) => {",
		"No controller runtime events have been persisted yet.",
		"Controller runtime events are currently unavailable.",
		"Loading controller runtime events…",
		"Failed to load controller runtime events: ${error.message}",
		"Rollout phase · ${event.rolloutPhase}",
		"Rollout step · ${event.currentStep}",
		"Event details · ${changeDetailsText(event.changeDetails)}",
		"const buildPlanPreviewRoute = (planId) => `/v1/plans/${encodeURIComponent(planId)}`;",
		"const fetchPlanPreview = (planId) => fetch(buildPlanPreviewRoute(planId))",
		"const buildApplyRequestRoute = (applyId) => `/v1/applies/${encodeURIComponent(applyId)}`;",
		"const fetchApplyRequest = (applyId) => fetch(buildApplyRequestRoute(applyId))",
		"const cloneJSONValue = (value) => {",
		"const buildPlanSourceFingerprintPayload = (detail) => {",
		"const derivePlanSourceFingerprint = async (detail) => {",
		"const createPlanPreview = (deploymentName, ifMatch = '') => fetch(`/v1/plans/deployments/${encodeURIComponent(deploymentName)}`, {",
		"const createApplyRequest = (planId) => fetch(`/v1/applies/plans/${encodeURIComponent(planId)}`, {",
		"method: 'POST',",
		"headers: { 'Content-Type': 'application/json' },",
		"body: ifMatch ? JSON.stringify({ ifMatch }) : '{}',",
		"const applicationDetail = currentPageState.page === 'application' ? fetchApplicationDetail(currentPageState.deployment) : fetchDeploymentDetail(currentPageState.deployment);",
		"applicationDetail",
		"} else if (currentPageState.page === 'audit') {",
		"loadAuditLogsPage();",
		"if (currentPageState.page === 'application') {",
		"renderApplicationDetailPage(detail);",
		"if (currentPageState.page === 'resource') {",
		"renderResourceDetailPage(detail);",
		"if (currentPageState.page === 'drift') {",
		"renderDeploymentWorkspace(currentPageState.page, detail);",
		"if (currentPageState.page === 'plan') {",
		"renderDeploymentWorkspace('plan', detail, {",
		"syncPlanPageURL(record.id, '');",
		"fetchDeploymentDetail(currentPageState.deployment)",
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
		"const parseResourceIdentity = (resourceId) => {",
		"const [type = '', name = '', ...descriptionParts] = String(resourceId || '').split('::');",
		"description: descriptionParts.join('::'),",
		"const bookmarkedResource = parseResourceIdentity(currentPageState.resourceId);",
		"const exactMatch = resources.find((resource) => {",
		"return resource.id === currentPageState.resourceId && resource.type === currentPageState.resourceType;",
		"const tolerantMatch = resources.find((resource) => {",
		"if (bookmarkedResource.type && resource.type !== bookmarkedResource.type) {",
		"if (bookmarkedResource.name && resource.name !== bookmarkedResource.name) {",
		"return Boolean(bookmarkedResource.type || bookmarkedResource.name);",
		"return exactMatch || tolerantMatch || resources.find((resource) => {",
		"type: bookmarkedResource.type || currentPageState.resourceType || 'resource',",
		"name: bookmarkedResource.name || bookmarkedResource.type || currentPageState.resourceType || 'Resource',",
		"description: bookmarkedResource.description || 'No linked resource metadata is available for this selection.',",
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
		"const renderDeploymentWorkspace = (mode, detail, planState = null, applyState = null) => {",
		"const workspaceTitle = mode === 'drift' ? 'Drift viewer' : 'Plan and apply';",
		"const runtimeSignalsSection = createDetailSection('Latest runtime signals');",
		"['Runtime source', metricValue(detail.runtimeSummary?.source)]",
		"['Runtime timestamp', metricValue(detail.runtimeSummary?.timestamp)]",
		"['Runtime history', Number.isFinite(detail.runtimeSummary?.historyCount) ? String(detail.runtimeSummary.historyCount) : '—']",
		"detail.runtimeSummary?.changes || []",
		"No runtime change signals are currently reported.",
		"const executionSection = createDetailSection('Rollout execution');",
		"rolloutExecutionResources(detail).filter((item) => normalizeResource(item).id === resource.id)",
		"No rollout execution resources are currently reported.",
		"const backLink = createNavLink(buildPageURL('application', { deployment: detail.name || currentPageState.deployment || '' }), 'Back to application');",
		"backLink.target = '_self';",
		"backLink.rel = '';",
		"const emptyTitle = mode === 'drift' ? 'Field-level drift' : 'Plan operations';",
		"const driftSeverityRank = (severity) => {",
		"const deriveDeploymentDriftRows = (detail) => {",
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
		"const driftRows = deriveDeploymentDriftRows(detail);",
		"createInput('Filter drift rows', '', 'Filter by field, value, or condition context', (event) => {",
		"createSelect('Severity', 'all', [",
		"createSelect('Sort by', 'field', [",
		"Field path",
		"Desired",
		"Actual",
		"Severity",
		"No field-level drift is currently derived from the deployment read model.",
		"const requestError = (kind, message, status = 0) => {",
		"const responseErrorMessage = (response, fallback) => {",
		"const buildPlanPreviewRoute = (planId) => `/v1/plans/${encodeURIComponent(planId)}`;",
		"const fetchPlanPreview = (planId) => fetch(buildPlanPreviewRoute(planId))",
		"const buildApplyRequestRoute = (applyId) => `/v1/applies/${encodeURIComponent(applyId)}`;",
		"const fetchApplyRequest = (applyId) => fetch(buildApplyRequestRoute(applyId))",
		"const createPlanPreview = (deploymentName, ifMatch = '') => fetch(`/v1/plans/deployments/${encodeURIComponent(deploymentName)}`, {",
		"const createApplyRequest = (planId) => fetch(`/v1/applies/plans/${encodeURIComponent(planId)}`, {",
		"body: ifMatch ? JSON.stringify({ ifMatch }) : '{}',",
		"const planPreviewCanApply = (planState) => {",
		"const planPreviewStateMessage = (planState, detail) => {",
		"Create Plan will POST /v1/plans/deployments/${deploymentName}, store an immutable server-authored preview, and update this workspace URL with a shareable planId.",
		"Apply Plan waits for a loaded persisted ready preview with a valid planId.",
		"Loading immutable plan preview ${activePlanId} from GET /v1/plans/${activePlanId}.",
		"const approvalSection = createDetailSection(activePlanRecord ? 'Plan preview approval guidance' : 'Derived approval guidance');",
		"Plan preview risk level",
		"Risk factors · ${riskReasons.join('; ')}",
		"Approval required · ${activePlanRecord.approval?.required ? 'Yes' : 'No'}",
		"Create Plan",
		"Apply Plan",
		"Plan preview actions",
		"Apply request actions",
		"const applyRecordStateMessage = (applyState, detail, planState) => {",
		"Live execution is intentionally disabled by default, so failed immutable apply records are expected and useful for operator review.",
		"Apply Plan will POST /v1/applies/plans/${planState.record.id}, persist an immutable apply request, and update this workspace URL with a shareable applyId.",
		"Apply Plan activates only when a loaded persisted ready plan preview with a valid planId is present.",
		"Loading immutable apply request ${activeApplyID} from GET /v1/applies/${activeApplyID}.",
		"Apply request created",
		"Apply request ready",
		"Apply request failed",
		"Apply request unavailable",
		"Apply request fetch error",
		"Stored apply request",
		"createNavLink(buildPageURL('plan', { deployment: detail.name || currentPageState.deployment, planId: activeApplyRecord.planId || activePlanRecord?.id || currentPageState.planId, applyId: activeApplyRecord.id }), 'Open bookmarked apply', '_self')",
		"createNavLink(buildApplyRequestRoute(activeApplyRecord.id), 'Apply JSON')",
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
		"const ifMatch = detail.sourceFingerprint || await derivePlanSourceFingerprint(detail).catch(() => '') || fallbackIfMatch;",
		"let applyState = {",
		"applyId: currentPageState.applyId,",
		"const syncPlanPageURL = (planId, applyId = '') => {",
		"applyId: applyId || '',",
		"createApplyRequest(planID)",
		"syncPlanPageURL(record.planId || planID, record.id);",
		"fetchApplyRequest(currentPageState.applyId)",
		"if (currentPageState.page === 'application') {",
		"renderApplicationDetailPage(detail);",
		"if (currentPageState.page === 'resource') {",
		"renderResourceDetailPage(detail);",
		"if (currentPageState.page === 'drift') {",
		"renderDeploymentWorkspace(currentPageState.page, detail);",
		"if (currentPageState.page === 'plan') {",
		"renderDeploymentWorkspace('plan', detail, {",
		"fetchDeploymentDetail(currentPageState.deployment)",
	} {
		if !strings.Contains(body, fragment) {
			t.Fatalf("expected embedded UI to include %q", fragment)
		}
	}
}

func TestEmbeddedUIIncludesOverviewStatusInFetchErrors(t *testing.T) {
	body := string(indexHTML)

	for _, fragment := range []string{
		"fetch('/v1/overview/applications')",
		"if (!response.ok) throw new Error(`overview request failed (${response.status})`);",
		"const labels = changeLabels(runtimeSummary?.changes);",
		"['Latest runtime', runtimeSummaryText(deployment.runtimeSummary), 'summary']",
		"['Latest changes', changeLabelsText(deployment.runtimeSummary?.changes)]",
		"['Runtime timestamp', runtimeSummaryTimestamp(deployment.runtimeSummary)]",
		"renderEmptyState(`Failed to load overview: ${error.message}`);",
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
		"const generationValue = (value) => Number.isFinite(value) ? String(value) : '—';",
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
		"const createSelect = (labelText, value, options, onChange) => {",
		"const createInput = (labelText, value, placeholder, onInput) => {",
		"const parseViewState = () => {",
		"const normalizeViewState = (applications, state) => {",
		"const syncViewState = (state) => {",
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
		"renderEmptyState('No applications match the current filters.');",
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
		"const resourceLogCacheKey = (deploymentName, resource) => [deploymentName || '', resource.type || '', resource.name || '', resource.id || ''].join('::');",
		"const fetchResourceLogs = (deploymentName, resource) => {",
		"const params = new URLSearchParams({",
		"resourceType: resource.type || '',",
		"resourceName: resource.name || '',",
		"fetch(`/v1/overview/deployments/${encodeURIComponent(deploymentName)}/resource-logs?${params.toString()}`)",
		"if (!response.ok) throw new Error(`resource logs request failed (${response.status})`);",
		"const renderResourceLogsState = (container, text, className = '') => {",
		"container.replaceChildren(heading, stateNode);",
		"const resourceLogsUnavailableReasonText = (reason) => {",
		"case 'unsupported_resource_type':",
		"return 'Recent logs are not supported for this resource type.';",
		"case 'cross_account_not_configured':",
		"return 'Cross-account access is not configured for this deployment.';",
		"case 'aws_unavailable':",
		"return 'AWS access is currently unavailable for recent logs.';",
		"const unavailableReason = resourceLogsUnavailableReasonText(result?.unavailableReason);",
		"section.append(createElement('div', 'detail-condition-meta', unavailableReason));",
		"const renderResourceLogsSection = (section, deploymentName, resource) => {",
		"const logsKey = resourceLogCacheKey(deploymentName, resource);",
		"renderResourceLogsState(section, 'Loading recent logs…');",
		"renderResourceLogsState(section, 'Recent logs are unavailable for this resource.');",
		"renderResourceLogsState(section, 'No recent logs are available for this resource.');",
		"renderResourceLogsState(container, `Failed to load recent logs: ${error.message}`, 'error')",
		"createDetailSection('Recent logs');",
		"Recent logs",
		"Logs source",
		"Log stream",
		"Log timestamp",
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

func TestSmokeLocalScriptAssertsRecentLogsInEmbeddedHTML(t *testing.T) {
	body, err := os.ReadFile("../../scripts/smoke-local.sh")
	if err != nil {
		t.Fatalf("read smoke-local.sh: %v", err)
	}

	text := string(body)
	if !strings.Contains(text, "assert 'Recent logs' in ui, ui") {
		t.Fatalf("expected smoke-local.sh to assert embedded html contains Recent logs")
	}
	if !strings.Contains(text, "assert 'Controller runtime events' in ui, ui") {
		t.Fatalf("expected smoke-local.sh to assert embedded html contains controller runtime events")
	}
	if !strings.Contains(text, "assert 'Audit timeline' in audit_ui, audit_ui") {
		t.Fatalf("expected smoke-local.sh to assert embedded html contains audit timeline copy")
	}
	for _, fragment := range []string{
		"assert_request_id_header() {",
		"assert headers.get(\"request-id\"), headers",
		"assert_request_id_header \"http://$HOST:$PORT/healthz\"",
		"assert_request_id_header \"http://$HOST:$PORT/v1/catalog/summary\"",
		"assert_request_id_header \"http://$HOST:$PORT/v1/overview/deployments/payments-api-prod/resource-logs?resourceType=trafficShiftPlan&resourceName=payments-api-prod-weighted-routing\"",
		"assert_request_id_header \"http://$HOST:$PORT/\"",
		"deployment_events_fixture=$(curl -fsS \"http://$HOST:$PORT/v1/overview/deployments/payments-api-prod/events\")",
		"resource_logs=$(curl -fsS \"http://$HOST:$PORT/v1/overview/deployments/payments-api-prod/resource-logs?resourceType=trafficShiftPlan&resourceName=payments-api-prod-weighted-routing\")",
		"worker_resource_logs=$(curl -fsS \"http://$HOST:$PORT/v1/overview/deployments/worker-prod/resource-logs?resourceType=ecsService&resourceName=worker-prod-service\")",
		"assert deployment_events_fixture['deploymentName'] == 'payments-api-prod', deployment_events_fixture",
		"assert isinstance(deployment_events_fixture['events'], list), deployment_events_fixture",
		"assert len(deployment_events_fixture['events']) == 0, deployment_events_fixture",
		"assert resource_logs['resourceType'] == 'trafficShiftPlan', resource_logs",
		"assert worker_resource_logs['deploymentName'] == 'worker-prod', worker_resource_logs",
		"assert worker_resource_logs['resourceType'] == 'ecsService', worker_resource_logs",
		"assert worker_resource_logs['resourceName'] == 'worker-prod-service', worker_resource_logs",
		"assert isinstance(worker_resource_logs['available'], bool), worker_resource_logs",
		"assert worker_resource_logs.get('entries') is None or isinstance(worker_resource_logs.get('entries'), list), worker_resource_logs",
		"if worker_resource_logs['available']:",
		"assert isinstance(worker_resource_logs.get('source'), str), worker_resource_logs",
		"assert isinstance(worker_resource_logs.get('unavailableReason'), str) and worker_resource_logs['unavailableReason'], worker_resource_logs",
		"assert 'Controller-derived runtime events from GET /v1/overview/deployments/{deploymentName}/events. Newest entries first. This is not audit/auth history.' in ui, ui",
		"assert 'Loading controller runtime events…' in ui, ui",
		"assert 'No controller runtime events have been persisted yet.' in ui, ui",
		"assert 'Controller runtime events are currently unavailable.' in ui, ui",
		"audit_ui=$(curl -fsS \"http://$HOST:$PORT/?page=audit&action=apply.requested&limit=1\")",
		"AUDIT_UI_HTML=\"$audit_ui\"",
		"audit_ui = os.environ['AUDIT_UI_HTML']",
		"assert 'Open audit log' in ui, ui",
		"assert 'Loading audit log…' in audit_ui, audit_ui",
		"assert 'Audit log is currently unavailable.' in audit_ui, audit_ui",
		"assert \"const fetchAuditLogs = ({ deploymentName = '', action = '', limit = 0 } = {}) => {\" in audit_ui, audit_ui",
		"assert \"if (action) params.set('action', action);\" in audit_ui, audit_ui",
		"assert \"createSelect('Action', currentPageState.auditAction, auditActionOptions, (event) => {\" in audit_ui, audit_ui",
		"assert \"fetch(`/v1/overview/deployments/${encodeURIComponent(deploymentName)}/events`)\" in ui, ui",
		"audit_logs_action_filtered=$(curl -fsS \"http://$HOST:$PORT/v1/audit-logs?action=apply.requested\")",
		"audit_logs_filtered=$(curl -fsS \"http://$HOST:$PORT/v1/audit-logs?deployment=payments-api-prod&limit=1\")",
		"audit_logs_empty=$(curl -fsS \"http://$HOST:$PORT/v1/audit-logs?deployment=worker-prod\")",
		"invalid_audit_action_status=$(curl -sS -o \"$INVALID_AUDIT_ACTION_BODY\" -w \"%{http_code}\" \"http://$HOST:$PORT/v1/audit-logs?action=invalid.action\")",
		"AUDIT_LOGS_ACTION_FILTERED_JSON=\"$audit_logs_action_filtered\"",
		"AUDIT_LOGS_FILTERED_JSON=\"$audit_logs_filtered\"",
		"AUDIT_LOGS_EMPTY_JSON=\"$audit_logs_empty\"",
		"audit_logs_action_filtered = json.loads(os.environ['AUDIT_LOGS_ACTION_FILTERED_JSON'])",
		"audit_logs_filtered = json.loads(os.environ['AUDIT_LOGS_FILTERED_JSON'])",
		"audit_logs_empty = json.loads(os.environ['AUDIT_LOGS_EMPTY_JSON'])",
		"assert len(audit_logs_action_filtered['entries']) == 1, audit_logs_action_filtered",
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
	if !strings.Contains(text, "assert isinstance(deployment_runtime_summary.get('changeDetails', []), list), deployment_detail") {
		t.Fatalf("expected smoke-local.sh to verify deployment detail runtimeSummary.changeDetails shape")
	}
	if !strings.Contains(text, "controller_events=$(curl -fsS \"http://$HOST:$PORT/v1/overview/deployments/payments-api-prod/events\"") {
		t.Fatalf("expected smoke-local.sh to hit deployment events after controller starts")
	}
	if !strings.Contains(text, "if deployment_history:") || !strings.Contains(text, "assert isinstance(deployment_history[0].get('changeDetails', []), list), deployment_detail") {
		t.Fatalf("expected smoke-local.sh to verify deployment detail history changeDetails shape")
	}
	if !strings.Contains(text, "assert events_payload['deploymentName'] == 'payments-api-prod', events_payload") || !strings.Contains(text, "assert event.get('source') == 'controller', events_payload") {
		t.Fatalf("expected smoke-local.sh to verify controller-populated deployment events payload")
	}
	if !strings.Contains(text, "assert 'Runtime change details' in ui, ui") {
		t.Fatalf("expected smoke-local.sh to assert embedded html contains Runtime change details")
	}
	for _, fragment := range []string{
		"assert 'Plan preview actions' in ui, ui",
		"assert 'Apply request actions' in ui, ui",
		"assert 'Plan preview approval guidance' in ui, ui",
		"assert 'Create Plan will POST /v1/plans/deployments/${deploymentName}, store an immutable server-authored preview, and update this workspace URL with a shareable planId.' in ui, ui",
		"assert 'const buildPlanPreviewRoute = (planId) => `/v1/plans/${encodeURIComponent(planId)}`;' in ui, ui",
		"assert 'const buildApplyRequestRoute = (applyId) => `/v1/applies/${encodeURIComponent(applyId)}`;' in ui, ui",
		"assert \"createNavLink(buildPageURL('plan', { deployment: detail.name || currentPageState.deployment, planId: activePlanRecord.id, applyId: activeApplyRecord?.id || applyState?.applyId || '' }), 'Open bookmarked workspace', '_self')\" in ui, ui",
		"assert \"createNavLink(buildPlanPreviewRoute(activePlanRecord.id), 'Plan JSON')\" in ui, ui",
		"assert \"createNavLink(buildPageURL('plan', { deployment: detail.name || currentPageState.deployment, planId: activeApplyRecord.planId || activePlanRecord?.id || currentPageState.planId, applyId: activeApplyRecord.id }), 'Open bookmarked apply', '_self')\" in ui, ui",
		"assert \"createNavLink(buildApplyRequestRoute(activeApplyRecord.id), 'Apply JSON')\" in ui, ui",
		"assert 'Server-authored plan preview loaded from GET /v1/plans/${activePlanRecord.id}.' in ui, ui",
		"assert 'Apply Plan will POST /v1/applies/plans/${planState.record.id}, persist an immutable apply request, and update this workspace URL with a shareable applyId.' in ui, ui",
		"assert 'Apply Plan activates only when a loaded persisted ready plan preview with a valid planId is present.' in ui, ui",
		"assert 'Live execution is intentionally disabled by default, so failed immutable apply records are expected and useful for operator review.' in ui, ui",
		"assert \"params.get('planId') || ''\" in ui, ui",
		"assert \"params.get('applyId') || ''\" in ui, ui",
		"plan_ui=$(curl -fsS \"http://$HOST:$PORT/?page=plan&deployment=payments-api-prod&planId=$plan_id\")",
		"apply_ui=$(curl -fsS \"http://$HOST:$PORT/?page=plan&deployment=payments-api-prod&planId=$plan_id&applyId=$apply_id\")",
		"assert 'planId' in plan_ui, plan_ui",
		"assert 'applyId' in apply_ui, apply_ui",
	} {
		if !strings.Contains(text, fragment) {
			t.Fatalf("expected smoke-local.sh to include %q", fragment)
		}
	}
}

func TestOverviewDocsLockECSRollingFallbackContract(t *testing.T) {
	files := map[string][]string{
		"../../README.md": {
			"- `ecsRolling`: `ecsService` (`{deployment}-service`)",
			"worker-prod/resource-logs?resourceType=ecsService&resourceName=worker-prod-service",
		},
		"../../docs/features/overview-dashboard.md": {
			"- `ecsRolling`: `ecsService`",
		},
		"../../docs/architecture/tenet-v1-overview-ui.md": {
			"- `ecsRolling`",
			"- `ecsService` → `{deployment}-service` → `ECS service Tenet updates with native rolling deployment`, optionally extended with rollout-strategy qualifiers such as `while respecting min healthy 90% and warmup 30s`",
			"worker-prod/resource-logs?resourceType=ecsService&resourceName=worker-prod-service",
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

func TestDockerSmokeScriptAndDocsContract(t *testing.T) {
	files := map[string][]string{
		"../../scripts/smoke-docker.sh": {
			"config --services",
			"for service in api controller redis; do",
			"assert_request_id_header() {",
			"headers=$(curl -fsS -D - -o /dev/null \"$url\")",
			"assert request_id, headers",
			"healthz_request_id=$(assert_request_id_header \"$API_BASE_URL/healthz\")",
			"summary_request_id=$(assert_request_id_header \"$API_BASE_URL/v1/catalog/summary\")",
			"worker_resource_logs_request_id=$(assert_request_id_header \"$API_BASE_URL/v1/overview/deployments/worker-prod/resource-logs?resourceType=ecsService&resourceName=worker-prod-service\")",
			"audit_logs_request_id=$(assert_request_id_header \"$API_BASE_URL/v1/audit-logs?action=apply.requested&limit=2\")",
			"ui_request_id=$(assert_request_id_header \"$API_BASE_URL/\")",
			"audit_ui_request_id=$(assert_request_id_header \"$API_BASE_URL/?page=audit&action=apply.requested&limit=2\")",
			"assert_structured_api_request_logs() {",
			"assert entries, \"expected structured api request log lines\"",
			"if entry.get(\"msg\") != \"http_request\":",
			"(os.environ[\"WORKER_RESOURCE_LOGS_REQUEST_ID\"], \"/v1/overview/deployments/worker-prod/resource-logs\")",
			"(os.environ[\"AUDIT_LOGS_REQUEST_ID\"], \"/v1/audit-logs\")",
			"assert entry.get(\"method\") == \"GET\", entry",
			"assert entry.get(\"status\") == 200, entry",
			"assert entry.get(\"result\") == \"success\", entry",
			"assert entry.get(\"duration_ms\") is not None, entry",
			"\"$ROOT/scripts/docker-up.sh\" \"${COMPOSE[@]}\"",
			"curl -fsS \"$API_BASE_URL/healthz\"",
			"curl -fsS \"$API_BASE_URL/v1/catalog/summary\"",
			"curl -fsS \"$API_BASE_URL/v1/overview/applications\"",
			"curl -fsS \"$API_BASE_URL/v1/overview/deployments/payments-api-prod\"",
			"deployment_source_fingerprint=$(DEPLOYMENT_DETAIL_JSON=\"$deployment_detail\" python3 - <<'PY'",
			"plan_preview_request_body=$(DEPLOYMENT_SOURCE_FINGERPRINT=\"$deployment_source_fingerprint\" python3 - <<'PY'",
			"plan_preview=$(curl -fsS -X POST -H 'Content-Type: application/json' -d \"$plan_preview_request_body\" \"$API_BASE_URL/v1/plans/deployments/payments-api-prod\")",
			"plan_preview_get=$(curl -fsS \"$API_BASE_URL/v1/plans/$plan_id\")",
			"assert plan_preview[\"status\"] in {\"ready\", \"failed\"}, plan_preview",
			"if [[ \"$plan_status\" != \"ready\" ]]; then",
			"smoke-docker: expected compose stack to return a ready immutable plan preview for payments-api-prod",
			"apply_record=$(curl -fsS -X POST \"$API_BASE_URL/v1/applies/plans/$plan_id\")",
			"apply_record_get=$(curl -fsS \"$API_BASE_URL/v1/applies/$apply_id\")",
			"plan_apply_ui=$(curl -fsS \"$API_BASE_URL/?page=plan&deployment=payments-api-prod&planId=$plan_id&applyId=$apply_id\")",
			"assert plan_preview[\"status\"] == \"ready\", plan_preview",
			"assert apply_record[\"status\"] == \"failed\", apply_record",
			"assert apply_record[\"error\"] == \"apply execution is disabled\", apply_record",
			"assert apply_record[\"details\"] == [",
			"assert apply_record[\"source\"][\"planStatus\"] == \"ready\", apply_record",
			"assert apply_record_get == apply_record, (apply_record_get, apply_record)",
			"curl -fsS \"$API_BASE_URL/v1/audit-logs?limit=2\"",
			"curl -fsS \"$API_BASE_URL/v1/audit-logs?action=apply.requested&limit=2\"",
			"invalid_audit_action_status=$(curl -sS -o \"$INVALID_AUDIT_ACTION_BODY\" -w \"%{http_code}\" \"$API_BASE_URL/v1/audit-logs?action=invalid.action\")",
			"assert isinstance(audit_logs.get(\"entries\"), list), audit_logs",
			"assert len(audit_logs[\"entries\"]) == 2, audit_logs",
			"assert {entry[\"action\"] for entry in audit_logs[\"entries\"]} == {\"apply.requested\", \"plan.created\"}, audit_logs",
			"assert isinstance(audit_logs_action_filtered.get(\"entries\"), list), audit_logs_action_filtered",
			"assert len(audit_logs_action_filtered[\"entries\"]) == 1, audit_logs_action_filtered",
			"assert audit_logs_action_filtered[\"entries\"][0][\"action\"] == \"apply.requested\", audit_logs_action_filtered",
			"assert os.environ[\"INVALID_AUDIT_ACTION_STATUS\"] == \"400\", os.environ[\"INVALID_AUDIT_ACTION_STATUS\"]",
			"assert \"invalid action query\" in invalid_audit_action_body, invalid_audit_action_body",
			"assert entry[\"actor\"][\"available\"] is False, entry",
			"assert \"do not persist operator identity\" in entry[\"actor\"][\"reason\"], entry",
			"curl -fsS \"$API_BASE_URL/v1/overview/deployments/payments-api-prod/events\"",
			"curl -fsS \"$API_BASE_URL/v1/overview/deployments/worker-prod/resource-logs?resourceType=ecsService&resourceName=worker-prod-service\"",
			"assert deployment_events[\"deploymentName\"] == \"payments-api-prod\", deployment_events",
			"assert isinstance(deployment_events.get(\"events\"), list), deployment_events",
			"assert worker_resource_logs[\"resourceType\"] == \"ecsService\", worker_resource_logs",
			"assert worker_resource_logs[\"resourceName\"] == \"worker-prod-service\", worker_resource_logs",
			"assert isinstance(worker_resource_logs[\"available\"], bool), worker_resource_logs",
			"assert worker_resource_logs.get(\"entries\") is None or isinstance(worker_resource_logs.get(\"entries\"), list), worker_resource_logs",
			"audit_ui=$(curl -fsS \"$API_BASE_URL/?page=audit&action=apply.requested&limit=2\")",
			"Audit timeline",
			"Operator audit log",
			"Audit filters: ${activeFilters.join(' · ')}",
			"Loading audit log…",
			"Audit log is currently unavailable.",
			"Immutable operator audit log from GET /v1/audit-logs. Newest entries first. Actor identity remains explicit when unavailable or unknown.",
			"action ${currentPageState.auditAction}",
			"const fetchAuditLogs = ({ deploymentName = '', action = '', limit = 0 } = {}) => {",
			"if (action) params.set('action', action);",
			"createSelect('Action', currentPageState.auditAction, auditActionOptions, (event) => {",
			"Controller runtime events",
			"Recent logs",
			"Plan preview actions",
			"Apply request actions",
			"Plan preview approval guidance",
			"Open bookmarked workspace",
			"Plan JSON",
			"Open bookmarked apply",
			"Apply JSON",
			"Apply Plan will POST /v1/applies/plans/${planState.record.id}, persist an immutable apply request, and update this workspace URL with a shareable applyId.",
			"Apply Plan activates only when a loaded persisted ready plan preview with a valid planId is present.",
			"Live execution is intentionally disabled by default, so failed immutable apply records are expected and useful for operator review.",
			"params.get('planId') || ''",
			"params.get('applyId') || ''",
			"fetch(`/v1/overview/deployments/${encodeURIComponent(deploymentName)}/resource-logs?${params.toString()}`)",
			"logs api --tail=200",
			"logs controller --tail=50",
			"api logs missing correlated structured request entries",
			"down --remove-orphans",
		},
		"../../Makefile": {
			".PHONY: test fmt docker-build docker-up docker-down smoke-local smoke-docker",
			"smoke-docker:",
			"./scripts/smoke-docker.sh",
		},
		"../../README.md": {
			"Use `make smoke-local` for the fastest fixture-backed API/UI regression check when you only need direct-process validation without Docker or Redis.",
			"Use `make smoke-docker` when you need to validate `compose.yaml`, confirm the `api`/`controller`/`redis` service contract, and prove the containerized local stack plus representative `Request-Id` headers, correlated structured API request logs, controller logs, deployment events, resource-log routes, `GET /v1/audit-logs`, `GET /v1/audit-logs?action=apply.requested&limit=2`, the invalid-action `400 Bad Request` boundary, the bookmarkable `/?page=audit&action={action}&limit={n}` audit view, immutable plan/apply read models, and embedded operator drill-down labels are reachable end to end.",
			"checks representative endpoints plus deployment-events/resource-logs/audit-log JSON shapes, verifies representative `Request-Id` response headers, fetches `GET /v1/audit-logs` plus `GET /v1/audit-logs?action=apply.requested&limit=2`, rejects `action=invalid.action` with `400 Bad Request`,",
			"`GET /v1/audit-logs?deployment={deploymentName}`",
			"`GET /v1/audit-logs?deployment={deploymentName}&limit={n}`",
			"fetches the bookmarkable `/?page=audit&action={action}&limit={n}` audit workspace URL",
			"`Audit filters: action apply.requested · limit 2`",
			"POST /v1/plans/deployments/payments-api-prod",
			"GET /v1/plans/{planId}",
			"POST /v1/applies/plans/{planId}",
			"GET /v1/applies/{applyId}",
			"/?page=plan&deployment=payments-api-prod&planId={planId}&applyId={applyId}",
			"correlated structured `http_request` logs with stable fields such as `request_id`, `method`, `path`, `status`, `duration_ms`, and `result`",
			"`Plan preview actions`, `Apply request actions`, `Controller runtime events`, and `Recent logs`",
			"make smoke-docker",
		},
		"../../docs/architecture/tenet-v1-local-runtime.md": {
			"### Docker stack smoke check",
			"make smoke-docker",
			"asserts the `api`, `controller`, and `redis` services are present in `compose.yaml`",
			"checks representative endpoints plus deployment-events/resource-logs/audit-log JSON shapes, verifies representative `Request-Id` response headers, fetches `GET /v1/audit-logs` plus `GET /v1/audit-logs?action=apply.requested&limit=2`, rejects `action=invalid.action` with `400 Bad Request`,",
			"fetches the bookmarkable `/?page=audit&action={action}&limit={n}` audit workspace URL",
			"`Audit filters: action apply.requested · limit 2`",
			"POST /v1/plans/deployments/payments-api-prod",
			"GET /v1/plans/{planId}",
			"POST /v1/applies/plans/{planId}",
			"GET /v1/applies/{applyId}",
			"/?page=plan&deployment=payments-api-prod&planId={planId}&applyId={applyId}",
			"correlated structured `http_request` logs with stable fields such as `request_id`, `method`, `path`, `status`, `duration_ms`, and `result`",
			"`Plan preview actions`, `Apply request actions`, `Controller runtime events`, and `Recent logs`",
			"`docker compose down --remove-orphans` even on failure",
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
