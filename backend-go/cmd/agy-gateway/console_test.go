package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConsoleHelpersCrossPlatform(t *testing.T) {
	customDir := t.TempDir()
	t.Setenv("AGY_WORKSPACE", customDir)
	if got := resolveConsoleWorkDir("fallback"); got != customDir {
		t.Fatalf("expected AGY_WORKSPACE=%q, got %q", customDir, got)
	}

	h := newSupervisorPlatformHandle()
	h.closeHandle()

	logFile := filepath.Join(customDir, "gateway.err.log")
	if err := os.WriteFile(logFile, []byte("linha1\nlinha2\nlinha3\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	showLogTail(logFile, 2)

	// LAN IP detection should never panic and exclude loopbacks
	for _, ip := range detectLANIPs() {
		if ip == "127.0.0.1" {
			t.Fatalf("loopback IP should be filtered from detectLANIPs")
		}
	}
}
