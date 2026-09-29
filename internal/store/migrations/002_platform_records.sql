CREATE TABLE IF NOT EXISTS schema_migrations (
    version TEXT PRIMARY KEY,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE services ADD COLUMN IF NOT EXISTS tenant_id TEXT NOT NULL DEFAULT 'default';
ALTER TABLE incidents ADD COLUMN IF NOT EXISTS tenant_id TEXT NOT NULL DEFAULT 'default';
ALTER TABLE anomalies ADD COLUMN IF NOT EXISTS tenant_id TEXT NOT NULL DEFAULT 'default';
ALTER TABLE alerts ADD COLUMN IF NOT EXISTS tenant_id TEXT NOT NULL DEFAULT 'default';

CREATE INDEX IF NOT EXISTS incidents_tenant_status_idx ON incidents (tenant_id, status, updated_at);

CREATE TABLE IF NOT EXISTS resources (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL DEFAULT 'default',
    document JSONB NOT NULL
);

CREATE TABLE IF NOT EXISTS topology_edges (
    id TEXT PRIMARY KEY,
    document JSONB NOT NULL
);

CREATE TABLE IF NOT EXISTS changes (
    id TEXT PRIMARY KEY,
    occurred_at TIMESTAMPTZ NOT NULL,
    document JSONB NOT NULL
);

CREATE TABLE IF NOT EXISTS health_events (
    id TEXT PRIMARY KEY,
    occurred_at TIMESTAMPTZ NOT NULL,
    document JSONB NOT NULL
);

CREATE TABLE IF NOT EXISTS silences (
    id TEXT PRIMARY KEY,
    document JSONB NOT NULL
);

CREATE TABLE IF NOT EXISTS maintenance_windows (
    id TEXT PRIMARY KEY,
    document JSONB NOT NULL
);
