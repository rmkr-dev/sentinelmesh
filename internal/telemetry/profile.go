package telemetry

import (
	"log/slog"
	"net/http"
	_ "net/http/pprof"
	"os"
	"strings"

	"github.com/grafana/pyroscope-go"
)

func startProfiling(service string) func() {
	var stops []func()
	if os.Getenv("PPROF_ENABLED") == "true" {
		srv := &http.Server{Addr: "127.0.0.1:6060", Handler: http.DefaultServeMux}
		go func() {
			if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				slog.Warn("pprof listener", "error", err.Error())
			}
		}()
		stops = append(stops, func() { _ = srv.Close() })
	}
	if strings.EqualFold(os.Getenv("PROFILING_ENABLED"), "true") {
		profiler, err := pyroscope.Start(pyroscope.Config{
			ApplicationName: service,
			ServerAddress:   env("PYROSCOPE_URL", "http://localhost:4040"),
			Tags:            map[string]string{"service_name": service},
			ProfileTypes: []pyroscope.ProfileType{
				pyroscope.ProfileCPU,
				pyroscope.ProfileAllocObjects,
				pyroscope.ProfileAllocSpace,
				pyroscope.ProfileInuseObjects,
				pyroscope.ProfileInuseSpace,
				pyroscope.ProfileGoroutines,
				pyroscope.ProfileMutexCount,
				pyroscope.ProfileMutexDuration,
			},
		})
		if err != nil {
			slog.Warn("profiling disabled", "error", err.Error())
		} else {
			stops = append(stops, func() { _ = profiler.Stop() })
		}
	}
	if len(stops) == 0 {
		return nil
	}
	return func() {
		for _, fn := range stops {
			fn()
		}
	}
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
