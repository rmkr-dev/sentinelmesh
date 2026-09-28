# Azure deployment

## Prerequisites

- A subscription you are allowed to create resource groups in.
- `Contributor` on that subscription, plus `User Access Administrator` or `Role Based Access Control Administrator` so the plan can create role assignments.
- Terraform 1.6 or newer.
- The Azure CLI, logged in with `az login`. Do not paste a client secret into the repo.

## Profiles

| Profile | Creates | Does not create |
| --- | --- | --- |
| `azure-minimal` | Resource group, Log Analytics (30-day retention), Application Insights, Azure Monitor workspace, data collection endpoint, user-assigned identity, ACR Basic, Key Vault (RBAC), action group, metric alert | AKS, VNet, Managed Grafana |
| `azure-production` | The minimal set plus a VNet, NSG, one-node AKS (`Standard_D2s_v5`), and Azure Managed Grafana | A second region |

```bash
export ARM_SUBSCRIPTION_ID=...
cd infra/terraform
terraform init
terraform plan -var-file=profiles/azure-minimal.tfvars -var=subscription_id=$ARM_SUBSCRIPTION_ID
terraform apply -var-file=profiles/azure-minimal.tfvars -var=subscription_id=$ARM_SUBSCRIPTION_ID
```

`make azure-plan PROFILE=azure-minimal` is the same plan. State defaults to the local `terraform.tfstate` file, which is gitignored. For a shared team, copy `backend.tf.example` to a backend block that uses a storage account you already manage. Do not commit the state file. It contains the Application Insights connection string.

## OTLP

After Application Insights exists, turn on OTLP support in the portal (or your organization's equivalent template) and copy the traces, logs, and metrics endpoint URLs into `AZURE_MONITOR_TRACES_ENDPOINT`, `AZURE_MONITOR_LOGS_ENDPOINT`, and `AZURE_MONITOR_METRICS_ENDPOINT`. Point the collector at `collectors/otel-collector-azure.yaml`. The collector identity needs **Monitoring Metrics Publisher** on the data collection rule. The Terraform identity is granted that role on the Log Analytics workspace; grant it on the DCR as well once the DCR exists. Microsoft documents the DCR stream names `Microsoft-OTLP-Traces`, `Microsoft-OTLP-Logs`, and the metrics stream. This repo does not invent a DCR body the provider cannot express stably.

Collector version: 0.148.0 or newer if you rely on `azure_auth.use_default`. The local compose file pins 0.136.0, which does not need that extension.

## Cleanup

```bash
make azure-destroy PROFILE=azure-minimal
```

Key Vault soft-delete can keep the name reserved for seven days. Purge it in the portal if you need to recreate the same name immediately.

## What was not live-tested

No Azure credentials were available in the build environment. Syntax and provider schema are checked with `terraform validate`. An apply was not run.
