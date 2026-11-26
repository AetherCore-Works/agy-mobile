package jobs

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Phoenix7290/agy-mobile-gateway/backend-go/internal/config"
	"github.com/Phoenix7290/agy-mobile-gateway/backend-go/internal/db"
	"github.com/Phoenix7290/agy-mobile-gateway/backend-go/internal/runner"
)

func TestBuildAgentPromptPlacesUserPromptFirst(t *testing.T) {
	m := &Manager{
		cfg: config.GatewaySettings{
			Dirs: config.AppDirs{
				InboxDir:  "/home/ai/agy-inbox",
				OutboxDir: "/home/ai/agy-outbox",
			},
		},
	}

	got := m.buildAgentPrompt("Verificar temperaturas do servidor", nil)
	if !strings.HasPrefix(got, "Verificar temperaturas do servidor") {
		t.Fatalf("expected user prompt first, got: %q", got)
	}
	if !strings.Contains(got, "[Contexto AGY Mobile Outbox:") {
		t.Fatalf("expected outbox context suffix, got: %q", got)
	}
	if title := runner.ExtractConversationTitle(got, "Conversa", 55); title != "Verificar temperaturas do servidor" {
		t.Fatalf("unexpected extracted title: %q", title)
	}

	withAttach := m.buildAgentPrompt("Analise este log", []string{"/home/ai/agy-inbox/app.log"})
	if !strings.HasPrefix(withAttach, "Analise este log") {
		t.Fatalf("expected user prompt before attachment note, got: %q", withAttach)
	}

	attachOnly := m.buildAgentPrompt("   ", []string{"/home/ai/agy-inbox/foto.png"})
	if !strings.HasPrefix(attachOnly, "[Arquivos anexados pelo usuário em ") {
		t.Fatalf("expected attachment note first when prompt is empty, got: %q", attachOnly)
	}

	slash := m.buildAgentPrompt("/usage", nil)
	if slash != "/usage" {
		t.Fatalf("expected unmodified slash command, got: %q", slash)
	}
}

func TestLearnedRuleInjectionAndTelemetryRecording(t *testing.T) {
	ollamaSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tags":
			_, _ = io.WriteString(w, `{"models":[{"name":"qwen3:8b"}]}`)
		case "/api/chat":
			w.Header().Set("Content-Type", "application/x-ndjson")
			_, _ = io.WriteString(w, "{\"message\":{\"content\":\"ok\"},\"done\":true}\n")
		}
	}))
	defer ollamaSrv.Close()

	tmpDir := t.TempDir()
	t.Setenv("GATEWAY_MODE", "standalone")
	t.Setenv("DATA_DIR", tmpDir)
	t.Setenv("OLLAMA_URL", ollamaSrv.URL)

	cfg := config.Load()
	store, err := db.Open(filepath.Join(tmpDir, "gateway.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	if _, err := store.CreateAegysRule(db.AegysRule{
		ID:      "rule-test-1",
		Kind:    "preference",
		Title:   "Respostas objetivas",
		Content: "Responda com exemplos práticos em tópicos curtos.",
		Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}

	mgr := NewManager(cfg, store, runner.Create(cfg))
	prompt := mgr.buildAgentPrompt("Verifique docker ps", nil)
	if !strings.HasPrefix(prompt, "Verifique docker ps") {
		t.Fatalf("expected user prompt first even with learned rule suffix, got %q", prompt)
	}
	if !strings.Contains(prompt, "Preferências e Regras Aprendidas (Aegys)") || !strings.Contains(prompt, "tópicos curtos") {
		t.Fatalf("expected learned Aegys rule injected into agent prompt, got %q", prompt)
	}

	slash := "/review"
	model := "ollama:qwen3:8b"
	started, err := mgr.StartJob(context.Background(), CreateRequest{
		Prompt:       "Faça review de segurança",
		Model:        &model,
		SlashCommand: &slash,
	})
	if err != nil {
		t.Fatalf("StartJob failed: %v", err)
	}
	convID, _ := started["conversation_id"].(*string)
	if convID == nil {
		t.Fatalf("expected local conversation id, got %+v", started)
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		stats, _ := store.GetTelemetryStats(5, 2)
		if stats.TotalJobsOK >= 1 && len(stats.TopCommands) >= 1 {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}

	if _, err := mgr.StartJob(context.Background(), CreateRequest{
		Prompt:         "Não era isso, corrija o comando anterior",
		ConversationID: convID,
		Model:          &model,
	}); err != nil {
		t.Fatal(err)
	}

	time.Sleep(100 * time.Millisecond)
	stats, err := store.GetTelemetryStats(5, 2)
	if err != nil {
		t.Fatal(err)
	}
	if stats.TotalJobsOK < 1 || stats.TotalCorrections < 1 || len(stats.TopCommands) == 0 || stats.TopCommands[0].Name != "/review" {
		t.Fatalf("unexpected telemetry stats: %+v", stats)
	}
}

func TestIsQuotaExceededErrorAndEventCode(t *testing.T) {
	if !IsQuotaExceededError("HTTP 429 RESOURCE_EXHAUSTED: 5-hour quota exceeded") {
		t.Fatal("expected 429 RESOURCE_EXHAUSTED to be detected")
	}
	if IsQuotaExceededError("connection refused") {
		t.Fatal("did not expect connection refused to be classified as quota_exceeded")
	}

	store, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	mgr := NewManager(config.GatewaySettings{Mode: "gateway"}, store, nil)
	jobID := "job-quota-1"
	if _, err := store.CreateJob(jobID, nil, "test", "auto", "agy", "gemini-3-flash"); err != nil {
		t.Fatal(err)
	}
	mgr.failJob(jobID, "", context.Background(), "429 RESOURCE_EXHAUSTED: rate limit exceeded")

	events, err := store.GetJobEventsAfter(jobID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].EventType != "error" {
		t.Fatalf("expected 1 error event, got %+v", events)
	}
	if code, _ := events[0].Payload["error_code"].(string); code != "quota_exceeded" {
		t.Fatalf("expected error_code=quota_exceeded, got %+v", events[0].Payload)
	}
}

func TestRecordToolTimelineStep(t *testing.T) {
	var timeline []ToolTimelineEntry
	starts := map[string]time.Time{}

	recordToolTimelineStep(&timeline, starts, "run_command", "git status", "ACTIVE")
	if len(timeline) != 1 || timeline[0].State != "ACTIVE" {
		t.Fatalf("expected 1 ACTIVE entry, got %+v", timeline)
	}
	time.Sleep(5 * time.Millisecond)
	recordToolTimelineStep(&timeline, starts, "run_command", "git status", "DONE")
	if len(timeline) != 1 || timeline[0].State != "DONE" || timeline[0].DurationMs < 1 {
		t.Fatalf("expected updated DONE entry with duration, got %+v", timeline)
	}
}
