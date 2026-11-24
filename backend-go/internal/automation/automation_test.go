package automation

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Phoenix7290/agy-mobile-gateway/backend-go/internal/config"
	"github.com/Phoenix7290/agy-mobile-gateway/backend-go/internal/db"
	"github.com/Phoenix7290/agy-mobile-gateway/backend-go/internal/jobs"
	"github.com/Phoenix7290/agy-mobile-gateway/backend-go/internal/runner"
)

func TestCronParseMatchesAndNextAfter(t *testing.T) {
	sched, err := ParseCron("0 8 * * 1-5")
	if err != nil {
		t.Fatalf("ParseCron failed: %v", err)
	}
	mon0800 := time.Date(2026, 11, 16, 8, 0, 0, 0, time.UTC) // Monday
	sun0800 := time.Date(2026, 11, 15, 8, 0, 0, 0, time.UTC) // Sunday
	if !sched.Matches(mon0800) {
		t.Fatal("expected Monday 08:00 to match")
	}
	if sched.Matches(sun0800) {
		t.Fatal("expected Sunday 08:00 NOT to match 1-5")
	}

	next, ok := sched.NextAfter(time.Date(2026, 11, 13, 9, 0, 0, 0, time.UTC)) // Friday 09:00 -> Monday 08:00
	if !ok || !next.Equal(mon0800) {
		t.Fatalf("expected next run Monday 08:00 UTC, got %v (ok=%v)", next, ok)
	}

	stepSched, err := ParseCron("*/15 * * * *")
	if err != nil {
		t.Fatalf("ParseCron step failed: %v", err)
	}
	if !stepSched.Matches(time.Date(2026, 11, 16, 10, 30, 0, 0, time.UTC)) {
		t.Fatal("expected :30 to match */15")
	}

	if _, err := ParseCron("@daily"); err != nil {
		t.Fatalf("expected @daily alias valid, got %v", err)
	}
	if _, err := ParseCron("0 25 * * *"); err == nil {
		t.Fatal("expected error for hour 25")
	}
}

func decryptWebPushForTest(t *testing.T, uaPriv *ecdh.PrivateKey, authSecret, wire []byte) string {
	t.Helper()
	if len(wire) < 22 {
		t.Fatalf("wire too short: %d", len(wire))
	}
	salt := wire[:16]
	rs := binary.BigEndian.Uint32(wire[16:20])
	if rs != 4096 {
		t.Fatalf("unexpected record size %d", rs)
	}
	idLen := int(wire[20])
	asPubBytes := wire[21 : 21+idLen]
	ciphertext := wire[21+idLen:]

	asPub, err := ecdh.P256().NewPublicKey(asPubBytes)
	if err != nil {
		t.Fatal(err)
	}
	ecdhSecret, err := uaPriv.ECDH(asPub)
	if err != nil {
		t.Fatal(err)
	}
	uaPubBytes := uaPriv.PublicKey().Bytes()
	keyInfo := append(append([]byte("WebPush: info\x00"), uaPubBytes...), asPubBytes...)
	prkKey := hkdfExtract(authSecret, ecdhSecret)
	ikm := hkdfExpand(prkKey, keyInfo, 32)
	prk := hkdfExtract(salt, ikm)
	cek := hkdfExpand(prk, []byte("Content-Encoding: aes128gcm\x00"), 16)
	nonce := hkdfExpand(prk, []byte("Content-Encoding: nonce\x00"), 12)

	block, err := aes.NewCipher(cek)
	if err != nil {
		t.Fatal(err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatal(err)
	}
	plainPadded, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		t.Fatalf("aes128gcm decrypt failed: %v", err)
	}
	if plainPadded[len(plainPadded)-1] != 0x02 {
		t.Fatalf("missing 0x02 record delimiter: %v", plainPadded)
	}
	return string(plainPadded[:len(plainPadded)-1])
}

func TestWebPushRFC8291RoundTripAndNotifier(t *testing.T) {
	store, err := db.Open(filepath.Join(t.TempDir(), "gateway.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	uaPriv, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	authSecret := make([]byte, 16)
	_, _ = rand.Read(authSecret)

	keys, err := EnsureVAPIDKeys(store)
	if err != nil {
		t.Fatalf("EnsureVAPIDKeys failed: %v", err)
	}
	keys2, err := EnsureVAPIDKeys(store)
	if err != nil || keys.PublicKey != keys2.PublicKey {
		t.Fatalf("expected persisted VAPID public key, got %+v vs %+v", keys, keys2)
	}

	var mu sync.Mutex
	var gotCipher []byte
	var gotAuthHeader string
	var gotUrgency string
	var gotNtfyTitle, gotNtfyMsg string

	pushSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if r.URL.Path == "/gone" {
			w.WriteHeader(http.StatusGone)
			return
		}
		gotAuthHeader = r.Header.Get("Authorization")
		gotUrgency = r.Header.Get("Urgency")
		gotCipher, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusCreated)
	}))
	defer pushSrv.Close()

	ntfySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		b, _ := io.ReadAll(r.Body)
		gotNtfyTitle = r.Header.Get("Title")
		gotNtfyMsg = string(b)
		w.WriteHeader(http.StatusOK)
	}))
	defer ntfySrv.Close()

	sub1, _ := json.Marshal(map[string]any{
		"endpoint": pushSrv.URL + "/ok",
		"keys": map[string]string{
			"p256dh": base64.RawURLEncoding.EncodeToString(uaPriv.PublicKey().Bytes()),
			"auth":   base64.RawURLEncoding.EncodeToString(authSecret),
		},
	})
	subGone, _ := json.Marshal(map[string]any{
		"endpoint": pushSrv.URL + "/gone",
		"keys": map[string]string{
			"p256dh": base64.RawURLEncoding.EncodeToString(uaPriv.PublicKey().Bytes()),
			"auth":   base64.RawURLEncoding.EncodeToString(authSecret),
		},
	})
	_ = store.UpsertPushSubscription(pushSrv.URL+"/ok", string(sub1))
	_ = store.UpsertPushSubscription(pushSrv.URL+"/gone", string(subGone))

	acfg := db.DefaultAlertConfig()
	acfg.NtfyURL = ntfySrv.URL
	acfg.NtfyTopic = "test-topic"
	acfg.WebPushEnabled = true
	acfg.NotifyDesktop = true
	_, _ = store.SaveAlertConfig(acfg)

	notifier := NewNotifier(store)
	desktopCalls := 0
	notifier.desktopNotifyFn = func(_ context.Context, _, _, _ string) (bool, error) {
		desktopCalls++
		return true, nil
	}
	liveCh, unsubLive := notifier.Subscribe()
	defer unsubLive()

	res := notifier.Dispatch(context.Background(), AlertInput{
		Source:   "test",
		Severity: "info",
		Title:    "Teste AGY",
		Message:  "Alerta ok",
	})
	if !res.NtfySent || res.PushSent != 1 || res.PushFailed != 1 || !res.DesktopSent || res.BrowserLiveSent != 1 || desktopCalls != 1 {
		t.Fatalf("unexpected dispatch result: %+v desktopCalls=%d", res, desktopCalls)
	}
	select {
	case liveEv := <-liveCh:
		if liveEv.Title != "Teste AGY" {
			t.Fatalf("unexpected live event: %+v", liveEv)
		}
	default:
		t.Fatal("expected live SSE alert event in subscriber channel")
	}

	mu.Lock()
	defer mu.Unlock()
	if gotNtfyTitle != "Teste AGY" || gotNtfyMsg != "Alerta ok" {
		t.Fatalf("unexpected ntfy delivery: title=%q msg=%q", gotNtfyTitle, gotNtfyMsg)
	}
	if !strings.HasPrefix(gotAuthHeader, "vapid t=") || !strings.Contains(gotAuthHeader, ", k="+keys.PublicKey) {
		t.Fatalf("unexpected VAPID header: %q", gotAuthHeader)
	}
	if gotUrgency != "high" {
		t.Fatalf("expected Urgency=high header for mobile delivery, got %q", gotUrgency)
	}
	if jwtPart := strings.TrimPrefix(strings.Split(gotAuthHeader, ",")[0], "vapid t="); strings.Count(jwtPart, ".") == 2 {
		claimSeg := strings.Split(jwtPart, ".")[1]
		if claimBytes, decErr := base64.RawURLEncoding.DecodeString(claimSeg); decErr != nil {
			t.Fatalf("failed to decode VAPID JWT claims: %v", decErr)
		} else if !strings.Contains(string(claimBytes), `"sub":"mailto:alerts@agy-gateway.app"`) {
			t.Fatalf("expected APNs-compatible sub in VAPID JWT claims, got %s", string(claimBytes))
		}
	}
	if res.PushError == "" {
		t.Fatalf("expected PushError populated from 410 Gone endpoint, got empty")
	}
	plain := decryptWebPushForTest(t, uaPriv, authSecret, gotCipher)
	if !strings.Contains(plain, `"title":"Teste AGY"`) {
		t.Fatalf("unexpected decrypted push payload: %s", plain)
	}

	subsLeft, _ := store.ListPushSubscriptions()
	if len(subsLeft) != 1 {
		t.Fatalf("expected 410 Gone subscription pruned leaving 1, got %d", len(subsLeft))
	}

	notifier.desktopNotifyFn = func(_ context.Context, _, _, _ string) (bool, error) {
		return false, errors.New("toast service disabled")
	}
	errRes := notifier.Dispatch(context.Background(), AlertInput{
		Source:   "test",
		Severity: "warning",
		Title:    "Erro OS",
		Message:  "Falha simulada",
	})
	if errRes.DesktopSent || !strings.Contains(errRes.DesktopError, "toast service disabled") {
		t.Fatalf("expected DesktopError populated on failure, got %+v", errRes)
	}
}

type fakeAlertRunner struct {
	runner.Runner
	tel runner.TelemetryData
}

func (f *fakeAlertRunner) GetTelemetry(_ context.Context) runner.TelemetryData {
	return f.tel
}

func TestSchedulerHardwareDockerWatchersAndCronCheck(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("DATA_DIR", tmp)
	t.Setenv("INBOX_DIR", filepath.Join(tmp, "inbox"))
	t.Setenv("OUTBOX_DIR", filepath.Join(tmp, "outbox"))

	cfg := config.Load()
	store, err := db.Open(filepath.Join(tmp, "gateway.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	_, _ = store.SaveAlertConfig(db.AlertConfig{
		NotifyJobDone:     true,
		NotifyLongJobSec:  30,
		NotifyDockerDown:  true,
		GPUTempThresholdC: 80,
		DiskFreeMinPct:    15,
	})

	fr := &fakeAlertRunner{
		Runner: runner.NewLocalRunner(cfg),
		tel: runner.TelemetryData{
			GPU: &runner.GPUInfo{Name: "RTX 4060 Ti", TempC: 84, VRAMUsedMB: 4000, VRAMTotalMB: 8192},
		},
	}
	mgr := jobs.NewManager(cfg, store, fr)
	notifier := NewNotifier(store)
	notifier.desktopNotifyFn = nil
	sched := NewScheduler(cfg, store, fr, mgr, notifier)

	dockerStates := []DockerState{
		{Name: "postgres", State: "running", Status: "Up 1h"},
	}
	sched.dockerListFn = func(_ context.Context) (bool, []DockerState) {
		return true, dockerStates
	}
	sched.diskFn = func(path string) (DiskSpaceInfo, error) {
		return DiskSpaceInfo{Path: path, FreeGB: 5.0, TotalGB: 100.0, FreePct: 5.0}, nil
	}

	// First tick seeds Docker state and triggers GPU + Disk alerts
	sched.TickWatchers(context.Background())

	// Simulate container crash -> second tick fires Docker drop alert (GPU/Disk respect cooldown!)
	dockerStates = []DockerState{
		{Name: "postgres", State: "exited", Status: "Exited (137)"},
	}
	sched.TickWatchers(context.Background())

	// Check Cron native check automation + long job alert
	_, _ = store.CreateAutomation(db.Automation{
		ID:       "auto-check",
		Name:     "Check Diário",
		CronExpr: "0 8 * * *",
		Kind:     "check",
		Prompt:   "disco e docker",
		NotifyOn: "always",
		Enabled:  true,
	})
	if _, err := sched.RunAutomationNow(context.Background(), "auto-check"); err != nil {
		t.Fatalf("RunAutomationNow failed: %v", err)
	}
	time.Sleep(80 * time.Millisecond)

	notifier.HandleJobFinish(jobs.JobFinishNotice{
		JobID:          "job-99",
		ConversationID: "conv-abc",
		Model:          "gemini-4-hydrogen-high",
		Status:         "completed",
		Summary:        "Refatoração pronta.",
		Elapsed:        65 * time.Second,
	})

	events, unread, err := store.ListAlertEvents(20)
	if err != nil {
		t.Fatal(err)
	}
	sources := map[string]int{}
	for _, ev := range events {
		sources[ev.Source]++
	}
	if sources["gpu"] != 1 || sources["disk"] != 1 || sources["docker"] != 1 || sources["cron"] != 1 || sources["job"] != 1 {
		t.Fatalf("expected 1 alert per source (with GPU/Disk cooldown), got %+v (unread=%d)", sources, unread)
	}
}

func TestSchedulerEvolutionOffPeakCronTrigger(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := db.Open(filepath.Join(tmpDir, "gateway.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	cfg := config.GatewaySettings{
		Mode:          "standalone",
		EvolutionCron: "0 3 * * *",
		Dirs:          config.AppDirs{DataDir: tmpDir},
		Mods:          map[string]any{"evolution": true},
	}
	notifier := NewNotifier(store)
	sched := NewScheduler(cfg, store, nil, nil, notifier)

	triggered := make(chan struct{}, 4)
	sched.OnEvolutionTick = func(_ context.Context) {
		triggered <- struct{}{}
	}

	// Non-matching time (14:20) does not fire evolution
	sched.nowFn = func() time.Time { return time.Date(2026, 11, 21, 14, 20, 0, 0, time.UTC) }
	sched.TickCron(context.Background())
	select {
	case <-triggered:
		t.Fatalf("did not expect off-peak evolution at 14:20")
	case <-time.After(30 * time.Millisecond):
	}

	// Matching off-peak time (03:00) fires once per minute even if TickCron polls every 20s
	sched.nowFn = func() time.Time { return time.Date(2026, 11, 21, 3, 0, 10, 0, time.UTC) }
	sched.TickCron(context.Background())
	sched.TickCron(context.Background())

	select {
	case <-triggered:
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("expected OnEvolutionTick at 03:00")
	}
	select {
	case <-triggered:
		t.Fatalf("did not expect duplicate OnEvolutionTick within the same minute")
	case <-time.After(40 * time.Millisecond):
	}
}
