# AKS

The production Terraform profile creates a one-node cluster with OIDC issuer and workload identity enabled, Azure Policy, local accounts disabled, and the Azure CNI network policy.

```bash
az aks get-credentials -g <rg> -n <cluster>
helm upgrade --install platform helm/platform \
  --namespace observability --create-namespace \
  --set platform.demo=false \
  --set platform.env=production \
  --set collector.azure.enabled=true \
  --set collector.azure.tracesEndpoint="$AZURE_MONITOR_TRACES_ENDPOINT" \
  --set collector.azure.logsEndpoint="$AZURE_MONITOR_LOGS_ENDPOINT" \
  --set collector.azure.metricsEndpoint="$AZURE_MONITOR_METRICS_ENDPOINT"
```

Create the database URL and API token as secrets `platform-db` (key `url`) and `platform-api` (key `token`) before the pods start. Both secret references are optional so a first render succeeds; the process will not become ready without the database when `STORE=postgres`.

The chart's Role can delete pods and patch deployments. That is the Kubernetes executor's privilege. Leave `remediation.enabled` false until you intend to use it.

`deploy-aks.yml` is a manual workflow. It uses OIDC and the `production` environment.
