package githubcontents

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	pathpkg "path"
	"strings"

	"github.com/DevopsArtFactory/tenet/internal/manifestcodec"
)

const defaultBaseURL = "https://api.github.com/"

type ErrorCode string

const (
	ErrorCodeInvalidInput               ErrorCode = "invalid_input"
	ErrorCodeUnauthorized               ErrorCode = "unauthorized"
	ErrorCodeNotFound                   ErrorCode = "not_found"
	ErrorCodeUnsupportedContentEncoding ErrorCode = "unsupported_content_encoding"
	ErrorCodeDecodeFailure              ErrorCode = "decode_failure"
	ErrorCodeUnexpectedStatus           ErrorCode = "unexpected_status"
)

// Error is an operator-facing GitHub config fetch or decode failure.
type Error struct {
	Code       ErrorCode
	Message    string
	StatusCode int
	Err        error
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// Request identifies one repository content object to fetch.
type Request struct {
	Repository string
	Owner      string
	Repo       string
	Ref        string
	Path       string
	Token      string
}

// File is one decoded GitHub contents API file payload plus raw bytes.
type File struct {
	Owner       string
	Repo        string
	Ref         string
	Name        string
	Path        string
	SHA         string
	Size        int
	HTMLURL     string
	DownloadURL string
	Content     []byte
}

// Decode applies Tenet's manifest decoding rules to the fetched file.
func (f File) Decode(out any) error {
	return Decode(f.Path, f.Content, out)
}

// Decode applies Tenet's manifest decoding rules to GitHub-fetched bytes.
func Decode(name string, content []byte, out any) error {
	if err := manifestcodec.Decode(name, content, out); err != nil {
		return &Error{
			Code:    ErrorCodeDecodeFailure,
			Message: fmt.Sprintf("GitHub desired-config decode failed for %q: %v", name, err),
			Err:     err,
		}
	}
	return nil
}

// Client fetches one file from the GitHub contents API.
type Client struct {
	BaseURL    string
	HTTPClient *http.Client
}

func (c Client) Fetch(ctx context.Context, req Request) (File, error) {
	owner, repo, err := normalizeRepository(req)
	if err != nil {
		return File{}, invalidInputError(err)
	}
	manifestPath, err := normalizePath(req.Path)
	if err != nil {
		return File{}, invalidInputError(err)
	}

	baseURL := c.BaseURL
	if strings.TrimSpace(baseURL) == "" {
		baseURL = defaultBaseURL
	}
	base, err := url.Parse(baseURL)
	if err != nil {
		return File{}, &Error{
			Code:    ErrorCodeInvalidInput,
			Message: "GitHub contents client is misconfigured with an invalid base URL",
			Err:     err,
		}
	}

	u, err := base.Parse(contentsAPIPath(owner, repo, manifestPath))
	if err != nil {
		return File{}, &Error{
			Code:    ErrorCodeInvalidInput,
			Message: "GitHub contents client could not construct a repository contents request",
			Err:     err,
		}
	}
	if ref := strings.TrimSpace(req.Ref); ref != "" {
		query := u.Query()
		query.Set("ref", ref)
		u.RawQuery = query.Encode()
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return File{}, fmt.Errorf("create GitHub contents request: %w", err)
	}
	httpReq.Header.Set("Accept", "application/vnd.github+json")
	httpReq.Header.Set("User-Agent", "tenet")
	if token := strings.TrimSpace(req.Token); token != "" {
		httpReq.Header.Set("Authorization", "Bearer "+token)
	}

	httpClient := c.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return File{}, fmt.Errorf("fetch GitHub contents: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return File{}, fmt.Errorf("read GitHub contents response: %w", err)
	}

	switch resp.StatusCode {
	case http.StatusOK:
		return decodeFileResponse(owner, repo, strings.TrimSpace(req.Ref), manifestPath, body)
	case http.StatusUnauthorized, http.StatusForbidden:
		return File{}, &Error{
			Code:       ErrorCodeUnauthorized,
			StatusCode: resp.StatusCode,
			Message:    fmt.Sprintf("GitHub desired-config fetch was unauthorized for %s/%s at %q; check repository visibility and token permissions", owner, repo, manifestPath),
		}
	case http.StatusNotFound:
		return File{}, &Error{
			Code:       ErrorCodeNotFound,
			StatusCode: resp.StatusCode,
			Message:    fmt.Sprintf("GitHub desired-config %q was not found in %s/%s at ref %q", manifestPath, owner, repo, displayRef(req.Ref)),
		}
	default:
		return File{}, &Error{
			Code:       ErrorCodeUnexpectedStatus,
			StatusCode: resp.StatusCode,
			Message:    fmt.Sprintf("GitHub desired-config fetch for %s/%s at %q returned unexpected status %d", owner, repo, manifestPath, resp.StatusCode),
		}
	}
}

type fileResponse struct {
	Type        string `json:"type"`
	Name        string `json:"name"`
	Path        string `json:"path"`
	SHA         string `json:"sha"`
	Size        int    `json:"size"`
	Encoding    string `json:"encoding"`
	Content     string `json:"content"`
	HTMLURL     string `json:"html_url"`
	DownloadURL string `json:"download_url"`
}

func decodeFileResponse(owner string, repo string, ref string, fallbackPath string, body []byte) (File, error) {
	var payload fileResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return File{}, fmt.Errorf("decode GitHub contents response: %w", err)
	}

	if strings.TrimSpace(payload.Path) == "" {
		payload.Path = fallbackPath
	}
	if strings.TrimSpace(payload.Name) == "" {
		payload.Name = pathpkg.Base(payload.Path)
	}

	content, err := decodeContent(payload.Path, payload.Encoding, payload.Content)
	if err != nil {
		return File{}, err
	}

	return File{
		Owner:       owner,
		Repo:        repo,
		Ref:         ref,
		Name:        payload.Name,
		Path:        payload.Path,
		SHA:         payload.SHA,
		Size:        payload.Size,
		HTMLURL:     payload.HTMLURL,
		DownloadURL: payload.DownloadURL,
		Content:     content,
	}, nil
}

func decodeContent(path string, encoding string, content string) ([]byte, error) {
	if strings.TrimSpace(encoding) != "base64" {
		return nil, &Error{
			Code:    ErrorCodeUnsupportedContentEncoding,
			Message: fmt.Sprintf("GitHub desired-config %q used unsupported content encoding %q", path, encoding),
		}
	}

	decoded, err := base64.StdEncoding.DecodeString(strings.NewReplacer("\n", "", "\r", "").Replace(content))
	if err != nil {
		return nil, &Error{
			Code:    ErrorCodeUnsupportedContentEncoding,
			Message: fmt.Sprintf("GitHub desired-config %q returned invalid base64 content: %v", path, err),
			Err:     err,
		}
	}
	return decoded, nil
}

func normalizeRepository(req Request) (string, string, error) {
	if strings.TrimSpace(req.Repository) != "" {
		if strings.TrimSpace(req.Owner) != "" || strings.TrimSpace(req.Repo) != "" {
			return "", "", fmt.Errorf("set either repository or owner/repo, not both")
		}
		parts := strings.Split(strings.TrimSpace(req.Repository), "/")
		if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
			return "", "", fmt.Errorf("repository must be formatted as owner/repo")
		}
		return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]), nil
	}

	owner := strings.TrimSpace(req.Owner)
	repo := strings.TrimSpace(req.Repo)
	if owner == "" || repo == "" {
		return "", "", fmt.Errorf("owner and repo are required when repository is not set")
	}
	return owner, repo, nil
}

func normalizePath(raw string) (string, error) {
	raw = strings.TrimSpace(strings.ReplaceAll(raw, "\\", "/"))
	if raw == "" {
		return "", fmt.Errorf("path is required")
	}
	if strings.HasPrefix(raw, "/") {
		return "", fmt.Errorf("path must be repository-relative")
	}

	segments := strings.Split(raw, "/")
	for _, segment := range segments {
		switch segment {
		case "", ".", "..":
			return "", fmt.Errorf("path must not contain empty, current-directory, or parent-directory segments")
		}
	}
	return strings.Join(segments, "/"), nil
}

func contentsAPIPath(owner string, repo string, manifestPath string) string {
	escapedPath := make([]string, 0, len(strings.Split(manifestPath, "/")))
	for _, segment := range strings.Split(manifestPath, "/") {
		escapedPath = append(escapedPath, url.PathEscape(segment))
	}
	return fmt.Sprintf("/repos/%s/%s/contents/%s", url.PathEscape(owner), url.PathEscape(repo), strings.Join(escapedPath, "/"))
}

func invalidInputError(err error) error {
	return &Error{
		Code:    ErrorCodeInvalidInput,
		Message: fmt.Sprintf("GitHub desired-config request was invalid: %v", err),
		Err:     err,
	}
}

func displayRef(ref string) string {
	if strings.TrimSpace(ref) == "" {
		return "default branch"
	}
	return strings.TrimSpace(ref)
}
