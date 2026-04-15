package main

import (
	"bytes"
	"sync"
)

type concurrentLogSink struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func newConcurrentLogSink() *concurrentLogSink {
	return &concurrentLogSink{}
}

func (s *concurrentLogSink) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Write(p)
}

func (s *concurrentLogSink) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.String()
}
