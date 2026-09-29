# Traces

Jaeger receives traces from the collector over OTLP. The UI is `http://localhost:16686` locally.

The platform's evidence pack calls Jaeger's `/api/traces` for the incident's primary service and keeps a short summary: trace id, operation, status, duration, peer. It does not store the full span tree.

Health and readiness spans are filtered in the collector so they do not dominate the trace backend.
