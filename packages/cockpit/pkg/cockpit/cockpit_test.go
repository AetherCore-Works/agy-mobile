package cockpit

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type fakeCollector struct {
	sample       TelemetryPoint
	procs        []ProcessInfo
	dockerOK     bool
	containers   []DockerContainer
	lastActionID string
	lastAction   string
	logsText     string
}

func (f *fakeCollector) Sample(_ context.Context) TelemetryPoint {
	return f.sample
}

func (f *fakeCollector) TopProcesses(_ context.Context) []ProcessInfo {
	return f.procs
}

func (f *fakeCollector) ListContainers(_ context.Context) (bool, []DockerContainer) {
	return f.dockerOK, f.containers
}

func (f *fakeCollector) DockerAction(_ context.Context, id, action string) (string, error) {
	cleanID, err := ValidateContainerID(id)
	if err != nil {
		return "", err
	}
	f.lastActionID = cleanID
	f.lastAction = action
	return cleanID, nil
}

func (f *fakeCollector) DockerLogs(_ context.Context, id string, _ int) (string, error) {
	cleanID, err := ValidateContainerID(id)
	if err != nil {
		return "", err
	}
	return cleanID + ": " + f.logsText, nil
}

type fakePower struct {
	executed []string
}

func (p *fakePower) Execute(action string) error {
	p.executed = append(p.executed, action)
	return nil
}

func TestSamplerRingBufferBounds(t *testing.T) {
	col := &fakeCollector{}
	store := NewSamplerStore(col, 5*time.Second, 3, true)

	for i := 1; i <= 5; i++ {
		store.RecordSample(TelemetryPoint{RAMUsedMB: i * 100, RAMTotalMB: 1000, RAMPct: float64(i * 10)})
	}
	ov := store.Overview(context.Background(), false)
	if len(ov.History) != 3 {
		t.Fatalf("expected ring buffer cap 3, got %d", len(ov.History))
	}
	if ov.History[0].RAMUsedMB != 300 || ov.History[2].RAMUsedMB != 500 {
		t.Fatalf("unexpected ring buffer entries: %+v", ov.History)
	}
}

func TestCockpitAPIOverviewDockerPowerAndAuth(t *testing.T) {
	cpu := 28.5
	gpuTemp := 54.0
	col := &fakeCollector{
		sample: TelemetryPoint{
			Timestamp:   "2026-11-17T18:00:00Z",
			CPUPct:      &cpu,
			RAMUsedMB:   12288,
			RAMTotalMB:  32768,
			RAMPct:      37.5,
			GPUName:     "NVIDIA RTX 4060 Ti",
			GPUTempC:    &gpuTemp,
			VRAMUsedMB:  4096,
			VRAMTotalMB: 8192,
			VRAMPct:     50.0,
		},
		procs: []ProcessInfo{
			{PID: 42, Name: "ollama", RAMMB: 1800, VRAMMB: 3500},
		},
		dockerOK: true,
		containers: []DockerContainer{
			{ID: "abc123def456", Name: "agy-ttyd", Image: "agy-ttyd:custom", State: "running", Status: "Up 2h"},
		},
		logsText: "server listening",
	}
	pwr := &fakePower{}
	store := NewSamplerStore(col, 5*time.Second, 10, true)
	store.RecordSample(col.sample)

	h := NewHandler(ServerConfig{AllowPower: true, AuthToken: "secret-tok"}, store, col, pwr)
	ts := httptest.NewServer(h)
	defer ts.Close()

	// 1. Health stays accessible without token
	healthResp, err := http.Get(ts.URL + "/health")
	if err != nil || healthResp.StatusCode != http.StatusOK {
		t.Fatalf("GET /health failed: err=%v resp=%v", err, healthResp)
	}
	_ = healthResp.Body.Close()

	// 2. Overview rejects missing token
	unauthResp, err := http.Get(ts.URL + "/overview")
	if err != nil || unauthResp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 without token, got err=%v resp=%v", err, unauthResp)
	}
	_ = unauthResp.Body.Close()

	// 3. Overview with valid token
	ovReq, _ := http.NewRequest(http.MethodGet, ts.URL+"/overview", nil)
	ovReq.Header.Set("Authorization", "Bearer secret-tok")
	ovResp, err := http.DefaultClient.Do(ovReq)
	if err != nil || ovResp.StatusCode != http.StatusOK {
		t.Fatalf("GET /overview failed: err=%v resp=%v", err, ovResp)
	}
	var ov OverviewResponse
	_ = json.NewDecoder(ovResp.Body).Decode(&ov)
	_ = ovResp.Body.Close()
	if ov.Current.VRAMUsedMB != 4096 || len(ov.Processes) != 1 || len(ov.Containers) != 1 {
		t.Fatalf("unexpected overview payload: %+v", ov)
	}

	// 4. Reject invalid Docker container ID (flag injection)
	badDockerReq, _ := http.NewRequest(http.MethodPost, ts.URL+"/docker/--privileged/start", nil)
	badDockerReq.Header.Set("Authorization", "Bearer secret-tok")
	badDockerResp, _ := http.DefaultClient.Do(badDockerReq)
	_ = badDockerResp.Body.Close()
	if badDockerResp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid container id, got %d", badDockerResp.StatusCode)
	}

	// 5. Docker stop & logs
	stopReq, _ := http.NewRequest(http.MethodPost, ts.URL+"/docker/agy-ttyd/stop", nil)
	stopReq.Header.Set("Authorization", "Bearer secret-tok")
	stopResp, _ := http.DefaultClient.Do(stopReq)
	_ = stopResp.Body.Close()
	if stopResp.StatusCode != http.StatusOK || col.lastAction != "stop" || col.lastActionID != "agy-ttyd" {
		t.Fatalf("docker stop mismatch: status=%d action=%s id=%s", stopResp.StatusCode, col.lastAction, col.lastActionID)
	}

	logsReq, _ := http.NewRequest(http.MethodGet, ts.URL+"/docker/agy-ttyd/logs?tail=50", nil)
	logsReq.Header.Set("Authorization", "Bearer secret-tok")
	logsResp, _ := http.DefaultClient.Do(logsReq)
	var logsBody map[string]any
	_ = json.NewDecoder(logsResp.Body).Decode(&logsBody)
	_ = logsResp.Body.Close()
	if logsResp.StatusCode != http.StatusOK || !strings.Contains(logsBody["logs"].(string), "server listening") {
		t.Fatalf("unexpected docker logs: %+v", logsBody)
	}

	// 6. Power action rejects mismatched confirmation and executes valid confirmation
	badPwrReq, _ := http.NewRequest(http.MethodPost, ts.URL+"/power", bytes.NewBufferString(`{"action":"reboot","confirm":"YES"}`))
	badPwrReq.Header.Set("Authorization", "Bearer secret-tok")
	badPwrResp, _ := http.DefaultClient.Do(badPwrReq)
	_ = badPwrResp.Body.Close()
	if badPwrResp.StatusCode != http.StatusBadRequest || len(pwr.executed) != 0 {
		t.Fatalf("expected 400 on wrong confirmation, got %d executed=%v", badPwrResp.StatusCode, pwr.executed)
	}

	okPwrReq, _ := http.NewRequest(http.MethodPost, ts.URL+"/power", bytes.NewBufferString(`{"action":"suspend","confirm":"suspender"}`))
	okPwrReq.Header.Set("Authorization", "Bearer secret-tok")
	okPwrResp, _ := http.DefaultClient.Do(okPwrReq)
	_ = okPwrResp.Body.Close()
	if okPwrResp.StatusCode != http.StatusOK || len(pwr.executed) != 1 || pwr.executed[0] != "suspend" {
		t.Fatalf("expected suspend execution, got status=%d executed=%v", okPwrResp.StatusCode, pwr.executed)
	}

	// 7. Disabled power controls return 403
	hNoPower := NewHandler(ServerConfig{AllowPower: false}, store, col, pwr)
	tsNoPwr := httptest.NewServer(hNoPower)
	defer tsNoPwr.Close()
	forbidResp, _ := http.Post(tsNoPwr.URL+"/power", "application/json", bytes.NewBufferString(`{"action":"lock","confirm":"BLOQUEAR"}`))
	_ = forbidResp.Body.Close()
	if forbidResp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 when AllowPower=false, got %d", forbidResp.StatusCode)
	}
}
