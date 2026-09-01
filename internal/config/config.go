package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
)

const (
	defaultPort          = 8080
	defaultDatabasePath  = "./data/personal.db"
	defaultAllowedOrigin = "http://localhost:3000"
	maxPort              = 65535
)

type Config struct {
	Port           int
	DatabasePath   string
	AllowedOrigins []string
}

func Load() (Config, error) {
	port, err := parsePort(getenv("PORT", strconv.Itoa(defaultPort)))
	if err != nil {
		return Config{}, err
	}
	databasePath := getenv("DATABASE_PATH", defaultDatabasePath)
	if strings.TrimSpace(databasePath) == "" {
		return Config{}, fmt.Errorf("DATABASE_PATH must not be empty")
	}
	origins, err := parseOrigins(getenv("ALLOWED_ORIGINS", defaultAllowedOrigin))
	if err != nil {
		return Config{}, err
	}
	return Config{Port: port, DatabasePath: databasePath, AllowedOrigins: origins}, nil
}

func parsePort(value string) (int, error) {
	port, err := strconv.Atoi(value)
	if err != nil || port < 1 || port > maxPort {
		return 0, fmt.Errorf("PORT must be an integer between 1 and %d", maxPort)
	}
	return port, nil
}

func parseOrigins(value string) ([]string, error) {
	parts := strings.Split(value, ",")
	if len(parts) == 0 {
		return nil, fmt.Errorf("ALLOWED_ORIGINS must contain at least one origin")
	}
	origins := make([]string, 0, len(parts))
	for _, part := range parts {
		origin := strings.TrimSpace(part)
		parsed, err := url.Parse(origin)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
			return nil, fmt.Errorf("ALLOWED_ORIGINS contains invalid origin %q", origin)
		}
		if parsed.Scheme != "http" && parsed.Scheme != "https" {
			return nil, fmt.Errorf("ALLOWED_ORIGINS contains invalid origin %q", origin)
		}
		origins = append(origins, origin)
	}
	return origins, nil
}

func getenv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}
