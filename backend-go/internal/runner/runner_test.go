package runner

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Phoenix7290/agy-mobile-gateway/backend-go/internal/config"
)

func TestCleanUserRequestAndTitleExtraction(t *testing.T) {
	raw := "<USER_REQUEST>\n[Arquivos anexados pelo usuário em /home/ai/agy-inbox: /home/ai/agy-inbox/IMG_4909.webp. Inspecione-os usando read_file conforme necessário.]\n\nVocê consegue ver essa foto?\n</USER_REQUEST>"
	got := ExtractConversationTitle(raw, "Conversa", 55)
	if got != "Você consegue ver essa foto?" {
		t.Fatalf("unexpected title: %q", got)
	}

	attachOnly := "<USER_REQUEST>\n[Arquivos anexados pelo usuário em /home/ai/agy-inbox: /home/ai/agy-inbox/foto1.png. Inspecione-os usando read_file conforme necessário.]\n</USER_REQUEST>"
	got2 := ExtractConversationTitle(attachOnly, "Conversa", 55)
	if got2 != "Anexo: foto1.png" {
		t.Fatalf("unexpected attach-only title: %q", got2)
	}

	longPrompt := "Gostaria de melhorar ainda mais. Bem, seria possível nós fazermos isso agora?"
	got3 := ExtractConversationTitle(longPrompt, "Conversa", 55)
	if got3 != "Gostaria de melhorar ainda mais. Bem, seria possível…" {
		t.Fatalf("unexpected word-boundary title: %q", got3)
	}
}

func TestLocalRunnerBrainScan(t *testing.T) {
	tmp := t.TempDir()
	brain := filepath.Join(tmp, "brain")
	logsDir := filepath.Join(brain, "conv-abc-123", ".system_generated", "logs")
	if err := os.MkdirAll(logsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	transcript := `{"type":"USER_INPUT","content":"<USER_REQUEST>\nTestando histórico no Go\n</USER_REQUEST>"}
{"type":"PLANNER_RESPONSE","content":"Tudo funcionando!"}
`
	if err := os.WriteFile(filepath.Join(logsDir, "transcript.jsonl"), []byte(transcript), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("AGY_BRAIN_DIR", brain)
	t.Setenv("INBOX_DIR", filepath.Join(tmp, "inbox"))
	t.Setenv("OUTBOX_DIR", filepath.Join(tmp, "outbox"))
	cfg := config.Load()
	r := NewLocalRunner(cfg)

	items, err := r.ListAGYConversations(context.Background(), 10)
	if err != nil || len(items) != 1 {
		t.Fatalf("ListAGYConversations got len=%d err=%v", len(items), err)
	}
	if items[0].Title != "Testando histórico no Go" || items[0].MessageCount != 2 {
		t.Fatalf("unexpected item: %+v", items[0])
	}

	detail, err := r.GetAGYConversation(context.Background(), "conv-abc-123")
	if err != nil || detail == nil || len(detail.Messages) != 2 {
		t.Fatalf("GetAGYConversation failed: %+v err=%v", detail, err)
	}
	if detail.Messages[0].Content != "Testando histórico no Go" {
		t.Fatalf("expected cleaned user message, got %q", detail.Messages[0].Content)
	}

	if _, err := r.DeleteAGYConversation(context.Background(), "../escape"); err == nil {
		t.Fatal("expected error for traversal conversation id")
	}
	deleted, err := r.DeleteAGYConversation(context.Background(), "conv-abc-123")
	if err != nil || !deleted {
		t.Fatalf("DeleteAGYConversation deleted=%v err=%v", deleted, err)
	}
	backups, err := filepath.Glob(filepath.Join(tmp, "brain-backups", "conv-abc-123_*.tar.gz"))
	if err != nil || len(backups) != 1 {
		t.Fatalf("expected 1 backup tar.gz before delete, got %v err=%v", backups, err)
	}
	itemsAfter, err := r.ListAGYConversations(context.Background(), 10)
	if err != nil || len(itemsAfter) != 0 {
		t.Fatalf("expected 0 conversations after delete, got len=%d err=%v", len(itemsAfter), err)
	}
	deletedAgain, err := r.DeleteAGYConversation(context.Background(), "conv-abc-123")
	if err != nil || deletedAgain {
		t.Fatalf("expected deletedAgain=false err=nil, got deleted=%v err=%v", deletedAgain, err)
	}

	if got := cleanHostName("CachyCore.lan\r\n"); got != "CachyCore" {
		t.Fatalf("expected CachyCore, got %q", got)
	}
	if got := r.HostName(context.Background()); got == "" {
		t.Fatal("expected non-empty LocalRunner.HostName")
	}
}
