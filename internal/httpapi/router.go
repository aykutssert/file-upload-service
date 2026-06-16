package httpapi

import (
	"net/http"

	"github.com/aykutssert/file-upload-service/internal/auth"
	"github.com/go-chi/chi/v5"
)

func NewRouter(
	checker readinessChecker,
	resolver auth.Resolver,
	uploads uploadCreator,
	presigner uploadPresigner,
) http.Handler {
	router := chi.NewRouter()
	router.Get("/health/live", liveHandler)
	router.Get("/health/ready", readyHandler(checker))
	if resolver != nil && uploads != nil && presigner != nil {
		router.Group(func(protected chi.Router) {
			protected.Use(auth.Middleware(resolver))
			protected.Use(auth.RequirePermission("file:create"))
			protected.Post("/v1/upload-sessions", createUploadHandler(uploads, presigner))
		})
	}
	return router
}
