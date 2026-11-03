// Package httpx holds small HTTP helpers shared by all modules.
package httpx

import (
	"encoding/json"
	"net/http"
)

// Health reports that the process is up. Readiness checks (database, queue)
// are added once those dependencies exist.
func Health(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
