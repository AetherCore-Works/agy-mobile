package cockpit

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
)

// ServerConfig holds runtime options for the optional Cockpit HTTP API.
type ServerConfig struct {
	AllowPower bool
	AuthToken  string
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeDockerError(w http.ResponseWriter, err error) {
	status := http.StatusBadGateway
	if errors.Is(err, ErrInvalidContainerID) {
		status = http.StatusBadRequest
	}
	writeJSON(w, status, map[string]string{"detail": err.Error()})
}

func authorized(req *http.Request, expectedToken string) bool {
	if expectedToken == "" {
		return true
	}
	hdr := strings.TrimSpace(req.Header.Get("Authorization"))
	if !strings.HasPrefix(hdr, "Bearer ") {
		return false
	}
	got := strings.TrimSpace(strings.TrimPrefix(hdr, "Bearer "))
	return subtle.ConstantTimeCompare([]byte(got), []byte(expectedToken)) == 1
}

// NewHandler builds the HTTP router for the Cockpit optional package.
func NewHandler(cfg ServerConfig, store *SamplerStore, col Collector, pwr PowerController) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":          true,
			"package":     "cockpit",
			"allow_power": cfg.AllowPower,
		})
	})

	guard := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, req *http.Request) {
			if !authorized(req, cfg.AuthToken) {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"detail": "Token Cockpit inválido"})
				return
			}
			next(w, req)
		}
	}

	mux.HandleFunc("GET /overview", guard(func(w http.ResponseWriter, req *http.Request) {
		force := req.URL.Query().Get("refresh") == "1" || req.URL.Query().Get("refresh") == "true"
		overview := store.Overview(req.Context(), force)
		writeJSON(w, http.StatusOK, overview)
	}))

	mux.HandleFunc("POST /docker/{id}/start", guard(func(w http.ResponseWriter, req *http.Request) {
		id := req.PathValue("id")
		out, err := col.DockerAction(req.Context(), id, "start")
		if err != nil {
			writeDockerError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": id, "action": "start", "output": out})
	}))

	mux.HandleFunc("POST /docker/{id}/stop", guard(func(w http.ResponseWriter, req *http.Request) {
		id := req.PathValue("id")
		out, err := col.DockerAction(req.Context(), id, "stop")
		if err != nil {
			writeDockerError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": id, "action": "stop", "output": out})
	}))

	mux.HandleFunc("GET /docker/{id}/logs", guard(func(w http.ResponseWriter, req *http.Request) {
		id := req.PathValue("id")
		tail := 80
		if raw := req.URL.Query().Get("tail"); raw != "" {
			if n, err := strconv.Atoi(raw); err == nil {
				tail = n
			}
		}
		logs, err := col.DockerLogs(req.Context(), id, tail)
		if err != nil {
			writeDockerError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": id, "logs": logs})
	}))

	mux.HandleFunc("POST /power", guard(func(w http.ResponseWriter, req *http.Request) {
		if !cfg.AllowPower {
			writeJSON(w, http.StatusForbidden, map[string]string{"detail": ErrPowerDisabled.Error()})
			return
		}
		var body PowerActionRequest
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"detail": "JSON inválido"})
			return
		}
		action := strings.ToLower(strings.TrimSpace(body.Action))
		expected := ExpectedConfirmWord(action)
		if expected == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"detail": "Ação de energia desconhecida"})
			return
		}
		if strings.ToUpper(strings.TrimSpace(body.Confirm)) != expected {
			writeJSON(w, http.StatusBadRequest, map[string]string{"detail": ErrInvalidPowerConfirm.Error()})
			return
		}
		if err := pwr.Execute(action); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"detail": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "action": action, "scheduled": true})
	}))

	return mux
}
