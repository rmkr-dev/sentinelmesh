# Container Apps

Send OTLP to the SentinelMesh collector. Do not add the Azure Monitor distro to the app.

On the Container Apps environment, set the OpenTelemetry destination to the collector OTLP endpoint. On each app set:

```text
OTEL_SERVICE_NAME=checkout
OTEL_RESOURCE_ATTRIBUTES=service.version=1.8.1,deployment.environment.name=production,cloud.provider=azure,cloud.platform=azure_container_apps,cloud.resource_id=/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/demo/providers/Microsoft.App/containerApps/checkout
```

Tag the app `sentinelmesh.service=checkout` so inventory binds it to the catalog.
