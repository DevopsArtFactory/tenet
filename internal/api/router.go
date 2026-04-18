package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
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

// Options configures the API router.
type Options struct {
	BuildInfo                            buildinfo.Info
	Logger                               *slog.Logger
	RequestTimeout                       time.Duration
	GitHubWebhookSecret                  string
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
		if options.OverviewApplications == nil {
			http.Error(w, "overview applications unavailable", http.StatusServiceUnavailable)
			return
		}
		applications, err := options.OverviewApplications(r.Context())
		if err != nil {
			respondReadProviderError(w, err, "overview applications unavailable")
			return
		}
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
		if options.OverviewAccounts == nil {
			http.Error(w, "overview accounts unavailable", http.StatusServiceUnavailable)
			return
		}
		accounts, err := options.OverviewAccounts(r.Context())
		if err != nil {
			respondReadProviderError(w, err, "overview accounts unavailable")
			return
		}
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
			detail, err := options.OverviewAccount(r.Context(), segments[0])
			if err != nil {
				respondReadLookupError(w, err, "overview account unavailable")
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(detail)
		case len(segments) == 5 && segments[0] != "" && segments[1] == "projects" && segments[2] != "" && segments[3] == "environments" && segments[4] != "":
			if options.OverviewProjectEnvironment == nil {
				http.Error(w, "overview project environment unavailable", http.StatusServiceUnavailable)
				return
			}
			detail, err := options.OverviewProjectEnvironment(r.Context(), segments[0], segments[2], segments[4])
			if err != nil {
				respondReadLookupError(w, err, "overview project environment unavailable")
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
		result, err := options.AuditLogs(r.Context(), query)
		if err != nil {
			respondInternalError(w)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(result)
	})
	mux.HandleFunc("/v1/plans/deployments/", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodPost) {
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
		record, err := options.PlanPreview(r.Context(), id)
		if err != nil {
			respondLookupError(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(record)
	})
	mux.HandleFunc("/v1/applies/plans/", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodPost) {
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
			record, err := options.ApplyRequest(r.Context(), segments[0])
			if err != nil {
				respondLookupError(w, err)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(record)
		case len(segments) == 2 && segments[0] != "" && segments[1] == "transitions":
			if !requireMethod(w, r, http.MethodPost) {
				return
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
		if options.DeploymentStatuses == nil {
			http.Error(w, "deployment statuses unavailable", http.StatusServiceUnavailable)
			return
		}
		snapshots, err := options.DeploymentStatuses(r.Context())
		if err != nil {
			respondReadProviderError(w, err, "deployment statuses unavailable")
			return
		}
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
	handler = requestActorMiddleware(handler)
	handler = requestIDMiddleware(handler)
	return handler
}
