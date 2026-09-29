// Package slo calculates SLI, error budget, and burn rate from event counts.
// It does not fetch telemetry. Callers supply good and total counts for a window.
package slo

import (
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/rmkr-dev/sentinelmesh/internal/domain"
)

const (
	defaultAtRiskBurn = 2.0
)

// Counts are the raw good and total events for one window.
type Counts struct {
	Good  float64
	Total float64
}

// Evaluate computes one window result from counts.
// Objective is a percentage in (0, 100]. Remaining budget is a percentage of
// the window budget and is negative when the window has already exhausted it.
func Evaluate(def domain.SLODefinition, window domain.SLOWindow, counts Counts, now time.Time) (domain.SLOResult, error) {
	if def.Service == "" || def.Name == "" {
		return domain.SLOResult{}, errors.New("slo service and name are required")
	}
	if def.Objective <= 0 || def.Objective > 100 {
		return domain.SLOResult{}, fmt.Errorf("slo objective must be in (0, 100], got %v", def.Objective)
	}
	if counts.Total < 0 || counts.Good < 0 {
		return domain.SLOResult{}, errors.New("event counts cannot be negative")
	}
	if counts.Good > counts.Total && counts.Total > 0 {
		return domain.SLOResult{}, errors.New("good events cannot exceed total events")
	}

	res := domain.SLOResult{
		Service:     def.Service,
		SLO:         def.Name,
		Window:      window.Name,
		Target:      def.Objective,
		EvaluatedAt: now.UTC(),
	}
	if counts.Total == 0 {
		res.Status = domain.SLONoData
		res.Current = 0
		res.ErrorBudgetRemaining = 100
		res.BurnRate = 0
		return res, nil
	}

	sli := counts.Good / counts.Total
	target := def.Objective / 100
	allowed := 1 - target
	if allowed <= 0 {
		return domain.SLOResult{}, errors.New("objective of 100% has no error budget")
	}
	errorRatio := 1 - sli
	burn := errorRatio / allowed
	consumed := burn * 100
	remaining := (1 - burn) * 100

	res.Good = counts.Good
	res.Total = counts.Total
	res.Current = round(sli*100, 4)
	res.BurnRate = round(burn, 4)
	res.ErrorBudgetConsumed = round(consumed, 4)
	res.ErrorBudgetRemaining = round(remaining, 4)
	res.Status = statusFor(sli, target, burn, window)
	return res, nil
}

func statusFor(sli, target, burn float64, window domain.SLOWindow) string {
	if sli+1e-12 < target {
		return domain.SLOBreached
	}
	threshold := window.BurnAlert
	if threshold <= 0 {
		threshold = defaultAtRiskBurn
	}
	if burn >= threshold {
		return domain.SLOAtRisk
	}
	return domain.SLOHealthy
}

// OverallStatus rolls window results into one service status.
// A compliance window below target is a breach. Otherwise a short-window
// burn alert is at-risk. No data across all windows stays no_data.
func OverallStatus(results []domain.SLOResult) string {
	if len(results) == 0 {
		return domain.SLONoData
	}
	sawData := false
	atRisk := false
	for _, r := range results {
		switch r.Status {
		case domain.SLOBreached:
			return domain.SLOBreached
		case domain.SLOAtRisk:
			atRisk = true
			sawData = true
		case domain.SLOHealthy:
			sawData = true
		}
	}
	if atRisk {
		return domain.SLOAtRisk
	}
	if !sawData {
		return domain.SLONoData
	}
	return domain.SLOHealthy
}

// MultiBurn pages only when both the short and long windows exceed their
// burn-rate thresholds. This is the Google SRE multi-window check.
func MultiBurn(short, long domain.SLOResult, shortThreshold, longThreshold float64) bool {
	if short.Status == domain.SLONoData || long.Status == domain.SLONoData {
		return false
	}
	if shortThreshold <= 0 {
		shortThreshold = 14.4
	}
	if longThreshold <= 0 {
		longThreshold = 6
	}
	return short.BurnRate >= shortThreshold && long.BurnRate >= longThreshold
}

func round(v float64, places int) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	p := math.Pow(10, float64(places))
	return math.Round(v*p) / p
}

// ParseObjective accepts 99.95 or 99.95%.
func ParseObjective(raw string) (float64, error) {
	s := raw
	if len(s) > 0 && s[len(s)-1] == '%' {
		s = s[:len(s)-1]
	}
	var v float64
	_, err := fmt.Sscanf(s, "%f", &v)
	if err != nil {
		return 0, fmt.Errorf("parse objective %q: %w", raw, err)
	}
	return v, nil
}
