package githubcontents

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

var commitSHA = regexp.MustCompile(`^[a-fA-F0-9]{40}$`)

// ResolveCommit resolves a branch/tag/ref once. Subsequent contents requests must
// use the returned SHA, so a moving branch cannot mix files from different commits.
func (c Client) ResolveCommit(ctx context.Context, req Request) (string, error) {
	owner, repo, err := normalizeRepository(req)
	if err != nil {
		return "", invalidInputError(err)
	}
	ref := strings.TrimSpace(req.Ref)
	if ref == "" {
		return "", invalidInputError(fmt.Errorf("an explicit GitHub ref is required for versioned execution"))
	}
	baseURL := strings.TrimSpace(c.BaseURL)
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	base, err := url.Parse(strings.TrimRight(baseURL, "/") + "/")
	if err != nil {
		return "", invalidInputError(err)
	}
	endpoint, err := base.Parse("repos/" + url.PathEscape(owner) + "/" + url.PathEscape(repo) + "/commits/" + url.PathEscape(ref))
	if err != nil {
		return "", invalidInputError(err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return "", err
	}
	// This media type returns only the SHA, without large commit diffs/file lists.
	request.Header.Set("Accept", "application/vnd.github.sha")
	request.Header.Set("User-Agent", "tenet")
	if token := strings.TrimSpace(req.Token); token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	client := c.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	response, err := client.Do(request)
	if err != nil {
		return "", fmt.Errorf("resolve GitHub revision: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		code := ErrorCodeUnexpectedStatus
		if response.StatusCode == http.StatusNotFound {
			code = ErrorCodeNotFound
		}
		if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
			code = ErrorCodeUnauthorized
		}
		return "", &Error{Code: code, StatusCode: response.StatusCode, Message: fmt.Sprintf("GitHub revision resolution returned status %d for %s/%s", response.StatusCode, owner, repo)}
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 129))
	if err != nil {
		return "", err
	}
	if len(body) > 128 {
		return "", &Error{Code: ErrorCodeInvalidResponse, Message: "GitHub revision response exceeded the SHA-only response limit"}
	}
	revision := strings.TrimSpace(string(body))
	if !commitSHA.MatchString(revision) {
		return "", &Error{Code: ErrorCodeInvalidResponse, Message: "GitHub revision response did not contain an immutable commit SHA"}
	}
	return strings.ToLower(revision), nil
}
