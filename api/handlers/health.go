package handlers

import (
	"fmt"
	"net/http"
)

func HealthHandler(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "OK")
}
