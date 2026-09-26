package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os/signal"
	"simpleapi/handlers"
	"simpleapi/telemetry"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	appCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	tp, err := telemetry.InitTracer()
	if err != nil {
		log.Fatalf("Не удалось запусть трасер: %v", err)
	}
	defer func() { _ = tp.Shutdown(appCtx) }()

	defer stop()

	cpuManager := handlers.NewCPUManager(appCtx)

	mux := http.NewServeMux()
	mux.HandleFunc("/health", handlers.HealthHandler)
	mux.HandleFunc("/eat", handlers.EatHandler)
	mux.HandleFunc("/burn", handlers.BurnHandler(cpuManager))
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/fail", handlers.FailHandler)
	mux.HandleFunc("/slow", handlers.SlowHandler)
	mux.HandleFunc("/load", handlers.LoadHandler)

	server := &http.Server{
		Addr:    ":8080",
		Handler: mux,
	}

	serverErrors := make(chan error, 1)

	go func() {
		telemetry.WriteLog("INFO", "Server started at http://localhost:8080", "", "")
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
