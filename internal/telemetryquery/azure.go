package telemetryquery

import (
	"context"
	"time"

	"github.com/rmkr-dev/sentinelmesh/internal/azure"
	"github.com/rmkr-dev/sentinelmesh/internal/domain"
)

// AzureMetrics reads Azure Monitor Prometheus endpoints.
type AzureMetrics struct {
	Client   azure.Client
	Endpoint string
}

func (a AzureMetrics) Instant(ctx context.Context, promQL string) (float64, bool, error) {
	return a.Client.PromQL(ctx, a.Endpoint, promQL)
}

func (a AzureMetrics) Vector(ctx context.Context, promQL string) ([]Sample, error) {
	v, ok, err := a.Client.PromQL(ctx, a.Endpoint, promQL)
	if err != nil || !ok {
		return nil, err
	}
	return []Sample{{Value: v}}, nil
}

func (a AzureMetrics) Range(ctx context.Context, promQL string, start, end time.Time, step time.Duration) ([]domain.MetricPoint, error) {
	if step <= 0 {
		step = time.Minute
	}
	var out []domain.MetricPoint
	for at := start; !at.After(end); at = at.Add(step) {
		v, ok, err := a.Client.PromQL(ctx, a.Endpoint, promQL)
		if err != nil {
			return nil, err
		}
		if ok {
			out = append(out, domain.MetricPoint{Time: at, Value: v})
		}
		if len(out) == 1 {
			break
		}
	}
	return out, nil
}
