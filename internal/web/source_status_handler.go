package web

import (
	"encoding/json"
	"net/http"

	"ai-flight-dashboard/internal/db"
	"ai-flight-dashboard/internal/onboarding"
)

func handleSourceStatus(w http.ResponseWriter, r *http.Request, database *db.DB) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	deviceID := r.URL.Query().Get("device")
	status, err := onboarding.BuildSourceCoverage(database, onboarding.Options{
		DeviceID: deviceID,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}
