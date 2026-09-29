# ADR-002 Azure integration strategy

Status: accepted

Azure is the primary cloud, integrated in two places:

- Terraform modules for the resource group, Log Analytics, Application Insights, Azure Monitor workspace, identity, ACR, Key Vault, and, in the production profile, network, AKS, and Managed Grafana.
- Collector config `collectors/otel-collector-azure.yaml`, which follows the current Microsoft OTLP ingestion path: Data Collection Endpoint URLs, the `azure_auth` extension, and `cumulativetodelta` for metrics.

The Go domain does not contain Azure types. Connection strings are outputs marked sensitive and are never committed.
