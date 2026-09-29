package azure

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rmkr-dev/sentinelmesh/internal/domain"
	"gopkg.in/yaml.v3"
)

// ResourceType is one documented Azure resource the poller queries.
type ResourceType struct {
	Type    string `yaml:"type"`
	Signals []struct {
		Metric string `yaml:"metric"`
	} `yaml:"signals"`
}

// LoadResourceTypes reads the three supported resource-type documents.
func LoadResourceTypes(dir string) ([]ResourceType, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []ResourceType
	for _, e := range entries {
		if e.IsDir() || !(strings.HasSuffix(e.Name(), ".yaml") || strings.HasSuffix(e.Name(), ".yml")) {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		var rt ResourceType
		if err := yaml.Unmarshal(b, &rt); err != nil {
			return nil, fmt.Errorf("%s: %w", e.Name(), err)
		}
		if rt.Type == "" {
			return nil, fmt.Errorf("%s: type is required", e.Name())
		}
		out = append(out, rt)
	}
	return out, nil
}

// Sink is the platform store surface the poller writes.
type Sink interface {
	SaveResource(ctx context.Context, r domain.Resource) error
	SaveChange(ctx context.Context, c domain.Change) error
	SaveHealth(ctx context.Context, h domain.HealthEvent) error
}

// Poller reads inventory, activity, and resource health on a schedule.
type Poller struct {
	Client       Client
	Subscription string
	WorkspaceID  string
	Types        []ResourceType
	Now          func() time.Time
}

// Poll saves one cycle of inventory, changes, and health.
func (p Poller) Poll(ctx context.Context, sink Sink) error {
	now := time.Now().UTC()
	if p.Now != nil {
		now = p.Now().UTC()
	}
	for _, rt := range p.Types {
		q := fmt.Sprintf("resources | where type =~ '%s' | project id, name, type, location, tags", strings.ReplaceAll(rt.Type, "'", ""))
		resources, err := p.Client.Inventory(ctx, InventoryQuery{Subscriptions: []string{p.Subscription}, Query: q})
		if err != nil {
			return err
		}
		for _, r := range resources {
			if err := sink.SaveResource(ctx, r); err != nil {
				return err
			}
		}
	}
	if p.WorkspaceID == "" {
		return nil
	}
	rows, err := p.Client.KQL(ctx, p.WorkspaceID, "AzureActivity | project TimeGenerated, OperationNameValue, ActivityStatusValue, ResourceId, Caller, CorrelationId, EventDataId", "PT1H")
	if err != nil {
		return err
	}
	for _, ch := range ChangesFromActivity(rows, now) {
		if err := sink.SaveChange(ctx, ch); err != nil {
			return err
		}
	}
	healthRows, err := p.Client.KQL(ctx, p.WorkspaceID, "AzureActivity | where CategoryValue == 'ResourceHealth' | project TimeGenerated, ResourceId, ActivityStatusValue, OperationNameValue, CorrelationId", "PT1H")
	if err != nil {
		return err
	}
	for _, row := range healthRows {
		if _, ok := row["Status"]; !ok {
			if v, ok := row["ActivityStatusValue"].(string); ok {
				row["Status"] = v
			}
		}
	}
	for _, h := range HealthFromRows(healthRows, now) {
		if err := sink.SaveHealth(ctx, h); err != nil {
			return err
		}
	}
	return nil
}
