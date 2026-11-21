package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
)

const slashSeededSettingKey = "slash_defaults_initialized"

func defaultSlashCommandPresets(now string) []SlashCommand {
	return []SlashCommand{
		{
			ID:             "preset-publish",
			Command:        "/publish",
			Description:    "Build e deploy de aplicação web/estática",
			PromptTemplate: "Execute o build e deploy da aplicação web configurada neste workspace. {{input}}",
			Model:          "",
			AutoSubmit:     true,
			SortOrder:      10,
			CreatedAt:      now,
			UpdatedAt:      now,
		},
		{
			ID:             "preset-status",
			Command:        "/status",
			Description:    "Checar saúde do host, containers e recursos",
			PromptTemplate: "Verifique o status atual do host, containers ativos, uso de disco/memória e eventuais alertas. {{input}}",
			Model:          "",
			AutoSubmit:     true,
			SortOrder:      20,
			CreatedAt:      now,
			UpdatedAt:      now,
		},
		{
			ID:             "preset-summarize",
			Command:        "/summarize",
			Description:    "Resumir logs, documentação ou mudanças recentes",
			PromptTemplate: "Resuma de forma objetiva os pontos principais, riscos e próximos passos do seguinte conteúdo: {{input}}",
			Model:          "",
			AutoSubmit:     false,
			SortOrder:      30,
			CreatedAt:      now,
			UpdatedAt:      now,
		},
		{
			ID:             "preset-review",
			Command:        "/review",
			Description:    "Revisar código, bugs, segurança e edge cases",
			PromptTemplate: "Faça uma revisão técnica completa do workspace atual focando em bugs, segurança, edge cases e qualidade. {{input}}",
			Model:          "",
			AutoSubmit:     true,
			SortOrder:      40,
			CreatedAt:      now,
			UpdatedAt:      now,
		},
		{
			ID:             "preset-test",
			Command:        "/test",
			Description:    "Executar suíte de testes e diagnosticar falhas",
			PromptTemplate: "Execute os testes relevantes no workspace atual e apresente um diagnóstico resumido dos resultados e eventuais falhas. {{input}}",
			Model:          "",
			AutoSubmit:     true,
			SortOrder:      50,
			CreatedAt:      now,
			UpdatedAt:      now,
		},
	}
}

func (g *GatewayDB) seedDefaultSlashCommandsLocked() error {
	var seededVal string
	err := g.sql.QueryRow("SELECT value FROM settings WHERE key = ?", slashSeededSettingKey).Scan(&seededVal)
	if err == nil {
		return nil
	}
	if err != sql.ErrNoRows {
		return err
	}

	var count int
	if err := g.sql.QueryRow("SELECT COUNT(*) FROM slash_commands").Scan(&count); err != nil {
		return err
	}
	now := utcNow()
	if count == 0 {
		for _, item := range defaultSlashCommandPresets(now) {
			autoInt := 0
			if item.AutoSubmit {
				autoInt = 1
			}
			if _, err := g.sql.Exec(
				`INSERT OR IGNORE INTO slash_commands
				 (id, command, description, prompt_template, model, auto_submit, sort_order, created_at, updated_at)
				 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				item.ID, item.Command, item.Description, item.PromptTemplate, item.Model, autoInt, item.SortOrder, now, now,
			); err != nil {
				return err
			}
		}
	}

	_, err = g.sql.Exec(
		`INSERT INTO settings (key, value, updated_at) VALUES (?, ?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
		slashSeededSettingKey, `"true"`, now,
	)
	return err
}

// RestoreDefaultSlashCommands inserts any missing starter slash command presets without overwriting custom ones.
func (g *GatewayDB) RestoreDefaultSlashCommands() ([]SlashCommand, error) {
	now := utcNow()
	g.mu.Lock()
	for _, item := range defaultSlashCommandPresets(now) {
		autoInt := 0
		if item.AutoSubmit {
			autoInt = 1
		}
		if _, err := g.sql.Exec(
			`INSERT INTO slash_commands
			 (id, command, description, prompt_template, model, auto_submit, sort_order, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
			 ON CONFLICT(command) DO NOTHING`,
			item.ID, item.Command, item.Description, item.PromptTemplate, item.Model, autoInt, item.SortOrder, now, now,
		); err != nil {
			g.mu.Unlock()
			return nil, err
		}
	}
	g.mu.Unlock()
	return g.ListSlashCommands()
}

// ListSlashCommands returns all persisted custom slash commands ordered for display.
func (g *GatewayDB) ListSlashCommands() ([]SlashCommand, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	rows, err := g.sql.Query(
		`SELECT id, command, description, prompt_template, model, auto_submit, sort_order, created_at, updated_at
		 FROM slash_commands
		 ORDER BY sort_order ASC, command ASC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []SlashCommand{}
	for rows.Next() {
		var item SlashCommand
		var autoInt int
		if err := rows.Scan(
			&item.ID, &item.Command, &item.Description, &item.PromptTemplate,
			&item.Model, &autoInt, &item.SortOrder, &item.CreatedAt, &item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		item.AutoSubmit = autoInt != 0
		items = append(items, item)
	}
	return items, rows.Err()
}

// CreateSlashCommand persists a new custom slash command.
func (g *GatewayDB) CreateSlashCommand(cmd SlashCommand) (*SlashCommand, error) {
	now := utcNow()
	if cmd.CreatedAt == "" {
		cmd.CreatedAt = now
	}
	cmd.UpdatedAt = now
	if cmd.SortOrder == 0 {
		cmd.SortOrder = 100
	}
	autoInt := 0
	if cmd.AutoSubmit {
		autoInt = 1
	}

	g.mu.Lock()
	_, err := g.sql.Exec(
		`INSERT INTO slash_commands
		 (id, command, description, prompt_template, model, auto_submit, sort_order, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		cmd.ID, cmd.Command, cmd.Description, cmd.PromptTemplate, cmd.Model, autoInt, cmd.SortOrder, cmd.CreatedAt, cmd.UpdatedAt,
	)
	g.mu.Unlock()
	if err != nil {
		return nil, err
	}
	return &cmd, nil
}

// UpdateSlashCommand applies partial edits to a custom slash command by ID.
func (g *GatewayDB) UpdateSlashCommand(id string, patch SlashCommandPatch) (*SlashCommand, error) {
	g.mu.Lock()
	var cur SlashCommand
	var autoInt int
	err := g.sql.QueryRow(
		`SELECT id, command, description, prompt_template, model, auto_submit, sort_order, created_at, updated_at
		 FROM slash_commands WHERE id = ?`, id,
	).Scan(
		&cur.ID, &cur.Command, &cur.Description, &cur.PromptTemplate,
		&cur.Model, &autoInt, &cur.SortOrder, &cur.CreatedAt, &cur.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		g.mu.Unlock()
		return nil, nil
	}
	if err != nil {
		g.mu.Unlock()
		return nil, err
	}
	cur.AutoSubmit = autoInt != 0

	if patch.Command != nil {
		cur.Command = strings.TrimSpace(*patch.Command)
	}
	if patch.Description != nil {
		cur.Description = strings.TrimSpace(*patch.Description)
	}
	if patch.PromptTemplate != nil {
		cur.PromptTemplate = strings.TrimSpace(*patch.PromptTemplate)
	}
	if patch.Model != nil {
		cur.Model = strings.TrimSpace(*patch.Model)
	}
	if patch.AutoSubmit != nil {
		cur.AutoSubmit = *patch.AutoSubmit
	}
	if patch.SortOrder != nil {
		cur.SortOrder = *patch.SortOrder
	}
	cur.UpdatedAt = utcNow()

	newAutoInt := 0
	if cur.AutoSubmit {
		newAutoInt = 1
	}
	_, err = g.sql.Exec(
		`UPDATE slash_commands
		 SET command = ?, description = ?, prompt_template = ?, model = ?, auto_submit = ?, sort_order = ?, updated_at = ?
		 WHERE id = ?`,
		cur.Command, cur.Description, cur.PromptTemplate, cur.Model, newAutoInt, cur.SortOrder, cur.UpdatedAt, cur.ID,
	)
	g.mu.Unlock()
	if err != nil {
		return nil, err
	}
	return &cur, nil
}

// DeleteSlashCommand removes a custom slash command by ID.
func (g *GatewayDB) DeleteSlashCommand(id string) (bool, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	res, err := g.sql.Exec("DELETE FROM slash_commands WHERE id = ?", id)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// RecordConversationArtifacts persists Outbox files produced during an assistant turn.
func (g *GatewayDB) RecordConversationArtifacts(convID, jobID, snippet string, filenames []string) error {
	cleanConv := strings.TrimSpace(convID)
	if cleanConv == "" || len(filenames) == 0 {
		return nil
	}
	seen := map[string]bool{}
	cleanNames := make([]string, 0, len(filenames))
	for _, f := range filenames {
		trimmed := strings.TrimSpace(f)
		if trimmed != "" && !seen[trimmed] {
			seen[trimmed] = true
			cleanNames = append(cleanNames, trimmed)
		}
	}
	if len(cleanNames) == 0 {
		return nil
	}
	rawJSON, err := json.Marshal(cleanNames)
	if err != nil {
		return err
	}
	cleanSnippet := clipArtifactSnippet(snippet, 160)

	g.mu.Lock()
	defer g.mu.Unlock()
	_, err = g.sql.Exec(
		`INSERT INTO conversation_artifacts (conv_id, job_id, filenames_json, snippet, created_at)
		 VALUES (?, ?, ?, ?, ?)`,
		cleanConv, jobID, string(rawJSON), cleanSnippet, utcNow(),
	)
	return err
}

// ListConversationArtifacts returns Outbox artifacts recorded for a conversation in chronological order.
func (g *GatewayDB) ListConversationArtifacts(convID string) ([]ConversationArtifact, error) {
	cleanConv := strings.TrimSpace(convID)
	if cleanConv == "" {
		return nil, nil
	}
	g.mu.Lock()
	defer g.mu.Unlock()

	rows, err := g.sql.Query(
		`SELECT id, conv_id, job_id, filenames_json, snippet, created_at
		 FROM conversation_artifacts
		 WHERE conv_id = ?
		 ORDER BY id ASC`,
		cleanConv,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []ConversationArtifact
	for rows.Next() {
		var item ConversationArtifact
		var rawJSON string
		if err := rows.Scan(&item.ID, &item.ConvID, &item.JobID, &rawJSON, &item.Snippet, &item.CreatedAt); err != nil {
			return nil, err
		}
		if json.Unmarshal([]byte(rawJSON), &item.Filenames) != nil {
			item.Filenames = nil
		}
		if len(item.Filenames) > 0 {
			items = append(items, item)
		}
	}
	return items, rows.Err()
}

func clipArtifactSnippet(s string, maxRunes int) string {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return ""
	}
	runes := []rune(trimmed)
	if len(runes) <= maxRunes {
		return string(runes)
	}
	return fmt.Sprintf("%s", string(runes[:maxRunes]))
}
