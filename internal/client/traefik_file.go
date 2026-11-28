package client

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
)

// ReadTraefikFile returns ErrNotFound when Dokploy reports an absent file.
func (c *DokployClient) ReadTraefikFile(path, serverID string) (string, error) {
	query := url.Values{"path": {path}}
	if serverID != "" {
		query.Set("serverId", serverID)
	}
	body, err := c.doRequest(http.MethodGet, "settings.readTraefikFile?"+query.Encode(), nil)
	if errors.Is(err, ErrNotFound) {
		// Dokploy signals a missing file with HTTP 200 and JSON null.
		return "", fmt.Errorf("Traefik file endpoint returned HTTP 404: %v", err)
	}
	if err != nil {
		return "", err
	}

	var content *string
	if err := json.Unmarshal(body, &content); err != nil {
		return "", fmt.Errorf("decode Traefik file: %w", err)
	}
	if content == nil {
		return "", ErrNotFound
	}
	return *content, nil
}

func (c *DokployClient) UpdateTraefikFile(path, serverID, content string) error {
	payload := struct {
		Path          string `json:"path"`
		TraefikConfig string `json:"traefikConfig"`
		ServerID      string `json:"serverId,omitempty"`
	}{
		Path:          path,
		TraefikConfig: content,
		ServerID:      serverID,
	}
	_, err := c.doRequest(http.MethodPost, "settings.updateTraefikFile", payload)
	return err
}
