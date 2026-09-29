// Package engine runs the observability loop: SLI evaluation, anomaly checks,
// correlation, incident updates, and deterministic RCA. The AI provider is not
// called from the loop.
package engine

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/rmkr-dev/sentinelmesh/internal/ai"
	"github.com/rmkr-dev/sentinelmesh/internal/alerting"
	"github.com/rmkr-dev/sentinelmesh/internal/anomaly"
	"github.com/rmkr-dev/sentinelmesh/internal/correlation"
	"github.com/rmkr-dev/sentinelmesh/internal/domain"
	"github.com/rmkr-dev/sentinelmesh/internal/incident"
	"github.com/rmkr-dev/sentinelmesh/internal/rca"
	"github.com/rmkr-dev/sentinelmesh/internal/redaction"
	"github.com/rmkr-dev/sentinelmesh/internal/runbook"
	"github.com/rmkr-dev/sentinelmesh/internal/slo"
	"github.com/rmkr-dev/sentinelmesh/internal/store"
	"github.com/rmkr-dev/sentinelmesh/internal/telemetryquery"
	"github.com/rmkr-dev/sentinelmesh/internal/topology"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
)

// Dependencies are the read-only telemetry backends. Any of them may be absent.
type Dependencies struct {
	Metrics telemetryquery.MetricQuery
	Traces  telemetryquery.TraceSearcher
	Logs    telemetryquery.LogSearcher
}

// Engine is the SRE control loop.
type Engine struct {
	Store       store.Store
	Deps        Dependencies
	Conventions telemetryquery.Conventions
	Runbooks    []runbook.Runbook
	Window      time.Duration
	Lookback    time.Duration
	Log         *slog.Logger
	Now         func() time.Time
	AI          ai.Provider
	AIEnabled   bool
	Redaction   redaction.Policy
	Retention   time.Duration
	Graph       correlation.Relater
	MaxHops     int
	Cluster     ClusterSource

	mu          sync.Mutex
	lastK8s     []domain.K8sEvent
	metricState string
	traceState  string
	logState    string
}

// ComponentHealth reports backend reachability. The AI layer is intentionally absent.
type ComponentHealth struct {
	Metrics string `json:"metrics"`
	Traces  string `json:"traces"`
	Logs    string `json:"logs"`
}

// Health returns the last observed backend state.
func (e *Engine) Health() ComponentHealth {
	e.mu.Lock()
	defer e.mu.Unlock()
	return ComponentHealth{
		Metrics: orState(e.metricState),
		Traces:  orState(e.traceState),
		Logs:    orState(e.logState),
	}
}

func (e *Engine) now() time.Time {
	if e.Now != nil {
		return e.Now().UTC()
	}
	return time.Now().UTC()
}

func (e *Engine) log() *slog.Logger {
	if e.Log != nil {
		return e.Log
	}
	return slog.Default()
}

// Tick evaluates the catalog once.
func (e *Engine) Tick(ctx context.Context) error {
	ctx, span := otel.Tracer("sentinelmesh").Start(ctx, "engine.tick")
	defer span.End()
	start := time.Now()
	err := e.tick(ctx)
	e.recordTick(ctx, time.Since(start), err)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		span.RecordError(err)
	}
	return err
}

func (e *Engine) tick(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	now := e.now()
	services, err := e.Store.ListServices(ctx)
	if err != nil {
		return err
	}
	defs, err := e.Store.ListSLOs(ctx)
	if err != nil {
		return err
	}
	results, sloSignals := e.evaluateSLOs(ctx, defs, now)
	anomalySignals := e.detectAnomalies(ctx, services, now)
	alertSignals := e.alertSignals(ctx, now)
	contextSignals := e.contextSignals(ctx, now)
	var k8sSignals []domain.Signal
	if e.Cluster != nil {
		signals, events, changes, err := e.Cluster.Collect(ctx, now)
		if err != nil {
			e.log().Warn("kubernetes collect", "error", err.Error())
		} else {
			k8sSignals = signals
			e.lastK8s = events
			for _, ch := range changes {
				contextSignals = append(contextSignals, domain.Signal{
					ID: ch.ID, Type: domain.SignalChange, Service: ch.Target, Summary: ch.Kind + " " + ch.Target,
					Fingerprint: "change|" + ch.ID, OccurredAt: ch.OccurredAt,
					Attributes: ch.Attributes,
				})
			}
		}
	}

	symptoms := append(append([]domain.Signal{}, sloSignals...), anomalySignals...)
	symptoms = append(symptoms, alertSignals...)
	symptoms = append(symptoms, k8sSignals...)
	all := append(symptoms, contextSignals...)

	deps := map[string][]string{}
	for _, s := range services {
		deps[s.Name] = s.Dependencies
	}
	e.Graph = topology.Graph{Edges: topology.FromCatalog(services, now)}
	if e.MaxHops <= 0 {
		e.MaxHops = 3
	}
	groups := correlation.Correlate(all, correlation.Options{
		Window:             e.window(),
		DeploymentLookback: e.lookback(),
		Dependencies:       deps,
		Graph:              e.Graph,
		MaxHops:            e.MaxHops,
	})
	open, err := e.Store.ListIncidents(ctx, store.IncidentFilter{})
	if err != nil {
		return err
	}
	for _, g := range groups {
		if err := e.upsertGroup(ctx, g, services, results, open, now); err != nil {
			return err
		}
	}
	fresh, err := e.Store.ListIncidents(ctx, store.IncidentFilter{})
	if err != nil {
		return err
	}
	for _, inc := range fresh {
		if err := e.maybeRecover(ctx, inc, now); err != nil {
			return err
		}
	}
	if err := e.sweep(ctx, now); err != nil {
		e.log().Warn("retention sweep failed", "error", err.Error())
	}
	return nil
}

func (e *Engine) recordTick(ctx context.Context, d time.Duration, err error) {
	meter := otel.Meter("sentinelmesh")
	hist, herr := meter.Float64Histogram("sentinelmesh.engine.tick.duration", metric.WithUnit("s"))
	if herr == nil {
		hist.Record(ctx, d.Seconds())
	}
	counter, cerr := meter.Int64Counter("sentinelmesh.engine.tick.errors")
	if cerr == nil && err != nil {
		counter.Add(ctx, 1)
	}
	_ = attribute.String("component", "engine")
}

func (e *Engine) sweep(ctx context.Context, now time.Time) error {
	keep := e.retention()
	before := now.Add(-keep)
	if err := e.Store.DeleteAnomaliesBefore(ctx, before); err != nil {
		return err
	}
	return e.Store.DeleteAlertsBefore(ctx, before)
}

func (e *Engine) retention() time.Duration {
	if e.Retention <= 0 {
		return 7 * 24 * time.Hour
	}
	return e.Retention
}

func (e *Engine) evaluateSLOs(ctx context.Context, defs []domain.SLODefinition, now time.Time) ([]domain.SLOResult, []domain.Signal) {
	var results []domain.SLOResult
	var signals []domain.Signal
	if e.Deps.Metrics == nil {
		e.metricState = "not_configured"
		return nil, nil
	}
	failed := false
	for _, def := range defs {
		for _, w := range def.Windows {
			counts, ok, err := e.counts(ctx, def, w)
			if err != nil {
				failed = true
				e.log().Warn("slo query failed", "service", def.Service, "slo", def.Name, "error", err.Error())
				continue
			}
			if !ok {
				res := domain.SLOResult{
					Service: def.Service, SLO: def.Name, Window: w.Name, Target: def.Objective,
					Status: domain.SLONoData, ErrorBudgetRemaining: 100, EvaluatedAt: now,
				}
				_ = e.Store.SaveSLOResult(ctx, res)
				results = append(results, res)
				continue
			}
			res, err := slo.Evaluate(def, w, counts, now)
			if err != nil {
				e.log().Warn("slo evaluate failed", "error", err.Error())
				continue
			}
			if err := e.Store.SaveSLOResult(ctx, res); err != nil {
				e.log().Warn("slo save failed", "error", err.Error())
			}
			results = append(results, res)
			// Compliance horizons stay on the SLO record. Only short windows
			// open incidents, so a 30 day budget burn does not hold an incident
			// open after the fast window has recovered.
			if operationalWindow(w) && (res.Status == domain.SLOBreached || res.Status == domain.SLOAtRisk) {
				signals = append(signals, domain.Signal{
					ID:          fmt.Sprintf("slo-%s-%s-%s", def.Service, def.Name, w.Name),
					Type:        domain.SignalSLO,
					Service:     def.Service,
					Severity:    res.Status,
					Summary:     fmt.Sprintf("%s %s %s is %s (SLI %.3f%%, burn %.2f)", def.Service, def.Name, w.Name, res.Status, res.Current, res.BurnRate),
					Fingerprint: fmt.Sprintf("slo|%s|%s|%s|%s", def.Service, def.Name, w.Name, res.Status),
					OccurredAt:  now,
					Attributes:  map[string]string{"slo": def.Name, "window": w.Name, "status": res.Status},
				})
			}
		}
	}
	if failed {
		e.metricState = "degraded"
	} else {
		e.metricState = "ok"
	}
	return results, signals
}

func (e *Engine) counts(ctx context.Context, def domain.SLODefinition, w domain.SLOWindow) (slo.Counts, bool, error) {
	conv := e.Conventions
	total, ok, err := e.Deps.Metrics.Instant(ctx, conv.TotalQuery(def.Service, w.Name))
	if err != nil || !ok {
		return slo.Counts{}, ok, err
	}
	if def.Indicator == "latency" {
		threshold := 0.5
		if def.ThresholdMS > 0 {
			threshold = def.ThresholdMS / 1000
		}
		samples, err := e.Deps.Metrics.Vector(ctx, conv.LatencyBucketsQuery(def.Service, w.Name))
		if err != nil {
			return slo.Counts{}, false, err
		}
		good, gok := telemetryquery.SelectLatencyBucket(samples, threshold)
		if !gok {
			good = 0
		}
		if good > total {
			good = total
		}
		return slo.Counts{Good: good, Total: total}, true, nil
	}
	bad, bok, err := e.Deps.Metrics.Instant(ctx, conv.BadQuery(def.Service, w.Name))
	if err != nil {
		return slo.Counts{}, false, err
	}
	if !bok {
		bad = 0
	}
	good := total - bad
	if good < 0 {
		good = 0
	}
	return slo.Counts{Good: good, Total: total}, true, nil
}

func (e *Engine) detectAnomalies(ctx context.Context, services []domain.Service, now time.Time) []domain.Signal {
	if e.Deps.Metrics == nil {
		return nil
	}
	var signals []domain.Signal
	for _, svc := range services {
		q := e.Conventions.ErrorRatioQuery(svc.Name, "1m")
		points, err := e.Deps.Metrics.Range(ctx, q, now.Add(-30*time.Minute), now, time.Minute)
		if err != nil {
			e.metricState = "degraded"
			continue
		}
		if len(points) == 0 {
			continue
		}
		series := anomaly.Series{Service: svc.Name, Metric: "error_ratio", Points: points}
		for _, det := range []anomaly.Detector{anomaly.ZScoreDetector{}, anomaly.RateChangeDetector{}, anomaly.ThresholdDetector{}} {
			params := anomaly.Params{}
			if det.Name() == "threshold" {
				params.Threshold = 0.05
			}
			found, err := det.Detect(series, params)
			if err != nil {
				continue
			}
			for _, a := range found {
				a.DetectedAt = now
				a.ID = anomaly.StableID(a.Service, a.Detector, a.Metric, now)
				_ = e.Store.SaveAnomaly(ctx, a)
				signals = append(signals, domain.Signal{
					ID:          a.ID,
					Type:        domain.SignalAnomaly,
					Service:     svc.Name,
					Severity:    "warning",
					Summary:     a.Summary,
					Fingerprint: "anomaly|" + svc.Name + "|" + a.Detector + "|" + a.Metric,
					OccurredAt:  now,
					Attributes:  map[string]string{"detector": a.Detector, "metric": a.Metric},
				})
			}
		}
	}
	return signals
}

func (e *Engine) alertSignals(ctx context.Context, now time.Time) []domain.Signal {
	alerts, err := e.Store.ListAlerts(ctx, now.Add(-e.window()))
	if err != nil {
		return nil
	}
	silences, _ := e.Store.ListSilences(ctx)
	windows, _ := e.Store.ListMaintenance(ctx)
	var signals []domain.Signal
	for _, a := range alerts {
		if a.Status != "firing" {
			continue
		}
		labels := map[string]string{"service": a.Service, "alertname": a.Name}
		for k, v := range a.Labels {
			labels[k] = v
		}
		if alerting.Silenced(silences, labels, now) || alerting.InMaintenance(windows, labels, now) {
			continue
		}
		if alerting.Flapping(alerts, alerting.NormalizeFingerprint(a), 4, 30*time.Minute, now) {
			continue
		}
		signals = append(signals, domain.Signal{
			ID:          a.ID,
			Type:        domain.SignalAlert,
			Service:     a.Service,
			Severity:    a.Severity,
			Summary:     a.Name + ": " + a.Summary,
			Fingerprint: "alert|" + a.Fingerprint,
			OccurredAt:  a.StartsAt,
			Attributes:  a.Labels,
		})
	}
	return signals
}

func (e *Engine) contextSignals(ctx context.Context, now time.Time) []domain.Signal {
	var signals []domain.Signal
	faults, err := e.Store.ListFaults(ctx)
	if err == nil {
		for _, f := range faults {
			if !f.Enabled {
				continue
			}
			signals = append(signals, domain.Signal{
				ID:          "fault-" + f.Name,
				Type:        domain.SignalFault,
				Service:     f.Service,
				Summary:     fmt.Sprintf("Fault %s enabled on %s (%s)", f.Name, f.Service, f.Kind),
				Fingerprint: "fault|" + f.Name,
				OccurredAt:  f.UpdatedAt,
				Attributes:  map[string]string{"observed": "true", "fault": f.Name, "kind": f.Kind},
			})
		}
	}
	deploys, err := e.Store.ListDeployments(ctx, "", 100)
	if err == nil {
		for _, d := range deploys {
			if now.Sub(d.Timestamp) > e.lookback() {
				continue
			}
			signals = append(signals, domain.Signal{
				ID:          "deploy-" + d.ID,
				Type:        domain.SignalDeployment,
				Service:     d.Service,
				Summary:     fmt.Sprintf("Deployment %s:%s", d.Service, d.Version),
				Fingerprint: "deploy|" + d.ID,
				OccurredAt:  d.Timestamp,
				Attributes:  map[string]string{"version": d.Version, "git_sha": d.GitSHA},
			})
		}
	}
	if changes, err := e.Store.ListChanges(ctx, now.Add(-e.lookback())); err == nil {
		for _, c := range changes {
			signals = append(signals, domain.Signal{
				ID: c.ID, Type: domain.SignalChange, Service: c.Target,
				Summary: c.Kind + " " + c.Target, Fingerprint: "change|" + c.ID, OccurredAt: c.OccurredAt,
				Attributes: c.Attributes,
			})
		}
	}
	if health, err := e.Store.ListHealth(ctx, now.Add(-e.lookback())); err == nil {
		for _, h := range health {
			signals = append(signals, domain.Signal{
				ID:   "health-" + h.Resource + "-" + h.At.UTC().Format(time.RFC3339),
				Type: domain.SignalResourceHealth, Service: h.Resource, Severity: h.State,
				Summary: "Azure reports " + h.Resource + " " + h.State, Fingerprint: "health|" + h.Resource + "|" + h.State,
				OccurredAt: h.At, Attributes: map[string]string{"state": h.State, "reason": h.Reason, "source": h.Source},
			})
		}
	}
	return signals
}

func (e *Engine) upsertGroup(ctx context.Context, g correlation.Group, services []domain.Service, results []domain.SLOResult, open []domain.Incident, now time.Time) error {
	primary := primaryService(g)
	var existing *domain.Incident
	for i := range open {
		inc := &open[i]
		if !incident.Open(inc.Status) {
			continue
		}
		if !overlaps(*inc, g.Services) {
			continue
		}
		if now.Sub(inc.UpdatedAt) > 15*time.Minute && now.Sub(inc.DetectedAt) > 15*time.Minute {
			continue
		}
		existing = inc
		break
	}
	if existing == nil {
		id, err := e.Store.NextIncidentID(ctx, now)
		if err != nil {
			return err
		}
		inc := domain.Incident{
			ID:              id,
			Severity:        severityFor(services, g, results),
			Status:          domain.StatusDetected,
			Title:           fmt.Sprintf("Correlated symptoms on %s", primary),
			Service:         primary,
			Environment:     environmentFor(services, primary),
			StartedAt:       g.Started,
			DetectedAt:      now,
			Signals:         append(append([]domain.Signal{}, g.Signals...), g.Context...),
			RelatedServices: others(g.Services, primary),
			UpdatedAt:       now,
		}
		inc.Events = timelineFromSignals(g, now)
		inc.Events = append([]domain.TimelineEntry{{
			ID: id + "-created", At: now, Kind: "incident", Actor: "engine",
			Message: "Incident created from correlated signals",
		}}, inc.Events...)
		e.applyAnalysis(ctx, &inc, results, now)
		return e.Store.SaveIncident(ctx, inc)
	}
	before := len(existing.Signals)
	for _, s := range append(append([]domain.Signal{}, g.Signals...), g.Context...) {
		if !hasFingerprint(existing.Signals, s.Fingerprint) {
			existing.Signals = append(existing.Signals, s)
			existing.Events = append(existing.Events, domain.TimelineEntry{
				ID: s.ID, At: s.OccurredAt, Kind: s.Type, Message: s.Summary, Actor: "engine",
			})
		}
	}
	existing.RelatedServices = union(existing.RelatedServices, others(g.Services, existing.Service))
	existing.UpdatedAt = now
	if len(existing.Signals) != before {
		e.applyAnalysis(ctx, existing, results, now)
	}
	return e.Store.SaveIncident(ctx, *existing)
}

func (e *Engine) applyAnalysis(ctx context.Context, inc *domain.Incident, results []domain.SLOResult, now time.Time) {
	deploys, _ := e.Store.ListDeployments(ctx, "", 50)
	faults, _ := e.Store.ListFaults(ctx)
	var changes []domain.Signal
	for _, f := range faults {
		if !f.Enabled {
			continue
		}
		if f.Service != inc.Service && !contains(inc.RelatedServices, f.Service) {
			continue
		}
		changes = append(changes, domain.Signal{
			Type: domain.SignalFault, Service: f.Service, OccurredAt: f.UpdatedAt,
			Summary:    fmt.Sprintf("Fault %s enabled on %s (%s)", f.Name, f.Service, f.Kind),
			Attributes: map[string]string{"observed": "true", "fault": f.Name, "kind": f.Kind},
		})
	}
	var relevant []domain.Deployment
	names := append([]string{inc.Service}, inc.RelatedServices...)
	for _, d := range deploys {
		if contains(names, d.Service) {
			relevant = append(relevant, d)
		}
	}
	var sloResults []domain.SLOResult
	for _, r := range results {
		if contains(names, r.Service) {
			sloResults = append(sloResults, r)
		}
	}
	pack := rca.Pack{
		Incident:    *inc,
		Deployments: relevant,
		Changes:     changes,
		SLO:         sloResults,
		K8s:         e.k8sFor(inc),
		Now:         now,
	}
	from, to := e.evidenceWindow(*inc, now)
	if e.Deps.Traces != nil {
		samples, err := e.Deps.Traces.SearchTraces(ctx, telemetryquery.TraceQuery{
			Service: inc.Service, Start: from, End: to, Limit: 10,
		})
		if err != nil {
			e.traceState = "degraded"
		} else {
			e.traceState = "ok"
			pack.Traces = samples
		}
	} else {
		e.traceState = "not_configured"
	}
	if e.Deps.Logs != nil {
		lines, err := e.Deps.Logs.SearchLogs(ctx, telemetryquery.LogQuery{
			Service: inc.Service, Start: from, End: to, Limit: 10,
		})
		if err != nil {
			e.logState = "degraded"
		} else {
			e.logState = "ok"
			pack.Logs = lines
		}
	} else {
		e.logState = "not_configured"
	}
	e.redactPack(&pack)
	e.redactSignals(inc)
	analysis := rca.Analyze(pack)
	matched := runbook.Match(e.Runbooks, *inc)
	analysis.RecommendedActions = append(analysis.RecommendedActions, runbook.Actions(matched)...)
	inc.Analysis = &analysis
	inc.Summary = analysis.Summary
	inc.SuspectedCauses = analysis.Hypotheses
	inc.Evidence = analysis.Evidence
	inc.RecommendedActions = analysis.RecommendedActions
	if inc.Impact == "" {
		inc.Impact = analysis.Impact
	}
}

func (e *Engine) maybeRecover(ctx context.Context, inc domain.Incident, now time.Time) error {
	if !incident.Open(inc.Status) {
		return nil
	}
	faults, err := e.Store.ListFaults(ctx)
	if err != nil {
		return err
	}
	names := append([]string{inc.Service}, inc.RelatedServices...)
	for _, f := range faults {
		if f.Enabled && contains(names, f.Service) {
			return nil
		}
	}
	results, err := e.Store.LatestSLOResults(ctx, "")
	if err != nil {
		return err
	}
	defs, err := e.Store.ListSLOs(ctx)
	if err != nil {
		return err
	}
	compliance := map[string]bool{}
	for _, def := range defs {
		for _, w := range def.Windows {
			if !operationalWindow(w) {
				compliance[def.Service+"|"+def.Name+"|"+w.Name] = true
			}
		}
	}
	saw := false
	for _, r := range results {
		if !contains(names, r.Service) {
			continue
		}
		if compliance[r.Service+"|"+r.SLO+"|"+r.Window] {
			continue
		}
		saw = true
		if r.Status == domain.SLOBreached || r.Status == domain.SLOAtRisk {
			return nil
		}
	}
	if !saw {
		return nil
	}
	alerts, err := e.Store.ListAlerts(ctx, now.Add(-e.window()))
	if err != nil {
		return err
	}
	for _, a := range alerts {
		if a.Status == "firing" && contains(names, a.Service) {
			return nil
		}
	}
	if err := incident.Transition(inc.Status, domain.StatusResolved); err != nil {
		return nil
	}
	resolved := now
	inc.Status = domain.StatusResolved
	inc.ResolvedAt = &resolved
	inc.UpdatedAt = now
	inc.Events = append(inc.Events, domain.TimelineEntry{
		ID: inc.ID + "-recovered", At: now, Kind: "recovery", Actor: "engine",
		Message: "SLO windows are healthy, no firing alerts, and no active faults. Incident marked resolved.",
	})
	e.applyAnalysis(ctx, &inc, results, now)
	return e.Store.SaveIncident(ctx, inc)
}

// AnalyzeIncident refreshes deterministic evidence and, when configured,
// asks the AI provider to narrate it. Provider failure leaves the
// deterministic analysis in place with ai_status=degraded.
func (e *Engine) AnalyzeIncident(ctx context.Context, id, actor string) (domain.Incident, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	inc, err := e.Store.GetIncident(ctx, id)
	if err != nil {
		return domain.Incident{}, err
	}
	now := e.now()
	results, err := e.Store.LatestSLOResults(ctx, "")
	if err != nil {
		return domain.Incident{}, err
	}
	e.applyAnalysis(ctx, &inc, results, now)
	if inc.Analysis == nil {
		return inc, e.Store.SaveIncident(ctx, inc)
	}
	inc.Events = append(inc.Events, domain.TimelineEntry{
		ID: id + "-analysis-" + now.Format("150405.000"), At: now, Kind: "analysis", Actor: actor,
		Message: "Deterministic analysis refreshed",
	})
	if e.AIEnabled && e.AI != nil {
		k8s := e.k8sFor(&inc)
		for i := range k8s {
			k8s[i].Message = redaction.RedactString(e.policy(), k8s[i].Message)
		}
		pack := domain.EvidencePack{
			Incident:         inc,
			SLOStatus:        results,
			Analysis:         inc.Analysis,
			MetricSummaries:  metricSummaries(results),
			KubernetesEvents: k8s,
		}
		e.redactEvidence(&pack)
		model, err := e.AI.Analyze(ctx, pack)
		if err != nil {
			degraded := ai.Degraded(*inc.Analysis, err)
			inc.Analysis = &degraded
			inc.Events = append(inc.Events, domain.TimelineEntry{
				ID: id + "-ai-degraded", At: now, Kind: "ai", Actor: e.AI.Name(),
				Message: "AI analysis degraded: " + err.Error(),
			})
		} else {
			merged := ai.Merge(*inc.Analysis, model, e.AI.Name())
			inc.Analysis = &merged
			inc.Summary = merged.Summary
			inc.SuspectedCauses = merged.Hypotheses
			inc.Events = append(inc.Events, domain.TimelineEntry{
				ID: id + "-ai", At: now, Kind: "ai", Actor: e.AI.Name(),
				Message: "AI-assisted narrative attached. Evidence grades were not upgraded by the model.",
			})
		}
	}
	inc.UpdatedAt = now
	if err := e.Store.SaveIncident(ctx, inc); err != nil {
		return domain.Incident{}, err
	}
	_ = e.Store.AddAudit(ctx, domain.AuditEvent{
		ID: "audit-" + id + "-" + now.Format("150405.000"), At: now, Actor: actor, Action: "incident.analyze",
		Target: id, Reason: "analysis requested", AIGenerated: inc.Analysis != nil && inc.Analysis.AIStatus == "completed",
	})
	return inc, nil
}

func metricSummaries(results []domain.SLOResult) []string {
	var out []string
	for _, r := range results {
		out = append(out, fmt.Sprintf("%s %s %s status=%s current=%.4f burn=%.2f", r.Service, r.SLO, r.Window, r.Status, r.Current, r.BurnRate))
	}
	return out
}

// operationalWindow reports whether a window describes current impact.
// Windows longer than five minutes, including the 30 day compliance window,
// are still evaluated and shown. They do not open or hold an incident.
// ClusterSource is the read-only Kubernetes adapter.
type ClusterSource interface {
	Collect(ctx context.Context, now time.Time) ([]domain.Signal, []domain.K8sEvent, []domain.Change, error)
}

func (e *Engine) k8sFor(inc *domain.Incident) []domain.K8sEvent {
	if len(e.lastK8s) == 0 {
		return nil
	}
	if inc.Service == "" && len(inc.RelatedServices) == 0 {
		return nil
	}
	names := map[string]bool{}
	if inc.Service != "" {
		names[inc.Service] = true
	}
	for _, svc := range inc.RelatedServices {
		if svc != "" {
			names[svc] = true
		}
	}
	var out []domain.K8sEvent
	for _, ev := range e.lastK8s {
		if ev.Service != "" && names[ev.Service] {
			out = append(out, ev)
		}
	}
	return out
}

func (e *Engine) evidenceWindow(inc domain.Incident, now time.Time) (time.Time, time.Time) {
	from := now.Add(-e.lookback())
	if !inc.StartedAt.IsZero() {
		earlier := inc.StartedAt.Add(-e.lookback())
		if earlier.Before(from) {
			from = earlier
		}
	}
	return from, now
}

func (e *Engine) policy() redaction.Policy {
	if len(e.Redaction.Headers) == 0 && len(e.Redaction.JSONFields) == 0 {
		return redaction.DefaultPolicy()
	}
	return e.Redaction
}

func (e *Engine) redactPack(pack *rca.Pack) {
	policy := e.policy()
	for i := range pack.Logs {
		pack.Logs[i].Body = redaction.RedactString(policy, pack.Logs[i].Body)
		pack.Logs[i].Attributes = redaction.RedactAttributes(policy, pack.Logs[i].Attributes)
	}
	for i := range pack.Traces {
		pack.Traces[i].Error = redaction.RedactString(policy, pack.Traces[i].Error)
		pack.Traces[i].Operation = redaction.RedactString(policy, pack.Traces[i].Operation)
	}
	for i := range pack.Changes {
		pack.Changes[i].Summary = redaction.RedactString(policy, pack.Changes[i].Summary)
		pack.Changes[i].Attributes = redaction.RedactAttributes(policy, pack.Changes[i].Attributes)
	}
}

func (e *Engine) redactEvidence(pack *domain.EvidencePack) {
	policy := e.policy()
	pack.Incident.Summary = redaction.RedactString(policy, pack.Incident.Summary)
	pack.Incident.Impact = redaction.RedactString(policy, pack.Incident.Impact)
	e.redactSignals(&pack.Incident)
	if pack.Analysis == nil {
		return
	}
	pack.Analysis.Summary = redaction.RedactString(policy, pack.Analysis.Summary)
	pack.Analysis.Impact = redaction.RedactString(policy, pack.Analysis.Impact)
	for i := range pack.Analysis.Evidence {
		pack.Analysis.Evidence[i].Summary = redaction.RedactString(policy, pack.Analysis.Evidence[i].Summary)
	}
	for i := range pack.Analysis.Hypotheses {
		pack.Analysis.Hypotheses[i].Statement = redaction.RedactString(policy, pack.Analysis.Hypotheses[i].Statement)
	}
}

func (e *Engine) redactSignals(inc *domain.Incident) {
	policy := e.policy()
	for i := range inc.Signals {
		inc.Signals[i].Summary = redaction.RedactString(policy, inc.Signals[i].Summary)
		inc.Signals[i].Attributes = redaction.RedactAttributes(policy, inc.Signals[i].Attributes)
	}
}

func operationalWindow(w domain.SLOWindow) bool {
	if w.Duration <= 0 {
		return !w.Compliance
	}
	return w.Duration <= 5*time.Minute
}

func (e *Engine) window() time.Duration {
	if e.Window <= 0 {
		return 5 * time.Minute
	}
	return e.Window
}

func (e *Engine) lookback() time.Duration {
	if e.Lookback <= 0 {
		return 30 * time.Minute
	}
	return e.Lookback
}

func primaryService(g correlation.Group) string {
	counts := map[string]int{}
	for _, s := range g.Signals {
		counts[s.Service]++
	}
	best := g.Services[0]
	for _, s := range g.Services {
		if counts[s] > counts[best] {
			best = s
		}
	}
	return best
}

func severityFor(services []domain.Service, g correlation.Group, results []domain.SLOResult) string {
	high := false
	for _, name := range g.Services {
		for _, s := range services {
			if s.Name == name && (s.Criticality == "high" || s.Criticality == "critical") {
				high = true
			}
		}
	}
	breached := false
	for _, r := range results {
		if contains(g.Services, r.Service) && r.Status == domain.SLOBreached {
			breached = true
		}
	}
	if breached && high && len(g.Services) > 1 {
		return domain.SeveritySEV1
	}
	if breached && high {
		return domain.SeveritySEV2
	}
	if breached {
		return domain.SeveritySEV3
	}
	return domain.SeveritySEV4
}

func environmentFor(services []domain.Service, name string) string {
	for _, s := range services {
		if s.Name == name && s.Environment != "" {
			return s.Environment
		}
	}
	return "local"
}

func others(services []string, primary string) []string {
	var out []string
	for _, s := range services {
		if s != primary {
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}

func timelineFromSignals(g correlation.Group, now time.Time) []domain.TimelineEntry {
	var all []domain.Signal
	all = append(all, g.Context...)
	all = append(all, g.Signals...)
	sort.Slice(all, func(i, j int) bool { return all[i].OccurredAt.Before(all[j].OccurredAt) })
	var events []domain.TimelineEntry
	for _, s := range all {
		at := s.OccurredAt
		if at.IsZero() {
			at = now
		}
		events = append(events, domain.TimelineEntry{
			ID: s.ID + "-tl", At: at, Kind: s.Type, Message: s.Summary, Actor: "engine",
		})
	}
	return events
}

func overlaps(inc domain.Incident, services []string) bool {
	names := append([]string{inc.Service}, inc.RelatedServices...)
	for _, s := range services {
		if contains(names, s) {
			return true
		}
	}
	return false
}

func hasFingerprint(signals []domain.Signal, fp string) bool {
	for _, s := range signals {
		if s.Fingerprint == fp {
			return true
		}
	}
	return false
}

func contains(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}

func union(a, b []string) []string {
	out := append([]string{}, a...)
	for _, s := range b {
		if !contains(out, s) {
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}

func orState(s string) string {
	if s == "" {
		return "unknown"
	}
	return s
}
