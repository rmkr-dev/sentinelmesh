package correlation

import (
	"testing"
	"time"

	"github.com/rmkr-dev/sentinelmesh/internal/domain"
)

func at(min int) time.Time {
	return time.Date(2026, 9, 28, 10, min, 0, 0, time.UTC)
}

func TestOneIncidentForRelatedSymptoms(t *testing.T) {
	deps := map[string][]string{
		"order-service":     {"payment-service", "inventory-service"},
		"inventory-service": {"shop-postgres"},
	}
	signals := []domain.Signal{
		{ID: "1", Type: domain.SignalAlert, Service: "payment-service", Summary: "500 errors", OccurredAt: at(34)},
		{ID: "2", Type: domain.SignalAnomaly, Service: "payment-service", Summary: "latency spike", OccurredAt: at(35)},
		{ID: "3", Type: domain.SignalK8s, Service: "payment-service", Summary: "pod restarts", OccurredAt: at(35)},
		{ID: "4", Type: domain.SignalTrace, Service: "inventory-service", Summary: "database timeout", OccurredAt: at(34), Attributes: map[string]string{"peer": "shop-postgres"}},
		{ID: "5", Type: domain.SignalAlert, Service: "order-service", Summary: "checkout errors", OccurredAt: at(36)},
		{ID: "6", Type: domain.SignalDeployment, Service: "payment-service", Summary: "deployed v1.8.2", OccurredAt: at(31)},
		{ID: "7", Type: domain.SignalAlert, Service: "notification-service", Summary: "unrelated", OccurredAt: at(90)},
	}
	groups := Correlate(signals, Options{
		Window:             5 * time.Minute,
		DeploymentLookback: 30 * time.Minute,
		Dependencies:       deps,
	})
	if len(groups) != 2 {
		t.Fatalf("groups = %d, want 2 (checkout blast radius + later notification)", len(groups))
	}
	var checkout Group
	for _, g := range groups {
		if len(g.Signals) > len(checkout.Signals) {
			checkout = g
		}
	}
	if len(checkout.Signals) != 5 {
		t.Fatalf("checkout signals = %d", len(checkout.Signals))
	}
	if len(checkout.Context) != 1 || checkout.Context[0].Type != domain.SignalDeployment {
		t.Fatalf("context = %+v", checkout.Context)
	}
}

func TestTraceIDJoinsUnrelatedNames(t *testing.T) {
	signals := []domain.Signal{
		{ID: "1", Type: domain.SignalTrace, Service: "a", OccurredAt: at(1), Attributes: map[string]string{"trace_id": "abc"}},
		{ID: "2", Type: domain.SignalLog, Service: "b", OccurredAt: at(2), Attributes: map[string]string{"trace_id": "abc"}},
	}
	groups := Correlate(signals, Options{Window: time.Minute})
	if len(groups) != 1 {
		t.Fatalf("groups=%d", len(groups))
	}
}

func BenchmarkCorrelate(b *testing.B) {
	signals := make([]domain.Signal, 0, 500)
	for i := 0; i < 500; i++ {
		signals = append(signals, domain.Signal{
			ID:         "s",
			Type:       domain.SignalAlert,
			Service:    "payment-service",
			OccurredAt: at(0).Add(time.Duration(i) * time.Second),
		})
	}
	opts := Options{Window: 5 * time.Minute}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Correlate(signals, opts)
	}
}
