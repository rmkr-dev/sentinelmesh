# Disaster recovery

## What is persistent

PostgreSQL volume `pgdata` holds platform state and the shop catalog/orders. Grafana, Prometheus, Loki, and Jaeger in the local compose file are ephemeral unless you add volumes. That is a local choice. Production Azure keeps Log Analytics and the Azure Monitor workspace, which are the telemetry systems of record.

## What can be rebuilt

Collector config, dashboards, SLO files, runbooks, and prompts are in git. Service binaries are rebuilt from this repo. Telemetry that was not exported during an outage is gone. That gap should appear as `no_data` or missing evidence, not as invented history.

## Local restore drill

```bash
make reset
make up
```

Catalog and SLO files are reloaded on startup. Incidents from the deleted volume are gone. That is the expected RPO for the local volume: whatever was on disk at the last write. RTO is the time to pull images and pass health checks, typically a few minutes on a warm cache.

## Production

Take Postgres backups on the schedule your database team already uses. Log Analytics retention is 30 days in the minimal profile; raise it in the monitoring module if the compliance window needs the samples. RPO for platform state is the Postgres backup interval. RPO for telemetry is the collector's retry queue, which is seconds to a few minutes, not a backup.

A practical test: delete a non-production platform database, restore the dump, start the process, and confirm `GET /api/v1/incidents` returns the restored ids. This environment did not run that test against Azure.
