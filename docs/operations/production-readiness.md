# Production readiness

Use this list before pointing a real team at the chart.

## Security

- [ ] `PLATFORM_API_TOKEN` set and stored in Key Vault or the `platform-api` secret
- [ ] `DEMO_ENABLED=false`
- [ ] `remediation.enabled=false` until the approval workflow is staffed
- [ ] Collector identity is workload identity, not a connection string in a ConfigMap
- [ ] Grafana anonymous access disabled
- [ ] Network policy left enabled

## Observability

- [ ] Collector exporting to Azure Monitor and, if you keep them, the in-cluster backends
- [ ] Alertmanager or an action group pointed at the people who should see pages
- [ ] Dashboards loaded
- [ ] Trace sampling set with `OTEL_TRACES_SAMPLER` for production volume

## Data

- [ ] Postgres backups and a tested restore
- [ ] Log Analytics retention matches the longest SLO window you will defend

## Identity

- [ ] GitHub OIDC federated credential limited to the `production` environment
- [ ] AKS local accounts disabled (the production profile does this)

## SLO and incidents

- [ ] Each service has an owner, a criticality, and an SLO file
- [ ] Runbooks exist for the paging alerts
- [ ] Someone has practiced `obsctl incident analyze` against a non-production fault

## Cost

- [ ] You know whether the profile includes AKS. See [../cost.md](../cost.md).
