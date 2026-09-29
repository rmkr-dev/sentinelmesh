// Package telemetry is the shared OpenTelemetry bootstrap for the shop and the platform.
// Application code exports OTLP only. Azure Monitor is a collector concern.
package telemetry

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"time"

	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/contrib/instrumentation/host"
	"go.opentelemetry.io/contrib/instrumentation/runtime"
	"go.opentelemetry.io/contrib/propagators/autoprop"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
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
	"go.opentelemetry.io/otel/trace"
)

// Options configures one process.
type Options struct {
	ServiceName string
	Attrs       []attribute.KeyValue
}

// Handle is the process telemetry handle.
type Handle struct {
	Shutdown    func(context.Context) error
	Logger      *slog.Logger
	Meter       metric.Meter
	Tracer      trace.Tracer
	Duration    metric.Float64Histogram
	Orders      metric.Int64Counter
	Charge      metric.Float64Histogram
	stopProfile func()
}

// InstallPropagator sets the process-wide propagator from OTEL_PROPAGATORS.
// The default is W3C tracecontext plus baggage.
func InstallPropagator() {
	otel.SetTextMapPropagator(autoprop.NewTextMapPropagator())
}

// Setup starts OTLP traces, metrics, and logs. Export failures retry in the background.
func Setup(ctx context.Context, opt Options) (Handle, error) {
	if opt.ServiceName == "" {
		return Handle{}, fmt.Errorf("service name is required")
	}
	InstallPropagator()
	attrs := append(resourceAttributes(opt.ServiceName), opt.Attrs...)
	res, err := resource.New(ctx,
		resource.WithFromEnv(),
		resource.WithTelemetrySDK(),
		resource.WithHost(),
		resource.WithProcess(),
		resource.WithOS(),
		resource.WithContainer(),
		resource.WithAttributes(attrs...),
	)
	if err != nil {
		return Handle{}, err
	}
	var shutdowns []func(context.Context) error

	traceExp, err := otlptracehttp.New(ctx)
	if err != nil {
		return Handle{}, fmt.Errorf("trace exporter: %w", err)
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExp),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(samplerFromEnv()),
	)
	otel.SetTracerProvider(tp)
	shutdowns = append(shutdowns, tp.Shutdown)

	metricExp, err := otlpmetrichttp.New(ctx)
	if err != nil {
		return Handle{}, fmt.Errorf("metric exporter: %w", err)
	}
	reader := sdkmetric.NewPeriodicReader(metricExp, sdkmetric.WithInterval(5*time.Second))
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader), sdkmetric.WithResource(res))
	otel.SetMeterProvider(mp)
	shutdowns = append(shutdowns, mp.Shutdown)

	if err := runtime.Start(runtime.WithMeterProvider(mp), runtime.WithMinimumReadMemStatsInterval(15*time.Second)); err != nil {
		slog.Warn("runtime metrics disabled", "error", err.Error())
	}
	if err := host.Start(host.WithMeterProvider(mp)); err != nil {
		slog.Warn("host metrics disabled", "error", err.Error())
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if logExp, err := otlploghttp.New(ctx); err != nil {
		slog.Warn("log exporter disabled", "error", err.Error())
	} else {
		lp := sdklog.NewLoggerProvider(
			sdklog.WithProcessor(sdklog.NewBatchProcessor(logExp)),
			sdklog.WithResource(res),
		)
		global.SetLoggerProvider(lp)
		logger = otelslog.NewLogger(opt.ServiceName, otelslog.WithLoggerProvider(lp))
		shutdowns = append(shutdowns, lp.Shutdown)
	}

	meter := mp.Meter(opt.ServiceName)
	hist, err := meter.Float64Histogram(
		"http.server.request.duration",
		metric.WithUnit("s"),
		metric.WithDescription("HTTP server request duration."),
		metric.WithExplicitBucketBoundaries(0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.4, 0.5, 0.8, 1, 2.5, 5, 10),
	)
	if err != nil {
		return Handle{}, err
	}
	orders, err := meter.Int64Counter("checkout.orders", metric.WithDescription("Completed checkout attempts."))
	if err != nil {
		return Handle{}, err
	}
	charge, err := meter.Float64Histogram(
		"payment.charge.amount",
		metric.WithUnit("{cents}"),
		metric.WithDescription("Approved charge amount in cents. No card data."),
		metric.WithExplicitBucketBoundaries(100, 500, 1000, 2000, 5000, 10000),
	)
	if err != nil {
		return Handle{}, err
	}
	stopProfile := startProfiling(opt.ServiceName)
	return Handle{
		Logger:      logger,
		Meter:       meter,
		Tracer:      tp.Tracer(opt.ServiceName),
		Duration:    hist,
		Orders:      orders,
		Charge:      charge,
		stopProfile: stopProfile,
		Shutdown: func(ctx context.Context) error {
			if stopProfile != nil {
				stopProfile()
			}
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

func resourceAttributes(service string) []attribute.KeyValue {
	attrs := []attribute.KeyValue{semconv.ServiceName(service)}
	if v := os.Getenv("K8S_POD_NAME"); v != "" {
		attrs = append(attrs, semconv.K8SPodName(v))
	}
	if v := os.Getenv("K8S_NAMESPACE_NAME"); v != "" {
		attrs = append(attrs, semconv.K8SNamespaceName(v))
	}
	if v := os.Getenv("K8S_NODE_NAME"); v != "" {
		attrs = append(attrs, semconv.K8SNodeName(v))
	}
	if v := os.Getenv("K8S_DEPLOYMENT_NAME"); v != "" {
		attrs = append(attrs, semconv.K8SDeploymentName(v))
	}
	return attrs
}

func samplerFromEnv() sdktrace.Sampler {
	name := os.Getenv("OTEL_TRACES_SAMPLER")
	ratio := 1.0
	if v := os.Getenv("OTEL_TRACES_SAMPLER_ARG"); v != "" {
		if n, err := strconv.ParseFloat(v, 64); err == nil {
			ratio = n
		}
	}
	switch name {
	case "always_off":
		return sdktrace.NeverSample()
	case "always_on":
		return sdktrace.AlwaysSample()
	case "traceidratio":
		return sdktrace.TraceIDRatioBased(ratio)
	case "parentbased_always_off":
		return sdktrace.ParentBased(sdktrace.NeverSample())
	case "parentbased_traceidratio":
		return sdktrace.ParentBased(sdktrace.TraceIDRatioBased(ratio))
	default:
		return sdktrace.ParentBased(sdktrace.AlwaysSample())
	}
}
