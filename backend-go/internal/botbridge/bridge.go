package botbridge

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Phoenix7290/agy-mobile-gateway/backend-go/internal/config"
	"github.com/Phoenix7290/agy-mobile-gateway/backend-go/internal/db"
	"github.com/Phoenix7290/agy-mobile-gateway/backend-go/internal/jobs"
)

// JobResult holds the final aggregated outcome of a job for webhooks or bot replies.
type JobResult struct {
	JobID          string   `json:"job_id"`
	ConversationID string   `json:"conversation_id,omitempty"`
	Status         string   `json:"status"`
	Reply          string   `json:"reply"`
	OutboxFiles    []string `json:"outbox_files,omitempty"`
	Error          string   `json:"error,omitempty"`
}

// Bridge encapsulates optional webhook, WhatsApp (Evolution API), and Discord DM connectors.
type Bridge struct {
	Cfg   config.GatewaySettings
	Store *db.GatewayDB
	Mgr   *jobs.Manager
	HTTP  *http.Client
}

// New creates a Bridge with sensible HTTP client defaults.
func New(cfg config.GatewaySettings, store *db.GatewayDB, mgr *jobs.Manager) *Bridge {
	return &Bridge{
		Cfg:   cfg,
		Store: store,
		Mgr:   mgr,
		HTTP:  &http.Client{Timeout: 15 * time.Second},
	}
}

// VerifyBearer checks an Authorization: Bearer <token> header in constant time.
func VerifyBearer(authHeader, expectedSecret string) bool {
	expected := strings.TrimSpace(expectedSecret)
	if expected == "" {
		return false
	}
	const prefix = "Bearer "
	if len(authHeader) <= len(prefix) || !strings.EqualFold(authHeader[:len(prefix)], prefix) {
		return false
	}
	token := strings.TrimSpace(authHeader[len(prefix):])
	return subtle.ConstantTimeCompare([]byte(token), []byte(expected)) == 1
}

// NormalizeDigits strips non-numeric characters from a phone number or JID prefix.
func NormalizeDigits(raw string) string {
	var b strings.Builder
	for _, r := range raw {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// IsAllowedWhatsApp enforces DM-only (@s.whatsapp.net), non-self (fromMe=false), and allowlist matching.
func (b *Bridge) IsAllowedWhatsApp(remoteJID string, fromMe bool) (string, bool) {
	if fromMe {
		return "", false
	}
	jid := strings.TrimSpace(remoteJID)
	if jid == "" || strings.HasSuffix(jid, "@g.us") || !strings.Contains(jid, "@s.whatsapp.net") {
		return "", false
	}
	number := NormalizeDigits(strings.SplitN(jid, "@", 2)[0])
	if number == "" || len(b.Cfg.WhatsAppAllowedNumbers) == 0 {
		return "", false
	}
	for _, allowed := range b.Cfg.WhatsAppAllowedNumbers {
		if NormalizeDigits(allowed) == number {
			return number, true
		}
	}
	return "", false
}

// IsAllowedDiscordUser enforces DM-only and strict Discord Snowflake User ID allowlist matching.
func (b *Bridge) IsAllowedDiscordUser(userID string, isDM bool) bool {
	if !isDM {
		return false
	}
	uid := strings.TrimSpace(userID)
	if uid == "" || len(b.Cfg.DiscordAllowedUserIDs) == 0 {
		return false
	}
	for _, allowed := range b.Cfg.DiscordAllowedUserIDs {
		if strings.TrimSpace(allowed) == uid {
			return true
		}
	}
	return false
}

// WaitForJobResult waits until a job completes, errors, or the timeout elapses.
func (b *Bridge) WaitForJobResult(ctx context.Context, jobID string, timeout time.Duration) (JobResult, error) {
	res := JobResult{JobID: jobID, Status: "running"}
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()

	for {
		job, err := b.Store.GetJob(jobID)
		if err != nil {
			return res, err
		}
		if job == nil {
			return res, fmt.Errorf("job not found")
		}
		if job.ConversationID != nil {
			res.ConversationID = *job.ConversationID
		}
		if job.Status == "completed" || job.Status == "done" || job.Status == "error" || job.Status == "cancelled" {
			res.Status = job.Status
			events, _ := b.Store.GetJobEventsAfter(jobID, 0)
			var textBuf strings.Builder
			for _, ev := range events {
				switch ev.EventType {
				case "delta":
					if t, ok := ev.Payload["text"].(string); ok {
						textBuf.WriteString(t)
					}
				case "done":
					if r, ok := ev.Payload["response"].(string); ok && strings.TrimSpace(r) != "" {
						res.Reply = r
					}
					if cid, ok := ev.Payload["conversation_id"].(string); ok && cid != "" {
						res.ConversationID = cid
					}
					if files, ok := ev.Payload["outbox_files"].([]any); ok {
						for _, f := range files {
							if s, ok := f.(string); ok && s != "" {
								res.OutboxFiles = append(res.OutboxFiles, s)
							}
						}
					}
				case "error":
					if msg, ok := ev.Payload["message"].(string); ok {
						res.Error = msg
					}
				}
			}
			if res.Reply == "" {
				res.Reply = textBuf.String()
			}
			return res, nil
		}

		sig := b.Mgr.WaitSignal(jobID)
		select {
		case <-ctx.Done():
			return res, ctx.Err()
		case <-deadline.C:
			return res, context.DeadlineExceeded
		case <-sig:
		}
	}
}

// SendWhatsAppText sends a plain text message via Evolution API v2.
func (b *Bridge) SendWhatsAppText(ctx context.Context, number, text string) error {
	if b.Cfg.EvolutionAPIURL == "" || b.Cfg.EvolutionInstance == "" {
		return fmt.Errorf("evolution api not configured")
	}
	endpoint := fmt.Sprintf("%s/message/sendText/%s", b.Cfg.EvolutionAPIURL, b.Cfg.EvolutionInstance)
	payload, _ := json.Marshal(map[string]any{
		"number": number,
		"text":   text,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if b.Cfg.EvolutionAPIKey != "" {
		req.Header.Set("apikey", b.Cfg.EvolutionAPIKey)
	}
	resp, err := b.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("evolution api status %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	return nil
}

// SendDiscordDM opens (or reuses) a DM channel with userID and sends a message via Discord REST API v10.
func (b *Bridge) SendDiscordDM(ctx context.Context, userID, text string) error {
	if b.Cfg.DiscordBotToken == "" {
		return fmt.Errorf("discord bot token not configured")
	}
	chBody, _ := json.Marshal(map[string]string{"recipient_id": userID})
	chReq, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://discord.com/api/v10/users/@me/channels", bytes.NewReader(chBody))
	if err != nil {
		return err
	}
	chReq.Header.Set("Content-Type", "application/json")
	chReq.Header.Set("Authorization", "Bot "+b.Cfg.DiscordBotToken)
	chResp, err := b.HTTP.Do(chReq)
	if err != nil {
		return err
	}
	defer chResp.Body.Close()
	if chResp.StatusCode >= 400 {
		raw, _ := io.ReadAll(io.LimitReader(chResp.Body, 512))
		return fmt.Errorf("discord dm channel error %d: %s", chResp.StatusCode, strings.TrimSpace(string(raw)))
	}
	var ch struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(chResp.Body).Decode(&ch); err != nil || ch.ID == "" {
		return fmt.Errorf("invalid discord dm channel response")
	}

	if len(text) > 1900 {
		text = text[:1900] + "\n…"
	}
	msgBody, _ := json.Marshal(map[string]string{"content": text})
	msgReq, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("https://discord.com/api/v10/channels/%s/messages", ch.ID), bytes.NewReader(msgBody))
	if err != nil {
		return err
	}
	msgReq.Header.Set("Content-Type", "application/json")
	msgReq.Header.Set("Authorization", "Bot "+b.Cfg.DiscordBotToken)
	msgResp, err := b.HTTP.Do(msgReq)
	if err != nil {
		return err
	}
	defer msgResp.Body.Close()
	if msgResp.StatusCode >= 400 {
		raw, _ := io.ReadAll(io.LimitReader(msgResp.Body, 512))
		return fmt.Errorf("discord send error %d: %s", msgResp.StatusCode, strings.TrimSpace(string(raw)))
	}
	return nil
}
