// Package store persists platform state. Telemetry stays in the observability backends.
package store

import (
	"context"
	"errors"
	"time"

	"github.com/rmkr-dev/sentinelmesh/internal/domain"
)

// ErrNotFound is returned when a record does not exist.
var ErrNotFound = errors.New("not found")

// IncidentFilter limits incident listings.
type IncidentFilter struct {
	Service string
	Status  string
	Limit   int
}

// Store is the platform state interface. Memory and Postgres both implement it.
type Store interface {
	Ping(ctx context.Context) error

	UpsertService(ctx context.Context, svc domain.Service) error
	ListServices(ctx context.Context) ([]domain.Service, error)
	GetService(ctx context.Context, name string) (domain.Service, error)

	UpsertSLO(ctx context.Context, def domain.SLODefinition) error
	ListSLOs(ctx context.Context) ([]domain.SLODefinition, error)
	SaveSLOResult(ctx context.Context, result domain.SLOResult) error
	LatestSLOResults(ctx context.Context, service string) ([]domain.SLOResult, error)

	CreateDeployment(ctx context.Context, d domain.Deployment) error
	ListDeployments(ctx context.Context, service string, limit int) ([]domain.Deployment, error)

	NextIncidentID(ctx context.Context, now time.Time) (string, error)
	SaveIncident(ctx context.Context, inc domain.Incident) error
	GetIncident(ctx context.Context, id string) (domain.Incident, error)
	ListIncidents(ctx context.Context, filter IncidentFilter) ([]domain.Incident, error)

	SaveAnomaly(ctx context.Context, a domain.Anomaly) error
	ListAnomalies(ctx context.Context, since time.Time, limit int) ([]domain.Anomaly, error)
	DeleteAnomaliesBefore(ctx context.Context, before time.Time) error
	DeleteAlertsBefore(ctx context.Context, before time.Time) error

	AddAudit(ctx context.Context, ev domain.AuditEvent) error
	ListAudit(ctx context.Context, limit int) ([]domain.AuditEvent, error)

	UpsertFault(ctx context.Context, f domain.Fault) error
	ListFaults(ctx context.Context) ([]domain.Fault, error)
	DeleteFault(ctx context.Context, name string) error

	SaveAlert(ctx context.Context, a domain.Alert) error
	ListAlerts(ctx context.Context, since time.Time) ([]domain.Alert, error)
	ListActiveAlerts(ctx context.Context, since time.Time) ([]domain.Alert, error)

	SaveRemediation(ctx context.Context, r domain.RemediationRequest) error
	GetRemediation(ctx context.Context, id string) (domain.RemediationRequest, error)
	ListRemediations(ctx context.Context, incidentID string) ([]domain.RemediationRequest, error)

	SaveResource(ctx context.Context, r domain.Resource) error
	ListResources(ctx context.Context, kind, service string) ([]domain.Resource, error)
	SaveEdge(ctx context.Context, e domain.TopologyEdge) error
	ListEdges(ctx context.Context) ([]domain.TopologyEdge, error)
	SaveChange(ctx context.Context, c domain.Change) error
	ListChanges(ctx context.Context, since time.Time) ([]domain.Change, error)
	SaveHealth(ctx context.Context, h domain.HealthEvent) error
	ListHealth(ctx context.Context, since time.Time) ([]domain.HealthEvent, error)
	SaveSilence(ctx context.Context, s domain.Silence) error
	ListSilences(ctx context.Context) ([]domain.Silence, error)
	SaveMaintenance(ctx context.Context, w domain.MaintenanceWindow) error
	ListMaintenance(ctx context.Context) ([]domain.MaintenanceWindow, error)
}
