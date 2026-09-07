package handlers

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"runtime"
	"sync"
)

type CPUManager struct {
	mu         sync.Mutex
	ctx        context.Context
	activeJobs int
	maxCPU     int
}

func NewCPUManager(ctx context.Context) *CPUManager {
	return &CPUManager{
		ctx:    ctx,
		maxCPU: runtime.NumCPU(),
	}
}

func (cm *CPUManager) StartWorker() bool {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if cm.activeJobs >= cm.maxCPU {
		return false
	}

	cm.activeJobs++

	go func() {
		defer func() {
			cm.mu.Lock()
			cm.activeJobs--
			cm.mu.Unlock()
		}()

		for range cm.ctx.Done() {
			return
		}
	}()

	return true
}

func BurnHandler(cm *CPUManager) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodGet {
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}

		if success := cm.StartWorker(); !success {
			w.WriteHeader(http.StatusTooManyRequests)
			fmt.Fprintf(w, "Failed: Maximum CPU load reached (%d cores).", cm.maxCPU)
			return
		}

		w.WriteHeader(http.StatusOK)
		log.Println("1 more core loaded")
	}
}
