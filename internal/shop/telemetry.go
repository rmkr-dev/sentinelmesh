package shop

import (
	"context"
	"log/slog"

	"github.com/rmkr-dev/sentinelmesh/internal/telemetry"
	"go.opentelemetry.io/otel/metric"
)

// Telemetry holds the SDK shutdown and the request histogram.
type Telemetry struct {
	Shutdown   func(context.Context) error
	Logger     *slog.Logger
	Duration   metric.Float64Histogram
	Orders     metric.Int64Counter
	Charge     metric.Float64Histogram
	TracerName string
}

// InstallPropagator sets the process-wide W3C propagator.
func InstallPropagator() { telemetry.InstallPropagator() }

// Setup configures OTLP traces, metrics, and logs through the shared bootstrap.
func Setup(ctx context.Context, service string) (Telemetry, error) {
	h, err := telemetry.Setup(ctx, telemetry.Options{ServiceName: service})
	if err != nil {
		return Telemetry{}, err
	}
	return Telemetry{
		Shutdown:   h.Shutdown,
		Logger:     h.Logger,
		Duration:   h.Duration,
		Orders:     h.Orders,
		Charge:     h.Charge,
		TracerName: service,
	}, nil
}
