# Metrics

Prometheus scrapes the collector's `:8889` and the platform's `/metrics`.

Application metrics are the HTTP histogram recorded by the demo services. The OpenTelemetry HTTP handler still creates spans, and its automatic server metrics are dropped at scrape time so each request is counted once. A latency SLO uses the cumulative histogram bucket whose upper bound covers the threshold. Prometheus renders a 1 second boundary as `le="1.0"`, so the engine does not match the threshold as a raw label string.

Platform metrics include `platform_slo_status` (0 no data, 1 healthy, 2 at risk, 3 breached) and `platform_http_requests_total`.

Alert rules in `deploy/prometheus/rules.yml`:

- `HighErrorRate` when the 5xx ratio is above 5% for 30 seconds.
- `PaymentHighLatency` when payment p99 is above 500ms for 30 seconds.

Alertmanager groups by alert name and service and posts the Alertmanager payload to `/api/v1/alerts/webhook`. The correlation step is what prevents one failure from becoming one incident per alert.
