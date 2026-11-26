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

export interface ToolTimelineEntry {
  name: string;
  preview?: string;
  state: string;
  duration_ms?: number;
}

export interface ChatMessage {
  role: 'user' | 'agent';
  content: string;
  outbox_files?: string[];
  tool_timeline?: ToolTimelineEntry[];
  job_id?: string;
  feedback?: 'up' | 'down' | null;
}

export interface SlashCommandItem {
  id: string;
  command: string;
  description: string;
  prompt_template: string;
  model: string;
  auto_submit: boolean;
  sort_order: number;
  created_at?: string;
  updated_at?: string;
}

export interface SlashCommandPayload {
  command: string;
  description: string;
  prompt_template: string;
  model?: string;
  auto_submit: boolean;
  sort_order?: number;
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

export async function fetchStatus(): Promise<{ online: boolean; mode?: string; active_os?: string; active_host_id?: string; host_name?: string; target_host: string; target_user: string; wol_mac?: string; unread_alerts?: number; pending_evolution?: number }> {
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
  host_name?: string;
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

export function outboxFileUrl(name: string, inline = false): string {
  const base = `/api/outbox/${encodeURIComponent(name)}`;
  return inline ? `${base}?inline=1` : base;
}

export async function fetchSlashCommands(): Promise<SlashCommandItem[]> {
  const res = await fetch('/api/slash-commands');
  if (!res.ok) throw new Error(await readErrorMessage(res, 'Falha ao carregar comandos slash'));
  const data = (await res.json()) as { commands?: SlashCommandItem[] };
  return data.commands ?? [];
}

export async function createSlashCommand(payload: SlashCommandPayload): Promise<SlashCommandItem> {
  const res = await fetch('/api/slash-commands', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  });
  if (!res.ok) throw new Error(await readErrorMessage(res, 'Falha ao criar comando slash'));
  const data = (await res.json()) as { command: SlashCommandItem };
  return data.command;
}

export async function updateSlashCommand(id: string, payload: Partial<SlashCommandPayload>): Promise<SlashCommandItem> {
  const res = await fetch(`/api/slash-commands/${encodeURIComponent(id)}`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  });
  if (!res.ok) throw new Error(await readErrorMessage(res, 'Falha ao atualizar comando slash'));
  const data = (await res.json()) as { command: SlashCommandItem };
  return data.command;
}

export async function deleteSlashCommand(id: string): Promise<void> {
  const res = await fetch(`/api/slash-commands/${encodeURIComponent(id)}`, {
    method: 'DELETE',
  });
  if (!res.ok) throw new Error(await readErrorMessage(res, 'Falha ao apagar comando slash'));
}

export async function resetSlashCommands(): Promise<SlashCommandItem[]> {
  const res = await fetch('/api/slash-commands/reset', { method: 'POST' });
  if (!res.ok) throw new Error(await readErrorMessage(res, 'Falha ao restaurar padrões'));
  const data = (await res.json()) as { commands?: SlashCommandItem[] };
  return data.commands ?? [];
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
  slash_command?: string;
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

export interface AutomationItem {
  id: string;
  name: string;
  cron_expr: string;
  kind: 'prompt' | 'check';
  prompt: string;
  model: string;
  permission_mode: PermissionMode;
  notify_ntfy: boolean;
  notify_push: boolean;
  notify_on: 'always' | 'error_only';
  enabled: boolean;
  last_run_at: string;
  last_status: string;
  last_summary: string;
  last_job_id: string;
  last_conv_id: string;
  next_run_at: string;
  created_at: string;
  updated_at: string;
}

export interface AlertConfig {
  notify_job_done: boolean;
  notify_long_job_sec: number;
  notify_docker_down: boolean;
  gpu_temp_threshold_c: number;
  disk_free_min_pct: number;
  ntfy_topic: string;
  ntfy_url: string;
  web_push_enabled: boolean;
  notify_desktop: boolean;
}

export interface AlertEvent {
  id: number;
  source: string;
  severity: 'info' | 'ok' | 'warning' | 'error';
  title: string;
  message: string;
  conv_id?: string;
  job_id?: string;
  read: boolean;
  created_at: string;
}

export interface WebPushStatus {
  supported: boolean;
  pushSupported: boolean;
  subscribed: boolean;
  permission: NotificationPermission;
  isIOS: boolean;
  isAndroid: boolean;
  isStandalone: boolean;
  isSecure: boolean;
}

async function readErrorMessage(res: Response, fallback: string): Promise<string> {
  try {
    const body = await res.json();
    if (body && typeof body.detail === 'string' && body.detail.trim()) {
      return body.detail;
    }
  } catch {
    // ignore
  }
  return `${fallback} (HTTP ${res.status})`;
}

export async function fetchAutomations(): Promise<{ automations: AutomationItem[]; unread_alerts: number }> {
  const res = await fetch('/api/automations');
  if (!res.ok) throw new Error(await readErrorMessage(res, 'Falha ao carregar automações'));
  return res.json();
}

export async function createAutomation(payload: {
  name: string;
  cron_expr: string;
  kind: 'prompt' | 'check';
  prompt: string;
  model: string;
  permission_mode: PermissionMode;
  notify_ntfy: boolean;
  notify_push: boolean;
  notify_on: 'always' | 'error_only';
  enabled: boolean;
}): Promise<AutomationItem> {
  const res = await fetch('/api/automations', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  });
  if (!res.ok) throw new Error(await readErrorMessage(res, 'Falha ao criar automação'));
  return res.json();
}

export async function updateAutomation(
  id: string,
  patch: Partial<{
    name: string;
    cron_expr: string;
    kind: 'prompt' | 'check';
    prompt: string;
    model: string;
    permission_mode: PermissionMode;
    notify_ntfy: boolean;
    notify_push: boolean;
    notify_on: 'always' | 'error_only';
    enabled: boolean;
  }>
): Promise<AutomationItem> {
  const res = await fetch(`/api/automations/${encodeURIComponent(id)}`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(patch),
  });
  if (!res.ok) throw new Error(await readErrorMessage(res, 'Falha ao atualizar automação'));
  return res.json();
}

export async function deleteAutomation(id: string): Promise<void> {
  const res = await fetch(`/api/automations/${encodeURIComponent(id)}`, { method: 'DELETE' });
  if (!res.ok) throw new Error(await readErrorMessage(res, 'Falha ao excluir automação'));
}

export async function runAutomationNow(id: string): Promise<void> {
  const res = await fetch(`/api/automations/${encodeURIComponent(id)}/run`, { method: 'POST' });
  if (!res.ok) throw new Error(await readErrorMessage(res, 'Falha ao executar automação'));
}

export async function fetchAlerts(limit = 40): Promise<{
  alerts: AlertEvent[];
  unread_count: number;
  config: AlertConfig;
}> {
  const res = await fetch(`/api/alerts?limit=${limit}`);
  if (!res.ok) throw new Error(await readErrorMessage(res, 'Falha ao carregar alertas'));
  return res.json();
}

export async function markAlertsRead(id = 0): Promise<{ unread_count: number }> {
  const res = await fetch('/api/alerts/read', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ id }),
  });
  if (!res.ok) throw new Error('Falha ao marcar alertas');
  return res.json();
}

export async function clearAlerts(): Promise<void> {
  await fetch('/api/alerts', { method: 'DELETE' });
}

export async function saveAlertConfig(cfg: AlertConfig): Promise<AlertConfig> {
  const res = await fetch('/api/alerts/config', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(cfg),
  });
  if (!res.ok) throw new Error(await readErrorMessage(res, 'Falha ao salvar regras'));
  return res.json();
}

export async function sendTestAlert(): Promise<{
  ntfy_sent: boolean;
  ntfy_error?: string;
  push_sent: number;
  push_failed: number;
  push_error?: string;
  desktop_sent: boolean;
  desktop_error?: string;
  browser_live_sent: number;
}> {
  const res = await fetch('/api/alerts/test', { method: 'POST' });
  if (!res.ok) throw new Error(await readErrorMessage(res, 'Falha no alerta de teste'));
  return res.json();
}

function urlBase64ToUint8Array(base64String: string): Uint8Array<ArrayBuffer> {
  const padding = '='.repeat((4 - (base64String.length % 4)) % 4);
  const base64 = (base64String + padding).replace(/-/g, '+').replace(/_/g, '/');
  const rawData = window.atob(base64);
  const buffer = new ArrayBuffer(rawData.length);
  const outputArray = new Uint8Array(buffer);
  for (let i = 0; i < rawData.length; ++i) {
    outputArray[i] = rawData.charCodeAt(i);
  }
  return outputArray;
}

export function getMobilePlatformInfo() {
  if (typeof window === 'undefined' || typeof navigator === 'undefined') {
    return { isIOS: false, isAndroid: false, isStandalone: false, isSecure: false };
  }
  const ua = navigator.userAgent || '';
  const isIOS =
    /iPad|iPhone|iPod/.test(ua) ||
    (navigator.platform === 'MacIntel' && (navigator.maxTouchPoints || 0) > 1);
  const isAndroid = /Android/i.test(ua);
  const isStandalone =
    (window.navigator as { standalone?: boolean }).standalone === true ||
    window.matchMedia?.('(display-mode: standalone)').matches === true;
  return {
    isIOS,
    isAndroid,
    isStandalone,
    isSecure: Boolean(window.isSecureContext),
  };
}

export async function registerGatewayServiceWorker(): Promise<ServiceWorkerRegistration | null> {
  if (typeof window === 'undefined' || !window.isSecureContext || !('serviceWorker' in navigator)) {
    return null;
  }
  try {
    return await navigator.serviceWorker.register('/sw.js', { scope: '/' });
  } catch {
    return null;
  }
}

export async function getWebPushStatus(): Promise<WebPushStatus> {
  const plat = getMobilePlatformInfo();
  const supported = plat.isSecure && 'Notification' in window;
  const pushSupported = supported && 'serviceWorker' in navigator && 'PushManager' in window;
  if (!supported) {
    return {
      supported: false,
      pushSupported: false,
      subscribed: false,
      permission: 'default',
      ...plat,
    };
  }
  const permission = Notification.permission;
  let subscribed = false;
  if (pushSupported) {
    try {
      const reg = await navigator.serviceWorker.getRegistration('/sw.js');
      const sub = reg ? await reg.pushManager.getSubscription() : null;
      subscribed = Boolean(sub);
    } catch {
      subscribed = false;
    }
  }
  return { supported: true, pushSupported, subscribed, permission, ...plat };
}

export async function enableBrowserWebPush(): Promise<{ mode: 'push' | 'foreground'; note?: string }> {
  const plat = getMobilePlatformInfo();
  if (plat.isIOS && !plat.isStandalone) {
    throw new Error('No iPhone/iPad: toque em Compartilhar -> "Adicionar à Tela de Início" no Safari e abra pelo ícone instalado para ativar alertas.');
  }
  if (!window.isSecureContext) {
    throw new Error('Notificações do sistema no celular exigem rota segura HTTPS / domínio próprio (ou localhost no PC).');
  }
  if (!('Notification' in window)) {
    throw new Error('Este navegador não suporta a API Web Notification.');
  }
  const perm = await Notification.requestPermission();
  if (perm !== 'granted') {
    throw new Error('Permissão de notificação negada no navegador.');
  }

  if (!('serviceWorker' in navigator) || !('PushManager' in window)) {
    return { mode: 'foreground', note: 'Ativo enquanto a aba estiver aberta (Push em segundo plano indisponível).' };
  }

  try {
    const reg = await navigator.serviceWorker.register('/sw.js', { scope: '/' });
    await navigator.serviceWorker.ready;

    const keyRes = await fetch('/api/push/vapid-public-key');
    if (!keyRes.ok) throw new Error('Falha ao obter chave VAPID');
    const { public_key } = (await keyRes.json()) as { public_key: string };

    let sub = await reg.pushManager.getSubscription();
    if (!sub) {
      sub = await reg.pushManager.subscribe({
        userVisibleOnly: true,
        applicationServerKey: urlBase64ToUint8Array(public_key),
      });
    }
    const subRes = await fetch('/api/push/subscribe', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(sub.toJSON()),
    });
    if (!subRes.ok) throw new Error('Falha ao registrar inscrição push no servidor');
    return { mode: 'push' };
  } catch (err: any) {
    return {
      mode: 'foreground',
      note: err?.message || 'Notificações no navegador ativas com a aba aberta.',
    };
  }
}

export async function disableBrowserWebPush(): Promise<void> {
  if (!('serviceWorker' in navigator)) return;
  const reg = await navigator.serviceWorker.getRegistration('/sw.js');
  const sub = reg ? await reg.pushManager.getSubscription() : null;
  if (!sub) return;
  const endpoint = sub.endpoint;
  await sub.unsubscribe();
  await fetch('/api/push/unsubscribe', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ endpoint }),
  });
}

export function showBrowserNotification(ev: AlertEvent, onOpenConv?: (convId: string) => void): void {
  if (typeof window === 'undefined' || !('Notification' in window)) return;
  if (Notification.permission !== 'granted') return;
  const tag = `agy-alert-${ev.id}`;
  const icon = '/static/icon-192.png';
  try {
    const n = new Notification(ev.title, {
      body: ev.message,
      icon,
      badge: icon,
      tag,
    });
    n.onclick = () => {
      window.focus();
      if (ev.conv_id && onOpenConv) {
        onOpenConv(ev.conv_id);
      }
      n.close();
    };
  } catch {
    if ('serviceWorker' in navigator) {
      void navigator.serviceWorker
        .getRegistration('/sw.js')
        .then((reg) =>
          reg?.showNotification(ev.title, {
            body: ev.message,
            icon,
            badge: icon,
            tag,
            data: { url: ev.conv_id ? `/?conv=${ev.conv_id}` : '/' },
          }),
        )
        .catch(() => {});
    }
  }
}

export async function transcribeAudio(audioBlob: Blob, filename = 'voice.webm'): Promise<string> {
  const form = new FormData();
  form.append('file', audioBlob, filename);
  const res = await fetch('/api/transcribe', {
    method: 'POST',
    body: form,
  });
  if (!res.ok) {
    throw new Error(await readErrorMessage(res, 'Falha na transcrição de áudio'));
  }
  const data = (await res.json()) as { text?: string };
  return (data.text || '').trim();
}

// --- Security & PIN / Token / WebAuthn Auth ---

export interface WebAuthnCredItem {
  id: string;
  credential_id: string;
  label: string;
  created_at: string;
}

export interface AuthStatus {
  enabled: boolean;
  authenticated: boolean;
  has_pin: boolean;
  has_env_token: boolean;
  webauthn_count: number;
  credentials?: WebAuthnCredItem[];
}

function b64urlToBytes(b64url: string): Uint8Array {
  const pad = '='.repeat((4 - (b64url.length % 4)) % 4);
  const b64 = (b64url + pad).replace(/-/g, '+').replace(/_/g, '/');
  const raw = atob(b64);
  const out = new Uint8Array(raw.length);
  for (let i = 0; i < raw.length; i++) out[i] = raw.charCodeAt(i);
  return out;
}

function bytesToB64url(buf: ArrayBuffer | Uint8Array): string {
  const bytes = buf instanceof Uint8Array ? buf : new Uint8Array(buf);
  let bin = '';
  for (let i = 0; i < bytes.length; i++) bin += String.fromCharCode(bytes[i]);
  return btoa(bin).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/g, '');
}

export async function fetchAuthStatus(): Promise<AuthStatus> {
  const res = await fetch('/api/auth/status');
  return res.json();
}

export async function loginWithSecret(secret: string): Promise<void> {
  const res = await fetch('/api/auth/login', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ token: secret, pin: secret }),
  });
  if (!res.ok) {
    throw new Error(await readErrorMessage(res, 'PIN ou token inválido'));
  }
}

export async function logoutSession(): Promise<{ ok: boolean; authenticated: boolean }> {
  const res = await fetch('/api/auth/logout', { method: 'POST' });
  return res.json();
}

export async function saveAuthPIN(pin: string): Promise<{ ok: boolean; enabled: boolean; has_pin: boolean }> {
  const res = await fetch('/api/auth/pin', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ pin }),
  });
  if (!res.ok) {
    throw new Error(await readErrorMessage(res, 'Erro ao salvar PIN'));
  }
  return res.json();
}

export async function registerWebAuthnPasskey(label = 'Dispositivo'): Promise<WebAuthnCredItem> {
  if (!window.PublicKeyCredential) {
    throw new Error('WebAuthn indisponível neste navegador (requer HTTPS ou localhost)');
  }
  const optRes = await fetch('/api/auth/webauthn/register/options', { method: 'POST' });
  if (!optRes.ok) {
    throw new Error(await readErrorMessage(optRes, 'Falha ao gerar desafio WebAuthn'));
  }
  const opts = await optRes.json();
  const cred = (await navigator.credentials.create({
    publicKey: {
      challenge: b64urlToBytes(opts.challenge) as BufferSource,
      rp: opts.rp,
      user: {
        id: b64urlToBytes(opts.user.id) as BufferSource,
        name: opts.user.name,
        displayName: opts.user.displayName,
      },
      pubKeyCredParams: opts.pubKeyCredParams,
      authenticatorSelection: {
        userVerification: 'preferred',
      },
      timeout: 60000,
    },
  })) as PublicKeyCredential | null;

  if (!cred) {
    throw new Error('Registro biométrico cancelado');
  }
  const resp = cred.response as AuthenticatorAttestationResponse;
  const verRes = await fetch('/api/auth/webauthn/register/verify', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      clientDataJSON: bytesToB64url(resp.clientDataJSON),
      attestationObject: bytesToB64url(resp.attestationObject),
      label,
    }),
  });
  if (!verRes.ok) {
    throw new Error(await readErrorMessage(verRes, 'Falha ao validar passkey'));
  }
  const data = await verRes.json();
  return data.credential as WebAuthnCredItem;
}

export async function loginWithWebAuthn(): Promise<void> {
  if (!window.PublicKeyCredential) {
    throw new Error('WebAuthn indisponível neste navegador');
  }
  const optRes = await fetch('/api/auth/webauthn/login/options', { method: 'POST' });
  if (!optRes.ok) {
    throw new Error(await readErrorMessage(optRes, 'Nenhuma biometria registrada'));
  }
  const opts = await optRes.json();
  const assertion = (await navigator.credentials.get({
    publicKey: {
      challenge: b64urlToBytes(opts.challenge) as BufferSource,
      rpId: opts.rpId,
      allowCredentials: (opts.allowCredentials || []).map((c: { id: string; type: PublicKeyCredentialType }) => ({
        type: c.type,
        id: b64urlToBytes(c.id) as BufferSource,
      })),
      userVerification: 'preferred',
      timeout: 60000,
    },
  })) as PublicKeyCredential | null;

  if (!assertion) {
    throw new Error('Autenticação biométrica cancelada');
  }
  const resp = assertion.response as AuthenticatorAssertionResponse;
  const verRes = await fetch('/api/auth/webauthn/login/verify', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      id: bytesToB64url(assertion.rawId),
      clientDataJSON: bytesToB64url(resp.clientDataJSON),
      authenticatorData: bytesToB64url(resp.authenticatorData),
      signature: bytesToB64url(resp.signature),
    }),
  });
  if (!verRes.ok) {
    throw new Error(await readErrorMessage(verRes, 'Falha na autenticação biométrica'));
  }
}

export async function deleteWebAuthnCredential(id: string): Promise<void> {
  const res = await fetch(`/api/auth/webauthn/${encodeURIComponent(id)}`, { method: 'DELETE' });
  if (!res.ok) {
    throw new Error(await readErrorMessage(res, 'Erro ao remover biometria'));
  }
}

// --- Multi-PC Host Profiles ---

export interface HostProfile {
  id: string;
  name: string;
  host: string;
  ssh_user: string;
  ssh_port: string;
  wol_mac: string;
  os_hint: string;
  is_active: boolean;
  sort_order: number;
}

export interface HostProfilePayload {
  name: string;
  host: string;
  ssh_user: string;
  ssh_port?: string;
  wol_mac?: string;
  os_hint?: string;
}

export async function fetchHosts(): Promise<{ hosts: HostProfile[]; active_host_id: string }> {
  const res = await fetch('/api/hosts');
  if (!res.ok) return { hosts: [], active_host_id: '' };
  return res.json();
}

export async function createHost(payload: HostProfilePayload): Promise<{ ok: boolean; host: HostProfile }> {
  const res = await fetch('/api/hosts', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  });
  if (!res.ok) {
    throw new Error(await readErrorMessage(res, 'Erro ao criar host'));
  }
  return res.json();
}

export async function updateHost(id: string, payload: HostProfilePayload): Promise<{ ok: boolean; host: HostProfile }> {
  const res = await fetch(`/api/hosts/${encodeURIComponent(id)}`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  });
  if (!res.ok) {
    throw new Error(await readErrorMessage(res, 'Erro ao atualizar host'));
  }
  return res.json();
}

export async function deleteHost(id: string): Promise<{ ok: boolean; hosts: HostProfile[]; active_host_id: string }> {
  const res = await fetch(`/api/hosts/${encodeURIComponent(id)}`, { method: 'DELETE' });
  if (!res.ok) {
    throw new Error(await readErrorMessage(res, 'Erro ao remover host'));
  }
  return res.json();
}

export async function activateHost(id: string): Promise<{ ok: boolean; active_host: HostProfile; active_host_id: string; host_name: string; hosts: HostProfile[] }> {
  const res = await fetch(`/api/hosts/${encodeURIComponent(id)}/activate`, { method: 'POST' });
  if (!res.ok) {
    throw new Error(await readErrorMessage(res, 'Erro ao ativar host'));
  }
  return res.json();
}

export interface CommandUsageStat {
  name: string;
  count: number;
  error_count?: number;
  last_used_at?: string;
}

export interface RepeatedPromptStat {
  prompt_norm: string;
  prompt_preview: string;
  preferred_model?: string;
  count: number;
  last_used_at: string;
}

export interface TelemetryStats {
  total_jobs_ok: number;
  total_job_errors: number;
  total_feedback_up: number;
  total_feedback_down: number;
  total_corrections: number;
  unconsolidated_events: number;
  top_commands: CommandUsageStat[];
  top_models: CommandUsageStat[];
  repeated_prompts: RepeatedPromptStat[];
}

export interface EvolutionProposal {
  id: string;
  cycle_id: string;
  kind: 'fast_path_slash' | 'prompt_rule' | 'preference';
  fingerprint: string;
  title: string;
  rationale: string;
  payload: Record<string, any>;
  status: 'pending' | 'approved' | 'rejected';
  applied_ref?: string;
  created_at: string;
  decided_at?: string;
}

export interface EvolutionCycle {
  id: string;
  trigger: string;
  status: 'completed' | 'partial' | 'skipped' | 'error' | 'running';
  events_processed: number;
  proposals_created: number;
  analyzer_backend: string;
  ai_memory_status: string;
  summary: string;
  error?: string;
  started_at: string;
  finished_at?: string;
}

export interface AegysRule {
  id: string;
  kind: 'prompt_rule' | 'preference';
  title: string;
  content: string;
  enabled: boolean;
  source_proposal_id?: string;
  created_at: string;
  updated_at: string;
}

export interface EvolutionReport {
  enabled: boolean;
  cron_expr: string;
  ollama_enabled: boolean;
  ai_memory_enabled: boolean;
  pending_count: number;
  stats: TelemetryStats;
  proposals: EvolutionProposal[];
  rules: AegysRule[];
  cycles: EvolutionCycle[];
}

export async function submitFeedback(payload: {
  job_id?: string;
  conversation_id?: string | null;
  rating: 'up' | 'down';
  correction?: string;
  message_snippet?: string;
}): Promise<{ ok: boolean; kind: string }> {
  const res = await fetch('/api/feedback', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      ...payload,
      conversation_id: payload.conversation_id || '',
    }),
  });
  if (!res.ok) throw new Error(await readErrorMessage(res, 'Falha ao enviar feedback'));
  return res.json();
}

export async function fetchEvolutionReport(): Promise<EvolutionReport> {
  const res = await fetch('/api/evolution/report');
  if (!res.ok) throw new Error(await readErrorMessage(res, 'Falha ao carregar relatório de evolução'));
  return res.json();
}

export async function runEvolutionCycle(force = true): Promise<{ ok: boolean; outcome: { skipped: boolean; reason?: string; cycle?: EvolutionCycle; proposals: EvolutionProposal[] }; report?: EvolutionReport }> {
  const res = await fetch('/api/evolution/run', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ force }),
  });
  if (!res.ok) throw new Error(await readErrorMessage(res, 'Falha ao executar consolidação'));
  return res.json();
}

export async function approveEvolutionProposal(id: string): Promise<{ ok: boolean; proposal: EvolutionProposal; slash_command?: SlashCommandItem; rule?: AegysRule; pending_count: number }> {
  const res = await fetch(`/api/evolution/proposals/${encodeURIComponent(id)}/approve`, { method: 'POST' });
  if (!res.ok) throw new Error(await readErrorMessage(res, 'Falha ao aprovar melhoria'));
  return res.json();
}

export async function rejectEvolutionProposal(id: string): Promise<{ ok: boolean; proposal: EvolutionProposal; pending_count: number }> {
  const res = await fetch(`/api/evolution/proposals/${encodeURIComponent(id)}/reject`, { method: 'POST' });
  if (!res.ok) throw new Error(await readErrorMessage(res, 'Falha ao dispensar melhoria'));
  return res.json();
}

export async function updateAegysRule(id: string, enabled: boolean): Promise<{ ok: boolean; rule: AegysRule }> {
  const res = await fetch(`/api/evolution/rules/${encodeURIComponent(id)}`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ enabled }),
  });
  if (!res.ok) throw new Error(await readErrorMessage(res, 'Falha ao atualizar regra'));
  return res.json();
}

export async function deleteAegysRule(id: string): Promise<void> {
  const res = await fetch(`/api/evolution/rules/${encodeURIComponent(id)}`, { method: 'DELETE' });
  if (!res.ok) throw new Error(await readErrorMessage(res, 'Falha ao remover regra'));
}
