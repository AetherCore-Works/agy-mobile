package cockpit

// TelemetryPoint represents a single timestamped workstation hardware sample.
type TelemetryPoint struct {
	Timestamp   string   `json:"ts"`
	CPUPct      *float64 `json:"cpu_pct"`
	CPUTempC    *float64 `json:"cpu_temp_c"`
	RAMUsedMB   int      `json:"ram_used_mb"`
	RAMTotalMB  int      `json:"ram_total_mb"`
	RAMPct      float64  `json:"ram_pct"`
	GPUName     string   `json:"gpu_name,omitempty"`
	GPUTempC    *float64 `json:"gpu_temp_c"`
	VRAMUsedMB  int      `json:"vram_used_mb"`
	VRAMTotalMB int      `json:"vram_total_mb"`
	VRAMPct     float64  `json:"vram_pct"`
}

// ProcessInfo describes a top host process ranked by RAM and/or GPU VRAM usage.
type ProcessInfo struct {
	PID    int     `json:"pid"`
	Name   string  `json:"name"`
	RAMMB  float64 `json:"ram_mb"`
	VRAMMB int     `json:"vram_mb,omitempty"`
}

// DockerContainer describes a local Docker container returned by docker ps -a.
type DockerContainer struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Image  string `json:"image"`
	State  string `json:"state"`
	Status string `json:"status"`
	Ports  string `json:"ports,omitempty"`
}

// OverviewResponse is the aggregated live snapshot served by GET /overview.
type OverviewResponse struct {
	Hostname        string            `json:"hostname"`
	OS              string            `json:"os"`
	SampleIntervalS int               `json:"sample_interval_sec"`
	Current         TelemetryPoint    `json:"current"`
	History         []TelemetryPoint  `json:"history"`
	Processes       []ProcessInfo     `json:"processes"`
	DockerAvailable bool              `json:"docker_available"`
	Containers      []DockerContainer `json:"containers"`
	AllowPower      bool              `json:"allow_power"`
}

// PowerActionRequest triggers a confirmation-protected host power action.
type PowerActionRequest struct {
	Action  string `json:"action"`
	Confirm string `json:"confirm"`
}
