package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"simpleapi/handlers"
	"simpleapi/internal/store"
	"simpleapi/telemetry"
)

var processed = prometheus.NewCounter(prometheus.CounterOpts{Name: "worker_orders_processed_total", Help: "Committed order status transitions."})
var failures = prometheus.NewCounter(prometheus.CounterOpts{Name: "worker_errors_total", Help: "Worker polling or transaction failures."})
var lastPoll atomic.Int64

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if os.Getenv("OTEL_SERVICE_NAME") == "" {
		_ = os.Setenv("OTEL_SERVICE_NAME", "shop-worker")
	}
	tp, err := telemetry.InitTracer()
	if err != nil {
		return err
	}
	defer func() {
		c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = tp.Shutdown(c)
	}()
	db, err := store.Open()
	if err != nil {
		return err
	}
	defer db.Close()
	prometheus.MustRegister(processed, failures)
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		if time.Now().Unix()-lastPoll.Load() > 30 {
			http.Error(w, "worker loop stalled", 503)
			return
		}
		handlers.HealthHandler(w, r)
	})
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
		c, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if db.PingContext(c) != nil {
			http.Error(w, "database unavailable", 503)
			return
		}
		fmt.Fprint(w, "ok")
	})
	lastPoll.Store(time.Now().Unix())
	addr := os.Getenv("LISTEN_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	server := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	errCh := make(chan error, 1)
	go func() { errCh <- server.ListenAndServe() }()
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				c, cancel := context.WithTimeout(ctx, 5*time.Second)
				err := process(c, db)
				cancel()
				lastPoll.Store(time.Now().Unix())
				if err != nil && ctx.Err() == nil {
					failures.Inc()
					telemetry.WriteLog("ERROR", "order processing failed", "worker", "")
				}
			}
		}
	}()
	select {
	case <-ctx.Done():
	case err = <-errCh:
		stop()
	}
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = server.Shutdown(c)
	<-done
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func process(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var id int64
	var parent string
	err = tx.QueryRowContext(ctx, `SELECT id,traceparent FROM orders WHERE status='pending' ORDER BY id FOR UPDATE SKIP LOCKED LIMIT 1`).Scan(&id, &parent)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	ctx = otel.GetTextMapPropagator().Extract(ctx, propagation.MapCarrier{"traceparent": parent})
	ctx, span := otel.Tracer("shop-worker").Start(ctx, "process-order", trace.WithSpanKind(trace.SpanKindConsumer), trace.WithAttributes(attribute.Int64("order.id", id)))
	defer span.End()
	_, err = tx.ExecContext(ctx, `UPDATE orders SET status='processed',processed_at=now() WHERE id=$1`, id)
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "transaction failed")
		return err
	}
	processed.Inc()
	telemetry.WriteLog("INFO", "order "+strconv.FormatInt(id, 10)+" processed", "worker", span.SpanContext().TraceID().String())
	return nil
}

func main() {
	if err := run(); err != nil {
		telemetry.WriteLog("ERROR", "worker stopped: "+err.Error(), "", "")
		os.Exit(1)
	}
}
