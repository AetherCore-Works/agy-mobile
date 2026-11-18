//go:build windows

package automation

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const windowsToastScript = `
$aumid = "AGY.MobileGateway"
$regPath = "HKCU:\Software\Classes\AppUserModelId\$aumid"
if (-not (Test-Path $regPath)) {
  New-Item -Path $regPath -Force | Out-Null
  Set-ItemProperty -Path $regPath -Name "DisplayName" -Value "AGY Mobile Gateway" -Force
}
$notifPath = "HKCU:\Software\Microsoft\Windows\CurrentVersion\Notifications\Settings\$aumid"
if (-not (Test-Path $notifPath)) {
  New-Item -Path $notifPath -Force | Out-Null
  Set-ItemProperty -Path $notifPath -Name "ShowInActionCenter" -Value 1 -Type DWord -Force
  Set-ItemProperty -Path $notifPath -Name "Enabled" -Value 1 -Type DWord -Force
}
try {
  [Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType = WindowsRuntime] | Out-Null
  [Windows.Data.Xml.Dom.XmlDocument, Windows.Data.Xml.Dom.XmlDocument, ContentType = WindowsRuntime] | Out-Null
  $xml = [Windows.UI.Notifications.ToastNotificationManager]::GetTemplateContent([Windows.UI.Notifications.ToastTemplateType]::ToastText02)
  $nodes = $xml.GetElementsByTagName("text")
  $nodes.Item(0).AppendChild($xml.CreateTextNode($env:AGY_NOTIFY_TITLE)) | Out-Null
  $nodes.Item(1).AppendChild($xml.CreateTextNode($env:AGY_NOTIFY_BODY)) | Out-Null
  $toast = [Windows.UI.Notifications.ToastNotification]::new($xml)
  [Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier($aumid).Show($toast)
} catch {
  Add-Type -AssemblyName System.Windows.Forms, System.Drawing
  $ni = New-Object System.Windows.Forms.NotifyIcon
  $ni.Icon = [System.Drawing.SystemIcons]::Information
  $ni.BalloonTipIcon = [System.Windows.Forms.ToolTipIcon]::Info
  if ($env:AGY_NOTIFY_SEV -eq "error") {
    $ni.Icon = [System.Drawing.SystemIcons]::Error
    $ni.BalloonTipIcon = [System.Windows.Forms.ToolTipIcon]::Error
  } elseif ($env:AGY_NOTIFY_SEV -eq "warning") {
    $ni.Icon = [System.Drawing.SystemIcons]::Warning
    $ni.BalloonTipIcon = [System.Windows.Forms.ToolTipIcon]::Warning
  }
  $ni.BalloonTipTitle = $env:AGY_NOTIFY_TITLE
  $ni.BalloonTipText = $env:AGY_NOTIFY_BODY
  $ni.Visible = $true
  $ni.ShowBalloonTip(6000)
  Start-Sleep -Milliseconds 350
  $ni.Dispose()
}
`

func resolveWindowsPowerShell() string {
	if root := strings.TrimSpace(os.Getenv("SystemRoot")); root != "" {
		candidate := filepath.Join(root, "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return "powershell.exe"
}

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

// SendOSNotification shows a native Windows 10/11 Toast (or NotifyIcon balloon fallback) without console flash.
func SendOSNotification(ctx context.Context, title, message, severity string) (bool, error) {
	t, m := cleanNotificationFields(title, message)
	sev := strings.ToLower(strings.TrimSpace(severity))

	cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(cctx, resolveWindowsPowerShell(), "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", windowsToastScript)
	cmd.Env = append(os.Environ(),
		"AGY_NOTIFY_TITLE="+t,
		"AGY_NOTIFY_BODY="+m,
		"AGY_NOTIFY_SEV="+sev,
	)
	configureOSProcess(cmd)

	out, err := cmd.CombinedOutput()
	if err != nil {
		detail := strings.TrimSpace(string(out))
		if detail != "" {
			return false, fmt.Errorf("windows notify: %w: %s", err, detail)
		}
		return false, fmt.Errorf("windows notify: %w", err)
	}
	return true, nil
}
