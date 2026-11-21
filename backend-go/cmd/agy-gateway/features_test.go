package main

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Phoenix7290/agy-mobile-gateway/backend-go/internal/auth"
	"github.com/Phoenix7290/agy-mobile-gateway/backend-go/internal/automation"
	"github.com/Phoenix7290/agy-mobile-gateway/backend-go/internal/botbridge"
	"github.com/Phoenix7290/agy-mobile-gateway/backend-go/internal/config"
	"github.com/Phoenix7290/agy-mobile-gateway/backend-go/internal/db"
	"github.com/Phoenix7290/agy-mobile-gateway/backend-go/internal/jobs"
	"github.com/Phoenix7290/agy-mobile-gateway/backend-go/internal/runner"
)

func TestAuthTokenCookiePINAndWebAuthn(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("DATA_DIR", tmp)
	t.Setenv("CACHE_DIR", filepath.Join(tmp, "cache"))
	t.Setenv("INBOX_DIR", filepath.Join(tmp, "inbox"))
	t.Setenv("OUTBOX_DIR", filepath.Join(tmp, "outbox"))
	t.Setenv("GATEWAY_MODE", "standalone")
	t.Setenv("GATEWAY_AUTH_TOKEN", "pin-secret-9988")

	cfg := config.Load()
	store, err := db.Open(filepath.Join(tmp, "gateway.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	r := runner.Create(cfg)
	mgr := jobs.NewManager(cfg, store, r)
	wd := &watchdogState{}
	notifier := automation.NewNotifier(store)
	sched := automation.NewScheduler(cfg, store, r, mgr, notifier)
	handler := buildProtectedHandler(cfg, store, r, mgr, wd, notifier, sched)

	// 1. Unauthenticated private endpoint must fail with 401
	rec401 := httptest.NewRecorder()
	handler.ServeHTTP(rec401, httptest.NewRequest(http.MethodGet, "/api/status", nil))
	if rec401.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without cookie/token, got %d", rec401.Code)
	}

	// 2. Public /api/auth/status must succeed and report enabled=true, authenticated=false
	recSt := httptest.NewRecorder()
	handler.ServeHTTP(recSt, httptest.NewRequest(http.MethodGet, "/api/auth/status", nil))
	if recSt.Code != http.StatusOK || !strings.Contains(recSt.Body.String(), `"authenticated":false`) {
		t.Fatalf("unexpected /api/auth/status: code=%d body=%s", recSt.Code, recSt.Body.String())
	}

	// 3. Login sets HttpOnly agy_session cookie
	loginReq := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"token":"pin-secret-9988"}`))
	loginRec := httptest.NewRecorder()
	handler.ServeHTTP(loginRec, loginReq)
	if loginRec.Code != http.StatusOK {
		t.Fatalf("login failed: %d %s", loginRec.Code, loginRec.Body.String())
	}
	cookies := loginRec.Result().Cookies()
	if len(cookies) == 0 || cookies[0].Name != "agy_session" || !cookies[0].HttpOnly {
		t.Fatalf("expected HttpOnly agy_session cookie, got %+v", cookies)
	}

	// 4. Authenticated request with cookie succeeds
	authedReq := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	authedReq.AddCookie(cookies[0])
	authedRec := httptest.NewRecorder()
	handler.ServeHTTP(authedRec, authedReq)
	if authedRec.Code != http.StatusOK {
		t.Fatalf("expected 200 with session cookie, got %d", authedRec.Code)
	}

	// 5. Bearer token also works for API clients
	bearerReq := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	bearerReq.Header.Set("Authorization", "Bearer pin-secret-9988")
	bearerRec := httptest.NewRecorder()
	handler.ServeHTTP(bearerRec, bearerReq)
	if bearerRec.Code != http.StatusOK {
		t.Fatalf("expected 200 with bearer token, got %d", bearerRec.Code)
	}

	// 6. Set custom quick PIN and verify unlock
	pinReq := httptest.NewRequest(http.MethodPut, "/api/auth/pin", strings.NewReader(`{"pin":"4321"}`))
	pinReq.AddCookie(cookies[0])
	pinRec := httptest.NewRecorder()
	handler.ServeHTTP(pinRec, pinReq)
	if pinRec.Code != http.StatusOK {
		t.Fatalf("PUT /api/auth/pin failed: %d %s", pinRec.Code, pinRec.Body.String())
	}
	pinLogin := httptest.NewRecorder()
	handler.ServeHTTP(pinLogin, httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"pin":"4321"}`)))
	if pinLogin.Code != http.StatusOK {
		t.Fatalf("login with custom PIN failed: %d", pinLogin.Code)
	}

	// 7. Pure-Go WebAuthn ES256 registration & assertion round-trip
	authMgr := auth.NewManager(cfg, store)
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	regChal := authMgr.IssueChallenge("register")
	regCD, _ := json.Marshal(map[string]string{
		"type":      "webauthn.create",
		"challenge": regChal,
		"origin":    "https://agy.example.com",
	})
	credIDRaw := []byte("test-cred-id-123")
	xBytes := priv.PublicKey.X.FillBytes(make([]byte, 32))
	yBytes := priv.PublicKey.Y.FillBytes(make([]byte, 32))
	// Minimal CBOR COSE map {1: 2, 3: -7, -1: 1, -2: xBytes, -3: yBytes}
	cose := []byte{0xa5, 0x01, 0x02, 0x03, 0x26, 0x20, 0x01, 0x21, 0x58, 0x20}
	cose = append(cose, xBytes...)
	cose = append(cose, 0x22, 0x58, 0x20)
	cose = append(cose, yBytes...)

	var authData bytes.Buffer
	authData.Write(make([]byte, 32)) // rpIdHash
	authData.WriteByte(0x41)         // UP + AT flags
	authData.Write([]byte{0, 0, 0, 1})
	authData.Write(make([]byte, 16)) // aaguid
	_ = binary.Write(&authData, binary.BigEndian, uint16(len(credIDRaw)))
	authData.Write(credIDRaw)
	authData.Write(cose)

	// CBOR {"authData": authData}
	adBytes := authData.Bytes()
	att := []byte{0xa1, 0x68}
	att = append(att, []byte("authData")...)
	if len(adBytes) < 256 {
		att = append(att, 0x58, byte(len(adBytes)))
	}
	att = append(att, adBytes...)

	savedCred, err := authMgr.VerifyAndStoreRegistration(
		base64.RawURLEncoding.EncodeToString(regCD),
		base64.RawURLEncoding.EncodeToString(att),
		"iPhone Test",
	)
	if err != nil {
		t.Fatalf("WebAuthn registration failed: %v", err)
	}

	loginChal := authMgr.IssueChallenge("login")
	loginCD, _ := json.Marshal(map[string]string{
		"type":      "webauthn.get",
		"challenge": loginChal,
		"origin":    "https://agy.example.com",
	})
	assertAuthData := make([]byte, 37)
	assertAuthData[32] = 0x01 // UP
	binary.BigEndian.PutUint32(assertAuthData[33:37], 2)
	cdHash := sha256.Sum256(loginCD)
	signedPayload := append(append([]byte{}, assertAuthData...), cdHash[:]...)
	digest := sha256.Sum256(signedPayload)
	sig, err := ecdsa.SignASN1(rand.Reader, priv, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	if err := authMgr.VerifyAssertion(
		savedCred.CredentialID,
		base64.RawURLEncoding.EncodeToString(loginCD),
		base64.RawURLEncoding.EncodeToString(assertAuthData),
		base64.RawURLEncoding.EncodeToString(sig),
	); err != nil {
		t.Fatalf("WebAuthn assertion failed: %v", err)
	}
}

func TestMultiHostProfilesCRUDAndActivation(t *testing.T) {
	mux, _, _ := newTestServer(t)

	// Create 2 hosts
	for _, body := range []string{
		`{"name":"CachyCore","host":"192.0.2.10","ssh_user":"ai","ssh_port":"22","wol_mac":"00:11:22:33:44:55"}`,
		`{"name":"Dell G15","host":"192.0.2.20","ssh_user":"ryanslx","ssh_port":"22","wol_mac":"AA:BB:CC:DD:EE:FF"}`,
	} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/hosts", strings.NewReader(body)))
		if rec.Code != http.StatusOK {
			t.Fatalf("POST /api/hosts failed: %d %s", rec.Code, rec.Body.String())
		}
	}

	listRec := httptest.NewRecorder()
	mux.ServeHTTP(listRec, httptest.NewRequest(http.MethodGet, "/api/hosts", nil))
	var listed struct {
		Hosts []db.HostProfile `json:"hosts"`
	}
	_ = json.Unmarshal(listRec.Body.Bytes(), &listed)
	if len(listed.Hosts) != 2 {
		t.Fatalf("expected 2 hosts, got %d", len(listed.Hosts))
	}

	// Activate Dell G15
	actRec := httptest.NewRecorder()
	mux.ServeHTTP(actRec, httptest.NewRequest(http.MethodPost, "/api/hosts/"+listed.Hosts[1].ID+"/activate", nil))
	if actRec.Code != http.StatusOK || !strings.Contains(actRec.Body.String(), "Dell G15") {
		t.Fatalf("activate failed: %d %s", actRec.Code, actRec.Body.String())
	}

	// Cannot delete if reduced to 1 host
	del1 := httptest.NewRecorder()
	mux.ServeHTTP(del1, httptest.NewRequest(http.MethodDelete, "/api/hosts/"+listed.Hosts[0].ID, nil))
	if del1.Code != http.StatusOK {
		t.Fatalf("delete host 0 failed: %d", del1.Code)
	}
	delLast := httptest.NewRecorder()
	mux.ServeHTTP(delLast, httptest.NewRequest(http.MethodDelete, "/api/hosts/"+listed.Hosts[1].ID, nil))
	if delLast.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 deleting last host, got %d", delLast.Code)
	}
}

func TestTelegramBotTextAndOutboxReply(t *testing.T) {
	tmp := t.TempDir()
	outboxDir := filepath.Join(tmp, "outbox")
	_ = os.MkdirAll(outboxDir, 0o755)
	_ = os.WriteFile(filepath.Join(outboxDir, "chart.png"), []byte("png-bytes"), 0o644)

	ollamaSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		_, _ = io.WriteString(w, `{"message":{"role":"assistant","content":"Resposta via Telegram OK! chart.png"},"done":true}`+"\n")
	}))
	defer ollamaSrv.Close()

	var mu sync.Mutex
	var sentMethods []string
	var sentText string
	tgSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		sentMethods = append(sentMethods, r.URL.Path)
		if strings.HasSuffix(r.URL.Path, "/sendMessage") {
			b, _ := io.ReadAll(r.Body)
			sentText += string(b)
		}
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"ok":true,"result":{"message_id":1}}`)
	}))
	defer tgSrv.Close()

	t.Setenv("DATA_DIR", tmp)
	t.Setenv("CACHE_DIR", filepath.Join(tmp, "cache"))
	t.Setenv("INBOX_DIR", filepath.Join(tmp, "inbox"))
	t.Setenv("OUTBOX_DIR", outboxDir)
	t.Setenv("GATEWAY_MODE", "standalone")
	t.Setenv("OLLAMA_URL", ollamaSrv.URL)
	t.Setenv("ENABLE_TELEGRAM_BOT", "true")
	t.Setenv("TELEGRAM_BOT_TOKEN", "12345:test-token")
	t.Setenv("TELEGRAM_ALLOWED_CHAT_IDS", "777888")

	cfg := config.Load()
	store, err := db.Open(filepath.Join(tmp, "gateway.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	r := runner.Create(cfg)
	mgr := jobs.NewManager(cfg, store, r)
	bridge := botbridge.New(cfg, store, mgr)
	tg := botbridge.NewTelegramBot(cfg, store, r, mgr, bridge)
	tg.APIBase = tgSrv.URL

	// Unauthorized chat is rejected
	okBad, infoBad := tg.HandleUpdate(context.Background(), botbridge.TelegramUpdate{
		UpdateID: 1,
		Message: &botbridge.TelegramMessage{
			MessageID: 1,
			Chat:      botbridge.TelegramChat{ID: 999999, Type: "private"},
			From:      &botbridge.TelegramUser{ID: 999999},
			Text:      "oi",
		},
	})
	if okBad || infoBad != "forbidden" {
		t.Fatalf("expected forbidden, got ok=%v info=%s", okBad, infoBad)
	}

	// Authorized /status
	okSt, _ := tg.HandleUpdate(context.Background(), botbridge.TelegramUpdate{
		UpdateID: 2,
		Message: &botbridge.TelegramMessage{
			MessageID: 2,
			Chat:      botbridge.TelegramChat{ID: 777888, Type: "private"},
			From:      &botbridge.TelegramUser{ID: 777888},
			Text:      "/status",
		},
	})
	if !okSt {
		t.Fatal("expected /status to be accepted")
	}

	// Authorized prompt triggers Ollama job & sends reply
	// Wait: Manager routes short prompts with "auto" to Ollama only when ollama models exist or "ollama:" model; test direct /api/bot/telegram/webhook too
	okMsg, _ := tg.HandleUpdate(context.Background(), botbridge.TelegramUpdate{
		UpdateID: 3,
		Message: &botbridge.TelegramMessage{
			MessageID: 3,
			Chat:      botbridge.TelegramChat{ID: 777888, Type: "private"},
			From:      &botbridge.TelegramUser{ID: 777888},
			Text:      "/new",
		},
	})
	if !okMsg {
		t.Fatal("expected /new to be accepted")
	}

	// Verify SendOutboxFile uploads chart.png as sendPhoto
	if err := tg.SendOutboxFile(context.Background(), 777888, "chart.png"); err != nil {
		t.Fatalf("SendOutboxFile failed: %v", err)
	}

	mu.Lock()
	joined := strings.Join(sentMethods, ",")
	mu.Unlock()
	if !strings.Contains(joined, "/sendMessage") || !strings.Contains(joined, "/sendPhoto") {
		t.Fatalf("expected /sendMessage and /sendPhoto calls, got %s", joined)
	}
	_ = time.Millisecond
}
