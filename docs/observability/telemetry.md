# Telemetry conventions

Metrics use the HTTP duration histogram `http.server.request.duration` with unit seconds. After the collector's Prometheus exporter and resource-to-label conversion, queries use:

```text
http_server_request_duration_seconds_count{service_name="payment-service",http_response_status_code="500"}
```

The names are configurable under `telemetry` in `config/base.yaml`.

Spans are server spans from `otelhttp` plus client spans on outbound calls. Inventory adds a `SELECT products` span with `peer.service=shop-postgres`.

Logs are JSON through the OpenTelemetry slog bridge and include `trace_id` when a span is active.

Redaction runs in the collector and again in the platform. `internal/engine` scrubs log bodies, trace errors, and signal attributes before `SaveIncident` and before `ai.Provider.Analyze`. The alert webhook and event ingest apply the same policy. `telemetry.header_denylist`, `query_denylist`, and `redact_emails` in `config/base.yaml` build that policy. Tests cover bearer tokens, email addresses, and `card_token`.
