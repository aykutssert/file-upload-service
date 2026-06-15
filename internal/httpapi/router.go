package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

func NewRouter(checker readinessChecker) http.Handler {
	router := chi.NewRouter()
	router.Get("/health/live", liveHandler)
	router.Get("/health/ready", readyHandler(checker))
	return router
}
