package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"sync"
	"syscall"
	"time"
)

func envInt(name string, fallback, min, max int) int {
	raw := os.Getenv(name)
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < min || value > max {
		log.Fatalf("%s must be an integer between %d and %d", name, min, max)
	}
	return value
}

func main() {
	cpuWorkers := envInt("CPU_WORKERS", 1, 0, 64)
	memoryMiB := envInt("MEMORY_MIB", 64, 0, 16384)

	ctx, stop := signal.NotifyContext(
		context.Background(), syscall.SIGTERM, syscall.SIGINT,
	)
	defer stop()

	log.Printf("starting batch: CPU_WORKERS=%d MEMORY_MIB=%d",
		cpuWorkers, memoryMiB)

	// Touch every page so the allocation consumes physical memory.
	memory := make([]byte, memoryMiB*1024*1024)
	for i := 0; i < len(memory); i += 4096 {
		memory[i] = 1
	}

	var workers sync.WaitGroup
	for i := 0; i < cpuWorkers; i++ {
		workers.Add(1)
		go func(seed uint64) {
			defer workers.Done()
			value := seed
			for {
				select {
				case <-ctx.Done():
					runtime.KeepAlive(value)
					return
				default:
					for j := 0; j < 100000; j++ {
						value = value*6364136223846793005 + 1
					}
				}
			}
		}(uint64(i + 1))
	}

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			workers.Wait()
			runtime.KeepAlive(memory)
			log.Println("batch stopped")
			return
		case <-ticker.C:
			// Keep the allocated pages active.
			for i := 0; i < len(memory); i += 4096 {
				memory[i]++
			}
			log.Printf("running: CPU_WORKERS=%d MEMORY_MIB=%d",
				cpuWorkers, memoryMiB)
		}
	}
}
