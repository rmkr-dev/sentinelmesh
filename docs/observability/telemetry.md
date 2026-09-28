# Telemetry conventions

Metrics use the HTTP duration histogram `http.server.request.duration` with unit seconds. After the collector's Prometheus exporter and resource-to-label conversion, queries use:

```text
http_server_request_duration_seconds_count{service_name="payment-service",http_response_status_code="500"}
```

The names are configurable under `telemetry` in `config/base.yaml`.

Spans are server spans from `otelhttp` plus client spans on outbound calls. Inventory adds a `SELECT products` span with `peer.service=shop-postgres`.

Logs are JSON through the OpenTelemetry slog bridge and include `trace_id` when a span is active.

Redaction runs twice: the collector drops secret-like attribute keys, and `internal/redaction` scrubs anything the platform would persist or send to a model. Tests cover authorization headers, cookies, `card_token`, query parameter `token`, email addresses, and AWS-style access key ids.
