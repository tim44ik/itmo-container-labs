package main

import (
	"encoding/json"
	"math/rand"
	"net/http"
	"strconv"
	"time"

	"simpleapi/telemetry"
)

func versionHandler(version string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		status := http.StatusOK
		defer func() {
			telemetry.HttpRequestsTotal.WithLabelValues("/version", strconv.Itoa(status)).Inc()
			telemetry.HttpDurationHistogram.WithLabelValues("/version").Observe(time.Since(start).Seconds())
		}()
		if r.Method != http.MethodGet {
			status = http.StatusMethodNotAllowed
			w.WriteHeader(status)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"version": version})
	})
}

func releaseBehavior(next http.Handler, version string, fail, flaky bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if version == "v2" && fail && path == "/order" && r.Method == http.MethodPost {
			writeInjectedFailure(w, path)
			return
		}
		if version == "v1" && flaky && path == "/orders" && r.Method == http.MethodGet && rand.Intn(2) == 0 {
			writeInjectedFailure(w, path)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeInjectedFailure(w http.ResponseWriter, path string) {
	start := time.Now()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusInternalServerError)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": "injected release failure"})
	telemetry.HttpRequestsTotal.WithLabelValues(path, "500").Inc()
	telemetry.HttpDurationHistogram.WithLabelValues(path).Observe(time.Since(start).Seconds())
}
