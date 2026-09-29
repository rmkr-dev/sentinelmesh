# Changelog

## Unreleased

- CI grants `security-events: write` to the security job so the reusable workflow can start.
- Shop services propagate W3C trace context. `OTEL_PROPAGATORS` selects the formats.
- Stored evidence and the AI evidence pack are redacted with the configured policy.
- The remediation executor is selected from configuration. `kubernetes` uses the in-cluster service account.
- Trace and log searches are time-bounded interfaces.
- Anomaly rows upsert on a stable id and expire after the configured retention.
- API tokens are compared in constant time. A webhook token is accepted only on webhook routes. When a token is configured, `X-Actor` is ignored.

## 0.1.0

First release of the Cloud Observability & AIOps Platform.

- OpenTelemetry shop, collector, Prometheus, Grafana, Jaeger, Loki, and Alertmanager for local mode.
- SLO, anomaly, correlation, incident, runbook, and remediation packages.
- Evidence-based RCA with optional mock, OpenAI-compatible, and Azure OpenAI providers.
- Terraform profiles `azure-minimal` and `azure-production`.
- Helm chart, GitHub Actions, and the `obsctl` CLI.

Azure apply was not executed in the environment that produced this release. Terraform validation is the check that was run.
