// Package replay drives the engine from recorded Azure fixtures. No subscription is required.
package replay

import (
	"context"
	"os"
	"time"

	"github.com/rmkr-dev/sentinelmesh/internal/azure"
	"github.com/rmkr-dev/sentinelmesh/internal/correlation"
	"github.com/rmkr-dev/sentinelmesh/internal/domain"
	"github.com/rmkr-dev/sentinelmesh/internal/rca"
	"github.com/rmkr-dev/sentinelmesh/internal/topology"
)

// StorageThrottling groups a storage alert, a function error signal, and an activity-log write.
func StorageThrottling(ctx context.Context) (domain.Incident, error) {
	_ = ctx
	now := time.Date(2026, 9, 29, 12, 4, 0, 0, time.UTC)
	alertBody, err := os.ReadFile("testdata/azure/storage-throttle-alert.json")
	if err != nil {
		alertBody = defaultAlert
	}
	alerts, err := azure.ParseAlerts(alertBody)
	if err != nil {
		return domain.Incident{}, err
	}
	change := domain.Change{
		ID: "c1", Kind: "activity", Source: "activity_log", Target: "shopsa",
		Actor: "00000000-0000-0000-0000-000000000000", OccurredAt: now.Add(-4 * time.Minute),
		Attributes: map[string]string{"operation": "Microsoft.Storage/storageAccounts/write"},
	}
	health := domain.HealthEvent{Resource: "shopsa", State: "Degraded", Reason: "throttling", Source: "resource_health", At: now.Add(-2 * time.Minute)}
	signals := []domain.Signal{
		{ID: "slo-fn", Type: domain.SignalSLO, Service: "checkout-fn", Severity: "breached", Summary: "checkout-fn availability breached", OccurredAt: now, Attributes: map[string]string{"slo": "availability"}},
		{ID: "alert-storage", Type: domain.SignalPlatform, Service: "shopsa", Severity: alerts[0].Severity, Summary: alerts[0].Summary, OccurredAt: now.Add(-time.Minute), Attributes: map[string]string{"resource_id": alerts[0].Labels["resource_id"]}},
		{ID: "health", Type: domain.SignalResourceHealth, Service: "shopsa", Summary: "Azure reports shopsa Degraded", OccurredAt: health.At, Attributes: map[string]string{"state": health.State}},
		{ID: change.ID, Type: domain.SignalChange, Service: "shopsa", Summary: "storage account write", OccurredAt: change.OccurredAt, Attributes: change.Attributes},
	}
	g := topology.Graph{Edges: topology.FromCatalog([]domain.Service{
		{Name: "checkout-fn", Dependencies: []string{"shopsa"}},
	}, now)}
	groups := correlation.Correlate(signals, correlation.Options{
		Window: 10 * time.Minute, DeploymentLookback: 30 * time.Minute, Graph: g, MaxHops: 2,
		Dependencies: map[string][]string{"checkout-fn": {"shopsa"}},
	})
	if len(groups) != 1 {
		return domain.Incident{}, errString("expected one correlated group")
	}
	inc := domain.Incident{
		ID: "INC-REPLAY-1", Status: domain.StatusDetected, Service: "shopsa",
		RelatedServices: []string{"checkout-fn"}, StartedAt: groups[0].Started, DetectedAt: now,
		Signals: append(groups[0].Signals, groups[0].Context...),
	}
	analysis := rca.Analyze(rca.Pack{
		Incident: inc, Now: now,
		Changes: []domain.Signal{signals[3]},
	})
	inc.Analysis = &analysis
	inc.Evidence = analysis.Evidence
	_ = health
	return inc, nil
}

type errString string

func (e errString) Error() string { return string(e) }

var defaultAlert = []byte(`{"schemaId":"azureMonitorCommonAlertSchema","data":{"essentials":{"alertId":"a1","alertRule":"storage-throttle","severity":"Sev2","signalType":"Metric","monitorCondition":"Fired","alertTargetIDs":["/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/demo/providers/Microsoft.Storage/storageAccounts/shopsa"],"firedDateTime":"2026-09-29T12:03:00Z","description":"Storage throttling"}}}`)
