package handlers

import (
	"net/http"
	"simpleapi/telemetry"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

func FailHandler(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	tracer := otel.Tracer("api-tracer")
	_, span := tracer.Start(req.Context(), "GET /fail")
	defer span.End()
	traceID := span.SpanContext().TraceID().String()

	span.SetStatus(1, "Internal Server Error")
	span.SetAttributes(attribute.String("error", "true"))

	telemetry.HttpRequestsTotal.WithLabelValues("/fail", "500").Inc()
	telemetry.WriteLog("ERROR", "Внутренняя ошибка на ручка /fail", "/fail", traceID)

	w.WriteHeader(http.StatusInternalServerError)
	w.Write([]byte("error"))
}
