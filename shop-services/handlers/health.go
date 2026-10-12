package handlers

import (
	"fmt"
	"net/http"
	"os"
	"strings"
)

func HealthHandler(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	if strings.EqualFold(os.Getenv("HEALTH_FAIL"), "true") {
		http.Error(w, "health failure requested", http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "ok")
}
