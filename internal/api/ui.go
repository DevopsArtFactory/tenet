package api

import (
	_ "embed"
	"net/http"
)

//go:embed web/index.html
var indexHTML []byte

func serveUI(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(indexHTML)
}
