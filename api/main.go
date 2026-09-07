package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os/signal"
	"simpleapi/handlers"
	"syscall"
	"time"
)

func main() {
	appCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cpuManager := handlers.NewCPUManager(appCtx)

	mux := http.NewServeMux()
	mux.HandleFunc("/health", handlers.HealthHandler)
	mux.HandleFunc("/eat", handlers.EatHandler)
	mux.HandleFunc("/burn", handlers.BurnHandler(cpuManager))

	server := &http.Server{
		Addr:    ":8080",
		Handler: mux,
	}

	serverErrors := make(chan error, 1)

	go func() {
		log.Println("Server started at http://localhost:8080")
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
			log.Printf("Critical error while shutting down server: %v", err)
			_ = server.Close()
		}
	}

	log.Println("Server successfully and safely completed work.")
}
