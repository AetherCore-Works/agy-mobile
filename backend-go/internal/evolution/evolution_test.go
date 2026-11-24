package evolution

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Phoenix7290/agy-mobile-gateway/backend-go/internal/automation"
	"github.com/Phoenix7290/agy-mobile-gateway/backend-go/internal/config"
	"github.com/Phoenix7290/agy-mobile-gateway/backend-go/internal/db"
	"github.com/Phoenix7290/agy-mobile-gateway/backend-go/internal/jobs"
	"github.com/Phoenix7290/agy-mobile-gateway/backend-go/internal/runner"
)

func TestEvolutionCycleOllamaAIMemoryAndOneClickApproval(t *testing.T) {
	tmpDir := t.TempDir()

	var (
		mu             sync.Mutex
		calledMCPTools []string
	)

	mcpSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID     any            `json:"id"`
			Method string         `json:"method"`
			Params map[string]any `json:"params"`
		}
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &req)

		w.Header().Set("Content-Type", "application/json")
		switch req.Method {
		case "initialize":
			w.Header().Set("Mcp-Session-Id", "sess-test-1")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      req.ID,
				"result":  map[string]any{"protocolVersion": "2024-11-05"},
			})
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case "tools/call":
			toolName, _ := req.Params["name"].(string)
			mu.Lock()
			calledMCPTools = append(calledMCPTools, toolName)
			mu.Unlock()
			w.Header().Set("Content-Type", "text/event-stream")
			payload, _ := json.Marshal(map[string]any{
				"jsonrpc": "2.0",
				"id":      req.ID,
				"result": map[string]any{
					"content": []map[string]any{
						{"type": "text", "text": "Consolidação ok para " + toolName},
					},
				},
			})
			_, _ = w.Write([]byte("event: message\ndata: " + string(payload) + "\n\n"))
		}
	}))
	defer mcpSrv.Close()

	ollamaSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tags":
			_, _ = io.WriteString(w, `{"models":[{"name":"qwen3:8b"}]}`)
		case "/api/chat":
			respJSON := `<think>analisando telemetria...</think>` + "\n```json\n" + `{
				"summary": "Detectado pedido recorrente de status docker/disco e correção sobre caminhos no Windows.",
				"proposals": [
					{
						"kind": "fast_path_slash",
						"title": "Atalho Fast-Path /infra_check",
						"rationale": "Pedido repetido 3 vezes na janela recente.",
						"command": "/infra_check",
						"description": "Checar Docker e disco rapidamente",
						"prompt_template": "Verifique containers docker e espaco em disco {{input}}",
						"model": "ollama:qwen3:8b",
						"auto_submit": true
					},
					{
						"kind": "prompt_rule",
						"title": "Confirmar caminhos absolutos no Windows",
						"rationale": "Correção do usuário após caminho incorreto.",
						"rule_content": "No Windows, verifique sempre caminhos absolutos existentes antes de executar scripts."
					}
				]
			}` + "\n```"
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"done": true,
				"message": map[string]string{
					"content": respJSON,
				},
			})
		}
	}))
	defer ollamaSrv.Close()

	t.Setenv("GATEWAY_MODE", "standalone")
	t.Setenv("DATA_DIR", tmpDir)
	t.Setenv("OLLAMA_URL", ollamaSrv.URL)
	t.Setenv("AI_MEMORY_SERVER_URL", mcpSrv.URL)
	t.Setenv("ENABLE_EVOLUTION", "true")
	t.Setenv("EVOLUTION_FASTPATH_MIN_HITS", "3")

	cfg := config.Load()
	store, err := db.Open(filepath.Join(tmpDir, "gateway.db"))
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	defer store.Close()

	r := runner.Create(cfg)
	notifier := automation.NewNotifier(store)
	svc := NewService(cfg, store, r, notifier)

	// Record telemetry events: 3 identical prompts, 1 slash usage, 1 error, 1 correction.
	for i := 0; i < 3; i++ {
		if err := store.RecordTelemetryEvent(db.TelemetryEvent{
			Kind:          "job_ok",
			PromptPreview: "Verifique containers docker e espaco em disco",
			Model:         "ollama:qwen3:8b",
			Engine:        "ollama",
			DurationMs:    320,
		}); err != nil {
			t.Fatal(err)
		}
	}
	_ = store.RecordTelemetryEvent(db.TelemetryEvent{
		Kind:         "slash_used",
		SlashCommand: "/publish",
		Model:        "gemini-4-hydrogen-high",
		Engine:       "agy",
	})
	_ = store.RecordTelemetryEvent(db.TelemetryEvent{
		Kind:          "job_error",
		PromptPreview: "Executar deploy no Windows sk-secret1234567890abcdef",
		ErrorMessage:  "caminho não encontrado",
		Model:         "gemini-4-hydrogen-high",
		Engine:        "agy",
	})
	_ = store.RecordTelemetryEvent(db.TelemetryEvent{
		Kind:           "user_correction",
		PromptPreview:  "Executar deploy no Windows",
		CorrectionNote: "não era isso, use o caminho em c:\\AGY-PROJECT",
		Model:          "gemini-4-hydrogen-high",
		Engine:         "agy",
	})

	outcome, err := svc.RunCycle(context.Background(), "cron", false)
	if err != nil {
		t.Fatalf("RunCycle failed: %v", err)
	}
	if outcome.Skipped || outcome.Cycle == nil {
		t.Fatalf("expected active evolution cycle, got %+v", outcome)
	}
	if outcome.Cycle.EventsProcessed != 6 {
		t.Fatalf("expected 6 events processed, got %d", outcome.Cycle.EventsProcessed)
	}
	if outcome.Cycle.AIMemoryStatus != "ok" {
		t.Fatalf("expected ai-memory status ok, got %q", outcome.Cycle.AIMemoryStatus)
	}
	if outcome.Cycle.AnalyzerBackend != "ollama:qwen3:8b" {
		t.Fatalf("expected analyzer backend ollama:qwen3:8b, got %q", outcome.Cycle.AnalyzerBackend)
	}
	if len(outcome.Proposals) < 2 {
		t.Fatalf("expected >= 2 proposals, got %d", len(outcome.Proposals))
	}

	// Ensure alerts captured evolution report notification.
	alerts, unread, err := store.ListAlertEvents(10)
	if err != nil || unread == 0 || len(alerts) == 0 || alerts[0].Source != "evolution" {
		t.Fatalf("expected evolution alert in DB, got alerts=%+v unread=%d err=%v", alerts, unread, err)
	}

	var slashPropID, rulePropID string
	for _, p := range outcome.Proposals {
		if p.Kind == "fast_path_slash" && p.Payload["command"] == "/infra_check" {
			slashPropID = p.ID
		} else if p.Kind == "prompt_rule" {
			rulePropID = p.ID
		}
	}
	if slashPropID == "" || rulePropID == "" {
		t.Fatalf("expected both fast_path_slash and prompt_rule proposals, got %+v", outcome.Proposals)
	}

	// 1-click approve Fast-Path -> inserts directly into slash_commands.
	slashRes, err := svc.ApproveProposal(context.Background(), slashPropID)
	if err != nil || slashRes.SlashCommand == nil || slashRes.SlashCommand.Command != "/infra_check" {
		t.Fatalf("ApproveProposal fast_path_slash failed: res=%+v err=%v", slashRes, err)
	}
	cmds, _ := store.ListSlashCommands()
	foundCmd := false
	for _, sc := range cmds {
		if sc.Command == "/infra_check" {
			foundCmd = true
			break
		}
	}
	if !foundCmd {
		t.Fatalf("expected /infra_check in slash_commands table after 1-click approve")
	}

	// 1-click approve Rule -> inserts into aegys_rules + syncs to ai-memory.
	ruleRes, err := svc.ApproveProposal(context.Background(), rulePropID)
	if err != nil || ruleRes.Rule == nil || !ruleRes.Rule.Enabled {
		t.Fatalf("ApproveProposal prompt_rule failed: res=%+v err=%v", ruleRes, err)
	}
	time.Sleep(150 * time.Millisecond)

	mu.Lock()
	calledSnapshot := append([]string{}, calledMCPTools...)
	mu.Unlock()
	if len(calledSnapshot) < 2 || calledSnapshot[0] != "memory_consolidate" || calledSnapshot[1] != "memory_auto_improve" {
		t.Fatalf("expected memory_consolidate and memory_auto_improve MCP calls, got %+v", calledSnapshot)
	}

	// Next cycle without force skips when no new events occurred; forced cycle does not duplicate fingerprints.
	skipOut, err := svc.RunCycle(context.Background(), "cron", false)
	if err != nil || !skipOut.Skipped {
		t.Fatalf("expected second idle cycle to be skipped, got %+v err=%v", skipOut, err)
	}

	_ = store.RecordTelemetryEvent(db.TelemetryEvent{Kind: "feedback_up", Model: "ollama:ryan-fast:8b"})
	forceOut, err := svc.RunCycle(context.Background(), "manual", true)
	if err != nil {
		t.Fatalf("forced cycle failed: %v", err)
	}
	for _, p := range forceOut.Proposals {
		if p.Kind == "fast_path_slash" && p.Payload["command"] == "/infra_check" {
			t.Fatalf("expected already-approved proposal fingerprint not to duplicate: %+v", p)
		}
	}
}

func TestTelemetrySanitizationAndCorrectionHeuristic(t *testing.T) {
	sanitized := db.SanitizeTelemetryText("use bearer abcdef1234567890 and api_key=mysecretvalue123 or sk-proj1234567890abcd")
	if strings.Contains(sanitized, "abcdef1234567890") || strings.Contains(sanitized, "mysecretvalue123") || strings.Contains(sanitized, "sk-proj1234567890abcd") {
		t.Fatalf("expected secrets redacted from telemetry text, got %q", sanitized)
	}
	if !jobs.DetectUserCorrection("Não era isso, você errou o diretório") {
		t.Fatalf("expected DetectUserCorrection to detect PT correction phrase")
	}
	if jobs.DetectUserCorrection("/diff") {
		t.Fatalf("did not expect slash command to match user correction")
	}
}
