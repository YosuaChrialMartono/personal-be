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

func NewRouter(db *sql.DB, service *wishlist.Service, logger *slog.Logger, allowedOrigins []string) http.Handler {
	router := chi.NewRouter()
	router.Use(middleware.RequestID)
	router.Use(loggingMiddleware(logger))
	router.Use(recoveryMiddleware(logger))
	router.Use(corsMiddleware(allowedOrigins))
	router.Get("/healthz", healthHandler(db))
	handler := newWishlistHandler(service)
	router.Get("/api/v1/wishlist/items", handler.list)
	router.Post("/api/v1/wishlist/items", handler.create)
	router.Put("/api/v1/wishlist/items/order", handler.reorder)
	router.Patch("/api/v1/wishlist/items/{id}", handler.update)
	router.Delete("/api/v1/wishlist/items/{id}", handler.delete)
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
