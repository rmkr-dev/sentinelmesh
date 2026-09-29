# Changelog

## Unreleased

- The CI gate ignores the marker scanner's own word list, and config tests ignore `DATABASE_URL` from the integration job.
- Alertmanager v4 webhooks, active-alert selection, token and OIDC roles, production config checks, Azure inventory polling, and a CI secret scan that fetches full history.
- End-to-end tests drive the API and engine: a storage alert, function errors, a change, and resource health become one incident; a silence blocks that alert; Kubernetes evidence is limited to the matching service.
- Kubernetes clients trust the service account CA and re-read the projected token after it expires. List calls follow continue tokens.
- Document trunk-based branches and the protection settings the owner still has to turn on. Pull requests fail when their own commits or body include tool attribution.
- CI installs Go from `go.mod` before a pinned govulncheck, and secret scanning runs even if that scan fails. Publishing to GHCR waits for a green `ci` run and logs in before the push.
- Shared telemetry bootstrap with runtime and host metrics, sampler, database spans, and optional Pyroscope.
- Kubernetes read-only adapter, multi-hop topology, alert noise control, robust anomaly detectors, and ranked hypotheses.
- Azure Resource Graph, Log Analytics, Azure Monitor PromQL, and the common alert schema.
- Versioned SQL migrations, advisory-lock leader election, OIDC role helpers, and escaped investigation UI output.
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
- Evidence-based RCA with optional OpenAI-compatible and Azure OpenAI providers. `ai.enabled=false` keeps analysis deterministic.
- Terraform profiles `azure-minimal` and `azure-production`.
- Helm chart, GitHub Actions, and the `obsctl` CLI.

Azure apply was not executed in the environment that produced this release. Terraform validation is the check that was run.
