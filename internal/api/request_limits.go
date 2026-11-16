package api

import (
	"bytes"
	"errors"
	"io"
	"net/http"

	"github.com/DevopsArtFactory/tenet/internal/githubwebhook"
)

const maxAPIRequestBodyBytes = 1 << 20

func requestBodyLimitMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		limit := int64(maxAPIRequestBodyBytes)
		webhook := r.URL.Path == "/api/v1/github/webhook"
		if webhook {
			limit = githubwebhook.MaxBodyBytes
		}
		if r.ContentLength > limit {
			http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
			return
		}
		// Webhook decoding enforces its own streaming limit, including for non-HTTP callers.
		if webhook || r.Body == nil || r.Body == http.NoBody {
			next.ServeHTTP(w, r)
			return
		}
		body := http.MaxBytesReader(w, r.Body, limit)
		defer body.Close()
		payload, err := io.ReadAll(body)
		if err != nil {
			var oversized *http.MaxBytesError
			if errors.As(err, &oversized) {
				http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
			} else {
				http.Error(w, "failed to read request body", http.StatusBadRequest)
			}
			return
		}
		// Validate the whole body before a handler can mutate state, including chunked requests.
		r.Body = io.NopCloser(bytes.NewReader(payload))
		next.ServeHTTP(w, r)
	})
}
