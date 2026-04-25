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
		"const fetchDeploymentDetail = (deploymentName) => fetchEmbeddedReadRouteJSON(`/v1/overview/deployments/${encodeURIComponent(deploymentName)}`, {",
		"unavailable: 'deployment detail unavailable',",
		"failure: 'detail request failed',",
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
		"const createQuickNavChip = (text, ariaLabel) => {",
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
		"Drift view",
		"Plan/apply",
		"Audit log",
		"const resourceGroups = nonEmptyResourceSections(detail);",
		"if (!resourceGroups.length) {",
		"'No linked resources are currently reported for this application. Use controller runtime events below to inspect the latest rollout signals.'",
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
		"createNavLink(buildPageURL('application', { deployment: deploymentName }), 'Back to application', '_self')",
		"createNavLink(buildPageURL('drift', { deployment: deploymentName }), 'Drift view', '_self')",
		"createNavLink(buildPageURL('plan', { deployment: deploymentName }), 'Plan/apply', '_self')",
		"createNavLink(buildPageURL('audit', { deployment: deploymentName }), 'Audit log', '_self')",
		"createQuickNavChip(resource.name, `Current resource: ${resource.name}`)",
		"const activeSectionID = currentHashTargetID();",
		"createQuickNavChip(currentSectionLink.label, `Current section: ${currentSectionLink.label}`)",
		"createElement('div', 'quick-nav-empty', 'No section is currently targeted.')",
		"sectionLink?.sectionID !== currentSectionLink?.sectionID",
		"createElement('div', 'quick-nav-empty', 'No additional section links are available for this resource.')",
		"const pushResourceSection = (section, sectionID, label) => {",
		"sectionID,",
		"href: buildPageSectionURL('resource', sectionID, {",
		"pushResourceSection(historySection, 'resource-runtime-history', 'Runtime history');",
		"pushResourceSection(executionSection, 'resource-rollout-execution', 'Rollout execution');",
		"pushResourceSection(conditionsSection, 'resource-status-conditions', 'Conditions');",
		"pushResourceSection(inspectionSection, 'resource-live-inspection', 'Live inspection');",
		"pushResourceSection(logsSection, 'resource-recent-logs', 'Recent logs');",
		".filter((candidate) => candidate.id !== resource.id);",
		"createNavLink(buildPageURL('resource', {",
		"resourceType: sibling.type,",
		"resourceId: sibling.id,",
		"}), sibling.name, '_self')",
		"'No sibling linked resources are available for this application.'",
		"const quickNav = createResourceQuickNav(detail, resource, resourceSectionLinks);",
		"page.append(quickNav, shell, historySection, executionSection, conditionsSection, inspectionSection, logsSection);",
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
		"const scrollToCurrentHashTarget = () => {",
		"target.scrollIntoView({ block: 'start', behavior: 'auto' });",
		"window.addEventListener('hashchange', () => {",
		"if (currentPageState.page !== 'application' && currentPageState.page !== 'account' && currentPageState.page !== 'resource' && currentPageState.page !== 'drift' && currentPageState.page !== 'plan' && currentPageState.page !== 'audit') return;",
		"syncActiveResourceQuickNav();",
		"const createDeploymentQuickNav = (deploymentName, currentWorkspace, linkedResources = null, sectionLinks = [], currentSectionEmptyLabel = '') => {",
		"const trackCurrentSection = Boolean(currentSectionEmptyLabel) && sectionLinks.some((sectionLink) => sectionLink?.sectionID);",
		"createElement('div', 'quick-nav-label', 'Current section')",
		"createQuickNavChip(currentSectionLink.label, `Current section: ${currentSectionLink.label}`)",
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
		"createQuickNavChip('Application detail', 'Current destination: Application detail')",
		"createElement('div', 'quick-nav-current', `${deploymentName || 'Deployment'} · Application detail`),",
		"createElement('div', 'quick-nav-label', 'Current section')",
		"const activeSectionID = currentHashTargetID();",
		"createQuickNavChip(currentSectionLink.label, `Current section: ${currentSectionLink.label}`)",
		"createElement('div', 'quick-nav-empty', 'No section is currently targeted.')",
		"sectionLink?.sectionID !== currentSectionLink?.sectionID",
		"createElement('div', 'quick-nav-empty', 'No additional section links are available for this application.')",
		"const applicationSectionIDs = {",
		"'Workloads': 'application-workloads',",
		"'Traffic routing': 'application-traffic-routing',",
		"'Config and access': 'application-config-access',",
		"sectionID,",
		"pushApplicationSectionLink(groupSection.id, group.title);",
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
		"createQuickNavChip('Account detail', 'Current destination: Account detail')",
		"createElement('div', 'quick-nav-current', `${accountLabel} · Account detail`),",
		"createElement('div', 'quick-nav-label', 'Current section')",
		"createElement('div', 'quick-nav-label', 'On this page')",
		"createQuickNavChip(currentSectionLink.label, `Current section: ${currentSectionLink.label}`)",
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
		"pushPlanSection(recordDetailsSection, 'plan-apply-details', 'Apply request details');",
		"pushPlanSection(applyOperationsSection, 'plan-apply-operations', 'Apply operations');",
		"pushPlanSection(planOperationsSection, 'plan-operations', 'Plan operations');",
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
		"const validPages = new Set(['applications', 'application', 'account', 'resource', 'drift', 'plan', 'audit']);",
		"page: validPages.has(page) ? page : 'applications',",
		"accountId: params.get('accountId') || '',",
		"deployment: params.get('deployment') || '',",
		"planId: params.get('planId') || '',",
		"applyId: params.get('applyId') || '',",
		"resourceType: params.get('resourceType') || '',",
		"resourceId: params.get('resourceId') || '',",
		"auditLimit: normalizeAuditLimit(params.get('limit') || ''),",
		"const buildPageURL = (page, extras = {}) => {",
		"if (currentViewState.query) params.set('q', currentViewState.query);",
		"if (currentViewState.syncState && currentViewState.syncState !== 'all') params.set('sync', currentViewState.syncState);",
		"if (currentViewState.account && currentViewState.account !== 'all') params.set('account', currentViewState.account);",
		"if (page === 'audit' && currentPageState.page === 'audit') {",
		"const pathname = window.location.pathname || '/';",
		"return query ? `${pathname}?${query}` : pathname;",
		"const renderApplicationsOverview = (applications, focusState = null) => {",
		"Application status overview",
		"Account detail",
		"Last update",
		"Search applications",
		"Open audit log",
		"Open account",
		"createNavLink(buildPageURL('account', { accountId: deployment.awsAccountId }), 'Open account', '_self')",
		"Open application",
		"createNavLink(buildPageURL('application', { deployment: deployment.name }), 'Open application', '_self')",
		"const renderAccountDetailPage = (account, accountState = null) => {",
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
		"const renderApplicationDetailPage = (detail, applicationState = null) => {",
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
		"Open drift or plan/apply from here before the final resource page.",
		"const eventsSection = createDetailSection('Controller runtime events');",
		"renderDeploymentEventsSection(eventsSection, eventsDeploymentName).catch(() => {});",
		"activeResourceQuickNavSync = typeof quickNav?.syncCurrentSection === 'function' ? quickNav.syncCurrentSection : null;",
		"syncActiveResourceQuickNav();",
		"scrollToCurrentHashTarget();",
		"backLink.target = '_self';",
		"const renderResourceDetailPage = (detail, resourceState = null) => {",
		"const resourceDetailReadRoute = (deploymentName) => `GET /v1/overview/deployments/${deploymentName || '{deploymentName}'}`;",
		"const createResourceDetailActions = (resourceState = null) => [",
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
		"const normalizeAuditLimit = (value) => {",
		"const auditLimitSelectOptions = (limit) => {",
		"const presets = [1, 2, 5, 10, 20, 50];",
		"const fetchEmbeddedReadRouteJSON = (route, labels) => fetch(route)",
		"const applicationsOverviewReadRoute = () => 'GET /v1/overview/applications';",
		"const createApplicationsOverviewActions = (overviewState = null) => [",
		"'Refresh applications overview'",
		"const applicationsOverviewRefreshState = (overviewState = null) => {",
		"'Refreshing applications overview from GET /v1/overview/applications…'",
		"Current applications overview remains visible below.",
		"Failed to refresh applications overview:",
		"const fetchAccountDetail = (accountID) => fetchEmbeddedReadRouteJSON(`/v1/overview/accounts/${encodeURIComponent(accountID)}`, {",
		"unavailable: 'account detail unavailable',",
		"failure: 'account detail request failed',",
		"const accountDetailReadRoute = (accountID) => `GET /v1/overview/accounts/${accountID || '{awsAccountId}'}`;",
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
		"const createApplicationDetailActions = (applicationState = null) => [",
		"'Refresh application detail'",
		"const applicationDetailRefreshState = (applicationState = null) => {",
		"'Refreshing application detail from GET /v1/overview/applications/{deploymentName}…'",
		"Current application detail remains visible below.",
		"Failed to refresh application detail:",
		"const resourceDetailReadRoute = (deploymentName) => `GET /v1/overview/deployments/${deploymentName || '{deploymentName}'}`;",
		"const createResourceDetailActions = (resourceState = null) => [",
		"'Refresh resource detail'",
		"const resourceDetailRefreshState = (resourceState = null) => {",
		"'Refreshing resource detail from GET /v1/overview/deployments/{deploymentName}…'",
		"Current resource detail remains visible below.",
		"Failed to refresh resource detail:",
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
		"const createAuditLogsIntro = () => createElement('div', 'detail-section-copy', 'Immutable operator audit log from GET /v1/audit-logs. Newest entries first. Actor identity remains explicit when unavailable or unknown.');",
		"const syncAuditQueryParams = (params, pageState) => {",
		"const renderAuditPageChrome = () => {",
		"createSelect('Action', currentPageState.auditAction, auditActionOptions, (event) => {",
		"createSelect('Limit', String(currentPageState.auditLimit || ''), auditLimitSelectOptions(currentPageState.auditLimit), (event) => {",
		"Audit filters: none",
		"Audit filters: ${activeFilters.join(' · ')}",
		"const createDeploymentQuickNav = (deploymentName, currentWorkspace, linkedResources = null, sectionLinks = [], currentSectionEmptyLabel = '') => {",
		"const createAuditQuickNav = (sectionLinks = []) => createDeploymentQuickNav(",
		"'No section is currently targeted.',",
		"const deploymentContextSectionID = 'audit-deployment-context';",
		"deploymentCallout.setAttribute('id', deploymentContextSectionID);",
		"const timelineSectionID = 'audit-timeline';",
		"timelineSection.setAttribute('id', timelineSectionID);",
		"buildPageSectionURL('audit', sectionID, {",
		"const renderAuditLogsState = (text, className = '', detailText = '') => {",
		"const quickNav = createAuditQuickNav(auditSectionLinks);",
		"activeResourceQuickNavSync = typeof quickNav?.syncCurrentSection === 'function' ? quickNav.syncCurrentSection : null;",
		"scrollToCurrentHashTarget();",
		"Loading audit log…",
		"Audit log is currently unavailable.",
		"No audit activity has been persisted yet.",
		"Open drift view",
		"Open plan/apply workspace",
		"createNavLink(buildPageURL('application', { deployment: entry.deploymentName }), 'Open application drill-down', '_self')",
		"createNavLink(buildPageURL('drift', { deployment: entry.deploymentName }), 'Open drift view', '_self')",
		"createNavLink(buildPageURL('plan', {",
		"const createAuditLogsActions = (isLoading = false) => [",
		"'Refresh audit log'",
		"const auditLogsRefreshStateText = (auditState) => {",
		"const loadAuditLogsPage = (options = {}) => {",
		"deploymentName: currentPageState.deployment,",
		"action: currentPageState.auditAction,",
		"limit: currentPageState.auditLimit,",
		"const buildDeploymentEventsReadRoute = (deploymentName) => `/v1/overview/deployments/${encodeURIComponent(deploymentName)}/events`;",
		"const deploymentEventsReadRoute = (deploymentName) => `GET ${buildDeploymentEventsReadRoute(deploymentName)}`;",
		"const fetchDeploymentEvents = (deploymentName) => fetchEmbeddedReadRouteJSON(buildDeploymentEventsReadRoute(deploymentName), {",
		"unavailable: 'deployment events unavailable',",
		"failure: 'deployment events request failed',",
		"const createDeploymentEventsIntro = () => createElement('div', 'detail-section-copy', 'Controller-derived runtime events from GET /v1/overview/deployments/{deploymentName}/events. Newest entries first. This is not audit/auth history.');",
		"const createDeploymentEventsActions = (section, deploymentName, isLoading = false) => [",
		"'Refresh runtime events'",
		"() => renderDeploymentEventsSection(section, deploymentName, { force: true }).catch(() => {}),",
		"const sortTimelineEvents = (events) => (events || [])",
		"const renderDeploymentEventsState = (section, deploymentName, text, className = '', detailText = '', isLoading = false) => {",
		"createDetailSectionHeader('Controller runtime events', createDeploymentEventsActions(section, deploymentName, isLoading))",
		"const renderDeploymentEventsPayload = (section, deploymentName, result) => {",
		"No controller runtime events have been persisted yet.",
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
		"const derivePlanSourceFingerprint = async (detail) => {",
		"const createPlanPreview = (deploymentName, ifMatch = '') => fetch(`/v1/plans/deployments/${encodeURIComponent(deploymentName)}`, {",
		"const createApplyRequest = (planId) => fetch(`/v1/applies/plans/${encodeURIComponent(planId)}`, {",
		"method: 'POST',",
		"headers: { 'Content-Type': 'application/json' },",
		"body: ifMatch ? JSON.stringify({ ifMatch }) : '{}',",
		"const applicationDetail = currentPageState.page === 'application' ? fetchApplicationDetail(currentPageState.deployment) : fetchDeploymentDetail(currentPageState.deployment);",
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
		"renderDeploymentWorkspace('plan', detail, {",
		"syncPlanPageURL(record.id, '');",
		"fetchDeploymentDetail(currentPageState.deployment)",
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
		"const operatorReadAuthRequiredMessage = (route) => `Tenet rejected ${route} because authenticated identity is required on this operator read route. Retry this bookmarkable page through the trusted front door so it can authenticate you and forward trusted identity headers.`;",
		"const operatorReadForbiddenMessage = (route) => `Tenet rejected ${route} after recognizing the caller's identity, but this operator is not authorized for that route or deployment.`;",
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
		"const buildEmbeddedPageReadRoute = (pageState = currentPageState) => {",
		"return 'GET /v1/overview/applications';",
		"return `GET /v1/overview/applications/${deploymentName}`;",
		"return `GET /v1/overview/deployments/${deploymentName}`;",
		"return `GET ${buildAuditLogsRoute({",
		"const renderOperatorReadFailurePage = (pageLabel, route, error) => {",
		"createElement('h2', 'account-name', heading)",
		"createElement('div', 'account-meta', operatorReadFailureMessage(route, error))",
		"const renderOperatorReadFailurePanel = (container, title, actions, route, error) => {",
		"createElement('div', 'detail-state error', heading)",
		"createElement('div', 'detail-condition-meta', operatorReadFailureMessage(route, error))",
		"const deploymentEventsReadRoute = (deploymentName) => `GET ${buildDeploymentEventsReadRoute(deploymentName)}`;",
		"createDeploymentEventsActions(section, deploymentName),",
		"deploymentEventsReadRoute(deploymentName),",
		"createDeploymentEventsActions(container, deploymentName),",
		"renderAuditLogsState(operatorReadFailureHeading(error), 'error', operatorReadFailureMessage(buildEmbeddedPageReadRoute(currentPageState), error));",
		"renderOperatorReadFailurePage(embeddedPageTitle(currentPageState.page), buildEmbeddedPageReadRoute(currentPageState), error);",
		"const fetchResourcePanelJSON = (route, failureLabel) => Promise.resolve()",
		".then(() => fetch(route))",
		"if (response.status === 401 || response.status === 403) {",
		"throw requestError('fetch-error', message, response.status);",
		"const buildResourceLogsRoute = (deploymentName, resource) => {",
		"const resourceLogsReadRoute = (deploymentName, resource) => `GET ${buildResourceLogsRoute(deploymentName, resource)}`;",
		"const buildResourceInspectionRoute = (deploymentName, resource) => {",
		"const resourceInspectionReadRoute = (deploymentName, resource) => `GET ${buildResourceInspectionRoute(deploymentName, resource)}`;",
		"if (renderOperatorReadFailurePanel(",
		"'Recent logs',",
		"resourceLogsReadRoute(deploymentName, resource),",
		"'Live inspection',",
		"resourceInspectionReadRoute(deploymentName, resource),",
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
		"return exactMatch || aliasMatch || tolerantMatch || resources.find((resource) => {",
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
		"const driftWorkspaceRefreshStateText = (workspaceState) => {",
		"return 'Refreshing drift workspace from GET /v1/overview/deployments/{deploymentName}…';",
		"return `Failed to refresh drift workspace: ${workspaceState.error?.message || 'unknown error'}`;",
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
		"const createDeploymentQuickNav = (deploymentName, currentWorkspace, linkedResources = null, sectionLinks = [], currentSectionEmptyLabel = '') => {",
		"const createWorkspaceQuickNav = (detail, mode, sectionLinks = null) => {",
		"const currentWorkspace = mode === 'drift' ? 'Drift view' : 'Plan/apply';",
		"const linkedResources = mode === 'drift' ? groupResourceSections(detail).flatMap((group) => group.resources || []) : null;",
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
		"createQuickNavChip(currentWorkspace, `Current destination: ${currentWorkspace}`)",
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
		"'No linked resources are available in this drift workspace yet.'",
		"const quickNav = createWorkspaceQuickNav(detail, mode, planSectionLinks);",
		"activeResourceQuickNavSync = typeof quickNav?.syncCurrentSection === 'function' ? quickNav.syncCurrentSection : null;",
		"header.append(head, quickNav, ...sections);",
		"const emptyTitle = mode === 'drift' ? 'Field-level drift' : 'Plan operations';",
		"const driftSeverityRank = (severity) => {",
		"const driftSeverityCounts = (rows) => rows.reduce((counts, row) => {",
		"const driftRowLabel = (count) => `${count} drift row${count === 1 ? '' : 's'}`;",
		"const driftSummaryCopy = (totalRows, visibleRows) => {",
		"const driftValueText = (value) => value == null || value === '' ? '—' : String(value);",
		"const normalizeManagedDriftRow = (row) => {",
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
		"const driftRefreshState = driftWorkspaceRefreshStateText(workspaceState);",
		"workspaceState?.status === 'error' ? 'detail-state error' : 'detail-state'",
		"const defaultDriftFilters = { query: '', severity: 'all', sort: 'field' };",
		"const hasActiveDriftFilters = () => (",
		"let driftQuery = workspaceState?.filters?.query || defaultDriftFilters.query;",
		"let driftSeverity = workspaceState?.filters?.severity || defaultDriftFilters.severity;",
		"let driftSort = workspaceState?.filters?.sort || defaultDriftFilters.sort;",
		"const syncDriftFilters = () => {",
		"workspaceState.filters = {",
		"'Clear drift filters',",
		"'Drift filters: defaults'",
		"createInput('Filter drift rows', driftQuery, 'Filter by field, value, or condition context', (event) => {",
		"syncDriftFilters();",
		"createMetric('Drift summary', driftSummaryCopy(driftRows.length, filteredRows.length), 'summary')",
		"createMetric('Total rows', String(driftRows.length))",
		"createMetric('Visible rows', String(filteredRows.length))",
		"createMetric('High', String(filteredCounts.high))",
		"createMetric('Medium', String(filteredCounts.medium))",
		"createMetric('Low', String(filteredCounts.low))",
		"createSelect('Severity', driftSeverity, [",
		"createSelect('Sort by', driftSort, [",
		"Field path",
		"Desired",
		"Actual",
		"Severity",
		"No field-level drift rows are currently reported.",
		"Filters currently hide all ${driftRowLabel(totalRows)}.",
		"Showing ${driftRowLabel(visibleRows)} out of ${driftRowLabel(totalRows)} after filters.",
		"No field-level drift rows match the current filters. Adjust the query or severity filters to inspect reported drift.",
		"No controller-authored field-level drift was reported for this deployment.",
		"No derived fallback field-level drift was detected from the deployment read model.",
		"Active drift source",
		"Current source: controller-authored managedDrift rows.",
		"Current source: derived fallback read-model heuristics.",
		"const requestError = (kind, message, status = 0) => {",
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
		"const mapRecordOperations = (operations, fallbackPrefix) => (operations || []).map((operation, index) => ({",
		"const previewOperations = (record) => mapRecordOperations(record?.operations || [], 'Plan step');",
		"const applyOperations = (record) => (record?.operations || []).map((operation, index) => {",
		"status: applyOperationStatusClassName(operation?.status),",
		"statusLabel: applyOperationStatusLabel(operation?.status),",
		"lifecycleStatus: normalizedStatus,",
		"const createOperationsSection = ({ title, intro, operations, emptyMessage }) => {",
		"const planPreviewCanApply = (planState) => {",
		"const operatorWriteAuthRequiredState = (state) => state?.request === 'create' && state?.status === 'fetch-error' && Number(state?.error?.status) === 401;",
		"const operatorWriteForbiddenState = (state) => state?.request === 'create' && state?.status === 'fetch-error' && Number(state?.error?.status) === 403;",
		"const operatorWriteRoleRequiredState = (state) => operatorWriteForbiddenState(state) && String(state?.error?.message || '').includes('authenticated operator role is required for this operator write route');",
		"const operatorWriteAuthRequiredMessage = (route, action) => `Tenet rejected ${route} because authenticated identity is required on operator write routes. When auth enforcement is enabled, retry ${action} through the trusted front door so it can authenticate you and forward trusted identity headers.`;",
		"const operatorWriteRoleRequiredMessage = (route, action) => `Tenet rejected ${route} after recognizing the caller's identity because this operator write route requires a trusted forwarded operator/admin role. Retry ${action} through the trusted front door so it can forward X-Tenet-Authenticated-Roles for an operator-capable session.`;",
		"const operatorWriteForbiddenMessage = (route, action) => `Tenet rejected ${route} after recognizing the caller's identity, but this operator is not authorized to ${action.toLowerCase()} for that deployment through this route.`;",
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
		"Plan preview actions",
		"Apply request actions",
		"const applyRecordStateMessage = (applyState, detail, planState) => {",
		"Live execution is intentionally disabled by default, so failed immutable apply records are expected and useful for operator review.",
		"Future immutable lifecycle states render here as queued, running, succeeded, cancelled, or failed.",
		"Apply Plan activates only when a loaded persisted ready plan preview with a valid planId is present.",
		"Loading immutable apply request ${activeApplyID} from GET /v1/applies/${activeApplyID}.",
		"operatorWriteAuthRequiredMessage(route, 'Apply Plan')",
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
		"const applyLifecycleProgressText = (record) => {",
		"Current step ${currentStep} of ${totalSteps}",
		"Completed ${completedSteps} of ${totalSteps}",
		"Lifecycle metadata is not available yet",
		"Apply request queued",
		"Apply request running",
		"Apply request succeeded",
		"Apply request cancelled",
		"Apply request failed",
		"Apply request unavailable",
		"Apply request fetch error",
		"Stored apply request",
		"Apply lifecycle summary",
		"Apply lifecycle metadata",
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
		"const ifMatch = detail.sourceFingerprint || await derivePlanSourceFingerprint(detail).catch(() => '') || fallbackIfMatch;",
		"createNavLink(buildPageURL('audit', { deployment: detail.name || currentPageState.deployment || '' }), 'Audit log', '_self')",
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
		"const fetchApplicationsOverview = () => fetchEmbeddedReadRouteJSON('/v1/overview/applications', {",
		"unavailable: 'applications overview unavailable',",
		"failure: 'applications overview request failed',",
		"const fetchAccountDetail = (accountID) => fetchEmbeddedReadRouteJSON(`/v1/overview/accounts/${encodeURIComponent(accountID)}`, {",
		"unavailable: 'account detail unavailable',",
		"failure: 'account detail request failed',",
		"const accountDetailReadRoute = (accountID) => `GET /v1/overview/accounts/${accountID || '{awsAccountId}'}`;",
		"const createAccountDetailActions = (accountState = null) => [",
		"const accountDetailRefreshState = (accountState = null) => {",
		"const labels = changeLabels(runtimeSummary?.changes);",
		"['Latest runtime', runtimeSummaryText(deployment.runtimeSummary), 'summary']",
		"['Latest changes', changeLabelsText(deployment.runtimeSummary?.changes)]",
		"['Runtime timestamp', runtimeSummaryTimestamp(deployment.runtimeSummary)]",
		"fetchApplicationsOverview()",
		"fetchAccountDetail(currentPageState.accountId)",
		"renderOperatorReadFailurePage(embeddedPageTitle(currentPageState.page), buildEmbeddedPageReadRoute(currentPageState), error);",
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
		"const buildResourceLogsRoute = (deploymentName, resource) => {",
		"resourceType: resource?.type || '',",
		"resourceName: resource?.name || '',",
		"return `/v1/overview/deployments/${encodeURIComponent(deploymentName)}/resource-logs?${params.toString()}`;",
		"const resourceLogsReadRoute = (deploymentName, resource) => `GET ${buildResourceLogsRoute(deploymentName, resource)}`;",
		"return fetchResourcePanelJSON(buildResourceLogsRoute(deploymentName, resource), 'resource logs request failed');",
		"const fetchResourcePanelJSON = (route, failureLabel) => Promise.resolve()",
		".then(() => fetch(route))",
		"const createDetailSectionHeader = (title, actions = []) => {",
		"const createRefreshButton = (label, disabled, onRefresh) => {",
		"const renderResourceLogsState = (container, text, className = '', beforeNodes = [], afterNodes = [], actions = []) => {",
		"container.replaceChildren(heading, ...beforeNodes, stateNode, ...afterNodes);",
		"const resourceLogsUnavailableReasonText = (reason) => {",
		"case 'unsupported_resource_type':",
		"return 'Recent logs are not supported for this resource type.';",
		"case 'cross_account_not_configured':",
		"return 'Cross-account access is not configured for this deployment.';",
		"case 'aws_unavailable':",
		"return 'AWS access is currently unavailable for recent logs.';",
		"const lookupContextText = (value) => value == null || value === '' ? '—' : String(value);",
		"const lookupContextBoolText = (value) => typeof value === 'boolean' ? boolLabel(value) : '—';",
		"const resourceLogsLookupContextFields = (lookupContext) => [",
		"['Lookup region', lookupContextText(lookupContext?.region)]",
		"['Control account ID', lookupContextText(lookupContext?.controlAccountId)]",
		"['Target account ID', lookupContextText(lookupContext?.targetAccountId)]",
		"['Cross-account lookup', lookupContextBoolText(lookupContext?.crossAccount)]",
		"['Observer role configured', lookupContextBoolText(lookupContext?.observerRoleConfigured)]",
		"const resourceLogsBackingResourceFields = (result) => {",
		"if (result?.backingResourceType) {",
		"fields.push(['Backing resource type', lookupContextText(result.backingResourceType)]);",
		"if (result?.backingResourceName) {",
		"fields.push(['Backing resource name', lookupContextText(result.backingResourceName)]);",
		"const backingResourceFields = resourceLogsBackingResourceFields(result);",
		"const createResourceLogsLookupContextGrid = (lookupContext, extraFields = []) => {",
		"const createResourceLogsActions = (section, deploymentName, resource, isLoading = false) => [",
		"'Refresh recent logs'",
		"renderResourceLogsSection(section, deploymentName, resource, { force: true }).catch(() => {})",
		"[...extraFields, ...resourceLogsLookupContextFields(lookupContext)].forEach(([label, value]) => {",
		"const unavailableReason = resourceLogsUnavailableReasonText(result?.unavailableReason);",
		"const metadata = createResourceLogsLookupContextGrid(result?.lookupContext, backingResourceFields);",
		"const details = unavailableReason",
		"renderResourceLogsState(section, 'Recent logs are unavailable for this resource.', '', [metadata], details, actions);",
		"renderResourceLogsState(section, 'No recent logs are available for this resource.', '', [metadata], [], actions);",
		"...backingResourceFields,",
		"const renderResourceLogsSection = (section, deploymentName, resource, options = {}) => {",
		"const force = Boolean(options?.force);",
		"const logsKey = resourceLogCacheKey(deploymentName, resource);",
		"if (!force && cached?.status === 'ready') {",
		"renderResourceLogsState(section, 'Loading recent logs…', '', [], [], loadingActions);",
		"resourceLogsReadRoute(deploymentName, resource),",
		"renderResourceLogsState(",
		"`Failed to load recent logs: ${error.message}`",
		"createResourceLogsActions(container, deploymentName, resource),",
		"createDetailSection('Recent logs');",
		"Recent logs",
		"Refresh recent logs",
		"Logs source",
		"Backing resource type",
		"Backing resource name",
		"Lookup region",
		"Control account ID",
		"Target account ID",
		"Cross-account lookup",
		"Observer role configured",
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

func TestEmbeddedUIResourceDetailLiveInspectionContract(t *testing.T) {
	body := string(indexHTML)

	for _, fragment := range []string{
		"const resourceInspectionStateByKey = new Map();",
		"const resourceInspectionCacheKey = (deploymentName, resource) => resourceLogCacheKey(deploymentName, resource);",
		"const buildResourceInspectionRoute = (deploymentName, resource) => {",
		"return `/v1/overview/deployments/${encodeURIComponent(deploymentName)}/resource-inspection?${params.toString()}`;",
		"const resourceInspectionReadRoute = (deploymentName, resource) => `GET ${buildResourceInspectionRoute(deploymentName, resource)}`;",
		"const fetchResourceInspection = (deploymentName, resource) => {",
		"return fetchResourcePanelJSON(buildResourceInspectionRoute(deploymentName, resource), 'resource inspection request failed');",
		"const createResourceInspectionActions = (section, detail, resource, isLoading = false) => {",
		"'Refresh live inspection'",
		"renderResourceInspectionSection(section, detail, resource, { force: true }).catch(() => {})",
		"const renderResourceInspectionState = (container, text, className = '', beforeNodes = [], afterNodes = [], actions = []) => {",
		"const resourceInspectionUnavailableReasonText = (reason) => {",
		"return 'Live inspection is not supported for this resource type.';",
		"return 'Cross-account access is not configured for live inspection for this deployment.';",
		"return 'No running tasks were found for this ECS service.';",
		"return 'No instances were found in this Auto Scaling Group.';",
		"return 'AWS access is currently unavailable for live inspection.';",
		"const resourceLogsLookupContextFields = (lookupContext) => [",
		"const lookupContextFields = (lookupContext) => resourceLogsLookupContextFields(lookupContext);",
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
		"Task definition",
		"Cluster ARN",
		"Service ARN",
		"Service deployments",
		"Deployment status",
		"Rollout state",
		"Desired/running/pending",
		"Rollout reason",
		"Recent ECS service events",
		"Event timestamp",
		"const autoScalingObservedLaunchTemplate = (group) => {",
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
		"Instances",
		"Protected from scale-in",
		"Weighted capacity",
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
		"const renderTrafficRoutingInspection = (section, deploymentName, resource, result) => {",
		"Read-only traffic-routing summary from GET /v1/overview/deployments/{deploymentName}/resource-inspection.",
		"Resource kind",
		"Display name",
		"Inspection mode",
		"Rollout summary",
		"const renderResourceInspectionPayload = (section, detail, resource, result) => {",
		"renderResourceInspectionState(section, 'Live inspection is unavailable for this resource.', '', [metadata], details, actions);",
		"renderResourceInspectionState(section, 'No live inspection details are available for this resource yet.', '', [metadata], [], actions);",
		"const renderResourceInspectionSection = (section, detail, resource, options = {}) => {",
		"const force = Boolean(options?.force);",
		"const inspectionKey = resourceInspectionCacheKey(deploymentName, resource);",
		"if (!force && cached?.status === 'ready') {",
		"renderResourceInspectionState(section, 'Loading live inspection…', '', [], [], loadingActions);",
		"resourceInspectionReadRoute(deploymentName, resource),",
		"renderResourceInspectionState(",
		"`Failed to load live inspection: ${error.message}`",
		"createResourceInspectionActions(container, detail, resource),",
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
	} {
		if strings.Contains(body, unsafe) {
			t.Fatalf("expected embedded UI to avoid unsafe live-inspection rendering %q", unsafe)
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
	if !strings.Contains(text, "assert 'Refresh recent logs' in ui, ui") {
		t.Fatalf("expected smoke-local.sh to assert embedded html contains Refresh recent logs")
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
	if !strings.Contains(text, "assert 'Lookup region' in ui, ui") {
		t.Fatalf("expected smoke-local.sh to assert embedded html contains Lookup region")
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
	if !strings.Contains(text, "assert 'Refresh drift workspace' in ui, ui") {
		t.Fatalf("expected smoke-local.sh to assert embedded html contains Refresh drift workspace")
	}
	if !strings.Contains(text, "assert 'Refresh applications overview' in ui, ui") {
		t.Fatalf("expected smoke-local.sh to assert embedded html contains Refresh applications overview")
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
	if !strings.Contains(text, "assert 'Refresh application detail' in application_ui, application_ui") {
		t.Fatalf("expected smoke-local.sh to assert embedded html contains Refresh application detail")
	}
	if !strings.Contains(text, "assert 'Refresh resource detail' in body, body") {
		t.Fatalf("expected smoke-local.sh to assert embedded html contains Refresh resource detail")
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
	if !strings.Contains(text, "assert 'Linked resources' in ui, ui") {
		t.Fatalf("expected smoke-local.sh to assert embedded html contains drift linked-resources copy")
	}
	if !strings.Contains(text, "assert 'No linked resources are available in this drift workspace yet.' in ui, ui") {
		t.Fatalf("expected smoke-local.sh to assert embedded html contains drift linked-resources empty-state copy")
	}
	if !strings.Contains(text, "assert 'Refreshing drift workspace from GET /v1/overview/deployments/{deploymentName}…' in ui, ui") {
		t.Fatalf("expected smoke-local.sh to assert embedded html contains drift refresh loading copy")
	}
	if !strings.Contains(text, "assert 'Failed to refresh drift workspace:' in ui, ui") {
		t.Fatalf("expected smoke-local.sh to assert embedded html contains drift refresh error copy")
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
	if !strings.Contains(text, "bootstrap_resource_logs=$(curl -fsS \"http://$HOST:$PORT/v1/overview/deployments/payments-api-prod/resource-logs?resourceType=userDataBootstrapSource&resourceName=git%3Auserdata%2Fpayments-api-prod.sh\")") {
		t.Fatalf("expected smoke-local.sh to fetch bootstrap-source recent logs payload")
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
	if !strings.Contains(text, "assert \"createQuickNavChip('Account detail', 'Current destination: Account detail')\" in account_ui, account_ui") {
		t.Fatalf("expected smoke-local.sh to assert account page current-destination chip contract")
	}
	if !strings.Contains(text, "assert \"createQuickNavChip(currentSectionLink.label, `Current section: ${currentSectionLink.label}`)\" in account_ui, account_ui") {
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
	if !strings.Contains(text, "for label, body in ((\"resource_ui\", resource_ui), (\"ecs_resource_ui\", ecs_resource_ui), (\"bootstrap_resource_ui\", bootstrap_resource_ui)):") {
		t.Fatalf("expected smoke-local.sh to assert all bookmarkable resource pages")
	}
	if !strings.Contains(text, "\"return 'Account detail';\",") || !strings.Contains(text, "\"return `GET /v1/overview/accounts/${accountID}`;\",") {
		t.Fatalf("expected smoke-local.sh to assert account-page operator read auth labels")
	}
	if !strings.Contains(text, "assert \"return 'Applications overview';\" in ui, ui") || !strings.Contains(text, "assert \"return 'GET /v1/overview/applications';\" in ui, ui") {
		t.Fatalf("expected smoke-local.sh to assert landing-page operator read auth labels")
	}
	if !strings.Contains(text, "assert \"createNavLink(buildPageURL('resource', { deployment: detail.name, resourceType: resource.type, resourceId: resource.id }), 'Open resource', '_self')\" in application_ui, application_ui") {
		t.Fatalf("expected smoke-local.sh to assert application page resource link contract")
	}
	if !strings.Contains(text, "assert \"const nonEmptyResourceSections = (detail) => groupResourceSections(detail).filter((group) => Array.isArray(group?.resources) && group.resources.length > 0);\" in application_ui, application_ui") {
		t.Fatalf("expected smoke-local.sh to assert application page filters empty linked-resource groups")
	}
	if !strings.Contains(text, "assert 'No linked resources are currently reported for this application. Use controller runtime events below to inspect the latest rollout signals.' in application_ui, application_ui") {
		t.Fatalf("expected smoke-local.sh to assert application page empty-state copy")
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
	if !strings.Contains(text, "assert \"createQuickNavChip('Application detail', 'Current destination: Application detail')\" in application_ui, application_ui") {
		t.Fatalf("expected smoke-local.sh to assert application page current-destination chip contract")
	}
	if !strings.Contains(text, "assert \"eventsSection.id = 'application-runtime-events';\" in application_ui, application_ui") {
		t.Fatalf("expected smoke-local.sh to assert application page runtime-events anchor contract")
	}
	if !strings.Contains(text, "for body in (resource_ui, ecs_resource_ui, bootstrap_resource_ui):") {
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
	if !strings.Contains(text, "assert \"createQuickNavChip(currentSectionLink.label, `Current section: ${currentSectionLink.label}`)\" in ui, ui") {
		t.Fatalf("expected smoke-local.sh to assert resource page current-section chip contract")
	}
	if !strings.Contains(text, "assert 'Audit timeline' in audit_ui, audit_ui") {
		t.Fatalf("expected smoke-local.sh to assert embedded html contains audit timeline copy")
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
	for _, fragment := range []string{
		"json_string_field_from_path() {",
		"response_path=$(write_payload wait-for-apply-status.json \"$response\")",
		"current_status=$(json_string_field_from_path \"$response_path\" status)",
		"assert_request_id_header() {",
		"assert headers.get(\"request-id\"), headers",
		"assert_request_id_header \"http://$HOST:$PORT/healthz\"",
		"assert_request_id_header \"http://$HOST:$PORT/v1/catalog/summary\"",
		"assert_request_id_header \"http://$HOST:$PORT/v1/overview/deployments/payments-api-staging/resource-logs?resourceType=codeDeployDeploymentGroup&resourceName=payments-api-staging-codedeploy-group\"",
		"assert_request_id_header \"http://$HOST:$PORT/\"",
		"TENET_GITHUB_WEBHOOK_SECRET=\"$WEBHOOK_SECRET\"",
		"webhook_push=$(curl -fsS -X POST \\",
		"http://$HOST:$PORT/api/v1/github/webhook",
		"deployment_events_fixture=$(curl -fsS \"http://$HOST:$PORT/v1/overview/deployments/payments-api-prod/events\")",
		"resource_logs=$(curl -fsS \"http://$HOST:$PORT/v1/overview/deployments/payments-api-staging/resource-logs?resourceType=codeDeployDeploymentGroup&resourceName=payments-api-staging-codedeploy-group\")",
		"worker_resource_logs=$(curl -fsS \"http://$HOST:$PORT/v1/overview/deployments/worker-prod/resource-logs?resourceType=ecsService&resourceName=worker-prod-service\")",
		"assert deployment_events_fixture['deploymentName'] == 'payments-api-prod', deployment_events_fixture",
		"assert isinstance(deployment_events_fixture['events'], list), deployment_events_fixture",
		"assert len(deployment_events_fixture['events']) == 0, deployment_events_fixture",
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
		"assert isinstance(worker_resource_inspection['ecsService'].get('serviceName'), str) and worker_resource_inspection['ecsService']['serviceName'], worker_resource_inspection",
		"minimum_healthy_percent = worker_resource_inspection['ecsService'].get('minimumHealthyPercent')",
		"maximum_percent = worker_resource_inspection['ecsService'].get('maximumPercent')",
		"health_check_grace_period_seconds = worker_resource_inspection['ecsService'].get('healthCheckGracePeriodSeconds')",
		"assert isinstance(worker_resource_inspection['ecsService'].get('deployments', []), list), worker_resource_inspection",
		"assert isinstance(worker_resource_inspection['ecsService'].get('events', []), list), worker_resource_inspection",
		"assert isinstance(candidate_resource_inspection['autoScalingGroup'].get('autoScalingGroupName'), str) and candidate_resource_inspection['autoScalingGroup']['autoScalingGroupName'], candidate_resource_inspection",
		"assert isinstance(candidate_resource_inspection['autoScalingGroup'].get('instances', []), list), candidate_resource_inspection",
		"assert 'Controller-derived runtime events from GET /v1/overview/deployments/{deploymentName}/events. Newest entries first. This is not audit/auth history.' in ui, ui",
		"assert 'Refresh runtime events' in ui, ui",
		"assert 'Loading controller runtime events…' in ui, ui",
		"assert 'No controller runtime events have been persisted yet.' in ui, ui",
		"assert 'Controller runtime events are currently unavailable.' in ui, ui",
		"assert \"const deploymentEventsReadRoute = (deploymentName) => `GET ${buildDeploymentEventsReadRoute(deploymentName)}`;\" in ui, ui",
		"assert \"createDeploymentEventsActions(section, deploymentName),\" in ui, ui",
		"assert \"createDeploymentEventsActions(container, deploymentName),\" in ui, ui",
		"assert \"deploymentEventsReadRoute(deploymentName),\" in ui, ui",
		"assert 'Loading live inspection…' in ui, ui",
		"assert 'Live inspection is unavailable for this resource.' in ui, ui",
		"assert 'No live inspection details are available for this resource yet.' in ui, ui",
		"assert 'Service deployments' in ui, ui",
		"assert 'Auto Scaling Group name' in ui, ui",
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
		"assert 'No audit activity has been persisted yet for deployment ${currentPageState.deployment}.' in audit_ui, audit_ui",
		"assert 'Loading audit log…' in audit_ui, audit_ui",
		"assert 'Audit log is currently unavailable.' in audit_ui, audit_ui",
		"assert 'Refresh audit log' in audit_ui, audit_ui",
		"assert 'Refreshing audit log from GET /v1/audit-logs…' in audit_ui, audit_ui",
		"assert 'Failed to refresh audit log:' in audit_ui, audit_ui",
		"assert \"const createAuditLogsActions = (isLoading = false) => [\" in audit_ui, audit_ui",
		"assert \"const auditLogsRefreshStateText = (auditState) => {\" in audit_ui, audit_ui",
		"assert \"const fetchAuditLogs = ({ deploymentName = '', action = '', limit = 0 } = {}) => {\" in audit_ui, audit_ui",
		"assert \"if (action) params.set('action', action);\" in audit_ui, audit_ui",
		"assert \"const createAuditQuickNav = (sectionLinks = []) => createDeploymentQuickNav(\" in audit_ui, audit_ui",
		"assert \"createSelect('Action', currentPageState.auditAction, auditActionOptions, (event) => {\" in audit_ui, audit_ui",
		"assert \"const normalizeAuditLimit = (value) => {\" in audit_ui, audit_ui",
		"assert \"const auditLimitSelectOptions = (limit) => {\" in audit_ui, audit_ui",
		"assert \"const syncAuditQueryParams = (params, pageState) => {\" in audit_ui, audit_ui",
		"assert \"createSelect('Limit', String(currentPageState.auditLimit || ''), auditLimitSelectOptions(currentPageState.auditLimit), (event) => {\" in audit_ui, audit_ui",
		"assert 'Open drift view' in audit_ui, audit_ui",
		"assert \"const buildDeploymentEventsReadRoute = (deploymentName) => `/v1/overview/deployments/${encodeURIComponent(deploymentName)}/events`;\" in ui, ui",
		"assert \"const fetchDeploymentEvents = (deploymentName) => fetchEmbeddedReadRouteJSON(buildDeploymentEventsReadRoute(deploymentName), {\" in ui, ui",
		"audit_logs_action_filtered=$(curl -fsS \"http://$HOST:$PORT/v1/audit-logs?action=apply.requested\")",
		"audit_logs_filtered=$(curl -fsS \"http://$HOST:$PORT/v1/audit-logs?deployment=payments-api-prod&limit=1\")",
		"audit_logs_empty=$(curl -fsS \"http://$HOST:$PORT/v1/audit-logs?deployment=worker-prod\")",
		"actor_plan_preview=$(curl -fsS -X POST \\",
		"X-Tenet-Actor-Id: explicit-456",
		"X-Tenet-Actor-Name: Alex Explicit",
		"X-User-ID: operator-123",
		"actor_audit_logs=$(curl -fsS \"http://$HOST:$PORT/v1/audit-logs?deployment=payments-api-staging&action=apply.requested&limit=1\")",
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
	if !strings.Contains(text, "assert webhook_push['status'] == 'accepted', webhook_push") || !strings.Contains(text, "assert webhook_ping['status'] == 'ignored', webhook_ping") || !strings.Contains(text, "assert 'github webhook signature mismatch' in invalid_webhook, invalid_webhook") {
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
	if !strings.Contains(text, "assert events_payload['deploymentName'] == 'payments-api-prod', events_payload") || !strings.Contains(text, "assert event.get('source') == 'controller', events_payload") {
		t.Fatalf("expected smoke-local.sh to verify controller-populated deployment events payload")
	}
	if !strings.Contains(text, "assert 'Runtime change details' in ui, ui") {
		t.Fatalf("expected smoke-local.sh to assert embedded html contains Runtime change details")
	}
	if !strings.Contains(text, "assert 'Current source: controller-authored managedDrift rows.' in ui, ui") || !strings.Contains(text, "assert 'Current source: derived fallback read-model heuristics.' in ui, ui") {
		t.Fatalf("expected smoke-local.sh to assert embedded html contains drift-source callout copy")
	}
	if !strings.Contains(text, "assert \"const linkedResources = mode === 'drift' ? groupResourceSections(detail).flatMap((group) => group.resources || []) : null;\" in ui, ui") {
		t.Fatalf("expected smoke-local.sh to assert drift workspace resource-link contract")
	}
	for _, fragment := range []string{
		"assert 'On this page' in ui, ui",
		"assert 'Drift summary' in ui, ui",
		"assert 'Total rows' in ui, ui",
		"assert 'Visible rows' in ui, ui",
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
		"assert 'const operatorWriteAuthRequiredMessage = (route, action) => `Tenet rejected ${route} because authenticated identity is required on operator write routes. When auth enforcement is enabled, retry ${action} through the trusted front door so it can authenticate you and forward trusted identity headers.`;' in ui, ui",
		"assert 'const operatorWriteForbiddenMessage = (route, action) => `Tenet rejected ${route} after recognizing the caller\\x27s identity, but this operator is not authorized to ${action.toLowerCase()} for that deployment through this route.`;' in ui, ui",
		"assert 'Operator access denied' in ui, ui",
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

func TestDocsDescribeTrustedFrontDoorAuthAndSmokeScope(t *testing.T) {
	files := map[string][]string{
		"../../README.md": {
			"### Trusted front-door auth",
			"Front-door auth remains additive and opt-in.",
			"TENET_API_TRUST_AUTHENTICATED_HEADERS",
			"TENET_API_ENFORCE_AUTHENTICATED_IDENTITY",
			"TENET_API_ENFORCE_OPERATOR_ROLE_AUTHORIZATION",
			"TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION",
			"strips any client-supplied `X-Tenet-Authenticated-*`, `X-Tenet-Authenticated-Roles`, and `X-Tenet-Authorized-Deployments` headers before forwarding trusted values",
			"Do not expose a header-trusting `tenet-api` instance directly to untrusted clients",
			"The same trusted front-door read contract also covers `GET /v1/overview/accounts/{awsAccountId}`",
			"`GET /v1/overview/deployments/{deploymentName}/resource-logs`, `GET /v1/overview/deployments/{deploymentName}/resource-inspection`",
			"### Smoke verification scope",
			"Use `make smoke-local` for the fastest fixture-backed API/UI regression check when you only need direct-process validation without Docker or Redis.",
			"The request-actor header contract by sending conflicting `X-Tenet-Actor-*` and `X-User-*` identities together",
			"Bookmarkable `/?page=application&deployment=payments-api-prod`, `/?page=account&accountId=123456789012`, the existing traffic-shift resource route, and one representative ECS resource route",
			"Trusted front-door auth by restarting `tenet-api`",
			"gates guarded application, raw-status collection, account collection, unscoped audit collection, raw-status detail, account detail, grouped-account, immutable plan/apply, deployment-scoped audit reads, plus the exact `GET /v1/overview/deployments/{deploymentName}/resource-logs` and `GET /v1/overview/deployments/{deploymentName}/resource-inspection` drill-down routes.",
			"the unscoped audit collection still returns `{\"entries\":[]}` when no audit rows are visible",
			"Use `make smoke-docker` when you need to validate the containerized local stack itself.",
			"API health, representative endpoints, deployment-events/resource-logs/audit-log JSON shapes, and representative `Request-Id` response headers",
			"The full `GET /v1/audit-logs` collection with expected `plan.created` and `apply.requested` actions present",
			"The same smoke-created `POST /v1/applies/{applyId}/transitions` route boundaries: `400` malformed JSON, `404` unknown apply, and `409` invalid terminal move with operator-facing text.",
			"The same actor-header precedence check used by direct-process smoke, plus the default disabled immutable apply contract.",
			"Trusted front-door auth by restarting the Compose `api` service",
			"controller-authored `managedDrift[]` evidence in `GET /v1/overview/deployments/payments-api-prod` with stable `field` / `desired` / `actual` / `severity` rows and aligned `runtimeSummary.managedDrift`",
			"survive the Compose `api` service restart",
			"`TENET_AWS_ENABLE_APPLY=true make docker-up`",
			"correlated bookmarkable shell reads for the application drill-down, account drill-down, drift workspace, deployment-scoped audit workspace, and bookmarkable plan/apply workspace",
			"The raw embedded shell/docs contract also keeps the labels `Plan preview actions`, `Apply request actions`, `Controller runtime events`, `Recent logs`, `Lookup region`, and `Observer role configured` present",
			"temp files via the shared `write_payload` helper",
			"The compose-backed smoke now covers that shipped bootstrap-source drill-down directly",
			"`aws_unavailable` and `no_log_source` as part of the expected local no-AWS contract",
			"Linux CI runners",
			"syntax-checks `scripts/smoke-local.sh` with `bash -n`, then runs `make smoke-local`",
			"syntax-checks `scripts/smoke-docker.sh` with `bash -n` before running `make smoke-docker`",
			"make smoke-docker",
		},
		"../../docs/architecture/tenet-v1-local-runtime.md": {
			"## Trusted front-door auth",
			"Front-door auth remains additive and opt-in.",
			"TENET_API_TRUST_AUTHENTICATED_HEADERS",
			"TENET_API_ENFORCE_AUTHENTICATED_IDENTITY",
			"TENET_API_ENFORCE_OPERATOR_ROLE_AUTHORIZATION",
			"TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION",
			"strips any client-supplied `X-Tenet-Authenticated-*`, `X-Tenet-Authenticated-Roles`, and `X-Tenet-Authorized-Deployments` headers before forwarding trusted values",
			"Do not expose a header-trusting `tenet-api` instance directly to untrusted clients",
			"The same trusted front-door read contract also covers `GET /v1/overview/accounts/{awsAccountId}`",
			"`GET /v1/overview/deployments/{deploymentName}/resource-logs`, `GET /v1/overview/deployments/{deploymentName}/resource-inspection`",
			"Use `make smoke-local` for the fastest fixture-backed regression check when you only need direct-process validation.",
			"Additive `X-User-*` alias support plus explicit `X-Tenet-Actor-*` precedence by sending both header families with conflicting identities",
			"Bookmarkable `/?page=application&deployment=payments-api-prod`, `/?page=account&accountId=123456789012`, the existing traffic-shift resource route, and one representative ECS resource route",
			"Trusted front-door auth by restarting `tenet-api`",
			"gates guarded application, raw-status collection, account collection, unscoped audit collection, raw-status detail, account detail, grouped-account, immutable plan/apply, deployment-scoped audit reads, plus the exact `GET /v1/overview/deployments/{deploymentName}/resource-logs` and `GET /v1/overview/deployments/{deploymentName}/resource-inspection` drill-down routes.",
			"The bookmarkable `/?page=plan` workspace, but only behind a trusted front door that strips inbound trust headers before forwarding its own values",
			"### Docker stack smoke check",
			"make smoke-docker",
			"`docker compose config` plus the `api`, `controller`, and `redis` services present in `compose.yaml`",
			"The full `GET /v1/audit-logs` collection, exact-action filtering with `GET /v1/audit-logs?action=apply.requested&limit=2`, and `400 Bad Request` on `action=invalid.action`",
			"controller-authored `managedDrift[]` evidence in `GET /v1/overview/deployments/payments-api-prod` with stable `field` / `desired` / `actual` / `severity` rows and aligned `runtimeSummary.managedDrift`",
			"The same direct-process actor coverage by sending both header families with conflicting identities on a `payments-api-staging` plan/apply flow",
			"Bookmarkable application, account, drift, audit, and plan workspace URLs, checking raw embedded application/account/resource/drift/audit/plan shell HTML/script fragments instead of runtime DOM text",
			"Drift page source/summary/table copy, audit filter copy such as `Audit filters: action apply.requested · limit 2`, the `Clear audit filters` recovery affordance, and correlated `http_request` logs",
			"Trusted front-door auth by restarting the Compose `api` service",
			"authenticated but unauthorized transitions fail with `403`",
			"survive the Compose `api` service restart",
			"`TENET_AWS_ENABLE_APPLY=true make docker-up`",
			"correlated structured `http_request` logs with stable fields such as `request_id`, `method`, `path`, `status`, `duration_ms`, and `result` for the bookmarkable application drill-down shell request, bookmarkable account drill-down shell request, bookmarkable drift workspace request, the deployment-scoped audit workspace request, and the bookmarkable plan/apply workspace shell request",
			"`Plan preview actions`, `Apply request actions`, `Controller runtime events`, `Recent logs`, `Lookup region`, and `Observer role configured`",
			"temp files through the shared `write_payload` helper",
			"bookmarkable bootstrap-source resource page plus the paired `userDataBootstrapSource` `resource-logs` and `resource-inspection` routes",
			"`aws_unavailable` / `no_log_source` unavailable payloads",
			"Linux CI runners with tighter environment-size limits",
			"syntax-checks `scripts/smoke-local.sh` with `bash -n`, then runs `make smoke-local`",
			"syntax-checks `scripts/smoke-docker.sh` with `bash -n`, then runs `make smoke-docker`",
			"`docker compose down --remove-orphans` even on failure",
		},
		"../../docs/architecture/tenet-v1-overview-ui.md": {
			"`/?page=plan&deployment={deploymentName}&planId={planId}` workspace keeps the URL shareable/bookmarkable",
			"`POST /v1/applies/{applyId}/transitions`",
			"when `POST /v1/plans/deployments/{deploymentName}`, `POST /v1/applies/plans/{planId}`, or `POST /v1/applies/{applyId}/transitions` returns `401 Unauthorized` under auth enforcement, the embedded workspace explains that authenticated identity is required on operator write routes and tells operators to retry through the trusted front door instead of showing generic fetch-error copy",
			"the persisted apply deployment for transitions",
			"trusted front door",
			"bookmarkable `/?page=resource&deployment=payments-api-prod&resourceType=userDataBootstrapSource&resourceId=userDataBootstrapSource::file:userdata/payments-api-prod.sh::Normalized userdata bootstrap source for candidate EC2 capacity` page",
			"`aws_unavailable` and `no_log_source` as expected local no-AWS outcomes",
		},
		"../../docs/features/overview-dashboard.md": {
			"when `TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION` is enabled, `GET /v1/overview/applications` requires trusted `X-Tenet-Authenticated-*` identity",
			"when `TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION` is enabled, stored `GET /v1/plans/{planId}` reloads reuse the same trusted `X-Tenet-Authenticated-*` identity",
			"when `TENET_API_ENFORCE_OPERATOR_ROLE_AUTHORIZATION` is enabled, `POST /v1/plans/deployments/{deploymentName}`",
			"when `TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION` is enabled, stored `GET /v1/applies/{applyId}` reloads use the same trusted `X-Tenet-Authenticated-*` identity",
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
		"assert_fragments(\"application_ui\", application_ui, (",
		"\"Quick navigation\",",
		"\"Applications overview\",",
		"\"Current destination\",",
		"\"createNavLink(buildPageURL('applications'), 'Applications overview', '_self')\",",
		"\"createQuickNavChip('Application detail', 'Current destination: Application detail')\",",
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
		"\"createQuickNavChip('Account detail', 'Current destination: Account detail')\",",
		"\"createQuickNavChip(currentSectionLink.label, `Current section: ${currentSectionLink.label}`)\",",
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
}

func TestOverviewDocsDescribeAccountSectionNavigation(t *testing.T) {
	files := map[string][]string{
		"../../README.md": {
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
		"../../README.md": {
			"`Refresh account detail`",
			"re-runs only `GET /v1/overview/accounts/{awsAccountId}`",
			"leaves the stale account content visible with explicit loading/error copy",
		},
		"../../docs/features/overview-dashboard.md": {
			"`Refresh account detail`",
			"re-runs only `GET /v1/overview/accounts/{awsAccountId}`",
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
		"../../README.md": {
			"`Clear audit filters`",
			"operators can recover to the unfiltered `/?page=audit` slice without dropping bookmarkable `q` / `sync` / `account` state",
		},
		"../../docs/features/overview-dashboard.md": {
			"`Clear audit filters`",
			"resets those audit filters back to the default `/?page=audit` slice",
		},
		"../../docs/architecture/tenet-v1-overview-ui.md": {
			"`Clear audit filters` recovery action",
			"without dropping bookmarkable applications-shell query state such as `q`, `sync`, or `account`",
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
		"../../README.md": {
			"`Refresh live inspection` control",
			"`Refresh recent logs` control",
			"re-run only that panel-local backend route in place",
			"leaves the sibling panel and the rest of the resource page visible",
			"replaces generic fetch-error copy with the same trusted-front-door / access-denied guidance used on other embedded read paths",
		},
		"../../docs/features/overview-dashboard.md": {
			"`Refresh recent logs` and `Refresh live inspection` controls",
			"re-run only that panel's backend route in place",
			"leaves the sibling panel and the rest of the resource page visible",
			"replaces generic fetch-error copy with the same trusted-front-door / access-denied guidance used on other embedded read paths",
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
		"../../README.md": {
			"The same bookmarkable application, account, traffic-shift resource, ECS resource, drift, audit, and plan routes locked by direct-process smoke",
			"the bookmarkable application shell still exposes the `_self` `Applications overview` return path plus the non-click `Application detail` destination chip",
			"raw embedded application/account/resource/drift/audit/plan shell HTML/script fragments instead of runtime DOM text",
			"the shipped operator read-auth guidance hooks for initial `401` / `403` failures remain present",
		},
		"../../docs/architecture/tenet-v1-local-runtime.md": {
			"Bookmarkable application, account, drift, audit, and plan workspace URLs, checking raw embedded application/account/resource/drift/audit/plan shell HTML/script fragments instead of runtime DOM text",
			"the bookmarkable application shell still exposes the `_self` `Applications overview` return path plus the non-click `Application detail` destination chip",
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
