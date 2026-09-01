package httpapi

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/YosuaChrialMartono/personal-be/internal/database"
	"github.com/YosuaChrialMartono/personal-be/internal/database/migrations"
	"github.com/YosuaChrialMartono/personal-be/internal/wishlist"
)

type testServer struct {
	server *httptest.Server
	db     *sql.DB
}

func newTestServer(t *testing.T) testServer {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "wishlist.db")
	if err := migrations.Apply(path, false); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	db, err := database.Open(context.Background(), path)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	repository := wishlist.NewSQLiteRepository(db)
	handler := NewRouter(db, wishlist.NewService(repository), slog.New(slog.NewTextHandler(io.Discard, nil)), []string{"http://localhost:3000"})
	server := httptest.NewServer(handler)
	t.Cleanup(func() {
		server.Close()
		_ = db.Close()
	})
	return testServer{server: server, db: db}
}

func TestWishlistCRUDAndOrdering(t *testing.T) {
	ts := newTestServer(t)

	status, body := requestJSON(t, ts.server.Client(), http.MethodGet, ts.server.URL+"/healthz", nil, "")
	if status != http.StatusOK || string(body) != "{\"status\":\"ok\"}\n" {
		t.Fatalf("health response = %d %s", status, body)
	}

	first := createItem(t, ts.server, map[string]any{
		"title": " Keyboard ", "description": " Desk ", "priceMinor": 1099, "currency": "USD",
	})
	if first.Title != "Keyboard" || first.Description != "Desk" || first.Position != 0 {
		t.Fatalf("first item = %+v", first)
	}
	second := createItem(t, ts.server, map[string]any{
		"title": "Monitor", "description": "", "priceMinor": 2500000, "currency": "IDR",
	})
	if second.Position != 1 {
		t.Fatalf("second position = %d", second.Position)
	}

	status, body = requestJSON(t, ts.server.Client(), http.MethodPatch, ts.server.URL+"/api/v1/wishlist/items/"+first.ID,
		map[string]any{"title": " Better Keyboard ", "priceMinor": 1299}, "")
	if status != http.StatusOK {
		t.Fatalf("update status = %d, body = %s", status, body)
	}
	var updated struct {
		Item wishlist.Item `json:"item"`
	}
	decode(t, body, &updated)
	if updated.Item.Title != "Better Keyboard" || updated.Item.PriceMinor != 1299 || updated.Item.Description != "Desk" {
		t.Fatalf("updated item = %+v", updated.Item)
	}
	if !updated.Item.UpdatedAt.After(updated.Item.CreatedAt) {
		t.Fatalf("updated timestamp did not advance: created=%s updated=%s", updated.Item.CreatedAt, updated.Item.UpdatedAt)
	}

	status, body = requestJSON(t, ts.server.Client(), http.MethodPut, ts.server.URL+"/api/v1/wishlist/items/order",
		map[string]any{"itemIds": []string{second.ID, first.ID}}, "")
	if status != http.StatusNoContent || len(body) != 0 {
		t.Fatalf("reorder response = %d %s", status, body)
	}

	items := listItems(t, ts.server)
	if len(items) != 2 || items[0].ID != second.ID || items[0].Position != 0 || items[1].ID != first.ID || items[1].Position != 1 {
		t.Fatalf("reordered items = %+v", items)
	}

	status, body = requestJSON(t, ts.server.Client(), http.MethodDelete, ts.server.URL+"/api/v1/wishlist/items/"+second.ID, nil, "")
	if status != http.StatusNoContent || len(body) != 0 {
		t.Fatalf("delete response = %d %s", status, body)
	}
	items = listItems(t, ts.server)
	if len(items) != 1 || items[0].ID != first.ID || items[0].Position != 0 {
		t.Fatalf("compacted items = %+v", items)
	}
}

func TestWishlistValidationAndReorderConflict(t *testing.T) {
	ts := newTestServer(t)
	item := createItem(t, ts.server, map[string]any{
		"title": "Keyboard", "priceMinor": 100, "currency": "USD",
	})

	tests := []struct {
		name   string
		method string
		path   string
		body   any
		status int
		code   string
	}{
		{"unknown field", http.MethodPost, "/api/v1/wishlist/items", map[string]any{"title": "x", "priceMinor": 1, "currency": "IDR", "extra": true}, http.StatusBadRequest, "invalid_request"},
		{"invalid values", http.MethodPost, "/api/v1/wishlist/items", map[string]any{"title": " ", "priceMinor": -1, "currency": "GBP"}, http.StatusUnprocessableEntity, "validation_failed"},
		{"empty patch", http.MethodPatch, "/api/v1/wishlist/items/" + item.ID, map[string]any{}, http.StatusUnprocessableEntity, "validation_failed"},
		{"invalid id", http.MethodDelete, "/api/v1/wishlist/items/not-a-uuid", nil, http.StatusBadRequest, "invalid_request"},
		{"missing item", http.MethodDelete, "/api/v1/wishlist/items/00000000-0000-0000-0000-000000000000", nil, http.StatusNotFound, "not_found"},
		{"duplicate reorder ids", http.MethodPut, "/api/v1/wishlist/items/order", map[string]any{"itemIds": []string{item.ID, item.ID}}, http.StatusUnprocessableEntity, "validation_failed"},
		{"conflicting reorder ids", http.MethodPut, "/api/v1/wishlist/items/order", map[string]any{"itemIds": []string{"00000000-0000-0000-0000-000000000000"}}, http.StatusConflict, "conflict"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, body := requestJSON(t, ts.server.Client(), tt.method, ts.server.URL+tt.path, tt.body, "")
			if status != tt.status {
				t.Fatalf("status = %d, body = %s", status, body)
			}
			var response struct {
				Error struct {
					Code string `json:"code"`
				} `json:"error"`
			}
			decode(t, body, &response)
			if response.Error.Code != tt.code {
				t.Fatalf("error code = %q, body = %s", response.Error.Code, body)
			}
		})
	}

	items := listItems(t, ts.server)
	if len(items) != 1 || items[0].ID != item.ID || items[0].Position != 0 {
		t.Fatalf("items changed after failed reorder = %+v", items)
	}
}

func TestMigrationsCanBeReappliedAndRolledBack(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "wishlist.db")
	if err := migrations.Apply(path, false); err != nil {
		t.Fatalf("apply migration: %v", err)
	}
	if err := migrations.Apply(path, false); err != nil {
		t.Fatalf("reapply migration: %v", err)
	}
	if err := migrations.Apply(path, true); err != nil {
		t.Fatalf("rollback migration: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("database file missing after rollback: %v", err)
	}
}

func createItem(t *testing.T, server *httptest.Server, payload map[string]any) wishlist.Item {
	t.Helper()
	status, body := requestJSON(t, server.Client(), http.MethodPost, server.URL+"/api/v1/wishlist/items", payload, "")
	if status != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", status, body)
	}
	var response struct {
		Item wishlist.Item `json:"item"`
	}
	decode(t, body, &response)
	return response.Item
}

func listItems(t *testing.T, server *httptest.Server) []wishlist.Item {
	t.Helper()
	status, body := requestJSON(t, server.Client(), http.MethodGet, server.URL+"/api/v1/wishlist/items", nil, "")
	if status != http.StatusOK {
		t.Fatalf("list status = %d, body = %s", status, body)
	}
	var response struct {
		Items []wishlist.Item `json:"items"`
	}
	decode(t, body, &response)
	return response.Items
}

func requestJSON(t *testing.T, client *http.Client, method, url string, payload any, origin string) (int, []byte) {
	t.Helper()
	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		body = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(context.Background(), method, url, body)
	if err != nil {
		t.Fatal(err)
	}
	if payload != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if origin != "" {
		request.Header.Set("Origin", origin)
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	result, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return response.StatusCode, result
}

func decode(t *testing.T, body []byte, target any) {
	t.Helper()
	if err := json.Unmarshal(body, target); err != nil {
		t.Fatalf("decode %s: %v", body, err)
	}
}
