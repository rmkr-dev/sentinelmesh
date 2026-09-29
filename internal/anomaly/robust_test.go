package anomaly

import (
	"testing"
	"time"

	"github.com/rmkr-dev/sentinelmesh/internal/domain"
)

func TestMADIgnoresShortSeries(t *testing.T) {
	series := Series{Service: "payment-service", Metric: "error_ratio", Points: []domain.MetricPoint{
		{Time: time.Now(), Value: 1},
	}}
	got, err := MADDetector{}.Detect(series, Params{})
	if err != nil || len(got) != 0 {
		t.Fatalf("%+v %v", got, err)
	}
}

func TestSeasonalDetectsJump(t *testing.T) {
	var points []domain.MetricPoint
	for i := 0; i < 8; i++ {
		points = append(points, domain.MetricPoint{Time: time.Unix(int64(i), 0), Value: 1})
	}
	points = append(points, domain.MetricPoint{Time: time.Unix(8, 0), Value: 5})
	got, err := SeasonalDetector{}.Detect(Series{Service: "s", Metric: "m", Points: points}, Params{Window: 7, Threshold: 0.5})
	if err != nil || len(got) != 1 {
		t.Fatalf("%+v %v", got, err)
	}
}
