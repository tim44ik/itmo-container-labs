package handlers

import (
	"fmt"
	"math/rand"
	"net/http"
	"simpleapi/telemetry"
	"time"

	"go.opentelemetry.io/otel"
)

func SlowHandler(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	tracer := otel.Tracer("api-tracer")
	ctx, span := tracer.Start(req.Context(), "GET /slow")
	defer span.End()
	traceID := span.SpanContext().TraceID().String()

	start := time.Now()

	_, childSpan := tracer.Start(ctx, "slow-op")
	sleepTime := time.Duration(1+rand.Intn(3)) * time.Second
	time.Sleep(sleepTime)
	childSpan.End()

	telemetry.HttpRequestsTotal.WithLabelValues("slow", "200").Inc()
	telemetry.HttpDurationHistogram.WithLabelValues("/slow").Observe(time.Since(start).Seconds())
	telemetry.WriteLog("INFO", fmt.Sprintf("Запрос обработан за %v", sleepTime), "/slow", traceID)

	w.Write([]byte("slow response"))
}
