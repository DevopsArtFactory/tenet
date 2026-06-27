package controllerrefreshaudit

import (
	"context"
	"fmt"

	"github.com/DevopsArtFactory/tenet/internal/requestactor"
)

var ErrNotFound = fmt.Errorf("controller refresh audit record not found")

// Record captures the safe, non-secret evidence for one accepted manual controller refresh request.
type Record struct {
	ID             string             `json:"id"`
	Route          string             `json:"route"`
	Status         string             `json:"status"`
	Result         string             `json:"result"`
	RequestedAt    string             `json:"requestedAt"`
	Actor          requestactor.Actor `json:"actor"`
	RequestID      string             `json:"requestId,omitempty"`
	DeliveryID     string             `json:"deliveryId,omitempty"`
	Backend        string             `json:"backend,omitempty"`
	Channel        string             `json:"channel,omitempty"`
	NoopReason     string             `json:"noopReason,omitempty"`
	FailureReason  string             `json:"failureReason,omitempty"`
	HTTPStatusCode int                `json:"httpStatusCode,omitempty"`
}

// Store persists immutable manual controller refresh audit records.
type Store interface {
	List(context.Context) ([]Record, error)
	Save(context.Context, Record) error
}
