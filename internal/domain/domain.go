// Package domain holds the cloud-neutral models for the observability platform.
// Azure, Kubernetes, and OpenTelemetry adapters live outside this package.
package domain

import "time"

const (
	SeveritySEV1 = "SEV1"
	SeveritySEV2 = "SEV2"
	SeveritySEV3 = "SEV3"
	SeveritySEV4 = "SEV4"

	StatusDetected      = "detected"
	StatusTriaged       = "triaged"
	StatusInvestigating = "investigating"
	StatusMitigating    = "mitigating"
	StatusMonitoring    = "monitoring"
	StatusResolved      = "resolved"
	StatusClosed        = "closed"

	GradeConfirmed          = "confirmed"
	GradeStronglyCorrelated = "strongly_correlated"
	GradeProbable           = "probable"
	GradePossible           = "possible"
	GradeInsufficient       = "insufficient_evidence"

	SLOHealthy  = "healthy"
	SLOAtRisk   = "at_risk"
	SLOBreached = "breached"
	SLONoData   = "no_data"

	SignalAlert          = "alert"
	SignalAnomaly        = "anomaly"
	SignalSLO            = "slo"
	SignalTrace          = "trace"
	SignalLog            = "log"
	SignalK8s            = "k8s"
	SignalDeployment     = "deployment"
	SignalFault          = "fault"
	SignalChange         = "change"
	SignalResourceHealth = "resource_health"
	SignalPlatform       = "platform"
)

// Service is a catalog entry. Names are stable identifiers, not cloud resource IDs.
type Service struct {
	Name         string            `json:"name"`
	Team         string            `json:"team"`
	Owner        string            `json:"owner"`
	Criticality  string            `json:"criticality"`
	Environment  string            `json:"environment"`
	Version      string            `json:"version,omitempty"`
	Repository   string            `json:"repository,omitempty"`
	Dependencies []string          `json:"dependencies,omitempty"`
	Attributes   map[string]string `json:"attributes,omitempty"`
	UpdatedAt    time.Time         `json:"updated_at"`
}

// SLODefinition is the configured objective. Objectives are percentages (99.95 = 99.95%).
type SLODefinition struct {
	ID          string            `json:"id"`
	Service     string            `json:"service"`
	Name        string            `json:"name"`
	Objective   float64           `json:"objective"`
	Indicator   string            `json:"indicator"`
	ThresholdMS float64           `json:"threshold_ms,omitempty"`
	Windows     []SLOWindow       `json:"windows"`
	Description string            `json:"description,omitempty"`
	Params      map[string]string `json:"params,omitempty"`
}

// SLOWindow is one evaluation horizon. Compliance windows determine breach;
// shorter windows feed burn-rate alerting.
type SLOWindow struct {
	Name       string        `json:"name"`
	Duration   time.Duration `json:"duration"`
	Compliance bool          `json:"compliance"`
	BurnAlert  float64       `json:"burn_alert,omitempty"`
}

// SLOResult is a computed SLI for one window. Values are derived, never stored as constants.
type SLOResult struct {
	Service              string    `json:"service"`
	SLO                  string    `json:"slo"`
	Window               string    `json:"window"`
	Target               float64   `json:"target"`
	Current              float64   `json:"current"`
	ErrorBudgetRemaining float64   `json:"error_budget_remaining"`
	ErrorBudgetConsumed  float64   `json:"error_budget_consumed"`
	BurnRate             float64   `json:"burn_rate"`
	Good                 float64   `json:"good"`
	Total                float64   `json:"total"`
	Status               string    `json:"status"`
	EvaluatedAt          time.Time `json:"evaluated_at"`
}

// Deployment records a change the correlation engine can compare with symptoms.
type Deployment struct {
	ID          string            `json:"deployment_id"`
	Service     string            `json:"service"`
	Version     string            `json:"version"`
	Environment string            `json:"environment"`
	GitSHA      string            `json:"git_sha"`
	Repository  string            `json:"repository"`
	Author      string            `json:"author"`
	Timestamp   time.Time         `json:"timestamp"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// Signal is any observable event the correlator can group.
type Signal struct {
	ID          string            `json:"id"`
	Type        string            `json:"type"`
	Service     string            `json:"service"`
	Severity    string            `json:"severity"`
	Summary     string            `json:"summary"`
	Fingerprint string            `json:"fingerprint"`
	OccurredAt  time.Time         `json:"occurred_at"`
	Attributes  map[string]string `json:"attributes,omitempty"`
}

// TimelineEntry is one line on an incident timeline.
type TimelineEntry struct {
	ID         string            `json:"id"`
	At         time.Time         `json:"at"`
	Kind       string            `json:"kind"`
	Message    string            `json:"message"`
	Actor      string            `json:"actor,omitempty"`
	Attributes map[string]string `json:"attributes,omitempty"`
}

// Evidence is a fact cited by a hypothesis. Grade describes how strong the fact is,
// not whether the hypothesis is proven.
type Evidence struct {
	ID         string            `json:"id"`
	Kind       string            `json:"kind"`
	Source     string            `json:"source"`
	Summary    string            `json:"summary"`
	Grade      string            `json:"grade"`
	Timestamp  time.Time         `json:"timestamp"`
	Attributes map[string]string `json:"attributes,omitempty"`
}

// Hypothesis is a proposed explanation. It must cite evidence IDs.
type Hypothesis struct {
	ID          string   `json:"id"`
	Statement   string   `json:"statement"`
	Grade       string   `json:"grade"`
	EvidenceIDs []string `json:"evidence_ids"`
	AIGenerated bool     `json:"ai_generated"`
	Score       float64  `json:"score,omitempty"`
	Rationale   string   `json:"rationale,omitempty"`
}

// RecommendedAction is advice. It is not an execution request.
type RecommendedAction struct {
	ID               string `json:"id"`
	Title            string `json:"title"`
	Detail           string `json:"detail"`
	RunbookID        string `json:"runbook_id,omitempty"`
	Remediation      string `json:"remediation,omitempty"`
	RequiresApproval bool   `json:"requires_approval"`
}

// Analysis is the evidence-based RCA payload. AI output uses the same schema
// and is always marked.
type Analysis struct {
	Summary             string              `json:"summary"`
	Impact              string              `json:"impact"`
	Timeline            []TimelineEntry     `json:"timeline"`
	Evidence            []Evidence          `json:"evidence"`
	Hypotheses          []Hypothesis        `json:"hypotheses"`
	Confidence          float64             `json:"confidence"`
	ConfidenceLabel     string              `json:"confidence_label"`
	RecommendedActions  []RecommendedAction `json:"recommended_actions"`
	RollbackRecommended bool                `json:"rollback_recommended"`
	MissingEvidence     []string            `json:"missing_evidence"`
	AIGenerated         bool                `json:"ai_generated"`
	AIStatus            string              `json:"ai_status"`
	Deterministic       bool                `json:"deterministic"`
	GeneratedAt         time.Time           `json:"generated_at"`
}

// Incident is the normalized incident record.
type Incident struct {
	ID                 string              `json:"incident_id"`
	Severity           string              `json:"severity"`
	Status             string              `json:"status"`
	Title              string              `json:"title"`
	Service            string              `json:"service"`
	Environment        string              `json:"environment"`
	StartedAt          time.Time           `json:"started_at"`
	DetectedAt         time.Time           `json:"detected_at"`
	ResolvedAt         *time.Time          `json:"resolved_at,omitempty"`
	Signals            []Signal            `json:"signals"`
	Events             []TimelineEntry     `json:"events"`
	SuspectedCauses    []Hypothesis        `json:"suspected_causes"`
	Evidence           []Evidence          `json:"evidence"`
	RecommendedActions []RecommendedAction `json:"recommended_actions"`
	Analysis           *Analysis           `json:"analysis,omitempty"`
	Summary            string              `json:"summary,omitempty"`
	HumanNotes         string              `json:"human_notes,omitempty"`
	Impact             string              `json:"impact,omitempty"`
	RelatedServices    []string            `json:"related_services,omitempty"`
	Tenant             string              `json:"tenant,omitempty"`
	UpdatedAt          time.Time           `json:"updated_at"`
}

// Resource is a cloud or cluster object bound to a catalog service.
type Resource struct {
	ID         string            `json:"id"`
	Kind       string            `json:"kind"`
	Provider   string            `json:"provider"`
	Name       string            `json:"name"`
	Region     string            `json:"region,omitempty"`
	Tenant     string            `json:"tenant,omitempty"`
	Service    string            `json:"service,omitempty"`
	Attributes map[string]string `json:"attributes,omitempty"`
	UpdatedAt  time.Time         `json:"updated_at"`
}

// TopologyNode is one vertex in the service and infrastructure graph.
type TopologyNode struct {
	ID         string            `json:"id"`
	Kind       string            `json:"kind"`
	Name       string            `json:"name"`
	Service    string            `json:"service,omitempty"`
	Status     string            `json:"status,omitempty"`
	Attributes map[string]string `json:"attributes,omitempty"`
}

// TopologyEdge extends a dependency edge with provenance.
type TopologyEdge struct {
	From       string    `json:"from"`
	To         string    `json:"to"`
	Kind       string    `json:"kind"`
	Source     string    `json:"source"`
	Status     string    `json:"status,omitempty"`
	Detail     string    `json:"detail,omitempty"`
	Confidence float64   `json:"confidence"`
	LastSeen   time.Time `json:"last_seen"`
}

// Change is a control-plane or deployment change. It is context, not a cause by itself.
type Change struct {
	ID         string            `json:"id"`
	Kind       string            `json:"kind"`
	Source     string            `json:"source"`
	Target     string            `json:"target"`
	Actor      string            `json:"actor,omitempty"`
	Tenant     string            `json:"tenant,omitempty"`
	OccurredAt time.Time         `json:"occurred_at"`
	Attributes map[string]string `json:"attributes,omitempty"`
}

// HealthEvent is a platform health fact from Resource Health or Service Health.
type HealthEvent struct {
	Resource string    `json:"resource"`
	State    string    `json:"state"`
	Reason   string    `json:"reason,omitempty"`
	Source   string    `json:"source"`
	Tenant   string    `json:"tenant,omitempty"`
	At       time.Time `json:"at"`
}

// Silence suppresses new incidents that match its labels until it expires.
type Silence struct {
	ID        string            `json:"id"`
	Tenant    string            `json:"tenant,omitempty"`
	Owner     string            `json:"owner"`
	Reason    string            `json:"reason"`
	Matchers  map[string]string `json:"matchers"`
	StartsAt  time.Time         `json:"starts_at"`
	EndsAt    time.Time         `json:"ends_at"`
	CreatedAt time.Time         `json:"created_at"`
}

// MaintenanceWindow is a planned suppression window.
type MaintenanceWindow struct {
	ID       string            `json:"id"`
	Tenant   string            `json:"tenant,omitempty"`
	Owner    string            `json:"owner"`
	Reason   string            `json:"reason"`
	Matchers map[string]string `json:"matchers"`
	StartsAt time.Time         `json:"starts_at"`
	EndsAt   time.Time         `json:"ends_at"`
}

// Tenant is an isolation boundary. The default tenant is "default".
type Tenant struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Principal is an authenticated caller.
type Principal struct {
	Subject string   `json:"subject"`
	Name    string   `json:"name"`
	Tenant  string   `json:"tenant"`
	Roles   []string `json:"roles"`
}

// Anomaly is a detector finding. Detector names are statistical methods, not model brands.
type Anomaly struct {
	ID         string            `json:"id"`
	Service    string            `json:"service"`
	Metric     string            `json:"metric"`
	Detector   string            `json:"detector"`
	Score      float64           `json:"score"`
	Value      float64           `json:"value"`
	Baseline   float64           `json:"baseline"`
	Summary    string            `json:"summary"`
	DetectedAt time.Time         `json:"detected_at"`
	Attributes map[string]string `json:"attributes,omitempty"`
}

// Alert is an incoming alert from Alertmanager or an equivalent router.
type Alert struct {
	ID          string            `json:"id"`
	Fingerprint string            `json:"fingerprint"`
	Name        string            `json:"name"`
	Service     string            `json:"service"`
	Severity    string            `json:"severity"`
	Status      string            `json:"status"`
	Summary     string            `json:"summary"`
	StartsAt    time.Time         `json:"starts_at"`
	EndsAt      *time.Time        `json:"ends_at,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

// Fault is a deterministic demo fault. Production environments leave the catalog disabled.
type Fault struct {
	Name      string            `json:"name"`
	Service   string            `json:"service"`
	Kind      string            `json:"kind"`
	Enabled   bool              `json:"enabled"`
	Params    map[string]string `json:"params,omitempty"`
	UpdatedAt time.Time         `json:"updated_at"`
}

// AuditEvent records a mutation. AI-generated content sets AIGenerated.
type AuditEvent struct {
	ID          string         `json:"id"`
	At          time.Time      `json:"at"`
	Actor       string         `json:"actor"`
	Action      string         `json:"action"`
	Target      string         `json:"target"`
	Reason      string         `json:"reason,omitempty"`
	AIGenerated bool           `json:"ai_generated"`
	Before      map[string]any `json:"before_state,omitempty"`
	After       map[string]any `json:"after_state,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

// RemediationRequest is a proposed or executed action. Default policy does not execute it.
type RemediationRequest struct {
	ID         string         `json:"id"`
	IncidentID string         `json:"incident_id,omitempty"`
	Action     string         `json:"action"`
	Target     string         `json:"target"`
	Namespace  string         `json:"namespace,omitempty"`
	Replicas   int            `json:"replicas,omitempty"`
	Status     string         `json:"status"`
	Reason     string         `json:"reason"`
	Actor      string         `json:"actor"`
	Approver   string         `json:"approver,omitempty"`
	Before     map[string]any `json:"before_state,omitempty"`
	After      map[string]any `json:"after_state,omitempty"`
	Error      string         `json:"error,omitempty"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
}

// DependencyEdge is a catalog or runtime edge with the latest known health.
type DependencyEdge struct {
	From   string `json:"from"`
	To     string `json:"to"`
	Status string `json:"status"`
	Source string `json:"source"`
	Detail string `json:"detail,omitempty"`
}

// MetricPoint is one sample used by anomaly detection and evidence.
type MetricPoint struct {
	Time  time.Time `json:"time"`
	Value float64   `json:"value"`
}

// LogExcerpt is a redacted log line cited as evidence.
type LogExcerpt struct {
	Timestamp  time.Time         `json:"timestamp"`
	Service    string            `json:"service"`
	Severity   string            `json:"severity"`
	Body       string            `json:"body"`
	TraceID    string            `json:"trace_id,omitempty"`
	Attributes map[string]string `json:"attributes,omitempty"`
}

// TraceSample is a redacted trace summary, not a full span dump.
type TraceSample struct {
	TraceID    string    `json:"trace_id"`
	Service    string    `json:"service"`
	Operation  string    `json:"operation"`
	Status     string    `json:"status"`
	DurationMS float64   `json:"duration_ms"`
	StartedAt  time.Time `json:"started_at"`
	Error      string    `json:"error,omitempty"`
	Peer       string    `json:"peer,omitempty"`
}

// K8sEvent is a normalized Kubernetes event.
type K8sEvent struct {
	Reason    string    `json:"reason"`
	Message   string    `json:"message"`
	Type      string    `json:"type"`
	Object    string    `json:"object"`
	Namespace string    `json:"namespace"`
	Count     int       `json:"count"`
	At        time.Time `json:"at"`
}

// EvidencePack is the only input the AI layer is allowed to see.
type EvidencePack struct {
	Incident         Incident         `json:"incident"`
	Metrics          []MetricPoint    `json:"metrics"`
	MetricSummaries  []string         `json:"metric_summaries"`
	Logs             []LogExcerpt     `json:"logs"`
	Traces           []TraceSample    `json:"traces"`
	Deployments      []Deployment     `json:"deployments"`
	KubernetesEvents []K8sEvent       `json:"kubernetes_events"`
	SLOStatus        []SLOResult      `json:"slo_status"`
	Dependencies     []DependencyEdge `json:"dependencies"`
	Changes          []Signal         `json:"changes"`
	Analysis         *Analysis        `json:"deterministic_analysis,omitempty"`
}
