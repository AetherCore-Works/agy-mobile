//go:build windows

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConsoleHelpersWorkDirAndJobObject(t *testing.T) {
	customDir := t.TempDir()
	t.Setenv("AGY_WORKSPACE", customDir)
	if got := resolveConsoleWorkDir("fallback"); got != customDir {
		t.Fatalf("expected AGY_WORKSPACE=%q, got %q", customDir, got)
	}

	job, err := createKillOnCloseJob()
	if err != nil || job == 0 {
		t.Fatalf("createKillOnCloseJob failed: job=%v err=%v", job, err)
	}
	// Close handle cleanly
	sup := &consoleSupervisor{job: job}
	sup.closeJob()

	logFile := filepath.Join(customDir, "gateway.err.log")
	if err := os.WriteFile(logFile, []byte("linha1\nlinha2\nlinha3\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Smoke check showLogTail does not panic
	showLogTail(logFile, 2)
}
