package azure

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/rmkr-dev/sentinelmesh/internal/domain"
)

// PromQL runs an instant query against an Azure Monitor workspace Prometheus endpoint.
func (c Client) PromQL(ctx context.Context, endpoint, query string) (float64, bool, error) {
	u, err := url.Parse(endpoint + "/api/v1/query")
	if err != nil {
		return 0, false, err
	}
	q := u.Query()
	q.Set("query", query)
	u.RawQuery = q.Encode()
	req, err := http.NewRequest(http.MethodGet, c.absolute(u.String()), nil)
	if err != nil {
		return 0, false, err
	}
	if c.Scope == "" {
		c.Scope = "https://prometheus.monitor.azure.com/.default"
	}
	body, status, err := c.Do(ctx, req)
	if err != nil {
		return 0, false, err
	}
	if status >= 300 {
		return 0, false, fmt.Errorf("azure monitor prometheus status %d", status)
	}
	var parsed struct {
		Status string `json:"status"`
		Data   struct {
			Result []struct {
				Value []any `json:"value"`
			} `json:"result"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return 0, false, err
	}
	if parsed.Status != "success" || len(parsed.Data.Result) == 0 || len(parsed.Data.Result[0].Value) < 2 {
		return 0, false, nil
	}
	raw, _ := parsed.Data.Result[0].Value[1].(string)
	var n float64
	if _, err := fmt.Sscan(raw, &n); err != nil {
		return 0, false, err
	}
	return n, true, nil
}

// KQL runs a workspace query. Results are row maps. Callers redact them.
func (c Client) KQL(ctx context.Context, workspaceID, kql string, timespan string) ([]map[string]any, error) {
	if timespan == "" {
		timespan = "PT1H"
	}
	payload, _ := json.Marshal(map[string]string{"query": kql, "timespan": timespan})
	endpoint := c.absolute(fmt.Sprintf("https://api.loganalytics.io/v1/workspaces/%s/query", workspaceID))
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.Scope == "" {
		c.Scope = "https://api.loganalytics.io/.default"
	}
	body, status, err := c.Do(ctx, req)
	if err != nil {
		return nil, err
	}
	if status >= 300 {
		return nil, fmt.Errorf("log analytics status %d", status)
	}
	var parsed struct {
		Tables []struct {
			Columns []struct {
				Name string `json:"name"`
			} `json:"columns"`
			Rows [][]any `json:"rows"`
		} `json:"tables"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, err
	}
	if len(parsed.Tables) == 0 {
		return nil, nil
	}
	table := parsed.Tables[0]
	var out []map[string]any
	for _, row := range table.Rows {
		item := map[string]any{}
		for i, col := range table.Columns {
			if i < len(row) {
				item[col.Name] = row[i]
			}
		}
		out = append(out, item)
	}
	return out, nil
}

// ChangesFromActivity maps AzureActivity rows to domain changes.
// Only the caller object id or UPN is kept. IPs and claim dumps are dropped.
func ChangesFromActivity(rows []map[string]any, now time.Time) []domain.Change {
	var out []domain.Change
	for _, row := range rows {
		op, _ := row["OperationNameValue"].(string)
		if op == "" {
			op, _ = row["OperationName"].(string)
		}
		status, _ := row["ActivityStatusValue"].(string)
		if status != "" && status != "Success" && status != "Succeeded" {
			continue
		}
		target, _ := row["ResourceId"].(string)
		actor, _ := row["Caller"].(string)
		id, _ := row["CorrelationId"].(string)
		if id == "" {
			id, _ = row["EventDataId"].(string)
		}
		if id == "" {
			id = op + "|" + target
		}
		out = append(out, domain.Change{
			ID: id, Kind: "activity", Source: "activity_log", Target: target, Actor: actor, OccurredAt: rowTime(row, now),
			Attributes: map[string]string{"operation": op},
		})
	}
	return out
}

// HealthFromRows maps Resource Health rows.
func HealthFromRows(rows []map[string]any, now time.Time) []domain.HealthEvent {
	var out []domain.HealthEvent
	for _, row := range rows {
		res, _ := row["ResourceId"].(string)
		state, _ := row["Status"].(string)
		if state == "" {
			state, _ = row["properties_currentHealthStatus"].(string)
		}
		reason, _ := row["Reason"].(string)
		out = append(out, domain.HealthEvent{Resource: res, State: state, Reason: reason, Source: "resource_health", At: rowTime(row, now)})
	}
	return out
}

func rowTime(row map[string]any, fallback time.Time) time.Time {
	for _, key := range []string{"TimeGenerated", "EventTimestamp", "time", "timestamp"} {
		switch v := row[key].(type) {
		case string:
			if t, err := time.Parse(time.RFC3339, v); err == nil {
				return t.UTC()
			}
		case time.Time:
			return v.UTC()
		}
	}
	return fallback
}
