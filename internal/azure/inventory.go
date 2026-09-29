package azure

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/rmkr-dev/sentinelmesh/internal/domain"
)

// InventoryQuery is an Azure Resource Graph request.
type InventoryQuery struct {
	Subscriptions []string
	Query         string
}

// Inventory lists resources and maps them to domain.Resource.
// Bind a resource to a service with the tag sentinelmesh.service.
func (c Client) Inventory(ctx context.Context, q InventoryQuery) ([]domain.Resource, error) {
	payload, _ := json.Marshal(map[string]any{
		"subscriptions": q.Subscriptions,
		"query":         q.Query,
		"options":       map[string]any{"resultFormat": "objectArray"},
	})
	req, err := http.NewRequest(http.MethodPost, "https://management.azure.com/providers/Microsoft.ResourceGraph/resources?api-version=2022-10-01", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	body, status, err := c.Do(ctx, req)
	if err != nil {
		return nil, err
	}
	if status >= 300 {
		return nil, fmt.Errorf("resource graph status %d", status)
	}
	var parsed struct {
		Data []struct {
			ID       string            `json:"id"`
			Name     string            `json:"name"`
			Type     string            `json:"type"`
			Location string            `json:"location"`
			Tags     map[string]string `json:"tags"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	var out []domain.Resource
	for _, row := range parsed.Data {
		svc := ""
		if row.Tags != nil {
			svc = row.Tags["sentinelmesh.service"]
		}
		out = append(out, domain.Resource{
			ID: row.ID, Kind: row.Type, Provider: "azure", Name: row.Name, Region: row.Location,
			Service: svc, Attributes: row.Tags, UpdatedAt: now,
		})
	}
	return out, nil
}
