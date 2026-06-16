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
	keyCreator keyCreatorInterface,
	keyRevoker keyRevokerInterface,
) http.Handler {
	router := chi.NewRouter()
	router.Get("/health/live", liveHandler)
	router.Get("/health/ready", readyHandler(checker))
	if resolver != nil && uploads != nil && presigner != nil {
		router.Group(func(protected chi.Router) {
			protected.Use(auth.Middleware(resolver))
			protected.Group(func(write chi.Router) {
				write.Use(auth.RequirePermission("file:create"))
				write.Post("/v1/upload-sessions", createUploadHandler(uploads, presigner))
				write.Post("/v1/files/{id}/complete", completeUploadHandler(uploads, presigner))
			})
			protected.Group(func(read chi.Router) {
				read.Use(auth.RequirePermission("file:read"))
				read.Get("/v1/files", listUploadsHandler(uploads))
				read.Post("/v1/files/batch", batchLookupHandler(uploads))
				read.Get("/v1/files/{id}/download", downloadHandler(uploads, presigner))
			})
			protected.Group(func(del chi.Router) {
				del.Use(auth.RequirePermission("file:delete"))
				del.Delete("/v1/files/{id}", deleteUploadHandler(uploads))
			})
			if keyCreator != nil && keyRevoker != nil {
				protected.Post("/v1/keys", createKeyHandler(keyCreator))
				protected.Delete("/v1/keys/{id}", revokeKeyHandler(keyRevoker))
			}
		})
	}
	return router
}
