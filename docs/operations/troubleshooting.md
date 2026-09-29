# Troubleshooting

| Symptom | Check |
| --- | --- |
| `/ready` is 503 | Postgres is down or `DATABASE_URL` is wrong. |
| SLOs stay `no_data` | Prometheus has no `http_server_request_duration_seconds_count`. Confirm the collector is up and traffic is flowing. |
| No traces | Collector exporter to Jaeger. Jaeger UI service list. |
| No logs | Loki OTLP endpoint and `allow_structured_metadata: true`. |
| Incidents never open | `POST /api/v1/engine/tick` and read `/api/v1/slos`. A healthy SLI does not open an incident. |
| Analyze returns degraded | The model URL is down. The deterministic summary is still in the body. |
| Fault seems ignored | The service polls once a second. `GET /api/v1/demo/faults` shows the desired state. `DEMO_ENABLED` must be true. |
| Grafana panels empty | Datasource uid `prometheus` must match provisioning. Wait for a scrape. |

`make logs` follows the compose output.
