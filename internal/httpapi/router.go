package httpapi

import (
	"context"
	"database/sql"
	"log/slog"
	"net/http"
	"time"

	"github.com/YosuaChrialMartono/personal-be/internal/wishlist"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func NewRouter(db *sql.DB, service *wishlist.Service, logger *slog.Logger, allowedOrigins []string, auth *Auth) http.Handler {
	router := chi.NewRouter()
	router.Use(middleware.RequestID)
	router.Use(loggingMiddleware(logger))
	router.Use(recoveryMiddleware(logger))
	router.Use(corsMiddleware(allowedOrigins))
	router.Get("/healthz", healthHandler(db))
	router.Get("/auth/login", auth.start)
	router.Get("/auth/callback", auth.callback)
	router.Get("/auth/session", auth.session)
	router.Post("/auth/logout", auth.logout)
	handler := newWishlistHandler(service)
	router.Group(func(r chi.Router) {
		r.Use(auth.require)
		r.Get("/api/v1/wishlist/items", handler.list)
		r.Post("/api/v1/wishlist/items", handler.create)
		r.Put("/api/v1/wishlist/items/order", handler.reorder)
		r.Patch("/api/v1/wishlist/items/{id}", handler.update)
		r.Delete("/api/v1/wishlist/items/{id}", handler.delete)
	})
	return router
}

func healthHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := db.PingContext(ctx); err != nil {
			loggerFromContext(r.Context()).Error("health check failed", "error", err)
			writeError(w, http.StatusServiceUnavailable, "internal_error", "database is unavailable")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}
