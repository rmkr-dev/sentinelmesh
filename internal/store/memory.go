package store

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/rmkr-dev/sentinelmesh/internal/domain"
)

// Memory is an ephemeral store for tests and explicit local runs.
type Memory struct {
	mu           sync.Mutex
	services     map[string]domain.Service
	slos         map[string]domain.SLODefinition
	sloResults   []domain.SLOResult
	deployments  []domain.Deployment
	incidents    map[string]domain.Incident
	anomalies    []domain.Anomaly
	audit        []domain.AuditEvent
	faults       map[string]domain.Fault
	alerts       []domain.Alert
	remediations map[string]domain.RemediationRequest
	resources    []domain.Resource
	edges        []domain.TopologyEdge
	changes      []domain.Change
	health       []domain.HealthEvent
	silences     []domain.Silence
	maintenance  []domain.MaintenanceWindow
	seqYear      int
	seq          int
}

// NewMemory returns an empty store.
func NewMemory() *Memory {
	return &Memory{
		services:     map[string]domain.Service{},
		slos:         map[string]domain.SLODefinition{},
		incidents:    map[string]domain.Incident{},
		faults:       map[string]domain.Fault{},
		remediations: map[string]domain.RemediationRequest{},
	}
}

func (m *Memory) Ping(context.Context) error { return nil }

func clone[T any](v T) T {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	var out T
	if err := json.Unmarshal(b, &out); err != nil {
		panic(err)
	}
	return out
}

func (m *Memory) UpsertService(_ context.Context, svc domain.Service) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.services[svc.Name] = clone(svc)
	return nil
}

func (m *Memory) ListServices(context.Context) ([]domain.Service, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]domain.Service, 0, len(m.services))
	for _, s := range m.services {
		out = append(out, clone(s))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (m *Memory) GetService(_ context.Context, name string) (domain.Service, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.services[name]
	if !ok {
		return domain.Service{}, ErrNotFound
	}
	return clone(s), nil
}

func (m *Memory) UpsertSLO(_ context.Context, def domain.SLODefinition) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.slos[def.ID] = clone(def)
	return nil
}

func (m *Memory) ListSLOs(context.Context) ([]domain.SLODefinition, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]domain.SLODefinition, 0, len(m.slos))
	for _, s := range m.slos {
		out = append(out, clone(s))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Service == out[j].Service {
			return out[i].Name < out[j].Name
		}
		return out[i].Service < out[j].Service
	})
	return out, nil
}

func (m *Memory) SaveSLOResult(_ context.Context, result domain.SLOResult) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	replaced := false
	for i := range m.sloResults {
		if m.sloResults[i].Service == result.Service && m.sloResults[i].SLO == result.SLO && m.sloResults[i].Window == result.Window {
			m.sloResults[i] = clone(result)
			replaced = true
		}
	}
	if !replaced {
		m.sloResults = append(m.sloResults, clone(result))
	}
	return nil
}

func (m *Memory) LatestSLOResults(_ context.Context, service string) ([]domain.SLOResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []domain.SLOResult
	for _, r := range m.sloResults {
		if service == "" || r.Service == service {
			out = append(out, clone(r))
		}
	}
	return out, nil
}

func (m *Memory) CreateDeployment(_ context.Context, d domain.Deployment) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.deployments = append(m.deployments, clone(d))
	return nil
}

func (m *Memory) ListDeployments(_ context.Context, service string, limit int) ([]domain.Deployment, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []domain.Deployment
	for i := len(m.deployments) - 1; i >= 0; i-- {
		d := m.deployments[i]
		if service != "" && d.Service != service {
			continue
		}
		out = append(out, clone(d))
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (m *Memory) NextIncidentID(_ context.Context, now time.Time) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	year := now.UTC().Year()
	if m.seqYear != year {
		m.seqYear = year
		m.seq = 0
	}
	m.seq++
	return fmt.Sprintf("INC-%d-%04d", year, m.seq), nil
}

func (m *Memory) SaveIncident(_ context.Context, inc domain.Incident) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if inc.ID == "" {
		return fmt.Errorf("incident id is required")
	}
	m.incidents[inc.ID] = clone(inc)
	return nil
}

func (m *Memory) GetIncident(_ context.Context, id string) (domain.Incident, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	inc, ok := m.incidents[id]
	if !ok {
		return domain.Incident{}, ErrNotFound
	}
	return clone(inc), nil
}

func (m *Memory) ListIncidents(_ context.Context, filter IncidentFilter) ([]domain.Incident, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []domain.Incident
	for _, inc := range m.incidents {
		if filter.Service != "" && inc.Service != filter.Service && !has(inc.RelatedServices, filter.Service) {
			continue
		}
		if filter.Status != "" && inc.Status != filter.Status {
			continue
		}
		out = append(out, clone(inc))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].DetectedAt.After(out[j].DetectedAt) })
	if filter.Limit > 0 && len(out) > filter.Limit {
		out = out[:filter.Limit]
	}
	return out, nil
}

func (m *Memory) SaveAnomaly(_ context.Context, a domain.Anomaly) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.anomalies {
		if m.anomalies[i].ID == a.ID {
			m.anomalies[i] = clone(a)
			return nil
		}
	}
	m.anomalies = append(m.anomalies, clone(a))
	return nil
}

func (m *Memory) DeleteAnomaliesBefore(_ context.Context, before time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	kept := make([]domain.Anomaly, 0, len(m.anomalies))
	for _, a := range m.anomalies {
		if a.DetectedAt.Before(before) {
			continue
		}
		kept = append(kept, a)
	}
	m.anomalies = kept
	return nil
}

func (m *Memory) DeleteAlertsBefore(_ context.Context, before time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	kept := make([]domain.Alert, 0, len(m.alerts))
	for _, a := range m.alerts {
		if a.StartsAt.Before(before) {
			continue
		}
		kept = append(kept, a)
	}
	m.alerts = kept
	return nil
}

func (m *Memory) ListAnomalies(_ context.Context, since time.Time, limit int) ([]domain.Anomaly, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []domain.Anomaly
	for i := len(m.anomalies) - 1; i >= 0; i-- {
		a := m.anomalies[i]
		if !since.IsZero() && a.DetectedAt.Before(since) {
			continue
		}
		out = append(out, clone(a))
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (m *Memory) AddAudit(_ context.Context, ev domain.AuditEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.audit = append(m.audit, clone(ev))
	return nil
}

func (m *Memory) ListAudit(_ context.Context, limit int) ([]domain.AuditEvent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []domain.AuditEvent
	for i := len(m.audit) - 1; i >= 0; i-- {
		out = append(out, clone(m.audit[i]))
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (m *Memory) UpsertFault(_ context.Context, f domain.Fault) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.faults[f.Name] = clone(f)
	return nil
}

func (m *Memory) ListFaults(context.Context) ([]domain.Fault, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]domain.Fault, 0, len(m.faults))
	for _, f := range m.faults {
		out = append(out, clone(f))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (m *Memory) DeleteFault(_ context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.faults, name)
	return nil
}

func (m *Memory) SaveAlert(_ context.Context, a domain.Alert) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.alerts {
		if m.alerts[i].Fingerprint == a.Fingerprint && m.alerts[i].StartsAt.Equal(a.StartsAt) {
			m.alerts[i] = clone(a)
			return nil
		}
	}
	m.alerts = append(m.alerts, clone(a))
	return nil
}

func (m *Memory) ListAlerts(_ context.Context, since time.Time) ([]domain.Alert, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []domain.Alert
	for _, a := range m.alerts {
		if !since.IsZero() && a.StartsAt.Before(since) {
			continue
		}
		out = append(out, clone(a))
	}
	return out, nil
}

func (m *Memory) SaveRemediation(_ context.Context, r domain.RemediationRequest) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.remediations[r.ID] = clone(r)
	return nil
}

func (m *Memory) GetRemediation(_ context.Context, id string) (domain.RemediationRequest, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.remediations[id]
	if !ok {
		return domain.RemediationRequest{}, ErrNotFound
	}
	return clone(r), nil
}

func (m *Memory) ListRemediations(_ context.Context, incidentID string) ([]domain.RemediationRequest, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []domain.RemediationRequest
	for _, r := range m.remediations {
		if incidentID != "" && r.IncidentID != incidentID {
			continue
		}
		out = append(out, clone(r))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

func has(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}
