export interface CockpitTelemetryPoint {
  ts: string;
  cpu_pct: number | null;
  cpu_temp_c: number | null;
  ram_used_mb: number;
  ram_total_mb: number;
  ram_pct: number;
  gpu_name?: string;
  gpu_temp_c: number | null;
  vram_used_mb: number;
  vram_total_mb: number;
  vram_pct: number;
}

export interface CockpitProcessInfo {
  pid: number;
  name: string;
  ram_mb: number;
  vram_mb?: number;
}

export interface CockpitDockerContainer {
  id: string;
  name: string;
  image: string;
  state: string;
  status: string;
  ports?: string;
}

export interface CockpitOverview {
  hostname: string;
  os: string;
  sample_interval_sec: number;
  current: CockpitTelemetryPoint;
  history: CockpitTelemetryPoint[];
  processes: CockpitProcessInfo[];
  docker_available: boolean;
  containers: CockpitDockerContainer[];
  allow_power: boolean;
}

export type CockpitPowerAction = 'lock' | 'suspend' | 'reboot';

async function readDetailError(res: Response, fallback: string): Promise<string> {
  try {
    const body = await res.json();
    if (body && typeof body.detail === 'string' && body.detail.trim()) {
      return body.detail;
    }
  } catch {
    // ignore non-json error body
  }
  return `${fallback} (HTTP ${res.status})`;
}

export async function fetchCockpitOverview(refresh = false): Promise<CockpitOverview> {
  const qs = refresh ? '?refresh=1' : '';
  const res = await fetch(`/api/cockpit/overview${qs}`);
  if (!res.ok) {
    throw new Error(await readDetailError(res, 'Cockpit indisponível'));
  }
  return res.json();
}

export async function runCockpitDockerAction(id: string, action: 'start' | 'stop'): Promise<void> {
  const res = await fetch(`/api/cockpit/docker/${encodeURIComponent(id)}/${action}`, {
    method: 'POST',
  });
  if (!res.ok) {
    throw new Error(await readDetailError(res, `Falha em docker ${action}`));
  }
}

export async function fetchCockpitDockerLogs(id: string, tail = 80): Promise<string> {
  const res = await fetch(`/api/cockpit/docker/${encodeURIComponent(id)}/logs?tail=${tail}`);
  if (!res.ok) {
    throw new Error(await readDetailError(res, 'Falha ao buscar logs'));
  }
  const body = await res.json();
  return body.logs ?? '';
}

export async function triggerCockpitPower(
  action: CockpitPowerAction,
  confirm: string
): Promise<{ ok: boolean; action: string; scheduled: boolean }> {
  const res = await fetch('/api/cockpit/power', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ action, confirm }),
  });
  if (!res.ok) {
    throw new Error(await readDetailError(res, 'Falha na ação de energia'));
  }
  return res.json();
}
