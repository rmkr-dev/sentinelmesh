package slo

import (
	"math"
	"testing"
	"time"

	"github.com/rmkr-dev/sentinelmesh/internal/domain"
)

func def() domain.SLODefinition {
	return domain.SLODefinition{
		Service:   "payment-service",
		Name:      "availability",
		Objective: 99.95,
		Indicator: "availability",
		Windows: []domain.SLOWindow{{
			Name:       "30d",
			Duration:   30 * 24 * time.Hour,
			Compliance: true,
		}},
	}
}

func TestEvaluateAvailabilityBudget(t *testing.T) {
	// 9972 good of 10000 → SLI 99.72%. Allowed error ratio is 0.05%.
	// Actual error ratio is 0.28%. Burn rate = 0.28 / 0.05 = 5.6.
	res, err := Evaluate(def(), def().Windows[0], Counts{Good: 9972, Total: 10000}, time.Date(2026, 9, 28, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(res.Current-99.72) > 0.001 {
		t.Fatalf("current = %v", res.Current)
	}
	if math.Abs(res.BurnRate-5.6) > 0.001 {
		t.Fatalf("burn = %v", res.BurnRate)
	}
	if res.ErrorBudgetRemaining >= 0 {
		t.Fatalf("budget should be exhausted, remaining=%v", res.ErrorBudgetRemaining)
	}
	if res.Status != domain.SLOBreached {
		t.Fatalf("status = %s", res.Status)
	}
	if res.Target != 99.95 || res.Service != "payment-service" || res.SLO != "availability" {
		t.Fatalf("identity fields = %+v", res)
	}
}

func TestEvaluateNoData(t *testing.T) {
	res, err := Evaluate(def(), def().Windows[0], Counts{}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != domain.SLONoData {
		t.Fatal(res.Status)
	}
}

func TestEvaluateHealthy(t *testing.T) {
	res, err := Evaluate(def(), def().Windows[0], Counts{Good: 9999, Total: 10000}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	// error ratio 0.01%, allowed 0.05%, burn 0.2
	if res.Status != domain.SLOHealthy {
		t.Fatalf("status=%s burn=%v", res.Status, res.BurnRate)
	}
	if res.ErrorBudgetRemaining <= 0 {
		t.Fatal(res.ErrorBudgetRemaining)
	}
}

func TestAtRiskBurnWithoutBreach(t *testing.T) {
	// Objective 99% so 99.5% SLI is still above target, but burn is 0.5/1 = 0.5.
	// Use a low burn alert to force at_risk: burn of a 98%? Wait need sli >= target and burn high.
	// target 99%, allowed 1%. If SLI is 99.0 exactly, burn = 1.0. Use threshold 1 and SLI just above.
	d := def()
	d.Objective = 99
	w := domain.SLOWindow{Name: "5m", BurnAlert: 1}
	// 995 good / 1000 = 99.5%. error 0.5%, burn = 0.5. Not at risk.
	// 991/1000 = 99.1%. error 0.9%, burn = 0.9. Still under 1 and above target.
	// To exceed burn 2 while staying above 99%: error ratio < 1% and burn>=2 means error/0.01 >= 2 → error >= 2% which is below target.
	// So at_risk with burn>=2 and above target is impossible for threshold 2 when allowed is small...
	// burn = error/allowed. sli >= target means error <= allowed means burn <= 1.
	// Therefore at_risk ONLY happens when BurnAlert <= 1, i.e. a fast-burn threshold on a window
	// that is still inside budget. Use BurnAlert 0.5.
	w.BurnAlert = 0.4
	res, err := Evaluate(d, w, Counts{Good: 995, Total: 1000}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != domain.SLOAtRisk {
		t.Fatalf("status=%s burn=%v current=%v", res.Status, res.BurnRate, res.Current)
	}
}

func TestMultiBurn(t *testing.T) {
	short := domain.SLOResult{Status: domain.SLOAtRisk, BurnRate: 15}
	long := domain.SLOResult{Status: domain.SLOAtRisk, BurnRate: 7}
	if !MultiBurn(short, long, 14.4, 6) {
		t.Fatal("expected page")
	}
	long.BurnRate = 2
	if MultiBurn(short, long, 14.4, 6) {
		t.Fatal("long window should suppress the page")
	}
	short.Status = domain.SLONoData
	if MultiBurn(short, long, 14.4, 6) {
		t.Fatal("no data must not page")
	}
}

func TestRejectBadInput(t *testing.T) {
	if _, err := Evaluate(def(), def().Windows[0], Counts{Good: 10, Total: 1}, time.Now()); err == nil {
		t.Fatal("expected error")
	}
	d := def()
	d.Objective = 100
	if _, err := Evaluate(d, d.Windows[0], Counts{Good: 1, Total: 1}, time.Now()); err == nil {
		t.Fatal("expected error for 100% objective")
	}
}

func TestOverallStatus(t *testing.T) {
	if OverallStatus(nil) != domain.SLONoData {
		t.Fatal()
	}
	if OverallStatus([]domain.SLOResult{{Status: domain.SLOHealthy}, {Status: domain.SLOBreached}}) != domain.SLOBreached {
		t.Fatal()
	}
	if OverallStatus([]domain.SLOResult{{Status: domain.SLOHealthy}, {Status: domain.SLOAtRisk}}) != domain.SLOAtRisk {
		t.Fatal()
	}
}

func TestParseObjective(t *testing.T) {
	v, err := ParseObjective("99.95%")
	if err != nil || v != 99.95 {
		t.Fatal(v, err)
	}
}
