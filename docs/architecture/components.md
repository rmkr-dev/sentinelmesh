# Components

## Platform API

Listens on `:8080`. Serves `/health`, `/ready`, `/metrics`, `/api/v1/*`, and the investigation UI under `/ui/`.

Readiness checks PostgreSQL when `store=postgres`. It does not check the model.

## Sample shop

One binary, `demo`, selects its behavior from `SERVICE_NAME`. Checkout is `storefront → api-gateway → order-service → inventory, payment, notification`. Inventory and order use the `shop` database when `SHOP_DATABASE_URL` is set.

Each service polls `GET /api/v1/demo/faults` once a second. If the platform is unreachable, the last snapshot remains in effect and the service keeps serving.

## Collector

`collectors/otel-collector.yaml` receives OTLP, drops health spans, redacts secret-like keys, and exports traces to Jaeger, metrics to a Prometheus scrape endpoint, and logs to Loki's OTLP receiver. Memory limiting, batching, and exporter retry are enabled.

`collectors/otel-collector-azure.yaml` sends the same streams to Azure Monitor OTLP endpoints with the `azure_auth` extension. Metrics pass through `cumulativetodelta` because Application Insights experiences expect delta temporality.

## Backends

Prometheus evaluates `deploy/prometheus/rules.yml` and Alertmanager posts to `/api/v1/alerts/webhook`. Grafana is provisioned from `dashboards/grafana`. Jaeger and Loki are queried only while building an evidence pack. Those queries use `TraceSearcher` and `LogSearcher` and are limited to the incident window plus the deployment lookback.

## Wired and not wired

| Capability | State |
| --- | --- |
| W3C tracecontext and baggage propagation | Wired in `internal/shop` via `OTEL_PROPAGATORS` |
| Platform redaction before Postgres and the AI pack | Wired from `telemetry.header_denylist`, `query_denylist`, and `redact_emails` |
| Remediation executor `demo` or `kubernetes` | Wired. Kubernetes reads the in-cluster service account token |
| Stable anomaly ids and 7 day retention | Wired |
| API token compared in constant time, webhook token, actor taken from the authenticated principal | Wired when a token is configured |
| Kubernetes events inside RCA | `internal/e2e` proves a CrashLoop on payment-service does not cite an OOM from another namespace. Live kind and AKS were not used |
| Azure Monitor query adapters | Library plus httptest. SLO evaluation still reads Prometheus unless a backend URL is set. Not live-tested on a subscription |
| Kubernetes Grafana dashboard series | Local Compose does not run kubeletstats. The kind/Helm agent config is the source of those series |
