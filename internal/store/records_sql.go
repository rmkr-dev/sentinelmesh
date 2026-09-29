package store

import (
	"context"
	"time"

	"github.com/rmkr-dev/sentinelmesh/internal/domain"
)

func (p *Postgres) SaveResource(ctx context.Context, r domain.Resource) error {
	return p.upsert(ctx, `INSERT INTO resources (id, tenant_id, document) VALUES ($1, $2, $3)
		ON CONFLICT (id) DO UPDATE SET tenant_id = EXCLUDED.tenant_id, document = EXCLUDED.document`, r.ID, tenantOrDefault(r.Tenant), r)
}

func (p *Postgres) ListResources(ctx context.Context, kind, service string) ([]domain.Resource, error) {
	rows, err := p.pool.Query(ctx, `SELECT document FROM resources ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	all, err := collect[domain.Resource](rows)
	if err != nil {
		return nil, err
	}
	var out []domain.Resource
	for _, r := range all {
		if kind != "" && r.Kind != kind {
			continue
		}
		if service != "" && r.Service != service {
			continue
		}
		out = append(out, r)
	}
	return out, nil
}

func (p *Postgres) SaveEdge(ctx context.Context, e domain.TopologyEdge) error {
	id := e.From + "|" + e.To + "|" + e.Kind
	return p.upsert(ctx, `INSERT INTO topology_edges (id, document) VALUES ($1, $2)
		ON CONFLICT (id) DO UPDATE SET document = EXCLUDED.document`, id, e)
}

func (p *Postgres) ListEdges(ctx context.Context) ([]domain.TopologyEdge, error) {
	rows, err := p.pool.Query(ctx, `SELECT document FROM topology_edges`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collect[domain.TopologyEdge](rows)
}

func (p *Postgres) SaveChange(ctx context.Context, c domain.Change) error {
	return p.upsert(ctx, `INSERT INTO changes (id, occurred_at, document) VALUES ($1, $2, $3)
		ON CONFLICT (id) DO UPDATE SET occurred_at = EXCLUDED.occurred_at, document = EXCLUDED.document`, c.ID, c.OccurredAt, c)
}

func (p *Postgres) ListChanges(ctx context.Context, since time.Time) ([]domain.Change, error) {
	rows, err := p.pool.Query(ctx, `SELECT document FROM changes WHERE occurred_at >= $1 ORDER BY occurred_at`, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collect[domain.Change](rows)
}

func (p *Postgres) SaveHealth(ctx context.Context, h domain.HealthEvent) error {
	id := h.Resource + "|" + h.At.UTC().Format(time.RFC3339Nano)
	return p.upsert(ctx, `INSERT INTO health_events (id, occurred_at, document) VALUES ($1, $2, $3)
		ON CONFLICT (id) DO UPDATE SET document = EXCLUDED.document`, id, h.At, h)
}

func (p *Postgres) ListHealth(ctx context.Context, since time.Time) ([]domain.HealthEvent, error) {
	rows, err := p.pool.Query(ctx, `SELECT document FROM health_events WHERE occurred_at >= $1`, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collect[domain.HealthEvent](rows)
}

func (p *Postgres) SaveSilence(ctx context.Context, s domain.Silence) error {
	return p.upsert(ctx, `INSERT INTO silences (id, document) VALUES ($1, $2)
		ON CONFLICT (id) DO UPDATE SET document = EXCLUDED.document`, s.ID, s)
}

func (p *Postgres) ListSilences(ctx context.Context) ([]domain.Silence, error) {
	rows, err := p.pool.Query(ctx, `SELECT document FROM silences`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collect[domain.Silence](rows)
}

func (p *Postgres) SaveMaintenance(ctx context.Context, w domain.MaintenanceWindow) error {
	return p.upsert(ctx, `INSERT INTO maintenance_windows (id, document) VALUES ($1, $2)
		ON CONFLICT (id) DO UPDATE SET document = EXCLUDED.document`, w.ID, w)
}

func (p *Postgres) ListMaintenance(ctx context.Context) ([]domain.MaintenanceWindow, error) {
	rows, err := p.pool.Query(ctx, `SELECT document FROM maintenance_windows`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collect[domain.MaintenanceWindow](rows)
}

func (p *Postgres) TryLock(ctx context.Context, key int64) (bool, error) {
	if p.leader != nil {
		return true, nil
	}
	conn, err := p.pool.Acquire(ctx)
	if err != nil {
		return false, err
	}
	var got bool
	if err := conn.QueryRow(ctx, `SELECT pg_try_advisory_lock($1)`, key).Scan(&got); err != nil {
		conn.Release()
		return false, err
	}
	if !got {
		conn.Release()
		return false, nil
	}
	p.leader = conn
	return true, nil
}

func tenantOrDefault(v string) string {
	if v == "" {
		return "default"
	}
	return v
}
