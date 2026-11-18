//go:build !windows && !darwin

package automation

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
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

func escapeGVariantSingleQuoted(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `'`, `\'`)
	s = strings.ReplaceAll(s, "\r\n", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	return s
}

func buildLinuxDesktopEnv() []string {
	env := os.Environ()
	uid := strconv.Itoa(os.Getuid())

	xdgRuntime := strings.TrimSpace(os.Getenv("XDG_RUNTIME_DIR"))
	if xdgRuntime == "" {
		candidate := filepath.Join("/run/user", uid)
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			xdgRuntime = candidate
			env = append(env, "XDG_RUNTIME_DIR="+xdgRuntime)
		}
	}

	if strings.TrimSpace(os.Getenv("DBUS_SESSION_BUS_ADDRESS")) == "" && xdgRuntime != "" {
		busPath := filepath.Join(xdgRuntime, "bus")
		if _, err := os.Stat(busPath); err == nil {
			env = append(env, "DBUS_SESSION_BUS_ADDRESS=unix:path="+busPath)
		}
	}

	if strings.TrimSpace(os.Getenv("DISPLAY")) == "" && strings.TrimSpace(os.Getenv("WAYLAND_DISPLAY")) == "" {
		if xdgRuntime != "" {
			if _, err := os.Stat(filepath.Join(xdgRuntime, "wayland-0")); err == nil {
				env = append(env, "WAYLAND_DISPLAY=wayland-0")
			}
		}
		for _, disp := range []struct {
			sock string
			val  string
		}{
			{"/tmp/.X11-unix/X0", ":0"},
			{"/tmp/.X11-unix/X1", ":1"},
		} {
			if _, err := os.Stat(disp.sock); err == nil {
				env = append(env, "DISPLAY="+disp.val)
				break
			}
		}
	}

	if strings.TrimSpace(os.Getenv("XAUTHORITY")) == "" {
		if home, err := os.UserHomeDir(); err == nil && home != "" {
			xauth := filepath.Join(home, ".Xauthority")
			if _, err := os.Stat(xauth); err == nil {
				env = append(env, "XAUTHORITY="+xauth)
			}
		}
	}

	return env
}

// SendOSNotification shows a native Linux/BSD desktop notification via notify-send, gdbus, kdialog, or zenity.
func SendOSNotification(ctx context.Context, title, message, severity string) (bool, error) {
	t, m := cleanNotificationFields(title, message)
	sev := strings.ToLower(strings.TrimSpace(severity))

	urgency := "normal"
	icon := "dialog-information"
	urgencyByte := "1"
	switch sev {
	case "error":
		urgency = "critical"
		icon = "dialog-error"
		urgencyByte = "2"
	case "warning":
		urgency = "critical"
		icon = "dialog-warning"
		urgencyByte = "2"
	}

	cctx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()
	desktopEnv := buildLinuxDesktopEnv()

	runBackend := func(bin string, args ...string) error {
		cmd := exec.CommandContext(cctx, bin, args...)
		cmd.Env = desktopEnv
		out, err := cmd.CombinedOutput()
		if err != nil {
			detail := strings.TrimSpace(string(out))
			if detail != "" {
				return fmt.Errorf("%s: %w: %s", filepath.Base(bin), err, detail)
			}
			return fmt.Errorf("%s: %w", filepath.Base(bin), err)
		}
		return nil
	}

	var firstErr error
	if p, err := exec.LookPath("notify-send"); err == nil {
		if runErr := runBackend(p, "-a", "AGY Mobile Gateway", "-u", urgency, "-i", icon, "--", t, m); runErr == nil {
			return true, nil
		} else {
			firstErr = runErr
		}
	}

	if p, err := exec.LookPath("gdbus"); err == nil {
		runErr := runBackend(
			p, "call", "--session",
			"--dest", "org.freedesktop.Notifications",
			"--object-path", "/org/freedesktop/Notifications",
			"--method", "org.freedesktop.Notifications.Notify",
			"AGY Mobile Gateway",
			"0",
			icon,
			"'"+escapeGVariantSingleQuoted(t)+"'",
			"'"+escapeGVariantSingleQuoted(m)+"'",
			"[]",
			"{'urgency': <byte "+urgencyByte+">}",
			"6000",
		)
		if runErr == nil {
			return true, nil
		} else if firstErr == nil {
			firstErr = runErr
		}
	}

	if p, err := exec.LookPath("kdialog"); err == nil {
		if runErr := runBackend(p, "--title", t, "--passivepopup", m, "8"); runErr == nil {
			return true, nil
		} else if firstErr == nil {
			firstErr = runErr
		}
	}

	if p, err := exec.LookPath("zenity"); err == nil {
		if runErr := runBackend(p, "--notification", "--text="+t+": "+m); runErr == nil {
			return true, nil
		} else if firstErr == nil {
			firstErr = runErr
		}
	}

	if firstErr != nil {
		return false, firstErr
	}
	return false, errors.New("nenhum servidor de notificação desktop disponível (instale notify-send / libnotify)")
}
