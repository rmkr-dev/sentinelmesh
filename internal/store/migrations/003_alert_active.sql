CREATE INDEX IF NOT EXISTS alerts_status_starts_idx ON alerts ((document->>'status'), starts_at);
