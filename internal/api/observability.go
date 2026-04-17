package api

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"runtime/debug"
	"sync"
	"time"
)

func panicRecoveryMiddleware(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			recovered := recover()
			if recovered == nil {
				return
			}

			setRequestResult(r.Context(), "panic", "panic recovered")
			logger.Error(
				"http_request_panic",
				slog.String("request_id", RequestIDFromContext(r.Context())),
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", http.StatusInternalServerError),
				slog.Any("panic", recovered),
				slog.String("stack", string(debug.Stack())),
			)

			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = fmt.Fprintf(w, "%s (request_id=%s)\n", http.StatusText(http.StatusInternalServerError), RequestIDFromContext(r.Context()))
		}()

		next.ServeHTTP(w, r)
	})
}

func requestTimeoutMiddleware(timeout time.Duration, next http.Handler) http.Handler {
	if timeout <= 0 {
		return next
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), timeout)
		defer cancel()

		buffered := newBufferedResponseWriter()
		done := make(chan struct{})

		go func() {
			defer close(done)
			next.ServeHTTP(buffered, r.WithContext(ctx))
		}()

		select {
		case <-done:
			buffered.writeTo(w)
		case <-ctx.Done():
			if !errors.Is(ctx.Err(), context.DeadlineExceeded) {
				<-done
				buffered.writeTo(w)
				return
			}

			buffered.abandon()
			setRequestResult(r.Context(), "timeout", "request timed out")
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = fmt.Fprintf(w, "request timed out (request_id=%s)\n", RequestIDFromContext(r.Context()))
		}
	})
}

func requestLoggingMiddleware(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recorder := &statusResponseWriter{ResponseWriter: w}
		started := time.Now()

		next.ServeHTTP(recorder, r)

		status := recorder.statusCode()
		result, errText := requestResultFromContext(r.Context())
		if result == "" {
			result = classifyRequestResult(status)
		}

		attrs := []slog.Attr{
			slog.String("request_id", RequestIDFromContext(r.Context())),
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.Int("status", status),
			slog.Int64("duration_ms", time.Since(started).Milliseconds()),
			slog.String("result", result),
		}
		if errText != "" {
			attrs = append(attrs, slog.String("error", errText))
		}

		logger.LogAttrs(r.Context(), requestLogLevel(status, result), "http_request", attrs...)
	})
}

func classifyRequestResult(status int) string {
	switch {
	case status >= 500:
		return "server_error"
	case status >= 400:
		return "client_error"
	default:
		return "success"
	}
}

func requestLogLevel(status int, result string) slog.Level {
	switch {
	case result == "panic":
		return slog.LevelError
	case result == "timeout":
		return slog.LevelWarn
	case status >= 500:
		return slog.LevelError
	case status >= 400:
		return slog.LevelWarn
	default:
		return slog.LevelInfo
	}
}

type statusResponseWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusResponseWriter) Header() http.Header {
	return w.ResponseWriter.Header()
}

func (w *statusResponseWriter) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusResponseWriter) Write(body []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(body)
}

func (w *statusResponseWriter) Flush() {
	flusher, ok := w.ResponseWriter.(http.Flusher)
	if ok {
		flusher.Flush()
	}
}

func (w *statusResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := w.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, fmt.Errorf("response writer does not support hijacking")
	}
	return hijacker.Hijack()
}

func (w *statusResponseWriter) Push(target string, opts *http.PushOptions) error {
	pusher, ok := w.ResponseWriter.(http.Pusher)
	if !ok {
		return http.ErrNotSupported
	}
	return pusher.Push(target, opts)
}

func (w *statusResponseWriter) statusCode() int {
	if w.status != 0 {
		return w.status
	}
	return http.StatusOK
}

type bufferedResponseWriter struct {
	mu     sync.Mutex
	header http.Header
	status int
	body   bytes.Buffer
	closed bool
}

func newBufferedResponseWriter() *bufferedResponseWriter {
	return &bufferedResponseWriter{header: make(http.Header)}
}

func (w *bufferedResponseWriter) Header() http.Header {
	return w.header
}

func (w *bufferedResponseWriter) WriteHeader(status int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return
	}
	if w.status == 0 {
		w.status = status
	}
}

func (w *bufferedResponseWriter) Write(body []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return 0, http.ErrHandlerTimeout
	}
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.body.Write(body)
}

func (w *bufferedResponseWriter) abandon() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.closed = true
}

func (w *bufferedResponseWriter) writeTo(dst http.ResponseWriter) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for key, values := range w.header {
		for _, value := range values {
			dst.Header().Add(key, value)
		}
	}

	// The current HTTP surface is request/response only, so buffering is safe here.
	status := w.status
	if status == 0 {
		status = http.StatusOK
	}
	dst.WriteHeader(status)
	_, _ = w.body.WriteTo(dst)
}
