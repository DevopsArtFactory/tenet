package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DevopsArtFactory/tenet/internal/authstate"
)

func TestManagedLoginReturnsBackpressure(t *testing.T) {
	options, _ := managedLoginTestOptions(t, &fakeAuthProvider{})
	store := options.AuthStateStore.(authstate.FileStore)
	now := time.Now().UTC()
	states := make([]authstate.State, 120)
	for i := range states {
		states[i] = authstate.State{Token: fmt.Sprint(i), Provider: "github", CreatedAt: now, ExpiresAt: now.Add(10 * time.Minute)}
	}
	if err := authstate.Save(store.Path, states); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	NewRouter(options).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/auth/login/github", nil))
	if response.Code != http.StatusTooManyRequests || response.Header().Get("Retry-After") == "" {
		t.Fatalf("status=%d retry=%q body=%s", response.Code, response.Header().Get("Retry-After"), response.Body.String())
	}
	if response.Header().Get("Location") != "" || len(response.Result().Cookies()) != 0 {
		t.Fatal("limited login still issued an authorization redirect or cookie")
	}
}

func TestManagedLoginRejectsOversizedReturnURL(t *testing.T) {
	options, _ := managedLoginTestOptions(t, &fakeAuthProvider{})
	response := httptest.NewRecorder()
	NewRouter(options).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/auth/login/github?returnUrl=/"+strings.Repeat("a", 2048), nil))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400", response.Code)
	}
	states, err := options.AuthStateStore.List(t.Context())
	if err != nil || len(states) != 0 {
		t.Fatalf("invalid login wrote state: %d %v", len(states), err)
	}
}
