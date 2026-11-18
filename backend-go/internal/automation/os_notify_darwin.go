//go:build darwin

package automation

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

func cleanNotificationFields(title, message string) (string, string) {
	t := strings.TrimSpace(strings.ReplaceAll(title, "\x00", ""))
	if t == "" {
		t = "AGY Mobile Gateway"
	}
	m := strings.TrimSpace(strings.ReplaceAll(message, "\x00", ""))
	if m == "" {
		m = t
	}
	if len([]rune(m)) > 240 {
		m = string([]rune(m)[:240]) + "..."
	}
	return t, m
}

// SendOSNotification shows a native macOS Notification Center alert via osascript or terminal-notifier.
func SendOSNotification(ctx context.Context, title, message, _ string) (bool, error) {
	t, m := cleanNotificationFields(title, message)

	cctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	var firstErr error
	if osaPath, err := exec.LookPath("osascript"); err == nil {
		cmd := exec.CommandContext(
			cctx,
			osaPath,
			"-e", "on run argv",
			"-e", `display notification (item 2 of argv) with title (item 1 of argv) sound name "default"`,
			"-e", "end run",
			t,
			m,
		)
		out, runErr := cmd.CombinedOutput()
		if runErr == nil {
			return true, nil
		}
		detail := strings.TrimSpace(string(out))
		if detail != "" {
			firstErr = fmt.Errorf("osascript: %w: %s", runErr, detail)
		} else {
			firstErr = fmt.Errorf("osascript: %w", runErr)
		}
	}

	if tnPath, err := exec.LookPath("terminal-notifier"); err == nil {
		cmd := exec.CommandContext(
			cctx,
			tnPath,
			"-title", t,
			"-message", m,
			"-group", "agy-mobile-gateway",
			"-sound", "default",
		)
		out, runErr := cmd.CombinedOutput()
		if runErr == nil {
			return true, nil
		}
		detail := strings.TrimSpace(string(out))
		if detail != "" {
			firstErr = fmt.Errorf("terminal-notifier: %w: %s", runErr, detail)
		} else {
			firstErr = fmt.Errorf("terminal-notifier: %w", runErr)
		}
	}

	if firstErr != nil {
		return false, firstErr
	}
	return false, errors.New("osascript/terminal-notifier não encontrado no macOS")
}
