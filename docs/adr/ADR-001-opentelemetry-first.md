# ADR-001 OpenTelemetry-first architecture

Status: accepted

The shop and any future service emit OTLP. The collector is the only place that knows about Jaeger, Loki, Prometheus, or Azure Monitor. Application code does not import an Azure Monitor distro.

Consequence: a new backend is a collector exporter, not a change to the incident model.
