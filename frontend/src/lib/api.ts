export type PermissionMode = 'readonly' | 'copilot' | 'autonomous';

export interface ModelInfo {
  id: string;
  name: string;
  type: 'cloud' | 'local';
  badge: string;
}

export interface ConversationSummary {
  id: string;
  title: string;
  updated_at: string;
  engine: 'agy' | 'ollama';
  model?: string;
  message_count?: number;
  pinned?: boolean;
  tag?: string;
  tag_color?: string;
  custom_title?: string;
}

export interface ConversationMetaPatch {
  pinned?: boolean;
  custom_title?: string;
  tag?: string;
  tag_color?: string;
}

export interface ChatMessage {
  role: 'user' | 'agent';
  content: string;
}

export interface GatewaySettings {
  default_permission_mode: PermissionMode;
  default_model: string;
  default_effort: string;
  theme_mode?: 'dark' | 'light';
  theme_palette?: 'sky' | 'emerald' | 'violet' | 'indigo' | 'amber' | 'rose';
}

export interface OutboxFile {
  name: string;
  size: number;
  mtime: number;
}

export interface WatchdogState {
  online: boolean | null;
  last_checked: string | null;
  last_seen_online: string | null;
  uptime_checks: number;
  downtime_events: number;
}

export async function fetchStatus(): Promise<{ online: boolean; target_host: string; target_user: string }> {
  const res = await fetch('/api/status');
  return res.json();
}

export interface TelemetryData {
  enabled?: boolean;
  gpu: { name: string; vram_used_mb: number; vram_total_mb: number; temp_c: number } | null;
  cpu_temp: number | null;
  ram: { used_mb: number; total_mb: number } | null;
}

export async function fetchWatchdog(): Promise<WatchdogState> {
  const res = await fetch('/api/watchdog');
  return res.json();
}

export async function fetchTelemetry(): Promise<TelemetryData> {
  const res = await fetch('/api/telemetry');
  return res.json();
}

export async function fetchFeatures(): Promise<{
  version: string;
  mode?: string;
  dirs?: { inbox: string; outbox: string };
  features: Record<string, any>;
}> {
  const res = await fetch('/api/features');
  return res.json();
}

export async function triggerWol(): Promise<{ ok: boolean; message: string }> {
  const res = await fetch('/api/wol', { method: 'POST' });
  return res.json();
}

export async function fetchModels(): Promise<ModelInfo[]> {
  const res = await fetch('/api/models');
  const data = await res.json();
  return data.models ?? [];
}

export async function fetchSettings(): Promise<GatewaySettings> {
  const res = await fetch('/api/settings');
  return res.json();
}

export async function updateSettings(patch: Partial<GatewaySettings>): Promise<GatewaySettings> {
  const res = await fetch('/api/settings', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(patch),
  });
  return res.json();
}

export async function fetchConversations(
  q = '',
  limit = 40,
  offset = 0,
  engine: '' | 'agy' | 'ollama' = '',
  tag = ''
): Promise<{ conversations: ConversationSummary[]; total: number; has_more: boolean; tags?: string[] }> {
  const params = new URLSearchParams({ limit: String(limit), offset: String(offset) });
  if (q.trim()) params.set('q', q.trim());
  if (engine) params.set('engine', engine);
  if (tag.trim()) params.set('tag', tag.trim());
  const res = await fetch(`/api/conversations?${params}`);
  return res.json();
}

export async function fetchConversation(
  id: string
): Promise<{
  id: string;
  title?: string;
  messages: ChatMessage[];
  active_job_id?: string | null;
  engine?: string;
  pinned?: boolean;
  tag?: string;
  tag_color?: string;
}> {
  const res = await fetch(`/api/conversations/${encodeURIComponent(id)}`);
  if (!res.ok) throw new Error('Conversa não encontrada');
  return res.json();
}

export async function updateConversationMeta(
  id: string,
  patch: ConversationMetaPatch
): Promise<{ conv_id: string; pinned: boolean; custom_title: string; tag: string; tag_color: string }> {
  const res = await fetch(`/api/conversations/${encodeURIComponent(id)}/meta`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(patch),
  });
  if (!res.ok) throw new Error('Falha ao atualizar sessão');
  return res.json();
}

export async function createSnapshot(payload: {
  title: string;
  content_md: string;
  theme_mode: 'dark' | 'light';
  theme_palette: string;
}): Promise<{ id: string; url: string; created_at: string }> {
  const res = await fetch('/api/snapshots', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  });
  if (!res.ok) throw new Error('Falha ao criar link de snapshot');
  return res.json();
}

export async function deleteConversation(
  id: string
): Promise<{ ok: boolean; deleted_local: boolean; deleted_agy: boolean }> {
  const res = await fetch(`/api/conversations/${encodeURIComponent(id)}`, { method: 'DELETE' });
  if (!res.ok) throw new Error('Falha ao apagar conversa');
  return res.json();
}

export async function uploadFile(file: File): Promise<{ ok: boolean; filename: string; path: string; size: number }> {
  const form = new FormData();
  form.append('file', file);
  const res = await fetch('/api/upload', { method: 'POST', body: form });
  if (!res.ok) throw new Error('Falha no upload');
  return res.json();
}

export async function fetchOutbox(): Promise<OutboxFile[]> {
  const res = await fetch('/api/outbox');
  const data = await res.json();
  return data.files ?? [];
}

export async function deleteOutboxFile(name: string): Promise<void> {
  await fetch(`/api/outbox/${encodeURIComponent(name)}`, { method: 'DELETE' });
}

export async function createJob(payload: {
  prompt: string;
  conversation_id: string | null;
  model: string;
  permission_mode: PermissionMode;
  attached_files?: string[];
}): Promise<{ job_id: string; conversation_id: string | null; engine: string; model?: string; status: string }> {
  const res = await fetch('/api/jobs', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  });
  if (!res.ok) throw new Error(`HTTP ${res.status}`);
  return res.json();
}

export async function cancelJob(jobId: string): Promise<void> {
  await fetch(`/api/jobs/${encodeURIComponent(jobId)}/cancel`, { method: 'POST' });
}
