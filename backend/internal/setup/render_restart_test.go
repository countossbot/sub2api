package setup

import (
	"os"
	"testing"
)

// A5: Render free containers have no persistent disk, so DATA_DIR is wiped on
// every redeploy. A restart must be able to rebuild config.yaml from env vars
// alone and must not fail because an admin already exists in the external DB.
func TestEnvOnlyRestartIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DATA_DIR", dir)
	t.Setenv("AUTO_SETUP", "true")
	t.Setenv("DATABASE_URL", "postgresql://avnadmin:pw@pg.aivencloud.com:10317/sub2api?sslmode=require")
	t.Setenv("REDIS_URL", "rediss://default:pw@valkey.aivencloud.com:10318")

	if GetDataDir() != dir {
		t.Fatalf("GetDataDir() = %q, want %q", GetDataDir(), dir)
	}

	// Cold start on a wiped disk: no config.yaml, no install lock.
	if !NeedsSetup() {
		t.Fatalf("expected NeedsSetup()=true on empty DATA_DIR")
	}

	cfg := &SetupConfig{
		Server:   ServerConfig{Host: "0.0.0.0", Port: envIntOrDefaultWithFallback("SERVER_PORT", "PORT", 8080)},
		Database: DatabaseConfig{Host: "localhost", Port: 5432},
		Redis:    RedisConfig{Host: "localhost", Port: 6379},
		JWT:      JWTConfig{Secret: "test-secret", ExpireHour: 24},
		Timezone: "UTC",
	}
	applyEnvConnectionStrings(cfg)

	// Repeat the config-write step twice: it must succeed both times and the
	// resulting file must be identical (no append/duplicate corruption).
	for i := 0; i < 2; i++ {
		if err := writeConfigFile(cfg); err != nil {
			t.Fatalf("writeConfigFile run %d failed: %v", i+1, err)
		}
	}

	first, err := os.ReadFile(GetConfigFilePath())
	if err != nil {
		t.Fatalf("config.yaml not readable: %v", err)
	}
	if len(first) == 0 {
		t.Fatalf("config.yaml is empty")
	}
	if err := writeConfigFile(cfg); err != nil {
		t.Fatalf("second writeConfigFile failed: %v", err)
	}
	second, err := os.ReadFile(GetConfigFilePath())
	if err != nil {
		t.Fatalf("config.yaml not readable after rewrite: %v", err)
	}
	if string(first) != string(second) {
		t.Fatalf("config.yaml is not stable across restarts")
	}

	// Once the config exists, a second process must not re-enter setup.
	if NeedsSetup() {
		t.Fatalf("expected NeedsSetup()=false after config.yaml exists")
	}

	// The external DB already holds the admin: bootstrap must be a no-op, not
	// an error.
	if d := decideAdminBootstrap(10, 1); d.shouldCreate {
		t.Fatalf("admin bootstrap should be skipped when an admin exists (reason=%s)", d.reason)
	}

	// Simulate a fresh deploy (disk wiped, DB retains data).
	if err := os.Remove(GetConfigFilePath()); err != nil {
		t.Fatalf("failed to simulate wiped disk: %v", err)
	}
	if !NeedsSetup() {
		t.Fatalf("expected NeedsSetup()=true after disk wipe")
	}
	if err := writeConfigFile(cfg); err != nil {
		t.Fatalf("config regeneration after wipe failed: %v", err)
	}
	if _, err := os.Stat(GetConfigFilePath()); err != nil {
		t.Fatalf("config.yaml was not regenerated: %v", err)
	}
}
