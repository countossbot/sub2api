package setup

import (
	"testing"
)

// A3: connection-string parsing for the external Aiven PostgreSQL / Valkey.
func TestDatabaseURLConfig(t *testing.T) {
	tests := []struct {
		name     string
		raw      string
		wantNil  bool
		wantErr  bool
		host     string
		port     int
		user     string
		password string
		dbName   string
		sslMode  string
	}{
		{
			name:     "aiven style with sslmode=require",
			raw:      "postgresql://avnadmin:FAKE_PG_PLACEHOLDER@pg-EXAMPLE-host.aivencloud.com:10317/sub2api?sslmode=require",
			host:     "pg-EXAMPLE-host.aivencloud.com",
			port:     10317,
			user:     "avnadmin",
			password: "FAKE_PG_PLACEHOLDER",
			dbName:   "sub2api",
			sslMode:  "require",
		},
		{
			name:     "percent-encoded special characters in password",
			raw:      "postgresql://avn%40admin:p%40ss%2Fw%3Ard%23@db.example.com:5432/app?sslmode=verify-full",
			host:     "db.example.com",
			port:     5432,
			user:     "avn@admin",
			password: "p@ss/w:rd#",
			dbName:   "app",
			sslMode:  "verify-full",
		},
		{
			name:     "postgres scheme defaults",
			raw:      "postgres://u:p@localhost/app",
			host:     "localhost",
			port:     5432,
			user:     "u",
			password: "p",
			dbName:   "app",
			sslMode:  "disable",
		},
		{name: "empty is not configured", raw: "   ", wantNil: true},
		{name: "wrong scheme rejected", raw: "mysql://u:p@h:3306/db", wantErr: true},
		{name: "missing host rejected", raw: "postgresql:///db", wantErr: true},
		{name: "bad port rejected", raw: "postgresql://u:p@h:abc/db", wantErr: true},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got, ok, err := databaseURLConfig(tc.raw)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got cfg=%+v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.wantNil {
				if ok || got != nil {
					t.Fatalf("expected no config, got ok=%v cfg=%+v", ok, got)
				}
				return
			}
			if !ok || got == nil {
				t.Fatalf("expected config, got ok=%v cfg=%+v", ok, got)
			}
			if got.Host != tc.host || got.Port != tc.port || got.User != tc.user ||
				got.Password != tc.password || got.DBName != tc.dbName || got.SSLMode != tc.sslMode {
				t.Fatalf("unexpected config: %+v", got)
			}
		})
	}
}

func TestRedisURLConfig(t *testing.T) {
	tests := []struct {
		name      string
		raw       string
		wantNil   bool
		wantErr   bool
		host      string
		port      int
		username  string
		password  string
		db        int
		enableTLS bool
	}{
		{
			name:      "aiven rediss enables TLS",
			raw:       "rediss://default:FAKE_REDIS_PLACEHOLDER@valkey-EXAMPLE-host.aivencloud.com:10318",
			host:      "valkey-EXAMPLE-host.aivencloud.com",
			port:      10318,
			username:  "default",
			password:  "FAKE_REDIS_PLACEHOLDER",
			db:        0,
			enableTLS: true,
		},
		{
			name:      "redis scheme keeps TLS off and honors db index",
			raw:       "redis://:secret@cache.internal:6379/3",
			host:      "cache.internal",
			port:      6379,
			username:  "",
			password:  "secret",
			db:        3,
			enableTLS: false,
		},
		{name: "empty is not configured", raw: "", wantNil: true},
		{name: "wrong scheme rejected", raw: "http://host:6379", wantErr: true},
		{name: "missing host rejected", raw: "rediss://:pw@/0", wantErr: true},
		{name: "bad db index rejected", raw: "redis://h:6379/abc", wantErr: true},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got, ok, err := redisURLConfig(tc.raw)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got cfg=%+v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.wantNil {
				if ok || got != nil {
					t.Fatalf("expected no config, got ok=%v cfg=%+v", ok, got)
				}
				return
			}
			if !ok || got == nil {
				t.Fatalf("expected config, got ok=%v cfg=%+v", ok, got)
			}
			if got.Host != tc.host || got.Port != tc.port || got.Username != tc.username ||
				got.Password != tc.password || got.DB != tc.db || got.EnableTLS != tc.enableTLS {
				t.Fatalf("unexpected config: %+v", got)
			}
		})
	}
}

// A4: PORT fallback for Render, while SERVER_PORT keeps precedence.
func TestEnvIntOrDefaultWithFallback(t *testing.T) {
	tests := []struct {
		name     string
		server   string
		port     string
		setPort  bool
		expected int
	}{
		{name: "SERVER_PORT wins", server: "9000", port: "10000", setPort: true, expected: 9000},
		{name: "PORT used when SERVER_PORT unset", port: "10000", setPort: true, expected: 10000},
		{name: "default when neither set", expected: 8080},
		{name: "invalid PORT falls back to default", port: "not-a-number", setPort: true, expected: 8080},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("SERVER_PORT", tc.server)
			if tc.setPort {
				t.Setenv("PORT", tc.port)
			}
			if got := envIntOrDefaultWithFallback("SERVER_PORT", "PORT", 8080); got != tc.expected {
				t.Fatalf("got %d, want %d", got, tc.expected)
			}
		})
	}
}

// A3/A5: URLs overlay only the fields the fine-grained vars left at defaults,
// so docker-compose style deployments (DATABASE_HOST set) are untouched.
func TestApplyEnvConnectionStrings(t *testing.T) {
	t.Run("urls fill defaults", func(t *testing.T) {
		t.Setenv("DATABASE_URL", "postgresql://avnadmin:pw@pg.aivencloud.com:10317/sub2api?sslmode=require")
		t.Setenv("REDIS_URL", "rediss://default:pw@valkey.aivencloud.com:10318")

		cfg := &SetupConfig{
			Database: DatabaseConfig{Host: "localhost", Port: 5432},
			Redis:    RedisConfig{Host: "localhost", Port: 6379},
		}
		applyEnvConnectionStrings(cfg)

		if cfg.Database.Host != "pg.aivencloud.com" || cfg.Database.Port != 10317 ||
			cfg.Database.SSLMode != "require" || cfg.Database.DBName != "sub2api" {
			t.Fatalf("database not applied: %+v", cfg.Database)
		}
		if cfg.Redis.Host != "valkey.aivencloud.com" || cfg.Redis.Port != 10318 || !cfg.Redis.EnableTLS {
			t.Fatalf("redis not applied: %+v", cfg.Redis)
		}
	})

	t.Run("explicit fine-grained vars win", func(t *testing.T) {
		t.Setenv("DATABASE_HOST", "compose-postgres")
		t.Setenv("REDIS_HOST", "compose-redis")
		t.Setenv("DATABASE_URL", "postgresql://avnadmin:pw@pg.aivencloud.com:10317/sub2api?sslmode=require")
		t.Setenv("REDIS_URL", "rediss://default:pw@valkey.aivencloud.com:10318")

		cfg := &SetupConfig{
			Database: DatabaseConfig{Host: "compose-postgres", Port: 5432},
			Redis:    RedisConfig{Host: "compose-redis", Port: 6379},
		}
		applyEnvConnectionStrings(cfg)

		if cfg.Database.Host != "compose-postgres" {
			t.Fatalf("DATABASE_URL overrode explicit DATABASE_HOST: %+v", cfg.Database)
		}
		if cfg.Redis.Host != "compose-redis" {
			t.Fatalf("REDIS_URL overrode explicit REDIS_HOST: %+v", cfg.Redis)
		}
	})
}
