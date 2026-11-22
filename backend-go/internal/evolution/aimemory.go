package evolution

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Phoenix7290/agy-mobile-gateway/backend-go/internal/db"
)

// AIMemoryOutcome captures results from calling the ai-memory MCP server tools.
type AIMemoryOutcome struct {
	Configured   bool     `json:"configured"`
	Reachable    bool     `json:"reachable"`
	Consolidated bool     `json:"consolidated"`
	AutoImproved bool     `json:"auto_improved"`
	Status       string   `json:"status"` // disabled | ok | partial | unreachable
	Insights     []string `json:"insights,omitempty"`
}

// AIMemoryClient invokes MCP tools (memory_consolidate, memory_auto_improve, memory_write_page) over Streamable HTTP.
type AIMemoryClient struct {
	endpoint   string
	httpClient *http.Client
}

// NewAIMemoryClient creates an MCP HTTP client for AI_MEMORY_SERVER_URL.
func NewAIMemoryClient(endpoint string) *AIMemoryClient {
	return &AIMemoryClient{
		endpoint: strings.TrimRight(strings.TrimSpace(endpoint), "/"),
		httpClient: &http.Client{
			Timeout: 12 * time.Second,
		},
	}
}

type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      any    `json:"id,omitempty"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string         `json:"jsonrpc"`
	ID      any            `json:"id"`
	Result  map[string]any `json:"result"`
	Error   *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func (c *AIMemoryClient) postRPC(ctx context.Context, sessionID string, payload rpcRequest) (rpcResponse, string, error) {
	var out rpcResponse
	raw, err := json.Marshal(payload)
	if err != nil {
		return out, sessionID, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(raw))
	if err != nil {
		return out, sessionID, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if sessionID != "" {
		req.Header.Set("Mcp-Session-Id", sessionID)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return out, sessionID, err
	}
	defer resp.Body.Close()

	if sid := strings.TrimSpace(resp.Header.Get("Mcp-Session-Id")); sid != "" {
		sessionID = sid
	}
	bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if resp.StatusCode >= 400 {
		return out, sessionID, fmt.Errorf("MCP HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(bodyBytes)))
	}

	contentType := strings.ToLower(resp.Header.Get("Content-Type"))
	trimmed := strings.TrimSpace(string(bodyBytes))
	if trimmed == "" {
		return out, sessionID, nil
	}

	if strings.Contains(contentType, "text/event-stream") || strings.HasPrefix(trimmed, "event:") || strings.HasPrefix(trimmed, "data:") {
		sc := bufio.NewScanner(bytes.NewReader(bodyBytes))
		var last rpcResponse
		found := false
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if !strings.HasPrefix(line, "data:") {
				continue
			}
			dataStr := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if dataStr == "" || dataStr == "[DONE]" {
				continue
			}
			var cand rpcResponse
			if json.Unmarshal([]byte(dataStr), &cand) == nil {
				last = cand
				found = true
			}
		}
		if found {
			out = last
			if out.Error != nil {
				return out, sessionID, fmt.Errorf("MCP RPC %d: %s", out.Error.Code, out.Error.Message)
			}
			return out, sessionID, nil
		}
	}

	if err := json.Unmarshal(bodyBytes, &out); err != nil {
		return out, sessionID, fmt.Errorf("invalid MCP JSON response: %w", err)
	}
	if out.Error != nil {
		return out, sessionID, fmt.Errorf("MCP RPC %d: %s", out.Error.Code, out.Error.Message)
	}
	return out, sessionID, nil
}

func (c *AIMemoryClient) initSession(ctx context.Context) (string, error) {
	_, sid, err := c.postRPC(ctx, "", rpcRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "initialize",
		Params: map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]any{},
			"clientInfo": map[string]string{
				"name":    "agy-mobile-gateway",
				"version": "1.0.0",
			},
		},
	})
	if err != nil {
		return "", err
	}
	_, sid, _ = c.postRPC(ctx, sid, rpcRequest{
		JSONRPC: "2.0",
		Method:  "notifications/initialized",
	})
	return sid, nil
}

func extractMCPContentText(res map[string]any) string {
	if res == nil {
		return ""
	}
	content, _ := res["content"].([]any)
	var parts []string
	for _, raw := range content {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if txt, ok := item["text"].(string); ok && strings.TrimSpace(txt) != "" {
			parts = append(parts, strings.TrimSpace(txt))
		}
	}
	if len(parts) > 0 {
		return strings.Join(parts, "\n")
	}
	if structured, ok := res["structuredContent"]; ok {
		b, err := json.Marshal(structured)
		if err == nil {
			return string(b)
		}
	}
	return ""
}

// CallTool invokes one MCP tool by name over JSON-RPC.
func (c *AIMemoryClient) CallTool(ctx context.Context, sessionID, toolName string, args map[string]any) (string, string, error) {
	resp, nextSID, err := c.postRPC(ctx, sessionID, rpcRequest{
		JSONRPC: "2.0",
		ID:      2,
		Method:  "tools/call",
		Params: map[string]any{
			"name":      toolName,
			"arguments": args,
		},
	})
	if err != nil {
		return "", nextSID, err
	}
	if isErr, _ := resp.Result["isError"].(bool); isErr {
		txt := extractMCPContentText(resp.Result)
		if txt == "" {
			txt = "tool returned isError=true"
		}
		return "", nextSID, fmt.Errorf("%s", txt)
	}
	return extractMCPContentText(resp.Result), nextSID, nil
}

// ConsolidateAndImprove invokes memory_consolidate and memory_auto_improve when AI_MEMORY_SERVER_URL is configured.
func (c *AIMemoryClient) ConsolidateAndImprove(ctx context.Context, summaryDigest map[string]any) AIMemoryOutcome {
	if c == nil || c.endpoint == "" {
		return AIMemoryOutcome{Configured: false, Status: "disabled"}
	}

	outcome := AIMemoryOutcome{Configured: true}
	sid, err := c.initSession(ctx)
	if err != nil {
		outcome.Status = "unreachable"
		return outcome
	}
	outcome.Reachable = true

	summaryBytes, _ := json.Marshal(summaryDigest)
	summaryStr := string(summaryBytes)

	if text, nextSID, err := c.CallTool(ctx, sid, "memory_consolidate", map[string]any{
		"scope":   "aegys_telemetry",
		"source":  "agy-mobile-gateway",
		"summary": summaryStr,
	}); err == nil {
		sid = nextSID
		outcome.Consolidated = true
		if t := strings.TrimSpace(text); t != "" {
			outcome.Insights = append(outcome.Insights, t)
		}
	}

	if text, _, err := c.CallTool(ctx, sid, "memory_auto_improve", map[string]any{
		"scope":     "aegys_telemetry",
		"focus":     "preferences_fastpath_and_rules",
		"telemetry": summaryStr,
	}); err == nil {
		outcome.AutoImproved = true
		if t := strings.TrimSpace(text); t != "" {
			outcome.Insights = append(outcome.Insights, t)
		}
	}

	switch {
	case outcome.Consolidated && outcome.AutoImproved:
		outcome.Status = "ok"
	case outcome.Consolidated || outcome.AutoImproved:
		outcome.Status = "partial"
	default:
		outcome.Status = "partial"
	}
	return outcome
}

// SyncApprovedRule best-effort mirrors an approved Aegys rule/preference to ai-memory.
func (c *AIMemoryClient) SyncApprovedRule(ctx context.Context, rule db.AegysRule) error {
	if c == nil || c.endpoint == "" {
		return nil
	}
	sid, err := c.initSession(ctx)
	if err != nil {
		return err
	}
	content := fmt.Sprintf("# Aegys Learned %s: %s\n\n%s\n\nApprovedAt: %s\n", rule.Kind, rule.Title, rule.Content, rule.CreatedAt)
	_, _, err = c.CallTool(ctx, sid, "memory_write_page", map[string]any{
		"page":    "aegys/learned-" + rule.ID + ".md",
		"content": content,
		"append":  false,
	})
	return err
}
