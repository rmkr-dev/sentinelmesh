package shop

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/contrib/propagators/autoprop"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/metric"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// Telemetry holds the SDK shutdown and the request histogram.
type Telemetry struct {
	Shutdown   func(context.Context) error
	Logger     *slog.Logger
	Duration   metric.Float64Histogram
	TracerName string
}

// InstallPropagator sets the process-wide propagator. OTEL_PROPAGATORS selects
// the formats. The default is W3C tracecontext plus baggage. Without this, the
// OpenTelemetry Go global propagator is a no-op and each service starts its own trace.
func InstallPropagator() {
	otel.SetTextMapPropagator(autoprop.NewTextMapPropagator())
}

// Setup configures OTLP traces, metrics, and logs. Export failures do not stop the process;
// the SDK retries in the background. If the endpoint is unset, stdout logging is still configured.
func Setup(ctx context.Context, service string) (Telemetry, error) {
	res, err := resource.New(ctx,
		resource.WithFromEnv(),
		resource.WithTelemetrySDK(),
		resource.WithHost(),
		resource.WithAttributes(semconv.ServiceName(service)),
	)
	if err != nil {
		return Telemetry{}, err
	}
	var shutdowns []func(context.Context) error

	traceExp, err := otlptracehttp.New(ctx)
	if err != nil {
		return Telemetry{}, fmt.Errorf("trace exporter: %w", err)
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExp),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)
	InstallPropagator()
	shutdowns = append(shutdowns, tp.Shutdown)

	metricExp, err := otlpmetrichttp.New(ctx)
	if err != nil {
		return Telemetry{}, fmt.Errorf("metric exporter: %w", err)
	}
	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExp, sdkmetric.WithInterval(5*time.Second))),
		sdkmetric.WithResource(res),
	)
	otel.SetMeterProvider(mp)
	shutdowns = append(shutdowns, mp.Shutdown)

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	logExp, err := otlploghttp.New(ctx)
	if err != nil {
		slog.Warn("log exporter disabled", "error", err.Error())
	} else {
		lp := sdklog.NewLoggerProvider(
			sdklog.WithProcessor(sdklog.NewBatchProcessor(logExp)),
			sdklog.WithResource(res),
		)
		global.SetLoggerProvider(lp)
		logger = otelslog.NewLogger(service, otelslog.WithLoggerProvider(lp))
		shutdowns = append(shutdowns, lp.Shutdown)
	}

	hist, err := mp.Meter(service).Float64Histogram(
		"http.server.request.duration",
		metric.WithUnit("s"),
		metric.WithDescription("HTTP server request duration."),
		metric.WithExplicitBucketBoundaries(0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.4, 0.5, 0.8, 1, 2.5, 5, 10),
	)
	if err != nil {
		return Telemetry{}, err
	}
	return Telemetry{
		Logger:     logger,
		Duration:   hist,
		TracerName: service,
		Shutdown: func(ctx context.Context) error {
			var joined error
			for _, fn := range shutdowns {
				if err := fn(ctx); err != nil {
					joined = err
				}
			}
			return joined
		},
	}, nil
}
