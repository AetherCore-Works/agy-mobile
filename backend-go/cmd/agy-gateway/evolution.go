package main

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/Phoenix7290/agy-mobile-gateway/backend-go/internal/db"
	"github.com/Phoenix7290/agy-mobile-gateway/backend-go/internal/evolution"
)

type feedbackCreateRequest struct {
	JobID          string `json:"job_id"`
	ConversationID string `json:"conversation_id"`
	Rating         string `json:"rating"` // up | down
	Correction     string `json:"correction"`
	MessageSnippet string `json:"message_snippet"`
}

func registerEvolutionRoutes(mux *http.ServeMux, store *db.GatewayDB, evo *evolution.Service) {
	mux.HandleFunc("POST /api/feedback", func(w http.ResponseWriter, req *http.Request) {
		var body feedbackCreateRequest
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"detail": "JSON inválido"})
			return
		}
		rating := strings.ToLower(strings.TrimSpace(body.Rating))
		correction := strings.TrimSpace(body.Correction)
		if rating != "up" && rating != "down" && correction == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"detail": "Informe rating ('up'|'down') ou texto de correção"})
			return
		}

		kind := "feedback_up"
		if correction != "" {
			kind = "user_correction"
		} else if rating == "down" {
			kind = "feedback_down"
		}

		convID := strings.TrimSpace(body.ConversationID)
		jobID := strings.TrimSpace(body.JobID)
		model := ""
		engine := ""
		promptPreview := strings.TrimSpace(body.MessageSnippet)
		if jobID != "" {
			if j, err := store.GetJob(jobID); err == nil && j != nil {
				model = j.Model
				engine = j.Engine
				if j.ConversationID != nil && convID == "" {
					convID = *j.ConversationID
				}
				if j.Prompt != "" {
					promptPreview = j.Prompt
				}
			}
		}

		if err := store.RecordTelemetryEvent(db.TelemetryEvent{
			Kind:           kind,
			ConvID:         convID,
			JobID:          jobID,
			PromptPreview:  promptPreview,
			Model:          model,
			Engine:         engine,
			CorrectionNote: correction,
		}); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"detail": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "kind": kind})
	})

	mux.HandleFunc("GET /api/evolution/report", func(w http.ResponseWriter, _ *http.Request) {
		report, err := evo.BuildReport()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"detail": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, report)
	})

	mux.HandleFunc("POST /api/evolution/run", func(w http.ResponseWriter, req *http.Request) {
		var body struct {
			Force *bool `json:"force"`
		}
		if req.Body != nil {
			_ = json.NewDecoder(req.Body).Decode(&body)
		}
		force := true
		if body.Force != nil {
			force = *body.Force
		}

		outcome, err := evo.RunCycle(req.Context(), "manual", force)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"detail": err.Error()})
			return
		}
		report, repErr := evo.BuildReport()
		if repErr != nil {
			writeJSON(w, http.StatusOK, map[string]any{"ok": true, "outcome": outcome})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":      true,
			"outcome": outcome,
			"report":  report,
		})
	})

	mux.HandleFunc("POST /api/evolution/proposals/{id}/approve", func(w http.ResponseWriter, req *http.Request) {
		res, err := evo.ApproveProposal(req.Context(), req.PathValue("id"))
		if err != nil {
			status := http.StatusBadRequest
			if strings.Contains(err.Error(), "não encontrada") {
				status = http.StatusNotFound
			} else if strings.Contains(err.Error(), "já foi") {
				status = http.StatusConflict
			}
			writeJSON(w, status, map[string]string{"detail": err.Error()})
			return
		}
		pendingCount, _ := store.CountPendingEvolutionProposals()
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":            true,
			"proposal":      res.Proposal,
			"slash_command": res.SlashCommand,
			"rule":          res.Rule,
			"pending_count": pendingCount,
		})
	})

	mux.HandleFunc("POST /api/evolution/proposals/{id}/reject", func(w http.ResponseWriter, req *http.Request) {
		prop, err := evo.RejectProposal(req.PathValue("id"))
		if err != nil {
			status := http.StatusBadRequest
			if strings.Contains(err.Error(), "não encontrada") {
				status = http.StatusNotFound
			} else if strings.Contains(err.Error(), "já foi") {
				status = http.StatusConflict
			}
			writeJSON(w, status, map[string]string{"detail": err.Error()})
			return
		}
		pendingCount, _ := store.CountPendingEvolutionProposals()
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":            true,
			"proposal":      prop,
			"pending_count": pendingCount,
		})
	})

	mux.HandleFunc("PATCH /api/evolution/rules/{id}", func(w http.ResponseWriter, req *http.Request) {
		var body struct {
			Enabled bool `json:"enabled"`
		}
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"detail": "JSON inválido"})
			return
		}
		rule, err := store.SetAegysRuleEnabled(req.PathValue("id"), body.Enabled)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"detail": err.Error()})
			return
		}
		if rule == nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"detail": "Regra não encontrada"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "rule": rule})
	})

	mux.HandleFunc("DELETE /api/evolution/rules/{id}", func(w http.ResponseWriter, req *http.Request) {
		deleted, err := store.DeleteAegysRule(req.PathValue("id"))
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"detail": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "deleted": deleted})
	})
}
