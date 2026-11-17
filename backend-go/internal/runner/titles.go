package runner

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// CleanUserRequest strips AGY <USER_REQUEST> wrapper tags and injected mobile Outbox hints from transcript text.
func CleanUserRequest(text string) string {
	cleaned := text
	if idx := strings.Index(cleaned, "<USER_REQUEST>"); idx >= 0 {
		cleaned = cleaned[idx+len("<USER_REQUEST>"):]
	}
	if idx := strings.Index(cleaned, "</USER_REQUEST>"); idx >= 0 {
		cleaned = cleaned[:idx]
	}
	var kept []string
	for _, line := range strings.Split(cleaned, "\n") {
		s := strings.TrimSpace(line)
		if strings.HasPrefix(s, "[Contexto AGY Mobile Outbox:") ||
			strings.HasPrefix(s, "[ Importante: Salve ou copie o arquivo final gerado no diretório Outbox") {
			continue
		}
		kept = append(kept, line)
	}
	return strings.TrimSpace(strings.Join(kept, "\n"))
}

// ExtractConversationTitle picks the first meaningful line, skipping attachment wrapper notices.
func ExtractConversationTitle(text, fallback string, maxLen int) string {
	if maxLen <= 0 {
		maxLen = 55
	}
	cleaned := CleanUserRequest(text)
	attachFallback := ""
	for _, rawLine := range strings.Split(cleaned, "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "[Arquivos anexados pelo usuário em ") {
			if attachFallback == "" {
				if _, afterColon, ok := strings.Cut(line, ":"); ok {
					filesPart := strings.SplitN(afterColon, ". Inspecione-os", 2)[0]
					filesPart = strings.TrimSuffix(strings.TrimSpace(filesPart), "]")
					var names []string
					for _, p := range strings.Split(filesPart, ",") {
						p = strings.TrimSpace(p)
						if p != "" {
							names = append(names, filepath.Base(p))
						}
					}
					if len(names) > 0 {
						attachFallback = "Anexo: " + strings.Join(names, ", ")
					}
				}
			}
			continue
		}
		return truncateRunes(line, maxLen)
	}
	if attachFallback != "" {
		return truncateRunes(attachFallback, maxLen)
	}
	return fallback
}

func truncateRunes(s string, maxLen int) string {
	s = strings.Join(strings.Fields(s), " ")
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	if maxLen <= 1 {
		return string(runes[:maxLen])
	}
	cut := runes[:maxLen-1]
	lastSpace := -1
	for i := len(cut) - 1; i >= 0; i-- {
		if cut[i] == ' ' {
			lastSpace = i
			break
		}
	}
	if lastSpace >= maxLen/2 {
		cut = cut[:lastSpace]
	}
	trimmed := strings.TrimRight(string(cut), " ,;:-")
	return trimmed + "…"
}

func loadSummaryTitles(cliDir string) map[string]string {
	out := map[string]string{}
	cachePath := filepath.Join(cliDir, "gateway_titles.json")
	if raw, err := os.ReadFile(cachePath); err == nil {
		var cached map[string]string
		if json.Unmarshal(raw, &cached) == nil {
			for cid, title := range cached {
				if cid != "" && strings.TrimSpace(title) != "" {
					out[cid] = strings.TrimSpace(title)
				}
			}
		}
	}

	dbPath := filepath.Join(cliDir, "conversation_summaries.db")
	if _, err := os.Stat(dbPath); err != nil {
		return out
	}
	dsn := fmt.Sprintf("file:%s?mode=ro&_pragma=busy_timeout(1000)", dbPath)
	conn, err := sql.Open("sqlite", dsn)
	if err != nil {
		return out
	}
	defer conn.Close()

	rows, err := conn.Query("SELECT conversation_id, title FROM conversation_summaries WHERE title != ''")
	if err != nil {
		return out
	}
	defer rows.Close()

	for rows.Next() {
		var cid, title string
		if err := rows.Scan(&cid, &title); err == nil && cid != "" && strings.TrimSpace(title) != "" {
			out[cid] = strings.TrimSpace(title)
		}
	}
	return out
}
