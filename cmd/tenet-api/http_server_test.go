package main

import (
	"io"
	"net"
	"net/http"
	"sync/atomic"
	"testing"
	"time"
)

func TestHTTPServerClosesIncompleteHeaders(t *testing.T) {
	var called atomic.Bool
	server := newHTTPServer("", http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called.Store(true) }), 30*time.Second)
	if server.ReadHeaderTimeout <= 0 || server.ReadTimeout <= 0 || server.WriteTimeout <= 0 || server.IdleTimeout <= 0 || server.MaxHeaderBytes <= 0 {
		t.Fatal("HTTP server must bound connection resource use")
	}
	server.ReadHeaderTimeout = 50 * time.Millisecond
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	defer func() { server.Close(); <-done }()
	conn, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(2 * time.Second))
	if _, err := io.WriteString(conn, "GET / HTTP/1.1\r\nHost: localhost\r\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := io.ReadAll(conn); err != nil {
		t.Fatalf("server did not close slow header connection: %v", err)
	}
	if called.Load() {
		t.Fatal("incomplete request reached handler")
	}
}
