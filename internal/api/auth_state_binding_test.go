package api

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/DevopsArtFactory/tenet/internal/authprovider"
)

func TestOAuthCallbackRequiresInitiatingBrowser(t *testing.T) {
	for _, providerName := range []string{authprovider.GitHubProviderName, authprovider.GoogleProviderName, authprovider.OktaOIDCProviderName} {
		t.Run(providerName, func(t *testing.T) {
			provider := &fakeAuthProvider{name: providerName}
			options, _ := managedLoginTestOptions(t, provider)
			router := NewRouter(options)
			routes := options.AuthRuntime.TenetManagedLogin
			login := func() (*http.Cookie, string) {
				t.Helper()
				response := httptest.NewRecorder()
				router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "https://tenet.example"+routes.LoginRoute, nil))
				if response.Code != http.StatusFound {
					t.Fatalf("login: %d", response.Code)
				}
				location, err := url.Parse(response.Header().Get("Location"))
				if err != nil {
					t.Fatal(err)
				}
				cookies := response.Result().Cookies()
				if len(cookies) != 1 {
					t.Fatalf("expected browser binding cookie, got %d", len(cookies))
				}
				cookie := cookies[0]
				if !cookie.HttpOnly || !cookie.Secure || cookie.SameSite != http.SameSiteLaxMode || cookie.MaxAge != 600 || cookie.Domain != "" {
					t.Fatalf("unsafe cookie attributes: %+v", cookie)
				}
				return cookie, location.Query().Get("state")
			}
			cookie, state := login()
			otherCookie, _ := login()
			callback := func(binding *http.Cookie) *httptest.ResponseRecorder {
				req := httptest.NewRequest(http.MethodGet, "https://tenet.example"+routes.CallbackRoute+"?state="+url.QueryEscape(state)+"&code=code", nil)
				if binding != nil {
					req.AddCookie(binding)
				}
				response := httptest.NewRecorder()
				router.ServeHTTP(response, req)
				return response
			}
			for _, invalid := range []*http.Cookie{nil, otherCookie, {Name: cookie.Name, Value: state}} {
				response := callback(invalid)
				if response.Code != http.StatusBadRequest || len(provider.exchangedCodes) != 0 || len(response.Result().Cookies()) != 0 {
					t.Fatalf("unbound callback accepted: %d", response.Code)
				}
			}
			response := callback(cookie)
			if response.Code != http.StatusFound || len(provider.exchangedCodes) != 1 {
				t.Fatalf("bound callback failed: %d %s", response.Code, response.Body.String())
			}
			cookies := response.Result().Cookies()
			if len(cookies) != 2 || cookies[1].Name != cookie.Name || cookies[1].MaxAge != -1 {
				t.Fatalf("binding not cleared: %+v", cookies)
			}
			if response := callback(cookie); response.Code != http.StatusBadRequest || len(provider.exchangedCodes) != 1 {
				t.Fatalf("replayed callback accepted: %d", response.Code)
			}
		})
	}
}
