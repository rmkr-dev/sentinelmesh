package shop

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

func TestOutboundRequestCarriesTraceparent(t *testing.T) {
	InstallPropagator()
	tp := sdktrace.NewTracerProvider()
	otel.SetTracerProvider(tp)
	t.Cleanup(func() {
		_ = tp.Shutdown(context.Background())
	})

	var header string
	var downstream trace.TraceID
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header = r.Header.Get("traceparent")
		ctx := otel.GetTextMapPropagator().Extract(r.Context(), propagation.HeaderCarrier(r.Header))
		span := trace.SpanFromContext(ctx)
		downstream = span.SpanContext().TraceID()
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	ctx, span := otel.Tracer("storefront").Start(context.Background(), "checkout")
	defer span.End()
	client := &http.Client{Transport: otelhttp.NewTransport(http.DefaultTransport)}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if header == "" {
		t.Fatal("traceparent was not sent")
	}
	if !downstream.IsValid() || downstream != span.SpanContext().TraceID() {
		t.Fatalf("downstream %s caller %s", downstream, span.SpanContext().TraceID())
	}
}
