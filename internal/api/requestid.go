package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// RequestIDHeader is the canonical response header Tenet uses for request correlation.
const RequestIDHeader = "Request-Id"

type requestIDContextKey struct{}
type requestStateContextKey struct{}

type requestState struct {
	mu     sync.Mutex
	result string
	err    string
}

var fallbackRequestIDCounter atomic.Uint64

func requestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := strings.TrimSpace(r.Header.Get(RequestIDHeader))
		if requestID == "" {
			requestID = newRequestID()
		}

		ctx := context.WithValue(r.Context(), requestIDContextKey{}, requestID)
		ctx = context.WithValue(ctx, requestStateContextKey{}, &requestState{})

		w.Header().Set(RequestIDHeader, requestID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequestIDFromContext returns the request correlation ID stored on the request context.
func RequestIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	requestID, _ := ctx.Value(requestIDContextKey{}).(string)
	return requestID
}

func setRequestResult(ctx context.Context, result, err string) {
	if ctx == nil {
		return
	}
	state, _ := ctx.Value(requestStateContextKey{}).(*requestState)
	if state == nil {
		return
	}

	state.mu.Lock()
	defer state.mu.Unlock()
	if state.result != "" {
		return
	}
	state.result = result
	state.err = err
}

func requestResultFromContext(ctx context.Context) (string, string) {
	if ctx == nil {
		return "", ""
	}
	state, _ := ctx.Value(requestStateContextKey{}).(*requestState)
	if state == nil {
		return "", ""
	}

	state.mu.Lock()
	defer state.mu.Unlock()
	return state.result, state.err
}

func newRequestID() string {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err == nil {
		return hex.EncodeToString(buf[:])
	}
	return fmt.Sprintf("%x-%016x", time.Now().UTC().UnixNano(), fallbackRequestIDCounter.Add(1))
}
