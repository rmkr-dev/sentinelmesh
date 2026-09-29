# Auto-instrumentation on AKS

The OpenTelemetry Operator is optional. Install it in the cluster, then apply an Instrumentation resource that exports to the agent collector:

```yaml
apiVersion: opentelemetry.io/v1alpha1
kind: Instrumentation
metadata:
  name: sentinelmesh
  namespace: shop
spec:
  exporter:
    endpoint: http://otel-collector-agent.observability.svc:4317
  propagators:
    - tracecontext
    - baggage
  java:
    image: ghcr.io/open-telemetry/opentelemetry-operator/autoinstrumentation-java:2.11.0
  nodejs:
    image: ghcr.io/open-telemetry/opentelemetry-operator/autoinstrumentation-nodejs:0.57.0
  python:
    image: ghcr.io/open-telemetry/opentelemetry-operator/autoinstrumentation-python:0.51b0
  dotnet:
    image: ghcr.io/open-telemetry/opentelemetry-operator/autoinstrumentation-dotnet:1.9.0
```

Annotate the workload with `instrumentation.opentelemetry.io/inject-java: "sentinelmesh"` (or `inject-nodejs`, `inject-python`, `inject-dotnet`).

This chart does not install the operator. Apply it only when you run non-Go workloads. The Go shop uses the shared `internal/telemetry` bootstrap instead.
