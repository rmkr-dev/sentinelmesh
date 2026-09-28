// Package telemetryquery builds and executes read-only queries against
// Prometheus, Jaeger, and Loki. Telemetry is not copied into the platform database.
package telemetryquery

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/rmkr-dev/sentinelmesh/internal/domain"
)

// Sample is one instant-query series.
type Sample struct {
	Labels map[string]string
	Value  float64
}

// Conventions describe the metric names produced by the collector.
type Conventions struct {
	RequestMetric string
	ServiceLabel  string
	StatusLabel   string
}

func (c Conventions) withDefaults() Conventions {
	if c.RequestMetric == "" {
		c.RequestMetric = "http_server_request_duration_seconds"
	}
	if c.ServiceLabel == "" {
		c.ServiceLabel = "service_name"
	}
	if c.StatusLabel == "" {
		c.StatusLabel = "http_response_status_code"
	}
	return c
}

// TotalQuery is the PromQL for request count increase over a window.
func (c Conventions) TotalQuery(service, window string) string {
	c = c.withDefaults()
	return fmt.Sprintf(`sum(increase(%s_count{%s=%q}[%s]))`, c.RequestMetric, c.ServiceLabel, service, window)
}

// BadQuery is the PromQL for 5xx request count increase over a window.
func (c Conventions) BadQuery(service, window string) string {
	c = c.withDefaults()
	return fmt.Sprintf(`sum(increase(%s_count{%s=%q,%s=~"5.."}[%s]))`, c.RequestMetric, c.ServiceLabel, service, c.StatusLabel, window)
}

// LatencyBucketsQuery returns one series per histogram bucket.
// Callers pick the cumulative bucket that covers the SLO threshold. The
// Prometheus exporter renders a boundary of 1 as le="1.0", so an exact
// string match on the threshold is not reliable.
func (c Conventions) LatencyBucketsQuery(service, window string) string {
	c = c.withDefaults()
	return fmt.Sprintf(`sum by (le) (increase(%s_bucket{%s=%q}[%s]))`, c.RequestMetric, c.ServiceLabel, service, window)
}

// SelectLatencyBucket returns the cumulative count for the smallest bucket
// whose upper bound is at least thresholdSeconds. +Inf is the fallback.
func SelectLatencyBucket(samples []Sample, thresholdSeconds float64) (float64, bool) {
	best := math.Inf(1)
	var value float64
	found := false
	var inf float64
	hasInf := false
	for _, s := range samples {
		le := s.Labels["le"]
		if le == "+Inf" {
			inf = s.Value
			hasInf = true
			continue
		}
		n, err := strconv.ParseFloat(le, 64)
		if err != nil {
			continue
		}
		if n+1e-9 >= thresholdSeconds && n < best {
			best = n
			value = s.Value
			found = true
		}
	}
	if found {
		return value, true
	}
	if hasInf {
		return inf, true
	}
	return 0, false
}

// ErrorRatioQuery is a range query used by anomaly detection.
func (c Conventions) ErrorRatioQuery(service, rateWindow string) string {
	c = c.withDefaults()
	bad := fmt.Sprintf(`sum(rate(%s_count{%s=%q,%s=~"5.."}[%s]))`, c.RequestMetric, c.ServiceLabel, service, c.StatusLabel, rateWindow)
	total := fmt.Sprintf(`sum(rate(%s_count{%s=%q}[%s]))`, c.RequestMetric, c.ServiceLabel, service, rateWindow)
	return fmt.Sprintf(`(%s) / clamp_min(%s, 1e-9)`, bad, total)
}

// LatencyP99Query returns a p99-style histogram quantile over the rate window.
func (c Conventions) LatencyP99Query(service, rateWindow string) string {
	c = c.withDefaults()
	return fmt.Sprintf(`histogram_quantile(0.99, sum by (le) (rate(%s_bucket{%s=%q}[%s])))`, c.RequestMetric, c.ServiceLabel, service, rateWindow)
}

// MetricQuery reads scalar and range data.
type MetricQuery interface {
	Instant(ctx context.Context, promQL string) (float64, bool, error)
	Vector(ctx context.Context, promQL string) ([]Sample, error)
	Range(ctx context.Context, promQL string, start, end time.Time, step time.Duration) ([]domain.MetricPoint, error)
}

// PrometheusClient is the HTTP client for the Prometheus query API.
type PrometheusClient struct {
	BaseURL string
	HTTP    *http.Client
}

func (p PrometheusClient) client() *http.Client {
	if p.HTTP != nil {
		return p.HTTP
	}
	return &http.Client{Timeout: 10 * time.Second}
}

func (p PrometheusClient) Instant(ctx context.Context, promQL string) (float64, bool, error) {
	if p.BaseURL == "" {
		return 0, false, fmt.Errorf("prometheus url is empty")
	}
	u, err := url.Parse(p.BaseURL + "/api/v1/query")
	if err != nil {
		return 0, false, err
	}
	q := u.Query()
	q.Set("query", promQL)
	u.RawQuery = q.Encode()
	body, err := p.get(ctx, u.String())
	if err != nil {
		return 0, false, err
	}
	var parsed promResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return 0, false, err
	}
	if parsed.Status != "success" {
		return 0, false, fmt.Errorf("prometheus status %s", parsed.Status)
	}
	if len(parsed.Data.Result) == 0 {
		return 0, false, nil
	}
	v, ok := scalar(parsed.Data.Result[0].Value)
	return v, ok, nil
}

// Vector returns every series from an instant query.
func (p PrometheusClient) Vector(ctx context.Context, promQL string) ([]Sample, error) {
	if p.BaseURL == "" {
		return nil, fmt.Errorf("prometheus url is empty")
	}
	u, err := url.Parse(p.BaseURL + "/api/v1/query")
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("query", promQL)
	u.RawQuery = q.Encode()
	body, err := p.get(ctx, u.String())
	if err != nil {
		return nil, err
	}
	var parsed promResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, err
	}
	if parsed.Status != "success" {
		return nil, fmt.Errorf("prometheus status %s", parsed.Status)
	}
	var out []Sample
	for _, row := range parsed.Data.Result {
		v, ok := scalar(row.Value)
		if !ok {
			continue
		}
		out = append(out, Sample{Labels: row.Metric, Value: v})
	}
	return out, nil
}

func (p PrometheusClient) Range(ctx context.Context, promQL string, start, end time.Time, step time.Duration) ([]domain.MetricPoint, error) {
	if p.BaseURL == "" {
		return nil, fmt.Errorf("prometheus url is empty")
	}
	u, err := url.Parse(p.BaseURL + "/api/v1/query_range")
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("query", promQL)
	q.Set("start", strconv.FormatInt(start.Unix(), 10))
	q.Set("end", strconv.FormatInt(end.Unix(), 10))
	q.Set("step", strconv.Itoa(int(step.Seconds())))
	u.RawQuery = q.Encode()
	body, err := p.get(ctx, u.String())
	if err != nil {
		return nil, err
	}
	var parsed promRangeResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, err
	}
	if len(parsed.Data.Result) == 0 {
		return nil, nil
	}
	var points []domain.MetricPoint
	for _, sample := range parsed.Data.Result[0].Values {
		if len(sample) != 2 {
			continue
		}
		ts, _ := sample[0].(float64)
		raw, _ := sample[1].(string)
		val, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			continue
		}
		points = append(points, domain.MetricPoint{Time: time.Unix(int64(ts), 0).UTC(), Value: val})
	}
	return points, nil
}

func (p PrometheusClient) get(ctx context.Context, rawURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := p.client().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("prometheus %d", resp.StatusCode)
	}
	return body, nil
}

type promResponse struct {
	Status string `json:"status"`
	Data   struct {
		Result []struct {
			Metric map[string]string `json:"metric"`
			Value  []any             `json:"value"`
		} `json:"result"`
	} `json:"data"`
}

type promRangeResponse struct {
	Status string `json:"status"`
	Data   struct {
		Result []struct {
			Values [][]any `json:"values"`
		} `json:"result"`
	} `json:"data"`
}

func scalar(v []any) (float64, bool) {
	if len(v) != 2 {
		return 0, false
	}
	s, ok := v[1].(string)
	if !ok {
		return 0, false
	}
	n, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	return n, true
}

// JaegerClient reads trace summaries.
type JaegerClient struct {
	BaseURL string
	HTTP    *http.Client
}

func (j JaegerClient) client() *http.Client {
	if j.HTTP != nil {
		return j.HTTP
	}
	return &http.Client{Timeout: 10 * time.Second}
}

// Search returns recent traces for a service.
func (j JaegerClient) Search(ctx context.Context, service string, limit int) ([]domain.TraceSample, error) {
	if j.BaseURL == "" {
		return nil, fmt.Errorf("jaeger url is empty")
	}
	if limit <= 0 {
		limit = 20
	}
	u, err := url.Parse(j.BaseURL + "/api/traces")
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("service", service)
	q.Set("limit", strconv.Itoa(limit))
	q.Set("lookback", "1h")
	u.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := j.client().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("jaeger %d", resp.StatusCode)
	}
	var parsed struct {
		Data []struct {
			TraceID string `json:"traceID"`
			Spans   []struct {
				OperationName string `json:"operationName"`
				StartTime     int64  `json:"startTime"`
				Duration      int64  `json:"duration"`
				Tags          []struct {
					Key   string      `json:"key"`
					Value interface{} `json:"value"`
				} `json:"tags"`
				ProcessID string `json:"processID"`
			} `json:"spans"`
			Processes map[string]struct {
				ServiceName string `json:"serviceName"`
			} `json:"processes"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, err
	}
	var out []domain.TraceSample
	for _, tr := range parsed.Data {
		if len(tr.Spans) == 0 {
			continue
		}
		root := tr.Spans[0]
		status := "ok"
		errMsg := ""
		peer := ""
		for _, tag := range root.Tags {
			if tag.Key == "error" && fmt.Sprint(tag.Value) == "true" {
				status = "error"
			}
			if tag.Key == "otel.status_code" && fmt.Sprint(tag.Value) == "ERROR" {
				status = "error"
			}
			if tag.Key == "http.response.status_code" || tag.Key == "http.status_code" {
				code := fmt.Sprint(tag.Value)
				if len(code) > 0 && code[0] == '5' {
					status = "error"
					errMsg = "HTTP " + code
				}
			}
			if tag.Key == "peer.service" {
				peer = fmt.Sprint(tag.Value)
			}
		}
		svc := service
		if p, ok := tr.Processes[root.ProcessID]; ok && p.ServiceName != "" {
			svc = p.ServiceName
		}
		out = append(out, domain.TraceSample{
			TraceID:    tr.TraceID,
			Service:    svc,
			Operation:  root.OperationName,
			Status:     status,
			DurationMS: float64(root.Duration) / 1000,
			StartedAt:  time.UnixMicro(root.StartTime).UTC(),
			Error:      errMsg,
			Peer:       peer,
		})
	}
	return out, nil
}

// LokiClient reads recent log lines.
type LokiClient struct {
	BaseURL string
	HTTP    *http.Client
}

func (l LokiClient) client() *http.Client {
	if l.HTTP != nil {
		return l.HTTP
	}
	return &http.Client{Timeout: 10 * time.Second}
}

// Search returns recent log lines for a service label.
func (l LokiClient) Search(ctx context.Context, service string, limit int) ([]domain.LogExcerpt, error) {
	if l.BaseURL == "" {
		return nil, fmt.Errorf("loki url is empty")
	}
	if limit <= 0 {
		limit = 20
	}
	u, err := url.Parse(l.BaseURL + "/loki/api/v1/query_range")
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("query", fmt.Sprintf(`{service_name=%q}`, service))
	q.Set("limit", strconv.Itoa(limit))
	u.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := l.client().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("loki %d", resp.StatusCode)
	}
	var parsed struct {
		Data struct {
			Result []struct {
				Stream map[string]string `json:"stream"`
				Values [][]string        `json:"values"`
			} `json:"result"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, err
	}
	var out []domain.LogExcerpt
	for _, stream := range parsed.Data.Result {
		for _, row := range stream.Values {
			if len(row) < 2 {
				continue
			}
			ns, _ := strconv.ParseInt(row[0], 10, 64)
			out = append(out, domain.LogExcerpt{
				Timestamp: time.Unix(0, ns).UTC(),
				Service:   service,
				Severity:  stream.Stream["severity"],
				Body:      row[1],
				TraceID:   stream.Stream["trace_id"],
			})
		}
	}
	return out, nil
}
