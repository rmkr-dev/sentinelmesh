# ADR-004 Telemetry storage separation

Status: accepted

PostgreSQL stores incidents, catalog, SLO definitions, latest SLO results, deployments, faults, remediations, and audit events. Spans, logs, and raw samples stay in the telemetry backends. The evidence pack stores short redacted excerpts, not a second copy of the backends.
