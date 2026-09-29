// Package anomaly implements pluggable statistical detectors.
// These are threshold and distribution checks, not trained models.
package anomaly

import (
	"fmt"
	"math"
	"time"

	"github.com/rmkr-dev/sentinelmesh/internal/domain"
)

// Params configures a detector. Zero values select documented defaults.
type Params struct {
	Threshold float64
	Window    int
	MinPoints int
}

// Detector scores a series and returns zero or more anomalies.
type Detector interface {
	Name() string
	Detect(series Series, p Params) ([]domain.Anomaly, error)
}

// Series is an ordered metric.
type Series struct {
	Service string
	Metric  string
	Points  []domain.MetricPoint
}

// Registry holds the built-in detectors.
func Registry() []Detector {
	return []Detector{
		ZScoreDetector{},
		RollingWindowDetector{},
		ThresholdDetector{},
		RateChangeDetector{},
	}
}

// StableID is the upsert key for one detector finding in a one-minute bucket.
func StableID(service, detector, metric string, at time.Time) string {
	bucket := at.UTC().Truncate(time.Minute).Unix()
	return fmt.Sprintf("%s|%s|%s|%d", service, detector, metric, bucket)
}

func ByName(name string) (Detector, bool) {
	for _, d := range Registry() {
		if d.Name() == name {
			return d, true
		}
	}
	return nil, false
}

// ZScoreDetector flags the latest point when its z-score against the prior
// window exceeds the threshold (default 3).
type ZScoreDetector struct{}

func (ZScoreDetector) Name() string { return "zscore" }

func (ZScoreDetector) Detect(series Series, p Params) ([]domain.Anomaly, error) {
	threshold := orDefault(p.Threshold, 3)
	minPoints := orInt(p.MinPoints, 8)
	if len(series.Points) < minPoints {
		return nil, nil
	}
	last := series.Points[len(series.Points)-1]
	base := series.Points[:len(series.Points)-1]
	mean, std := meanStd(values(base))
	if std == 0 {
		if last.Value == mean {
			return nil, nil
		}
		return []domain.Anomaly{finding(series, "zscore", math.Inf(1), last.Value, mean, last.Time,
			fmt.Sprintf("value %g deviates from a zero-variance baseline of %g", last.Value, mean))}, nil
	}
	z := (last.Value - mean) / std
	if math.Abs(z) < threshold {
		return nil, nil
	}
	return []domain.Anomaly{finding(series, "zscore", z, last.Value, mean, last.Time,
		fmt.Sprintf("z-score %.2f exceeds %.2f (value %g, baseline mean %g)", z, threshold, last.Value, mean))}, nil
}

// RollingWindowDetector compares the latest point with the mean and standard
// deviation of the previous N points.
type RollingWindowDetector struct{}

func (RollingWindowDetector) Name() string { return "rolling_window" }

func (RollingWindowDetector) Detect(series Series, p Params) ([]domain.Anomaly, error) {
	window := orInt(p.Window, 10)
	k := orDefault(p.Threshold, 3)
	if len(series.Points) < window+1 {
		return nil, nil
	}
	last := series.Points[len(series.Points)-1]
	base := series.Points[len(series.Points)-1-window : len(series.Points)-1]
	mean, std := meanStd(values(base))
	limit := mean + k*std
	low := mean - k*std
	if last.Value <= limit && last.Value >= low {
		return nil, nil
	}
	return []domain.Anomaly{finding(series, "rolling_window", last.Value-mean, last.Value, mean, last.Time,
		fmt.Sprintf("value %g is outside rolling band [%g, %g]", last.Value, low, limit))}, nil
}

// ThresholdDetector flags the latest point when it crosses a static threshold.
type ThresholdDetector struct{}

func (ThresholdDetector) Name() string { return "threshold" }

func (ThresholdDetector) Detect(series Series, p Params) ([]domain.Anomaly, error) {
	if len(series.Points) == 0 {
		return nil, nil
	}
	last := series.Points[len(series.Points)-1]
	if last.Value < p.Threshold {
		return nil, nil
	}
	return []domain.Anomaly{finding(series, "threshold", last.Value, last.Value, p.Threshold, last.Time,
		fmt.Sprintf("value %g crossed threshold %g", last.Value, p.Threshold))}, nil
}

// RateChangeDetector flags a fractional jump between the last two points.
// Threshold is a fraction (0.5 = 50% increase). A zero baseline with a
// positive latest value is reported as a rate change.
type RateChangeDetector struct{}

func (RateChangeDetector) Name() string { return "rate_of_change" }

func (RateChangeDetector) Detect(series Series, p Params) ([]domain.Anomaly, error) {
	threshold := orDefault(p.Threshold, 0.5)
	if len(series.Points) < 2 {
		return nil, nil
	}
	prev := series.Points[len(series.Points)-2]
	last := series.Points[len(series.Points)-1]
	var rate float64
	switch {
	case prev.Value == 0 && last.Value == 0:
		return nil, nil
	case prev.Value == 0:
		rate = math.Inf(1)
	default:
		rate = (last.Value - prev.Value) / math.Abs(prev.Value)
	}
	if rate < threshold {
		return nil, nil
	}
	return []domain.Anomaly{finding(series, "rate_of_change", rate, last.Value, prev.Value, last.Time,
		fmt.Sprintf("rate of change %.2f exceeds %.2f (from %g to %g)", rate, threshold, prev.Value, last.Value))}, nil
}

func finding(series Series, detector string, score, value, baseline float64, at time.Time, summary string) domain.Anomaly {
	return domain.Anomaly{
		Service:    series.Service,
		Metric:     series.Metric,
		Detector:   detector,
		Score:      score,
		Value:      value,
		Baseline:   baseline,
		Summary:    summary,
		DetectedAt: at,
	}
}

func values(points []domain.MetricPoint) []float64 {
	out := make([]float64, len(points))
	for i, p := range points {
		out[i] = p.Value
	}
	return out
}

func meanStd(vs []float64) (float64, float64) {
	if len(vs) == 0 {
		return 0, 0
	}
	var sum float64
	for _, v := range vs {
		sum += v
	}
	mean := sum / float64(len(vs))
	var ss float64
	for _, v := range vs {
		d := v - mean
		ss += d * d
	}
	variance := ss / float64(len(vs))
	return mean, math.Sqrt(variance)
}

func orDefault(v, def float64) float64 {
	if v == 0 {
		return def
	}
	return v
}

func orInt(v, def int) int {
	if v == 0 {
		return def
	}
	return v
}
