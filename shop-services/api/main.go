package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"simpleapi/handlers"
	"simpleapi/internal/orders"
	"simpleapi/internal/store"
	"simpleapi/telemetry"
	"strconv"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	appCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	db, err := store.Open()
	if err != nil {
		log.Fatal("DATABASE_URL must contain a valid PostgreSQL connection string")
	}
	defer db.Close()
	if len(os.Args) > 1 && os.Args[1] == "migrate" {
		ctx, cancel := context.WithTimeout(appCtx, 30*time.Second)
		defer cancel()
		if err := store.Migrate(ctx, db); err != nil {
			log.Fatal("schema migration failed: ", err)
		}
		log.Println("schema migration completed")
		return
	}
	tp, err := telemetry.InitTracer()
	if err != nil {
		log.Fatalf("Не удалось запусть трасер: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = tp.Shutdown(ctx)
	}()

	cpuManager := handlers.NewCPUManager(appCtx)

	mux := http.NewServeMux()
	mux.HandleFunc("/health", handlers.HealthHandler)
	mux.HandleFunc("/eat", handlers.EatHandler)
	mux.HandleFunc("/burn", handlers.BurnHandler(cpuManager))
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/fail", handlers.FailHandler)
	mux.HandleFunc("/slow", handlers.SlowHandler)
	mux.HandleFunc("/load", handlers.LoadHandler)
	version := os.Getenv("API_VERSION")
	if version == "" {
		version = "v1"
	}
	if version != "v1" && version != "v2" {
		log.Fatalf("API_VERSION must be v1 or v2")
	}
	mux.Handle("/version", versionHandler(version))
	shop := orders.API{DB: db}
	mux.HandleFunc("/order", shop.Create)
	mux.HandleFunc("/orders", shop.List)
	mux.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := db.PingContext(ctx); err != nil {
			http.Error(w, "database unavailable", 503)
			return
		}
		w.Write([]byte("ok"))
	})

	addr := os.Getenv("LISTEN_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	fail, err := envBool("FAIL")
	if err != nil {
		log.Fatal(err)
	}
	flaky, err := envBool("FLAKY")
	if err != nil {
		log.Fatal(err)
	}
	server := &http.Server{
		Addr:              addr,
		Handler:           releaseBehavior(mux, version, fail, flaky),
		ReadHeaderTimeout: 5 * time.Second,
	}

	serverErrors := make(chan error, 1)

	go func() {
		telemetry.WriteLog("INFO", "Server starting on "+addr, "", "")
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErrors <- err
		}
	}()

	select {
	case err := <-serverErrors:
		log.Fatalf("Critical error: %v", err)

	case <-appCtx.Done():
		log.Println("Shutdown signal received, shutting down server...")

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Fatalf("Critical error while shutting down server: %v", err)
			_ = server.Close()
		}
	}

	log.Println("Server successfully and safely completed work.")
}

func envBool(name string) (bool, error) {
	value := os.Getenv(name)
	if value == "" {
		return false, nil
	}
	enabled, err := strconv.ParseBool(value)
	if err != nil {
		return false, errors.New(name + " must be a boolean")
	}
	return enabled, nil
}
