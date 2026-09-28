package telemetryquery

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSelectLatencyBucket(t *testing.T) {
	samples := []Sample{
		{Labels: map[string]string{"le": "0.5"}, Value: 10},
		{Labels: map[string]string{"le": "1.0"}, Value: 40},
		{Labels: map[string]string{"le": "+Inf"}, Value: 42},
	}
	v, ok := SelectLatencyBucket(samples, 1)
	if !ok || v != 40 {
		t.Fatalf("got %v %v", v, ok)
	}
	v, ok = SelectLatencyBucket(samples, 0.4)
	if !ok || v != 10 {
		t.Fatalf("got %v %v", v, ok)
	}
}

func TestQueryBuilders(t *testing.T) {
	c := Conventions{}
	q := c.TotalQuery("payment-service", "5m")
	if q != `sum(increase(http_server_request_duration_seconds_count{service_name="payment-service"}[5m]))` {
		t.Fatal(q)
	}
	if c.BadQuery("payment-service", "5m") == q {
		t.Fatal("bad query should differ")
	}
}

func TestPrometheusInstant(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/query" {
			t.Fatal(r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[{"value":[1710000000,"12.5"]}]}}`))
	}))
	defer srv.Close()
	v, ok, err := (PrometheusClient{BaseURL: srv.URL}).Instant(context.Background(), "up")
	if err != nil || !ok || v != 12.5 {
		t.Fatalf("v=%v ok=%v err=%v", v, ok, err)
	}
}

func TestPrometheusEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[]}}`))
	}))
	defer srv.Close()
	_, ok, err := (PrometheusClient{BaseURL: srv.URL}).Instant(context.Background(), "up")
	if err != nil || ok {
		t.Fatal(ok, err)
	}
}

func TestJaegerSearch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"traceID":"abc","processes":{"p1":{"serviceName":"payment-service"}},"spans":[{"operationName":"charge","startTime":1710000000000000,"duration":1500000,"processID":"p1","tags":[{"key":"http.response.status_code","value":"500"},{"key":"peer.service","value":"processor"}]}]}]}`))
	}))
	defer srv.Close()
	samples, err := (JaegerClient{BaseURL: srv.URL}).Search(context.Background(), "payment-service", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(samples) != 1 || samples[0].Status != "error" || samples[0].Peer != "processor" {
		t.Fatalf("%+v", samples)
	}
	if time.Since(samples[0].StartedAt) < 0 {
		t.Fatal("timestamp in the future unexpectedly relative? still ok")
	}
}
