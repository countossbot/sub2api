package setup

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
)

// This file adds support for configuring external PostgreSQL / Redis through a
// single connection-string environment variable (DATABASE_URL / REDIS_URL).
// It is required for deployments on platforms without a persistent disk and
// without a managed database (e.g. Render free containers backed by Aiven).
//
// Precedence: the fine-grained variables (DATABASE_HOST, REDIS_HOST, ...) win
// when explicitly set, so existing docker-compose deployments are unaffected;
// the connection string is used as the base value otherwise.

// databaseURLEnv / redisURLEnv are the accepted connection-string variables.
const (
	databaseURLEnv = "DATABASE_URL"
	redisURLEnv    = "REDIS_URL"
	dbURLEnv       = "DB_URL" // common alias
)

// databaseURLConfig parses a postgresql:// (or postgres://) connection string
// into the fine-grained database configuration. Recognized query parameters:
// sslmode. Returns (nil, false) when the variable is empty.
func databaseURLConfig(raw string) (*DatabaseConfig, bool, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, false, nil
	}

	u, err := url.Parse(raw)
	if err != nil {
		return nil, true, fmt.Errorf("invalid %s: %w", databaseURLEnv, err)
	}
	switch strings.ToLower(u.Scheme) {
	case "postgres", "postgresql":
	default:
		return nil, true, fmt.Errorf("invalid %s: unsupported scheme %q (want postgresql://)", databaseURLEnv, u.Scheme)
	}

	host := u.Hostname()
	if host == "" {
		return nil, true, fmt.Errorf("invalid %s: missing host", databaseURLEnv)
	}

	port := 5432
	if p := u.Port(); p != "" {
		parsed, err := strconv.Atoi(p)
		if err != nil || parsed <= 0 || parsed > 65535 {
			return nil, true, fmt.Errorf("invalid %s: bad port %q", databaseURLEnv, p)
		}
		port = parsed
	}

	password := ""
	if u.User != nil {
		password, _ = u.User.Password()
	}

	dbName := strings.TrimPrefix(u.Path, "/")
	if dbName == "" {
		dbName = "sub2api"
	}

	sslMode := "disable"
	if q := u.Query().Get("sslmode"); q != "" {
		sslMode = q
	}

	return &DatabaseConfig{
		Host:     host,
		Port:     port,
		User:     u.User.Username(),
		Password: password,
		DBName:   dbName,
		SSLMode:  sslMode,
	}, true, nil
}

// redisURLConfig parses a redis:// or rediss:// connection string into the
// fine-grained Redis configuration. rediss:// implies EnableTLS, and the path
// segment (/0) selects the logical database.
func redisURLConfig(raw string) (*RedisConfig, bool, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, false, nil
	}

	u, err := url.Parse(raw)
	if err != nil {
		return nil, true, fmt.Errorf("invalid %s: %w", redisURLEnv, err)
	}
	if !strings.EqualFold(u.Scheme, "redis") && !strings.EqualFold(u.Scheme, "rediss") {
		return nil, true, fmt.Errorf("invalid %s: unsupported scheme %q (want redis:// or rediss://)", redisURLEnv, u.Scheme)
	}

	host := u.Hostname()
	if host == "" {
		return nil, true, fmt.Errorf("invalid %s: missing host", redisURLEnv)
	}

	port := 6379
	if p := u.Port(); p != "" {
		parsed, err := strconv.Atoi(p)
		if err != nil || parsed <= 0 || parsed > 65535 {
			return nil, true, fmt.Errorf("invalid %s: bad port %q", redisURLEnv, p)
		}
		port = parsed
	}

	password := ""
	if u.User != nil {
		password, _ = u.User.Password()
	}

	db := 0
	if path := strings.Trim(u.Path, "/"); path != "" {
		parsed, err := strconv.Atoi(path)
		if err != nil || parsed < 0 {
			return nil, true, fmt.Errorf("invalid %s: bad database index %q", redisURLEnv, path)
		}
		db = parsed
	}

	return &RedisConfig{
		Host:      host,
		Port:      port,
		Username:  u.User.Username(),
		Password:  password,
		DB:        db,
		EnableTLS: strings.EqualFold(u.Scheme, "rediss"),
	}, true, nil
}

// firstNonEmptyEnv returns the value of the first environment variable that is
// set to a non-blank value.
func firstNonEmptyEnv(keys ...string) string {
	for _, key := range keys {
		if v := strings.TrimSpace(os.Getenv(key)); v != "" {
			return v
		}
	}
	return ""
}

// envIntOrDefaultWithFallback returns the integer value of the primary
// environment variable when set, otherwise the fallback variable, otherwise
// def. Used so SERVER_PORT keeps working while PORT (injected by Render and
// other Heroku-like platforms) is honored when SERVER_PORT is absent.
func envIntOrDefaultWithFallback(primary, fallback string, def int) int {
	if raw := strings.TrimSpace(os.Getenv(primary)); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil && v > 0 {
			return v
		}
	}
	if raw := strings.TrimSpace(os.Getenv(fallback)); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil && v > 0 {
			return v
		}
	}
	return def
}

// applyEnvConnectionStrings overlays DATABASE_URL / REDIS_URL onto cfg. A
// fine-grained environment variable that was explicitly set takes precedence,
// so we only replace fields that still hold their builder defaults.
func applyEnvConnectionStrings(cfg *SetupConfig) {
	if dbURL := firstNonEmptyEnv(databaseURLEnv, dbURLEnv); dbURL != "" {
		if parsed, ok, err := databaseURLConfig(dbURL); err == nil && ok {
			if getEnvOrDefault("DATABASE_HOST", "localhost") == "localhost" {
				cfg.Database.Host = parsed.Host
				cfg.Database.Port = parsed.Port
				cfg.Database.User = parsed.User
				cfg.Database.Password = parsed.Password
				cfg.Database.DBName = parsed.DBName
				cfg.Database.SSLMode = parsed.SSLMode
			}
		}
	}

	if redisURL := firstNonEmptyEnv(redisURLEnv); redisURL != "" {
		if parsed, ok, err := redisURLConfig(redisURL); err == nil && ok {
			if getEnvOrDefault("REDIS_HOST", "localhost") == "localhost" {
				cfg.Redis.Host = parsed.Host
				cfg.Redis.Port = parsed.Port
				cfg.Redis.Username = parsed.Username
				cfg.Redis.Password = parsed.Password
				cfg.Redis.DB = parsed.DB
				cfg.Redis.EnableTLS = parsed.EnableTLS
			}
		}
	}
}
