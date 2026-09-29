# App Service

Use the language OpenTelemetry SDK with OTLP environment variables pointed at the collector. Set `cloud.platform=azure_app_service` and `cloud.resource_id` to the site resource id.

App Service logs and the plan's CPU and memory metrics are collected by diagnostic settings and the `azuremonitor` receiver, not by an in-process Azure exporter.
