package auditlog

import (
	"context"
	"fmt"
	"sort"

	"github.com/DevopsArtFactory/tenet/internal/applyrequest"
	"github.com/DevopsArtFactory/tenet/internal/catalog"
	"github.com/DevopsArtFactory/tenet/internal/planpreview"
	"github.com/DevopsArtFactory/tenet/internal/requestactor"
)

// Action identifies one supported audit timeline activity.
type Action string

const (
	ActionPlanCreated    Action = "plan.created"
	ActionApplyRequested Action = "apply.requested"
)

// Query captures optional audit timeline filters.
type Query struct {
	Limit          int    `json:"limit,omitempty"`
	DeploymentName string `json:"deploymentName,omitempty"`
	Action         Action `json:"action,omitempty"`
}

// Actor makes missing operator identity explicit instead of inventing auth data.
type Actor = requestactor.Actor

// Metadata captures action-specific audit details from immutable source records.
type Metadata struct {
	Status            string   `json:"status,omitempty"`
	PlanStatus        string   `json:"planStatus,omitempty"`
	SourceFingerprint string   `json:"sourceFingerprint,omitempty"`
	RiskLevel         string   `json:"riskLevel,omitempty"`
	ApprovalRequired  *bool    `json:"approvalRequired,omitempty"`
	Error             string   `json:"error,omitempty"`
	Details           []string `json:"details,omitempty"`
	Errors            []string `json:"errors,omitempty"`
}

// Source captures the local immutable record that produced an audit timeline entry.
type Source struct {
	Kind        string `json:"kind"`
	Store       string `json:"store"`
	RecordRoute string `json:"recordRoute"`
	SourceRoute string `json:"sourceRoute,omitempty"`
}

// Entry is one local-friendly audit timeline row projected from persisted records.
type Entry struct {
	Timestamp      string   `json:"timestamp"`
	Action         string   `json:"action"`
	DeploymentName string   `json:"deploymentName,omitempty"`
	AppRef         string   `json:"appRef,omitempty"`
	EnvironmentRef string   `json:"environmentRef,omitempty"`
	PlanID         string   `json:"planId,omitempty"`
	ApplyID        string   `json:"applyId,omitempty"`
	Actor          Actor    `json:"actor"`
	Metadata       Metadata `json:"metadata"`
	Source         Source   `json:"source"`
}

// Result is the read-only audit timeline payload.
type Result struct {
	Entries []Entry `json:"entries"`
}

// Service projects immutable plan preview and apply request records into an audit timeline.
type Service struct {
	ListPlanPreviews  func() ([]planpreview.Record, error)
	ListApplyRequests func() ([]applyrequest.Record, error)
	ListDeployments   func(context.Context) ([]catalog.DeploymentView, error)
}

// List returns a newest-first local audit timeline derived from persisted records.
func (s Service) List(ctx context.Context, query Query) (Result, error) {
	if s.ListPlanPreviews == nil {
		return Result{}, fmt.Errorf("audit log plan preview provider is required")
	}
	if s.ListApplyRequests == nil {
		return Result{}, fmt.Errorf("audit log apply request provider is required")
	}
	if query.Limit < 0 {
		return Result{}, fmt.Errorf("audit log limit must be greater than zero")
	}

	planRecords, err := s.ListPlanPreviews()
	if err != nil {
		return Result{}, err
	}
	applyRecords, err := s.ListApplyRequests()
	if err != nil {
		return Result{}, err
	}
	deployments, err := s.loadDeployments(ctx)
	if err != nil {
		return Result{}, err
	}

	entries := make([]Entry, 0, len(planRecords)+len(applyRecords))
	for _, record := range planRecords {
		if query.DeploymentName != "" && record.DeploymentName != query.DeploymentName {
			continue
		}
		if query.Action != "" && query.Action != ActionPlanCreated {
			continue
		}
		entries = append(entries, projectPlan(record, deployments[record.DeploymentName]))
	}
	for _, record := range applyRecords {
		if query.DeploymentName != "" && record.DeploymentName != query.DeploymentName {
			continue
		}
		if query.Action != "" && query.Action != ActionApplyRequested {
			continue
		}
		entries = append(entries, projectApply(record, deployments[record.DeploymentName]))
	}

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Timestamp != entries[j].Timestamp {
			return entries[i].Timestamp > entries[j].Timestamp
		}
		if entries[i].Action != entries[j].Action {
			return entries[i].Action > entries[j].Action
		}
		return entries[i].stableID() > entries[j].stableID()
	})

	if query.Limit > 0 && query.Limit < len(entries) {
		entries = entries[:query.Limit]
	}
	return Result{Entries: entries}, nil
}

func (s Service) loadDeployments(ctx context.Context) (map[string]catalog.DeploymentView, error) {
	if s.ListDeployments == nil {
		return nil, nil
	}
	deployments, err := s.ListDeployments(ctx)
	if err != nil {
		return nil, err
	}
	index := make(map[string]catalog.DeploymentView, len(deployments))
	for _, deployment := range deployments {
		index[deployment.Name] = deployment
	}
	return index, nil
}

func projectPlan(record planpreview.Record, deployment catalog.DeploymentView) Entry {
	approvalRequired := record.Approval.Required
	return Entry{
		Timestamp:      record.CreatedAt,
		Action:         "plan.created",
		DeploymentName: record.DeploymentName,
		AppRef:         deployment.AppRef,
		EnvironmentRef: deployment.EnvironmentRef,
		PlanID:         record.ID,
		Actor:          requestactor.Normalize(record.Actor),
		Metadata: Metadata{
			Status:            string(record.Status),
			SourceFingerprint: record.Source.Fingerprint,
			RiskLevel:         record.Risk.Level,
			ApprovalRequired:  &approvalRequired,
			Errors:            append([]string(nil), record.Errors...),
		},
		Source: Source{
			Kind:        "planPreview",
			Store:       "planPreviewStore",
			RecordRoute: "/v1/plans/" + record.ID,
			SourceRoute: record.Source.Route,
		},
	}
}

func projectApply(record applyrequest.Record, deployment catalog.DeploymentView) Entry {
	return Entry{
		Timestamp:      record.CreatedAt,
		Action:         "apply.requested",
		DeploymentName: record.DeploymentName,
		AppRef:         deployment.AppRef,
		EnvironmentRef: deployment.EnvironmentRef,
		PlanID:         record.PlanID,
		ApplyID:        record.ID,
		Actor:          requestactor.Normalize(record.Actor),
		Metadata: Metadata{
			Status:            string(record.Status),
			PlanStatus:        record.Source.PlanStatus,
			SourceFingerprint: record.Source.Fingerprint,
			Error:             record.Error,
			Details:           append([]string(nil), record.Details...),
		},
		Source: Source{
			Kind:        "applyRequest",
			Store:       "applyRequestStore",
			RecordRoute: "/v1/applies/" + record.ID,
			SourceRoute: record.Source.Route,
		},
	}
}

func (e Entry) stableID() string {
	if e.ApplyID != "" {
		return e.ApplyID
	}
	return e.PlanID
}

// ValidAction reports whether the provided query value is one of the supported audit actions.
func ValidAction(value string) bool {
	switch Action(value) {
	case ActionPlanCreated, ActionApplyRequested:
		return true
	default:
		return false
	}
}
