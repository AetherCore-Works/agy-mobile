package db

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"regexp"
	"strings"
)

// TelemetryEvent captures usage, errors, slash invocations, and user feedback/corrections.
type TelemetryEvent struct {
	ID             int64    `json:"id"`
	Kind           string   `json:"kind"` // job_ok | job_error | slash_used | feedback_up | feedback_down | user_correction
	ConvID         string   `json:"conv_id,omitempty"`
	JobID          string   `json:"job_id,omitempty"`
	SlashCommand   string   `json:"slash_command,omitempty"`
	PromptNorm     string   `json:"prompt_norm,omitempty"`
	PromptPreview  string   `json:"prompt_preview,omitempty"`
	Model          string   `json:"model,omitempty"`
	Engine         string   `json:"engine,omitempty"`
	ExecutedTools  []string `json:"executed_tools,omitempty"`
	ErrorMessage   string   `json:"error_message,omitempty"`
	CorrectionNote string   `json:"correction_note,omitempty"`
	DurationMs     int64    `json:"duration_ms,omitempty"`
	CycleID        string   `json:"cycle_id,omitempty"`
	CreatedAt      string   `json:"created_at"`
}

// CommandUsageStat summarizes frequency of a slash command or execution model.
type CommandUsageStat struct {
	Name       string `json:"name"`
	Count      int    `json:"count"`
	ErrorCount int    `json:"error_count,omitempty"`
	LastUsedAt string `json:"last_used_at,omitempty"`
}

// RepeatedPromptStat identifies recurring non-slash prompts eligible for Fast-Path shortcuts.
type RepeatedPromptStat struct {
	PromptNorm    string `json:"prompt_norm"`
	PromptPreview string `json:"prompt_preview"`
	PreferredModel string `json:"preferred_model,omitempty"`
	Count         int    `json:"count"`
	LastUsedAt    string `json:"last_used_at"`
}

// FailureSample holds a recent execution error or user correction for consolidation analysis.
type FailureSample struct {
	Kind           string `json:"kind"`
	Model          string `json:"model,omitempty"`
	SlashCommand   string `json:"slash_command,omitempty"`
	PromptPreview  string `json:"prompt_preview,omitempty"`
	ErrorMessage   string `json:"error_message,omitempty"`
	CorrectionNote string `json:"correction_note,omitempty"`
	CreatedAt      string `json:"created_at"`
}

// TelemetryStats aggregates metrics displayed in the Evolution Report and passed to analyzers.
type TelemetryStats struct {
	TotalJobsOK        int                  `json:"total_jobs_ok"`
	TotalJobErrors     int                  `json:"total_job_errors"`
	TotalFeedbackUp    int                  `json:"total_feedback_up"`
	TotalFeedbackDown  int                  `json:"total_feedback_down"`
	TotalCorrections   int                  `json:"total_corrections"`
	Unconsolidated     int                  `json:"unconsolidated_events"`
	TopCommands        []CommandUsageStat   `json:"top_commands"`
	TopModels          []CommandUsageStat   `json:"top_models"`
	RepeatedPrompts    []RepeatedPromptStat `json:"repeated_prompts"`
	RecentFailures     []FailureSample      `json:"recent_failures"`
}

// EvolutionCycle tracks one off-peak or manual self-improvement consolidation run.
type EvolutionCycle struct {
	ID               string `json:"id"`
	Trigger          string `json:"trigger"`
	Status           string `json:"status"` // completed | partial | skipped | error
	EventsProcessed  int    `json:"events_processed"`
	ProposalsCreated int    `json:"proposals_created"`
	AnalyzerBackend  string `json:"analyzer_backend"`
	AIMemoryStatus   string `json:"ai_memory_status"`
	Summary          string `json:"summary"`
	Error            string `json:"error,omitempty"`
	StartedAt        string `json:"started_at"`
	FinishedAt       string `json:"finished_at,omitempty"`
}

// EvolutionProposal represents a single 1-click self-improvement suggestion.
type EvolutionProposal struct {
	ID          string         `json:"id"`
	CycleID     string         `json:"cycle_id"`
	Kind        string         `json:"kind"` // fast_path_slash | prompt_rule | preference
	Fingerprint string         `json:"fingerprint"`
	Title       string         `json:"title"`
	Rationale   string         `json:"rationale"`
	Payload     map[string]any `json:"payload"`
	Status      string         `json:"status"` // pending | approved | rejected
	AppliedRef  string         `json:"applied_ref,omitempty"`
	CreatedAt   string         `json:"created_at"`
	DecidedAt   string         `json:"decided_at,omitempty"`
}

// AegysRule stores an approved persistent rule or preference applied to future agent prompts.
type AegysRule struct {
	ID               string `json:"id"`
	Kind             string `json:"kind"` // prompt_rule | preference
	Title            string `json:"title"`
	Content          string `json:"content"`
	Enabled          bool   `json:"enabled"`
	SourceProposalID string `json:"source_proposal_id,omitempty"`
	CreatedAt        string `json:"created_at"`
	UpdatedAt        string `json:"updated_at"`
}

var (
	secretBearerRe   = regexp.MustCompile(`(?i)(authorization:\s*bearer\s+|bearer\s+)[A-Za-z0-9._\-]{8,}`)
	secretKeyValRe   = regexp.MustCompile(`(?i)\b(api[_-]?key|token|secret|password|passwd|pwd)\s*[:=]\s*["']?([^\s"';]{6,})`)
	secretPrefixRe   = regexp.MustCompile(`\b(sk-[A-Za-z0-9_\-]{12,}|ghp_[A-Za-z0-9]{16,}|AIza[0-9A-Za-z_\-]{16,}|xox[baprs]-[A-Za-z0-9\-]{10,})`)
	privateKeyBlockRe = regexp.MustCompile(`(?s)-----BEGIN [A-Z0-9 ]*PRIVATE KEY-----.*?-----END [A-Z0-9 ]*PRIVATE KEY-----`)
	multiSpaceRe     = regexp.MustCompile(`\s+`)
)

func clipTelemetryRunes(s string, maxLen int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= maxLen {
		return string(r)
	}
	return string(r[:maxLen])
}

// SanitizeTelemetryText redacts credentials/keys before storing or sending prompt snippets to local analyzers.
func SanitizeTelemetryText(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	s = privateKeyBlockRe.ReplaceAllString(s, "[REDACTED_PRIVATE_KEY]")
	s = secretBearerRe.ReplaceAllString(s, "${1}[REDACTED]")
	s = secretPrefixRe.ReplaceAllString(s, "[REDACTED_TOKEN]")
	s = secretKeyValRe.ReplaceAllString(s, "${1}=[REDACTED]")
	return clipTelemetryRunes(s, 500)
}

// NormalizePromptForGrouping canonicalizes non-slash user prompts to detect repeated requests reliably.
func NormalizePromptForGrouping(raw string) string {
	s := SanitizeTelemetryText(raw)
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.TrimRight(s, ".,!?;:")
	s = multiSpaceRe.ReplaceAllString(s, " ")
	return clipTelemetryRunes(s, 220)
}

// FingerprintString computes a compact deterministic SHA-256 hex key for deduplication.
func FingerprintString(parts ...string) string {
	h := sha256.New()
	for _, p := range parts {
		_, _ = h.Write([]byte(strings.ToLower(strings.TrimSpace(p))))
		_, _ = h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))[:24]
}

// RecordTelemetryEvent persists a usage, error, or feedback event into SQLite.
func (g *GatewayDB) RecordTelemetryEvent(ev TelemetryEvent) error {
	if ev.CreatedAt == "" {
		ev.CreatedAt = utcNow()
	}
	ev.Kind = strings.TrimSpace(ev.Kind)
	if ev.Kind == "" {
		return nil
	}
	ev.SlashCommand = strings.ToLower(strings.TrimSpace(ev.SlashCommand))
	ev.PromptPreview = SanitizeTelemetryText(ev.PromptPreview)
	if ev.PromptNorm == "" && ev.PromptPreview != "" {
		ev.PromptNorm = NormalizePromptForGrouping(ev.PromptPreview)
	}
	ev.ErrorMessage = SanitizeTelemetryText(ev.ErrorMessage)
	ev.CorrectionNote = SanitizeTelemetryText(ev.CorrectionNote)

	tools := ev.ExecutedTools
	if tools == nil {
		tools = []string{}
	}
	toolsJSON, _ := json.Marshal(tools)

	g.mu.Lock()
	defer g.mu.Unlock()

	_, err := g.sql.Exec(
		`INSERT INTO telemetry_events (
			kind, conv_id, job_id, slash_command, prompt_norm, prompt_preview,
			model, engine, executed_tools_json, error_message, correction_note,
			duration_ms, cycle_id, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		ev.Kind, ev.ConvID, ev.JobID, ev.SlashCommand, ev.PromptNorm, ev.PromptPreview,
		ev.Model, ev.Engine, string(toolsJSON), ev.ErrorMessage, ev.CorrectionNote,
		ev.DurationMs, ev.CycleID, ev.CreatedAt,
	)
	return err
}

// FindPreviousJobInConversation returns the latest completed or failed job prior to excludeJobID in convID.
func (g *GatewayDB) FindPreviousJobInConversation(convID, excludeJobID string) (*Job, error) {
	convID = strings.TrimSpace(convID)
	if convID == "" {
		return nil, nil
	}
	g.mu.Lock()
	defer g.mu.Unlock()

	var j Job
	var cid, errText sql.NullString
	err := g.sql.QueryRow(
		`SELECT id, conversation_id, engine, model, permission_mode, prompt, status, error, created_at, updated_at
		 FROM jobs
		 WHERE conversation_id = ? AND id != ?
		 ORDER BY created_at DESC LIMIT 1`,
		convID, excludeJobID,
	).Scan(&j.ID, &cid, &j.Engine, &j.Model, &j.PermissionMode, &j.Prompt, &j.Status, &errText, &j.CreatedAt, &j.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if cid.Valid {
		j.ConversationID = &cid.String
	}
	if errText.Valid {
		j.Error = &errText.String
	}
	return &j, nil
}

// ListUnconsolidatedTelemetry returns up to limit telemetry events not yet assigned to a consolidation cycle.
func (g *GatewayDB) ListUnconsolidatedTelemetry(limit int) ([]TelemetryEvent, error) {
	if limit <= 0 || limit > 1000 {
		limit = 300
	}
	g.mu.Lock()
	defer g.mu.Unlock()

	rows, err := g.sql.Query(
		`SELECT id, kind, conv_id, job_id, slash_command, prompt_norm, prompt_preview,
		        model, engine, executed_tools_json, error_message, correction_note,
		        duration_ms, cycle_id, created_at
		 FROM telemetry_events
		 WHERE cycle_id = ''
		 ORDER BY id ASC
		 LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []TelemetryEvent{}
	for rows.Next() {
		var ev TelemetryEvent
		var toolsRaw string
		if err := rows.Scan(
			&ev.ID, &ev.Kind, &ev.ConvID, &ev.JobID, &ev.SlashCommand, &ev.PromptNorm, &ev.PromptPreview,
			&ev.Model, &ev.Engine, &toolsRaw, &ev.ErrorMessage, &ev.CorrectionNote,
			&ev.DurationMs, &ev.CycleID, &ev.CreatedAt,
		); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(toolsRaw), &ev.ExecutedTools)
		if ev.ExecutedTools == nil {
			ev.ExecutedTools = []string{}
		}
		out = append(out, ev)
	}
	return out, rows.Err()
}

// MarkTelemetryConsolidated assigns processed telemetry events up to maxID to cycleID.
func (g *GatewayDB) MarkTelemetryConsolidated(maxID int64, cycleID string) error {
	if maxID <= 0 || strings.TrimSpace(cycleID) == "" {
		return nil
	}
	g.mu.Lock()
	defer g.mu.Unlock()

	_, err := g.sql.Exec(
		`UPDATE telemetry_events SET cycle_id = ? WHERE cycle_id = '' AND id <= ?`,
		cycleID, maxID,
	)
	return err
}

// GetTelemetryStats computes aggregated command usage, repeated prompts, models, and failure samples.
func (g *GatewayDB) GetTelemetryStats(topLimit, minRepeatHits int) (TelemetryStats, error) {
	if topLimit <= 0 || topLimit > 30 {
		topLimit = 8
	}
	if minRepeatHits < 2 {
		minRepeatHits = 2
	}

	stats := TelemetryStats{
		TopCommands:     []CommandUsageStat{},
		TopModels:       []CommandUsageStat{},
		RepeatedPrompts: []RepeatedPromptStat{},
		RecentFailures:  []FailureSample{},
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	kindRows, err := g.sql.Query(`SELECT kind, COUNT(*) FROM telemetry_events GROUP BY kind`)
	if err != nil {
		return stats, err
	}
	for kindRows.Next() {
		var k string
		var cnt int
		if kindRows.Scan(&k, &cnt) == nil {
			switch k {
			case "job_ok":
				stats.TotalJobsOK = cnt
			case "job_error":
				stats.TotalJobErrors = cnt
			case "feedback_up":
				stats.TotalFeedbackUp = cnt
			case "feedback_down":
				stats.TotalFeedbackDown = cnt
			case "user_correction":
				stats.TotalCorrections = cnt
			}
		}
	}
	_ = kindRows.Close()

	_ = g.sql.QueryRow(`SELECT COUNT(*) FROM telemetry_events WHERE cycle_id = ''`).Scan(&stats.Unconsolidated)

	cmdRows, err := g.sql.Query(
		`SELECT slash_command, COUNT(*) AS cnt, MAX(created_at) AS last_used
		 FROM telemetry_events
		 WHERE kind = 'slash_used' AND slash_command != ''
		 GROUP BY slash_command
		 ORDER BY cnt DESC, last_used DESC
		 LIMIT ?`,
		topLimit,
	)
	if err == nil {
		for cmdRows.Next() {
			var s CommandUsageStat
			if cmdRows.Scan(&s.Name, &s.Count, &s.LastUsedAt) == nil {
				stats.TopCommands = append(stats.TopCommands, s)
			}
		}
		_ = cmdRows.Close()
	}

	modelRows, err := g.sql.Query(
		`SELECT model,
		        COUNT(*) AS cnt,
		        SUM(CASE WHEN kind = 'job_error' THEN 1 ELSE 0 END) AS errs,
		        MAX(created_at) AS last_used
		 FROM telemetry_events
		 WHERE kind IN ('job_ok', 'job_error') AND model != ''
		 GROUP BY model
		 ORDER BY cnt DESC, last_used DESC
		 LIMIT ?`,
		topLimit,
	)
	if err == nil {
		for modelRows.Next() {
			var s CommandUsageStat
			if modelRows.Scan(&s.Name, &s.Count, &s.ErrorCount, &s.LastUsedAt) == nil {
				stats.TopModels = append(stats.TopModels, s)
			}
		}
		_ = modelRows.Close()
	}

	repRows, err := g.sql.Query(
		`SELECT prompt_norm,
		        MAX(prompt_preview) AS preview,
		        MAX(model) AS model,
		        COUNT(*) AS cnt,
		        MAX(created_at) AS last_used
		 FROM telemetry_events
		 WHERE kind = 'job_ok'
		   AND slash_command = ''
		   AND prompt_norm != ''
		   AND LENGTH(prompt_norm) >= 8
		 GROUP BY prompt_norm
		 HAVING COUNT(*) >= ?
		 ORDER BY cnt DESC, last_used DESC
		 LIMIT ?`,
		minRepeatHits, topLimit,
	)
	if err == nil {
		for repRows.Next() {
			var r RepeatedPromptStat
			if repRows.Scan(&r.PromptNorm, &r.PromptPreview, &r.PreferredModel, &r.Count, &r.LastUsedAt) == nil {
				stats.RepeatedPrompts = append(stats.RepeatedPrompts, r)
			}
		}
		_ = repRows.Close()
	}

	failRows, err := g.sql.Query(
		`SELECT kind, model, slash_command, prompt_preview, error_message, correction_note, created_at
		 FROM telemetry_events
		 WHERE kind IN ('job_error', 'feedback_down', 'user_correction')
		 ORDER BY id DESC
		 LIMIT ?`,
		topLimit,
	)
	if err == nil {
		for failRows.Next() {
			var f FailureSample
			if failRows.Scan(&f.Kind, &f.Model, &f.SlashCommand, &f.PromptPreview, &f.ErrorMessage, &f.CorrectionNote, &f.CreatedAt) == nil {
				stats.RecentFailures = append(stats.RecentFailures, f)
			}
		}
		_ = failRows.Close()
	}

	return stats, nil
}

// CreateEvolutionCycle inserts a new consolidation cycle record.
func (g *GatewayDB) CreateEvolutionCycle(c EvolutionCycle) (*EvolutionCycle, error) {
	if c.StartedAt == "" {
		c.StartedAt = utcNow()
	}
	if c.Status == "" {
		c.Status = "running"
	}
	g.mu.Lock()
	defer g.mu.Unlock()

	_, err := g.sql.Exec(
		`INSERT INTO evolution_cycles (
			id, trigger, status, events_processed, proposals_created,
			analyzer_backend, ai_memory_status, summary, error, started_at, finished_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.ID, c.Trigger, c.Status, c.EventsProcessed, c.ProposalsCreated,
		c.AnalyzerBackend, c.AIMemoryStatus, c.Summary, c.Error, c.StartedAt, c.FinishedAt,
	)
	if err != nil {
		return nil, err
	}
	out := c
	return &out, nil
}

// FinishEvolutionCycle updates completion status and counters for an evolution cycle.
func (g *GatewayDB) FinishEvolutionCycle(c EvolutionCycle) error {
	if c.FinishedAt == "" {
		c.FinishedAt = utcNow()
	}
	g.mu.Lock()
	defer g.mu.Unlock()

	_, err := g.sql.Exec(
		`UPDATE evolution_cycles
		 SET status = ?, events_processed = ?, proposals_created = ?,
		     analyzer_backend = ?, ai_memory_status = ?, summary = ?, error = ?, finished_at = ?
		 WHERE id = ?`,
		c.Status, c.EventsProcessed, c.ProposalsCreated,
		c.AnalyzerBackend, c.AIMemoryStatus, c.Summary, c.Error, c.FinishedAt, c.ID,
	)
	return err
}

// ListEvolutionCycles returns the newest evolution consolidation runs.
func (g *GatewayDB) ListEvolutionCycles(limit int) ([]EvolutionCycle, error) {
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	g.mu.Lock()
	defer g.mu.Unlock()

	rows, err := g.sql.Query(
		`SELECT id, trigger, status, events_processed, proposals_created,
		        analyzer_backend, ai_memory_status, summary, error, started_at, finished_at
		 FROM evolution_cycles
		 ORDER BY started_at DESC
		 LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []EvolutionCycle{}
	for rows.Next() {
		var c EvolutionCycle
		if err := rows.Scan(
			&c.ID, &c.Trigger, &c.Status, &c.EventsProcessed, &c.ProposalsCreated,
			&c.AnalyzerBackend, &c.AIMemoryStatus, &c.Summary, &c.Error, &c.StartedAt, &c.FinishedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// HasProposalFingerprint returns true if a matching suggestion already exists as pending, approved, or rejected.
func (g *GatewayDB) HasProposalFingerprint(fp string) (bool, error) {
	fp = strings.TrimSpace(fp)
	if fp == "" {
		return false, nil
	}
	g.mu.Lock()
	defer g.mu.Unlock()

	var count int
	err := g.sql.QueryRow(
		`SELECT COUNT(*) FROM evolution_proposals WHERE fingerprint = ?`,
		fp,
	).Scan(&count)
	return count > 0, err
}

// CreateEvolutionProposal stores one self-improvement suggestion awaiting or decided by the user.
func (g *GatewayDB) CreateEvolutionProposal(p EvolutionProposal) (*EvolutionProposal, error) {
	if p.CreatedAt == "" {
		p.CreatedAt = utcNow()
	}
	if p.Status == "" {
		p.Status = "pending"
	}
	if p.Payload == nil {
		p.Payload = map[string]any{}
	}
	payloadRaw, err := json.Marshal(p.Payload)
	if err != nil {
		return nil, err
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	_, err = g.sql.Exec(
		`INSERT INTO evolution_proposals (
			id, cycle_id, kind, fingerprint, title, rationale, payload_json, status, applied_ref, created_at, decided_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.ID, p.CycleID, p.Kind, p.Fingerprint, p.Title, p.Rationale, string(payloadRaw), p.Status, p.AppliedRef, p.CreatedAt, p.DecidedAt,
	)
	if err != nil {
		return nil, err
	}
	out := p
	return &out, nil
}

func scanProposal(scanner interface{ Scan(...any) error }) (*EvolutionProposal, error) {
	var p EvolutionProposal
	var payloadRaw string
	if err := scanner.Scan(
		&p.ID, &p.CycleID, &p.Kind, &p.Fingerprint, &p.Title, &p.Rationale,
		&payloadRaw, &p.Status, &p.AppliedRef, &p.CreatedAt, &p.DecidedAt,
	); err != nil {
		return nil, err
	}
	p.Payload = map[string]any{}
	_ = json.Unmarshal([]byte(payloadRaw), &p.Payload)
	return &p, nil
}

// GetEvolutionProposal fetches a single proposal by ID or nil if absent.
func (g *GatewayDB) GetEvolutionProposal(id string) (*EvolutionProposal, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	row := g.sql.QueryRow(
		`SELECT id, cycle_id, kind, fingerprint, title, rationale, payload_json, status, applied_ref, created_at, decided_at
		 FROM evolution_proposals WHERE id = ?`,
		id,
	)
	p, err := scanProposal(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return p, err
}

// ListEvolutionProposals lists evolution suggestions ordered pending first, newest first.
func (g *GatewayDB) ListEvolutionProposals(statusFilter string, limit int) ([]EvolutionProposal, error) {
	if limit <= 0 || limit > 100 {
		limit = 40
	}
	g.mu.Lock()
	defer g.mu.Unlock()

	var rows *sql.Rows
	var err error
	if statusFilter != "" {
		rows, err = g.sql.Query(
			`SELECT id, cycle_id, kind, fingerprint, title, rationale, payload_json, status, applied_ref, created_at, decided_at
			 FROM evolution_proposals
			 WHERE status = ?
			 ORDER BY created_at DESC
			 LIMIT ?`,
			statusFilter, limit,
		)
	} else {
		rows, err = g.sql.Query(
			`SELECT id, cycle_id, kind, fingerprint, title, rationale, payload_json, status, applied_ref, created_at, decided_at
			 FROM evolution_proposals
			 ORDER BY CASE WHEN status = 'pending' THEN 0 ELSE 1 END ASC, created_at DESC
			 LIMIT ?`,
			limit,
		)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []EvolutionProposal{}
	for rows.Next() {
		p, scanErr := scanProposal(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, *p)
	}
	return out, rows.Err()
}

// CountPendingEvolutionProposals returns the number of proposals waiting for 1-click decision.
func (g *GatewayDB) CountPendingEvolutionProposals() (int, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	var count int
	err := g.sql.QueryRow(`SELECT COUNT(*) FROM evolution_proposals WHERE status = 'pending'`).Scan(&count)
	return count, err
}

// UpdateEvolutionProposalDecision marks a proposal approved or rejected along with any created target ID.
func (g *GatewayDB) UpdateEvolutionProposalDecision(id, status, appliedRef string) (*EvolutionProposal, error) {
	decidedAt := utcNow()
	g.mu.Lock()
	res, err := g.sql.Exec(
		`UPDATE evolution_proposals SET status = ?, applied_ref = ?, decided_at = ? WHERE id = ?`,
		status, appliedRef, decidedAt, id,
	)
	g.mu.Unlock()
	if err != nil {
		return nil, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return nil, nil
	}
	return g.GetEvolutionProposal(id)
}

// CreateAegysRule persists a learned prompt rule or preference approved by the user.
func (g *GatewayDB) CreateAegysRule(r AegysRule) (*AegysRule, error) {
	now := utcNow()
	if r.CreatedAt == "" {
		r.CreatedAt = now
	}
	r.UpdatedAt = now
	if r.Kind == "" {
		r.Kind = "prompt_rule"
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	_, err := g.sql.Exec(
		`INSERT INTO aegys_rules (id, kind, title, content, enabled, source_proposal_id, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID, r.Kind, r.Title, r.Content, boolToInt(r.Enabled), r.SourceProposalID, r.CreatedAt, r.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	out := r
	return &out, nil
}

// ListAegysRules returns all learned rules ordered newest first.
func (g *GatewayDB) ListAegysRules() ([]AegysRule, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	rows, err := g.sql.Query(
		`SELECT id, kind, title, content, enabled, source_proposal_id, created_at, updated_at
		 FROM aegys_rules
		 ORDER BY enabled DESC, updated_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []AegysRule{}
	for rows.Next() {
		var r AegysRule
		var enabledInt int
		if err := rows.Scan(&r.ID, &r.Kind, &r.Title, &r.Content, &enabledInt, &r.SourceProposalID, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, err
		}
		r.Enabled = enabledInt != 0
		out = append(out, r)
	}
	return out, rows.Err()
}

// ListEnabledAegysRules returns active learned rules capped to limit for lightweight prompt injection.
func (g *GatewayDB) ListEnabledAegysRules(limit int) ([]AegysRule, error) {
	if limit <= 0 || limit > 20 {
		limit = 6
	}
	g.mu.Lock()
	defer g.mu.Unlock()

	rows, err := g.sql.Query(
		`SELECT id, kind, title, content, enabled, source_proposal_id, created_at, updated_at
		 FROM aegys_rules
		 WHERE enabled = 1
		 ORDER BY updated_at DESC
		 LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []AegysRule{}
	for rows.Next() {
		var r AegysRule
		var enabledInt int
		if err := rows.Scan(&r.ID, &r.Kind, &r.Title, &r.Content, &enabledInt, &r.SourceProposalID, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, err
		}
		r.Enabled = enabledInt != 0
		out = append(out, r)
	}
	return out, rows.Err()
}

// SetAegysRuleEnabled enables or pauses an approved rule by ID.
func (g *GatewayDB) SetAegysRuleEnabled(id string, enabled bool) (*AegysRule, error) {
	now := utcNow()
	g.mu.Lock()
	res, err := g.sql.Exec(
		`UPDATE aegys_rules SET enabled = ?, updated_at = ? WHERE id = ?`,
		boolToInt(enabled), now, id,
	)
	g.mu.Unlock()
	if err != nil {
		return nil, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return nil, nil
	}
	list, err := g.ListAegysRules()
	if err != nil {
		return nil, err
	}
	for i := range list {
		if list[i].ID == id {
			return &list[i], nil
		}
	}
	return nil, nil
}

// DeleteAegysRule deletes a learned rule by ID.
func (g *GatewayDB) DeleteAegysRule(id string) (bool, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	res, err := g.sql.Exec(`DELETE FROM aegys_rules WHERE id = ?`, id)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}
