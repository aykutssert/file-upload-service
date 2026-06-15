package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

const readinessTimeout = 2 * time.Second

type healthResponse struct {
	Status string `json:"status"`
}

type readinessChecker interface {
	Ping(context.Context) error
}

func liveHandler(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, healthResponse{Status: "ok"})
}

func readyHandler(checker readinessChecker) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		ctx, cancel := context.WithTimeout(request.Context(), readinessTimeout)
		defer cancel()

		if err := checker.Ping(ctx); err != nil {
			writeJSON(
				w,
				http.StatusServiceUnavailable,
				healthResponse{Status: "unavailable"},
			)
			return
		}

		writeJSON(w, http.StatusOK, healthResponse{Status: "ready"})
	}
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
