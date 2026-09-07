package handlers

import (
	"log"
	"net/http"
	"strconv"
	"sync"
)

var (
	alloc [][]byte
	mu    sync.Mutex
)

func EatHandler(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	mb := req.URL.Query().Get("mb")
	conv, err := strconv.Atoi(mb)
	if err != nil || conv <= 0 {
		http.Error(w, "Invalid 'mb' parameter", http.StatusBadRequest)
		return
	}

	buf := make([]byte, 1024*1024*conv)
	for i := 0; i < len(buf); i += 4096 {
		buf[i] = 1
	}

	mu.Lock()
	alloc = append(alloc, buf)
	mu.Unlock()

	w.WriteHeader(http.StatusOK)
	log.Printf("%d MB allocated\n", conv)
}
