package store

import (
	"context"
	"time"

	"github.com/rmkr-dev/sentinelmesh/internal/domain"
)

func (m *Memory) SaveResource(_ context.Context, r domain.Resource) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.resources {
		if m.resources[i].ID == r.ID {
			m.resources[i] = clone(r)
			return nil
		}
	}
	m.resources = append(m.resources, clone(r))
	return nil
}

func (m *Memory) ListResources(_ context.Context, kind, service string) ([]domain.Resource, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []domain.Resource
	for _, r := range m.resources {
		if kind != "" && r.Kind != kind {
			continue
		}
		if service != "" && r.Service != service {
			continue
		}
		out = append(out, clone(r))
	}
	return out, nil
}

func (m *Memory) SaveEdge(_ context.Context, e domain.TopologyEdge) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.edges = append(m.edges, clone(e))
	return nil
}

func (m *Memory) ListEdges(_ context.Context) ([]domain.TopologyEdge, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]domain.TopologyEdge, len(m.edges))
	for i := range m.edges {
		out[i] = clone(m.edges[i])
	}
	return out, nil
}

func (m *Memory) SaveChange(_ context.Context, c domain.Change) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.changes = append(m.changes, clone(c))
	return nil
}

func (m *Memory) ListChanges(_ context.Context, since time.Time) ([]domain.Change, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []domain.Change
	for _, c := range m.changes {
		if !since.IsZero() && c.OccurredAt.Before(since) {
			continue
		}
		out = append(out, clone(c))
	}
	return out, nil
}

func (m *Memory) SaveHealth(_ context.Context, h domain.HealthEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.health = append(m.health, clone(h))
	return nil
}

func (m *Memory) ListHealth(_ context.Context, since time.Time) ([]domain.HealthEvent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []domain.HealthEvent
	for _, h := range m.health {
		if !since.IsZero() && h.At.Before(since) {
			continue
		}
		out = append(out, clone(h))
	}
	return out, nil
}

func (m *Memory) SaveSilence(_ context.Context, s domain.Silence) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.silences {
		if m.silences[i].ID == s.ID {
			m.silences[i] = clone(s)
			return nil
		}
	}
	m.silences = append(m.silences, clone(s))
	return nil
}

func (m *Memory) ListSilences(_ context.Context) ([]domain.Silence, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]domain.Silence, len(m.silences))
	for i := range m.silences {
		out[i] = clone(m.silences[i])
	}
	return out, nil
}

func (m *Memory) SaveMaintenance(_ context.Context, w domain.MaintenanceWindow) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.maintenance = append(m.maintenance, clone(w))
	return nil
}

func (m *Memory) ListMaintenance(_ context.Context) ([]domain.MaintenanceWindow, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]domain.MaintenanceWindow, len(m.maintenance))
	for i := range m.maintenance {
		out[i] = clone(m.maintenance[i])
	}
	return out, nil
}

func (m *Memory) TryLock(context.Context, int64) (bool, error) { return true, nil }
