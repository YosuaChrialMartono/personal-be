package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/YosuaChrialMartono/personal-be/internal/wishlist"
)

type errorResponse struct {
	Error errorBody `json:"error"`
}

type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, errorResponse{Error: errorBody{Code: code, Message: message}})
}

func writeDomainError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, wishlist.ErrValidation):
		writeError(w, http.StatusUnprocessableEntity, "validation_failed", validationErrorMessage(err))
	case errors.Is(err, wishlist.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "wishlist item not found")
	case errors.Is(err, wishlist.ErrConflict):
		writeError(w, http.StatusConflict, "conflict", "wishlist order conflicts with current items")
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "an internal error occurred")
	}
}

func validationErrorMessage(err error) string {
	var validation wishlist.ValidationError
	if wishlist.AsValidationError(err, &validation) {
		return validation.Message
	}
	return "request validation failed"
}
