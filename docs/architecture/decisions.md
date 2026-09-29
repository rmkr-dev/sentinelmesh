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
- Later records: [ADR-011](../adr/ADR-011-shared-telemetry-bootstrap.md), [ADR-012](../adr/ADR-012-collector-topology.md), [ADR-013](../adr/ADR-013-azure-resource-monitoring.md), [ADR-014](../adr/ADR-014-topology-and-ranking.md), [ADR-015](../adr/ADR-015-alert-noise-control.md), [ADR-016](../adr/ADR-016-tenancy-and-authorization.md).
