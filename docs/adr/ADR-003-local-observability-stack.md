# ADR-003 Local observability stack

Status: accepted

`make up` runs Prometheus, Grafana, Jaeger, Loki, Alertmanager, the collector, PostgreSQL, the platform, the shop, and a traffic generator. Grafana anonymous access is viewer-only and is a local convenience. It is not the production setting.
