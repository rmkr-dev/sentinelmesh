# ADR-015 Alert normalization and noise control

## Status

Accepted

## Decision

Alertmanager, Azure Monitor, and Kubernetes alerts normalize to `domain.Alert`. The fingerprint is alert name plus service, not pod-specific labels. Flapping (three or more state changes inside the window) suppresses a new incident. Silences and maintenance windows are stored, audited, and checked before incident creation when the engine loads them.

## Consequences

A flapping alert annotates the existing incident instead of opening another one.
