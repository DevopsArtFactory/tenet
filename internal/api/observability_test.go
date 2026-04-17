package api

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRequestTimeoutMiddlewareRejectsLateWritesAfterTimeout(t *testing.T) {
	allowLateWrite := make(chan struct{})
	lateWriteErr := make(chan error, 1)
	handlerDone := make(chan struct{})

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
		<-allowLateWrite
		_, err := w.Write([]byte("late body"))
		lateWriteErr <- err
		close(handlerDone)
	})

	middleware := requestIDMiddleware(requestTimeoutMiddleware(10*time.Millisecond, handler))

	req := httptest.NewRequest(http.MethodGet, "/slow", nil)
	req.Header.Set(RequestIDHeader, "timeout-request-id")
	resp := httptest.NewRecorder()

	middleware.ServeHTTP(resp, req)

	if resp.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 timeout response, got %d", resp.Code)
	}
	body := resp.Body.String()
	if !strings.Contains(body, "request timed out") || !strings.Contains(body, "request_id=timeout-request-id") {
		t.Fatalf("expected explicit correlated timeout response, got %q", body)
	}

	close(allowLateWrite)

	select {
	case err := <-lateWriteErr:
		if !errors.Is(err, http.ErrHandlerTimeout) {
			t.Fatalf("expected late write to fail with ErrHandlerTimeout, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for late write result")
	}

	select {
	case <-handlerDone:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for handler completion")
	}

	if resp.Body.String() != body {
		t.Fatalf("expected timeout response body to remain stable, got %q", resp.Body.String())
	}
}
