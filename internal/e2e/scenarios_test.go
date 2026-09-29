package e2e

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rmkr-dev/sentinelmesh/internal/api"
	"github.com/rmkr-dev/sentinelmesh/internal/domain"
	"github.com/rmkr-dev/sentinelmesh/internal/engine"
	"github.com/rmkr-dev/sentinelmesh/internal/kube"
	"github.com/rmkr-dev/sentinelmesh/internal/store"
	"github.com/rmkr-dev/sentinelmesh/internal/telemetryquery"
)

func TestAzureStorageThrottleOneIncident(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemory()
	now := time.Date(2026, 9, 29, 12, 4, 0, 0, time.UTC)
	resourceID := "/subscriptions/00000000-0000-0000-0000-000000000000/resourcegroups/demo/providers/microsoft.storage/storageaccounts/shopsa"
	if err := st.UpsertService(ctx, domain.Service{
		Name: "checkout-fn", Criticality: "high", Environment: "local", Dependencies: []string{"shopsa"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertService(ctx, domain.Service{
		Name: "shopsa", Criticality: "high", Environment: "local",
		Attributes: map[string]string{"azure.resource_ids": resourceID},
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertSLO(ctx, domain.SLODefinition{
		ID: "checkout-fn:availability", Service: "checkout-fn", Name: "availability", Objective: 99.9, Indicator: "availability",
		Windows: []domain.SLOWindow{{Name: "5m", Duration: 5 * time.Minute}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveChange(ctx, domain.Change{
		ID: "chg-1", Kind: "activity", Source: "activity_log", Target: "shopsa",
		Actor: "00000000-0000-0000-0000-000000000000", OccurredAt: now.Add(-4 * time.Minute),
		Attributes: map[string]string{"operation": "Microsoft.Storage/storageAccounts/write"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveHealth(ctx, domain.HealthEvent{
		Resource: "shopsa", State: "Degraded", Reason: "throttling", Source: "resource_health", At: now.Add(-2 * time.Minute),
	}); err != nil {
		t.Fatal(err)
	}
	conv := telemetryquery.Conventions{}
	eng := &engine.Engine{
		Store: st,
		Deps: engine.Dependencies{Metrics: scripted{instant: map[string]float64{
			conv.TotalQuery("checkout-fn", "5m"): 100,
			conv.BadQuery("checkout-fn", "5m"):   40,
		}}},
		Now: func() time.Time { return now },
	}
	body := readFixture(t, "storage-throttle-alert.json")
	h := (&api.Server{Store: st, Engine: eng}).Handler()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/alerts/azure-monitor", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("alert %d %s", rr.Code, rr.Body.String())
	}
	tick := httptest.NewRequest(http.MethodPost, "/api/v1/engine/tick", nil)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, tick)
	if rr.Code != http.StatusOK {
		t.Fatalf("tick %d %s", rr.Code, rr.Body.String())
	}
	incidents, err := st.ListIncidents(ctx, store.IncidentFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(incidents) != 1 {
		t.Fatalf("incidents=%d %+v", len(incidents), incidents)
	}
	inc := incidents[0]
	names := append([]string{inc.Service}, inc.RelatedServices...)
	if !has(names, "shopsa") || !has(names, "checkout-fn") {
		t.Fatalf("services primary=%s related=%v", inc.Service, inc.RelatedServices)
	}
	var change, health bool
	for _, s := range inc.Signals {
		if s.Type == domain.SignalChange {
			change = true
		}
		if s.Type == domain.SignalResourceHealth {
			health = true
		}
	}
	if !change || !health {
		t.Fatalf("missing change or health: %+v", inc.Signals)
	}
}

func TestSilenceBlocksAzureAlertIncident(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemory()
	now := time.Date(2026, 9, 29, 12, 4, 0, 0, time.UTC)
	resourceID := "/subscriptions/00000000-0000-0000-0000-000000000000/resourcegroups/demo/providers/microsoft.storage/storageaccounts/shopsa"
	_ = st.UpsertService(ctx, domain.Service{
		Name: "shopsa", Attributes: map[string]string{"azure.resource_ids": resourceID},
	})
	_ = st.SaveSilence(ctx, domain.Silence{
		ID: "sil-1", Owner: "sre", Reason: "maintenance",
		Matchers: map[string]string{"service": "shopsa"},
		StartsAt: now.Add(-time.Hour), EndsAt: now.Add(time.Hour),
	})
	eng := &engine.Engine{Store: st, Deps: engine.Dependencies{Metrics: scripted{instant: map[string]float64{}}}, Now: func() time.Time { return now }}
	h := (&api.Server{Store: st, Engine: eng}).Handler()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/alerts/azure-monitor", bytes.NewReader(readFixture(t, "storage-throttle-alert.json")))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatal(rr.Body.String())
	}
	if err := eng.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	incidents, _ := st.ListIncidents(ctx, store.IncidentFilter{})
	if len(incidents) != 0 {
		t.Fatalf("silence did not block incident: %+v", incidents)
	}
}

func TestCrashLoopEvidenceStaysOnItsService(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemory()
	now := time.Date(2026, 9, 29, 13, 0, 0, 0, time.UTC)
	_ = st.UpsertService(ctx, domain.Service{Name: "payment-service", Criticality: "high", Environment: "local"})
	_ = st.UpsertSLO(ctx, domain.SLODefinition{
		ID: "payment-service:availability", Service: "payment-service", Name: "availability", Objective: 99.9, Indicator: "availability",
		Windows: []domain.SLOWindow{{Name: "5m", Duration: 5 * time.Minute}},
	})
	kubeSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/events":
			_, _ = w.Write([]byte(`{"items":[
				{"type":"Warning","reason":"BackOff","message":"back-off restarting","metadata":{"namespace":"shop"},"involvedObject":{"kind":"Pod","name":"payment-abc"},"lastTimestamp":"2026-09-29T12:59:00Z"},
				{"type":"Warning","reason":"OOMKilled","message":"oom in other namespace","metadata":{"namespace":"other"},"involvedObject":{"kind":"Pod","name":"inventory-xyz"},"lastTimestamp":"2026-09-29T12:59:00Z"}
			]}`))
		case "/api/v1/pods":
			_, _ = w.Write([]byte(`{"items":[
				{"metadata":{"name":"payment-abc","namespace":"shop","labels":{"app.kubernetes.io/name":"payment-service"}},"status":{"containerStatuses":[{"state":{"waiting":{"reason":"CrashLoopBackOff"}}}]}},
				{"metadata":{"name":"inventory-xyz","namespace":"other","labels":{"app.kubernetes.io/name":"inventory-service"}},"status":{"containerStatuses":[{"state":{"terminated":{"reason":"OOMKilled"}}}]}}
			]}`))
		case "/api/v1/nodes":
			_, _ = w.Write([]byte(`{"items":[]}`))
		default:
			_, _ = w.Write([]byte(`{"items":[]}`))
		}
	}))
	defer kubeSrv.Close()
	conv := telemetryquery.Conventions{}
	eng := &engine.Engine{
		Store: st,
		Deps: engine.Dependencies{Metrics: scripted{instant: map[string]float64{
			conv.TotalQuery("payment-service", "5m"): 50,
			conv.BadQuery("payment-service", "5m"):   20,
		}}},
		Cluster: kube.Client{BaseURL: kubeSrv.URL, ServiceLabel: "app.kubernetes.io/name"},
		Now:     func() time.Time { return now },
	}
	if err := eng.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	incidents, _ := st.ListIncidents(ctx, store.IncidentFilter{})
	var payment domain.Incident
	for _, inc := range incidents {
		if inc.Service == "payment-service" || has(inc.RelatedServices, "payment-service") {
			payment = inc
			break
		}
	}
	if payment.ID == "" {
		t.Fatalf("no payment incident: %+v", incidents)
	}
	for _, s := range payment.Signals {
		if s.Service == "inventory-service" || s.Attributes["namespace"] == "other" {
			t.Fatalf("unrelated k8s signal on payment incident: %+v", s)
		}
	}
	if payment.Analysis != nil {
		for _, ev := range payment.Analysis.Evidence {
			if ev.Kind == "kubernetes" && (bytes.Contains([]byte(ev.Summary), []byte("other")) || bytes.Contains([]byte(ev.Summary), []byte("inventory"))) {
				t.Fatalf("unrelated evidence %s", ev.Summary)
			}
		}
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

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	path := filepath.Join("..", "..", "testdata", "azure", name)
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func has(ss []string, v string) bool {
	for _, s := range ss {
		if s == v {
			return true
		}
	}
	return false
}
