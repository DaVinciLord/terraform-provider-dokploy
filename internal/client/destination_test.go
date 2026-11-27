package client

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Dokploy v0.31 requires additionalFlags on destination.create and destination.update.
func TestDestinationRequestsSendAdditionalFlags(t *testing.T) {
	tests := []struct {
		name string
		path string
		call func(*DokployClient) error
	}{
		{
			name: "create",
			path: "/destination.create",
			call: func(c *DokployClient) error {
				_, err := c.CreateDestination(Destination{Name: "backups", Provider: "Scaleway"})
				return err
			},
		},
		{
			name: "update",
			path: "/destination.update",
			call: func(c *DokployClient) error {
				_, err := c.UpdateDestination(Destination{DestinationID: "dest-123", Name: "backups", Provider: "Scaleway"})
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != tt.path {
					t.Errorf("path = %q, want %q", r.URL.Path, tt.path)
				}

				var payload map[string]json.RawMessage
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Fatalf("decode request body: %v", err)
				}
				flags, ok := payload["additionalFlags"]
				if !ok {
					t.Fatalf("additionalFlags missing from %s payload", tt.path)
				}
				if string(flags) != "[]" {
					t.Errorf("additionalFlags = %s, want []", flags)
				}

				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"destinationId":"dest-123","name":"backups"}`))
			}))
			defer server.Close()

			if err := tt.call(NewDokployClient(server.URL, "test-api-key")); err != nil {
				t.Fatalf("request error = %v", err)
			}
		})
	}
}
