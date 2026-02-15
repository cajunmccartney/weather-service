package handlers

import (
	"encoding/json"
	"net/http"
)

// Health returns liveness status (always 200 if process is alive)
// Note: This is a liveness probe, not readiness (doesn't check dependencies)
func Health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status": "ok",
	})
}
