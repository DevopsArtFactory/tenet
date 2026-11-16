package githubwebhook

import (
	"context"
	"io"
	"strings"
	"testing"
)

type countingBody struct {
	io.Reader
	bytes int
}

func (r *countingBody) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	r.bytes += n
	return n, err
}
func TestDecodeBoundsUnauthenticatedBodyRead(t *testing.T) {
	const limit = 25 << 20
	body := &countingBody{Reader: strings.NewReader(strings.Repeat(" ", limit+4096))}
	_, err := Decode(context.Background(), Request{Secret: "test-secret", Header: signedHeader("test-secret", "push", "different-body"), Body: body})
	if err == nil || !strings.Contains(err.Error(), "too large") || body.bytes > limit+1 {
		t.Fatalf("read %d bytes, error=%v", body.bytes, err)
	}
}
