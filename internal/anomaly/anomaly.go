// Package anomaly implements pluggable statistical detectors.
// These are threshold and distribution checks, not trained models.
package anomaly

import (
	"fmt"
	"math"
	"sort"
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
		MADDetector{},
		EWMADetector{},
		SeasonalDetector{},
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

// MADDetector flags the latest point using a robust z-score (median and MAD).
type MADDetector struct{}

func (MADDetector) Name() string { return "mad" }

func (MADDetector) Detect(series Series, p Params) ([]domain.Anomaly, error) {
	minPoints := orInt(p.MinPoints, 8)
	if len(series.Points) < minPoints {
		return nil, nil
	}
	last := series.Points[len(series.Points)-1]
	base := values(series.Points[:len(series.Points)-1])
	med := median(base)
	var devs []float64
	for _, v := range base {
		devs = append(devs, math.Abs(v-med))
	}
	mad := median(devs)
	if mad == 0 {
		return nil, nil
	}
	score := 0.6745 * (last.Value - med) / mad
	limit := orDefault(p.Threshold, 3.5)
	if math.Abs(score) < limit {
		return nil, nil
	}
	return []domain.Anomaly{finding(series, "mad", score, last.Value, med, last.Time,
		fmt.Sprintf("robust z-score %.2f exceeds %.2f", score, limit))}, nil
}

// EWMADetector compares the latest point with an exponentially weighted mean.
type EWMADetector struct{}

func (EWMADetector) Name() string { return "ewma" }

func (EWMADetector) Detect(series Series, p Params) ([]domain.Anomaly, error) {
	if len(series.Points) < 5 {
		return nil, nil
	}
	alpha := 0.3
	mean := series.Points[0].Value
	var sq float64
	for _, pt := range series.Points[1 : len(series.Points)-1] {
		delta := pt.Value - mean
		mean += alpha * delta
		sq = (1-alpha)*sq + alpha*delta*delta
	}
	std := math.Sqrt(sq)
	last := series.Points[len(series.Points)-1]
	limit := orDefault(p.Threshold, 3)
	if std == 0 || math.Abs(last.Value-mean) < limit*std {
		return nil, nil
	}
	return []domain.Anomaly{finding(series, "ewma", (last.Value-mean)/std, last.Value, mean, last.Time,
		fmt.Sprintf("value %g is outside the EWMA band around %g", last.Value, mean))}, nil
}

// SeasonalDetector compares the latest point with the point one season earlier.
// Season length is Params.Window samples (default 7).
type SeasonalDetector struct{}

func (SeasonalDetector) Name() string { return "seasonal" }

func (SeasonalDetector) Detect(series Series, p Params) ([]domain.Anomaly, error) {
	season := orInt(p.Window, 7)
	if len(series.Points) <= season {
		return nil, nil
	}
	last := series.Points[len(series.Points)-1]
	prev := series.Points[len(series.Points)-1-season]
	if prev.Value == 0 {
		return nil, nil
	}
	delta := (last.Value - prev.Value) / math.Abs(prev.Value)
	limit := orDefault(p.Threshold, 0.5)
	if math.Abs(delta) < limit {
		return nil, nil
	}
	return []domain.Anomaly{finding(series, "seasonal", delta, last.Value, prev.Value, last.Time,
		fmt.Sprintf("seasonal change %.2f versus %d samples earlier", delta, season))}, nil
}

func median(vs []float64) float64 {
	if len(vs) == 0 {
		return 0
	}
	cp := append([]float64{}, vs...)
	sort.Float64s(cp)
	n := len(cp)
	if n%2 == 1 {
		return cp[n/2]
	}
	return (cp[n/2-1] + cp[n/2]) / 2
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
