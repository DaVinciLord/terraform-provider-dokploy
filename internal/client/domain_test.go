package client

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDeleteDomainUsesDeleteEndpoint(t *testing.T) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %q, want %q", r.Method, http.MethodPost)
		}
		if r.URL.Path != "/domain.delete" {
			t.Errorf("path = %q, want %q", r.URL.Path, "/domain.delete")
		}

		var payload map[string]string
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		if payload["domainId"] != "domain-123" {
			t.Errorf("domainId = %q, want %q", payload["domainId"], "domain-123")
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"domainId":"domain-123"}`))
	}))
	defer server.Close()

	client := NewDokployClient(server.URL, "test-api-key")
	if err := client.DeleteDomain("domain-123"); err != nil {
		t.Fatalf("DeleteDomain() error = %v", err)
	}
}
