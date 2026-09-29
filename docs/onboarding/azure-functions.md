# Functions

Use OpenTelemetry mode in `host.json` and OTLP exporter settings. A minimal sample lives in `examples/azure/functions/`.

Required resource attributes: `service.name`, `service.version`, `deployment.environment.name`, `cloud.provider=azure`, `cloud.platform=azure_functions`, `cloud.resource_id`.

The function should export to the collector. Diagnostic settings on the Function App and its storage account are the platform path for host logs and throttling metrics.
