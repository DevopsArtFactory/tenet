package main

import (
	"context"
	"fmt"
	"log"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/DevopsArtFactory/tenet/internal/domain"
	"github.com/DevopsArtFactory/tenet/internal/placement"
	"github.com/DevopsArtFactory/tenet/internal/reconcile"
	"github.com/DevopsArtFactory/tenet/internal/resourceinspection"
	"github.com/DevopsArtFactory/tenet/internal/statusstore"
)

const (
	runtimeHistorySourceController = "controller"
	runtimeHistoryMaxEntries       = 10
	controllerMarkedSyncedReason   = "Controller marked deployment synced"
	controllerDriftedSyncReason    = "Controller detected managed drift in completed deployment"
	controllerHealthySyncedSummary = "Deployment is healthy and synced"
	controllerHealthyDriftSummary  = "Deployment is healthy but has managed drift"
	rolloutHealthySyncedSummary    = "Deployment is synced and healthy"
	rolloutSyncedSummary           = "Deployment is synced"
	rolloutManagedDriftSummary     = "Deployment has managed drift"
)

var runtimeHistoryNow = time.Now

type runtimeLiveCapacityObservation struct {
	Desired             int
	Min                 int
	Max                 int
	TaskDefinitionImage string
}

var runtimeLiveCapacityLookup = runtimeLiveCapacityLookupFromInspection

func emitRuntimePlans(ctx context.Context, logger *log.Logger, config startupConfig, store statusstore.Store) error {
	source := config.catalogSource()
	if !source.configured() {
		logger.Printf("controller runtime disabled reason=no_catalog_source")
		return nil
	}

	compiled, err := source.loadCatalog(ctx)
	if err != nil {
		return fmt.Errorf("load runtime catalog: %w", err)
	}

	if store == nil {
		runtimeStore, err := openRuntimeStatusStore(ctx, logger, config)
		if err != nil {
			return err
		}
		if runtimeStore != nil {
			store = runtimeStore
			defer func() {
				_ = runtimeStore.Close()
			}()
		}
	}

	statuses := map[string]domain.DeploymentStatus{}
	if store != nil {
		if err := statusstore.SeedFromFileIfEmpty(ctx, store, config.StatusSeedPath); err != nil {
			return fmt.Errorf("seed runtime statuses: %w", err)
		}
		snapshots, err := store.List(ctx)
		if err != nil {
			return fmt.Errorf("load runtime statuses: %w", err)
		}
		for _, snapshot := range snapshots {
			statuses[snapshot.DeploymentName] = snapshot.Status
		}
	}

	currentStatuses := make(map[string]domain.DeploymentStatus, len(compiled.Deployments))
	if len(compiled.Deployments) == 0 {
		if batchWriter := runtimeStatusBatchWriter(store); batchWriter != nil {
			staleNames := make([]string, 0, len(statuses))
			for deploymentName := range statuses {
				staleNames = append(staleNames, deploymentName)
			}
			sort.Strings(staleNames)
			if err := batchWriter.Apply(ctx, nil, staleNames); err != nil {
				return fmt.Errorf("apply runtime status batch: %w", err)
			}
		} else if store != nil {
			if err := store.Save(ctx, nil); err != nil {
				return fmt.Errorf("save runtime statuses: %w", err)
			}
		}
		logger.Printf("controller runtime found no deployments")
		return nil
	}

	planner := reconcile.Planner{}
	liveInspectionService := resourceinspection.NewService(resourceinspection.Options{EnableAWS: true})
	for _, deployment := range compiled.Deployments {
		status := statuses[deployment.Name]
		previousStatus := status
		status = normalizeRuntimeStatusForPlanning(deployment, status)
		plan := planner.Build(deployment, status)
		status = normalizeRuntimeStatus(deployment, status, plan)
		status.Rollout = deriveRolloutSummary(deployment, status, plan)
		liveCapacity := runtimeLiveCapacity(ctx, logger, liveInspectionService, compiled.Accounts, compiled.Environments, deployment)
		status.ManagedDrift = deriveManagedDriftItems(deployment, status, liveCapacity)
		status = applyControllerManagedDriftStatus(status, plan)
		status.History = appendRuntimeHistory(previousStatus, status)
		currentStatuses[deployment.Name] = status
		logger.Printf(
			"controller reconcile deployment=%s strategy=%s execution_mode=%s desired_generation=%d observed_generation=%d sync_state=%s steps=%s",
			deployment.Name,
			deployment.Strategy,
			deployment.ExecutionMode,
			deployment.Generation,
			status.ObservedGeneration,
			syncStateLabel(status.Sync.State),
			stepSummary(plan),
		)
	}

	if store != nil {
		snapshots := make([]statusstore.Snapshot, 0, len(currentStatuses))
		for deploymentName, status := range currentStatuses {
			snapshots = append(snapshots, statusstore.Snapshot{DeploymentName: deploymentName, Status: status})
		}
		sort.Slice(snapshots, func(i, j int) bool {
			return snapshots[i].DeploymentName < snapshots[j].DeploymentName
		})
		staleNames := make([]string, 0)
		for deploymentName := range statuses {
			if _, ok := currentStatuses[deploymentName]; ok {
				continue
			}
			staleNames = append(staleNames, deploymentName)
		}
		sort.Strings(staleNames)
		if batchWriter := runtimeStatusBatchWriter(store); batchWriter != nil {
			changedSnapshots := changedRuntimeSnapshots(snapshots, statuses)
			if len(changedSnapshots) == 0 && len(staleNames) == 0 {
				return nil
			}
			if err := batchWriter.Apply(ctx, changedSnapshots, staleNames); err != nil {
				return fmt.Errorf("apply runtime status batch: %w", err)
			}
		} else {
			if err := store.Save(ctx, snapshots); err != nil {
				return fmt.Errorf("save runtime statuses: %w", err)
			}
		}
	}

	return nil
}

func changedRuntimeSnapshots(current []statusstore.Snapshot, previous map[string]domain.DeploymentStatus) []statusstore.Snapshot {
	changed := make([]statusstore.Snapshot, 0, len(current))
	for _, snapshot := range current {
		prior, ok := previous[snapshot.DeploymentName]
		if ok && reflect.DeepEqual(snapshot.Status, prior) {
			continue
		}
		changed = append(changed, snapshot)
	}
	return changed
}

func runtimeStatusBatchWriter(store statusstore.Store) statusstore.BatchWriter {
	writer, ok := store.(statusstore.BatchWriter)
	if !ok {
		return nil
	}
	return writer
}

func appendRuntimeHistory(previous domain.DeploymentStatus, current domain.DeploymentStatus) []domain.StatusHistoryEntry {
	history := append([]domain.StatusHistoryEntry(nil), current.History...)
	entry := deriveRuntimeHistoryEntry(current)
	previousEntry := deriveRuntimeHistoryEntry(previous)
	if len(history) == 0 {
		entry.Timestamp = runtimeHistoryNow().UTC().Format(time.RFC3339)
		entry.Changes, entry.ChangeDetails = runtimeHistoryInitialDiff(previousEntry, entry)
		return []domain.StatusHistoryEntry{entry}
	}

	latest := previousEntry
	if len(history) > 0 {
		latest = history[len(history)-1]
	}
	normalizedLatest := normalizeRuntimeHistoryEntryContext(latest, previousEntry)
	normalizedLatest = normalizeRuntimeHistoryEntryContext(normalizedLatest, entry)
	if len(history) > 0 && !reflect.DeepEqual(normalizedLatest, history[len(history)-1]) {
		history[len(history)-1] = normalizedLatest
	}
	if !runtimeHistoryMeaningfullyChanged(normalizedLatest, entry) {
		return capRuntimeHistory(history)
	}

	entry.Timestamp = runtimeHistoryNow().UTC().Format(time.RFC3339)
	if !runtimeHistoryHasControllerEntry(history) {
		entry.Changes, entry.ChangeDetails = runtimeHistoryInitialDiff(normalizedLatest, entry)
	} else {
		entry.Changes, entry.ChangeDetails = runtimeHistoryDiff(normalizedLatest, entry)
	}
	history = append(history, entry)
	return capRuntimeHistory(history)
}

func normalizeRuntimeHistoryEntryContext(entry domain.StatusHistoryEntry, fallback domain.StatusHistoryEntry) domain.StatusHistoryEntry {
	if entry.Source != runtimeHistorySourceController {
		return entry
	}
	if entry.DeployedArtifact == (domain.DeploymentArtifact{}) {
		entry.DeployedArtifact = fallback.DeployedArtifact
	}
	if len(entry.Conditions) == 0 && len(fallback.Conditions) > 0 {
		entry.Conditions = normalizeRuntimeHistoryConditions(fallback.Conditions)
	} else {
		entry.Conditions = normalizeRuntimeHistoryConditions(entry.Conditions)
	}
	if len(entry.ManagedDrift) == 0 && len(fallback.ManagedDrift) > 0 {
		entry.ManagedDrift = normalizeManagedDriftItems(fallback.ManagedDrift)
	} else {
		entry.ManagedDrift = normalizeManagedDriftItems(entry.ManagedDrift)
	}
	if entry.TargetType == "" {
		entry.TargetType = fallback.TargetType
	}
	if entry.Strategy == "" {
		entry.Strategy = fallback.Strategy
	}
	if entry.ExecutionMode == "" {
		entry.ExecutionMode = fallback.ExecutionMode
	}
	return entry
}

func runtimeLiveCapacity(ctx context.Context, logger *log.Logger, inspector resourceinspection.Service, accounts []domain.Account, environments []domain.Environment, deployment domain.Deployment) *runtimeLiveCapacityObservation {
	observation, err := runtimeLiveCapacityLookup(ctx, inspector, accounts, environments, deployment)
	if err == nil {
		return observation
	}
	if logger != nil {
		logger.Printf(
			"controller live managed drift enrichment skipped deployment=%s execution_mode=%s err=%v",
			deployment.Name,
			deployment.ExecutionMode,
			err,
		)
	}
	return nil
}

func runtimeLiveCapacityLookupFromInspection(ctx context.Context, inspector resourceinspection.Service, accounts []domain.Account, environments []domain.Environment, deployment domain.Deployment) (*runtimeLiveCapacityObservation, error) {
	resourceType, resourceName, ok := runtimeLiveCapacityResource(deployment)
	if !ok {
		return nil, nil
	}
	resolved, err := (placement.Resolver{}).Resolve(accounts, environments, deployment)
	if err != nil {
		return nil, err
	}
	result, err := inspector.Inspect(ctx, resourceinspection.Request{
		DeploymentName:  deployment.Name,
		ResourceType:    resourceType,
		ResourceName:    resourceName,
		TargetAccountID: resolved.AccountID,
		Region:          resolved.Region,
	})
	if err != nil {
		return nil, err
	}
	if !result.Available {
		return nil, nil
	}
	if result.AutoScalingGroup != nil {
		return &runtimeLiveCapacityObservation{
			Desired: int(result.AutoScalingGroup.DesiredCapacity),
			Min:     int(result.AutoScalingGroup.MinSize),
			Max:     int(result.AutoScalingGroup.MaxSize),
		}, nil
	}
	if result.ECSService != nil {
		return &runtimeLiveCapacityObservation{
			Desired:             int(result.ECSService.DesiredCount),
			TaskDefinitionImage: strings.TrimSpace(result.ECSService.TaskDefinitionImage),
		}, nil
	}
	return nil, nil
}

func runtimeLiveCapacityResource(deployment domain.Deployment) (resourceType string, resourceName string, ok bool) {
	for _, detail := range deriveRolloutExecutionDetails(deployment) {
		switch detail.Resource {
		case "candidateAutoScalingGroup", "serviceAutoScalingGroup", "ecsService":
			return detail.Resource, detail.Name, true
		}
	}
	return "", "", false
}

func deriveManagedDriftItems(deployment domain.Deployment, status domain.DeploymentStatus, liveCapacity *runtimeLiveCapacityObservation) []domain.ManagedDriftItem {
	items := make([]domain.ManagedDriftItem, 0, 8)

	desiredArtifact := describeDeploymentArtifact(deployment.DesiredArtifact)
	actualArtifact := describeDeploymentArtifact(status.DeployedArtifact)
	if desiredArtifact != "none" && actualArtifact != "none" && desiredArtifact != actualArtifact {
		items = append(items, domain.ManagedDriftItem{
			Field:    "artifact",
			Desired:  desiredArtifact,
			Actual:   actualArtifact,
			Severity: domain.ManagedDriftSeverityHigh,
		})
	}

	observedGeneration := status.ObservedGeneration
	if status.Rollout != nil && status.Rollout.ObservedGeneration != 0 {
		observedGeneration = status.Rollout.ObservedGeneration
	}
	if deployment.Generation > 0 && observedGeneration > 0 && deployment.Generation != observedGeneration {
		items = append(items, domain.ManagedDriftItem{
			Field:    "generation",
			Desired:  fmt.Sprintf("%d", deployment.Generation),
			Actual:   fmt.Sprintf("%d", observedGeneration),
			Severity: domain.ManagedDriftSeverityMedium,
		})
	}

	if status.Rollout != nil {
		if deployment.TargetType != "" && status.Rollout.TargetType != "" && deployment.TargetType != status.Rollout.TargetType {
			items = append(items, domain.ManagedDriftItem{
				Field:    "runtimeContext.targetType",
				Desired:  deployment.TargetType,
				Actual:   status.Rollout.TargetType,
				Severity: domain.ManagedDriftSeverityMedium,
			})
		}
		if deployment.Strategy != "" && status.Rollout.Strategy != "" && deployment.Strategy != status.Rollout.Strategy {
			items = append(items, domain.ManagedDriftItem{
				Field:    "runtimeContext.strategy",
				Desired:  deployment.Strategy,
				Actual:   status.Rollout.Strategy,
				Severity: domain.ManagedDriftSeverityMedium,
			})
		}
		if deployment.ExecutionMode != "" && status.Rollout.ExecutionMode != "" && deployment.ExecutionMode != status.Rollout.ExecutionMode {
			items = append(items, domain.ManagedDriftItem{
				Field:    "runtimeContext.executionMode",
				Desired:  deployment.ExecutionMode,
				Actual:   status.Rollout.ExecutionMode,
				Severity: domain.ManagedDriftSeverityMedium,
			})
		}
	}

	if liveCapacity != nil {
		if desiredImage := strings.TrimSpace(deployment.DesiredArtifact.Image); desiredImage != "" && liveCapacity.TaskDefinitionImage != "" && desiredImage != liveCapacity.TaskDefinitionImage {
			items = append(items, domain.ManagedDriftItem{
				Field:    "runtimeContext.taskDefinition.image",
				Desired:  desiredImage,
				Actual:   liveCapacity.TaskDefinitionImage,
				Severity: domain.ManagedDriftSeverityHigh,
			})
		}
		desiredCapacity := deployment.RolloutConfig.Capacity
		if desiredCapacity.Desired != liveCapacity.Desired {
			items = append(items, domain.ManagedDriftItem{
				Field:    "runtimeContext.capacity.desired",
				Desired:  fmt.Sprintf("%d", desiredCapacity.Desired),
				Actual:   fmt.Sprintf("%d", liveCapacity.Desired),
				Severity: domain.ManagedDriftSeverityMedium,
			})
		}
		switch deployment.ExecutionMode {
		case "asgWeightedTargetGroups", "asgInstanceRefresh":
			if desiredCapacity.Min != liveCapacity.Min {
				items = append(items, domain.ManagedDriftItem{
					Field:    "runtimeContext.capacity.min",
					Desired:  fmt.Sprintf("%d", desiredCapacity.Min),
					Actual:   fmt.Sprintf("%d", liveCapacity.Min),
					Severity: domain.ManagedDriftSeverityMedium,
				})
			}
			if desiredCapacity.Max != liveCapacity.Max {
				items = append(items, domain.ManagedDriftItem{
					Field:    "runtimeContext.capacity.max",
					Desired:  fmt.Sprintf("%d", desiredCapacity.Max),
					Actual:   fmt.Sprintf("%d", liveCapacity.Max),
					Severity: domain.ManagedDriftSeverityMedium,
				})
			}
		}
	}

	return normalizeManagedDriftItems(items)
}

func normalizeManagedDriftItems(items []domain.ManagedDriftItem) []domain.ManagedDriftItem {
	if len(items) == 0 {
		return nil
	}
	normalized := append([]domain.ManagedDriftItem(nil), items...)
	sort.Slice(normalized, func(i, j int) bool {
		return managedDriftItemLess(normalized[i], normalized[j])
	})
	return normalized
}

func managedDriftItemLess(left domain.ManagedDriftItem, right domain.ManagedDriftItem) bool {
	if left.Field != right.Field {
		return left.Field < right.Field
	}
	if left.Desired != right.Desired {
		return left.Desired < right.Desired
	}
	if left.Actual != right.Actual {
		return left.Actual < right.Actual
	}
	return left.Severity < right.Severity
}

func runtimeHistoryHasControllerEntry(history []domain.StatusHistoryEntry) bool {
	for _, entry := range history {
		if entry.Source == runtimeHistorySourceController {
			return true
		}
	}
	return false
}

func runtimeHistoryChanges(previous domain.StatusHistoryEntry, current domain.StatusHistoryEntry) []string {
	changes, _ := runtimeHistoryDiff(previous, current)
	return changes
}

func runtimeHistoryChangeDetails(previous domain.StatusHistoryEntry, current domain.StatusHistoryEntry) []string {
	_, details := runtimeHistoryDiff(previous, current)
	return details
}

func runtimeHistoryInitialDiff(previous domain.StatusHistoryEntry, current domain.StatusHistoryEntry) ([]string, []string) {
	changes := []string{"initial"}
	details := []string{"Initial controller-authored history entry"}
	diffChanges, diffDetails := runtimeHistoryDiff(previous, current)
	return append(changes, diffChanges...), append(details, diffDetails...)
}

func runtimeHistoryDiff(previous domain.StatusHistoryEntry, current domain.StatusHistoryEntry) ([]string, []string) {
	changes := make([]string, 0, 14)
	details := make([]string, 0, 14)
	if previous.Summary != current.Summary {
		changes = append(changes, "summary")
		details = append(details, fmt.Sprintf("Summary changed to %s", valueOrUnknown(current.Summary)))
	}
	if previous.DeployedArtifact != current.DeployedArtifact {
		changes = append(changes, "deployedArtifact")
		details = append(details, fmt.Sprintf("Artifact changed to %s", describeDeploymentArtifact(current.DeployedArtifact)))
	}
	if !reflect.DeepEqual(previous.ManagedDrift, current.ManagedDrift) {
		changes = append(changes, "managedDrift")
		details = append(details, describeManagedDriftDelta(previous.ManagedDrift, current.ManagedDrift))
	}
	if !runtimeHistoryConditionsEqual(previous.Conditions, current.Conditions) {
		changes = append(changes, "conditions")
		details = append(details, fmt.Sprintf("Conditions changed to %s", describeConditions(current.Conditions)))
	}
	if previous.Sync != current.Sync {
		changes = append(changes, "sync")
		details = append(details, fmt.Sprintf("Sync changed to %s%s", valueOrUnknown(string(current.Sync.State)), suffixWithColon(current.Sync.Reason)))
	}
	if previous.Health != current.Health {
		changes = append(changes, "health")
		details = append(details, fmt.Sprintf("Health changed to %s%s", valueOrUnknown(string(current.Health.Overall)), suffixWithColon(current.Health.Summary)))
	}
	if previous.DesiredGeneration != current.DesiredGeneration {
		changes = append(changes, "desiredGeneration")
		details = append(details, fmt.Sprintf("Desired generation changed from %d to %d", previous.DesiredGeneration, current.DesiredGeneration))
	}
	if previous.ObservedGeneration != current.ObservedGeneration {
		changes = append(changes, "observedGeneration")
		details = append(details, fmt.Sprintf("Observed generation changed from %d to %d", previous.ObservedGeneration, current.ObservedGeneration))
	}
	if previous.RolloutPhase != current.RolloutPhase {
		changes = append(changes, "rolloutPhase")
		details = append(details, fmt.Sprintf("Rollout phase changed from %s to %s", valueOrUnknown(string(previous.RolloutPhase)), valueOrUnknown(string(current.RolloutPhase))))
	}
	if previous.CurrentStep != current.CurrentStep {
		changes = append(changes, "currentStep")
		details = append(details, fmt.Sprintf("Current step changed from %s to %s", valueOrUnknown(previous.CurrentStep), valueOrUnknown(current.CurrentStep)))
	}
	if previous.TargetType != current.TargetType {
		changes = append(changes, "targetType")
		details = append(details, fmt.Sprintf("Target type changed from %s to %s", valueOrUnknown(previous.TargetType), valueOrUnknown(current.TargetType)))
	}
	if previous.Strategy != current.Strategy {
		changes = append(changes, "strategy")
		details = append(details, fmt.Sprintf("Strategy changed from %s to %s", valueOrUnknown(previous.Strategy), valueOrUnknown(current.Strategy)))
	}
	if previous.ExecutionMode != current.ExecutionMode {
		changes = append(changes, "executionMode")
		details = append(details, fmt.Sprintf("Execution mode changed from %s to %s", valueOrUnknown(previous.ExecutionMode), valueOrUnknown(current.ExecutionMode)))
	}
	return changes, details
}

func describeDeploymentArtifact(artifact domain.DeploymentArtifact) string {
	switch {
	case artifact.Binary != "":
		return artifact.Binary
	case artifact.Image != "":
		return artifact.Image
	default:
		return "none"
	}
}

func describeManagedDrift(items []domain.ManagedDriftItem) string {
	if len(items) == 0 {
		return "none"
	}
	parts := make([]string, 0, len(items))
	for _, item := range normalizeManagedDriftItems(items) {
		part := item.Field
		if item.Desired != "" || item.Actual != "" {
			part += fmt.Sprintf(" (%s -> %s)", valueOrUnknown(item.Desired), valueOrUnknown(item.Actual))
		}
		if item.Severity != "" {
			part += fmt.Sprintf(" [%s]", item.Severity)
		}
		parts = append(parts, part)
	}
	return strings.Join(parts, ", ")
}

func describeManagedDriftDelta(previous []domain.ManagedDriftItem, current []domain.ManagedDriftItem) string {
	previousItems := normalizeManagedDriftItems(previous)
	currentItems := normalizeManagedDriftItems(current)
	if !managedDriftItemsHaveUniqueFields(previousItems) || !managedDriftItemsHaveUniqueFields(currentItems) {
		return fmt.Sprintf("Managed drift changed to %s", describeManagedDrift(currentItems))
	}

	previousByField := make(map[string]domain.ManagedDriftItem, len(previousItems))
	for _, item := range previousItems {
		previousByField[item.Field] = item
	}
	currentByField := make(map[string]domain.ManagedDriftItem, len(currentItems))
	for _, item := range currentItems {
		currentByField[item.Field] = item
	}

	added := make([]domain.ManagedDriftItem, 0)
	removed := make([]domain.ManagedDriftItem, 0)
	updated := make([]managedDriftUpdate, 0)
	for _, item := range currentItems {
		previousItem, ok := previousByField[item.Field]
		if !ok {
			added = append(added, item)
			continue
		}
		if previousItem != item {
			updated = append(updated, managedDriftUpdate{
				Previous: previousItem,
				Current:  item,
			})
		}
	}
	for _, item := range previousItems {
		if _, ok := currentByField[item.Field]; !ok {
			removed = append(removed, item)
		}
	}

	parts := make([]string, 0, 3)
	if len(added) > 0 {
		labels := make([]string, 0, len(added))
		for _, item := range normalizeManagedDriftItems(added) {
			labels = append(labels, describeManagedDriftRow(item))
		}
		parts = append(parts, "added rows "+strings.Join(labels, ", "))
	}
	if len(removed) > 0 {
		labels := make([]string, 0, len(removed))
		for _, item := range normalizeManagedDriftItems(removed) {
			labels = append(labels, describeManagedDriftRow(item))
		}
		parts = append(parts, "removed rows "+strings.Join(labels, ", "))
	}
	if len(updated) > 0 {
		sort.Slice(updated, func(i, j int) bool {
			left := updated[i]
			right := updated[j]
			if left.Current.Field != right.Current.Field {
				return left.Current.Field < right.Current.Field
			}
			if managedDriftItemLess(left.Previous, right.Previous) {
				return true
			}
			if managedDriftItemLess(right.Previous, left.Previous) {
				return false
			}
			return managedDriftItemLess(left.Current, right.Current)
		})
		labels := make([]string, 0, len(updated))
		for _, item := range updated {
			labels = append(labels, describeManagedDriftUpdate(item.Previous, item.Current))
		}
		parts = append(parts, "updated rows "+strings.Join(labels, ", "))
	}
	if len(parts) == 0 {
		return fmt.Sprintf("Managed drift changed to %s", describeManagedDrift(currentItems))
	}
	return "Managed drift changed: " + strings.Join(parts, "; ")
}

type managedDriftUpdate struct {
	Previous domain.ManagedDriftItem
	Current  domain.ManagedDriftItem
}

func managedDriftItemsHaveUniqueFields(items []domain.ManagedDriftItem) bool {
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		if _, ok := seen[item.Field]; ok {
			return false
		}
		seen[item.Field] = struct{}{}
	}
	return true
}

func describeManagedDriftRow(item domain.ManagedDriftItem) string {
	state := describeManagedDriftRowState(item)
	if state == "" {
		return valueOrUnknown(item.Field)
	}
	return fmt.Sprintf("%s %s", valueOrUnknown(item.Field), state)
}

func describeManagedDriftUpdate(previous domain.ManagedDriftItem, current domain.ManagedDriftItem) string {
	field := current.Field
	if field == "" {
		field = previous.Field
	}
	return fmt.Sprintf("%s from %s to %s", valueOrUnknown(field), describeManagedDriftRowState(previous), describeManagedDriftRowState(current))
}

func describeManagedDriftRowState(item domain.ManagedDriftItem) string {
	parts := make([]string, 0, 2)
	if item.Severity != "" {
		parts = append(parts, fmt.Sprintf("[%s]", item.Severity))
	}
	if item.Desired != "" || item.Actual != "" {
		parts = append(parts, fmt.Sprintf("(desired=%s, actual=%s)", valueOrUnknown(item.Desired), valueOrUnknown(item.Actual)))
	}
	return strings.Join(parts, " ")
}

func describeConditions(conditions []domain.Condition) string {
	if len(conditions) == 0 {
		return "none"
	}
	parts := make([]string, 0, len(conditions))
	for _, condition := range normalizeRuntimeHistoryConditions(conditions) {
		part := fmt.Sprintf("%s=%s", valueOrUnknown(condition.Type), boolLabel(condition.Status))
		if condition.Reason != "" {
			part += fmt.Sprintf(" (%s)", condition.Reason)
		}
		parts = append(parts, part)
	}
	return strings.Join(parts, ", ")
}

func normalizeRuntimeHistoryConditions(conditions []domain.Condition) []domain.Condition {
	if len(conditions) == 0 {
		return nil
	}
	normalized := append([]domain.Condition(nil), conditions...)
	sort.Slice(normalized, func(i, j int) bool {
		left := normalized[i]
		right := normalized[j]
		if left.Type != right.Type {
			return left.Type < right.Type
		}
		if left.Status != right.Status {
			return !left.Status && right.Status
		}
		if left.Reason != right.Reason {
			return left.Reason < right.Reason
		}
		return left.Message < right.Message
	})
	return normalized
}

func runtimeHistoryConditionsEqual(previous []domain.Condition, current []domain.Condition) bool {
	return reflect.DeepEqual(normalizeRuntimeHistoryConditions(previous), normalizeRuntimeHistoryConditions(current))
}

func boolLabel(value bool) string {
	if value {
		return "True"
	}
	return "False"
}

func suffixWithColon(value string) string {
	if value == "" {
		return ""
	}
	return ": " + value
}

func valueOrUnknown(value string) string {
	if value == "" {
		return "Unknown"
	}
	return value
}

func capRuntimeHistory(history []domain.StatusHistoryEntry) []domain.StatusHistoryEntry {
	if len(history) <= runtimeHistoryMaxEntries {
		return history
	}
	return append([]domain.StatusHistoryEntry(nil), history[len(history)-runtimeHistoryMaxEntries:]...)
}

func deriveRuntimeHistoryEntry(status domain.DeploymentStatus) domain.StatusHistoryEntry {
	entry := domain.StatusHistoryEntry{
		Source:             runtimeHistorySourceController,
		Summary:            "Runtime status updated",
		ManagedDrift:       normalizeManagedDriftItems(status.ManagedDrift),
		DeployedArtifact:   status.DeployedArtifact,
		Conditions:         normalizeRuntimeHistoryConditions(status.Conditions),
		Sync:               status.Sync,
		Health:             status.Health,
		ObservedGeneration: status.ObservedGeneration,
	}
	if status.Rollout != nil {
		entry.Summary = status.Rollout.Summary
		entry.DesiredGeneration = status.Rollout.DesiredGeneration
		entry.ObservedGeneration = status.Rollout.ObservedGeneration
		entry.RolloutPhase = status.Rollout.Phase
		entry.CurrentStep = status.Rollout.CurrentStep
		entry.TargetType = status.Rollout.TargetType
		entry.Strategy = status.Rollout.Strategy
		entry.ExecutionMode = status.Rollout.ExecutionMode
	}
	return entry
}

func runtimeHistoryMeaningfullyChanged(previous domain.StatusHistoryEntry, current domain.StatusHistoryEntry) bool {
	return previous.Source != current.Source ||
		previous.Summary != current.Summary ||
		previous.DeployedArtifact != current.DeployedArtifact ||
		!reflect.DeepEqual(previous.ManagedDrift, current.ManagedDrift) ||
		!runtimeHistoryConditionsEqual(previous.Conditions, current.Conditions) ||
		previous.Sync != current.Sync ||
		previous.Health != current.Health ||
		previous.DesiredGeneration != current.DesiredGeneration ||
		previous.ObservedGeneration != current.ObservedGeneration ||
		previous.RolloutPhase != current.RolloutPhase ||
		previous.CurrentStep != current.CurrentStep ||
		previous.TargetType != current.TargetType ||
		previous.Strategy != current.Strategy ||
		previous.ExecutionMode != current.ExecutionMode
}

func syncStateLabel(state domain.SyncState) string {
	if state == "" {
		return "Unknown"
	}
	return string(state)
}

func normalizeRuntimeStatusForPlanning(deployment domain.Deployment, status domain.DeploymentStatus) domain.DeploymentStatus {
	if status.ObservedGeneration == 0 && status.Sync.State == domain.SyncStateSynced && status.Health.Overall == domain.HealthHealthy {
		status.ObservedGeneration = deployment.Generation
	}
	return status
}

func normalizeRuntimeStatus(deployment domain.Deployment, status domain.DeploymentStatus, plan reconcile.Plan) domain.DeploymentStatus {
	currentIndex := currentRolloutStepIndex(plan)
	completed := completedRolloutStep(plan, currentIndex)
	syncEmpty := status.Sync == (domain.Sync{})
	healthEmpty := status.Health == (domain.Health{})

	switch {
	case completed:
		if syncEmpty {
			status.Sync = domain.Sync{State: domain.SyncStateSynced, Reason: controllerMarkedSyncedReason}
		}
		if healthEmpty {
			status.Health = domain.Health{Overall: domain.HealthHealthy, Summary: controllerHealthySyncedSummary}
		}
		if status.ObservedGeneration == 0 {
			status.ObservedGeneration = deployment.Generation
		}
	case currentIndex >= 0:
		if syncEmpty {
			status.Sync = domain.Sync{State: domain.SyncStateReconciling, Reason: controllerRuntimeSyncReason(plan, currentIndex)}
		}
		if healthEmpty {
			status.Health = domain.Health{Overall: domain.HealthProgressing, Summary: rolloutSummaryForStep(deployment, status, plan, currentIndex)}
		}
	default:
		if syncEmpty {
			status.Sync = domain.Sync{State: domain.SyncStatePaused, Reason: "Controller paused rollout"}
		}
		if healthEmpty {
			status.Health = domain.Health{Overall: domain.HealthUnknown, Summary: "Rollout pending while deployment is paused"}
		}
	}

	return status
}

func applyControllerManagedDriftStatus(status domain.DeploymentStatus, plan reconcile.Plan) domain.DeploymentStatus {
	if len(status.ManagedDrift) == 0 {
		return status
	}
	currentIndex := currentRolloutStepIndex(plan)
	if !completedRolloutStep(plan, currentIndex) {
		return status
	}
	controllerOwnedSync := status.Sync == (domain.Sync{State: domain.SyncStateSynced, Reason: controllerMarkedSyncedReason})
	if controllerOwnedSync {
		status.Sync = domain.Sync{State: domain.SyncStateDrifted, Reason: controllerDriftedSyncReason}
		if status.Health == (domain.Health{Overall: domain.HealthHealthy, Summary: controllerHealthySyncedSummary}) {
			status.Health = domain.Health{Overall: domain.HealthHealthy, Summary: controllerHealthyDriftSummary}
		}
	}
	if status.Rollout != nil && (status.Rollout.Summary == rolloutHealthySyncedSummary || status.Rollout.Summary == rolloutSyncedSummary) {
		status.Rollout.Summary = rolloutManagedDriftSummary
	}
	return status
}

func controllerRuntimeSyncReason(plan reconcile.Plan, currentIndex int) string {
	if currentIndex < 0 || currentIndex >= len(plan.Steps) {
		return "Controller paused rollout"
	}
	if plan.Steps[currentIndex].Kind == reconcile.StepMarkSynced {
		return controllerMarkedSyncedReason
	}
	return fmt.Sprintf("Controller progressing rollout at %s", plan.Steps[currentIndex].Kind)
}

func deriveRolloutSummary(deployment domain.Deployment, status domain.DeploymentStatus, plan reconcile.Plan) *domain.RolloutSummary {
	steps := make([]domain.RolloutStepSummary, 0, len(plan.Steps))
	currentIndex := currentRolloutStepIndex(plan)
	completed := completedRolloutStep(plan, currentIndex)
	currentStep := ""
	if currentIndex >= 0 && !completed {
		currentStep = string(plan.Steps[currentIndex].Kind)
	}
	for index, step := range plan.Steps {
		steps = append(steps, domain.RolloutStepSummary{
			Kind:    string(step.Kind),
			Detail:  rolloutStepDetail(deployment, plan, step.Kind),
			Current: index == currentIndex && !completed,
			Status:  rolloutStepStatus(index, currentIndex, completed),
		})
	}

	return &domain.RolloutSummary{
		DesiredGeneration:  deployment.Generation,
		ObservedGeneration: status.ObservedGeneration,
		Phase:              rolloutPhaseForStep(plan, currentIndex),
		CurrentStep:        currentStep,
		Summary:            rolloutSummaryForStep(deployment, status, plan, currentIndex),
		TargetType:         deployment.TargetType,
		Strategy:           deployment.Strategy,
		ExecutionMode:      deployment.ExecutionMode,
		Steps:              steps,
		Execution:          deriveRolloutExecutionDetails(deployment),
	}
}

func currentRolloutStepIndex(plan reconcile.Plan) int {
	for index, step := range plan.Steps {
		if step.Kind == reconcile.StepResolvePlacement || step.Kind == reconcile.StepObserveCurrentState {
			continue
		}
		return index
	}
	return -1
}

func completedRolloutStep(plan reconcile.Plan, currentIndex int) bool {
	if currentIndex < 0 || currentIndex >= len(plan.Steps) {
		return false
	}
	return plan.Steps[currentIndex].Kind == reconcile.StepMarkSynced
}

func rolloutStepStatus(index int, currentIndex int, completed bool) domain.RolloutStepStatus {
	if completed {
		return domain.RolloutStepStatusCompleted
	}
	if currentIndex >= 0 {
		switch {
		case index < currentIndex:
			return domain.RolloutStepStatusCompleted
		case index == currentIndex:
			return domain.RolloutStepStatusCurrent
		default:
			return domain.RolloutStepStatusPending
		}
	}
	return domain.RolloutStepStatusPending
}

func rolloutStepDetail(deployment domain.Deployment, plan reconcile.Plan, kind reconcile.StepKind) string {
	switch kind {
	case reconcile.StepCreateCandidateCapacity:
		if detail := weightedTargetGroupCandidateDetail(deployment); detail != "" {
			return detail
		}
		return "Create a fresh candidate Auto Scaling Group bootstrapped from userdata before attaching it to the candidate target group."
	case reconcile.StepShiftTrafficCanary:
		if plan.ContainsStep(reconcile.StepCreateCodeDeployDeployment) {
			return codeDeployTrafficShiftDetail(deployment, "Shift a small canary slice of ECS service traffic to the replacement task set through CodeDeploy before the full cutover.")
		}
		return "Shift a small canary slice of production traffic to the candidate target group before full promotion."
	case reconcile.StepShiftTrafficLinear:
		if plan.ContainsStep(reconcile.StepCreateCodeDeployDeployment) {
			return codeDeployTrafficShiftDetail(deployment, "Advance ECS traffic in linear increments through CodeDeploy until the replacement task set serves all traffic.")
		}
		return "Advance ALB traffic in linear increments toward the candidate target group until promotion is safe."
	case reconcile.StepShiftTrafficFull:
		if plan.ContainsStep(reconcile.StepCreateCodeDeployDeployment) {
			return codeDeployTrafficShiftDetail(deployment, "Shift all ECS service traffic to the replacement task set through CodeDeploy in a single blue/green cutover.")
		}
		return "Shift all production traffic from the active target group to the candidate target group in one blue/green cutover."
	case reconcile.StepUpdateECSService:
		return ecsRollingUpdateDetail(deployment)
	case reconcile.StepCreateCodeDeployDeployment:
		if deployment.ExecutionMode == "ecsCodeDeploy" {
			return "Create a CodeDeploy deployment to coordinate blue/green ECS traffic shifting for the replacement task set."
		}
	case reconcile.StepVerifyServiceStability:
		if plan.ContainsStep(reconcile.StepCreateCodeDeployDeployment) {
			return "Wait for the ECS service to stabilize after CodeDeploy finishes shifting traffic to the replacement task set."
		}
		return "Wait for the ECS service to report a stable native rolling deployment after replacing tasks."
	}
	return ""
}

func deriveRolloutExecutionDetails(deployment domain.Deployment) []domain.RolloutExecutionDetail {
	switch deployment.ExecutionMode {
	case "asgWeightedTargetGroups":
		return []domain.RolloutExecutionDetail{{
			Resource:    "candidateAutoScalingGroup",
			Name:        deployment.Name + "-candidate",
			Description: weightedTargetGroupCandidateExecutionDescription(deployment),
		}, {
			Resource:    "activeTargetGroup",
			Name:        deployment.Name + "-active",
			Description: "Currently active target group receiving production traffic",
		}, {
			Resource:    "candidateTargetGroup",
			Name:        deployment.Name + "-candidate",
			Description: "Candidate target group Tenet will register and warm before traffic shifting",
		}, {
			Resource:    "productionListenerRule",
			Name:        deployment.Name + "-prod-listener",
			Description: "Production listener rule operators inspect while Tenet updates weighted forwarding during rollout",
		}, {
			Resource:    "trafficShiftPlan",
			Name:        deployment.Name + "-weighted-routing",
			Description: weightedTargetGroupTrafficShiftDescription(deployment),
		}, {
			Resource:    "userDataBootstrapSource",
			Name:        userDataBootstrapSource(deployment.UserData),
			Description: "Normalized userdata bootstrap source for candidate EC2 capacity",
		}}
	case "asgInstanceRefresh":
		description := "Service Auto Scaling Group Tenet refreshes in place"
		if batchSize := instanceRefreshBatchSize(deployment); batchSize > 0 {
			description += fmt.Sprintf(" with rolling-update batch size %d", batchSize)
		}
		return []domain.RolloutExecutionDetail{{
			Resource:    "serviceAutoScalingGroup",
			Name:        deployment.Name + "-service",
			Description: description,
		}, {
			Resource:    "userDataBootstrapSource",
			Name:        userDataBootstrapSource(deployment.UserData),
			Description: "Normalized userdata bootstrap source for refreshed EC2 instances",
		}}
	case "ecsCodeDeploy":
		return []domain.RolloutExecutionDetail{{
			Resource:    "ecsService",
			Name:        deployment.Name + "-service",
			Description: codeDeployExecutionDescription(deployment, "ECS service Tenet updates during CodeDeploy traffic shifting"),
		}, {
			Resource:    "codeDeployApplication",
			Name:        deployment.Name + "-codedeploy-app",
			Description: codeDeployExecutionDescription(deployment, "CodeDeploy application coordinating ECS traffic shifting"),
		}, {
			Resource:    "codeDeployDeploymentGroup",
			Name:        deployment.Name + "-codedeploy-group",
			Description: codeDeployExecutionDescription(deployment, "CodeDeploy deployment group Tenet targets for ECS rollout"),
		}, {
			Resource:    "productionListener",
			Name:        deployment.Name + "-prod-listener",
			Description: codeDeployExecutionDescription(deployment, "Production listener CodeDeploy shifts during ECS traffic shifting"),
		}, {
			Resource:    "testListener",
			Name:        deployment.Name + "-test-listener",
			Description: codeDeployExecutionDescription(deployment, "Test listener CodeDeploy uses to validate the replacement ECS task set before promotion"),
		}, {
			Resource:    "blueTaskSetTargetGroup",
			Name:        deployment.Name + "-blue",
			Description: codeDeployExecutionDescription(deployment, "Blue task set target group CodeDeploy treats as the live ECS traffic destination"),
		}, {
			Resource:    "greenTaskSetTargetGroup",
			Name:        deployment.Name + "-green",
			Description: codeDeployExecutionDescription(deployment, "Green task set target group CodeDeploy warms before shifting ECS traffic"),
		}}
	case "ecsRolling":
		description := "ECS service Tenet updates with native rolling deployment"
		if qualifiers := rolloutStrategyQualifiers(deployment.RolloutConfig.Strategy); len(qualifiers) > 0 {
			description += " while respecting " + strings.Join(qualifiers, " and ")
		}
		return []domain.RolloutExecutionDetail{{
			Resource:    "ecsService",
			Name:        deployment.Name + "-service",
			Description: description,
		}}
	default:
		return nil
	}
}

func userDataBootstrapSource(userData domain.DeploymentUserData) string {
	if userData.Type == "" && userData.Path == "" {
		return "unspecified"
	}
	if userData.Path == "" {
		return userData.Type
	}
	if userData.Type == "" {
		return userData.Path
	}
	return userData.Type + ":" + userData.Path
}

func weightedTargetGroupCandidateDetail(deployment domain.Deployment) string {
	capacity := deployment.RolloutConfig.Capacity
	warmup := deployment.RolloutConfig.Strategy.WarmupSeconds
	bootstrap := userDataBootstrapSource(deployment.UserData)
	hasCapacity := capacity.Desired > 1 || capacity.Min > 1 || capacity.Max > 1
	if warmup == 0 && !hasCapacity {
		return ""
	}

	parts := []string{"Create a fresh candidate Auto Scaling Group"}
	if bootstrap != "unspecified" {
		parts = append(parts, fmt.Sprintf("bootstrapped from normalized userdata source %s", bootstrap))
	} else {
		parts = append(parts, "bootstrapped from userdata")
	}
	if hasCapacity {
		parts = append(parts, fmt.Sprintf("with desired %d, min %d, and max %d instances", capacity.Desired, capacity.Min, capacity.Max))
	}
	detail := strings.Join(parts, " ")
	if warmup > 0 {
		detail += fmt.Sprintf(", then complete a warmup %ds", warmup)
	}
	detail += " before attaching it to the candidate target group."
	return detail
}

func weightedTargetGroupCandidateExecutionDescription(deployment domain.Deployment) string {
	capacity := deployment.RolloutConfig.Capacity
	description := fmt.Sprintf(
		"Candidate Auto Scaling Group Tenet creates from normalized userdata source %s with desired %d, min %d, and max %d instances",
		userDataBootstrapSource(deployment.UserData),
		capacity.Desired,
		capacity.Min,
		capacity.Max,
	)
	if warmup := deployment.RolloutConfig.Strategy.WarmupSeconds; warmup > 0 {
		description += fmt.Sprintf(" and warmup %ds", warmup)
	}
	return description + " for weighted target group rollout"
}

func weightedTargetGroupTrafficShiftDescription(deployment domain.Deployment) string {
	var description string
	switch deployment.Strategy {
	case "canary":
		description = "Weighted routing plan Tenet follows to shift a canary slice of production traffic from the active target group to the candidate target group before full promotion"
	case "linear":
		description = "Weighted routing plan Tenet follows to advance production traffic from the active target group to the candidate target group in linear increments"
	default:
		description = "Weighted routing plan Tenet follows to cut over all production traffic from the active target group to the candidate target group in a single blue/green promotion"
	}
	if qualifiers := rolloutStrategyQualifiers(deployment.RolloutConfig.Strategy); len(qualifiers) > 0 {
		description += " while respecting " + strings.Join(qualifiers, " and ")
	}
	return description
}

func codeDeployExecutionDescription(deployment domain.Deployment, base string) string {
	style := deployment.Strategy
	if style == "" {
		style = "blueGreen"
	}
	description := fmt.Sprintf("%s (style: %s", base, style)
	if qualifiers := rolloutStrategyQualifiers(deployment.RolloutConfig.Strategy); len(qualifiers) > 0 {
		description += "; qualifiers: " + strings.Join(qualifiers, " and ")
	}
	return description + ")"
}

func codeDeployTrafficShiftDetail(deployment domain.Deployment, base string) string {
	qualifiers := rolloutStrategyQualifiers(deployment.RolloutConfig.Strategy)
	if len(qualifiers) == 0 {
		return base
	}
	return base[:len(base)-1] + " while respecting " + strings.Join(qualifiers, " and ") + "."
}

func ecsRollingUpdateDetail(deployment domain.Deployment) string {
	base := "Update the ECS service to start a native rolling deployment that replaces running tasks in place."
	qualifiers := rolloutStrategyQualifiers(deployment.RolloutConfig.Strategy)
	if len(qualifiers) == 0 {
		return base
	}
	return base[:len(base)-1] + " while respecting " + strings.Join(qualifiers, " and ") + "."
}

func rolloutStrategyQualifiers(strategy domain.DeploymentRolloutStrategy) []string {
	qualifiers := make([]string, 0, 2)
	if strategy.MinHealthyPercent > 0 {
		qualifiers = append(qualifiers, fmt.Sprintf("min healthy %d%%", strategy.MinHealthyPercent))
	}
	if strategy.WarmupSeconds > 0 {
		qualifiers = append(qualifiers, fmt.Sprintf("warmup %ds", strategy.WarmupSeconds))
	}
	return qualifiers
}

func instanceRefreshBatchSize(deployment domain.Deployment) int {
	for _, stack := range deployment.RolloutConfig.Stacks {
		if stack.RollingUpdateInstanceCount > 0 {
			return stack.RollingUpdateInstanceCount
		}
	}
	return 0
}

func rolloutPhaseForStep(plan reconcile.Plan, currentIndex int) domain.RolloutPhase {
	if currentIndex < 0 || len(plan.Steps) == 0 {
		return domain.RolloutPhasePending
	}

	switch plan.Steps[currentIndex].Kind {
	case reconcile.StepPlanCandidateGeneration:
		return domain.RolloutPhasePlanning
	case reconcile.StepReconcileCurrentState:
		return domain.RolloutPhaseProvisioning
	case reconcile.StepCreateCandidateCapacity, reconcile.StepStartInstanceRefresh, reconcile.StepCreateCodeDeployDeployment, reconcile.StepUpdateECSService:
		return domain.RolloutPhaseProvisioning
	case reconcile.StepVerifyCandidateCapacity:
		return domain.RolloutPhaseHealthChecking
	case reconcile.StepShiftTrafficCanary, reconcile.StepShiftTrafficLinear, reconcile.StepShiftTrafficFull, reconcile.StepPromoteCandidateCapacity:
		return domain.RolloutPhasePromoting
	case reconcile.StepVerifyReplacementCapacity, reconcile.StepVerifyServiceStability:
		return domain.RolloutPhaseStabilizing
	case reconcile.StepMarkSynced:
		return domain.RolloutPhaseCompleted
	case reconcile.StepAwaitFreshDesired:
		return domain.RolloutPhaseAccepted
	default:
		return domain.RolloutPhaseAccepted
	}
}

func rolloutSummaryForStep(deployment domain.Deployment, status domain.DeploymentStatus, plan reconcile.Plan, currentIndex int) string {
	if currentIndex < 0 || len(plan.Steps) == 0 {
		return "Rollout pending"
	}

	switch plan.Steps[currentIndex].Kind {
	case reconcile.StepPlanCandidateGeneration:
		return "Planning candidate generation"
	case reconcile.StepReconcileCurrentState:
		return "Reconciling current state"
	case reconcile.StepCreateCandidateCapacity:
		return "Creating candidate capacity for weighted target groups"
	case reconcile.StepVerifyCandidateCapacity:
		return "Verifying candidate capacity before traffic shifting"
	case reconcile.StepShiftTrafficCanary:
		return "Shifting canary traffic through weighted target groups"
	case reconcile.StepShiftTrafficLinear:
		return trafficShiftSummary(plan)
	case reconcile.StepShiftTrafficFull:
		return trafficShiftSummary(plan)
	case reconcile.StepPromoteCandidateCapacity:
		return "Promoting candidate capacity through weighted target groups"
	case reconcile.StepStartInstanceRefresh:
		if batchSize := instanceRefreshBatchSize(deployment); batchSize > 0 {
			return fmt.Sprintf("Starting EC2 instance refresh with batch size %d", batchSize)
		}
		return "Starting EC2 instance refresh"
	case reconcile.StepVerifyReplacementCapacity:
		return "Verifying replacement capacity during instance refresh"
	case reconcile.StepUpdateECSService:
		if qualifiers := rolloutStrategyQualifiers(deployment.RolloutConfig.Strategy); len(qualifiers) > 0 {
			return "Updating ECS service deployment while respecting " + strings.Join(qualifiers, " and ")
		}
		return "Updating ECS service deployment"
	case reconcile.StepCreateCodeDeployDeployment:
		return "Creating CodeDeploy traffic shift deployment"
	case reconcile.StepVerifyServiceStability:
		if plan.ContainsStep(reconcile.StepCreateCodeDeployDeployment) {
			return "Stabilizing ECS service after CodeDeploy traffic shift"
		}
		return "Stabilizing ECS service after rolling update"
	case reconcile.StepMarkSynced:
		if status.Sync.State == domain.SyncStateSynced && status.Health.Overall == domain.HealthHealthy {
			return rolloutHealthySyncedSummary
		}
		return rolloutSyncedSummary
	case reconcile.StepAwaitFreshDesired:
		return "Waiting for fresher desired state"
	default:
		return "Observing current state"
	}
}

func trafficShiftSummary(plan reconcile.Plan) string {
	if plan.ContainsStep(reconcile.StepCreateCodeDeployDeployment) {
		return "Shifting traffic with ECS CodeDeploy"
	}
	return "Shifting traffic through weighted target groups"
}

func stepSummary(plan reconcile.Plan) string {
	if len(plan.Steps) == 0 {
		return "none"
	}
	steps := make([]string, 0, len(plan.Steps))
	for _, step := range plan.Steps {
		steps = append(steps, string(step.Kind))
	}
	return strings.Join(steps, ",")
}
