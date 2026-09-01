package config

import (
	"os"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	clearConfigEnvironment(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Port != 8080 || cfg.DatabasePath != "./data/personal.db" || len(cfg.AllowedOrigins) != 1 {
		t.Fatalf("Load() config = %+v", cfg)
	}
}

func clearConfigEnvironment(t *testing.T) {
	t.Helper()
	keys := []string{"PORT", "DATABASE_PATH", "ALLOWED_ORIGINS"}
	values := make(map[string]string, len(keys))
	present := make(map[string]bool, len(keys))
	for _, key := range keys {
		values[key], present[key] = os.LookupEnv(key)
		if err := os.Unsetenv(key); err != nil {
			t.Fatalf("unset %s: %v", key, err)
		}
	}
	t.Cleanup(func() {
		for _, key := range keys {
			if present[key] {
				_ = os.Setenv(key, values[key])
			} else {
				_ = os.Unsetenv(key)
			}
		}
	})
}

func TestLoadRejectsInvalidConfiguration(t *testing.T) {
	tests := []struct {
		name  string
		key   string
		value string
	}{
		{"invalid port", "PORT", "65536"},
		{"empty database path", "DATABASE_PATH", " "},
		{"invalid origin", "ALLOWED_ORIGINS", "localhost:3000"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("PORT", "8080")
			t.Setenv("DATABASE_PATH", "./data/personal.db")
			t.Setenv("ALLOWED_ORIGINS", "http://localhost:3000")
			t.Setenv(tt.key, tt.value)
			if _, err := Load(); err == nil {
				t.Fatal("Load() error = nil")
			}
		})
	}
}
