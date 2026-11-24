package main

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"

	"github.com/Phoenix7290/agy-mobile-gateway/backend-go/internal/db"
	"github.com/Phoenix7290/agy-mobile-gateway/backend-go/internal/runner"
	"github.com/google/uuid"
)

var validSlashCommandRe = regexp.MustCompile(`^/[a-z0-9][a-z0-9_-]{0,31}$`)

var reservedBuiltinSlash = map[string]bool{
	"/usage":   true,
	"/compact": true,
	"/diff":    true,
	"/mem":     true,
	"/plan":    true,
	"/clear":   true,
	"/status":  true,
	"/evolve":  true,
}

type slashCommandUpsertRequest struct {
	Command        string `json:"command"`
	Description    string `json:"description"`
	PromptTemplate string `json:"prompt_template"`
	Model          string `json:"model"`
	AutoSubmit     *bool  `json:"auto_submit"`
	SortOrder      *int   `json:"sort_order"`
}

type slashCommandPutRequest struct {
	Command        *string `json:"command"`
	Description    *string `json:"description"`
	PromptTemplate *string `json:"prompt_template"`
	Model          *string `json:"model"`
	AutoSubmit     *bool   `json:"auto_submit"`
	SortOrder      *int    `json:"sort_order"`
}

func normalizeSlashCommand(raw string) (string, string) {
	cmd := strings.ToLower(strings.TrimSpace(raw))
	cmd = strings.TrimPrefix(cmd, "/")
	if cmd == "" {
		return "", "Informe o nome do comando (ex: /publish)"
	}
	cmd = "/" + cmd
	if !validSlashCommandRe.MatchString(cmd) {
		return "", "Comando deve usar apenas letras minúsculas, números, '-' ou '_' (máx. 32 caracteres)"
	}
	if reservedBuiltinSlash[cmd] {
		return "", "Este comando é reservado pelo sistema (" + cmd + ")"
	}
	return cmd, ""
}

func registerSlashCommandRoutes(mux *http.ServeMux, store *db.GatewayDB) {
	mux.HandleFunc("GET /api/slash-commands", func(w http.ResponseWriter, _ *http.Request) {
		items, err := store.ListSlashCommands()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"detail": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"commands": items,
			"reserved": []string{"/usage", "/compact", "/diff", "/mem", "/plan", "/clear", "/status", "/evolve"},
		})
	})

	mux.HandleFunc("POST /api/slash-commands", func(w http.ResponseWriter, req *http.Request) {
		var body slashCommandUpsertRequest
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"detail": "JSON inválido"})
			return
		}
		cmdName, errMsg := normalizeSlashCommand(body.Command)
		if errMsg != "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"detail": errMsg})
			return
		}
		tpl := strings.TrimSpace(body.PromptTemplate)
		if tpl == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"detail": "O template de prompt é obrigatório"})
			return
		}
		autoSubmit := true
		if body.AutoSubmit != nil {
			autoSubmit = *body.AutoSubmit
		}
		sortOrder := 100
		if body.SortOrder != nil {
			sortOrder = *body.SortOrder
		}

		created, err := store.CreateSlashCommand(db.SlashCommand{
			ID:             "sc-" + uuid.NewString()[:8],
			Command:        cmdName,
			Description:    clipRunes(body.Description, 140),
			PromptTemplate: tpl,
			Model:          strings.TrimSpace(body.Model),
			AutoSubmit:     autoSubmit,
			SortOrder:      sortOrder,
		})
		if err != nil {
			if strings.Contains(strings.ToUpper(err.Error()), "UNIQUE") {
				writeJSON(w, http.StatusConflict, map[string]string{"detail": "Já existe um comando com esse nome (" + cmdName + ")"})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"detail": err.Error()})
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"command": created})
	})

	mux.HandleFunc("PUT /api/slash-commands/{id}", func(w http.ResponseWriter, req *http.Request) {
		id := strings.TrimSpace(req.PathValue("id"))
		var body slashCommandPutRequest
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"detail": "JSON inválido"})
			return
		}
		patch := db.SlashCommandPatch{}
		if body.Command != nil && strings.TrimSpace(*body.Command) != "" {
			cmdName, errMsg := normalizeSlashCommand(*body.Command)
			if errMsg != "" {
				writeJSON(w, http.StatusBadRequest, map[string]string{"detail": errMsg})
				return
			}
			patch.Command = &cmdName
		}
		if body.PromptTemplate != nil {
			tpl := strings.TrimSpace(*body.PromptTemplate)
			if tpl == "" {
				writeJSON(w, http.StatusBadRequest, map[string]string{"detail": "O template de prompt é obrigatório"})
				return
			}
			patch.PromptTemplate = &tpl
		}
		if body.Description != nil {
			desc := clipRunes(*body.Description, 140)
			patch.Description = &desc
		}
		if body.Model != nil {
			model := strings.TrimSpace(*body.Model)
			patch.Model = &model
		}
		if body.AutoSubmit != nil {
			patch.AutoSubmit = body.AutoSubmit
		}
		if body.SortOrder != nil {
			patch.SortOrder = body.SortOrder
		}

		updated, err := store.UpdateSlashCommand(id, patch)
		if err != nil {
			if strings.Contains(strings.ToUpper(err.Error()), "UNIQUE") {
				writeJSON(w, http.StatusConflict, map[string]string{"detail": "Já existe outro comando com esse nome"})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"detail": err.Error()})
			return
		}
		if updated == nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"detail": "Comando não encontrado"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"command": updated})
	})

	mux.HandleFunc("DELETE /api/slash-commands/{id}", func(w http.ResponseWriter, req *http.Request) {
		id := strings.TrimSpace(req.PathValue("id"))
		deleted, err := store.DeleteSlashCommand(id)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"detail": err.Error()})
			return
		}
		if !deleted {
			writeJSON(w, http.StatusNotFound, map[string]string{"detail": "Comando não encontrado"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	})

	mux.HandleFunc("POST /api/slash-commands/reset", func(w http.ResponseWriter, _ *http.Request) {
		items, err := store.RestoreDefaultSlashCommands()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"detail": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"commands": items})
	})
}

func attachArtifactsToMessages(store *db.GatewayDB, convID string, msgs []runner.ConversationMessage) {
	artifacts, err := store.ListConversationArtifacts(convID)
	if err != nil || len(artifacts) == 0 || len(msgs) == 0 {
		return
	}

	var agentIndices []int
	for i := range msgs {
		role := strings.ToLower(strings.TrimSpace(msgs[i].Role))
		if role == "agent" || role == "assistant" {
			agentIndices = append(agentIndices, i)
		}
	}
	if len(agentIndices) == 0 {
		return
	}

	appendUnique := func(msgIdx int, names []string) {
		seen := map[string]bool{}
		for _, cur := range msgs[msgIdx].OutboxFiles {
			seen[cur] = true
		}
		for _, name := range names {
			if name != "" && !seen[name] {
				seen[name] = true
				msgs[msgIdx].OutboxFiles = append(msgs[msgIdx].OutboxFiles, name)
			}
		}
	}

	for _, art := range artifacts {
		targetIdx := -1
		snip := strings.TrimSpace(art.Snippet)
		if snip != "" {
			prefix := snip
			if r := []rune(prefix); len(r) > 60 {
				prefix = string(r[:60])
			}
			for j := len(agentIndices) - 1; j >= 0; j-- {
				idx := agentIndices[j]
				if strings.Contains(msgs[idx].Content, prefix) {
					targetIdx = idx
					break
				}
			}
		}
		if targetIdx == -1 {
			for j := len(agentIndices) - 1; j >= 0; j-- {
				idx := agentIndices[j]
				for _, fname := range art.Filenames {
					if fname != "" && strings.Contains(msgs[idx].Content, fname) {
						targetIdx = idx
						break
					}
				}
				if targetIdx != -1 {
					break
				}
			}
		}
		if targetIdx == -1 {
			targetIdx = agentIndices[len(agentIndices)-1]
		}
		appendUnique(targetIdx, art.Filenames)
	}
}
