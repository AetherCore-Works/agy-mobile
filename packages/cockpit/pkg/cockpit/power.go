package cockpit

import (
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

var (
	// ErrPowerDisabled indicates power control endpoints were disabled via configuration.
	ErrPowerDisabled = errors.New("ações de energia estão desabilitadas nesta instância")
	// ErrInvalidPowerConfirm indicates the action or confirmation phrase did not match.
	ErrInvalidPowerConfirm = errors.New("confirmação inválida para ação de energia")
)

var expectedPowerConfirm = map[string]string{
	"lock":    "BLOQUEAR",
	"suspend": "SUSPENDER",
	"reboot":  "REINICIAR",
}

// ExpectedConfirmWord returns the required confirmation string for a power action, or empty if unsupported.
func ExpectedConfirmWord(action string) string {
	return expectedPowerConfirm[strings.ToLower(strings.TrimSpace(action))]
}

// PowerController abstracts OS screen-lock, suspend, and reboot commands.
type PowerController interface {
	Execute(action string) error
}

// OSPowerController executes host power actions after a short HTTP response grace delay.
type OSPowerController struct {
	Delay time.Duration
}

// Execute schedules the OS power command asynchronously so the HTTP response finishes flushing.
func (p *OSPowerController) Execute(action string) error {
	norm := strings.ToLower(strings.TrimSpace(action))
	if ExpectedConfirmWord(norm) == "" {
		return fmt.Errorf("ação de energia não suportada: %s", action)
	}
	delay := p.Delay
	if delay <= 0 {
		delay = 500 * time.Millisecond
	}

	time.AfterFunc(delay, func() {
		var cmd *exec.Cmd
		switch runtime.GOOS {
		case "windows":
			switch norm {
			case "lock":
				cmd = exec.Command("rundll32.exe", "user32.dll,LockWorkStation")
			case "suspend":
				cmd = exec.Command(
					"powershell", "-NoProfile", "-NonInteractive", "-Command",
					"Add-Type -AssemblyName System.Windows.Forms; [System.Windows.Forms.Application]::SetSuspendState([System.Windows.Forms.PowerState]::Suspend, $false, $false)",
				)
			case "reboot":
				cmd = exec.Command("shutdown.exe", "/r", "/t", "2", "/c", "AGY Cockpit Reboot")
			}
		case "linux":
			switch norm {
			case "lock":
				if _, err := exec.LookPath("loginctl"); err == nil {
					cmd = exec.Command("loginctl", "lock-session")
				} else {
					cmd = exec.Command("xdg-screensaver", "lock")
				}
			case "suspend":
				cmd = exec.Command("systemctl", "suspend")
			case "reboot":
				cmd = exec.Command("systemctl", "reboot")
			}
		case "darwin":
			switch norm {
			case "lock":
				cmd = exec.Command("pmset", "displaysleepnow")
			case "suspend":
				cmd = exec.Command("pmset", "sleepnow")
			case "reboot":
				cmd = exec.Command("osascript", "-e", `tell application "System Events" to restart`)
			}
		}
		if cmd != nil {
			configureOSProcess(cmd)
			_ = cmd.Run()
		}
	})
	return nil
}
