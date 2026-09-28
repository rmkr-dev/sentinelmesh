# Decisions index

The ADRs in [../adr](../adr/ADR-001-opentelemetry-first.md) are the record. Short version:

- OpenTelemetry is the telemetry contract.
- Azure Monitor is an exporter target, not a domain type.
- Platform state and telemetry stores stay separate.
- The model is optional and schema-limited.
- Remediation is off and requires a second person.
- Correlation prefers one incident per failure domain.
- SLO math is computed from good and total counts.
- GitHub deploy uses OIDC.
- Kubernetes install path is Helm.
