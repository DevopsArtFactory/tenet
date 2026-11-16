package api

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestLoginAndLogoutRejectBrowserRedirectEscapes(t *testing.T) {
	options, _ := managedLoginTestOptions(t, &fakeAuthProvider{})
	router := NewRouter(options)
	for _, path := range []string{`/\evil.example`, `/\\evil.example`, `/%5cevil.example`, `/%2fevil.example`, "/\t/evil.example", "/%09/evil.example", "https://evil.example", "//evil.example"} {
		for _, route := range []string{"/auth/login/github", "/auth/logout"} {
			t.Run(route+path, func(t *testing.T) {
				response := httptest.NewRecorder()
				router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, route+"?returnUrl="+url.QueryEscape(path), nil))
				if response.Code != http.StatusBadRequest || response.Header().Get("Location") != "" {
					t.Fatalf("unsafe redirect %q: %d %q", path, response.Code, response.Header().Get("Location"))
				}
			})
		}
	}
	for _, path := range []string{"/", "/?page=applications", "/settings?next=https%3A%2F%2Fexample.com#profile", "/apps/payments%20api"} {
		if !safeInAppReturnURL(path) {
			t.Errorf("safe in-app path rejected: %q", path)
		}
	}
}
