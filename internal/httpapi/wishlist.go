package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/YosuaChrialMartono/personal-be/internal/wishlist"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

const maxJSONBody = 1 << 20

type wishlistHandler struct {
	service *wishlist.Service
}

func newWishlistHandler(service *wishlist.Service) *wishlistHandler {
	return &wishlistHandler{service: service}
}

type createRequest struct {
	Title       json.RawMessage `json:"title"`
	Description json.RawMessage `json:"description"`
	PriceMinor  json.RawMessage `json:"priceMinor"`
	Currency    json.RawMessage `json:"currency"`
}

type patchRequest struct {
	Title       json.RawMessage `json:"title"`
	Description json.RawMessage `json:"description"`
	PriceMinor  json.RawMessage `json:"priceMinor"`
	Currency    json.RawMessage `json:"currency"`
}

type orderRequest struct {
	ItemIDs []string `json:"itemIds"`
}

func (h *wishlistHandler) list(w http.ResponseWriter, r *http.Request) {
	items, err := h.service.List(r.Context())
	if err != nil {
		loggerFromContext(r.Context()).Error("list wishlist items failed", "error", err)
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *wishlistHandler) create(w http.ResponseWriter, r *http.Request) {
	var request createRequest
	if _, err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	input, err := createInput(request)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "validation_failed", err.Error())
		return
	}
	item, err := h.service.Create(r.Context(), input)
	if err != nil {
		loggerFromContext(r.Context()).Error("create wishlist item failed", "error", err)
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"item": item})
}

func (h *wishlistHandler) update(w http.ResponseWriter, r *http.Request) {
	id, ok := validID(w, r)
	if !ok {
		return
	}
	var request patchRequest
	data, err := decodeJSON(w, r, &request)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	var body map[string]json.RawMessage
	if err := json.Unmarshal(data, &body); err != nil || body == nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "request body must be a JSON object")
		return
	}
	input, err := patchInput(body)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "validation_failed", err.Error())
		return
	}
	item, err := h.service.Update(r.Context(), id, input)
	if err != nil {
		loggerFromContext(r.Context()).Error("update wishlist item failed", "error", err)
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"item": item})
}

func (h *wishlistHandler) delete(w http.ResponseWriter, r *http.Request) {
	id, ok := validID(w, r)
	if !ok {
		return
	}
	if err := h.service.Delete(r.Context(), id); err != nil {
		loggerFromContext(r.Context()).Error("delete wishlist item failed", "error", err)
		writeDomainError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *wishlistHandler) reorder(w http.ResponseWriter, r *http.Request) {
	var request orderRequest
	if _, err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if request.ItemIDs == nil {
		writeError(w, http.StatusUnprocessableEntity, "validation_failed", "itemIds is required")
		return
	}
	if err := h.service.Reorder(r.Context(), request.ItemIDs); err != nil {
		loggerFromContext(r.Context()).Error("reorder wishlist items failed", "error", err)
		writeDomainError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func validID(w http.ResponseWriter, r *http.Request) (string, bool) {
	id := chi.URLParam(r, "id")
	if _, err := uuid.Parse(id); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "id must be a valid UUID")
		return "", false
	}
	return id, true
}

func decodeJSON(w http.ResponseWriter, r *http.Request, target any) ([]byte, error) {
	r.Body = http.MaxBytesReader(w, r.Body, maxJSONBody)
	data, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, errors.New("request body is invalid or too large")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return nil, errors.New("request body contains invalid JSON")
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, errors.New("request body must contain one JSON value")
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(data, &object); err != nil || object == nil {
		return nil, errors.New("request body must be a JSON object")
	}
	return data, nil
}

func createInput(request createRequest) (wishlist.CreateInput, error) {
	title, err := parseString(request.Title, "title")
	if err != nil {
		return wishlist.CreateInput{}, err
	}
	description, err := parseStringOrEmpty(request.Description, "description")
	if err != nil {
		return wishlist.CreateInput{}, err
	}
	price, err := parsePrice(request.PriceMinor)
	if err != nil {
		return wishlist.CreateInput{}, err
	}
	currency, err := parseString(request.Currency, "currency")
	if err != nil {
		return wishlist.CreateInput{}, err
	}
	return wishlist.CreateInput{Title: title, Description: description, PriceMinor: price, Currency: currency}, nil
}

func patchInput(body map[string]json.RawMessage) (wishlist.UpdateInput, error) {
	var input wishlist.UpdateInput
	if raw, ok := body["title"]; ok {
		value, err := parseString(raw, "title")
		if err != nil {
			return input, err
		}
		input.Title = &value
	}
	if raw, ok := body["description"]; ok {
		value, err := parseString(raw, "description")
		if err != nil {
			return input, err
		}
		input.Description = &value
	}
	if raw, ok := body["priceMinor"]; ok {
		value, err := parsePrice(raw)
		if err != nil {
			return input, err
		}
		input.PriceMinor = &value
	}
	if raw, ok := body["currency"]; ok {
		value, err := parseString(raw, "currency")
		if err != nil {
			return input, err
		}
		input.Currency = &value
	}
	return input, nil
}

func parseString(raw json.RawMessage, field string) (string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return "", errors.New(field + " must be a string")
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", errors.New(field + " must be a string")
	}
	return value, nil
}

func parseStringOrEmpty(raw json.RawMessage, field string) (string, error) {
	if len(raw) == 0 {
		return "", nil
	}
	return parseString(raw, field)
}

func parsePrice(raw json.RawMessage) (int64, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return 0, errors.New("priceMinor must be an integer")
	}
	value, err := strconv.ParseInt(strings.TrimSpace(string(raw)), 10, 64)
	if err != nil || value < 0 || value > wishlist.MaxPriceMinor {
		return 0, errors.New("priceMinor must be a non-negative safe integer")
	}
	return value, nil
}
