package anomaly

import (
	"math"
	"testing"
	"time"

	"github.com/rmkr-dev/sentinelmesh/internal/domain"
)

func series(vals ...float64) Series {
	base := time.Date(2026, 9, 28, 10, 0, 0, 0, time.UTC)
	s := Series{Service: "payment-service", Metric: "error_ratio"}
	for i, v := range vals {
		s.Points = append(s.Points, domain.MetricPoint{Time: base.Add(time.Duration(i) * time.Minute), Value: v})
	}
	return s
}

func TestZScoreFlagsSpike(t *testing.T) {
	s := series(0.01, 0.012, 0.009, 0.011, 0.01, 0.013, 0.008, 0.01, 0.5)
	got, err := ZScoreDetector{}.Detect(s, Params{Threshold: 3, MinPoints: 8})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d anomalies", len(got))
	}
	if got[0].Detector != "zscore" || got[0].Value != 0.5 {
		t.Fatalf("%+v", got[0])
	}
}

func TestZScoreQuietSeries(t *testing.T) {
	s := series(0.01, 0.012, 0.009, 0.011, 0.01, 0.013, 0.008, 0.011)
	got, err := ZScoreDetector{}.Detect(s, Params{})
	if err != nil || len(got) != 0 {
		t.Fatalf("got %+v err %v", got, err)
	}
}

func TestThreshold(t *testing.T) {
	s := series(0.01, 0.2)
	got, err := ThresholdDetector{}.Detect(s, Params{Threshold: 0.05})
	if err != nil || len(got) != 1 {
		t.Fatal(got, err)
	}
	got, err = ThresholdDetector{}.Detect(s, Params{Threshold: 0.5})
	if err != nil || len(got) != 0 {
		t.Fatal(got, err)
	}
}

func TestRateOfChange(t *testing.T) {
	s := series(0.01, 0.04)
	got, err := RateChangeDetector{}.Detect(s, Params{Threshold: 0.5})
	if err != nil || len(got) != 1 {
		t.Fatal(got, err)
	}
	if math.Abs(got[0].Score-3) > 0.01 {
		t.Fatalf("score %v", got[0].Score)
	}
}

func TestRollingWindow(t *testing.T) {
	vals := []float64{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 20}
	got, err := RollingWindowDetector{}.Detect(series(vals...), Params{Window: 10, Threshold: 3})
	if err != nil || len(got) != 1 {
		t.Fatal(got, err)
	}
}

func TestRegistryNames(t *testing.T) {
	for _, name := range []string{"zscore", "rolling_window", "threshold", "rate_of_change"} {
		if _, ok := ByName(name); !ok {
			t.Fatal(name)
		}
	}
}
