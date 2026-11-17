package main

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Phoenix7290/agy-mobile-gateway/backend-go/internal/config"
	"github.com/Phoenix7290/agy-mobile-gateway/backend-go/internal/db"
	"github.com/Phoenix7290/agy-mobile-gateway/backend-go/internal/jobs"
	"github.com/Phoenix7290/agy-mobile-gateway/backend-go/internal/runner"
	"github.com/Phoenix7290/agy-mobile-gateway/backend-go/internal/wol"
)

func newTestServer(t *testing.T) (*http.ServeMux, *db.GatewayDB, config.GatewaySettings) {
	t.Helper()
	return newTestServerWithEnv(t, nil)
}

func newTestServerWithEnv(t *testing.T, extra map[string]string) (*http.ServeMux, *db.GatewayDB, config.GatewaySettings) {
	t.Helper()
	tmpDir := t.TempDir()
	t.Setenv("GATEWAY_MODE", "standalone")
	t.Setenv("ENABLE_WOL", "false")
	t.Setenv("DATA_DIR", tmpDir)
	t.Setenv("CACHE_DIR", filepath.Join(tmpDir, "cache"))
	t.Setenv("INBOX_DIR", filepath.Join(tmpDir, "inbox"))
	t.Setenv("OUTBOX_DIR", filepath.Join(tmpDir, "outbox"))
	t.Setenv("AGY_BRAIN_DIR", filepath.Join(tmpDir, "brain"))
	for k, v := range extra {
		t.Setenv(k, v)
	}

	cfg := config.Load()
	store, err := db.Open(filepath.Join(tmpDir, "gateway.db"))
	if err != nil {
		t.Fatalf("db.Open failed: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	r := runner.Create(cfg)
	mgr := jobs.NewManager(cfg, store, r)
	wd := &watchdogState{}
	return buildMux(cfg, store, r, mgr, wd), store, cfg
}

func TestHealthFeaturesStatusModelsAndSettings(t *testing.T) {
	mux, _, _ := newTestServer(t)

	for _, path := range []string{"/api/health", "/api/features", "/api/status", "/api/watchdog", "/api/telemetry", "/api/settings", "/api/models", "/api/jobs/active", "/api/outbox", "/", "/manifest.json", "/static/icon.svg"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s: got status %d, want 200", path, rec.Code)
		}
	}

	// Update settings + validation
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPut, "/api/settings", bytes.NewBufferString(`{"default_permission_mode":"autonomous","default_model":"auto","theme_mode":"light","theme_palette":"emerald"}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT /api/settings expected 200, got %d", rec.Code)
	}
	var updatedSettings map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &updatedSettings)
	if updatedSettings["theme_mode"] != "light" || updatedSettings["theme_palette"] != "emerald" {
		t.Fatalf("expected persisted theme settings, got %+v", updatedSettings)
	}
	badRec := httptest.NewRecorder()
	mux.ServeHTTP(badRec, httptest.NewRequest(http.MethodPut, "/api/settings", bytes.NewBufferString(`{"default_permission_mode":"invalid"}`)))
	if badRec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid permission mode, got %d", badRec.Code)
	}
	badThemeRec := httptest.NewRecorder()
	mux.ServeHTTP(badThemeRec, httptest.NewRequest(http.MethodPut, "/api/settings", bytes.NewBufferString(`{"theme_palette":"neon"}`)))
	if badThemeRec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid theme palette, got %d", badThemeRec.Code)
	}
}

func TestInboxUploadOutboxAndWoLPacket(t *testing.T) {
	mux, _, cfg := newTestServer(t)

	// 1. Multipart upload -> saved in Inbox
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("file", "relatório teste!.txt")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.WriteString(fw, "conteudo inbox")
	_ = mw.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/upload", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/upload status %d: %s", rec.Code, rec.Body.String())
	}
	var upRes map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &upRes)
	if upRes["ok"] != true {
		t.Fatalf("unexpected upload payload: %+v", upRes)
	}

	// 2. Outbox list, download, delete
	if err := os.WriteFile(filepath.Join(cfg.Dirs.OutboxDir, "saida.txt"), []byte("ola outbox"), 0o644); err != nil {
		t.Fatal(err)
	}
	lsRec := httptest.NewRecorder()
	mux.ServeHTTP(lsRec, httptest.NewRequest(http.MethodGet, "/api/outbox", nil))
	if !strings.Contains(lsRec.Body.String(), "saida.txt") {
		t.Fatalf("expected saida.txt in outbox list: %s", lsRec.Body.String())
	}

	dlRec := httptest.NewRecorder()
	mux.ServeHTTP(dlRec, httptest.NewRequest(http.MethodGet, "/api/outbox/saida.txt", nil))
	if dlRec.Code != http.StatusOK || dlRec.Body.String() != "ola outbox" {
		t.Fatalf("unexpected outbox download: code=%d body=%q", dlRec.Code, dlRec.Body.String())
	}

	delRec := httptest.NewRecorder()
	mux.ServeHTTP(delRec, httptest.NewRequest(http.MethodDelete, "/api/outbox/saida.txt", nil))
	if delRec.Code != http.StatusOK {
		t.Fatalf("DELETE /api/outbox status %d", delRec.Code)
	}

	// 3. WoL disabled check + Magic Packet structure (102 bytes)
	wolRec := httptest.NewRecorder()
	mux.ServeHTTP(wolRec, httptest.NewRequest(http.MethodPost, "/api/wol", nil))
	if wolRec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 when WoL disabled, got %d", wolRec.Code)
	}
	pkt, err := wol.BuildMagicPacket("bc:fc:e7:ca:84:05")
	if err != nil || len(pkt) != 102 {
		t.Fatalf("unexpected magic packet len=%d err=%v", len(pkt), err)
	}
}

func TestConversationsSearchAndJobSSEStreaming(t *testing.T) {
	ollamaSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tags":
			_, _ = io.WriteString(w, `{"models":[{"name":"ryan-fast:8b"}]}`)
		case "/api/chat":
			w.Header().Set("Content-Type", "application/x-ndjson")
			_, _ = io.WriteString(w, "{\"message\":{\"content\":\"DNS \"},\"done\":false}\n")
			_, _ = io.WriteString(w, "{\"message\":{\"content\":\"resolve nomes.\"},\"done\":true}\n")
		}
	}))
	defer ollamaSrv.Close()

	t.Setenv("OLLAMA_URL", ollamaSrv.URL)
	mux, _, _ := newTestServer(t)

	ts := httptest.NewServer(mux)
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/api/jobs", "application/json", strings.NewReader(`{"prompt":"o que e DNS?","model":"ollama:ryan-fast:8b"}`))
	if err != nil {
		t.Fatalf("POST /api/jobs failed: %v", err)
	}
	defer resp.Body.Close()

	var created struct {
		JobID          string `json:"job_id"`
		ConversationID string `json:"conversation_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}

	sseResp, err := http.Get(ts.URL + "/api/jobs/" + created.JobID + "/events?after=0")
	if err != nil {
		t.Fatal(err)
	}
	defer sseResp.Body.Close()
	sseBytes, _ := io.ReadAll(sseResp.Body)
	sseText := string(sseBytes)
	if !strings.Contains(sseText, "DNS ") || !strings.Contains(sseText, "resolve nomes.") || !strings.Contains(sseText, "data: [DONE]") {
		t.Fatalf("unexpected SSE output: %s", sseText)
	}

	convResp, err := http.Get(ts.URL + "/api/conversations?q=DNS")
	if err != nil {
		t.Fatal(err)
	}
	defer convResp.Body.Close()
	var listBody struct {
		Total         int `json:"total"`
		Conversations []struct {
			ID string `json:"id"`
		} `json:"conversations"`
	}
	_ = json.NewDecoder(convResp.Body).Decode(&listBody)
	if listBody.Total != 1 || listBody.Conversations[0].ID != created.ConversationID {
		t.Fatalf("expected 1 matching FTS conversation, got %+v", listBody)
	}
}

func TestAGYMockStreamJobExecution(t *testing.T) {
	mockDir := t.TempDir()
	mockScript := filepath.Join(mockDir, "mock-agy.sh")
	scriptBody := `#!/bin/sh
echo '{"event":"init","conversation_id":"conv-mock-1"}'
echo '{"event":"step_update","step_update":{"step_type":"agent_response","text_delta":"Olá "}}'
echo '{"event":"result","result":{"conversation_id":"conv-mock-1","status":"SUCCESS","response":"Olá mundo"}}'
`
	if err := os.WriteFile(mockScript, []byte(scriptBody), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AGY_BIN", mockScript)

	mux, _, _ := newTestServer(t)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/api/jobs", "application/json", strings.NewReader(`{"prompt":"teste agy","model":"gemini-4-hydrogen-high"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var created struct {
		JobID string `json:"job_id"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&created)

	sseResp, err := http.Get(ts.URL + "/api/jobs/" + created.JobID + "/events?after=0")
	if err != nil {
		t.Fatal(err)
	}
	defer sseResp.Body.Close()
	sseBytes, _ := io.ReadAll(sseResp.Body)
	if !strings.Contains(string(sseBytes), `"conversation_id":"conv-mock-1"`) || !strings.Contains(string(sseBytes), "data: [DONE]") {
		t.Fatalf("unexpected AGY SSE output: %s", string(sseBytes))
	}
}

func TestIDEBridgeQueueExecution(t *testing.T) {
	bridgeDir := filepath.Join(t.TempDir(), ".agy-bridge")
	mux, _, cfg := newTestServerWithEnv(t, map[string]string{
		"GATEWAY_MODE":           "ide-bridge",
		"IDE_BRIDGE_DIR":         bridgeDir,
		"IDE_BRIDGE_TIMEOUT_SEC": "10",
	})
	if cfg.Mode != "ide-bridge" || cfg.Mods["ide_bridge"] != true {
		t.Fatalf("expected ide-bridge mode enabled, got mode=%s mods=%+v", cfg.Mode, cfg.Mods)
	}

	ts := httptest.NewServer(mux)
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/api/jobs", "application/json", strings.NewReader(`{"prompt":"oi ide","model":"gemini-4-hydrogen-high"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /api/jobs status = %d", resp.StatusCode)
	}
	var created struct {
		JobID string `json:"job_id"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&created)

	inboxPath := filepath.Join(bridgeDir, "inbox", created.JobID+".json")
	outboxPath := filepath.Join(bridgeDir, "outbox", created.JobID+".json")
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(inboxPath); err == nil {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if _, err := os.Stat(inboxPath); err != nil {
		t.Fatalf("expected inbox file %s to be written: %v", inboxPath, err)
	}
	if err := os.MkdirAll(filepath.Dir(outboxPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(outboxPath, []byte(`{"response":"Resposta da IDE","conversation_id":"ide-conv-1"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	sseResp, err := http.Get(ts.URL + "/api/jobs/" + created.JobID + "/events?after=0")
	if err != nil {
		t.Fatal(err)
	}
	defer sseResp.Body.Close()
	sseBytes, _ := io.ReadAll(sseResp.Body)
	out := string(sseBytes)
	if !strings.Contains(out, "Resposta da IDE") || !strings.Contains(out, `"conversation_id":"ide-conv-1"`) || !strings.Contains(out, "data: [DONE]") {
		t.Fatalf("unexpected IDE bridge SSE output: %s", out)
	}
}

func TestWhisperTranscribeMod(t *testing.T) {
	// Disabled check first
	disabledMux, _, _ := newTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/transcribe", nil)
	rec := httptest.NewRecorder()
	disabledMux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 when whisper disabled, got %d", rec.Code)
	}

	whisperSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/audio/transcriptions" {
			http.NotFound(w, r)
			return
		}
		if err := r.ParseMultipartForm(4 << 20); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		file, _, err := r.FormFile("file")
		if err != nil {
			http.Error(w, "missing file", http.StatusBadRequest)
			return
		}
		_ = file.Close()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"text":"olá whisper"}`))
	}))
	defer whisperSrv.Close()

	mux, _, cfg := newTestServerWithEnv(t, map[string]string{
		"ENABLE_WHISPER": "true",
		"WHISPER_URL":    whisperSrv.URL,
	})
	if cfg.Mods["whisper"] != true {
		t.Fatalf("expected whisper mod true, got %+v", cfg.Mods)
	}

	ts := httptest.NewServer(mux)
	defer ts.Close()

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	part, err := mw.CreateFormFile("file", "audio.webm")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write([]byte("fake-audio-bytes"))
	_ = mw.Close()

	postReq, err := http.NewRequest(http.MethodPost, ts.URL+"/api/transcribe", &body)
	if err != nil {
		t.Fatal(err)
	}
	postReq.Header.Set("Content-Type", mw.FormDataContentType())
	resp, err := http.DefaultClient.Do(postReq)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("POST /api/transcribe status = %d: %s", resp.StatusCode, string(raw))
	}
	var payload map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&payload)
	if payload["text"] != "olá whisper" {
		t.Fatalf("unexpected transcribe payload: %+v", payload)
	}
}

func TestConversationMetaSortingFilteringAndSnapshots(t *testing.T) {
	mux, store, _ := newTestServer(t)
	if err := store.EnsureLocalConversation("local-a", "Primeira", "ollama:qwen3:8b"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(10 * time.Millisecond)
	if err := store.EnsureLocalConversation("local-b", "Segunda", "ollama:qwen3:8b"); err != nil {
		t.Fatal(err)
	}

	ts := httptest.NewServer(mux)
	defer ts.Close()

	// Pin local-a, rename it, and attach a project tag
	patchReq, _ := http.NewRequest(
		http.MethodPatch,
		ts.URL+"/api/conversations/local-a/meta",
		strings.NewReader(`{"pinned":true,"custom_title":"Sessão Fixada","tag":"ProjetoX","tag_color":"emerald"}`),
	)
	patchReq.Header.Set("Content-Type", "application/json")
	patchResp, err := http.DefaultClient.Do(patchReq)
	if err != nil || patchResp.StatusCode != http.StatusOK {
		t.Fatalf("PATCH meta failed: err=%v resp=%v", err, patchResp)
	}
	_ = patchResp.Body.Close()

	// Reject invalid tag color
	badReq, _ := http.NewRequest(
		http.MethodPatch,
		ts.URL+"/api/conversations/local-a/meta",
		strings.NewReader(`{"tag_color":"neon"}`),
	)
	badResp, err := http.DefaultClient.Do(badReq)
	if err != nil || badResp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 on invalid tag_color, got %v err=%v", badResp, err)
	}
	_ = badResp.Body.Close()

	// Verify pinned item comes first and engine/tag filters work
	listResp, err := http.Get(ts.URL + "/api/conversations?engine=ollama&tag=ProjetoX")
	if err != nil {
		t.Fatal(err)
	}
	defer listResp.Body.Close()
	var listData struct {
		Conversations []runner.ConversationSummary `json:"conversations"`
		Tags          []string                     `json:"tags"`
	}
	_ = json.NewDecoder(listResp.Body).Decode(&listData)
	if len(listData.Conversations) != 1 || listData.Conversations[0].ID != "local-a" || listData.Conversations[0].Title != "Sessão Fixada" || !listData.Conversations[0].Pinned {
		t.Fatalf("unexpected filtered conversations: %+v", listData)
	}

	// Create snapshot and verify safe escaped HTML rendering
	snapReqBytes, _ := json.Marshal(map[string]string{
		"title":         "Teste <script>",
		"content_md":    "# Titulo\n\n<script>alert(1)</script>\n\n```go\nprintln(\"ok\")\n```",
		"theme_mode":    "light",
		"theme_palette": "violet",
	})
	snapResp, err := http.Post(
		ts.URL+"/api/snapshots",
		"application/json",
		bytes.NewReader(snapReqBytes),
	)
	if err != nil || snapResp.StatusCode != http.StatusOK {
		t.Fatalf("POST /api/snapshots failed: err=%v resp=%v", err, snapResp)
	}
	defer snapResp.Body.Close()
	var snapBody struct {
		ID  string `json:"id"`
		URL string `json:"url"`
	}
	_ = json.NewDecoder(snapResp.Body).Decode(&snapBody)
	if snapBody.ID == "" || !strings.HasPrefix(snapBody.URL, "/s/") {
		t.Fatalf("unexpected snapshot body: %+v", snapBody)
	}

	pageResp, err := http.Get(ts.URL + snapBody.URL)
	if err != nil || pageResp.StatusCode != http.StatusOK {
		t.Fatalf("GET snapshot page failed: err=%v resp=%v", err, pageResp)
	}
	defer pageResp.Body.Close()
	rawHTML, _ := io.ReadAll(pageResp.Body)
	htmlStr := string(rawHTML)
	if strings.Contains(htmlStr, "<script>alert(1)</script>") || !strings.Contains(htmlStr, "&lt;script&gt;alert(1)&lt;/script&gt;") {
		t.Fatalf("snapshot did not escape script tag safely: %s", htmlStr)
	}
}

func TestDeleteConversationsLocalAndAGY(t *testing.T) {
	mux, store, _ := newTestServer(t)
	if err := store.EnsureLocalConversation("local-del", "Chat Local Apagar", "ollama:qwen3:8b"); err != nil {
		t.Fatal(err)
	}
	agyBrain := os.Getenv("AGY_BRAIN_DIR")
	logsDir := filepath.Join(agyBrain, "agy-del-1", ".system_generated", "logs")
	if err := os.MkdirAll(logsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(logsDir, "transcript.jsonl"), []byte("{\"type\":\"USER_INPUT\",\"content\":\"oi\"}\n{\"type\":\"PLANNER_RESPONSE\",\"content\":\"ola\"}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	ts := httptest.NewServer(mux)
	defer ts.Close()

	// 1. Delete local conversation
	delLocalReq, _ := http.NewRequest(http.MethodDelete, ts.URL+"/api/conversations/local-del", nil)
	delLocalResp, err := http.DefaultClient.Do(delLocalReq)
	if err != nil || delLocalResp.StatusCode != http.StatusOK {
		t.Fatalf("DELETE local failed: err=%v resp=%v", err, delLocalResp)
	}
	var localPayload map[string]any
	_ = json.NewDecoder(delLocalResp.Body).Decode(&localPayload)
	_ = delLocalResp.Body.Close()
	if localPayload["deleted_local"] != true {
		t.Fatalf("expected deleted_local=true, got %+v", localPayload)
	}
	getLocalResp, _ := http.Get(ts.URL + "/api/conversations/local-del")
	_ = getLocalResp.Body.Close()
	if getLocalResp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 after local delete, got %d", getLocalResp.StatusCode)
	}

	// 2. Delete AGY conversation
	delAGYReq, _ := http.NewRequest(http.MethodDelete, ts.URL+"/api/conversations/agy-del-1", nil)
	delAGYResp, err := http.DefaultClient.Do(delAGYReq)
	if err != nil || delAGYResp.StatusCode != http.StatusOK {
		t.Fatalf("DELETE agy failed: err=%v resp=%v", err, delAGYResp)
	}
	var agyPayload map[string]any
	_ = json.NewDecoder(delAGYResp.Body).Decode(&agyPayload)
	_ = delAGYResp.Body.Close()
	if agyPayload["deleted_agy"] != true {
		t.Fatalf("expected deleted_agy=true, got %+v", agyPayload)
	}
	getAGYResp, _ := http.Get(ts.URL + "/api/conversations/agy-del-1")
	_ = getAGYResp.Body.Close()
	if getAGYResp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 after agy delete, got %d", getAGYResp.StatusCode)
	}

	// 3. Reject encoded path separator / traversal id
	badReq, _ := http.NewRequest(http.MethodDelete, ts.URL+"/api/conversations/..%5Cescape", nil)
	badResp, err := http.DefaultClient.Do(badReq)
	if err != nil {
		t.Fatal(err)
	}
	_ = badResp.Body.Close()
	if badResp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid id '..\\\\escape', got %d", badResp.StatusCode)
	}
}

func TestCockpitOptionalProxy(t *testing.T) {
	// 1. Default Core keeps cockpit optional package disabled
	muxOff, _, _ := newTestServer(t)
	featRec := httptest.NewRecorder()
	muxOff.ServeHTTP(featRec, httptest.NewRequest(http.MethodGet, "/api/features", nil))
	var featPayload struct {
		Features map[string]any `json:"features"`
	}
	_ = json.Unmarshal(featRec.Body.Bytes(), &featPayload)
	if enabled, _ := featPayload.Features["cockpit"].(bool); enabled {
		t.Fatalf("expected features.cockpit=false by default, got %+v", featPayload.Features)
	}

	// 2. Enabled proxy forwards path & Bearer token
	var gotPath, gotAuth string
	mockCockpit := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"hostname":"pc-test","allow_power":true}`)
	}))
	defer mockCockpit.Close()

	muxOn, _, _ := newTestServerWithEnv(t, map[string]string{
		"ENABLE_COCKPIT": "true",
		"COCKPIT_URL":    mockCockpit.URL,
		"COCKPIT_TOKEN":  "bridge-secret",
	})
	rec := httptest.NewRecorder()
	muxOn.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/cockpit/overview", nil))
	if rec.Code != http.StatusOK || gotPath != "/overview" || gotAuth != "Bearer bridge-secret" {
		t.Fatalf("proxy mismatch: status=%d path=%q auth=%q body=%s", rec.Code, gotPath, gotAuth, rec.Body.String())
	}

	// 3. Offline optional backend returns clean 502 JSON
	muxDown, _, _ := newTestServerWithEnv(t, map[string]string{
		"ENABLE_COCKPIT": "true",
		"COCKPIT_URL":    "http://127.0.0.1:1",
	})
	downRec := httptest.NewRecorder()
	muxDown.ServeHTTP(downRec, httptest.NewRequest(http.MethodGet, "/api/cockpit/overview", nil))
	if downRec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502 when cockpit unreachable, got %d", downRec.Code)
	}
}
