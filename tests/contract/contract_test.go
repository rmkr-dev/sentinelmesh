package contract

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/rmkr-dev/sentinelmesh/internal/domain"
	"gopkg.in/yaml.v3"
)

func TestIncidentShape(t *testing.T) {
	inc := domain.Incident{
		ID: "INC-2026-0001", Severity: "SEV2", Status: "investigating",
		Title: "Correlated symptoms on payment-service", Service: "payment-service",
		Environment: "production", StartedAt: time.Now().UTC(), DetectedAt: time.Now().UTC(),
		Signals: []domain.Signal{}, Events: []domain.TimelineEntry{},
		SuspectedCauses: []domain.Hypothesis{}, Evidence: []domain.Evidence{},
		RecommendedActions: []domain.RecommendedAction{},
	}
	raw, err := json.Marshal(inc)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"incident_id", "severity", "status", "service", "environment", "started_at", "detected_at", "signals", "events", "suspected_causes", "evidence", "recommended_actions"} {
		if _, ok := m[key]; !ok {
			t.Fatalf("missing %s", key)
		}
	}
}

func TestSLOShape(t *testing.T) {
	raw, _ := json.Marshal(domain.SLOResult{
		Service: "payment-service", SLO: "availability", Target: 99.95, Current: 99.72,
		ErrorBudgetRemaining: -460, BurnRate: 5.6, Status: "breached",
	})
	var m map[string]any
	_ = json.Unmarshal(raw, &m)
	for _, key := range []string{"service", "slo", "target", "current", "error_budget_remaining", "burn_rate", "status"} {
		if _, ok := m[key]; !ok {
			t.Fatalf("missing %s", key)
		}
	}
}

func TestOpenAPIParses(t *testing.T) {
	b, err := os.ReadFile("../../api/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := yaml.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	if doc["openapi"] == "" {
		t.Fatal("openapi version missing")
	}
	paths, _ := doc["paths"].(map[string]any)
	for _, p := range []string{"/health", "/ready", "/api/v1/incidents", "/api/v1/slos", "/api/v1/demo/faults"} {
		if _, ok := paths[p]; !ok {
			t.Fatalf("missing path %s", p)
		}
	}
}
