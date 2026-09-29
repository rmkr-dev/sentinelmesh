# ADR-013 Azure resource monitoring strategy

## Status

Accepted

## Decision

Three paths, used together:

1. Applications emit OTLP to the collector. The collector exports to Azure Monitor. Application code does not import an Azure Monitor distro.
2. Platform resources emit metrics through the collector `azuremonitor` receiver or diagnostic settings into Log Analytics.
3. The platform queries Azure with Entra ID: Resource Graph for inventory, Azure Monitor workspace PromQL for SLOs, Log Analytics KQL for logs and the Activity Log, and the common alert schema webhook for alerts.

A resource binds to a catalog service with the tag `sentinelmesh.service` or `Service.Attributes["azure.resource_ids"]`.

Activity Log rows are context. A platform health state can be a confirmed fact about Azure's report. It is not automatically the cause of an application symptom.

## Consequences

`azure-minimal` stays the cheap profile. `azure-full` is opt-in and was not applied in this environment. Replay tests cover the correlation path without a subscription.
