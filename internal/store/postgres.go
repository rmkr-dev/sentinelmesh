package store

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rmkr-dev/sentinelmesh/internal/domain"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

// Postgres stores platform state in PostgreSQL.
type Postgres struct {
	pool   *pgxpool.Pool
	leader *pgxpool.Conn
}

// NewPostgres opens a pool and applies migrations.
func NewPostgres(ctx context.Context, databaseURL string) (*Postgres, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, err
	}
	p := &Postgres{pool: pool}
	if err := p.migrate(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return p, nil
}

// Close releases the pool.
func (p *Postgres) Close() {
	if p.leader != nil {
		p.leader.Release()
		p.leader = nil
	}
	p.pool.Close()
}

func (p *Postgres) migrate(ctx context.Context) error {
	if _, err := p.pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version TEXT PRIMARY KEY,
		applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
	)`); err != nil {
		return err
	}
	entries, err := migrationFS.ReadDir("migrations")
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		var exists bool
		if err := p.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = $1)`, e.Name()).Scan(&exists); err != nil {
			return err
		}
		if exists {
			continue
		}
		b, err := migrationFS.ReadFile("migrations/" + e.Name())
		if err != nil {
			return err
		}
		tx, err := p.pool.Begin(ctx)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, string(b)); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("%s: %w", e.Name(), err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO schema_migrations (version) VALUES ($1)`, e.Name()); err != nil {
			_ = tx.Rollback(ctx)
			return err
		}
		if err := tx.Commit(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (p *Postgres) Ping(ctx context.Context) error { return p.pool.Ping(ctx) }

func (p *Postgres) UpsertService(ctx context.Context, svc domain.Service) error {
	return p.upsert(ctx, `INSERT INTO services (name, document) VALUES ($1, $2)
		ON CONFLICT (name) DO UPDATE SET document = EXCLUDED.document`, svc.Name, svc)
}

func (p *Postgres) ListServices(ctx context.Context) ([]domain.Service, error) {
	rows, err := p.pool.Query(ctx, `SELECT document FROM services ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collect[domain.Service](rows)
}

func (p *Postgres) GetService(ctx context.Context, name string) (domain.Service, error) {
	var svc domain.Service
	err := p.one(ctx, &svc, `SELECT document FROM services WHERE name = $1`, name)
	return svc, err
}

func (p *Postgres) UpsertSLO(ctx context.Context, def domain.SLODefinition) error {
	return p.upsert(ctx, `INSERT INTO slo_definitions (id, service, document) VALUES ($1, $2, $3)
		ON CONFLICT (id) DO UPDATE SET service = EXCLUDED.service, document = EXCLUDED.document`, def.ID, def.Service, def)
}

func (p *Postgres) ListSLOs(ctx context.Context) ([]domain.SLODefinition, error) {
	rows, err := p.pool.Query(ctx, `SELECT document FROM slo_definitions ORDER BY service, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collect[domain.SLODefinition](rows)
}

func (p *Postgres) SaveSLOResult(ctx context.Context, result domain.SLOResult) error {
	return p.upsert(ctx, `INSERT INTO slo_results (service, slo_name, window_name, evaluated_at, document)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (service, slo_name, window_name) DO UPDATE SET evaluated_at = EXCLUDED.evaluated_at, document = EXCLUDED.document`,
		result.Service, result.SLO, result.Window, result.EvaluatedAt, result)
}

func (p *Postgres) LatestSLOResults(ctx context.Context, service string) ([]domain.SLOResult, error) {
	q := `SELECT document FROM slo_results`
	args := []any{}
	if service != "" {
		q += ` WHERE service = $1`
		args = append(args, service)
	}
	q += ` ORDER BY service, slo_name, window_name`
	rows, err := p.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collect[domain.SLOResult](rows)
}

func (p *Postgres) CreateDeployment(ctx context.Context, d domain.Deployment) error {
	return p.upsert(ctx, `INSERT INTO deployments (id, service, occurred_at, document) VALUES ($1, $2, $3, $4)
		ON CONFLICT (id) DO UPDATE SET document = EXCLUDED.document`, d.ID, d.Service, d.Timestamp, d)
}

func (p *Postgres) ListDeployments(ctx context.Context, service string, limit int) ([]domain.Deployment, error) {
	if limit <= 0 {
		limit = 50
	}
	q := `SELECT document FROM deployments`
	args := []any{}
	if service != "" {
		q += ` WHERE service = $1`
		args = append(args, service)
	}
	args = append(args, limit)
	q += fmt.Sprintf(` ORDER BY occurred_at DESC LIMIT $%d`, len(args))
	rows, err := p.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collect[domain.Deployment](rows)
}

func (p *Postgres) NextIncidentID(ctx context.Context, now time.Time) (string, error) {
	year := now.UTC().Year()
	var value int
	err := p.pool.QueryRow(ctx, `INSERT INTO incident_counter (year, value) VALUES ($1, 1)
		ON CONFLICT (year) DO UPDATE SET value = incident_counter.value + 1
		RETURNING value`, year).Scan(&value)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("INC-%d-%04d", year, value), nil
}

func (p *Postgres) SaveIncident(ctx context.Context, inc domain.Incident) error {
	if inc.ID == "" {
		return fmt.Errorf("incident id is required")
	}
	return p.upsert(ctx, `INSERT INTO incidents (id, service, status, detected_at, updated_at, document)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (id) DO UPDATE SET service = EXCLUDED.service, status = EXCLUDED.status,
			detected_at = EXCLUDED.detected_at, updated_at = EXCLUDED.updated_at, document = EXCLUDED.document`,
		inc.ID, inc.Service, inc.Status, inc.DetectedAt, inc.UpdatedAt, inc)
}

func (p *Postgres) GetIncident(ctx context.Context, id string) (domain.Incident, error) {
	var inc domain.Incident
	err := p.one(ctx, &inc, `SELECT document FROM incidents WHERE id = $1`, id)
	return inc, err
}

func (p *Postgres) ListIncidents(ctx context.Context, filter IncidentFilter) ([]domain.Incident, error) {
	q := `SELECT document FROM incidents WHERE 1=1`
	args := []any{}
	if filter.Service != "" {
		args = append(args, filter.Service)
		q += fmt.Sprintf(` AND (service = $%d OR document->'related_services' ? $%d)`, len(args), len(args))
	}
	if filter.Status != "" {
		args = append(args, filter.Status)
		q += fmt.Sprintf(` AND status = $%d`, len(args))
	}
	q += ` ORDER BY detected_at DESC`
	if filter.Limit > 0 {
		args = append(args, filter.Limit)
		q += fmt.Sprintf(` LIMIT $%d`, len(args))
	}
	rows, err := p.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collect[domain.Incident](rows)
}

func (p *Postgres) SaveAnomaly(ctx context.Context, a domain.Anomaly) error {
	if a.ID == "" {
		a.ID = fmt.Sprintf("%s-%s-%d", a.Service, a.Detector, a.DetectedAt.UnixNano())
	}
	return p.upsert(ctx, `INSERT INTO anomalies (id, service, detected_at, document) VALUES ($1, $2, $3, $4)
		ON CONFLICT (id) DO UPDATE SET detected_at = EXCLUDED.detected_at, document = EXCLUDED.document`, a.ID, a.Service, a.DetectedAt, a)
}

func (p *Postgres) ListAnomalies(ctx context.Context, since time.Time, limit int) ([]domain.Anomaly, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := p.pool.Query(ctx, `SELECT document FROM anomalies WHERE detected_at >= $1 ORDER BY detected_at DESC LIMIT $2`, since, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collect[domain.Anomaly](rows)
}

func (p *Postgres) DeleteAnomaliesBefore(ctx context.Context, before time.Time) error {
	_, err := p.pool.Exec(ctx, `DELETE FROM anomalies WHERE detected_at < $1`, before)
	return err
}

func (p *Postgres) DeleteAlertsBefore(ctx context.Context, before time.Time) error {
	_, err := p.pool.Exec(ctx, `DELETE FROM alerts WHERE starts_at < $1`, before)
	return err
}

func (p *Postgres) AddAudit(ctx context.Context, ev domain.AuditEvent) error {
	return p.upsert(ctx, `INSERT INTO audit_log (id, occurred_at, document) VALUES ($1, $2, $3)`, ev.ID, ev.At, ev)
}

func (p *Postgres) ListAudit(ctx context.Context, limit int) ([]domain.AuditEvent, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := p.pool.Query(ctx, `SELECT document FROM audit_log ORDER BY occurred_at DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collect[domain.AuditEvent](rows)
}

func (p *Postgres) UpsertFault(ctx context.Context, f domain.Fault) error {
	return p.upsert(ctx, `INSERT INTO faults (name, document) VALUES ($1, $2)
		ON CONFLICT (name) DO UPDATE SET document = EXCLUDED.document`, f.Name, f)
}

func (p *Postgres) ListFaults(ctx context.Context) ([]domain.Fault, error) {
	rows, err := p.pool.Query(ctx, `SELECT document FROM faults ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collect[domain.Fault](rows)
}

func (p *Postgres) DeleteFault(ctx context.Context, name string) error {
	_, err := p.pool.Exec(ctx, `DELETE FROM faults WHERE name = $1`, name)
	return err
}

func (p *Postgres) SaveAlert(ctx context.Context, a domain.Alert) error {
	var prev domain.Alert
	err := p.one(ctx, &prev, `SELECT document FROM alerts WHERE id = $1 OR fingerprint = $2 ORDER BY starts_at ASC LIMIT 1`, a.ID, a.Fingerprint)
	if err == nil {
		a = mergeAlert(&prev, a)
	} else if errors.Is(err, ErrNotFound) {
		a = mergeAlert(nil, a)
	} else {
		return err
	}
	return p.upsert(ctx, `INSERT INTO alerts (id, fingerprint, starts_at, document) VALUES ($1, $2, $3, $4)
		ON CONFLICT (id) DO UPDATE SET fingerprint = EXCLUDED.fingerprint, document = EXCLUDED.document`, a.ID, a.Fingerprint, a.StartsAt, a)
}

func (p *Postgres) ListAlerts(ctx context.Context, since time.Time) ([]domain.Alert, error) {
	rows, err := p.pool.Query(ctx, `SELECT document FROM alerts WHERE starts_at >= $1 ORDER BY starts_at DESC`, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collect[domain.Alert](rows)
}

func (p *Postgres) ListActiveAlerts(ctx context.Context, since time.Time) ([]domain.Alert, error) {
	rows, err := p.pool.Query(ctx, `SELECT document FROM alerts
		WHERE document->>'status' = 'firing'
		   OR (document->>'status' = 'resolved' AND COALESCE(NULLIF(document->>'ends_at','')::timestamptz, starts_at) >= $1)
		ORDER BY starts_at DESC`, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collect[domain.Alert](rows)
}

func (p *Postgres) SaveRemediation(ctx context.Context, r domain.RemediationRequest) error {
	return p.upsert(ctx, `INSERT INTO remediations (id, incident_id, document) VALUES ($1, $2, $3)
		ON CONFLICT (id) DO UPDATE SET incident_id = EXCLUDED.incident_id, document = EXCLUDED.document`, r.ID, r.IncidentID, r)
}

func (p *Postgres) GetRemediation(ctx context.Context, id string) (domain.RemediationRequest, error) {
	var r domain.RemediationRequest
	err := p.one(ctx, &r, `SELECT document FROM remediations WHERE id = $1`, id)
	return r, err
}

func (p *Postgres) ListRemediations(ctx context.Context, incidentID string) ([]domain.RemediationRequest, error) {
	q := `SELECT document FROM remediations`
	args := []any{}
	if incidentID != "" {
		q += ` WHERE incident_id = $1`
		args = append(args, incidentID)
	}
	q += ` ORDER BY id DESC`
	rows, err := p.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collect[domain.RemediationRequest](rows)
}

func (p *Postgres) upsert(ctx context.Context, sql string, args ...any) error {
	encoded := make([]any, len(args))
	for i, a := range args {
		switch a.(type) {
		case string, time.Time:
			encoded[i] = a
		default:
			b, err := json.Marshal(a)
			if err != nil {
				return err
			}
			encoded[i] = b
		}
	}
	_, err := p.pool.Exec(ctx, sql, encoded...)
	return err
}

func (p *Postgres) one(ctx context.Context, dest any, sql string, args ...any) error {
	var raw []byte
	err := p.pool.QueryRow(ctx, sql, args...).Scan(&raw)
	if err != nil {
		if err == pgx.ErrNoRows {
			return ErrNotFound
		}
		return err
	}
	return json.Unmarshal(raw, dest)
}

func collect[T any](rows pgx.Rows) ([]T, error) {
	var out []T
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var item T
		if err := json.Unmarshal(raw, &item); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}
