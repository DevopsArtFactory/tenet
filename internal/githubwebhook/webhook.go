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
	"path"
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

const (
	DesiredStateSourceKindNone       = ""
	DesiredStateSourceKindFilesystem = "filesystem"
	DesiredStateSourceKindGitHub     = "github"
)

const (
	DesiredStateSourceMatchStatusNotConfigured      = "not_configured"
	DesiredStateSourceMatchStatusFilesystemSource   = "filesystem_source"
	DesiredStateSourceMatchStatusRepositoryMismatch = "repository_mismatch"
	DesiredStateSourceMatchStatusRefMismatch        = "ref_mismatch"
	DesiredStateSourceMatchStatusPathUntouched      = "path_untouched"
	DesiredStateSourceMatchStatusMatched            = "matched"
)

type Request struct {
	Secret             string
	Header             http.Header
	Body               io.Reader
	DesiredStateSource DesiredStateSourceConfig
}

type Response struct {
	DeliveryID              string                   `json:"deliveryId"`
	Event                   string                   `json:"event"`
	Status                  string                   `json:"status"`
	Processed               bool                     `json:"processed"`
	Reason                  string                   `json:"reason"`
	Repository              string                   `json:"repository,omitempty"`
	Ref                     string                   `json:"ref,omitempty"`
	Before                  string                   `json:"before,omitempty"`
	After                   string                   `json:"after,omitempty"`
	HeadCommit              *Commit                  `json:"headCommit,omitempty"`
	ChangedFiles            *ChangedFiles            `json:"changedFiles,omitempty"`
	DesiredStateSourceMatch *DesiredStateSourceMatch `json:"desiredStateSourceMatch,omitempty"`
}

type Commit struct {
	ID        string `json:"id"`
	Message   string `json:"message,omitempty"`
	Timestamp string `json:"timestamp,omitempty"`
}

type DesiredStateSourceConfig struct {
	Kind       string
	Repository string
	Ref        string
	Path       string
}

type DesiredStateSourceMatch struct {
	Configured  bool   `json:"configured"`
	Kind        string `json:"kind"`
	Matches     bool   `json:"matches"`
	MatchStatus string `json:"matchStatus"`
	Reason      string `json:"reason"`
	Repository  string `json:"repository"`
	Ref         string `json:"ref"`
	Path        string `json:"path"`
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
		DesiredStateSourceMatch: desiredStateSourceMatch(payload, req.DesiredStateSource, changedFiles.All),
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
	FullName      string `json:"full_name"`
	DefaultBranch string `json:"default_branch"`
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

func desiredStateSourceMatch(payload pushPayload, config DesiredStateSourceConfig, changedFiles []string) *DesiredStateSourceMatch {
	config.Kind = strings.TrimSpace(config.Kind)
	config.Repository = strings.TrimSpace(config.Repository)
	config.Ref = strings.TrimSpace(config.Ref)
	config.Path = normalizeRepoPath(config.Path)

	match := &DesiredStateSourceMatch{
		Configured: config.Kind != DesiredStateSourceKindNone,
		Kind:       config.Kind,
		Matches:    false,
		Repository: config.Repository,
		Ref:        config.Ref,
		Path:       config.Path,
	}

	switch config.Kind {
	case DesiredStateSourceKindNone:
		match.Reason = "desired-state source is not configured"
		match.MatchStatus = DesiredStateSourceMatchStatusNotConfigured
		return match
	case DesiredStateSourceKindFilesystem:
		match.Reason = "desired-state source uses the filesystem catalog, so GitHub push relevance is unavailable in this mode"
		match.MatchStatus = DesiredStateSourceMatchStatusFilesystemSource
		return match
	case DesiredStateSourceKindGitHub:
		if !strings.EqualFold(config.Repository, strings.TrimSpace(payload.Repository.FullName)) {
			match.Reason = fmt.Sprintf("desired-state source repository %q does not match push repository %q", config.Repository, payload.Repository.FullName)
			match.MatchStatus = DesiredStateSourceMatchStatusRepositoryMismatch
			return match
		}
		refMatch, refReason := desiredStateSourceRefMatch(config.Ref, payload.Ref, payload.Repository.DefaultBranch)
		if !refMatch {
			match.Reason = refReason
			match.MatchStatus = DesiredStateSourceMatchStatusRefMismatch
			return match
		}
		if config.Path != "" && !pathTouched(config.Path, changedFiles) {
			match.Reason = fmt.Sprintf("desired-state source path %q was not touched by this push", config.Path)
			match.MatchStatus = DesiredStateSourceMatchStatusPathUntouched
			return match
		}
		match.Matches = true
		match.MatchStatus = DesiredStateSourceMatchStatusMatched
		if config.Path == "" {
			match.Reason = "desired-state source repository and ref match this push"
			return match
		}
		match.Reason = "desired-state source repository, ref, and path match this push"
		return match
	default:
		match.Reason = fmt.Sprintf("desired-state source kind %q is unsupported for GitHub webhook matching", config.Kind)
		match.MatchStatus = DesiredStateSourceMatchStatusNotConfigured
		match.Configured = false
		match.Kind = DesiredStateSourceKindNone
		return match
	}
}

func normalizeRef(value string) string {
	value = strings.TrimSpace(value)
	for _, prefix := range []string{"refs/heads/", "refs/tags/"} {
		if strings.HasPrefix(value, prefix) {
			return strings.TrimPrefix(value, prefix)
		}
	}
	return value
}

func desiredStateSourceRefMatch(configRef string, pushRef string, defaultBranch string) (bool, string) {
	configRef = strings.TrimSpace(configRef)
	pushRef = strings.TrimSpace(pushRef)
	defaultBranch = strings.TrimSpace(defaultBranch)

	normalizedConfigRef := normalizeRef(configRef)
	normalizedPushRef := normalizeRef(pushRef)
	if normalizedConfigRef != "" {
		if normalizedConfigRef == normalizedPushRef {
			return true, ""
		}
		return false, fmt.Sprintf("desired-state source ref %q does not match push ref %q", configRef, pushRef)
	}

	if normalizedPushRef == "" {
		return false, fmt.Sprintf("desired-state source uses the repository default branch, but push ref %q is empty", pushRef)
	}
	if defaultBranch == "" {
		return false, fmt.Sprintf("desired-state source uses the repository default branch, but the push payload did not report repository.default_branch for ref %q", pushRef)
	}
	if normalizedPushRef == normalizeRef(defaultBranch) {
		return true, ""
	}
	return false, fmt.Sprintf("desired-state source uses the repository default branch %q, which does not match push ref %q", defaultBranch, pushRef)
}

func normalizeRepoPath(value string) string {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
	if value == "" {
		return ""
	}
	value = strings.TrimPrefix(value, "./")
	cleaned := path.Clean("/" + value)
	if cleaned == "/" || cleaned == "/." {
		return ""
	}
	return strings.TrimPrefix(cleaned, "/")
}

func pathTouched(configuredPath string, changedFiles []string) bool {
	for _, changedFile := range changedFiles {
		changedFile = normalizeRepoPath(changedFile)
		if changedFile == configuredPath || strings.HasPrefix(changedFile, configuredPath+"/") {
			return true
		}
	}
	return false
}

func contextErr(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	return ctx.Err()
}
