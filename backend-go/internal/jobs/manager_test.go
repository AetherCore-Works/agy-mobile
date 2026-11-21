package jobs

import (
	"strings"
	"testing"

	"github.com/Phoenix7290/agy-mobile-gateway/backend-go/internal/config"
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
