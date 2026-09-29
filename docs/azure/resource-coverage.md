# Azure resource coverage

Live-tested means this environment applied the resource and saw telemetry. Replay-tested means `go test ./internal/replay/` or `internal/azure` httptest fakes exercised the parser and correlation.

| Resource | Metrics | Logs | Changes | Health | Alerts | Live-tested |
| --- | --- | --- | --- | --- | --- | --- |
| Microsoft.Storage/storageAccounts | receiver defaults in `config/azure/resource-types` | diagnostic categories | Activity Log write | Resource Health | common alert schema | No. Replay-tested |
| Microsoft.Web/sites | Http5xx, requests | FunctionAppLogs | deployments/write | health check | common alert schema | No |
| Microsoft.App/containerApps | Requests, RestartCount | console logs | revision changes | replica count | common alert schema | No |
| Microsoft.ContainerService/managedClusters | node CPU and memory | Container Insights when `enable_container_insights` | cluster writes | node Ready | metric alerts | No. Terraform validates only |
| Microsoft.ServiceBus/namespaces | DLQ, backlog | diagnostic | namespace writes | resource health | metric alerts | No |

The metric names in `config/azure/resource-types/*.yaml` follow Microsoft's supported-metrics reference for those types. If a name drifts, the official list wins and the file should be edited.
