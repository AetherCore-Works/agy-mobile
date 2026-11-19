package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadReadsDotEnvFromDataDir(t *testing.T) {
	tmp := t.TempDir()
	envFile := filepath.Join(tmp, ".env")
	content := "ENABLE_TERMINAL=true\nENABLE_TELEMETRY=true\nENABLE_COCKPIT=true\nPORT=19088\n"
	if err := os.WriteFile(envFile, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DATA_DIR", tmp)

	cfg := Load()
	if cfg.Port != "19088" {
		t.Fatalf("expected PORT=19088 from DataDir .env, got %q", cfg.Port)
	}
	if cfg.Mods["terminal"] != true || cfg.Mods["telemetry"] != true || cfg.Mods["cockpit"] != true {
		t.Fatalf("expected terminal/telemetry/cockpit enabled from DataDir .env, got %+v", cfg.Mods)
	}
}
