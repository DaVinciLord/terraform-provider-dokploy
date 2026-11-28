package client

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestReadTraefikFileResponse(t *testing.T) {
	for _, tc := range []struct {
		name     string
		status   int
		body     string
		wantErr  bool
		notFound bool
	}{
		{"empty file", http.StatusOK, `""`, false, false},
		{"missing file", http.StatusOK, "null", true, true},
		{"missing endpoint", http.StatusNotFound, "null", true, false},
		{"unexpected JSON object", http.StatusOK, "{}", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = fmt.Fprint(w, tc.body)
			}))
			defer srv.Close()
			c := NewDokployClient(srv.URL, "fake-key")
			content, err := c.ReadTraefikFile("/etc/dokploy/traefik/dynamic/example.yaml", "")
			if content != "" || (err != nil) != tc.wantErr || errors.Is(err, ErrNotFound) != tc.notFound {
				t.Fatalf("got content %q and error %v, want err=%t and ErrNotFound=%t", content, err, tc.wantErr, tc.notFound)
			}
		})
	}
}
