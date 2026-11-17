package cockpit

import (
	"context"
	"os"
	"runtime"
	"sync"
	"time"
)

// SamplerStore maintains a bounded ring buffer of hardware telemetry samples and short-lived caches.
type SamplerStore struct {
	collector      Collector
	interval       time.Duration
	maxSamples     int
	allowPower     bool
	hostname       string

	mu            sync.RWMutex
	history       []TelemetryPoint
	procs         []ProcessInfo
	dockerOK      bool
	containers    []DockerContainer
	lastHeavyPoll time.Time
}

// NewSamplerStore initializes a SamplerStore with bounded history capacity.
func NewSamplerStore(col Collector, interval time.Duration, maxSamples int, allowPower bool) *SamplerStore {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	if maxSamples <= 0 {
		maxSamples = 72 // 6 minutes @ 5s interval
	}
	host, err := os.Hostname()
	if err != nil || host == "" {
		host = "workstation"
	}
	return &SamplerStore{
		collector:  col,
		interval:   interval,
		maxSamples: maxSamples,
		allowPower: allowPower,
		hostname:   host,
		history:    make([]TelemetryPoint, 0, maxSamples),
	}
}

// RecordSample appends one telemetry snapshot cleanly into the ring buffer.
func (s *SamplerStore) RecordSample(pt TelemetryPoint) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.history) >= s.maxSamples {
		copy(s.history, s.history[1:])
		s.history[len(s.history)-1] = pt
		return
	}
	s.history = append(s.history, pt)
}

// SampleNow forces an immediate sample and stores it in history.
func (s *SamplerStore) SampleNow(ctx context.Context) TelemetryPoint {
	pt := s.collector.Sample(ctx)
	s.RecordSample(pt)
	return pt
}

// Start runs the periodic sampler loop until ctx is canceled.
func (s *SamplerStore) Start(ctx context.Context) {
	_ = s.SampleNow(ctx)
	ticker := time.NewTicker(s.interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_ = s.SampleNow(ctx)
			}
		}
	}()
}

// Overview returns the current snapshot, recent SVG history, top processes, and Docker state.
func (s *SamplerStore) Overview(ctx context.Context, forceRefresh bool) OverviewResponse {
	s.mu.RLock()
	needHeavy := forceRefresh || s.lastHeavyPoll.IsZero() || time.Since(s.lastHeavyPoll) > 3*time.Second
	haveHistory := len(s.history) > 0
	s.mu.RUnlock()

	var current TelemetryPoint
	if forceRefresh || !haveHistory {
		current = s.SampleNow(ctx)
	}

	if needHeavy {
		procs := s.collector.TopProcesses(ctx)
		dockerOK, containers := s.collector.ListContainers(ctx)
		s.mu.Lock()
		s.procs = procs
		s.dockerOK = dockerOK
		s.containers = containers
		s.lastHeavyPoll = time.Now()
		s.mu.Unlock()
	}

	s.mu.RLock()
	histCopy := make([]TelemetryPoint, len(s.history))
	copy(histCopy, s.history)
	if len(histCopy) > 0 && !forceRefresh && haveHistory {
		current = histCopy[len(histCopy)-1]
	}
	procsCopy := append([]ProcessInfo(nil), s.procs...)
	containersCopy := append([]DockerContainer(nil), s.containers...)
	dockerOK := s.dockerOK
	s.mu.RUnlock()

	if procsCopy == nil {
		procsCopy = []ProcessInfo{}
	}
	if containersCopy == nil {
		containersCopy = []DockerContainer{}
	}

	return OverviewResponse{
		Hostname:        s.hostname,
		OS:              runtime.GOOS,
		SampleIntervalS: int(s.interval.Seconds()),
		Current:         current,
		History:         histCopy,
		Processes:       procsCopy,
		DockerAvailable: dockerOK,
		Containers:      containersCopy,
		AllowPower:      s.allowPower,
	}
}
