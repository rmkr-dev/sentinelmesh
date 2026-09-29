CREATE TABLE IF NOT EXISTS services (
    name TEXT PRIMARY KEY,
    document JSONB NOT NULL
);

CREATE TABLE IF NOT EXISTS slo_definitions (
    id TEXT PRIMARY KEY,
    service TEXT NOT NULL,
    document JSONB NOT NULL
);

CREATE TABLE IF NOT EXISTS slo_results (
    service TEXT NOT NULL,
    slo_name TEXT NOT NULL,
    window_name TEXT NOT NULL,
    evaluated_at TIMESTAMPTZ NOT NULL,
    document JSONB NOT NULL,
    PRIMARY KEY (service, slo_name, window_name)
);

CREATE TABLE IF NOT EXISTS deployments (
    id TEXT PRIMARY KEY,
    service TEXT NOT NULL,
    occurred_at TIMESTAMPTZ NOT NULL,
    document JSONB NOT NULL
);

CREATE TABLE IF NOT EXISTS incidents (
    id TEXT PRIMARY KEY,
    service TEXT NOT NULL,
    status TEXT NOT NULL,
    detected_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    document JSONB NOT NULL
);

CREATE TABLE IF NOT EXISTS anomalies (
    id TEXT PRIMARY KEY,
    service TEXT NOT NULL,
    detected_at TIMESTAMPTZ NOT NULL,
    document JSONB NOT NULL
);

CREATE TABLE IF NOT EXISTS audit_log (
    id TEXT PRIMARY KEY,
    occurred_at TIMESTAMPTZ NOT NULL,
    document JSONB NOT NULL
);

CREATE TABLE IF NOT EXISTS faults (
    name TEXT PRIMARY KEY,
    document JSONB NOT NULL
);

CREATE TABLE IF NOT EXISTS alerts (
    id TEXT PRIMARY KEY,
    fingerprint TEXT NOT NULL,
    starts_at TIMESTAMPTZ NOT NULL,
    document JSONB NOT NULL
);

CREATE TABLE IF NOT EXISTS remediations (
    id TEXT PRIMARY KEY,
    incident_id TEXT NOT NULL DEFAULT '',
    document JSONB NOT NULL
);

CREATE TABLE IF NOT EXISTS incident_counter (
    year INT PRIMARY KEY,
    value INT NOT NULL
);
