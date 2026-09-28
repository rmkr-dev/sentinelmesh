package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/rmkr-dev/sentinelmesh/internal/ai"
	"github.com/rmkr-dev/sentinelmesh/internal/domain"
	"github.com/rmkr-dev/sentinelmesh/internal/engine"
	"github.com/rmkr-dev/sentinelmesh/internal/remediation"
	"github.com/rmkr-dev/sentinelmesh/internal/store"
	"github.com/rmkr-dev/sentinelmesh/internal/telemetryquery"
)

func TestIncidentFlowAndAIDegradation(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemory()
	_ = st.UpsertService(ctx, domain.Service{Name: "payment-service", Team: "payments", Owner: "payments", Criticality: "high", Environment: "local"})
	_ = st.UpsertSLO(ctx, domain.SLODefinition{
		ID: "payment-service:availability", Service: "payment-service", Name: "availability", Objective: 99.9, Indicator: "availability",
		Windows: []domain.SLOWindow{{Name: "5m", Duration: 5 * time.Minute, Compliance: true}},
	})
	now := time.Date(2026, 9, 28, 10, 34, 0, 0, time.UTC)
	_ = st.CreateDeployment(ctx, domain.Deployment{
		ID: "dep-1", Service: "payment-service", Version: "v1.8.2", GitSHA: "abc1234", Timestamp: now.Add(-3 * time.Minute), Environment: "local",
	})
	conv := telemetryquery.Conventions{}
	eng := &engine.Engine{
		Store: st,
		Deps: engine.Dependencies{Metrics: scripted{instant: map[string]float64{
			conv.TotalQuery("payment-service", "5m"): 100,
			conv.BadQuery("payment-service", "5m"):   25,
		}}},
		Now:       func() time.Time { return now },
		AI:        failingProvider{},
		AIEnabled: true,
	}
	srv := &Server{
		Store: st, Engine: eng, Demo: true,
		Gate: remediation.Gate{Policy: remediation.DefaultPolicy()},
	}
	h := srv.Handler()

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/engine/tick", nil)
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("tick %d %s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/incidents", nil))
	var listed struct {
		Incidents []domain.Incident `json:"incidents"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Incidents) != 1 {
		t.Fatalf("incidents=%d body=%s", len(listed.Incidents), rr.Body.String())
	}
	id := listed.Incidents[0].ID

	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/v1/incidents/"+id+"/analyze", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("analyze %d %s", rr.Code, rr.Body.String())
	}
	var analyzed domain.Incident
	if err := json.Unmarshal(rr.Body.Bytes(), &analyzed); err != nil {
		t.Fatal(err)
	}
	if analyzed.Analysis == nil || analyzed.Analysis.AIStatus != "degraded" {
		t.Fatalf("ai status %+v", analyzed.Analysis)
	}
	if analyzed.Analysis.Deterministic != true || analyzed.Analysis.Summary == "" {
		t.Fatal("deterministic summary missing")
	}

	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/incidents/"+id+"/postmortem", nil))
	if !strings.Contains(rr.Body.String(), "No confirmed root cause") {
		t.Fatalf("postmortem:\n%s", rr.Body.String())
	}

	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/v1/remediations", bytes.NewBufferString(`{"action":"rollback_deployment","target":"payment-service","reason":"correlation"}`)))
	if rr.Code != http.StatusForbidden {
		t.Fatalf("remediation should be disabled, got %d %s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/v1/demo/faults", bytes.NewBufferString(`{"name":"payment-latency"}`)))
	if rr.Code != http.StatusOK {
		t.Fatalf("fault %d %s", rr.Code, rr.Body.String())
	}
}

func TestAuthRequired(t *testing.T) {
	srv := &Server{Store: store.NewMemory(), Token: "secret", Demo: true}
	h := srv.Handler()
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/services", nil))
	if rr.Code != http.StatusUnauthorized {
		t.Fatal(rr.Code)
	}
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatal(rr.Code)
	}
}

type scripted struct{ instant map[string]float64 }

func (s scripted) Instant(_ context.Context, q string) (float64, bool, error) {
	v, ok := s.instant[q]
	return v, ok, nil
}

func (s scripted) Vector(context.Context, string) ([]telemetryquery.Sample, error) {
	return nil, nil
}

func (s scripted) Range(context.Context, string, time.Time, time.Time, time.Duration) ([]domain.MetricPoint, error) {
	return nil, nil
}

type failingProvider struct{}

func (failingProvider) Name() string { return "fail" }

func (failingProvider) Analyze(context.Context, domain.EvidencePack) (domain.Analysis, error) {
	return domain.Analysis{}, ai.ErrUnavailable
}
