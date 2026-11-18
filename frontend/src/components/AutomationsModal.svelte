<script lang="ts">
  import { fade, fly } from 'svelte/transition';
  import { cubicOut } from 'svelte/easing';
  import Icon from './Icon.svelte';
  import {
    clearAlerts,
    createAutomation,
    deleteAutomation,
    disableBrowserWebPush,
    enableBrowserWebPush,
    fetchAlerts,
    fetchAutomations,
    getWebPushStatus,
    markAlertsRead,
    runAutomationNow,
    saveAlertConfig,
    sendTestAlert,
    updateAutomation,
    type AlertConfig,
    type AlertEvent,
    type AutomationItem,
    type ModelInfo,
    type PermissionMode,
    type WebPushStatus,
  } from '../lib/api';

  let {
    open = $bindable(false),
    models,
    defaultModel,
    canInstallPWA = false,
    onInstallPWA,
    onOpenConversation,
    onUnreadChange,
  }: {
    open: boolean;
    models: ModelInfo[];
    defaultModel: string;
    canInstallPWA?: boolean;
    onInstallPWA?: () => Promise<boolean>;
    onOpenConversation: (convId: string, title?: string) => void;
    onUnreadChange?: (unread: number) => void;
  } = $props();

  type ModalTab = 'cron' | 'alerts' | 'history';
  type ScheduleMode = 'minutes' | 'hours' | 'daily' | 'weekly' | 'custom';

  let tab = $state<ModalTab>('cron');
  let loading = $state(false);
  let feedback = $state('');
  let errorMsg = $state('');

  let automations = $state<AutomationItem[]>([]);
  let alerts = $state<AlertEvent[]>([]);
  let unreadCount = $state(0);
  let alertConfig = $state<AlertConfig>({
    notify_job_done: true,
    notify_long_job_sec: 45,
    notify_docker_down: true,
    gpu_temp_threshold_c: 82,
    disk_free_min_pct: 10,
    ntfy_topic: '',
    ntfy_url: 'https://ntfy.sh',
    web_push_enabled: true,
    notify_desktop: true,
  });

  // Browser notification + Web Push status
  let pushStatus = $state<WebPushStatus>({
    supported: false,
    pushSupported: false,
    subscribed: false,
    permission: 'default',
    isIOS: false,
    isAndroid: false,
    isStandalone: false,
    isSecure: false,
  });
  let installingPWA = $state(false);

  function generateRandomNtfyTopic() {
    const rand = Math.random().toString(36).slice(2, 10);
    alertConfig.ntfy_topic = `agy-${rand}`;
    flashFeedback(`Tópico "${alertConfig.ntfy_topic}" gerado. Clique em "Salvar Regras" e inscreva-se no app ntfy.`);
  }

  async function handleInstallApp() {
    if (!onInstallPWA) return;
    installingPWA = true;
    try {
      const ok = await onInstallPWA();
      if (ok) {
        flashFeedback('App AGY instalado na tela inicial!');
        pushStatus = await getWebPushStatus();
      }
    } finally {
      installingPWA = false;
    }
  }
  let pushBusy = $state(false);

  // Create Automation form state (visual human-friendly schedule builder + optional advanced Cron)
  let showNewForm = $state(false);
  let savingCron = $state(false);
  let newName = $state('');
  let scheduleMode = $state<ScheduleMode>('daily');
  let everyMinutes = $state(15);
  let everyHours = $state(1);
  let dailyTime = $state('08:00');
  let weeklyTime = $state('09:00');
  let weeklyDays = $state<number[]>([1, 2, 3, 4, 5]);
  let customCron = $state('0 8 * * *');

  let newKind = $state<'prompt' | 'check'>('prompt');
  let newPrompt = $state('');
  let checkDisk = $state(true);
  let checkDocker = $state(true);
  let checkGPU = $state(true);

  let newModel = $state('');
  let newPerm = $state<PermissionMode>('readonly');
  let newNotifyOn = $state<'always' | 'error_only'>('always');
  let newNotifyNtfy = $state(true);
  let newNotifyPush = $state(true);

  let savingRules = $state(false);
  let testingAlert = $state(false);
  let runningAutoId = $state<string | null>(null);

  const dayButtons = [
    { id: 1, label: 'Seg' },
    { id: 2, label: 'Ter' },
    { id: 3, label: 'Qua' },
    { id: 4, label: 'Qui' },
    { id: 5, label: 'Sex' },
    { id: 6, label: 'Sáb' },
    { id: 0, label: 'Dom' },
  ];

  function parseTimeParts(raw: string): { hour: number; min: number } {
    const [hStr, mStr] = (raw || '08:00').split(':');
    const h = Math.max(0, Math.min(23, Number.parseInt(hStr || '8', 10) || 0));
    const m = Math.max(0, Math.min(59, Number.parseInt(mStr || '0', 10) || 0));
    return { hour: h, min: m };
  }

  function formatHHMM(h: number, m: number): string {
    return `${String(h).padStart(2, '0')}:${String(m).padStart(2, '0')}`;
  }

  function formatDayList(days: number[]): string {
    if (days.length === 0) return '*';
    const sorted = [...days].sort((a, b) => a - b);
    if (sorted.join(',') === '1,2,3,4,5') return '1-5';
    return sorted.join(',');
  }

  let computedCron = $derived.by(() => {
    if (scheduleMode === 'custom') return customCron.trim() || '0 8 * * *';
    if (scheduleMode === 'minutes') return `*/${everyMinutes} * * * *`;
    if (scheduleMode === 'hours') return everyHours === 1 ? '0 * * * *' : `0 */${everyHours} * * *`;
    if (scheduleMode === 'daily') {
      const { hour, min } = parseTimeParts(dailyTime);
      return `${min} ${hour} * * *`;
    }
    const { hour, min } = parseTimeParts(weeklyTime);
    return `${min} ${hour} * * ${formatDayList(weeklyDays)}`;
  });

  function describeCronHuman(expr: string): string {
    const raw = (expr || '').trim();
    const parts = raw.split(/\s+/);
    if (parts.length !== 5) return raw;
    const [m, h, dom, mon, dow] = parts;
    if (m.startsWith('*/') && h === '*' && dom === '*' && mon === '*' && dow === '*') {
      return `A cada ${m.slice(2)} minutos`;
    }
    if (m === '0' && h === '*' && dom === '*' && mon === '*' && dow === '*') {
      return 'A cada 1 hora';
    }
    if (m === '0' && h.startsWith('*/') && dom === '*' && mon === '*' && dow === '*') {
      return `A cada ${h.slice(2)} horas`;
    }
    const mNum = Number(m);
    const hNum = Number(h);
    if (Number.isInteger(mNum) && Number.isInteger(hNum) && dom === '*' && mon === '*') {
      const hhmm = formatHHMM(hNum, mNum);
      if (dow === '*') return `Todo dia às ${hhmm}`;
      if (dow === '1-5') return `Seg a Sex às ${hhmm}`;
      const names = dow
        .split(',')
        .map((d) => dayButtons.find((b) => String(b.id) === d)?.label || d)
        .join(', ');
      return `${names} às ${hhmm}`;
    }
    return `Cron: ${raw}`;
  }

  function formatCheckLabels(rawPrompt: string): string {
    const low = (rawPrompt || '').trim().toLowerCase();
    if (!low.startsWith('checks:')) {
      return rawPrompt || 'Disco, Docker e GPU';
    }
    const items = low
      .slice(7)
      .split(',')
      .map((s) => s.trim())
      .filter(Boolean);
    const labels: string[] = [];
    if (items.includes('disk') || items.includes('disco')) labels.push('Espaço em disco');
    if (items.includes('docker')) labels.push('Containers Docker');
    if (items.includes('gpu')) labels.push('GPU & VRAM');
    return labels.length > 0 ? `Checar: ${labels.join(', ')}` : 'Checar sistema';
  }

  function toggleWeekday(dayId: number) {
    if (weeklyDays.includes(dayId)) {
      if (weeklyDays.length > 1) {
        weeklyDays = weeklyDays.filter((d) => d !== dayId);
      }
    } else {
      weeklyDays = [...weeklyDays, dayId];
    }
  }

  function switchScheduleMode(next: ScheduleMode) {
    if (next === 'custom') {
      customCron = computedCron;
    }
    scheduleMode = next;
  }

  function flashFeedback(msg: string) {
    feedback = msg;
    setTimeout(() => {
      if (feedback === msg) feedback = '';
    }, 3600);
  }

  async function refreshAll() {
    loading = true;
    errorMsg = '';
    try {
      const [autoRes, alertRes, pushRes] = await Promise.all([
        fetchAutomations(),
        fetchAlerts(50),
        getWebPushStatus().catch(() => ({
          supported: false,
          pushSupported: false,
          subscribed: false,
          permission: 'default' as NotificationPermission,
          isIOS: false,
          isAndroid: false,
          isStandalone: false,
          isSecure: false,
        })),
      ]);
      automations = autoRes.automations;
      alerts = alertRes.alerts;
      unreadCount = alertRes.unread_count;
      alertConfig = alertRes.config;
      pushStatus = pushRes;
      onUnreadChange?.(alertRes.unread_count);
    } catch (err: any) {
      errorMsg = err?.message || 'Erro ao carregar automações e alertas';
    } finally {
      loading = false;
    }
  }

  $effect(() => {
    if (open) {
      if (!newModel) newModel = defaultModel || 'gemini-4-hydrogen-high';
      refreshAll();
    } else {
      showNewForm = false;
      errorMsg = '';
    }
  });

  function fillExample() {
    newName = 'Checagem diária 08h';
    scheduleMode = 'daily';
    dailyTime = '08:00';
    newKind = 'check';
    checkDisk = true;
    checkDocker = true;
    checkGPU = false;
    newNotifyOn = 'always';
    showNewForm = true;
  }

  async function handleCreateAutomation(e: Event) {
    e.preventDefault();
    savingCron = true;
    errorMsg = '';

    let promptToSave = newPrompt.trim();
    if (newKind === 'check') {
      const selected: string[] = [];
      if (checkDisk) selected.push('disk');
      if (checkDocker) selected.push('docker');
      if (checkGPU) selected.push('gpu');
      if (selected.length === 0) {
        errorMsg = 'Selecione pelo menos 1 item para verificar no Check Nativo.';
        savingCron = false;
        return;
      }
      promptToSave = `checks:${selected.join(',')}`;
    }

    try {
      await createAutomation({
        name: newName.trim(),
        cron_expr: computedCron,
        kind: newKind,
        prompt: promptToSave,
        model: newModel || defaultModel,
        permission_mode: newPerm,
        notify_ntfy: newNotifyNtfy,
        notify_push: newNotifyPush,
        notify_on: newNotifyOn,
        enabled: true,
      });
      newName = '';
      newPrompt = '';
      showNewForm = false;
      flashFeedback('Automação agendada e salva no SQLite.');
      await refreshAll();
    } catch (err: any) {
      errorMsg = err?.message || 'Falha ao criar automação';
    } finally {
      savingCron = false;
    }
  }

  async function handleToggleEnabled(a: AutomationItem) {
    errorMsg = '';
    try {
      await updateAutomation(a.id, { enabled: !a.enabled });
      await refreshAll();
    } catch (err: any) {
      errorMsg = err?.message || 'Erro ao atualizar estado';
    }
  }

  async function handleRunNow(a: AutomationItem) {
    runningAutoId = a.id;
    errorMsg = '';
    try {
      await runAutomationNow(a.id);
      flashFeedback(`"${a.name}" disparada em segundo plano.`);
      setTimeout(() => {
        if (open) refreshAll();
      }, 1500);
    } catch (err: any) {
      errorMsg = err?.message || 'Falha ao executar automação';
    } finally {
      runningAutoId = null;
    }
  }

  async function handleDeleteAutomation(id: string) {
    errorMsg = '';
    try {
      await deleteAutomation(id);
      await refreshAll();
    } catch (err: any) {
      errorMsg = err?.message || 'Erro ao remover automação';
    }
  }

  async function handleSaveAlertRules(e: Event) {
    e.preventDefault();
    savingRules = true;
    errorMsg = '';
    try {
      alertConfig = await saveAlertConfig(alertConfig);
      flashFeedback('Regras de alerta e canais salvos.');
    } catch (err: any) {
      errorMsg = err?.message || 'Erro ao salvar regras';
    } finally {
      savingRules = false;
    }
  }

  async function handleTestAlert() {
    testingAlert = true;
    errorMsg = '';
    try {
      const res = await sendTestAlert();
      const parts = ['Histórico OK'];
      if (res.desktop_sent) parts.push('Sistema (SO) OK');
      else if (res.desktop_error) parts.push(`Sistema erro: ${res.desktop_error}`);
      if (res.browser_live_sent > 0) parts.push('Navegador ao vivo OK');
      if (res.push_sent > 0) parts.push(`Web Push: ${res.push_sent}`);
      else if (res.push_error) parts.push(`Web Push erro: ${res.push_error}`);
      if (res.ntfy_sent) parts.push('ntfy OK');
      else if (res.ntfy_error) parts.push(`ntfy erro: ${res.ntfy_error}`);
      flashFeedback(parts.join(' · '));
      await refreshAll();
    } catch (err: any) {
      errorMsg = err?.message || 'Falha ao enviar alerta de teste';
    } finally {
      testingAlert = false;
    }
  }

  async function handleToggleWebPush() {
    pushBusy = true;
    errorMsg = '';
    try {
      if (pushStatus.subscribed) {
        await disableBrowserWebPush();
        flashFeedback('Web Push em segundo plano desativado.');
      } else {
        const res = await enableBrowserWebPush();
        if (res.mode === 'push') {
          flashFeedback('Notificações do navegador + Web Push em segundo plano ativos!');
        } else {
          flashFeedback(res.note || 'Notificações no navegador ativadas!');
        }
      }
      pushStatus = await getWebPushStatus();
    } catch (err: any) {
      errorMsg = err?.message || 'Não foi possível ativar notificações no navegador';
    } finally {
      pushBusy = false;
    }
  }

  async function handleMarkAllRead() {
    const res = await markAlertsRead();
    unreadCount = res.unread_count;
    alerts = alerts.map((a) => ({ ...a, read: true }));
    onUnreadChange?.(unreadCount);
  }

  async function handleClearHistory() {
    await clearAlerts();
    alerts = [];
    unreadCount = 0;
    onUnreadChange?.(0);
  }

  function severityStyle(sev: string): string {
    if (sev === 'error') return 'bg-rose-500/15 border-rose-500/35 text-rose-400';
    if (sev === 'warning') return 'bg-amber-500/15 border-amber-500/35 text-amber-400';
    if (sev === 'ok') return 'bg-emerald-500/15 border-emerald-500/35 text-emerald-400';
    return 'bg-[var(--accent-soft)] border-[var(--accent-border)] text-[var(--accent-fg)]';
  }
</script>

{#if open}
  <div
    transition:fade={{ duration: 150 }}
    class="fixed inset-0 bg-[var(--bg-backdrop)] backdrop-blur-sm z-50 flex items-end sm:items-center justify-center p-0 sm:p-4"
  >
    <button
      type="button"
      aria-label="Fechar Automações e Alertas"
      onclick={() => (open = false)}
      class="absolute inset-0 cursor-default"
    ></button>

    <div
      transition:fly={{ y: 20, duration: 200, easing: cubicOut }}
      class="relative w-full sm:max-w-2xl max-h-[90dvh] bg-[var(--bg-elevated)] border-t sm:border border-[var(--border-subtle)] rounded-t-2xl sm:rounded-2xl flex flex-col shadow-2xl theme-surface overflow-hidden"
    >
      <!-- Header -->
      <div class="p-3.5 border-b border-[var(--border-subtle)] flex items-center justify-between gap-2 shrink-0">
        <div class="flex items-center gap-2 min-w-0">
          <Icon name="bell" size={17} class="text-[var(--accent-fg)]" />
          <div class="min-w-0">
            <h2 class="text-sm font-bold text-[var(--text-primary)] truncate">Automações & Alertas Ativos</h2>
            <p class="text-[11px] text-[var(--text-faint)] truncate">Agendamentos em SQLite · Notificações no Sistema, Navegador & ntfy</p>
          </div>
        </div>
        <button
          type="button"
          onclick={() => (open = false)}
          class="p-1.5 rounded-lg text-[var(--text-muted)] hover:text-[var(--text-primary)] transition-colors"
          aria-label="Fechar"
        >
          <Icon name="close" size={18} />
        </button>
      </div>

      <!-- Tabs -->
      <div class="grid grid-cols-3 gap-1 p-1.5 bg-[var(--bg-input)] border-b border-[var(--border-subtle)] shrink-0">
        <button
          type="button"
          onclick={() => (tab = 'cron')}
          class="py-1.5 px-2 rounded-lg text-xs font-semibold flex items-center justify-center gap-1.5 transition-all {tab === 'cron' ? 'bg-[var(--bg-elevated)] text-[var(--accent-fg)] border border-[var(--border-subtle)] shadow-sm' : 'text-[var(--text-muted)] hover:text-[var(--text-primary)]'}"
        >
          <Icon name="clock" size={13} />
          <span>Agendadas ({automations.length})</span>
        </button>
        <button
          type="button"
          onclick={() => (tab = 'alerts')}
          class="py-1.5 px-2 rounded-lg text-xs font-semibold flex items-center justify-center gap-1.5 transition-all {tab === 'alerts' ? 'bg-[var(--bg-elevated)] text-[var(--accent-fg)] border border-[var(--border-subtle)] shadow-sm' : 'text-[var(--text-muted)] hover:text-[var(--text-primary)]'}"
        >
          <Icon name="zap" size={13} />
          <span>Alertas</span>
        </button>
        <button
          type="button"
          onclick={() => {
            tab = 'history';
            if (unreadCount > 0) handleMarkAllRead();
          }}
          class="py-1.5 px-2 rounded-lg text-xs font-semibold flex items-center justify-center gap-1.5 transition-all {tab === 'history' ? 'bg-[var(--bg-elevated)] text-[var(--accent-fg)] border border-[var(--border-subtle)] shadow-sm' : 'text-[var(--text-muted)] hover:text-[var(--text-primary)]'}"
        >
          <Icon name="bell" size={13} />
          <span>Histórico</span>
          {#if unreadCount > 0}
            <span class="px-1.5 py-0.2 rounded-full bg-rose-600 text-white text-[10px] font-bold">{unreadCount}</span>
          {/if}
        </button>
      </div>

      <!-- Feedback / Error Banners -->
      {#if errorMsg}
        <div class="mx-3.5 mt-3 p-2.5 rounded-xl bg-rose-500/10 border border-rose-500/35 text-xs text-rose-400">
          {errorMsg}
        </div>
      {/if}
      {#if feedback}
        <div class="mx-3.5 mt-3 p-2.5 rounded-xl bg-emerald-500/15 border border-emerald-500/35 text-xs text-emerald-400 flex items-center gap-1.5">
          <Icon name="check" size={14} />
          <span>{feedback}</span>
        </div>
      {/if}

      <!-- Body -->
      <div class="flex-1 overflow-y-auto p-3.5 space-y-3">
        {#if tab === 'cron'}
          <div class="flex flex-wrap items-center justify-between gap-2">
            <button
              type="button"
              onclick={() => (showNewForm = !showNewForm)}
              class="px-3 py-1.5 rounded-xl bg-[var(--accent-solid)] hover:bg-[var(--accent-hover)] text-[var(--accent-on-solid)] text-xs font-semibold flex items-center gap-1.5 transition-all active:scale-95"
            >
              <Icon name={showNewForm ? 'close' : 'plus'} size={14} />
              <span>{showNewForm ? 'Cancelar criação' : 'Nova Tarefa Agendada'}</span>
            </button>

            <button
              type="button"
              onclick={fillExample}
              class="px-2.5 py-1.5 rounded-xl bg-[var(--bg-input)] border border-[var(--border-subtle)] hover:border-[var(--accent-border)] text-[11px] font-medium text-[var(--text-secondary)] hover:text-[var(--accent-fg)] transition-colors"
            >
              Usar exemplo 08h (Disco + Docker)
            </button>
          </div>

          {#if showNewForm}
            <form
              onsubmit={handleCreateAutomation}
              class="p-3 rounded-xl bg-[var(--bg-input)] border border-[var(--border-subtle)] space-y-3"
            >
              <div>
                <label for="cron-name" class="block text-[11px] font-medium text-[var(--text-muted)] mb-1">Nome da tarefa</label>
                <input
                  id="cron-name"
                  type="text"
                  bind:value={newName}
                  placeholder="Ex: Checagem matinal 08h"
                  class="w-full bg-[var(--bg-elevated)] border border-[var(--border-subtle)] rounded-lg px-2.5 py-1.5 text-xs text-[var(--text-primary)] focus:outline-none focus:border-[var(--accent-fg)]"
                />
              </div>

              <!-- Human-friendly Schedule Builder -->
              <div class="space-y-2 p-2.5 rounded-xl bg-[var(--bg-elevated)] border border-[var(--border-subtle)]">
                <div class="flex items-center justify-between gap-2">
                  <span class="text-[11px] font-semibold text-[var(--text-secondary)]">Quando executar?</span>
                  <button
                    type="button"
                    onclick={() => switchScheduleMode(scheduleMode === 'custom' ? 'daily' : 'custom')}
                    class="text-[10px] font-medium text-[var(--accent-fg)] hover:underline"
                  >
                    {scheduleMode === 'custom' ? 'Voltar ao modo simples' : 'Modo avançado (Cron)'}
                  </button>
                </div>

                {#if scheduleMode !== 'custom'}
                  <div class="grid grid-cols-2 sm:grid-cols-4 gap-1">
                    <button
                      type="button"
                      onclick={() => switchScheduleMode('minutes')}
                      class="py-1.5 px-2 rounded-lg border text-[11px] font-medium transition-all {scheduleMode === 'minutes' ? 'bg-[var(--accent-soft)] border-[var(--accent-border)] text-[var(--accent-fg)]' : 'bg-[var(--bg-input)] border-[var(--border-subtle)] text-[var(--text-muted)]'}"
                    >
                      A cada minutos
                    </button>
                    <button
                      type="button"
                      onclick={() => switchScheduleMode('hours')}
                      class="py-1.5 px-2 rounded-lg border text-[11px] font-medium transition-all {scheduleMode === 'hours' ? 'bg-[var(--accent-soft)] border-[var(--accent-border)] text-[var(--accent-fg)]' : 'bg-[var(--bg-input)] border-[var(--border-subtle)] text-[var(--text-muted)]'}"
                    >
                      A cada horas
                    </button>
                    <button
                      type="button"
                      onclick={() => switchScheduleMode('daily')}
                      class="py-1.5 px-2 rounded-lg border text-[11px] font-medium transition-all {scheduleMode === 'daily' ? 'bg-[var(--accent-soft)] border-[var(--accent-border)] text-[var(--accent-fg)]' : 'bg-[var(--bg-input)] border-[var(--border-subtle)] text-[var(--text-muted)]'}"
                    >
                      Todo dia
                    </button>
                    <button
                      type="button"
                      onclick={() => switchScheduleMode('weekly')}
                      class="py-1.5 px-2 rounded-lg border text-[11px] font-medium transition-all {scheduleMode === 'weekly' ? 'bg-[var(--accent-soft)] border-[var(--accent-border)] text-[var(--accent-fg)]' : 'bg-[var(--bg-input)] border-[var(--border-subtle)] text-[var(--text-muted)]'}"
                    >
                      Dias da semana
                    </button>
                  </div>

                  {#if scheduleMode === 'minutes'}
                    <div class="flex flex-wrap items-center gap-1.5">
                      {#each [5, 10, 15, 30] as mins}
                        <button
                          type="button"
                          onclick={() => (everyMinutes = mins)}
                          class="px-2.5 py-1 rounded-lg border text-xs font-medium {everyMinutes === mins ? 'bg-[var(--accent-solid)] border-[var(--accent-solid)] text-[var(--accent-on-solid)]' : 'bg-[var(--bg-input)] border-[var(--border-subtle)] text-[var(--text-secondary)]'}"
                        >
                          {mins} min
                        </button>
                      {/each}
                    </div>
                  {:else if scheduleMode === 'hours'}
                    <div class="flex flex-wrap items-center gap-1.5">
                      {#each [1, 2, 4, 6, 12] as hrs}
                        <button
                          type="button"
                          onclick={() => (everyHours = hrs)}
                          class="px-2.5 py-1 rounded-lg border text-xs font-medium {everyHours === hrs ? 'bg-[var(--accent-solid)] border-[var(--accent-solid)] text-[var(--accent-on-solid)]' : 'bg-[var(--bg-input)] border-[var(--border-subtle)] text-[var(--text-secondary)]'}"
                        >
                          {hrs}h
                        </button>
                      {/each}
                    </div>
                  {:else if scheduleMode === 'daily'}
                    <div class="flex items-center gap-2">
                      <label for="sched-daily-time" class="text-[11px] text-[var(--text-muted)]">Horário:</label>
                      <input
                        id="sched-daily-time"
                        type="time"
                        bind:value={dailyTime}
                        class="bg-[var(--bg-input)] border border-[var(--border-subtle)] rounded-lg px-2.5 py-1 text-xs font-mono text-[var(--text-primary)]"
                      />
                    </div>
                  {:else}
                    <div class="space-y-2">
                      <div class="flex flex-wrap gap-1">
                        {#each dayButtons as d}
                          <button
                            type="button"
                            onclick={() => toggleWeekday(d.id)}
                            class="px-2 py-1 rounded-lg border text-[11px] font-medium transition-all {weeklyDays.includes(d.id) ? 'bg-[var(--accent-solid)] border-[var(--accent-solid)] text-[var(--accent-on-solid)]' : 'bg-[var(--bg-input)] border-[var(--border-subtle)] text-[var(--text-muted)]'}"
                          >
                            {d.label}
                          </button>
                        {/each}
                      </div>
                      <div class="flex items-center gap-2">
                        <label for="sched-weekly-time" class="text-[11px] text-[var(--text-muted)]">Horário:</label>
                        <input
                          id="sched-weekly-time"
                          type="time"
                          bind:value={weeklyTime}
                          class="bg-[var(--bg-input)] border border-[var(--border-subtle)] rounded-lg px-2.5 py-1 text-xs font-mono text-[var(--text-primary)]"
                        />
                      </div>
                    </div>
                  {/if}
                {:else}
                  <div>
                    <label for="cron-expr-adv" class="block text-[10px] text-[var(--text-faint)] mb-1">
                      Expressão Cron manual (minuto · hora · dia · mês · semana)
                    </label>
                    <input
                      id="cron-expr-adv"
                      type="text"
                      bind:value={customCron}
                      placeholder="0 8 * * *"
                      class="w-full bg-[var(--bg-input)] border border-[var(--border-subtle)] rounded-lg px-2.5 py-1.5 text-xs font-mono text-[var(--text-primary)] focus:outline-none focus:border-[var(--accent-fg)]"
                    />
                  </div>
                {/if}

                <div class="text-[11px] text-[var(--text-muted)] flex flex-wrap items-center justify-between gap-2 pt-1 border-t border-[var(--border-subtle)]/70">
                  <span>Resumo: <strong class="text-[var(--text-primary)]">{describeCronHuman(computedCron)}</strong></span>
                  <span class="text-[10px] font-mono text-[var(--text-faint)]">{computedCron}</span>
                </div>
              </div>

              <!-- Kind Selector -->
              <div class="grid grid-cols-2 gap-1.5 p-1 rounded-lg bg-[var(--bg-elevated)] border border-[var(--border-subtle)]">
                <button
                  type="button"
                  onclick={() => (newKind = 'prompt')}
                  class="py-1.5 rounded-md text-[11px] font-semibold transition-all {newKind === 'prompt' ? 'bg-[var(--accent-soft)] border border-[var(--accent-border)] text-[var(--accent-fg)]' : 'text-[var(--text-muted)]'}"
                >
                  Prompt IA (escrever comando)
                </button>
                <button
                  type="button"
                  onclick={() => (newKind = 'check')}
                  class="py-1.5 rounded-md text-[11px] font-semibold transition-all {newKind === 'check' ? 'bg-[var(--accent-soft)] border border-[var(--accent-border)] text-[var(--accent-fg)]' : 'text-[var(--text-muted)]'}"
                >
                  Check Nativo (0 tokens)
                </button>
              </div>

              {#if newKind === 'prompt'}
                <div>
                  <label for="cron-prompt" class="block text-[11px] font-medium text-[var(--text-muted)] mb-1">O que a IA deve fazer nesse horário?</label>
                  <textarea
                    id="cron-prompt"
                    rows={2}
                    bind:value={newPrompt}
                    placeholder="Ex: Verifique espaço em disco e containers Docker com erro e me dê um resumo curto."
                    class="w-full bg-[var(--bg-elevated)] border border-[var(--border-subtle)] rounded-lg px-2.5 py-1.5 text-xs text-[var(--text-primary)] focus:outline-none focus:border-[var(--accent-fg)]"
                  ></textarea>
                </div>

                <div class="grid grid-cols-1 sm:grid-cols-2 gap-2">
                  <select
                    bind:value={newModel}
                    class="bg-[var(--bg-elevated)] border border-[var(--border-subtle)] rounded-lg px-2 py-1.5 text-[11px] text-[var(--text-primary)]"
                  >
                    {#each models as m}
                      <option value={m.id}>{m.name}</option>
                    {/each}
                  </select>
                  <select
                    bind:value={newPerm}
                    class="bg-[var(--bg-elevated)] border border-[var(--border-subtle)] rounded-lg px-2 py-1.5 text-[11px] text-[var(--text-primary)]"
                  >
                    <option value="readonly">Leitura (Plan)</option>
                    <option value="copilot">Copiloto</option>
                    <option value="autonomous">Autônomo</option>
                  </select>
                </div>
              {:else}
                <div class="p-2.5 rounded-xl bg-[var(--bg-elevated)] border border-[var(--border-subtle)] space-y-2">
                  <div class="text-[11px] font-medium text-[var(--text-secondary)]">Escolha o que o Check Nativo (sem custo de tokens) vai verificar:</div>
                  <div class="grid grid-cols-1 sm:grid-cols-3 gap-2">
                    <label class="p-2 rounded-lg bg-[var(--bg-input)] border border-[var(--border-subtle)] flex items-center gap-2 text-xs text-[var(--text-primary)] cursor-pointer">
                      <input type="checkbox" bind:checked={checkDisk} />
                      <span>Espaço em disco</span>
                    </label>
                    <label class="p-2 rounded-lg bg-[var(--bg-input)] border border-[var(--border-subtle)] flex items-center gap-2 text-xs text-[var(--text-primary)] cursor-pointer">
                      <input type="checkbox" bind:checked={checkDocker} />
                      <span>Containers Docker</span>
                    </label>
                    <label class="p-2 rounded-lg bg-[var(--bg-input)] border border-[var(--border-subtle)] flex items-center gap-2 text-xs text-[var(--text-primary)] cursor-pointer">
                      <input type="checkbox" bind:checked={checkGPU} />
                      <span>Temperatura GPU</span>
                    </label>
                  </div>
                </div>
              {/if}

              <div class="flex flex-wrap items-center justify-between gap-2 pt-1">
                <div class="flex flex-wrap items-center gap-3 text-[11px] text-[var(--text-secondary)]">
                  <label class="inline-flex items-center gap-1 cursor-pointer">
                    <input type="checkbox" bind:checked={newNotifyNtfy} />
                    <span>ntfy</span>
                  </label>
                  <label class="inline-flex items-center gap-1 cursor-pointer">
                    <input type="checkbox" bind:checked={newNotifyPush} />
                    <span>Web Push</span>
                  </label>
                  <select
                    bind:value={newNotifyOn}
                    class="bg-[var(--bg-elevated)] border border-[var(--border-subtle)] rounded-md px-1.5 py-0.5 text-[11px]"
                  >
                    <option value="always">Notificar sempre</option>
                    <option value="error_only">Somente se falhar</option>
                  </select>
                </div>

                <button
                  type="submit"
                  disabled={savingCron}
                  class="px-3 py-1.5 rounded-lg bg-[var(--accent-solid)] text-[var(--accent-on-solid)] text-xs font-semibold disabled:opacity-50"
                >
                  {savingCron ? 'Salvando...' : 'Agendar Tarefa'}
                </button>
              </div>
            </form>
          {/if}

          {#if loading && automations.length === 0}
            <div class="py-8 text-center text-xs text-[var(--text-faint)]">Carregando automações...</div>
          {:else if automations.length === 0}
            <div class="py-8 text-center text-xs text-[var(--text-faint)] border border-dashed border-[var(--border-subtle)] rounded-xl">
              Nenhuma automação cadastrada ainda.
            </div>
          {:else}
            <div class="space-y-2">
              {#each automations as auto (auto.id)}
                <div class="p-3 rounded-xl bg-[var(--bg-input)] border border-[var(--border-subtle)] space-y-2">
                  <div class="flex items-start justify-between gap-2">
                    <div class="min-w-0 flex-1 space-y-1">
                      <div class="flex flex-wrap items-center gap-1.5">
                        <span class="text-xs font-bold text-[var(--text-primary)]">{auto.name}</span>
                        <span
                          class="px-1.5 py-0.5 rounded bg-[var(--bg-elevated)] border border-[var(--border-subtle)] text-[10px] font-medium text-[var(--accent-fg)]"
                          title={`Cron: ${auto.cron_expr}`}
                        >
                          {describeCronHuman(auto.cron_expr)}
                        </span>
                        <span class="px-1.5 py-0.5 rounded bg-[var(--bg-chip)] text-[10px] font-medium text-[var(--text-secondary)]">
                          {auto.kind === 'check' ? 'Check 0-tokens' : 'Prompt IA'}
                        </span>
                      </div>
                      <p class="text-[11px] text-[var(--text-muted)] line-clamp-2">
                        {auto.kind === 'check' ? formatCheckLabels(auto.prompt) : auto.prompt}
                      </p>
                    </div>

                    <div class="flex items-center gap-1 shrink-0">
                      <button
                        type="button"
                        onclick={() => handleToggleEnabled(auto)}
                        class="px-2 py-1 rounded-lg border text-[10px] font-semibold transition-all {auto.enabled ? 'bg-emerald-500/15 border-emerald-500/35 text-emerald-400' : 'bg-[var(--bg-elevated)] border-[var(--border-subtle)] text-[var(--text-faint)]'}"
                      >
                        {auto.enabled ? 'Ativa' : 'Pausada'}
                      </button>
                      <button
                        type="button"
                        disabled={runningAutoId === auto.id || auto.last_status === 'running'}
                        onclick={() => handleRunNow(auto)}
                        class="p-1.5 rounded-lg bg-[var(--bg-elevated)] border border-[var(--border-subtle)] text-[var(--text-secondary)] hover:text-[var(--accent-fg)] disabled:opacity-45"
                        title="Executar agora"
                      >
                        <Icon name="play" size={12} />
                      </button>
                      <button
                        type="button"
                        onclick={() => handleDeleteAutomation(auto.id)}
                        class="p-1.5 rounded-lg bg-[var(--bg-elevated)] border border-[var(--border-subtle)] text-[var(--text-faint)] hover:text-rose-400"
                        title="Remover automação"
                      >
                        <Icon name="trash" size={12} />
                      </button>
                    </div>
                  </div>

                  <div class="flex flex-wrap items-center justify-between gap-2 text-[10px] text-[var(--text-faint)] pt-1 border-t border-[var(--border-subtle)]/70">
                    <span>
                      Próxima: {auto.next_run_at ? new Date(auto.next_run_at).toLocaleString('pt-BR', { day: '2-digit', month: '2-digit', hour: '2-digit', minute: '2-digit' }) : 'Pausada'}
                    </span>
                    {#if auto.last_status}
                      <span class="font-mono {auto.last_status === 'error' ? 'text-rose-400' : auto.last_status === 'ok' ? 'text-emerald-400' : 'text-[var(--accent-fg)]'}">
                        Última: {auto.last_status.toUpperCase()}
                      </span>
                    {/if}
                  </div>

                  {#if auto.last_summary}
                    <div class="p-2 rounded-lg bg-[var(--bg-elevated)] border border-[var(--border-subtle)] text-[11px] text-[var(--text-secondary)] space-y-1">
                      <p class="whitespace-pre-wrap break-words">{auto.last_summary}</p>
                      {#if auto.last_conv_id}
                        <button
                          type="button"
                          onclick={() => {
                            onOpenConversation(auto.last_conv_id, auto.name);
                            open = false;
                          }}
                          class="text-[11px] font-semibold text-[var(--accent-fg)] hover:underline"
                        >
                          Abrir conversa gerada
                        </button>
                      {/if}
                    </div>
                  {/if}
                </div>
              {/each}
            </div>
          {/if}
        {:else if tab === 'alerts'}
          <!-- Browser Notification + Mobile PWA (Android & iPhone iOS) Card -->
          <div class="p-3 rounded-xl bg-[var(--bg-input)] border border-[var(--border-subtle)] space-y-2.5">
            <div class="flex flex-wrap items-center justify-between gap-2">
              <div class="min-w-0 flex-1">
                <div class="flex flex-wrap items-center gap-1.5">
                  <span class="text-xs font-bold text-[var(--text-primary)]">Notificações no Navegador & PWA (PC / Android / iPhone)</span>
                  {#if pushStatus.isStandalone}
                    <span class="px-1.5 py-0.5 rounded bg-emerald-500/15 border border-emerald-500/35 text-[9px] font-semibold text-emerald-400">
                      PWA instalado
                    </span>
                  {/if}
                </div>
                <div class="text-[11px] text-[var(--text-muted)] mt-0.5 leading-relaxed">
                  {#if pushStatus.isIOS && !pushStatus.isStandalone}
                    {#if !pushStatus.isSecure}
                      No Safari (iPhone/iPad), alertas fora da página exigem acesso por <strong>rota segura HTTPS / domínio próprio</strong> + Compartilhar -&gt; <strong>"Adicionar à Tela de Início"</strong>. Em HTTP local, os avisos continuam aparecendo no navegador aberto.
                    {:else}
                      No iPhone/iPad (iOS 16.4+): no Safari, toque em <strong>Compartilhar</strong> -&gt; <strong>"Adicionar à Tela de Início"</strong> e abra pelo ícone criado para liberar Web Push no sistema.
                    {/if}
                  {:else if !pushStatus.isSecure}
                    Em HTTP local (<code class="font-mono text-[10px]">http://192.168...</code>), os alertas ficam ativos dentro da página no navegador. Notificações nativas no sistema mobile (PWA + Web Push) funcionam automaticamente ao acessar por <strong>rota segura HTTPS ou domínio próprio</strong>.
                  {:else if pushStatus.permission === 'denied'}
                    Permissão bloqueada nas configurações deste navegador/sistema.
                  {:else if pushStatus.subscribed}
                    Ativo neste aparelho (notificações na aba + Web Push no sistema via Service Worker).
                  {:else if pushStatus.permission === 'granted'}
                    Notificações do navegador permitidas (toque ao lado para registrar Web Push em segundo plano).
                  {:else}
                    Clique para permitir notificações deste navegador e receber alertas no sistema via Web Push (Android, iPhone e Desktop).
                  {/if}
                </div>
              </div>
              <div class="flex flex-wrap items-center gap-1.5 shrink-0">
                {#if canInstallPWA && !pushStatus.isStandalone}
                  <button
                    type="button"
                    disabled={installingPWA}
                    onclick={handleInstallApp}
                    class="px-2.5 py-1.5 rounded-xl bg-[var(--bg-elevated)] border border-[var(--accent-border)] text-[var(--accent-fg)] text-xs font-semibold transition-all active:scale-95"
                  >
                    {installingPWA ? 'Instalando...' : 'Instalar PWA'}
                  </button>
                {/if}
                <button
                  type="button"
                  disabled={!pushStatus.supported || pushStatus.permission === 'denied' || pushBusy}
                  onclick={handleToggleWebPush}
                  class="px-3 py-1.5 rounded-xl border text-xs font-semibold shrink-0 disabled:opacity-40 transition-all {pushStatus.subscribed || pushStatus.permission === 'granted' ? 'bg-emerald-500/15 border-emerald-500/35 text-emerald-400' : 'bg-[var(--accent-solid)] border-[var(--accent-solid)] text-[var(--accent-on-solid)]'}"
                >
                  {pushBusy ? '...' : pushStatus.subscribed ? 'Desativar Push' : 'Ativar no Navegador'}
                </button>
              </div>
            </div>
          </div>

          <!-- Active Rules & Channels Form -->
          <form onsubmit={handleSaveAlertRules} class="p-3 rounded-xl bg-[var(--bg-input)] border border-[var(--border-subtle)] space-y-3">
            <div class="text-xs font-bold text-[var(--text-primary)]">Sistema Operacional, ntfy & Gatilhos Automáticos</div>

            <label class="p-2.5 rounded-lg bg-[var(--bg-elevated)] border border-[var(--border-subtle)] flex items-center justify-between gap-2 cursor-pointer">
              <div>
                <div class="text-xs font-medium text-[var(--text-primary)]">Notificação nativa do Sistema (Windows / macOS / Linux)</div>
                <div class="text-[10px] text-[var(--text-faint)]">Envia alerta direto na Central de Notificações do PC onde o gateway roda (funciona sem navegador aberto).</div>
              </div>
              <input type="checkbox" bind:checked={alertConfig.notify_desktop} />
            </label>

            <div class="grid grid-cols-1 sm:grid-cols-2 gap-2.5">
              <div>
                <div class="flex items-center justify-between gap-2 mb-1">
                  <label for="ntfy-topic" class="block text-[11px] text-[var(--text-muted)]">Tópico ntfy (opcional)</label>
                  <button
                    type="button"
                    onclick={generateRandomNtfyTopic}
                    class="text-[10px] font-semibold text-[var(--accent-fg)] hover:underline"
                  >
                    Gerar tópico
                  </button>
                </div>
                <input
                  id="ntfy-topic"
                  type="text"
                  bind:value={alertConfig.ntfy_topic}
                  placeholder="ex: meu-agy-gateway"
                  class="w-full bg-[var(--bg-elevated)] border border-[var(--border-subtle)] rounded-lg px-2.5 py-1.5 text-xs text-[var(--text-primary)]"
                />
              </div>
              <div>
                <label for="ntfy-url" class="block text-[11px] text-[var(--text-muted)] mb-1">Servidor ntfy</label>
                <input
                  id="ntfy-url"
                  type="text"
                  bind:value={alertConfig.ntfy_url}
                  placeholder="https://ntfy.sh"
                  class="w-full bg-[var(--bg-elevated)] border border-[var(--border-subtle)] rounded-lg px-2.5 py-1.5 text-xs text-[var(--text-primary)]"
                />
              </div>
            </div>

            <div class="grid grid-cols-1 sm:grid-cols-2 gap-2.5 pt-1">
              <label class="p-2.5 rounded-lg bg-[var(--bg-elevated)] border border-[var(--border-subtle)] flex items-center justify-between gap-2 cursor-pointer">
                <span class="text-xs text-[var(--text-secondary)]">Notificar tarefas concluídas/erro</span>
                <input type="checkbox" bind:checked={alertConfig.notify_job_done} />
              </label>

              <label class="p-2.5 rounded-lg bg-[var(--bg-elevated)] border border-[var(--border-subtle)] flex items-center justify-between gap-2 cursor-pointer">
                <span class="text-xs text-[var(--text-secondary)]">Alertar se container Docker cair</span>
                <input type="checkbox" bind:checked={alertConfig.notify_docker_down} />
              </label>
            </div>

            <div class="grid grid-cols-3 gap-2">
              <div>
                <label for="long-job-sec" class="block text-[10px] text-[var(--text-muted)] mb-1">Job longo min (seg)</label>
                <input
                  id="long-job-sec"
                  type="number"
                  min={0}
                  max={3600}
                  bind:value={alertConfig.notify_long_job_sec}
                  class="w-full bg-[var(--bg-elevated)] border border-[var(--border-subtle)] rounded-lg px-2 py-1.5 text-xs font-mono text-[var(--text-primary)]"
                />
              </div>
              <div>
                <label for="gpu-temp-lim" class="block text-[10px] text-[var(--text-muted)] mb-1">GPU máx (°C, 0=off)</label>
                <input
                  id="gpu-temp-lim"
                  type="number"
                  min={0}
                  max={115}
                  bind:value={alertConfig.gpu_temp_threshold_c}
                  class="w-full bg-[var(--bg-elevated)] border border-[var(--border-subtle)] rounded-lg px-2 py-1.5 text-xs font-mono text-[var(--text-primary)]"
                />
              </div>
              <div>
                <label for="disk-min-pct" class="block text-[10px] text-[var(--text-muted)] mb-1">Disco mín (%, 0=off)</label>
                <input
                  id="disk-min-pct"
                  type="number"
                  min={0}
                  max={95}
                  bind:value={alertConfig.disk_free_min_pct}
                  class="w-full bg-[var(--bg-elevated)] border border-[var(--border-subtle)] rounded-lg px-2 py-1.5 text-xs font-mono text-[var(--text-primary)]"
                />
              </div>
            </div>

            <div class="flex flex-wrap items-center justify-between gap-2 pt-1">
              <button
                type="button"
                disabled={testingAlert}
                onclick={handleTestAlert}
                class="px-3 py-1.5 rounded-xl bg-[var(--bg-elevated)] border border-[var(--border-subtle)] hover:border-[var(--accent-border)] text-xs font-medium text-[var(--text-secondary)] hover:text-[var(--accent-fg)]"
              >
                {testingAlert ? 'Enviando...' : 'Enviar Alerta de Teste'}
              </button>

              <button
                type="submit"
                disabled={savingRules}
                class="px-3.5 py-1.5 rounded-xl bg-[var(--accent-solid)] hover:bg-[var(--accent-hover)] text-[var(--accent-on-solid)] text-xs font-semibold"
              >
                {savingRules ? 'Salvando...' : 'Salvar Regras'}
              </button>
            </div>
          </form>
        {:else}
          <!-- Alerts History Feed -->
          <div class="flex items-center justify-between gap-2">
            <span class="text-xs text-[var(--text-muted)]">{alerts.length} evento(s) recente(s)</span>
            <div class="flex items-center gap-2">
              {#if alerts.length > 0}
                <button
                  type="button"
                  onclick={handleClearHistory}
                  class="px-2.5 py-1 rounded-lg bg-[var(--bg-input)] border border-[var(--border-subtle)] text-[11px] text-[var(--text-muted)] hover:text-rose-400"
                >
                  Limpar histórico
                </button>
              {/if}
            </div>
          </div>

          {#if alerts.length === 0}
            <div class="py-8 text-center text-xs text-[var(--text-faint)] border border-dashed border-[var(--border-subtle)] rounded-xl">
              Nenhum alerta registrado ainda.
            </div>
          {:else}
            <div class="space-y-2">
              {#each alerts as ev (ev.id)}
                <div class="p-2.5 rounded-xl bg-[var(--bg-input)] border border-[var(--border-subtle)] space-y-1">
                  <div class="flex items-center justify-between gap-2">
                    <div class="flex items-center gap-1.5 min-w-0">
                      <span class="px-1.5 py-0.5 rounded border text-[9px] font-mono uppercase {severityStyle(ev.severity)}">
                        {ev.source}
                      </span>
                      <span class="text-xs font-semibold text-[var(--text-primary)] truncate">{ev.title}</span>
                    </div>
                    <span class="text-[10px] text-[var(--text-faint)] shrink-0">
                      {new Date(ev.created_at).toLocaleString('pt-BR', { day: '2-digit', month: '2-digit', hour: '2-digit', minute: '2-digit' })}
                    </span>
                  </div>
                  <p class="text-[11px] text-[var(--text-secondary)] whitespace-pre-wrap break-words">{ev.message}</p>
                  {#if ev.conv_id}
                    <button
                      type="button"
                      onclick={() => {
                        onOpenConversation(ev.conv_id || '', ev.title);
                        open = false;
                      }}
                      class="text-[11px] font-semibold text-[var(--accent-fg)] hover:underline"
                    >
                      Abrir sessão relacionada
                    </button>
                  {/if}
                </div>
              {/each}
            </div>
          {/if}
        {/if}
      </div>
    </div>
  </div>
{/if}
