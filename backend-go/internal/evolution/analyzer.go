package evolution

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/Phoenix7290/agy-mobile-gateway/backend-go/internal/config"
	"github.com/Phoenix7290/agy-mobile-gateway/backend-go/internal/db"
	"github.com/Phoenix7290/agy-mobile-gateway/backend-go/internal/runner"
)

// CandidateProposal represents one normalized improvement suggestion before persistence.
type CandidateProposal struct {
	Kind           string `json:"kind"` // fast_path_slash | prompt_rule | preference
	Title          string `json:"title"`
	Rationale      string `json:"rationale"`
	Command        string `json:"command,omitempty"`
	Description    string `json:"description,omitempty"`
	PromptTemplate string `json:"prompt_template,omitempty"`
	Model          string `json:"model,omitempty"`
	AutoSubmit     bool   `json:"auto_submit,omitempty"`
	RuleContent    string `json:"rule_content,omitempty"`
	PromptNormKey  string `json:"prompt_norm_key,omitempty"`
}

// AnalysisInput holds condensed telemetry data reviewed in each consolidation cycle.
type AnalysisInput struct {
	Stats            db.TelemetryStats
	ExistingCommands []string
	ExistingRules    []string
	AIMemoryInsights []string
	MinFastPathHits  int
}

// AnalysisOutput carries proposals extracted by ollama-worker, Ollama HTTP, or deterministic heuristics.
type AnalysisOutput struct {
	Backend    string
	Summary    string
	Candidates []CandidateProposal
}

var (
	thinkBlockRe = regexp.MustCompile(`(?is)<think>.*?</think>`)
	jsonFenceRe  = regexp.MustCompile("(?s)```(?:json)?\\s*(\\{.*?\\})\\s*```")
	nonSlugRe    = regexp.MustCompile(`[^a-z0-9_]+`)
)

// AnalyzeLocal prefers local ollama-worker CLI, then Ollama HTTP /api/chat, enriched with deterministic fast-path detection.
func AnalyzeLocal(ctx context.Context, cfg config.GatewaySettings, r runner.Runner, in AnalysisInput) AnalysisOutput {
	promptPayload := buildAnalyzerPrompt(in)

	// 1. Try ollama-worker CLI when installed on host (zero cloud token cost).
	if workerOut, ok := tryOllamaWorkerCLI(ctx, cfg.OllamaWorkerBin, promptPayload); ok {
		workerOut.Candidates = mergeDeterministicCandidates(workerOut.Candidates, in)
		return workerOut
	}

	// 2. Try Ollama HTTP /api/chat (local or tunneled via configured runner).
	if enabled, _ := cfg.Mods["ollama"].(bool); enabled && r != nil {
		if ollamaOut, ok := tryOllamaRunnerChat(ctx, cfg, r, promptPayload); ok {
			ollamaOut.Candidates = mergeDeterministicCandidates(ollamaOut.Candidates, in)
			return ollamaOut
		}
	}

	// 3. Deterministic local fallback if Ollama/worker are offline at off-peak time.
	cands := mergeDeterministicCandidates(nil, in)
	summary := fmt.Sprintf(
		"Análise heurística local: %d comandos frequentes, %d padrões repetidos, %d falhas/correções recentes.",
		len(in.Stats.TopCommands), len(in.Stats.RepeatedPrompts), len(in.Stats.RecentFailures),
	)
	return AnalysisOutput{
		Backend:    "heuristic",
		Summary:    summary,
		Candidates: cands,
	}
}

func buildAnalyzerPrompt(in AnalysisInput) string {
	digest := map[string]any{
		"top_commands":      in.Stats.TopCommands,
		"repeated_prompts":  in.Stats.RepeatedPrompts,
		"recent_failures":   in.Stats.RecentFailures,
		"existing_commands": in.ExistingCommands,
		"existing_rules":    in.ExistingRules,
		"ai_memory_notes":   in.AIMemoryInsights,
		"totals": map[string]int{
			"ok":          in.Stats.TotalJobsOK,
			"errors":      in.Stats.TotalJobErrors,
			"up":          in.Stats.TotalFeedbackUp,
			"down":        in.Stats.TotalFeedbackDown,
			"corrections": in.Stats.TotalCorrections,
		},
	}
	rawDigest, _ := json.Marshal(digest)
	return fmt.Sprintf(`Você é o módulo de auto-aprimoramento do Aegys (AGY Mobile Gateway).
Analise a telemetria abaixo e retorne EXCLUSIVAMENTE um JSON válido com este formato exato:
{
  "summary": "Resumo curto em português (1-2 frases) dos padrões encontrados",
  "proposals": [
    {
      "kind": "fast_path_slash | prompt_rule | preference",
      "title": "Título curto",
      "rationale": "Motivo objetivo baseado nos dados",
      "command": "/atalho (somente se kind=fast_path_slash)",
      "description": "Descrição curta do comando",
      "prompt_template": "Template com {{input}} opcional",
      "model": "opcional (ex: ollama:qwen3:8b ou vazio)",
      "auto_submit": true,
      "rule_content": "Regra objetiva concisa em PT-BR (se kind=prompt_rule ou preference)"
    }
  ]
}
Regras:
- Sugira no máximo 4 propostas de alto valor; não repita existing_commands nem existing_rules.
- Para prompts muito repetidos, prefira kind="fast_path_slash" enxuto.
- Para erros ou correções do usuário, gere kind="prompt_rule" ou "preference" prática.

Telemetria:
%s`, string(rawDigest))
}

func tryOllamaWorkerCLI(ctx context.Context, bin, prompt string) (AnalysisOutput, bool) {
	binName := strings.TrimSpace(bin)
	if binName == "" {
		binName = "ollama-worker"
	}
	if _, err := exec.LookPath(binName); err != nil {
		return AnalysisOutput{}, false
	}

	cctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()

	cmd := exec.CommandContext(cctx, binName, "-m", "fast", "Analise a telemetria JSON da entrada padrão e responda SOMENTE com o JSON solicitado.")
	cmd.Stdin = strings.NewReader(prompt)
	var outBuf bytes.Buffer
	cmd.Stdout = &outBuf
	if err := cmd.Run(); err != nil {
		return AnalysisOutput{}, false
	}
	parsed, ok := parseAnalyzerJSON(outBuf.String())
	if !ok {
		return AnalysisOutput{}, false
	}
	parsed.Backend = "ollama-worker"
	return parsed, true
}

func selectLocalEvolutionModel(ctx context.Context, cfg config.GatewaySettings, r runner.Runner) string {
	if m := strings.TrimSpace(cfg.EvolutionModel); m != "" {
		return strings.TrimPrefix(m, "ollama:")
	}
	localModels := r.ListOllamaModels(ctx)
	ids := map[string]bool{}
	for _, m := range localModels {
		ids[m.ID] = true
	}
	for _, pref := range []string{"ollama:qwen3:8b", "ollama:llama3.2:latest", "ollama:qwen3:14b"} {
		if ids[pref] {
			return strings.TrimPrefix(pref, "ollama:")
		}
	}
	if len(localModels) > 0 {
		return strings.TrimPrefix(localModels[0].ID, "ollama:")
	}
	return ""
}

func tryOllamaRunnerChat(ctx context.Context, cfg config.GatewaySettings, r runner.Runner, prompt string) (AnalysisOutput, bool) {
	model := selectLocalEvolutionModel(ctx, cfg, r)
	if model == "" {
		return AnalysisOutput{}, false
	}
	cctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()

	payload, _ := json.Marshal(map[string]any{
		"model":  model,
		"stream": false,
		"format": "json",
		"messages": []map[string]string{
			{"role": "system", "content": "Responda apenas JSON válido sem markdown extra."},
			{"role": "user", "content": prompt},
		},
	})
	stream, closeFn, err := r.OpenOllamaChat(cctx, payload)
	if err != nil {
		return AnalysisOutput{}, false
	}
	defer func() { _ = closeFn() }()

	rawBytes, err := io.ReadAll(io.LimitReader(stream, 2<<20))
	if err != nil || len(rawBytes) == 0 {
		return AnalysisOutput{}, false
	}

	content := extractOllamaResponseContent(rawBytes)
	parsed, ok := parseAnalyzerJSON(content)
	if !ok {
		return AnalysisOutput{}, false
	}
	parsed.Backend = "ollama:" + model
	return parsed, true
}

func extractOllamaResponseContent(rawBytes []byte) string {
	trimmed := strings.TrimSpace(string(rawBytes))
	var single struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		Response string `json:"response"`
	}
	if json.Unmarshal([]byte(trimmed), &single) == nil && (single.Message.Content != "" || single.Response != "") {
		if single.Message.Content != "" {
			return single.Message.Content
		}
		return single.Response
	}

	var buf strings.Builder
	sc := bufio.NewScanner(bytes.NewReader(rawBytes))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var chunk struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
			Response string `json:"response"`
		}
		if json.Unmarshal([]byte(line), &chunk) == nil {
			if chunk.Message.Content != "" {
				buf.WriteString(chunk.Message.Content)
			} else if chunk.Response != "" {
				buf.WriteString(chunk.Response)
			}
		}
	}
	if buf.Len() > 0 {
		return buf.String()
	}
	return trimmed
}

func parseAnalyzerJSON(raw string) (AnalysisOutput, bool) {
	cleaned := thinkBlockRe.ReplaceAllString(raw, "")
	cleaned = strings.TrimSpace(cleaned)
	if m := jsonFenceRe.FindStringSubmatch(cleaned); len(m) >= 2 {
		cleaned = m[1]
	} else if start := strings.Index(cleaned, "{"); start >= 0 {
		if end := strings.LastIndex(cleaned, "}"); end > start {
			cleaned = cleaned[start : end+1]
		}
	}

	var doc struct {
		Summary   string              `json:"summary"`
		Proposals []CandidateProposal `json:"proposals"`
	}
	if err := json.Unmarshal([]byte(cleaned), &doc); err != nil {
		return AnalysisOutput{}, false
	}
	return AnalysisOutput{
		Summary:    strings.TrimSpace(doc.Summary),
		Candidates: doc.Proposals,
	}, true
}

func mergeDeterministicCandidates(existing []CandidateProposal, in AnalysisInput) []CandidateProposal {
	out := append([]CandidateProposal{}, existing...)
	seenCmds := map[string]bool{}
	for _, c := range out {
		if c.Command != "" {
			seenCmds[strings.ToLower(c.Command)] = true
		}
	}

	// Ensure repeated non-slash prompts always surface a concrete Fast-Path shortcut proposal.
	for _, rep := range in.Stats.RepeatedPrompts {
		if rep.Count < in.MinFastPathHits {
			continue
		}
		cmd := deriveSlashSlug(rep.PromptPreview)
		if seenCmds[cmd] {
			continue
		}
		seenCmds[cmd] = true
		out = append(out, CandidateProposal{
			Kind:           "fast_path_slash",
			Title:          fmt.Sprintf("Atalho Fast-Path %s (%dx)", cmd, rep.Count),
			Rationale:      fmt.Sprintf("Você enviou este pedido %d vezes recentemente.", rep.Count),
			Command:        cmd,
			Description:    clipAnalyzerRunes(rep.PromptPreview, 72),
			PromptTemplate: rep.PromptPreview + " {{input}}",
			Model:          rep.PreferredModel,
			AutoSubmit:     true,
			PromptNormKey:  rep.PromptNorm,
		})
	}

	// If there are multiple recent failures/corrections and LLM proposed no rule, synthesize one focused diagnostic rule.
	hasRule := false
	for _, c := range out {
		if c.Kind == "prompt_rule" || c.Kind == "preference" {
			hasRule = true
			break
		}
	}
	if !hasRule && len(in.Stats.RecentFailures) > 0 {
		var notes []string
		for _, f := range in.Stats.RecentFailures {
			if f.CorrectionNote != "" {
				notes = append(notes, f.CorrectionNote)
			} else if f.ErrorMessage != "" {
				notes = append(notes, f.ErrorMessage)
			}
			if len(notes) >= 2 {
				break
			}
		}
		if len(notes) > 0 {
			sample := clipAnalyzerRunes(strings.Join(notes, " | "), 180)
			out = append(out, CandidateProposal{
				Kind:        "prompt_rule",
				Title:       "Validar pré-condições antes de responder após falhas recentes",
				Rationale:   fmt.Sprintf("Detectadas %d falha(s)/correção(ões) recente(s): %s", len(in.Stats.RecentFailures), sample),
				RuleContent: "Antes de finalizar tarefas semelhantes às falhas recentes (" + sample + "), valide caminhos, comandos e saídas reais no workspace.",
			})
		}
	}

	if len(out) > 6 {
		out = out[:6]
	}
	return out
}

func deriveSlashSlug(prompt string) string {
	words := strings.Fields(strings.ToLower(prompt))
	var chosen []string
	stop := map[string]bool{
		"o": true, "a": true, "os": true, "as": true, "de": true, "do": true, "da": true,
		"em": true, "no": true, "na": true, "para": true, "pra": true, "por": true,
		"que": true, "com": true, "um": true, "uma": true, "me": true, "meu": true, "minha": true,
		"verify": true, "check": true, "please": true, "the": true, "and": true, "porfavor": true,
	}
	for _, w := range words {
		clean := nonSlugRe.ReplaceAllString(w, "")
		if len(clean) < 2 || stop[clean] {
			continue
		}
		chosen = append(chosen, clean)
		if len(chosen) >= 2 {
			break
		}
	}
	if len(chosen) == 0 {
		return "/fast_atalho"
	}
	slug := strings.Join(chosen, "_")
	if len(slug) > 24 {
		slug = slug[:24]
	}
	return "/" + slug
}

func clipAnalyzerRunes(s string, maxLen int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= maxLen {
		return string(r)
	}
	return string(r[:maxLen]) + "..."
}
