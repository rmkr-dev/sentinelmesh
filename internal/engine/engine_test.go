package engine

import (
	"context"
	"testing"
	"time"

	"github.com/rmkr-dev/sentinelmesh/internal/domain"
	"github.com/rmkr-dev/sentinelmesh/internal/store"
	"github.com/rmkr-dev/sentinelmesh/internal/telemetryquery"
)

type scripted struct {
	instant map[string]float64
	series  map[string][]domain.MetricPoint
}

func (s scripted) Instant(_ context.Context, q string) (float64, bool, error) {
	v, ok := s.instant[q]
	return v, ok, nil
}

func (s scripted) Vector(context.Context, string) ([]telemetryquery.Sample, error) {
	return nil, nil
}

func (s scripted) Range(_ context.Context, q string, _, _ time.Time, _ time.Duration) ([]domain.MetricPoint, error) {
	return s.series[q], nil
}

func TestFaultToIncidentToRecovery(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemory()
	now := time.Date(2026, 9, 28, 10, 34, 17, 0, time.UTC)
	if err := st.UpsertService(ctx, domain.Service{
		Name: "payment-service", Team: "payments", Owner: "payments-oncall", Criticality: "high", Environment: "local",
		Dependencies: []string{},
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertService(ctx, domain.Service{
		Name: "order-service", Team: "checkout", Owner: "checkout-oncall", Criticality: "high", Environment: "local",
		Dependencies: []string{"payment-service"},
	}); err != nil {
		t.Fatal(err)
	}
	def := domain.SLODefinition{
		ID: "payment-service:availability", Service: "payment-service", Name: "availability",
		Objective: 99.9, Indicator: "availability",
		Windows: []domain.SLOWindow{{Name: "5m", Duration: 5 * time.Minute, Compliance: true, BurnAlert: 2}},
	}
	if err := st.UpsertSLO(ctx, def); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertSLO(ctx, domain.SLODefinition{
		ID: "order-service:availability", Service: "order-service", Name: "availability",
		Objective: 99.9, Indicator: "availability",
		Windows: []domain.SLOWindow{{Name: "5m", Duration: 5 * time.Minute, Compliance: true, BurnAlert: 2}},
	}); err != nil {
		t.Fatal(err)
	}
	deployed := now.Add(-3 * time.Minute)
	if err := st.CreateDeployment(ctx, domain.Deployment{
		ID: "dep-1", Service: "payment-service", Version: "v1.8.2", Environment: "local",
		GitSHA: "badbad1", Repository: "shop", Author: "demo", Timestamp: deployed,
	}); err != nil {
		t.Fatal(err)
	}
	conv := telemetryquery.Conventions{}
	totalQ := conv.TotalQuery("payment-service", "5m")
	badQ := conv.BadQuery("payment-service", "5m")
	orderTotal := conv.TotalQuery("order-service", "5m")
	orderBad := conv.BadQuery("order-service", "5m")
	metrics := scripted{instant: map[string]float64{
		totalQ: 100, badQ: 40,
		orderTotal: 100, orderBad: 35,
	}}
	eng := &Engine{
		Store: st,
		Deps:  Dependencies{Metrics: metrics},
		Now:   func() time.Time { return now },
	}
	if err := eng.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	incidents, err := st.ListIncidents(ctx, store.IncidentFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(incidents) != 1 {
		t.Fatalf("incidents=%d", len(incidents))
	}
	inc := incidents[0]
	if inc.Service != "payment-service" && inc.Service != "order-service" {
		t.Fatal(inc.Service)
	}
	if len(inc.RelatedServices)+1 < 2 {
		t.Fatalf("expected both services grouped, related=%v primary=%s", inc.RelatedServices, inc.Service)
	}
	if inc.Analysis == nil || inc.Analysis.ConfidenceLabel != domain.GradeStronglyCorrelated {
		t.Fatalf("analysis=%+v", inc.Analysis)
	}
	if !inc.Analysis.RollbackRecommended {
		t.Fatal("expected rollback recommendation")
	}
	if inc.Analysis.AIStatus != "not_requested" {
		t.Fatal(inc.Analysis.AIStatus)
	}
	results, err := st.LatestSLOResults(ctx, "payment-service")
	if err != nil || len(results) != 1 || results[0].Status != domain.SLOBreached {
		t.Fatalf("%+v %v", results, err)
	}

	metrics.instant[badQ] = 0
	metrics.instant[orderBad] = 0
	eng.Now = func() time.Time { return now.Add(2 * time.Minute) }
	if err := eng.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetIncident(ctx, inc.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != domain.StatusResolved {
		t.Fatalf("status=%s", got.Status)
	}
	healthy, _ := st.LatestSLOResults(ctx, "payment-service")
	if healthy[0].Status != domain.SLOHealthy {
		t.Fatalf("%+v", healthy[0])
	}
}

func TestAIAbsenceDoesNotBlockIncident(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemory()
	now := time.Now().UTC()
	_ = st.UpsertService(ctx, domain.Service{Name: "inventory-service", Criticality: "medium", Environment: "local"})
	_ = st.UpsertSLO(ctx, domain.SLODefinition{
		ID: "inventory-service:availability", Service: "inventory-service", Name: "availability", Objective: 99.9, Indicator: "availability",
		Windows: []domain.SLOWindow{{Name: "5m", Duration: 5 * time.Minute, Compliance: true}},
	})
	conv := telemetryquery.Conventions{}
	metrics := scripted{instant: map[string]float64{
		conv.TotalQuery("inventory-service", "5m"): 50,
		conv.BadQuery("inventory-service", "5m"):   20,
	}}
	eng := &Engine{Store: st, Deps: Dependencies{Metrics: metrics}, Now: func() time.Time { return now }}
	if err := eng.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	list, _ := st.ListIncidents(ctx, store.IncidentFilter{})
	if len(list) != 1 {
		t.Fatalf("len=%d", len(list))
	}
	if list[0].Analysis == nil || list[0].Analysis.Deterministic != true {
		t.Fatal("expected deterministic analysis without AI")
	}
}

func TestLongWindowDoesNotHoldIncident(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemory()
	now := time.Date(2026, 9, 28, 11, 0, 0, 0, time.UTC)
	_ = st.UpsertService(ctx, domain.Service{Name: "payment-service", Criticality: "high", Environment: "local"})
	_ = st.UpsertSLO(ctx, domain.SLODefinition{
		ID: "payment-service:availability", Service: "payment-service", Name: "availability",
		Objective: 99.9, Indicator: "availability",
		Windows: []domain.SLOWindow{
			{Name: "5m", Duration: 5 * time.Minute, BurnAlert: 14.4},
			{Name: "1h", Duration: time.Hour, BurnAlert: 6},
			{Name: "30d", Duration: 30 * 24 * time.Hour, Compliance: true, BurnAlert: 1},
		},
	})
	conv := telemetryquery.Conventions{}
	metrics := scripted{instant: map[string]float64{
		conv.TotalQuery("payment-service", "5m"):  100,
		conv.BadQuery("payment-service", "5m"):    20,
		conv.TotalQuery("payment-service", "1h"):  100,
		conv.BadQuery("payment-service", "1h"):    20,
		conv.TotalQuery("payment-service", "30d"): 100,
		conv.BadQuery("payment-service", "30d"):   20,
	}}
	eng := &Engine{Store: st, Deps: Dependencies{Metrics: metrics}, Now: func() time.Time { return now }}
	if err := eng.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	list, err := st.ListIncidents(ctx, store.IncidentFilter{})
	if err != nil || len(list) != 1 {
		t.Fatalf("incidents=%d err=%v", len(list), err)
	}
	metrics.instant[conv.BadQuery("payment-service", "5m")] = 0
	eng.Now = func() time.Time { return now.Add(time.Minute) }
	if err := eng.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetIncident(ctx, list[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != domain.StatusResolved {
		t.Fatalf("status=%s", got.Status)
	}
	again, _ := st.ListIncidents(ctx, store.IncidentFilter{})
	if len(again) != 1 {
		t.Fatalf("long window opened another incident: %d", len(again))
	}
}
