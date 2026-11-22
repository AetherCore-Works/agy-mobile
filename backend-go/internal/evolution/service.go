package evolution

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/Phoenix7290/agy-mobile-gateway/backend-go/internal/automation"
	"github.com/Phoenix7290/agy-mobile-gateway/backend-go/internal/config"
	"github.com/Phoenix7290/agy-mobile-gateway/backend-go/internal/db"
	"github.com/Phoenix7290/agy-mobile-gateway/backend-go/internal/runner"
	"github.com/google/uuid"
)

var validFastPathSlashRe = regexp.MustCompile(`^/[a-z0-9][a-z0-9_-]{0,31}$`)

var reservedSlashCommands = map[string]bool{
	"/usage":   true,
	"/compact": true,
	"/diff":    true,
	"/mem":     true,
	"/plan":    true,
	"/clear":   true,
	"/status":  true,
	"/evolve":  true,
}

// Report bundles current telemetry, pending/recent proposals, active rules, and recent cycles.
type Report struct {
	Enabled         bool                   `json:"enabled"`
	CronExpr        string                 `json:"cron_expr"`
	OllamaEnabled   bool                   `json:"ollama_enabled"`
	AIMemoryEnabled bool                   `json:"ai_memory_enabled"`
	PendingCount    int                    `json:"pending_count"`
	Stats           db.TelemetryStats      `json:"stats"`
	Proposals       []db.EvolutionProposal `json:"proposals"`
	Rules           []db.AegysRule         `json:"rules"`
	Cycles          []db.EvolutionCycle    `json:"cycles"`
}

// CycleOutcome returns the executed cycle record and newly queued proposals.
type CycleOutcome struct {
	Cycle     *db.EvolutionCycle     `json:"cycle,omitempty"`
	Proposals []db.EvolutionProposal `json:"proposals"`
	Skipped   bool                   `json:"skipped"`
	Reason    string                 `json:"reason,omitempty"`
}

// ApproveResult reports what was persisted when a proposal is approved in 1 click.
type ApproveResult struct {
	Proposal     *db.EvolutionProposal `json:"proposal"`
	SlashCommand *db.SlashCommand      `json:"slash_command,omitempty"`
	Rule         *db.AegysRule         `json:"rule,omitempty"`
}

// Service coordinates periodic off-peak consolidation, AI memory sync, and 1-click approvals.
type Service struct {
	cfg       config.GatewaySettings
	store     *db.GatewayDB
	runner    runner.Runner
	notifier  *automation.Notifier
	aiMem     *AIMemoryClient
	analyzeFn func(ctx context.Context, cfg config.GatewaySettings, r runner.Runner, in AnalysisInput) AnalysisOutput

	mu sync.Mutex
}

// NewService constructs the Aegys self-improvement evolution service.
func NewService(cfg config.GatewaySettings, store *db.GatewayDB, r runner.Runner, notifier *automation.Notifier) *Service {
	return &Service{
		cfg:       cfg,
		store:     store,
		runner:    r,
		notifier:  notifier,
		aiMem:     NewAIMemoryClient(cfg.AIMemoryURL),
		analyzeFn: AnalyzeLocal,
	}
}

// SetAnalyzerForTest overrides the local analysis step cleanly in tests.
func (s *Service) SetAnalyzerForTest(fn func(ctx context.Context, cfg config.GatewaySettings, r runner.Runner, in AnalysisInput) AnalysisOutput) {
	if fn != nil {
		s.analyzeFn = fn
	}
}

// SetAIMemoryURL overrides the MCP endpoint (useful in tests with httptest servers).
func (s *Service) SetAIMemoryURL(url string) {
	s.cfg.AIMemoryURL = strings.TrimRight(strings.TrimSpace(url), "/")
	s.aiMem = NewAIMemoryClient(s.cfg.AIMemoryURL)
}

// RunScheduled executes one off-peak scheduled cycle without forcing empty runs.
func (s *Service) RunScheduled(ctx context.Context) {
	if enabled, _ := s.cfg.Mods["evolution"].(bool); !enabled {
		return
	}
	_, _ = s.RunCycle(ctx, "cron", false)
}

// RunCycle analyzes unconsolidated telemetry, syncs with ai-memory + local ollama-worker, and queues proposals.
func (s *Service) RunCycle(ctx context.Context, trigger string, force bool) (*CycleOutcome, error) {
	if trigger == "" {
		trigger = "manual"
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	events, err := s.store.ListUnconsolidatedTelemetry(400)
	if err != nil {
		return nil, err
	}
	minHits := s.cfg.EvolutionFastPathHits
	if minHits < 2 {
		minHits = 3
	}
	stats, err := s.store.GetTelemetryStats(10, minHits)
	if err != nil {
		return nil, err
	}

	if !force && len(events) == 0 {
		return &CycleOutcome{
			Skipped:   true,
			Reason:    "Nenhum evento novo de telemetria desde o último ciclo.",
			Proposals: []db.EvolutionProposal{},
		}, nil
	}

	cycleID := "evo-" + strings.ReplaceAll(uuid.NewString(), "-", "")[:10]
	cycle := db.EvolutionCycle{
		ID:        cycleID,
		Trigger:   trigger,
		Status:    "running",
		StartedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	if _, err := s.store.CreateEvolutionCycle(cycle); err != nil {
		return nil, err
	}

	slashList, _ := s.store.ListSlashCommands()
	existingCmds := make([]string, 0, len(slashList))
	existingCmdMap := map[string]bool{}
	for _, sc := range slashList {
		low := strings.ToLower(sc.Command)
		existingCmds = append(existingCmds, low)
		existingCmdMap[low] = true
	}

	ruleList, _ := s.store.ListAegysRules()
	existingRules := make([]string, 0, len(ruleList))
	for _, r := range ruleList {
		existingRules = append(existingRules, r.Title+": "+r.Content)
	}

	digest := map[string]any{
		"cycle_id":        cycleID,
		"events_count":    len(events),
		"stats":           stats,
		"existing_cmds":   existingCmds,
		"existing_rules":  existingRules,
		"fast_path_limit": minHits,
	}
	memOutcome := s.aiMem.ConsolidateAndImprove(ctx, digest)

	analysis := s.analyzeFn(ctx, s.cfg, s.runner, AnalysisInput{
		Stats:            stats,
		ExistingCommands: existingCmds,
		ExistingRules:    existingRules,
		AIMemoryInsights: memOutcome.Insights,
		MinFastPathHits:  minHits,
	})

	created := []db.EvolutionProposal{}
	for _, cand := range analysis.Candidates {
		prop, ok := s.buildProposal(cycleID, cand, existingCmdMap)
		if !ok {
			continue
		}
		dup, dupErr := s.store.HasProposalFingerprint(prop.Fingerprint)
		if dupErr == nil && dup {
			continue
		}
		saved, saveErr := s.store.CreateEvolutionProposal(prop)
		if saveErr != nil {
			continue
		}
		created = append(created, *saved)
		if prop.Kind == "fast_path_slash" {
			if cmd, _ := prop.Payload["command"].(string); cmd != "" {
				existingCmdMap[strings.ToLower(cmd)] = true
			}
		}
	}

	var maxEventID int64
	for _, ev := range events {
		if ev.ID > maxEventID {
			maxEventID = ev.ID
		}
	}
	if maxEventID > 0 {
		_ = s.store.MarkTelemetryConsolidated(maxEventID, cycleID)
	}

	status := "completed"
	if len(events) == 0 && len(created) == 0 && !force {
		status = "skipped"
	} else if memOutcome.Configured && !memOutcome.Reachable {
		status = "partial"
	}

	summary := strings.TrimSpace(analysis.Summary)
	if summary == "" {
		summary = fmt.Sprintf("Ciclo concluído via %s (%d eventos, %d novas propostas).", analysis.Backend, len(events), len(created))
	}

	cycle.Status = status
	cycle.EventsProcessed = len(events)
	cycle.ProposalsCreated = len(created)
	cycle.AnalyzerBackend = analysis.Backend
	cycle.AIMemoryStatus = memOutcome.Status
	cycle.Summary = summary
	cycle.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
	_ = s.store.FinishEvolutionCycle(cycle)

	if len(created) > 0 && s.notifier != nil {
		s.notifier.Dispatch(ctx, automation.AlertInput{
			Source:   "evolution",
			Severity: "info",
			Priority: "default",
			Title:    "Aegys: Relatório de Evolução",
			Message:  fmt.Sprintf("%d melhoria(s) sugerida(s) para aprovação em 1 clique no chat.", len(created)),
		})
	}

	return &CycleOutcome{
		Cycle:     &cycle,
		Proposals: created,
	}, nil
}

func (s *Service) buildProposal(cycleID string, cand CandidateProposal, existingCmds map[string]bool) (db.EvolutionProposal, bool) {
	kind := strings.ToLower(strings.TrimSpace(cand.Kind))
	title := strings.TrimSpace(cand.Title)
	rationale := strings.TrimSpace(cand.Rationale)

	switch kind {
	case "fast_path_slash":
		cmd := ensureUniqueSlashCommand(cand.Command, existingCmds)
		tpl := strings.TrimSpace(cand.PromptTemplate)
		if tpl == "" {
			return db.EvolutionProposal{}, false
		}
		if title == "" {
			title = "Criar atalho Fast-Path " + cmd
		}
		desc := strings.TrimSpace(cand.Description)
		if desc == "" {
			desc = clipAnalyzerRunes(tpl, 80)
		}
		normKey := strings.TrimSpace(cand.PromptNormKey)
		if normKey == "" {
			normKey = db.NormalizePromptForGrouping(tpl)
		}
		fp := "fp:" + db.FingerprintString("fast_path_slash", normKey, cmd)
		return db.EvolutionProposal{
			ID:          "prop-" + strings.ReplaceAll(uuid.NewString(), "-", "")[:10],
			CycleID:     cycleID,
			Kind:        "fast_path_slash",
			Fingerprint: fp,
			Title:       title,
			Rationale:   rationale,
			Payload: map[string]any{
				"command":         cmd,
				"description":     desc,
				"prompt_template": tpl,
				"model":           strings.TrimSpace(cand.Model),
				"auto_submit":     cand.AutoSubmit,
			},
			Status: "pending",
		}, true

	case "prompt_rule", "preference":
		content := strings.TrimSpace(cand.RuleContent)
		if content == "" {
			content = rationale
		}
		if title == "" || content == "" {
			return db.EvolutionProposal{}, false
		}
		fp := "fp:" + db.FingerprintString(kind, title, content)
		return db.EvolutionProposal{
			ID:          "prop-" + strings.ReplaceAll(uuid.NewString(), "-", "")[:10],
			CycleID:     cycleID,
			Kind:        kind,
			Fingerprint: fp,
			Title:       title,
			Rationale:   rationale,
			Payload: map[string]any{
				"kind":    kind,
				"title":   title,
				"content": content,
			},
			Status: "pending",
		}, true

	default:
		return db.EvolutionProposal{}, false
	}
}

func ensureUniqueSlashCommand(raw string, existing map[string]bool) string {
	cmd := strings.ToLower(strings.TrimSpace(raw))
	cmd = "/" + strings.TrimPrefix(cmd, "/")
	if !validFastPathSlashRe.MatchString(cmd) {
		cmd = deriveSlashSlug(cmd)
	}
	base := cmd
	for idx := 2; idx <= 20; idx++ {
		if !reservedSlashCommands[cmd] && !existing[cmd] {
			return cmd
		}
		suffix := fmt.Sprintf("_%d", idx)
		trimmedBase := base
		if len(trimmedBase)+len(suffix) > 32 {
			trimmedBase = trimmedBase[:32-len(suffix)]
		}
		cmd = trimmedBase + suffix
	}
	return "/fast_" + strings.ReplaceAll(uuid.NewString(), "-", "")[:6]
}

// ApproveProposal executes the suggested improvement in 1 click inside SQLite and ai-memory.
func (s *Service) ApproveProposal(ctx context.Context, proposalID string) (*ApproveResult, error) {
	prop, err := s.store.GetEvolutionProposal(strings.TrimSpace(proposalID))
	if err != nil {
		return nil, err
	}
	if prop == nil {
		return nil, fmt.Errorf("proposta não encontrada")
	}
	if prop.Status != "pending" {
		return nil, fmt.Errorf("proposta já foi %s", prop.Status)
	}

	switch prop.Kind {
	case "fast_path_slash":
		existingList, _ := s.store.ListSlashCommands()
		existingMap := map[string]bool{}
		for _, sc := range existingList {
			existingMap[strings.ToLower(sc.Command)] = true
		}
		rawCmd, _ := prop.Payload["command"].(string)
		cmdName := ensureUniqueSlashCommand(rawCmd, existingMap)
		desc, _ := prop.Payload["description"].(string)
		tpl, _ := prop.Payload["prompt_template"].(string)
		model, _ := prop.Payload["model"].(string)
		autoSubmit := true
		if b, ok := prop.Payload["auto_submit"].(bool); ok {
			autoSubmit = b
		}
		if strings.TrimSpace(tpl) == "" {
			return nil, fmt.Errorf("template vazio na proposta")
		}

		created, err := s.store.CreateSlashCommand(db.SlashCommand{
			ID:             "sc-" + strings.ReplaceAll(uuid.NewString(), "-", "")[:10],
			Command:        cmdName,
			Description:    desc,
			PromptTemplate: tpl,
			Model:          strings.TrimSpace(model),
			AutoSubmit:     autoSubmit,
			SortOrder:      15,
		})
		if err != nil {
			return nil, err
		}
		updated, err := s.store.UpdateEvolutionProposalDecision(prop.ID, "approved", created.ID)
		if err != nil {
			return nil, err
		}
		return &ApproveResult{Proposal: updated, SlashCommand: created}, nil

	case "prompt_rule", "preference":
		content, _ := prop.Payload["content"].(string)
		title, _ := prop.Payload["title"].(string)
		if strings.TrimSpace(title) == "" {
			title = prop.Title
		}
		if strings.TrimSpace(content) == "" {
			raw, _ := json.Marshal(prop.Payload)
			content = string(raw)
		}
		rule, err := s.store.CreateAegysRule(db.AegysRule{
			ID:               "rule-" + strings.ReplaceAll(uuid.NewString(), "-", "")[:10],
			Kind:             prop.Kind,
			Title:            title,
			Content:          content,
			Enabled:          true,
			SourceProposalID: prop.ID,
		})
		if err != nil {
			return nil, err
		}
		updated, err := s.store.UpdateEvolutionProposalDecision(prop.ID, "approved", rule.ID)
		if err != nil {
			return nil, err
		}
		go func(rCopy db.AegysRule) {
			syncCtx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
			defer cancel()
			_ = s.aiMem.SyncApprovedRule(syncCtx, rCopy)
		}(*rule)
		return &ApproveResult{Proposal: updated, Rule: rule}, nil

	default:
		return nil, fmt.Errorf("tipo de proposta desconhecido: %s", prop.Kind)
	}
}

// RejectProposal marks a proposal rejected so identical suggestions are suppressed next cycle.
func (s *Service) RejectProposal(proposalID string) (*db.EvolutionProposal, error) {
	prop, err := s.store.GetEvolutionProposal(strings.TrimSpace(proposalID))
	if err != nil {
		return nil, err
	}
	if prop == nil {
		return nil, fmt.Errorf("proposta não encontrada")
	}
	if prop.Status != "pending" {
		return nil, fmt.Errorf("proposta já foi %s", prop.Status)
	}
	return s.store.UpdateEvolutionProposalDecision(prop.ID, "rejected", "")
}

// BuildReport assembles telemetry stats, proposals, learned rules, and cycle history for the UI.
func (s *Service) BuildReport() (Report, error) {
	enabled, _ := s.cfg.Mods["evolution"].(bool)
	ollamaEnabled, _ := s.cfg.Mods["ollama"].(bool)
	aiMemEnabled, _ := s.cfg.Mods["ai_memory"].(bool)
	minHits := s.cfg.EvolutionFastPathHits
	if minHits < 2 {
		minHits = 3
	}

	stats, err := s.store.GetTelemetryStats(8, minHits)
	if err != nil {
		return Report{}, err
	}
	proposals, err := s.store.ListEvolutionProposals("", 40)
	if err != nil {
		return Report{}, err
	}
	rules, err := s.store.ListAegysRules()
	if err != nil {
		return Report{}, err
	}
	cycles, err := s.store.ListEvolutionCycles(10)
	if err != nil {
		return Report{}, err
	}
	pending, _ := s.store.CountPendingEvolutionProposals()

	return Report{
		Enabled:         enabled,
		CronExpr:        s.cfg.EvolutionCron,
		OllamaEnabled:   ollamaEnabled,
		AIMemoryEnabled: aiMemEnabled || s.cfg.AIMemoryURL != "",
		PendingCount:    pending,
		Stats:           stats,
		Proposals:       proposals,
		Rules:           rules,
		Cycles:          cycles,
	}, nil
}
