package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Phoenix7290/agy-mobile-gateway/backend-go/internal/automation"
	"github.com/Phoenix7290/agy-mobile-gateway/backend-go/internal/db"
	"github.com/Phoenix7290/agy-mobile-gateway/backend-go/internal/jobs"
	"github.com/google/uuid"
)

type automationCreateRequest struct {
	Name           string `json:"name"`
	CronExpr       string `json:"cron_expr"`
	Kind           string `json:"kind"`
	Prompt         string `json:"prompt"`
	Model          string `json:"model"`
	PermissionMode string `json:"permission_mode"`
	NotifyNtfy     *bool  `json:"notify_ntfy"`
	NotifyPush     *bool  `json:"notify_push"`
	NotifyOn       string `json:"notify_on"`
	Enabled        *bool  `json:"enabled"`
}

type automationPatchRequest struct {
	Name           *string `json:"name"`
	CronExpr       *string `json:"cron_expr"`
	Kind           *string `json:"kind"`
	Prompt         *string `json:"prompt"`
	Model          *string `json:"model"`
	PermissionMode *string `json:"permission_mode"`
	NotifyNtfy     *bool   `json:"notify_ntfy"`
	NotifyPush     *bool   `json:"notify_push"`
	NotifyOn       *string `json:"notify_on"`
	Enabled        *bool   `json:"enabled"`
}

func registerAutomationRoutes(mux *http.ServeMux, store *db.GatewayDB, notifier *automation.Notifier, sched *automation.Scheduler) {
	mux.HandleFunc("GET /api/automations", func(w http.ResponseWriter, _ *http.Request) {
		list, err := store.ListAutomations()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"detail": err.Error()})
			return
		}
		unread, _ := store.CountUnreadAlerts()
		writeJSON(w, http.StatusOK, map[string]any{
			"automations":   list,
			"unread_alerts": unread,
		})
	})

	mux.HandleFunc("POST /api/automations", func(w http.ResponseWriter, req *http.Request) {
		var body automationCreateRequest
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"detail": "JSON inválido"})
			return
		}
		cronSched, err := automation.ParseCron(body.CronExpr)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"detail": err.Error()})
			return
		}

		kind := strings.ToLower(strings.TrimSpace(body.Kind))
		if kind != "check" {
			kind = "prompt"
		}
		prompt := strings.TrimSpace(body.Prompt)
		if kind == "prompt" && prompt == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"detail": "Prompt obrigatório para automação de IA"})
			return
		}
		name := clipRunes(body.Name, 120)
		if name == "" {
			if prompt != "" {
				name = clipRunes(prompt, 56)
			} else {
				name = "Checagem do Sistema"
			}
		}

		model := strings.TrimSpace(body.Model)
		if model == "" {
			model = "gemini-4-hydrogen-high"
		}
		perm := strings.TrimSpace(body.PermissionMode)
		if _, ok := jobs.PermissionModes[perm]; !ok {
			perm = "readonly"
		}
		notifyOn := strings.ToLower(strings.TrimSpace(body.NotifyOn))
		if notifyOn != "error_only" {
			notifyOn = "always"
		}

		notifyNtfy := body.NotifyNtfy == nil || *body.NotifyNtfy
		notifyPush := body.NotifyPush == nil || *body.NotifyPush
		enabled := body.Enabled == nil || *body.Enabled

		nextRunAt := ""
		if enabled {
			if next, ok := cronSched.NextAfter(time.Now()); ok {
				nextRunAt = next.Format(time.RFC3339)
			}
		}

		item := db.Automation{
			ID:             "auto-" + strings.ReplaceAll(uuid.NewString(), "-", "")[:10],
			Name:           name,
			CronExpr:       strings.TrimSpace(body.CronExpr),
			Kind:           kind,
			Prompt:         prompt,
			Model:          model,
			PermissionMode: perm,
			NotifyNtfy:     notifyNtfy,
			NotifyPush:     notifyPush,
			NotifyOn:       notifyOn,
			Enabled:        enabled,
			NextRunAt:      nextRunAt,
		}
		created, err := store.CreateAutomation(item)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"detail": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, created)
	})

	mux.HandleFunc("PATCH /api/automations/{id}", func(w http.ResponseWriter, req *http.Request) {
		id := strings.TrimSpace(req.PathValue("id"))
		cur, err := store.GetAutomation(id)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"detail": err.Error()})
			return
		}
		if cur == nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"detail": "Automação não encontrada"})
			return
		}

		var body automationPatchRequest
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"detail": "JSON inválido"})
			return
		}

		if body.Name != nil {
			if clipped := clipRunes(*body.Name, 120); clipped != "" {
				cur.Name = clipped
			}
		}
		if body.CronExpr != nil {
			cur.CronExpr = strings.TrimSpace(*body.CronExpr)
		}
		if body.Kind != nil {
			if strings.ToLower(strings.TrimSpace(*body.Kind)) == "check" {
				cur.Kind = "check"
			} else {
				cur.Kind = "prompt"
			}
		}
		if body.Prompt != nil {
			cur.Prompt = strings.TrimSpace(*body.Prompt)
		}
		if cur.Kind == "prompt" && cur.Prompt == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"detail": "Prompt obrigatório para automação de IA"})
			return
		}
		if body.Model != nil && strings.TrimSpace(*body.Model) != "" {
			cur.Model = strings.TrimSpace(*body.Model)
		}
		if body.PermissionMode != nil {
			if _, ok := jobs.PermissionModes[*body.PermissionMode]; ok {
				cur.PermissionMode = *body.PermissionMode
			}
		}
		if body.NotifyNtfy != nil {
			cur.NotifyNtfy = *body.NotifyNtfy
		}
		if body.NotifyPush != nil {
			cur.NotifyPush = *body.NotifyPush
		}
		if body.NotifyOn != nil {
			if strings.ToLower(strings.TrimSpace(*body.NotifyOn)) == "error_only" {
				cur.NotifyOn = "error_only"
			} else {
				cur.NotifyOn = "always"
			}
		}
		if body.Enabled != nil {
			cur.Enabled = *body.Enabled
		}

		cronSched, err := automation.ParseCron(cur.CronExpr)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"detail": err.Error()})
			return
		}
		if cur.Enabled {
			if next, ok := cronSched.NextAfter(time.Now()); ok {
				cur.NextRunAt = next.Format(time.RFC3339)
			}
		} else {
			cur.NextRunAt = ""
		}

		updated, err := store.UpdateAutomation(*cur)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"detail": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, updated)
	})

	mux.HandleFunc("DELETE /api/automations/{id}", func(w http.ResponseWriter, req *http.Request) {
		deleted, err := store.DeleteAutomation(req.PathValue("id"))
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"detail": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "deleted": deleted})
	})

	mux.HandleFunc("POST /api/automations/{id}/run", func(w http.ResponseWriter, req *http.Request) {
		if sched == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"detail": "Scheduler indisponível"})
			return
		}
		a, err := sched.RunAutomationNow(context.Background(), req.PathValue("id"))
		if err != nil {
			status := http.StatusBadRequest
			if strings.Contains(err.Error(), "não encontrada") {
				status = http.StatusNotFound
			} else if strings.Contains(err.Error(), "execução") {
				status = http.StatusConflict
			}
			writeJSON(w, status, map[string]string{"detail": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "automation": a})
	})

	mux.HandleFunc("GET /api/alerts", func(w http.ResponseWriter, req *http.Request) {
		limit := parseIntQuery(req.URL.Query().Get("limit"), 40, 1, 200)
		alerts, unread, err := store.ListAlertEvents(limit)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"detail": err.Error()})
			return
		}
		acfg, _ := store.GetAlertConfig()
		writeJSON(w, http.StatusOK, map[string]any{
			"alerts":       alerts,
			"unread_count": unread,
			"config":       acfg,
		})
	})

	mux.HandleFunc("GET /api/alerts/stream", func(w http.ResponseWriter, req *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")

		ch, unsub := notifier.Subscribe()
		defer unsub()

		unread, _ := store.CountUnreadAlerts()
		_, _ = fmt.Fprintf(w, "event: init\ndata: {\"unread_count\":%d}\n\n", unread)
		flusher.Flush()

		keepAlive := time.NewTicker(25 * time.Second)
		defer keepAlive.Stop()

		for {
			select {
			case <-req.Context().Done():
				return
			case <-keepAlive.C:
				_, _ = fmt.Fprint(w, ": keepalive\n\n")
				flusher.Flush()
			case ev, open := <-ch:
				if !open {
					return
				}
				raw, err := json.Marshal(ev)
				if err != nil {
					continue
				}
				_, _ = fmt.Fprintf(w, "event: alert\ndata: %s\n\n", raw)
				flusher.Flush()
			}
		}
	})

	mux.HandleFunc("POST /api/alerts/read", func(w http.ResponseWriter, req *http.Request) {
		var body struct {
			ID int64 `json:"id"`
		}
		_ = json.NewDecoder(req.Body).Decode(&body)
		if err := store.MarkAlertsRead(body.ID); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"detail": err.Error()})
			return
		}
		unread, _ := store.CountUnreadAlerts()
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "unread_count": unread})
	})

	mux.HandleFunc("DELETE /api/alerts", func(w http.ResponseWriter, _ *http.Request) {
		if err := store.ClearAlertEvents(); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"detail": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "unread_count": 0})
	})

	mux.HandleFunc("PUT /api/alerts/config", func(w http.ResponseWriter, req *http.Request) {
		var body db.AlertConfig
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"detail": "JSON inválido"})
			return
		}
		saved, err := store.SaveAlertConfig(body)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"detail": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, saved)
	})

	mux.HandleFunc("POST /api/alerts/test", func(w http.ResponseWriter, req *http.Request) {
		res := notifier.Dispatch(req.Context(), automation.AlertInput{
			Source:   "test",
			Severity: "info",
			Priority: "default",
			Title:    "AGY: Teste de Notificação",
			Message:  "Automações agendadas e alertas ativos estão operacionais.",
		})
		writeJSON(w, http.StatusOK, res)
	})

	mux.HandleFunc("GET /api/push/vapid-public-key", func(w http.ResponseWriter, _ *http.Request) {
		keys, err := automation.EnsureVAPIDKeys(store)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"detail": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"public_key": keys.PublicKey,
			"supported":  true,
		})
	})

	mux.HandleFunc("POST /api/push/subscribe", func(w http.ResponseWriter, req *http.Request) {
		var raw json.RawMessage
		if err := json.NewDecoder(req.Body).Decode(&raw); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"detail": "JSON inválido"})
			return
		}
		var sub automation.BrowserPushSubscription
		if json.Unmarshal(raw, &sub) != nil || strings.TrimSpace(sub.Endpoint) == "" || strings.TrimSpace(sub.Keys.P256dh) == "" || strings.TrimSpace(sub.Keys.Auth) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"detail": "Subscription Web Push incompleta"})
			return
		}
		if err := store.UpsertPushSubscription(sub.Endpoint, string(raw)); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"detail": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	})

	mux.HandleFunc("POST /api/push/unsubscribe", func(w http.ResponseWriter, req *http.Request) {
		var body struct {
			Endpoint string `json:"endpoint"`
		}
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil || strings.TrimSpace(body.Endpoint) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"detail": "Endpoint obrigatório"})
			return
		}
		if err := store.DeletePushSubscription(body.Endpoint); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"detail": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	})
}
