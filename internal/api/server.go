// Package api is the HTTP surface of the platform.
package api

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rmkr-dev/sentinelmesh/internal/azure"
	"github.com/rmkr-dev/sentinelmesh/internal/domain"
	"github.com/rmkr-dev/sentinelmesh/internal/engine"
	"github.com/rmkr-dev/sentinelmesh/internal/faults"
	"github.com/rmkr-dev/sentinelmesh/internal/incident"
	"github.com/rmkr-dev/sentinelmesh/internal/postmortem"
	"github.com/rmkr-dev/sentinelmesh/internal/redaction"
	"github.com/rmkr-dev/sentinelmesh/internal/remediation"
	"github.com/rmkr-dev/sentinelmesh/internal/runbook"
	"github.com/rmkr-dev/sentinelmesh/internal/slo"
	"github.com/rmkr-dev/sentinelmesh/internal/store"
	"github.com/rmkr-dev/sentinelmesh/internal/version"
)

// Server serves the platform API and the investigation UI.
type Server struct {
	Store        store.Store
	Engine       *engine.Engine
	Runbooks     []runbook.Runbook
	Gate         remediation.Gate
	Executor     remediation.Executor
	Demo         bool
	Token        string
	WebhookToken string
	Principals   []Principal
	Redaction    redaction.Policy
	WebDir       string
	Log          *slog.Logger
	GrafanaURL   string
	JaegerURL    string
	metrics      *metrics
}

type metrics struct {
	registry *prometheus.Registry
	requests *prometheus.CounterVec
	slo      *prometheus.GaugeVec
}

func newMetrics() *metrics {
	reg := prometheus.NewRegistry()
	m := &metrics{
		registry: reg,
		requests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "platform_http_requests_total",
			Help: "HTTP requests served by the platform API.",
		}, []string{"path", "code"}),
		slo: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "platform_slo_status",
			Help: "Latest SLO status coded as 0 no_data, 1 healthy, 2 at_risk, 3 breached.",
		}, []string{"service", "slo", "window"}),
	}
	reg.MustRegister(m.requests, m.slo)
	return m
}

// Handler returns the root handler.
func (s *Server) Handler() http.Handler {
	if s.metrics == nil {
		s.metrics = newMetrics()
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.health)
	mux.HandleFunc("GET /ready", s.ready)
	mux.Handle("GET /metrics", promhttp.HandlerFor(s.metrics.registry, promhttp.HandlerOpts{}))
	mux.HandleFunc("GET /api/v1/meta", s.meta)
	mux.HandleFunc("GET /api/v1/services", s.listServices)
	mux.HandleFunc("POST /api/v1/services", s.upsertService)
	mux.HandleFunc("GET /api/v1/services/{service}", s.getService)
	mux.HandleFunc("GET /api/v1/slos", s.listSLOs)
	mux.HandleFunc("GET /api/v1/slos/{service}", s.slosForService)
	mux.HandleFunc("POST /api/v1/engine/tick", s.tick)
	mux.HandleFunc("GET /api/v1/incidents", s.listIncidents)
	mux.HandleFunc("GET /api/v1/incidents/{id}", s.getIncident)
	mux.HandleFunc("POST /api/v1/incidents/{id}/analyze", s.analyze)
	mux.HandleFunc("GET /api/v1/incidents/{id}/timeline", s.timeline)
	mux.HandleFunc("GET /api/v1/incidents/{id}/postmortem", s.postmortem)
	mux.HandleFunc("POST /api/v1/incidents/{id}/transition", s.transition)
	mux.HandleFunc("POST /api/v1/incidents/{id}/notes", s.notes)
	mux.HandleFunc("GET /api/v1/anomalies", s.anomalies)
	mux.HandleFunc("GET /api/v1/deployments", s.listDeployments)
	mux.HandleFunc("POST /api/v1/deployments", s.createDeployment)
	mux.HandleFunc("GET /api/v1/dependencies", s.dependencies)
	mux.HandleFunc("POST /api/v1/alerts/webhook", s.alertWebhook)
	mux.HandleFunc("POST /api/v1/alerts/azure-monitor", s.azureAlert)
	mux.HandleFunc("GET /api/v1/resources", s.listResources)
	mux.HandleFunc("GET /api/v1/topology", s.topology)
	mux.HandleFunc("GET /api/v1/silences", s.listSilences)
	mux.HandleFunc("POST /api/v1/silences", s.createSilence)
	mux.HandleFunc("POST /api/v1/events", s.ingestEvent)
	mux.HandleFunc("GET /api/v1/runbooks", s.listRunbooks)
	mux.HandleFunc("GET /api/v1/audit", s.audit)
	mux.HandleFunc("GET /api/v1/demo/faults", s.listFaults)
	mux.HandleFunc("POST /api/v1/demo/faults", s.enableFault)
	mux.HandleFunc("DELETE /api/v1/demo/faults/{name}", s.disableFault)
	mux.HandleFunc("GET /api/v1/remediations", s.listRemediations)
	mux.HandleFunc("POST /api/v1/remediations", s.createRemediation)
	mux.HandleFunc("POST /api/v1/remediations/{id}/decision", s.decideRemediation)
	if s.WebDir != "" {
		fileServer := http.FileServer(http.Dir(s.WebDir))
		mux.Handle("GET /ui/", http.StripPrefix("/ui/", fileServer))
	}
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/ui/", http.StatusFound)
	})
	return s.wrap(mux)
}

func (s *Server) wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:")
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		name, ok := s.authorize(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "missing or invalid bearer token")
			return
		}
		if name != "" {
			r = r.WithContext(context.WithValue(r.Context(), actorContextKey{}, name))
		}
		next.ServeHTTP(w, r)
	})
}

type actorContextKey struct{}

// Principal is an authenticated caller. WebhookOnly tokens are accepted only on webhook routes.
type Principal struct {
	Name        string
	Token       string
	WebhookOnly bool
}

func (s *Server) authorize(r *http.Request) (string, bool) {
	if isPublic(r.URL.Path) {
		return "", true
	}
	if s.routeOpen(r.URL.Path) {
		return "", true
	}
	bearer := bearerToken(r)
	webhook := isWebhook(r.URL.Path)
	matched := ""
	for _, p := range s.principals() {
		if p.Token == "" || !tokenEqual(bearer, p.Token) {
			continue
		}
		if p.WebhookOnly && !webhook {
			continue
		}
		if matched == "" {
			matched = p.Name
		}
	}
	return matched, matched != ""
}

func (s *Server) principals() []Principal {
	if len(s.Principals) > 0 {
		return s.Principals
	}
	var out []Principal
	if s.Token != "" {
		out = append(out, Principal{Name: "api", Token: s.Token})
	}
	if s.WebhookToken != "" {
		out = append(out, Principal{Name: "alertmanager", Token: s.WebhookToken, WebhookOnly: true})
	}
	return out
}

func (s *Server) routeOpen(path string) bool {
	ps := s.principals()
	if isWebhook(path) {
		for _, p := range ps {
			if p.Token != "" {
				return false
			}
		}
		return true
	}
	for _, p := range ps {
		if !p.WebhookOnly && p.Token != "" {
			return false
		}
	}
	return true
}

func isPublic(path string) bool {
	switch path {
	case "/health", "/ready", "/metrics", "/":
		return true
	}
	return strings.HasPrefix(path, "/ui/")
}

func isWebhook(path string) bool {
	return path == "/api/v1/alerts/webhook" || path == "/api/v1/alerts/azure-monitor" || path == "/api/v1/events"
}

func bearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if !strings.HasPrefix(h, prefix) {
		return ""
	}
	return strings.TrimSpace(h[len(prefix):])
}

func tokenEqual(got, want string) bool {
	if want == "" {
		return false
	}
	sumGot := sha256.Sum256([]byte(got))
	sumWant := sha256.Sum256([]byte(want))
	return subtle.ConstantTimeCompare(sumGot[:], sumWant[:]) == 1
}

func (s *Server) policy() redaction.Policy {
	if len(s.Redaction.Headers) == 0 && len(s.Redaction.JSONFields) == 0 {
		return redaction.DefaultPolicy()
	}
	return s.Redaction
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	body := map[string]any{"status": "ok", "version": version.Version}
	if s.Engine != nil {
		body["dependencies"] = s.Engine.Health()
	}
	body["ai"] = "optional"
	writeJSON(w, http.StatusOK, body)
}

func (s *Server) ready(w http.ResponseWriter, r *http.Request) {
	if err := s.Store.Ping(r.Context()); err != nil {
		writeError(w, http.StatusServiceUnavailable, "store unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func (s *Server) meta(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"name":        "cloud-observability-aiops-platform",
		"version":     version.Version,
		"grafana_url": s.GrafanaURL,
		"jaeger_url":  s.JaegerURL,
		"demo":        s.Demo,
	})
}

func (s *Server) listServices(w http.ResponseWriter, r *http.Request) {
	items, err := s.Store.ListServices(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"services": nonNil(items)})
}

func (s *Server) getService(w http.ResponseWriter, r *http.Request) {
	svc, err := s.Store.GetService(r.Context(), r.PathValue("service"))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "service not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, svc)
}

func (s *Server) upsertService(w http.ResponseWriter, r *http.Request) {
	var svc domain.Service
	if err := readJSON(r, &svc); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if !validName(svc.Name) {
		writeError(w, http.StatusBadRequest, "service name must be a DNS label")
		return
	}
	svc.UpdatedAt = time.Now().UTC()
	if err := s.Store.UpsertService(r.Context(), svc); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.recordAudit(r.Context(), "service.upsert", svc.Name, "catalog update", false)
	writeJSON(w, http.StatusOK, svc)
}

func (s *Server) listSLOs(w http.ResponseWriter, r *http.Request) {
	defs, err := s.Store.ListSLOs(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	results, err := s.Store.LatestSLOResults(r.Context(), "")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.publishSLO(results)
	writeJSON(w, http.StatusOK, map[string]any{
		"definitions": defsOrEmpty(defs),
		"results":     resultsOrEmpty(results),
		"overall":     rollup(results),
	})
}

func (s *Server) slosForService(w http.ResponseWriter, r *http.Request) {
	service := r.PathValue("service")
	results, err := s.Store.LatestSLOResults(r.Context(), service)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"service": service,
		"results": resultsOrEmpty(results),
		"overall": slo.OverallStatus(results),
	})
}

func (s *Server) tick(w http.ResponseWriter, r *http.Request) {
	if s.Engine == nil {
		writeError(w, http.StatusServiceUnavailable, "engine is not configured")
		return
	}
	if err := s.Engine.Tick(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "dependencies": s.Engine.Health()})
}

func (s *Server) listIncidents(w http.ResponseWriter, r *http.Request) {
	items, err := s.Store.ListIncidents(r.Context(), store.IncidentFilter{
		Service: r.URL.Query().Get("service"),
		Status:  r.URL.Query().Get("status"),
		Limit:   100,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"incidents": incidentsOrEmpty(items)})
}

func (s *Server) getIncident(w http.ResponseWriter, r *http.Request) {
	inc, err := s.Store.GetIncident(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "incident not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, inc)
}

func (s *Server) analyze(w http.ResponseWriter, r *http.Request) {
	if s.Engine == nil {
		writeError(w, http.StatusServiceUnavailable, "engine is not configured")
		return
	}
	inc, err := s.Engine.AnalyzeIncident(r.Context(), r.PathValue("id"), actor(r))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "incident not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, inc)
}

func (s *Server) timeline(w http.ResponseWriter, r *http.Request) {
	inc, err := s.Store.GetIncident(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "incident not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"incident_id": inc.ID, "events": eventsOrEmpty(inc.Events)})
}

func (s *Server) postmortem(w http.ResponseWriter, r *http.Request) {
	inc, err := s.Store.GetIncident(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "incident not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	_, _ = w.Write([]byte(postmortem.Render(inc)))
}

func (s *Server) transition(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Status string `json:"status"`
		Reason string `json:"reason"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	inc, err := s.Store.GetIncident(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "incident not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := incident.Transition(inc.Status, body.Status); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	now := time.Now().UTC()
	inc.Status = body.Status
	inc.UpdatedAt = now
	if body.Status == domain.StatusResolved || body.Status == domain.StatusClosed {
		inc.ResolvedAt = &now
	}
	inc.Events = append(inc.Events, domain.TimelineEntry{
		ID: newID("tl"), At: now, Kind: "status", Actor: actor(r),
		Message: fmt.Sprintf("Status changed to %s. %s", body.Status, body.Reason),
	})
	if err := s.Store.SaveIncident(r.Context(), inc); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.recordAudit(r.Context(), "incident.transition", inc.ID, body.Reason, false)
	writeJSON(w, http.StatusOK, inc)
}

func (s *Server) notes(w http.ResponseWriter, r *http.Request) {
	var body struct {
		HumanNotes string `json:"human_notes"`
		Impact     string `json:"impact"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	inc, err := s.Store.GetIncident(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "incident not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if body.HumanNotes != "" {
		inc.HumanNotes = body.HumanNotes
	}
	if body.Impact != "" {
		inc.Impact = body.Impact
	}
	inc.UpdatedAt = time.Now().UTC()
	if err := s.Store.SaveIncident(r.Context(), inc); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.recordAudit(r.Context(), "incident.notes", inc.ID, "human notes updated", false)
	writeJSON(w, http.StatusOK, inc)
}

func (s *Server) anomalies(w http.ResponseWriter, r *http.Request) {
	items, err := s.Store.ListAnomalies(r.Context(), time.Now().UTC().Add(-24*time.Hour), 100)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"anomalies": anomaliesOrEmpty(items)})
}

func (s *Server) listDeployments(w http.ResponseWriter, r *http.Request) {
	items, err := s.Store.ListDeployments(r.Context(), r.URL.Query().Get("service"), 50)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deployments": deploysOrEmpty(items)})
}

func (s *Server) createDeployment(w http.ResponseWriter, r *http.Request) {
	var d domain.Deployment
	if err := readJSON(r, &d); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if !validName(d.Service) || d.Version == "" {
		writeError(w, http.StatusBadRequest, "service and version are required")
		return
	}
	if d.ID == "" {
		d.ID = newID("dep")
	}
	if d.Timestamp.IsZero() {
		d.Timestamp = time.Now().UTC()
	}
	if err := s.Store.CreateDeployment(r.Context(), d); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.recordAudit(r.Context(), "deployment.create", d.Service+":"+d.Version, d.ID, false)
	writeJSON(w, http.StatusCreated, d)
}

func (s *Server) dependencies(w http.ResponseWriter, r *http.Request) {
	services, err := s.Store.ListServices(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	results, _ := s.Store.LatestSLOResults(r.Context(), "")
	statusOf := map[string]string{}
	byService := map[string][]domain.SLOResult{}
	for _, res := range results {
		byService[res.Service] = append(byService[res.Service], res)
	}
	for name, rows := range byService {
		statusOf[name] = slo.OverallStatus(rows)
	}
	var edges []domain.DependencyEdge
	for _, svc := range services {
		for _, dep := range svc.Dependencies {
			st := statusOf[dep]
			if st == "" {
				st = "unknown"
			}
			edges = append(edges, domain.DependencyEdge{From: svc.Name, To: dep, Status: st, Source: "catalog"})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"dependencies": edgesOrEmpty(edges)})
}

func (s *Server) alertWebhook(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Alerts []struct {
			Status      string            `json:"status"`
			Labels      map[string]string `json:"labels"`
			Annotations map[string]string `json:"annotations"`
			StartsAt    time.Time         `json:"startsAt"`
			EndsAt      time.Time         `json:"endsAt"`
			Fingerprint string            `json:"fingerprint"`
		} `json:"alerts"`
	}
	if err := readJSON(r, &payload); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	for _, a := range payload.Alerts {
		service := a.Labels["service"]
		if service == "" {
			service = a.Labels["service_name"]
		}
		name := a.Labels["alertname"]
		id := a.Fingerprint
		if id == "" {
			id = newID("alrt")
		}
		summary := a.Annotations["summary"]
		if summary == "" {
			summary = a.Annotations["description"]
		}
		policy := s.policy()
		summary = redaction.RedactString(policy, summary)
		a.Labels = redaction.RedactAttributes(policy, a.Labels)
		a.Annotations = redaction.RedactAttributes(policy, a.Annotations)
		var ends *time.Time
		if !a.EndsAt.IsZero() {
			ends = &a.EndsAt
		}
		if err := s.Store.SaveAlert(r.Context(), domain.Alert{
			ID: id, Fingerprint: id, Name: name, Service: service, Severity: a.Labels["severity"],
			Status: a.Status, Summary: summary, StartsAt: a.StartsAt, EndsAt: ends,
			Labels: a.Labels, Annotations: a.Annotations,
		}); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	writeJSON(w, http.StatusAccepted, map[string]int{"accepted": len(payload.Alerts)})
}

func (s *Server) azureAlert(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	alerts, err := azure.ParseAlerts(body)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	policy := s.policy()
	for _, a := range alerts {
		a.Summary = redaction.RedactString(policy, a.Summary)
		a.Labels = redaction.RedactAttributes(policy, a.Labels)
		if err := s.Store.SaveAlert(r.Context(), a); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	writeJSON(w, http.StatusAccepted, map[string]int{"accepted": len(alerts)})
}

func (s *Server) listResources(w http.ResponseWriter, r *http.Request) {
	items, err := s.Store.ListResources(r.Context(), r.URL.Query().Get("type"), r.URL.Query().Get("service"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if items == nil {
		items = []domain.Resource{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"resources": items})
}

func (s *Server) topology(w http.ResponseWriter, r *http.Request) {
	edges, err := s.Store.ListEdges(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	services, _ := s.Store.ListServices(r.Context())
	if edges == nil {
		edges = []domain.TopologyEdge{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"services": nonNil(services), "edges": edges})
}

func (s *Server) listSilences(w http.ResponseWriter, r *http.Request) {
	items, err := s.Store.ListSilences(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if items == nil {
		items = []domain.Silence{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"silences": items})
}

func (s *Server) createSilence(w http.ResponseWriter, r *http.Request) {
	var body domain.Silence
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if body.ID == "" {
		body.ID = newID("sil")
	}
	if body.Owner == "" {
		body.Owner = actor(r)
	}
	if body.CreatedAt.IsZero() {
		body.CreatedAt = time.Now().UTC()
	}
	if err := s.Store.SaveSilence(r.Context(), body); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.recordAudit(r.Context(), "silence.create", body.ID, body.Reason, false)
	writeJSON(w, http.StatusCreated, body)
}

func (s *Server) ingestEvent(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Type       string            `json:"type"`
		Service    string            `json:"service"`
		Summary    string            `json:"summary"`
		Severity   string            `json:"severity"`
		Attributes map[string]string `json:"attributes"`
		OccurredAt time.Time         `json:"occurred_at"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if body.Service == "" || body.Summary == "" {
		writeError(w, http.StatusBadRequest, "service and summary are required")
		return
	}
	if body.OccurredAt.IsZero() {
		body.OccurredAt = time.Now().UTC()
	}
	id := newID("evt")
	policy := s.policy()
	labels := redaction.RedactAttributes(policy, body.Attributes)
	if labels == nil {
		labels = map[string]string{}
	}
	labels["signal_type"] = body.Type
	if err := s.Store.SaveAlert(r.Context(), domain.Alert{
		ID: id, Fingerprint: id, Name: body.Type, Service: body.Service, Severity: body.Severity,
		Status: "firing", Summary: redaction.RedactString(policy, body.Summary), StartsAt: body.OccurredAt, Labels: labels,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"id": id})
}

func (s *Server) listRunbooks(w http.ResponseWriter, r *http.Request) {
	books := s.Runbooks
	if books == nil {
		books = []runbook.Runbook{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"runbooks": books})
}

func (s *Server) recordAudit(ctx context.Context, action, target, reason string, ai bool) {
	_ = s.Store.AddAudit(ctx, domain.AuditEvent{
		ID: newID("audit"), At: time.Now().UTC(), Actor: "api", Action: action, Target: target, Reason: reason, AIGenerated: ai,
	})
}

func (s *Server) auditList(w http.ResponseWriter, r *http.Request) {
	items, err := s.Store.ListAudit(r.Context(), 100)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"audit": auditOrEmpty(items)})
}

func (s *Server) audit(w http.ResponseWriter, r *http.Request) { s.auditList(w, r) }

func (s *Server) listFaults(w http.ResponseWriter, r *http.Request) {
	if !s.Demo {
		writeError(w, http.StatusNotFound, "demo faults are disabled")
		return
	}
	items, err := s.Store.ListFaults(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"faults": faultsOrEmpty(items), "catalog": faults.Catalog()})
}

func (s *Server) enableFault(w http.ResponseWriter, r *http.Request) {
	if !s.Demo {
		writeError(w, http.StatusNotFound, "demo faults are disabled")
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	spec, ok := faults.Find(body.Name)
	if !ok {
		writeError(w, http.StatusBadRequest, "unknown fault")
		return
	}
	now := time.Now().UTC()
	f := domain.Fault{Name: spec.Name, Service: spec.Service, Kind: spec.Kind, Enabled: true, UpdatedAt: now}
	if err := s.Store.UpsertFault(r.Context(), f); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if spec.Name == "deployment-regression" {
		_ = s.Store.CreateDeployment(r.Context(), domain.Deployment{
			ID: newID("dep"), Service: spec.Service, Version: "v1.8.2", Environment: "local",
			GitSHA: "badbad1", Repository: "github.com/rmkr-dev/sentinelmesh", Author: actor(r), Timestamp: now,
			Metadata: map[string]string{"change": "deployment-regression"},
		})
	}
	s.recordAudit(r.Context(), "demo.fault.enable", spec.Name, spec.Description, false)
	writeJSON(w, http.StatusOK, f)
}

func (s *Server) disableFault(w http.ResponseWriter, r *http.Request) {
	if !s.Demo {
		writeError(w, http.StatusNotFound, "demo faults are disabled")
		return
	}
	name := r.PathValue("name")
	spec, _ := faults.Find(name)
	if err := s.Store.DeleteFault(r.Context(), name); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if name == "deployment-regression" {
		now := time.Now().UTC()
		_ = s.Store.CreateDeployment(r.Context(), domain.Deployment{
			ID: newID("dep"), Service: spec.Service, Version: "v1.8.3", Environment: "local",
			GitSHA: "good123", Repository: "github.com/rmkr-dev/sentinelmesh", Author: actor(r), Timestamp: now,
			Metadata: map[string]string{"change": "rollback"},
		})
	}
	s.recordAudit(r.Context(), "demo.fault.disable", name, "fault disabled", false)
	writeJSON(w, http.StatusOK, map[string]string{"name": name, "enabled": "false"})
}

func (s *Server) listRemediations(w http.ResponseWriter, r *http.Request) {
	items, err := s.Store.ListRemediations(r.Context(), r.URL.Query().Get("incident_id"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"remediations": remOrEmpty(items)})
}

func (s *Server) createRemediation(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Action     string `json:"action"`
		Target     string `json:"target"`
		Namespace  string `json:"namespace"`
		Replicas   int    `json:"replicas"`
		IncidentID string `json:"incident_id"`
		Reason     string `json:"reason"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	req, err := s.Gate.NewRequest(body.Action, body.Target, body.Reason, actor(r), body.IncidentID, time.Now().UTC())
	if err != nil {
		writeError(w, http.StatusForbidden, err.Error())
		return
	}
	req.ID = newID("rem")
	req.Namespace = body.Namespace
	req.Replicas = body.Replicas
	if req.Status == remediation.StatusApproved {
		done, runErr := remediation.Run(r.Context(), s.Executor, req, time.Now().UTC())
		req = done
		if runErr != nil {
			_ = s.Store.SaveRemediation(r.Context(), req)
			writeError(w, http.StatusBadGateway, runErr.Error())
			return
		}
	}
	if err := s.Store.SaveRemediation(r.Context(), req); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.recordAudit(r.Context(), "remediation.create", req.ID, body.Reason, false)
	writeJSON(w, http.StatusCreated, req)
}

func (s *Server) decideRemediation(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Approve bool `json:"approve"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	req, err := s.Store.GetRemediation(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "remediation not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	req, err = remediation.Approve(req, actor(r), body.Approve, time.Now().UTC())
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	if req.Status == remediation.StatusApproved {
		done, runErr := remediation.Run(r.Context(), s.Executor, req, time.Now().UTC())
		req = done
		_ = s.Store.SaveRemediation(r.Context(), req)
		s.recordAudit(r.Context(), "remediation.execute", req.ID, req.Reason, false)
		if runErr != nil {
			writeError(w, http.StatusBadGateway, runErr.Error())
			return
		}
		writeJSON(w, http.StatusOK, req)
		return
	}
	if err := s.Store.SaveRemediation(r.Context(), req); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.recordAudit(r.Context(), "remediation.reject", req.ID, req.Reason, false)
	writeJSON(w, http.StatusOK, req)
}

func (s *Server) publishSLO(results []domain.SLOResult) {
	if s.metrics == nil {
		return
	}
	for _, r := range results {
		var code float64
		switch r.Status {
		case domain.SLOHealthy:
			code = 1
		case domain.SLOAtRisk:
			code = 2
		case domain.SLOBreached:
			code = 3
		}
		s.metrics.slo.WithLabelValues(r.Service, r.SLO, r.Window).Set(code)
	}
}

func rollup(results []domain.SLOResult) map[string]string {
	by := map[string][]domain.SLOResult{}
	for _, r := range results {
		by[r.Service] = append(by[r.Service], r)
	}
	out := map[string]string{}
	for svc, rows := range by {
		out[svc] = slo.OverallStatus(rows)
	}
	return out
}

func actor(r *http.Request) string {
	if name, ok := r.Context().Value(actorContextKey{}).(string); ok && name != "" {
		return name
	}
	if a := strings.TrimSpace(r.Header.Get("X-Actor")); a != "" {
		return a
	}
	return "api"
}

func validName(name string) bool {
	if name == "" || len(name) > 63 {
		return false
	}
	for _, c := range name {
		if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '-' {
			return false
		}
	}
	return true
}

func newID(prefix string) string {
	var b [6]byte
	_, _ = rand.Read(b[:])
	return prefix + "-" + hex.EncodeToString(b[:])
}

func readJSON(r *http.Request, dest any) error {
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dest); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(true)
	_ = enc.Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func nonNil(s []domain.Service) []domain.Service {
	if s == nil {
		return []domain.Service{}
	}
	return s
}
func defsOrEmpty(s []domain.SLODefinition) []domain.SLODefinition {
	if s == nil {
		return []domain.SLODefinition{}
	}
	return s
}
func resultsOrEmpty(s []domain.SLOResult) []domain.SLOResult {
	if s == nil {
		return []domain.SLOResult{}
	}
	return s
}
func incidentsOrEmpty(s []domain.Incident) []domain.Incident {
	if s == nil {
		return []domain.Incident{}
	}
	return s
}
func eventsOrEmpty(s []domain.TimelineEntry) []domain.TimelineEntry {
	if s == nil {
		return []domain.TimelineEntry{}
	}
	return s
}
func anomaliesOrEmpty(s []domain.Anomaly) []domain.Anomaly {
	if s == nil {
		return []domain.Anomaly{}
	}
	return s
}
func deploysOrEmpty(s []domain.Deployment) []domain.Deployment {
	if s == nil {
		return []domain.Deployment{}
	}
	return s
}
func edgesOrEmpty(s []domain.DependencyEdge) []domain.DependencyEdge {
	if s == nil {
		return []domain.DependencyEdge{}
	}
	return s
}
func auditOrEmpty(s []domain.AuditEvent) []domain.AuditEvent {
	if s == nil {
		return []domain.AuditEvent{}
	}
	return s
}
func faultsOrEmpty(s []domain.Fault) []domain.Fault {
	if s == nil {
		return []domain.Fault{}
	}
	return s
}
func remOrEmpty(s []domain.RemediationRequest) []domain.RemediationRequest {
	if s == nil {
		return []domain.RemediationRequest{}
	}
	return s
}
