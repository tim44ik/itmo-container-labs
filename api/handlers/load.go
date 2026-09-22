package handlers

import (
	"net/http"
	"simpleapi/telemetry"

	"go.opentelemetry.io/otel"
)

func LoadHandler(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	tracer := otel.Tracer("api-tracer")
	_, span := tracer.Start(req.Context(), "GET /load")
	defer span.End()
	traceID := span.SpanContext().TraceID().String()

	telemetry.WriteLog("INFO", "RPS надо поднять", "/load", traceID)

	for range 50 {
		telemetry.HttpRequestsTotal.WithLabelValues("/load", "200").Inc()
	}

	w.Write([]byte("load generated"))
}
