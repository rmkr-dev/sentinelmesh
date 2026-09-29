# Complete demo

About fifteen minutes. Start from a clean volume if you want the story to begin with no incidents: `make reset && make up`.

## 1. Start

```bash
make up
make status
```

Wait until `platform` is healthy.

## 2. Grafana

Open http://localhost:3000. The Observability folder contains Executive Overview, Service Overview, Kubernetes, Incident Investigation, and SLO. Anonymous viewers can read. Sign in as `admin` / `observability` to edit.

## 3. Application

Open http://localhost:8081 and buy an item. The path is storefront, gateway, order, inventory, payment, notification.

## 4. Traffic

The `trafficgen` service is already checking out at about two requests per second.

## 5. Healthy SLO

Open http://localhost:8080/ui/. After a minute of successful traffic, short-window SLIs should be healthy. A window with no samples stays `no_data`.

## 6. Bad version

```bash
make fault-enable FAULT=deployment-regression
```

This enables payment failures and latency, and records deployment `payment-service` `v1.8.2` with git sha `badbad1`.

## 7. Telemetry

- Prometheus: error ratio for `payment-service` and `order-service`.
- Jaeger: error traces for the charge span.
- Loki: error logs with `trace_id`. The card token does not appear.

## 8. Incident

The engine ticks every 10 seconds. You can also press nothing and wait, or run `make demo`, which ticks explicitly. One incident covers the payment failure and the order failure. Alerts that fire for both services join that incident.

## 9. Analyze

```bash
go run ./cmd/obsctl incident list
go run ./cmd/obsctl incident analyze INC-...
```

Or use the Analyze button in the UI.

## 10. AI output

With the default mock provider, the summary is marked `ai_status=completed` only after analyze, and it repeats the deterministic text. It does not add a new cause. If you point `AI_PROVIDER` at a broken URL, analyze still returns the incident with `ai_status=degraded`.

## 11. Deployment correlation

The timeline shows the deployment, then the error signal. The hypothesis grade is `strongly_correlated` unless the control-plane fault record is the stronger confirmed fact. Read the statement. It says the timing is not proof by itself. The confirmed grade, when present, cites the fault you just enabled.

## 12. Rollback

```bash
make fault-disable FAULT=deployment-regression
```

The platform records `v1.8.3` with sha `good123`. To exercise approval instead, set `remediation.enabled=true` in the local overlay, restart the platform, and have a second actor approve `rollback_deployment`. The default image ships with remediation disabled.

## 13. Recovery

Successful traffic replaces the bad samples in the short windows. The next ticks mark the incident `resolved` and append a recovery timeline event.

## 14. Budget

The SLO panel shows burn rate and remaining budget moving back toward healthy on the short windows. The 30 day window moves more slowly. That is the point of a long compliance window.

Other faults are listed by `obsctl demo fault list`.
