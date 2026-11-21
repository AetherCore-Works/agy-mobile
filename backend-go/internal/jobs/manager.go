package jobs

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/Phoenix7290/agy-mobile-gateway/backend-go/internal/config"
	"github.com/Phoenix7290/agy-mobile-gateway/backend-go/internal/db"
	"github.com/Phoenix7290/agy-mobile-gateway/backend-go/internal/runner"
	"github.com/google/uuid"
)

// PermissionModes maps UI modes to AGY CLI permission flags.
var PermissionModes = map[string][]string{
	"readonly":   {"--mode", "plan"},
	"copilot":    {"--mode", "accept-edits"},
	"autonomous": {"--dangerously-skip-permissions"},
}

// CreateRequest mirrors JobCreateRequest in FastAPI main.py.
type CreateRequest struct {
	Prompt         string   `json:"prompt"`
	ConversationID *string  `json:"conversation_id"`
	Model          *string  `json:"model"`
	PermissionMode *string  `json:"permission_mode"`
	AttachedFiles  []string `json:"attached_files"`
}

// JobFinishNotice describes a completed or failed background job for active alerts.
type JobFinishNotice struct {
	JobID          string
	ConversationID string
	Engine         string
	Model          string
	Status         string // "completed" | "error"
	Summary        string
	Elapsed        time.Duration
}

type jobRunMeta struct {
	startedAt time.Time
	engine    string
	model     string
}

// Manager coordinates async job execution, cancellation, and SSE broadcast signals.
type Manager struct {
	cfg    config.GatewaySettings
	store  *db.GatewayDB
	runner runner.Runner

	OnJobFinish func(JobFinishNotice)

	mu        sync.Mutex
	cancels   map[string]context.CancelFunc
	notifiers map[string]chan struct{}
	meta      map[string]jobRunMeta
}

// NewManager creates a Manager wired to config, SQLite store, and execution runner.
func NewManager(cfg config.GatewaySettings, store *db.GatewayDB, r runner.Runner) *Manager {
	return &Manager{
		cfg:       cfg,
		store:     store,
		runner:    r,
		cancels:   map[string]context.CancelFunc{},
		notifiers: map[string]chan struct{}{},
		meta:      map[string]jobRunMeta{},
	}
}

func (m *Manager) notifyChannel(jobID string) chan struct{} {
	m.mu.Lock()
	defer m.mu.Unlock()
	ch, ok := m.notifiers[jobID]
	if !ok {
		ch = make(chan struct{})
		m.notifiers[jobID] = ch
	}
	return ch
}

// WaitSignal returns the current broadcast channel for jobID; when closed, new events or completion occurred.
func (m *Manager) WaitSignal(jobID string) <-chan struct{} {
	return m.notifyChannel(jobID)
}

func (m *Manager) broadcast(jobID string) {
	m.mu.Lock()
	if ch, ok := m.notifiers[jobID]; ok {
		close(ch)
		m.notifiers[jobID] = make(chan struct{})
	}
	m.mu.Unlock()
}

func (m *Manager) emitEvent(jobID, eventType string, payload map[string]any) {
	_, _ = m.store.AppendJobEvent(jobID, eventType, payload)
	m.broadcast(jobID)
}

// RouteAutoModel picks a local Ollama model for light prompts or Cloud Hydrogen for coding/complex prompts.
func (m *Manager) RouteAutoModel(ctx context.Context, prompt string, attached []string) string {
	pLow := strings.ToLower(strings.TrimSpace(prompt))
	if len(attached) > 0 || strings.HasPrefix(pLow, "/") || len([]rune(prompt)) > 350 {
		return "gemini-4-hydrogen-high"
	}

	heavyKeywords := []string{
		"crie", "edite", "alter", "refator", "bug", "erro", "implement", "docker",
		"git ", "python", "svelte", "typescript", "bash", "arquivo", "ssh", "sql",
		"api", "nginx", "cloudflare", "compose", "systemd", "código", "codigo",
	}
	for _, kw := range heavyKeywords {
		if strings.Contains(pLow, kw) {
			return "gemini-4-hydrogen-high"
		}
	}

	if enabled, _ := m.cfg.Mods["ollama"].(bool); enabled {
		localModels := m.runner.ListOllamaModels(ctx)
		ids := map[string]bool{}
		for _, lm := range localModels {
			ids[lm.ID] = true
		}
		for _, pref := range []string{"ollama:qwen3:8b", "ollama:llama3.2:latest", "ollama:qwen3:14b"} {
			if ids[pref] {
				return pref
			}
		}
		if len(localModels) > 0 {
			return localModels[0].ID
		}
	}
	return "gemini-4-hydrogen-high"
}

// buildAgentPrompt appends mobile Inbox/Outbox context to non-slash prompts so the user's prompt stays first for CLI title/preview extraction.
func (m *Manager) buildAgentPrompt(userPrompt string, attachedFiles []string) string {
	trimmed := strings.TrimSpace(userPrompt)
	if strings.HasPrefix(trimmed, "/") && len(attachedFiles) == 0 {
		return trimmed
	}

	var suffix strings.Builder
	if len(attachedFiles) > 0 {
		fmt.Fprintf(
			&suffix,
			"[Arquivos anexados pelo usuário em %s: %s. Inspecione-os usando read_file conforme necessário.]\n",
			m.cfg.Dirs.InboxDir,
			strings.Join(attachedFiles, ", "),
		)
	}
	if m.cfg.Dirs.OutboxDir != "" {
		fmt.Fprintf(
			&suffix,
			"[Contexto AGY Mobile Outbox: Diretório para download no celular: %s. Sempre que o usuário pedir para baixar, exportar, enviar para o outbox ou disponibilizar qualquer arquivo/relatório/zip/script para o celular, salve ou copie o arquivo final obrigatoriamente dentro de %s e cite o nome exato do arquivo na resposta.]\n",
			m.cfg.Dirs.OutboxDir,
			m.cfg.Dirs.OutboxDir,
		)
	}
	ctxBlock := strings.TrimSpace(suffix.String())
	if ctxBlock == "" {
		return trimmed
	}
	if trimmed == "" {
		return ctxBlock
	}
	return trimmed + "\n\n" + ctxBlock
}

func (m *Manager) snapshotOutbox(ctx context.Context) map[string]float64 {
	snap := map[string]float64{}
	files, err := m.runner.ListOutbox(ctx)
	if err != nil {
		return snap
	}
	for _, f := range files {
		snap[f.Name] = f.Mtime
	}
	return snap
}

func (m *Manager) detectNewOutboxFiles(ctx context.Context, before map[string]float64) []string {
	files, err := m.runner.ListOutbox(ctx)
	if err != nil {
		return nil
	}
	var added []string
	for _, f := range files {
		prevMtime, existed := before[f.Name]
		if !existed || f.Mtime > prevMtime+0.001 {
			added = append(added, f.Name)
		}
	}
	return added
}

func (m *Manager) collectAndRecordOutbox(ctx context.Context, convID, jobID, snippet string, before map[string]float64) []string {
	newOutbox := m.detectNewOutboxFiles(ctx, before)
	if len(newOutbox) > 0 && strings.TrimSpace(convID) != "" {
		_ = m.store.RecordConversationArtifacts(convID, jobID, snippet, newOutbox)
	}
	return newOutbox
}

// StartJob validates/persists a new job and launches its background execution goroutine.
func (m *Manager) StartJob(ctx context.Context, req CreateRequest) (map[string]any, error) {
	userPrompt := strings.TrimSpace(req.Prompt)
	agentPrompt := m.buildAgentPrompt(userPrompt, req.AttachedFiles)

	requestedModel := "gemini-4-hydrogen-high"
	if req.Model != nil && strings.TrimSpace(*req.Model) != "" {
		requestedModel = strings.TrimSpace(*req.Model)
	}
	selectedModel := requestedModel
	if requestedModel == "auto" {
		selectedModel = m.RouteAutoModel(ctx, userPrompt, req.AttachedFiles)
	}

	permMode := "copilot"
	if req.PermissionMode != nil {
		if _, ok := PermissionModes[*req.PermissionMode]; ok {
			permMode = *req.PermissionMode
		}
	}

	engine := "agy"
	if strings.HasPrefix(selectedModel, "ollama:") {
		engine = "ollama"
	}

	var convID *string
	if req.ConversationID != nil && strings.TrimSpace(*req.ConversationID) != "" {
		c := strings.TrimSpace(*req.ConversationID)
		convID = &c
	}
	if engine == "ollama" && (convID == nil || !strings.HasPrefix(*convID, "local-")) {
		newID := "local-" + strings.ReplaceAll(uuid.NewString(), "-", "")[:10]
		convID = &newID
	}

	jobID := strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
	if _, err := m.store.CreateJob(jobID, convID, engine, selectedModel, permMode, userPrompt); err != nil {
		return nil, err
	}

	jobCtx, cancel := context.WithCancel(context.Background())
	m.mu.Lock()
	m.cancels[jobID] = cancel
	if _, ok := m.notifiers[jobID]; !ok {
		m.notifiers[jobID] = make(chan struct{})
	}
	m.meta[jobID] = jobRunMeta{
		startedAt: time.Now(),
		engine:    engine,
		model:     selectedModel,
	}
	m.mu.Unlock()

	if engine == "ollama" {
		go m.runOllamaJob(jobCtx, jobID, *convID, userPrompt, agentPrompt, selectedModel)
	} else if m.cfg.Mode == "ide-bridge" {
		go m.runIDEBridgeJob(jobCtx, jobID, convID, agentPrompt, selectedModel, permMode)
	} else {
		go m.runAGYJob(jobCtx, jobID, convID, agentPrompt, selectedModel, permMode)
	}

	return map[string]any{
		"job_id":          jobID,
		"conversation_id": convID,
		"engine":          engine,
		"model":           selectedModel,
		"status":          "running",
	}, nil
}

// CancelJob terminates a running job's subprocess and context.
func (m *Manager) CancelJob(jobID string) string {
	m.mu.Lock()
	cancel, ok := m.cancels[jobID]
	m.mu.Unlock()
	if ok {
		cancel()
		return "cancelling"
	}
	return "not_running"
}

func (m *Manager) cleanupJob(jobID string) {
	m.mu.Lock()
	if cancel, ok := m.cancels[jobID]; ok {
		cancel()
		delete(m.cancels, jobID)
	}
	delete(m.meta, jobID)
	m.mu.Unlock()
	m.broadcast(jobID)
}

func (m *Manager) runOllamaJob(ctx context.Context, jobID, convID, userPrompt, agentPrompt, selectedModel string) {
	defer m.cleanupJob(jobID)

	ollamaModel := strings.TrimPrefix(selectedModel, "ollama:")
	title := runner.ExtractConversationTitle(userPrompt, "Conversa Local", 60)
	_ = m.store.EnsureLocalConversation(convID, title, selectedModel)
	prior, _ := m.store.GetLocalMessages(convID)
	_ = m.store.AppendLocalMessage(convID, "user", userPrompt)

	m.emitEvent(jobID, "init", map[string]any{"type": "init", "conversation_id": convID, "model": selectedModel})

	msgs := make([]map[string]string, 0, len(prior)+1)
	for _, p := range prior {
		msgs = append(msgs, map[string]string{"role": p.Role, "content": p.Content})
	}
	msgs = append(msgs, map[string]string{"role": "user", "content": agentPrompt})

	payload, _ := json.Marshal(map[string]any{
		"model":    ollamaModel,
		"messages": msgs,
		"stream":   true,
	})

	stream, closeFn, err := m.runner.OpenOllamaChat(ctx, payload)
	if err != nil {
		m.failJob(jobID, convID, ctx, err.Error())
		return
	}
	defer func() { _ = closeFn() }()

	var accumulated strings.Builder
	scanner := bufio.NewScanner(stream)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var chunk struct {
			Error   string `json:"error"`
			Done    bool   `json:"done"`
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		}
		if json.Unmarshal([]byte(line), &chunk) != nil {
			continue
		}
		if chunk.Error != "" {
			m.failJob(jobID, convID, ctx, chunk.Error)
			return
		}
		if chunk.Message.Content != "" {
			accumulated.WriteString(chunk.Message.Content)
			m.emitEvent(jobID, "delta", map[string]any{"type": "delta", "text": chunk.Message.Content})
		}
		if chunk.Done {
			break
		}
	}

	if errors.Is(ctx.Err(), context.Canceled) {
		m.failJob(jobID, convID, ctx, "Execução cancelada pelo usuário.")
		return
	}
	if err := scanner.Err(); err != nil && accumulated.Len() == 0 {
		m.failJob(jobID, convID, ctx, err.Error())
		return
	}

	accStr := accumulated.String()
	if accStr != "" {
		_ = m.store.AppendLocalMessage(convID, "assistant", accStr)
	}
	doneStatus := "completed"
	_ = m.store.UpdateJob(jobID, &convID, &doneStatus, nil)
	m.emitEvent(jobID, "done", map[string]any{
		"type":            "done",
		"conversation_id": convID,
		"status":          "SUCCESS",
		"executed_tools":  []string{},
	})
	preview := accStr
	if len([]rune(preview)) > 200 {
		preview = string([]rune(preview)[:200])
	}
	if preview == "" {
		preview = "Tarefa finalizada."
	}
	m.notifyJobResult(jobID, convID, "completed", "AGY (Local) Concluído", preview, "default")
}

func (m *Manager) runAGYJob(ctx context.Context, jobID string, convID *string, prompt, selectedModel, permMode string) {
	defer m.cleanupJob(jobID)

	beforeOutbox := m.snapshotOutbox(ctx)

	permFlags := PermissionModes[permMode]
	if len(permFlags) == 0 {
		permFlags = PermissionModes["copilot"]
	}
	args := []string{"agy", "-p", prompt, "--output-format", "stream-json"}
	args = append(args, permFlags...)
	if convID != nil && *convID != "" {
		args = append(args, "--conversation", *convID)
	}
	if selectedModel != "" {
		args = append(args, "--model", selectedModel)
	}

	cmd, stdout, stderr, err := m.runner.StartAGYProcess(ctx, args)
	if err != nil {
		m.failJob(jobID, "", ctx, err.Error())
		return
	}

	currentConvID := ""
	if convID != nil {
		currentConvID = *convID
	}
	emittedAny := false
	executedTools := []string{}
	seenTools := map[string]bool{}
	var respPreview strings.Builder

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 64*1024), 8*1024*1024)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var raw map[string]any
		if json.Unmarshal([]byte(line), &raw) != nil {
			continue
		}
		evType, _ := raw["event"].(string)

		switch evType {
		case "init":
			if cid, ok := raw["conversation_id"].(string); ok && cid != "" {
				currentConvID = cid
				_ = m.store.UpdateJob(jobID, &currentConvID, nil, nil)
			}
			m.emitEvent(jobID, "init", map[string]any{
				"type":            "init",
				"conversation_id": nullableStr(currentConvID),
				"model":           selectedModel,
			})

		case "step_update":
			step, _ := raw["step_update"].(map[string]any)
			stepType, _ := step["step_type"].(string)
			if stepType == "tool" {
				toolName, _ := step["tool_name"].(string)
				if toolName == "" {
					toolName = "tool"
				}
				toolState, _ := step["state"].(string)
				if toolState == "" {
					toolState = "ACTIVE"
				}
				cmdPreview := extractToolPreview(step)
				if toolState == "ACTIVE" && !seenTools[toolName] {
					seenTools[toolName] = true
					executedTools = append(executedTools, toolName)
				}
				m.emitEvent(jobID, "tool", map[string]any{
					"type":    "tool",
					"name":    toolName,
					"state":   toolState,
					"preview": truncateStr(cmdPreview, 80),
				})
				if toolName == "ask_permission" || toolName == "ask_custom_permission" || toolState == "WAITING" {
					m.emitEvent(jobID, "approval_needed", map[string]any{
						"type":    "approval_needed",
						"tool":    toolName,
						"command": cmdPreview,
					})
				}
			} else if stepType == "agent_response" {
				if delta, ok := step["text_delta"].(string); ok && delta != "" {
					emittedAny = true
					if respPreview.Len() < 320 {
						respPreview.WriteString(delta)
					}
					m.emitEvent(jobID, "delta", map[string]any{"type": "delta", "text": delta})
				}
			}

		case "command_result":
			cmdObj, _ := raw["command"].(map[string]any)
			cmdName, _ := cmdObj["name"].(string)
			cmdData, _ := cmdObj["data"].(map[string]any)
			if cmdName == "usage" && cmdData != nil {
				mdTable := formatUsageTable(cmdData)
				emittedAny = true
				m.emitEvent(jobID, "delta", map[string]any{"type": "delta", "text": mdTable})
			} else if cmdName == "compact" {
				emittedAny = true
				m.emitEvent(jobID, "delta", map[string]any{
					"type": "delta",
					"text": "**Contexto compactado (`/compact`) com sucesso.** O histórico foi resumido para economizar tokens.",
				})
			}

		case "result":
			res, _ := raw["result"].(map[string]any)
			if cid, ok := res["conversation_id"].(string); ok && cid != "" {
				currentConvID = cid
			}
			finalResp, _ := res["response"].(string)
			finalResp = strings.TrimSpace(finalResp)
			if !emittedAny {
				if finalResp != "" {
					emittedAny = true
					if respPreview.Len() == 0 {
						respPreview.WriteString(finalResp)
					}
					m.emitEvent(jobID, "delta", map[string]any{"type": "delta", "text": finalResp})
				} else {
					emittedAny = true
					m.emitEvent(jobID, "delta", map[string]any{"type": "delta", "text": formatToolsSummary(executedTools)})
				}
			}
			resStatus, _ := res["status"].(string)
			if resStatus == "" {
				resStatus = "SUCCESS"
			}
			snippet := finalResp
			if snippet == "" {
				snippet = respPreview.String()
			}
			newOutbox := m.collectAndRecordOutbox(ctx, currentConvID, jobID, snippet, beforeOutbox)
			m.emitEvent(jobID, "done", map[string]any{
				"type":            "done",
				"conversation_id": nullableStr(currentConvID),
				"status":          resStatus,
				"executed_tools":  executedTools,
				"response":        finalResp,
				"outbox_files":    newOutbox,
			})
			doneStatus := "completed"
			_ = m.store.UpdateJob(jobID, strPtrOrNil(currentConvID), &doneStatus, nil)
		}
	}

	stderrBytes, _ := io.ReadAll(io.LimitReader(stderr, 4096))
	waitErr := cmd.Wait()

	if errors.Is(ctx.Err(), context.Canceled) {
		m.failJob(jobID, currentConvID, ctx, "Execução cancelada pelo usuário.")
		return
	}

	jobRecord, _ := m.store.GetJob(jobID)
	if waitErr != nil && !emittedAny {
		errMsg := strings.TrimSpace(string(stderrBytes))
		if errMsg == "" {
			errMsg = fmt.Sprintf("Processo saiu com erro (%v)", waitErr)
		}
		m.failJob(jobID, currentConvID, ctx, errMsg)
		return
	}
	if !emittedAny {
		summary := formatToolsSummary(executedTools)
		m.emitEvent(jobID, "delta", map[string]any{"type": "delta", "text": summary})
		newOutbox := m.collectAndRecordOutbox(ctx, currentConvID, jobID, summary, beforeOutbox)
		m.emitEvent(jobID, "done", map[string]any{
			"type":            "done",
			"conversation_id": nullableStr(currentConvID),
			"status":          "SUCCESS",
			"executed_tools":  executedTools,
			"outbox_files":    newOutbox,
		})
		doneStatus := "completed"
		_ = m.store.UpdateJob(jobID, strPtrOrNil(currentConvID), &doneStatus, nil)
	} else if jobRecord != nil && jobRecord.Status == "running" {
		newOutbox := m.collectAndRecordOutbox(ctx, currentConvID, jobID, respPreview.String(), beforeOutbox)
		m.emitEvent(jobID, "done", map[string]any{
			"type":            "done",
			"conversation_id": nullableStr(currentConvID),
			"status":          "SUCCESS",
			"executed_tools":  executedTools,
			"outbox_files":    newOutbox,
		})
		doneStatus := "completed"
		_ = m.store.UpdateJob(jobID, strPtrOrNil(currentConvID), &doneStatus, nil)
	}

	m.notifyJobResult(jobID, currentConvID, "completed", "AGY Concluído", fmt.Sprintf("Sessão finalizada (%s).", selectedModel), "default")
}

func (m *Manager) failJob(jobID, convID string, ctx context.Context, msg string) {
	if errors.Is(ctx.Err(), context.Canceled) {
		msg = "Execução cancelada pelo usuário."
	}
	errStatus := "error"
	_ = m.store.UpdateJob(jobID, strPtrOrNil(convID), &errStatus, &msg)
	m.emitEvent(jobID, "error", map[string]any{"type": "error", "error": msg})
	if !errors.Is(ctx.Err(), context.Canceled) {
		m.notifyJobResult(jobID, convID, "error", "AGY Erro", msg, "high")
	}
}

func (m *Manager) notifyJobResult(jobID, convID, status, ntfyTitle, summary, ntfyPriority string) {
	m.mu.Lock()
	meta, ok := m.meta[jobID]
	delete(m.meta, jobID)
	cb := m.OnJobFinish
	m.mu.Unlock()

	if cb != nil {
		var elapsed time.Duration
		if ok && !meta.startedAt.IsZero() {
			elapsed = time.Since(meta.startedAt)
		}
		go cb(JobFinishNotice{
			JobID:          jobID,
			ConversationID: convID,
			Engine:         meta.engine,
			Model:          meta.model,
			Status:         status,
			Summary:        summary,
			Elapsed:        elapsed,
		})
		return
	}
	go m.sendNtfy(ntfyTitle, summary, convID, ntfyPriority)
}

func (m *Manager) sendNtfy(title, message, convID, priority string) {
	if enabled, _ := m.cfg.Mods["ntfy"].(bool); !enabled {
		return
	}
	topic := strings.TrimSpace(os.Getenv("NTFY_TOPIC"))
	if topic == "" {
		return
	}
	baseURL := strings.TrimRight(os.Getenv("NTFY_URL"), "/")
	if baseURL == "" {
		baseURL = "https://ntfy.sh"
	}
	publicURL := strings.TrimRight(os.Getenv("PUBLIC_URL"), "/")

	if len(message) > 500 {
		message = message[:500]
	}
	req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/%s", baseURL, topic), strings.NewReader(message))
	if err != nil {
		return
	}
	req.Header.Set("Title", title)
	req.Header.Set("Priority", priority)
	if publicURL != "" && convID != "" {
		req.Header.Set("Click", fmt.Sprintf("%s/?conv=%s", publicURL, convID))
	}
	client := &http.Client{Timeout: 4 * time.Second}
	resp, err := client.Do(req)
	if err == nil {
		_ = resp.Body.Close()
	}
}

func extractToolPreview(step map[string]any) string {
	info, _ := step["tool_info"].(map[string]any)
	params, _ := info["parameters"].(map[string]any)
	for _, key := range []string{"CommandLine", "Path", "query"} {
		if v, ok := params[key].(string); ok && v != "" {
			return v
		}
	}
	return ""
}

func formatUsageTable(cmdData map[string]any) string {
	lines := []string{
		"### Cota de Modelos AGY\n",
		"| Grupo | Janela | Restante | Reset |",
		"| :--- | :--- | :--- | :--- |",
	}
	groups, _ := cmdData["groups"].([]any)
	for _, gRaw := range groups {
		grp, _ := gRaw.(map[string]any)
		gName, _ := grp["name"].(string)
		if gName == "" {
			gName = "-"
		}
		buckets, _ := grp["buckets"].([]any)
		for _, bRaw := range buckets {
			b, _ := bRaw.(map[string]any)
			rem, _ := b["remaining_fraction"].(float64)
			pct := math.Round(rem * 100)
			win, _ := b["window"].(string)
			if win == "" {
				win = "-"
			}
			rst, _ := b["reset_time"].(string)
			rst = strings.ReplaceAll(strings.ReplaceAll(rst, "T", " "), "Z", " UTC")
			lines = append(lines, fmt.Sprintf("| %s | %s | **%.0f%%** | `%s` |", gName, win, pct, rst))
		}
	}
	return strings.Join(lines, "\n")
}

func formatToolsSummary(tools []string) string {
	if len(tools) == 0 {
		return "*Instrução concluída com sucesso no host.*"
	}
	var b strings.Builder
	b.WriteString("*Instruções executadas no host com sucesso.*\n\n**Ações executadas:**\n")
	for _, t := range tools {
		b.WriteString("- `" + t + "`\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func truncateStr(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen])
}

func strPtrOrNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func nullableStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}
