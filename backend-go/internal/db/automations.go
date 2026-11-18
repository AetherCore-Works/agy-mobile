package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// Automation defines a SQLite-persisted scheduled Cron task (AI prompt or native check).
type Automation struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	CronExpr       string `json:"cron_expr"`
	Kind           string `json:"kind"` // "prompt" | "check"
	Prompt         string `json:"prompt"`
	Model          string `json:"model"`
	PermissionMode string `json:"permission_mode"`
	NotifyNtfy     bool   `json:"notify_ntfy"`
	NotifyPush     bool   `json:"notify_push"`
	NotifyOn       string `json:"notify_on"` // "always" | "error_only"
	Enabled        bool   `json:"enabled"`
	LastRunAt      string `json:"last_run_at"`
	LastStatus     string `json:"last_status"`
	LastSummary    string `json:"last_summary"`
	LastJobID      string `json:"last_job_id"`
	LastConvID     string `json:"last_conv_id"`
	NextRunAt      string `json:"next_run_at"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
}

// AlertConfig stores user-configurable active alert rules and delivery endpoints.
type AlertConfig struct {
	NotifyJobDone     bool   `json:"notify_job_done"`
	NotifyLongJobSec  int    `json:"notify_long_job_sec"`
	NotifyDockerDown  bool   `json:"notify_docker_down"`
	GPUTempThresholdC int    `json:"gpu_temp_threshold_c"`
	DiskFreeMinPct    int    `json:"disk_free_min_pct"`
	NtfyTopic         string `json:"ntfy_topic"`
	NtfyURL           string `json:"ntfy_url"`
	WebPushEnabled    bool   `json:"web_push_enabled"`
	NotifyDesktop     bool   `json:"notify_desktop"`
}

// AlertEvent represents a persisted notification sent by Cron or active hardware/job watchers.
type AlertEvent struct {
	ID        int64  `json:"id"`
	Source    string `json:"source"`
	Severity  string `json:"severity"` // "info" | "ok" | "warning" | "error"
	Title     string `json:"title"`
	Message   string `json:"message"`
	ConvID    string `json:"conv_id,omitempty"`
	JobID     string `json:"job_id,omitempty"`
	Read      bool   `json:"read"`
	CreatedAt string `json:"created_at"`
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

// DefaultAlertConfig returns safe initial alert rules seeded with env fallbacks when present.
func DefaultAlertConfig() AlertConfig {
	ntfyURL := strings.TrimRight(os.Getenv("NTFY_URL"), "/")
	if ntfyURL == "" {
		ntfyURL = "https://ntfy.sh"
	}
	return AlertConfig{
		NotifyJobDone:     true,
		NotifyLongJobSec:  45,
		NotifyDockerDown:  true,
		GPUTempThresholdC: 82,
		DiskFreeMinPct:    10,
		NtfyTopic:         strings.TrimSpace(os.Getenv("NTFY_TOPIC")),
		NtfyURL:           ntfyURL,
		WebPushEnabled:    true,
		NotifyDesktop:     true,
	}
}

func scanAutomation(scanner interface{ Scan(...any) error }) (*Automation, error) {
	var a Automation
	var notifyNtfy, notifyPush, enabled int
	err := scanner.Scan(
		&a.ID, &a.Name, &a.CronExpr, &a.Kind, &a.Prompt, &a.Model, &a.PermissionMode,
		&notifyNtfy, &notifyPush, &a.NotifyOn, &enabled,
		&a.LastRunAt, &a.LastStatus, &a.LastSummary, &a.LastJobID, &a.LastConvID, &a.NextRunAt,
		&a.CreatedAt, &a.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	a.NotifyNtfy = notifyNtfy != 0
	a.NotifyPush = notifyPush != 0
	a.Enabled = enabled != 0
	return &a, nil
}

// CreateAutomation persists a new scheduled Cron task in SQLite.
func (g *GatewayDB) CreateAutomation(a Automation) (*Automation, error) {
	now := utcNow()
	if a.CreatedAt == "" {
		a.CreatedAt = now
	}
	a.UpdatedAt = now

	g.mu.Lock()
	defer g.mu.Unlock()

	_, err := g.sql.Exec(
		`INSERT INTO automations (
			id, name, cron_expr, kind, prompt, model, permission_mode,
			notify_ntfy, notify_push, notify_on, enabled,
			last_run_at, last_status, last_summary, last_job_id, last_conv_id, next_run_at,
			created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.ID, a.Name, a.CronExpr, a.Kind, a.Prompt, a.Model, a.PermissionMode,
		boolToInt(a.NotifyNtfy), boolToInt(a.NotifyPush), a.NotifyOn, boolToInt(a.Enabled),
		a.LastRunAt, a.LastStatus, a.LastSummary, a.LastJobID, a.LastConvID, a.NextRunAt,
		a.CreatedAt, a.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	out := a
	return &out, nil
}

// ListAutomations returns all scheduled Cron tasks ordered newest first.
func (g *GatewayDB) ListAutomations() ([]Automation, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	rows, err := g.sql.Query(
		`SELECT id, name, cron_expr, kind, prompt, model, permission_mode,
		        notify_ntfy, notify_push, notify_on, enabled,
		        last_run_at, last_status, last_summary, last_job_id, last_conv_id, next_run_at,
		        created_at, updated_at
		 FROM automations
		 ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Automation{}
	for rows.Next() {
		a, err := scanAutomation(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *a)
	}
	return out, rows.Err()
}

// GetAutomation fetches a single scheduled task by ID or returns nil if missing.
func (g *GatewayDB) GetAutomation(id string) (*Automation, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	row := g.sql.QueryRow(
		`SELECT id, name, cron_expr, kind, prompt, model, permission_mode,
		        notify_ntfy, notify_push, notify_on, enabled,
		        last_run_at, last_status, last_summary, last_job_id, last_conv_id, next_run_at,
		        created_at, updated_at
		 FROM automations WHERE id = ?`,
		id,
	)
	a, err := scanAutomation(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return a, err
}

// UpdateAutomation updates configuration fields for an existing scheduled task.
func (g *GatewayDB) UpdateAutomation(a Automation) (*Automation, error) {
	a.UpdatedAt = utcNow()

	g.mu.Lock()
	defer g.mu.Unlock()

	res, err := g.sql.Exec(
		`UPDATE automations
		 SET name = ?, cron_expr = ?, kind = ?, prompt = ?, model = ?, permission_mode = ?,
		     notify_ntfy = ?, notify_push = ?, notify_on = ?, enabled = ?, next_run_at = ?, updated_at = ?
		 WHERE id = ?`,
		a.Name, a.CronExpr, a.Kind, a.Prompt, a.Model, a.PermissionMode,
		boolToInt(a.NotifyNtfy), boolToInt(a.NotifyPush), a.NotifyOn, boolToInt(a.Enabled), a.NextRunAt, a.UpdatedAt, a.ID,
	)
	if err != nil {
		return nil, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return nil, nil
	}
	out := a
	return &out, nil
}

// UpdateAutomationRunState persists execution outcome and next trigger timestamp for a Cron task.
func (g *GatewayDB) UpdateAutomationRunState(id, lastRunAt, lastStatus, lastSummary, lastJobID, lastConvID, nextRunAt string) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	_, err := g.sql.Exec(
		`UPDATE automations
		 SET last_run_at = ?, last_status = ?, last_summary = ?, last_job_id = ?, last_conv_id = ?, next_run_at = ?, updated_at = ?
		 WHERE id = ?`,
		lastRunAt, lastStatus, lastSummary, lastJobID, lastConvID, nextRunAt, utcNow(), id,
	)
	return err
}

// DeleteAutomation deletes a scheduled task by ID.
func (g *GatewayDB) DeleteAutomation(id string) (bool, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	res, err := g.sql.Exec("DELETE FROM automations WHERE id = ?", id)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// GetAlertConfig loads active alert rules and ntfy/push configuration from settings.
func (g *GatewayDB) GetAlertConfig() (AlertConfig, error) {
	cfg := DefaultAlertConfig()

	g.mu.Lock()
	defer g.mu.Unlock()

	var raw string
	err := g.sql.QueryRow("SELECT value FROM settings WHERE key = 'alert_config'").Scan(&raw)
	if err == sql.ErrNoRows {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}
	_ = json.Unmarshal([]byte(raw), &cfg)
	if strings.TrimSpace(cfg.NtfyURL) == "" {
		cfg.NtfyURL = DefaultAlertConfig().NtfyURL
	}
	return cfg, nil
}

// SaveAlertConfig persists updated active alert rules and delivery endpoints in settings.
func (g *GatewayDB) SaveAlertConfig(cfg AlertConfig) (AlertConfig, error) {
	if cfg.NotifyLongJobSec < 0 {
		cfg.NotifyLongJobSec = 0
	}
	if cfg.GPUTempThresholdC < 0 {
		cfg.GPUTempThresholdC = 0
	}
	if cfg.DiskFreeMinPct < 0 {
		cfg.DiskFreeMinPct = 0
	}
	if cfg.DiskFreeMinPct > 95 {
		cfg.DiskFreeMinPct = 95
	}
	cfg.NtfyTopic = strings.TrimSpace(cfg.NtfyTopic)
	cfg.NtfyURL = strings.TrimRight(strings.TrimSpace(cfg.NtfyURL), "/")
	if cfg.NtfyURL == "" {
		cfg.NtfyURL = "https://ntfy.sh"
	}

	encoded, err := json.Marshal(cfg)
	if err != nil {
		return cfg, err
	}
	now := utcNow()

	g.mu.Lock()
	defer g.mu.Unlock()

	_, err = g.sql.Exec(
		`INSERT INTO settings (key, value, updated_at)
		 VALUES ('alert_config', ?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
		string(encoded), now,
	)
	return cfg, err
}

// InsertAlertEvent records a triggered alert into SQLite and caps history to the newest 200 items.
func (g *GatewayDB) InsertAlertEvent(ev AlertEvent) (*AlertEvent, error) {
	if ev.CreatedAt == "" {
		ev.CreatedAt = utcNow()
	}
	if ev.Severity == "" {
		ev.Severity = "info"
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	res, err := g.sql.Exec(
		`INSERT INTO alert_events (source, severity, title, message, conv_id, job_id, read, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		ev.Source, ev.Severity, ev.Title, ev.Message, ev.ConvID, ev.JobID, boolToInt(ev.Read), ev.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	_, _ = g.sql.Exec("DELETE FROM alert_events WHERE id NOT IN (SELECT id FROM alert_events ORDER BY id DESC LIMIT 200)")
	out := ev
	out.ID = id
	return &out, nil
}

// ListAlertEvents returns recent alert events along with total unread count.
func (g *GatewayDB) ListAlertEvents(limit int) ([]AlertEvent, int, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	var unread int
	_ = g.sql.QueryRow("SELECT COUNT(*) FROM alert_events WHERE read = 0").Scan(&unread)

	rows, err := g.sql.Query(
		`SELECT id, source, severity, title, message, conv_id, job_id, read, created_at
		 FROM alert_events
		 ORDER BY id DESC
		 LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, unread, err
	}
	defer rows.Close()

	out := []AlertEvent{}
	for rows.Next() {
		var ev AlertEvent
		var readInt int
		if err := rows.Scan(&ev.ID, &ev.Source, &ev.Severity, &ev.Title, &ev.Message, &ev.ConvID, &ev.JobID, &readInt, &ev.CreatedAt); err != nil {
			return nil, unread, err
		}
		ev.Read = readInt != 0
		out = append(out, ev)
	}
	return out, unread, rows.Err()
}

// CountUnreadAlerts returns the count of unread alerts efficiently for polling badges.
func (g *GatewayDB) CountUnreadAlerts() (int, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	var unread int
	err := g.sql.QueryRow("SELECT COUNT(*) FROM alert_events WHERE read = 0").Scan(&unread)
	return unread, err
}

// MarkAlertsRead marks a specific alert read (id > 0) or all alerts read (id <= 0).
func (g *GatewayDB) MarkAlertsRead(id int64) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	if id > 0 {
		_, err := g.sql.Exec("UPDATE alert_events SET read = 1 WHERE id = ?", id)
		return err
	}
	_, err := g.sql.Exec("UPDATE alert_events SET read = 1 WHERE read = 0")
	return err
}

// ClearAlertEvents removes all persisted alert history.
func (g *GatewayDB) ClearAlertEvents() error {
	g.mu.Lock()
	defer g.mu.Unlock()

	_, err := g.sql.Exec("DELETE FROM alert_events")
	return err
}

// UpsertPushSubscription stores or refreshes a browser Web Push subscription JSON.
func (g *GatewayDB) UpsertPushSubscription(endpoint, subscriptionJSON string) error {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" || strings.TrimSpace(subscriptionJSON) == "" {
		return fmt.Errorf("invalid push subscription")
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	_, err := g.sql.Exec(
		`INSERT INTO push_subscriptions (endpoint, subscription_json, created_at)
		 VALUES (?, ?, ?)
		 ON CONFLICT(endpoint) DO UPDATE SET subscription_json = excluded.subscription_json, created_at = excluded.created_at`,
		endpoint, subscriptionJSON, utcNow(),
	)
	return err
}

// DeletePushSubscription removes a browser Web Push subscription by endpoint URL.
func (g *GatewayDB) DeletePushSubscription(endpoint string) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	_, err := g.sql.Exec("DELETE FROM push_subscriptions WHERE endpoint = ?", strings.TrimSpace(endpoint))
	return err
}

// ListPushSubscriptions returns all stored PushSubscription JSON payloads.
func (g *GatewayDB) ListPushSubscriptions() ([]string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	rows, err := g.sql.Query("SELECT subscription_json FROM push_subscriptions")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []string{}
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// GetOrSetSettingString reads a string setting atomically or persists generator() output if missing.
func (g *GatewayDB) GetOrSetSettingString(key string, generator func() (string, error)) (string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	var raw string
	err := g.sql.QueryRow("SELECT value FROM settings WHERE key = ?", key).Scan(&raw)
	if err == nil {
		var decoded string
		if json.Unmarshal([]byte(raw), &decoded) == nil && decoded != "" {
			return decoded, nil
		}
		if raw != "" {
			return raw, nil
		}
	} else if err != sql.ErrNoRows {
		return "", err
	}

	val, err := generator()
	if err != nil {
		return "", err
	}
	encoded, err := json.Marshal(val)
	if err != nil {
		return "", err
	}
	if _, err := g.sql.Exec(
		`INSERT INTO settings (key, value, updated_at)
		 VALUES (?, ?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
		key, string(encoded), utcNow(),
	); err != nil {
		return "", err
	}
	return val, nil
}
