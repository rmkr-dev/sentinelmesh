# Upgrades

The platform version is `internal/version.Version`, currently 0.1.0.

1. Read `CHANGELOG.md`.
2. Apply database migrations by starting the new process. Migrations are embedded SQL and are idempotent (`IF NOT EXISTS`).
3. Roll the collector after the platform if the metric names change. Update `telemetry.request_metric` in the same change as the dashboard queries.
4. Helm: `helm upgrade` with the same values. Probes gate the rollout.

Do not upgrade by editing a running container.
