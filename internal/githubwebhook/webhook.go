package githubwebhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const (
	HeaderDeliveryID = "X-GitHub-Delivery"
	HeaderEvent      = "X-GitHub-Event"
	HeaderSignature  = "X-Hub-Signature-256"
)

const (
	StatusAccepted = "accepted"
	StatusIgnored  = "ignored"
)

type Request struct {
	Secret string
	Header http.Header
	Body   io.Reader
}

type Response struct {
	DeliveryID   string        `json:"deliveryId"`
	Event        string        `json:"event"`
	Status       string        `json:"status"`
	Processed    bool          `json:"processed"`
	Reason       string        `json:"reason"`
	Repository   string        `json:"repository,omitempty"`
	Ref          string        `json:"ref,omitempty"`
	Before       string        `json:"before,omitempty"`
	After        string        `json:"after,omitempty"`
	HeadCommit   *Commit       `json:"headCommit,omitempty"`
	ChangedFiles *ChangedFiles `json:"changedFiles,omitempty"`
}

type Commit struct {
	ID        string `json:"id"`
	Message   string `json:"message,omitempty"`
	Timestamp string `json:"timestamp,omitempty"`
}

type ChangedFiles struct {
	Added    []string `json:"added,omitempty"`
	Modified []string `json:"modified,omitempty"`
	Removed  []string `json:"removed,omitempty"`
	All      []string `json:"all,omitempty"`
}

type ValidationError struct {
	Message string
}

func (e *ValidationError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

func Decode(ctx context.Context, req Request) (Response, error) {
	if err := contextErr(ctx); err != nil {
		return Response{}, err
	}

	deliveryID := strings.TrimSpace(req.Header.Get(HeaderDeliveryID))
	if deliveryID == "" {
		return Response{}, &ValidationError{Message: HeaderDeliveryID + " header is required"}
	}

	event := strings.TrimSpace(req.Header.Get(HeaderEvent))
	if event == "" {
		return Response{}, &ValidationError{Message: HeaderEvent + " header is required"}
	}

	signature := strings.TrimSpace(req.Header.Get(HeaderSignature))
	if signature == "" {
		return Response{}, &ValidationError{Message: HeaderSignature + " header is required"}
	}

	secret := strings.TrimSpace(req.Secret)
	if secret == "" {
		return Response{}, &ValidationError{Message: "webhook secret is required"}
	}

	body, err := io.ReadAll(req.Body)
	if err != nil {
		return Response{}, &ValidationError{Message: fmt.Sprintf("read webhook body: %v", err)}
	}
	if len(strings.TrimSpace(string(body))) == 0 {
		return Response{}, &ValidationError{Message: "webhook body is required"}
	}
	if err := contextErr(ctx); err != nil {
		return Response{}, err
	}
	if err := verifySignature(secret, signature, body); err != nil {
		return Response{}, err
	}

	if event != "push" {
		if err := decodeSingleJSONObject(body, &map[string]json.RawMessage{}); err != nil {
			return Response{}, err
		}
		return Response{
			DeliveryID: deliveryID,
			Event:      event,
			Status:     StatusIgnored,
			Processed:  false,
			Reason:     fmt.Sprintf("github event %q is ignored because reconcile wiring is not implemented in this slice", event),
		}, nil
	}

	var payload pushPayload
	if err := decodeSingleJSONObject(body, &payload); err != nil {
		return Response{}, err
	}
	if err := payload.validate(); err != nil {
		return Response{}, err
	}

	changedFiles := collectChangedFiles(payload.Commits)
	response := Response{
		DeliveryID: deliveryID,
		Event:      event,
		Status:     StatusAccepted,
		Processed:  false,
		Reason:     "github push event validated, but reconcile wiring is not implemented in this slice",
		Repository: payload.Repository.FullName,
		Ref:        payload.Ref,
		Before:     payload.Before,
		After:      payload.After,
		ChangedFiles: &ChangedFiles{
			Added:    changedFiles.Added,
			Modified: changedFiles.Modified,
			Removed:  changedFiles.Removed,
			All:      changedFiles.All,
		},
	}
	if payload.HeadCommit.ID != "" || payload.HeadCommit.Message != "" || payload.HeadCommit.Timestamp != "" {
		response.HeadCommit = &Commit{
			ID:        payload.HeadCommit.ID,
			Message:   payload.HeadCommit.Message,
			Timestamp: payload.HeadCommit.Timestamp,
		}
	}
	return response, nil
}

type pushPayload struct {
	Ref        string         `json:"ref"`
	Before     string         `json:"before"`
	After      string         `json:"after"`
	Repository pushRepository `json:"repository"`
	HeadCommit pushCommit     `json:"head_commit"`
	Commits    []pushCommit   `json:"commits"`
}

type pushRepository struct {
	FullName string `json:"full_name"`
}

type pushCommit struct {
	ID        string   `json:"id"`
	Message   string   `json:"message"`
	Timestamp string   `json:"timestamp"`
	Added     []string `json:"added"`
	Modified  []string `json:"modified"`
	Removed   []string `json:"removed"`
}

func (p pushPayload) validate() error {
	switch {
	case strings.TrimSpace(p.Repository.FullName) == "":
		return &ValidationError{Message: "push payload repository.full_name is required"}
	case strings.TrimSpace(p.Ref) == "":
		return &ValidationError{Message: "push payload ref is required"}
	case strings.TrimSpace(p.Before) == "":
		return &ValidationError{Message: "push payload before is required"}
	case strings.TrimSpace(p.After) == "":
		return &ValidationError{Message: "push payload after is required"}
	default:
		return nil
	}
}

func verifySignature(secret string, raw string, body []byte) error {
	if !strings.HasPrefix(raw, "sha256=") {
		return &ValidationError{Message: HeaderSignature + " header must use sha256"}
	}
	decoded, err := hex.DecodeString(strings.TrimPrefix(raw, "sha256="))
	if err != nil {
		return &ValidationError{Message: HeaderSignature + " header is not valid hex"}
	}

	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)
	expected := mac.Sum(nil)
	if !hmac.Equal(decoded, expected) {
		return &ValidationError{Message: "github webhook signature mismatch"}
	}
	return nil
}

func decodeSingleJSONObject(body []byte, dest any) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	if err := decoder.Decode(dest); err != nil {
		return &ValidationError{Message: fmt.Sprintf("invalid webhook JSON body: %v", err)}
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return &ValidationError{Message: "invalid webhook JSON body: multiple JSON values are not allowed"}
	}
	return nil
}

type changedFilesAccumulator struct {
	Added    []string
	Modified []string
	Removed  []string
	All      []string

	addedSeen    map[string]struct{}
	modifiedSeen map[string]struct{}
	removedSeen  map[string]struct{}
	allSeen      map[string]struct{}
}

func collectChangedFiles(commits []pushCommit) changedFilesAccumulator {
	acc := changedFilesAccumulator{
		addedSeen:    map[string]struct{}{},
		modifiedSeen: map[string]struct{}{},
		removedSeen:  map[string]struct{}{},
		allSeen:      map[string]struct{}{},
	}
	for _, commit := range commits {
		acc.addByCategory(&acc.Added, acc.addedSeen, commit.Added)
		acc.addByCategory(&acc.Modified, acc.modifiedSeen, commit.Modified)
		acc.addByCategory(&acc.Removed, acc.removedSeen, commit.Removed)
	}
	acc.addByCategory(&acc.All, acc.allSeen, acc.Added)
	acc.addByCategory(&acc.All, acc.allSeen, acc.Modified)
	acc.addByCategory(&acc.All, acc.allSeen, acc.Removed)
	return acc
}

func (c *changedFilesAccumulator) addByCategory(target *[]string, seen map[string]struct{}, items []string) {
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		*target = append(*target, item)
	}
}

func contextErr(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	return ctx.Err()
}
