package cockpit

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ErrInvalidContainerID indicates a container identifier failed strict character validation.
var ErrInvalidContainerID = errors.New("ID de container inválido")

var validContainerIDRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,127}$`)

// ValidateContainerID ensures Docker container IDs or names contain no CLI flags or path characters.
func ValidateContainerID(id string) (string, error) {
	trimmed := strings.TrimSpace(id)
	if !validContainerIDRe.MatchString(trimmed) {
		return "", ErrInvalidContainerID
	}
	return trimmed, nil
}

// Collector gathers host hardware telemetry, top processes, and Docker container state.
type Collector interface {
	Sample(ctx context.Context) TelemetryPoint
	TopProcesses(ctx context.Context) []ProcessInfo
	ListContainers(ctx context.Context) (bool, []DockerContainer)
	DockerAction(ctx context.Context, id, action string) (string, error)
	DockerLogs(ctx context.Context, id string, tail int) (string, error)
}

// SystemCollector implements Collector against the local OS, nvidia-smi, and Docker CLI.
type SystemCollector struct {
	mu           sync.Mutex
	prevCPUTotal uint64
	prevCPUIdle  uint64
}

// NewSystemCollector initializes a SystemCollector.
func NewSystemCollector() *SystemCollector {
	return &SystemCollector{}
}

func runHiddenCommand(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	configureOSProcess(cmd)
	return cmd.CombinedOutput()
}

func round1(v float64) float64 {
	return math.Round(v*10) / 10
}

func pct(used, total int) float64 {
	if total <= 0 {
		return 0
	}
	p := (float64(used) / float64(total)) * 100.0
	if p < 0 {
		p = 0
	}
	if p > 100 {
		p = 100
	}
	return round1(p)
}

// Sample collects a single point-in-time hardware telemetry snapshot.
func (c *SystemCollector) Sample(ctx context.Context) TelemetryPoint {
	pt := TelemetryPoint{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}

	c.collectGPU(ctx, &pt)

	switch runtime.GOOS {
	case "windows":
		c.collectWindowsMetrics(ctx, &pt)
	case "linux":
		c.collectLinuxMetrics(&pt)
	case "darwin":
		c.collectDarwinMetrics(ctx, &pt)
	}

	pt.RAMPct = pct(pt.RAMUsedMB, pt.RAMTotalMB)
	pt.VRAMPct = pct(pt.VRAMUsedMB, pt.VRAMTotalMB)
	return pt
}

func (c *SystemCollector) collectGPU(ctx context.Context, pt *TelemetryPoint) {
	cctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	out, err := runHiddenCommand(
		cctx,
		"nvidia-smi",
		"--query-gpu=name,memory.used,memory.total,temperature.gpu",
		"--format=csv,noheader,nounits",
	)
	if err != nil {
		return
	}
	line := strings.Split(strings.TrimSpace(string(out)), "\n")[0]
	parts := strings.Split(line, ",")
	if len(parts) < 4 {
		return
	}
	used, err1 := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	tot, err2 := strconv.ParseFloat(strings.TrimSpace(parts[2]), 64)
	temp, err3 := strconv.ParseFloat(strings.TrimSpace(parts[3]), 64)
	if err1 != nil || err2 != nil {
		return
	}
	pt.GPUName = strings.TrimSpace(parts[0])
	pt.VRAMUsedMB = int(math.Round(used))
	pt.VRAMTotalMB = int(math.Round(tot))
	if err3 == nil {
		t := round1(temp)
		pt.GPUTempC = &t
	}
}

func (c *SystemCollector) collectWindowsMetrics(ctx context.Context, pt *TelemetryPoint) {
	cctx, cancel := context.WithTimeout(ctx, 3500*time.Millisecond)
	defer cancel()
	script := `$os=Get-CimInstance Win32_OperatingSystem;$cpu=(Get-CimInstance Win32_PerfFormattedData_PerfOS_Processor -Filter "Name='_Total'").PercentProcessorTime;[PSCustomObject]@{ram_total_mb=[int][math]::Round($os.TotalVisibleMemorySize/1024);ram_free_mb=[int][math]::Round($os.FreePhysicalMemory/1024);cpu_pct=[double]$cpu}|ConvertTo-Json -Compress`
	out, err := runHiddenCommand(cctx, "powershell", "-NoProfile", "-NonInteractive", "-Command", script)
	if err != nil {
		return
	}
	var res struct {
		RAMTotalMB int     `json:"ram_total_mb"`
		RAMFreeMB  int     `json:"ram_free_mb"`
		CPUPct     float64 `json:"cpu_pct"`
	}
	if json.Unmarshal(bytes.TrimSpace(out), &res) != nil {
		return
	}
	if res.RAMTotalMB > 0 {
		pt.RAMTotalMB = res.RAMTotalMB
		used := res.RAMTotalMB - res.RAMFreeMB
		if used < 0 {
			used = 0
		}
		pt.RAMUsedMB = used
	}
	cpu := round1(math.Max(0, math.Min(100, res.CPUPct)))
	pt.CPUPct = &cpu
}

func (c *SystemCollector) collectLinuxMetrics(pt *TelemetryPoint) {
	if f, err := os.Open("/proc/meminfo"); err == nil {
		var totalKB, availKB int
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			k, v, ok := strings.Cut(scanner.Text(), ":")
			if !ok {
				continue
			}
			fields := strings.Fields(v)
			if len(fields) == 0 {
				continue
			}
			n, _ := strconv.Atoi(fields[0])
			switch k {
			case "MemTotal":
				totalKB = n
			case "MemAvailable":
				availKB = n
			}
		}
		_ = f.Close()
		if totalKB > 0 {
			totMB := totalKB / 1024
			availMB := availKB / 1024
			pt.RAMTotalMB = totMB
			pt.RAMUsedMB = totMB - availMB
		}
	}

	if rawStat, err := os.ReadFile("/proc/stat"); err == nil {
		lines := strings.Split(string(rawStat), "\n")
		if len(lines) > 0 && strings.HasPrefix(lines[0], "cpu ") {
			fields := strings.Fields(lines[0])
			if len(fields) >= 5 {
				var total, idle uint64
				for i, f := range fields[1:] {
					v, _ := strconv.ParseUint(f, 10, 64)
					total += v
					if i == 3 || i == 4 { // idle + iowait
						idle += v
					}
				}
				c.mu.Lock()
				if c.prevCPUTotal > 0 && total > c.prevCPUTotal {
					dTotal := float64(total - c.prevCPUTotal)
					dIdle := float64(idle - c.prevCPUIdle)
					pctVal := round1(math.Max(0, math.Min(100, ((dTotal-dIdle)/dTotal)*100.0)))
					pt.CPUPct = &pctVal
				}
				c.prevCPUTotal = total
				c.prevCPUIdle = idle
				c.mu.Unlock()
			}
		}
	}

	if paths, err := filepath.Glob("/sys/class/hwmon/hwmon*/temp1_input"); err == nil {
		maxC := -1.0
		for i, p := range paths {
			if i >= 4 {
				break
			}
			raw, err := os.ReadFile(p)
			if err != nil {
				continue
			}
			milli, err := strconv.Atoi(strings.TrimSpace(string(raw)))
			if err != nil {
				continue
			}
			cVal := float64(milli) / 1000.0
			if cVal > maxC {
				maxC = cVal
			}
		}
		if maxC >= 0 {
			rounded := round1(maxC)
			pt.CPUTempC = &rounded
		}
	}
}

func (c *SystemCollector) collectDarwinMetrics(ctx context.Context, pt *TelemetryPoint) {
	cctx, cancel := context.WithTimeout(ctx, 2500*time.Millisecond)
	defer cancel()
	if memOut, err := runHiddenCommand(cctx, "sysctl", "-n", "hw.memsize"); err == nil {
		if totalBytes, err := strconv.ParseUint(strings.TrimSpace(string(memOut)), 10, 64); err == nil {
			pt.RAMTotalMB = int(totalBytes / (1024 * 1024))
		}
	}
	if vmOut, err := runHiddenCommand(cctx, "vm_stat"); err == nil && pt.RAMTotalMB > 0 {
		var pageSize uint64 = 4096
		var active, wired, compressed uint64
		for _, line := range strings.Split(string(vmOut), "\n") {
			if strings.Contains(line, "page size of") {
				for _, tok := range strings.Fields(line) {
					if n, err := strconv.ParseUint(tok, 10, 64); err == nil && n >= 4096 {
						pageSize = n
					}
				}
			}
			k, v, ok := strings.Cut(line, ":")
			if !ok {
				continue
			}
			valStr := strings.TrimSuffix(strings.TrimSpace(v), ".")
			n, _ := strconv.ParseUint(valStr, 10, 64)
			switch strings.TrimSpace(k) {
			case "Pages active":
				active = n
			case "Pages wired down":
				wired = n
			case "Pages occupied by compressor":
				compressed = n
			}
		}
		usedMB := int(((active + wired + compressed) * pageSize) / (1024 * 1024))
		if usedMB > pt.RAMTotalMB {
			usedMB = pt.RAMTotalMB
		}
		pt.RAMUsedMB = usedMB
	}
}

func (c *SystemCollector) collectGPUProcesses(ctx context.Context) map[int]ProcessInfo {
	outMap := map[int]ProcessInfo{}
	cctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	out, err := runHiddenCommand(
		cctx,
		"nvidia-smi",
		"--query-compute-apps=pid,process_name,used_gpu_memory",
		"--format=csv,noheader,nounits",
	)
	if err != nil {
		return outMap
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		parts := strings.Split(line, ",")
		if len(parts) < 3 {
			continue
		}
		pid, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
		vramF, err2 := strconv.ParseFloat(strings.TrimSpace(parts[2]), 64)
		if err1 != nil || err2 != nil || pid <= 0 {
			continue
		}
		name := filepath.Base(strings.TrimSpace(parts[1]))
		cur := outMap[pid]
		cur.PID = pid
		if cur.Name == "" {
			cur.Name = name
		}
		cur.VRAMMB += int(math.Round(vramF))
		outMap[pid] = cur
	}
	return outMap
}

// TopProcesses returns top memory and GPU consumers on the host.
func (c *SystemCollector) TopProcesses(ctx context.Context) []ProcessInfo {
	gpuProcs := c.collectGPUProcesses(ctx)
	byPID := map[int]ProcessInfo{}

	switch runtime.GOOS {
	case "windows":
		cctx, cancel := context.WithTimeout(ctx, 3*time.Second)
		script := `Get-Process|Sort-Object WorkingSet64 -Descending|Select-Object -First 8 Id,ProcessName,@{N='ram_mb';E={[math]::Round($_.WorkingSet64/1MB,1)}}|ConvertTo-Json -Compress`
		out, err := runHiddenCommand(cctx, "powershell", "-NoProfile", "-NonInteractive", "-Command", script)
		cancel()
		if err == nil {
			trimmed := bytes.TrimSpace(out)
			if len(trimmed) > 0 {
				var rows []struct {
					ID          int     `json:"Id"`
					ProcessName string  `json:"ProcessName"`
					RAMMB       float64 `json:"ram_mb"`
				}
				if trimmed[0] == '[' {
					_ = json.Unmarshal(trimmed, &rows)
				} else if trimmed[0] == '{' {
					var single struct {
						ID          int     `json:"Id"`
						ProcessName string  `json:"ProcessName"`
						RAMMB       float64 `json:"ram_mb"`
					}
					if json.Unmarshal(trimmed, &single) == nil {
						rows = append(rows, single)
					}
				}
				for _, r := range rows {
					if r.ID <= 0 {
						continue
					}
					byPID[r.ID] = ProcessInfo{
						PID:   r.ID,
						Name:  r.ProcessName,
						RAMMB: r.RAMMB,
					}
				}
			}
		}
	default:
		cctx, cancel := context.WithTimeout(ctx, 2*time.Second)
		args := []string{"-eo", "pid,comm,rss", "--sort=-rss"}
		if runtime.GOOS == "darwin" {
			args = []string{"-Ao", "pid,comm,rss", "-r"}
		}
		out, err := runHiddenCommand(cctx, "ps", args...)
		cancel()
		if err == nil {
			lines := strings.Split(strings.TrimSpace(string(out)), "\n")
			for i, line := range lines {
				if i == 0 || i > 9 {
					continue
				}
				fields := strings.Fields(line)
				if len(fields) < 3 {
					continue
				}
				pid, _ := strconv.Atoi(fields[0])
				rssKB, _ := strconv.ParseFloat(fields[len(fields)-1], 64)
				name := strings.Join(fields[1:len(fields)-1], " ")
				if pid <= 0 {
					continue
				}
				byPID[pid] = ProcessInfo{
					PID:   pid,
					Name:  filepath.Base(name),
					RAMMB: round1(rssKB / 1024.0),
				}
			}
		}
	}

	for pid, g := range gpuProcs {
		if existing, ok := byPID[pid]; ok {
			existing.VRAMMB = g.VRAMMB
			byPID[pid] = existing
		} else {
			byPID[pid] = g
		}
	}

	list := make([]ProcessInfo, 0, len(byPID))
	for _, p := range byPID {
		list = append(list, p)
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].VRAMMB != list[j].VRAMMB {
			return list[i].VRAMMB > list[j].VRAMMB
		}
		return list[i].RAMMB > list[j].RAMMB
	})
	if len(list) > 10 {
		list = list[:10]
	}
	return list
}

// ListContainers returns whether the Docker CLI/daemon is reachable and its container list.
func (c *SystemCollector) ListContainers(ctx context.Context) (bool, []DockerContainer) {
	if _, err := exec.LookPath("docker"); err != nil {
		return false, []DockerContainer{}
	}
	cctx, cancel := context.WithTimeout(ctx, 2500*time.Millisecond)
	defer cancel()
	out, err := runHiddenCommand(cctx, "docker", "ps", "-a", "--no-trunc", "--format", "{{json .}}")
	if err != nil {
		return false, []DockerContainer{}
	}
	containers := []DockerContainer{}
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || line[0] != '{' {
			continue
		}
		var raw struct {
			ID     string `json:"ID"`
			Names  string `json:"Names"`
			Image  string `json:"Image"`
			State  string `json:"State"`
			Status string `json:"Status"`
			Ports  string `json:"Ports"`
		}
		if json.Unmarshal([]byte(line), &raw) != nil {
			continue
		}
		shortID := raw.ID
		if len(shortID) > 12 {
			shortID = shortID[:12]
		}
		containers = append(containers, DockerContainer{
			ID:     shortID,
			Name:   raw.Names,
			Image:  raw.Image,
			State:  strings.ToLower(raw.State),
			Status: raw.Status,
			Ports:  raw.Ports,
		})
	}
	return true, containers
}

// DockerAction validates id and runs docker start or docker stop.
func (c *SystemCollector) DockerAction(ctx context.Context, id, action string) (string, error) {
	cleanID, err := ValidateContainerID(id)
	if err != nil {
		return "", err
	}
	if action != "start" && action != "stop" {
		return "", fmt.Errorf("ação Docker inválida")
	}
	cctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	out, err := runHiddenCommand(cctx, "docker", action, cleanID)
	msg := strings.TrimSpace(string(out))
	if err != nil {
		if msg == "" {
			msg = err.Error()
		}
		return msg, fmt.Errorf("%s", msg)
	}
	return msg, nil
}

// DockerLogs returns the recent combined logs for the given container.
func (c *SystemCollector) DockerLogs(ctx context.Context, id string, tail int) (string, error) {
	cleanID, err := ValidateContainerID(id)
	if err != nil {
		return "", err
	}
	if tail <= 0 {
		tail = 80
	}
	if tail > 500 {
		tail = 500
	}
	cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	out, err := runHiddenCommand(cctx, "docker", "logs", "--tail", strconv.Itoa(tail), cleanID)
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return msg, fmt.Errorf("%s", msg)
	}
	return string(out), nil
}
