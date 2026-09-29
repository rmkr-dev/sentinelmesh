# ADR-011 Shared telemetry bootstrap and profiling

## Status

Accepted

## Decision

Shop services and the platform process share `internal/telemetry`. It sets the W3C propagator, a sampler from `OTEL_TRACES_SAMPLER`, resource attributes from the environment and the Kubernetes downward API, runtime and host metrics, and optional Pyroscope profiling behind `PROFILING_ENABLED`.

The OTLP profiles signal is still experimental. Pyroscope is the local profiling path until the SDK and collector support profiles as a stable signal.

## Consequences

Application code still exports OTLP only. Azure Monitor remains a collector backend.
