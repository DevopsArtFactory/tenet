package main

import (
	"net/http"
	"time"
)

func newHTTPServer(addr string, handler http.Handler, requestTimeout time.Duration) *http.Server {
	if requestTimeout <= 0 {
		requestTimeout = 30 * time.Second
	}
	return &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       requestTimeout,
		WriteTimeout:      requestTimeout + 5*time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    32 << 10,
	}
}
