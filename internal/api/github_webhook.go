package api

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/DevopsArtFactory/tenet/internal/controllerrefresh"
	"github.com/DevopsArtFactory/tenet/internal/githubwebhook"
)

var githubWebhookNow = time.Now

func finalizeGitHubWebhookResponse(ctx context.Context, logger *slog.Logger, response githubwebhook.Response, publisher controllerrefresh.Publisher) githubwebhook.Response {
	if response.Status != githubwebhook.StatusAccepted {
		return response
	}

	match := response.DesiredStateSourceMatch
	switch {
	case match == nil || !match.Configured:
		response.Processed = false
		response.Reason = "github push event validated, but controller refresh handoff is unavailable because the desired-state source is not configured"
		return response
	case match.Kind == githubwebhook.DesiredStateSourceKindFilesystem:
		response.Processed = false
		response.Reason = "github push event validated, but controller refresh handoff is unavailable because the active desired-state source uses the filesystem catalog"
		return response
	case !match.Matches:
		response.Processed = false
		response.Reason = "github push event validated, but controller refresh handoff was skipped because this push does not match the active desired-state source"
		return response
	case publisher == nil:
		response.Processed = false
		response.Reason = "github push event validated, but controller refresh handoff is unavailable because this API runtime is not using the redis status backend"
		return response
	}

	requestedAt := githubWebhookNow().UTC().Format(time.RFC3339)
	request := controllerrefresh.Request{
		RequestID:   githubWebhookRequestID(response),
		DeliveryID:  response.DeliveryID,
		Repository:  response.Repository,
		Ref:         response.Ref,
		After:       response.After,
		RequestedAt: requestedAt,
	}
	receipt, err := publisher.Publish(ctx, request)
	if err != nil {
		logger.Error("github webhook controller refresh handoff failed", "delivery_id", response.DeliveryID, "repository", response.Repository, "ref", response.Ref, "error", err)
		response.Processed = false
		response.Reason = "github push event validated, but controller refresh handoff failed because the redis publisher is unavailable"
		return response
	}

	response.Processed = true
	response.Reason = "github push event validated and controller refresh handoff published"
	response.ControllerRefresh = &githubwebhook.ControllerRefresh{
		Backend:     receipt.Backend,
		Channel:     receipt.Channel,
		RequestID:   receipt.RequestID,
		PublishedAt: requestedAt,
	}
	return response
}

func githubWebhookRequestID(response githubwebhook.Response) string {
	if response.After == "" {
		return fmt.Sprintf("github-webhook:%s", response.DeliveryID)
	}
	return fmt.Sprintf("github-webhook:%s:%s", response.DeliveryID, response.After)
}
