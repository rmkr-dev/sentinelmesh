# Local deployment

Requirements: Docker with Compose v2, and nothing else for `make up`. Go 1.25 is required to build binaries on the host.

```bash
cp .env.example .env
make up
make status
```

| Port | Service |
| --- | --- |
| 8080 | Platform API and investigation UI |
| 8081 | Storefront |
| 3000 | Grafana. Anonymous viewers can read. Admin password is `observability` unless `GRAFANA_ADMIN_PASSWORD` is set. |
| 16686 | Jaeger |
| 9090 | Prometheus |
| 9093 | Alertmanager |
| 3100 | Loki |
| 4317 / 4318 | Collector OTLP |

`make down` stops containers. `make reset` deletes the Postgres volume.

The platform container uses `STORE=postgres`. A host-run `go run ./cmd/platform` uses the memory store unless `DATABASE_URL` and `STORE=postgres` are set.

Faults:

```bash
make fault-enable FAULT=inventory-errors
make fault-disable FAULT=inventory-errors
```

Catalog: `payment-latency`, `inventory-errors`, `database-timeout`, `cpu-pressure`, `memory-pressure`, `dependency-outage`, `deployment-regression`.

Compose sets `PLATFORM_WEBHOOK_TOKEN=local-demo-webhook` and Alertmanager reads the same value from `deploy/alertmanager/webhook.token`. That token is accepted only on `/api/v1/alerts/webhook` and `/api/v1/events`. The API stays open until `PLATFORM_API_TOKEN` is set. The investigation UI sends a token from `sessionStorage` key `platformToken` when one has been saved in the token field. The shop fault poller sends `PLATFORM_API_TOKEN` when that variable is set.
