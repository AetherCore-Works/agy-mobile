<script lang="ts">
  import { fade, fly } from 'svelte/transition';
  import { cubicOut } from 'svelte/easing';
  import Icon from './Icon.svelte';
  import {
    deleteConversation,
    fetchConversations,
    fetchTelemetry,
    fetchWatchdog,
    updateSettings,
    type ConversationSummary,
    type GatewaySettings,
    type ModelInfo,
    type PermissionMode,
    type TelemetryData,
    type WatchdogState,
  } from '../lib/api';
  import { THEME_PALETTES, type ThemeMode, type ThemePalette } from '../lib/theme';

  let {
    open = $bindable(false),
    activeConvId,
    models,
    watchdogEnabled = true,
    telemetryEnabled = false,
    themeMode,
    themePalette,
    settings = $bindable<GatewaySettings>(),
    onSelectConversation,
    onNewConversation,
    onSettingsSaved,
    onThemeChange,
  }: {
    open: boolean;
    activeConvId: string | null;
    models: ModelInfo[];
    watchdogEnabled?: boolean;
    telemetryEnabled?: boolean;
    themeMode: ThemeMode;
    themePalette: ThemePalette;
    settings: GatewaySettings;
    onSelectConversation: (id: string) => void;
    onNewConversation: () => void;
    onSettingsSaved: (s: GatewaySettings) => void;
    onThemeChange: (mode: ThemeMode, palette: ThemePalette) => void;
  } = $props();

  let activeTab = $state<'chats' | 'settings'>('chats');
  let conversations = $state<ConversationSummary[]>([]);
  let searchQuery = $state('');
  let loading = $state(false);
  let hasMore = $state(false);
  let offset = $state(0);
  let savingSettings = $state(false);
  let settingsSavedMsg = $state('');
  let watchdog = $state<WatchdogState | null>(null);
  let telemetry = $state<TelemetryData | null>(null);

  const PAGE_SIZE = 30;

  async function load(reset = true) {
    loading = true;
    try {
      const nextOffset = reset ? 0 : offset;
      const data = await fetchConversations(searchQuery, PAGE_SIZE, nextOffset);
      if (reset) {
        conversations = data.conversations;
      } else {
        conversations = [...conversations, ...data.conversations];
      }
      hasMore = data.has_more;
      offset = nextOffset + data.conversations.length;
    } finally {
      loading = false;
    }
  }

  let searchTimer: ReturnType<typeof setTimeout> | null = null;
  function handleSearchInput() {
    if (searchTimer) clearTimeout(searchTimer);
    searchTimer = setTimeout(() => load(true), 250);
  }

  async function handleDelete(e: MouseEvent, id: string) {
    e.stopPropagation();
    await deleteConversation(id);
    await load(true);
  }

  async function saveSettingsPatch(patch: Partial<GatewaySettings>) {
    savingSettings = true;
    settingsSavedMsg = '';
    try {
      const updated = await updateSettings(patch);
      settings = updated;
      onSettingsSaved(updated);
      settingsSavedMsg = 'Salvo permanentemente';
      setTimeout(() => (settingsSavedMsg = ''), 2000);
    } finally {
      savingSettings = false;
    }
  }

  $effect(() => {
    if (open && activeTab === 'chats') {
      load(true);
    } else if (open && activeTab === 'settings') {
      if (watchdogEnabled) {
        fetchWatchdog().then((w) => (watchdog = w)).catch(() => {});
      }
      if (telemetryEnabled) {
        fetchTelemetry().then((t) => (telemetry = t)).catch(() => {});
      }
    }
  });

  const permOptions: { value: PermissionMode; label: string; desc: string; icon: 'shield' | 'zap' | 'eye' }[] = [
    { value: 'copilot', label: 'Copiloto', desc: 'Aprova edições (--mode accept-edits)', icon: 'shield' },
    { value: 'autonomous', label: 'Autônomo', desc: 'Execução total sem bloqueios', icon: 'zap' },
    { value: 'readonly', label: 'Leitura', desc: 'Somente planejamento (--mode plan)', icon: 'eye' },
  ];
</script>

{#if open}
  <!-- Backdrop -->
  <button
    transition:fade={{ duration: 180 }}
    type="button"
    aria-label="Fechar menu"
    onclick={() => (open = false)}
    data-no-ripple
    class="fixed inset-0 bg-[var(--bg-backdrop)] backdrop-blur-sm z-40"
  ></button>

  <!-- Drawer -->
  <aside
    transition:fly={{ x: -320, duration: 220, easing: cubicOut }}
    class="fixed inset-y-0 left-0 w-80 max-w-[88vw] bg-[var(--bg-elevated)] border-r border-[var(--border-subtle)] z-50 flex flex-col shadow-2xl select-none theme-surface"
  >
    <!-- Header -->
    <div class="p-3.5 border-b border-[var(--border-subtle)] flex items-center justify-between">
      <div class="flex items-center gap-2">
        <Icon name="clock" size={17} class="text-[var(--accent-fg)]" />
        <h2 class="text-sm font-bold text-[var(--text-primary)]">AGY Gateway</h2>
        {#if loading && conversations.length > 0}
          <span class="px-1.5 py-0.5 rounded-full bg-[var(--accent-soft)] border border-[var(--accent-border)] text-[10px] text-[var(--accent-fg)] animate-pulse">Sincronizando</span>
        {/if}
      </div>
      <button
        type="button"
        onclick={() => (open = false)}
        class="p-1 rounded-lg text-[var(--text-muted)] hover:text-[var(--text-primary)] transition-colors"
      >
        <Icon name="close" size={18} />
      </button>
    </div>

    <!-- Tabs -->
    <div class="grid grid-cols-2 border-b border-[var(--border-subtle)] bg-[var(--bg-input)] p-1 gap-1 theme-surface">
      <button
        type="button"
        onclick={() => (activeTab = 'chats')}
        class="py-1.5 rounded-lg text-xs font-semibold flex items-center justify-center gap-1.5 transition-all {activeTab === 'chats' ? 'bg-[var(--bg-elevated)] text-[var(--text-primary)] shadow-sm border border-[var(--border-subtle)]' : 'text-[var(--text-muted)] hover:text-[var(--text-primary)]'}"
      >
        <Icon name="clock" size={14} />
        <span>Sessões</span>
      </button>
      <button
        type="button"
        onclick={() => (activeTab = 'settings')}
        class="py-1.5 rounded-lg text-xs font-semibold flex items-center justify-center gap-1.5 transition-all {activeTab === 'settings' ? 'bg-[var(--bg-elevated)] text-[var(--text-primary)] shadow-sm border border-[var(--border-subtle)]' : 'text-[var(--text-muted)] hover:text-[var(--text-primary)]'}"
      >
        <Icon name="settings" size={14} />
        <span>Configs</span>
      </button>
    </div>

    {#if activeTab === 'chats'}
      <div class="p-3 border-b border-[var(--border-subtle)] space-y-2">
        <button
          type="button"
          onclick={() => {
            onNewConversation();
            open = false;
          }}
          class="w-full py-2 px-3 rounded-xl bg-[var(--accent-solid)] hover:bg-[var(--accent-hover)] text-[var(--accent-on-solid)] text-xs font-semibold flex items-center justify-center gap-1.5 shadow-sm transition-all active:scale-[0.98]"
        >
          <Icon name="plus" size={15} />
          <span>Nova Conversa</span>
        </button>

        <div class="relative flex items-center">
          <Icon name="search" size={14} class="absolute left-2.5 text-[var(--text-faint)] pointer-events-none" />
          <input
            type="text"
            bind:value={searchQuery}
            oninput={handleSearchInput}
            placeholder="Buscar (FTS5)..."
            class="w-full bg-[var(--bg-input)] border border-[var(--border-subtle)] rounded-lg pl-8 pr-2.5 py-1.5 text-xs text-[var(--text-primary)] placeholder-[var(--text-faint)] focus:outline-none focus:border-[var(--accent-fg)] transition-colors"
          />
        </div>
      </div>

      <div class="flex-1 overflow-y-auto p-2 space-y-1.5">
        {#if loading && conversations.length === 0}
          <div class="text-center py-8 text-xs text-[var(--text-faint)]">Sincronizando sessões...</div>
        {:else if conversations.length === 0}
          <div class="text-center py-8 text-xs text-[var(--text-faint)]">Nenhuma sessão encontrada.</div>
        {:else}
          {#each conversations as c (c.id)}
            <div
              animate:flip={{ duration: 300, easing: cubicOut }}
              role="button"
              tabindex="0"
              onclick={() => {
                onSelectConversation(c.id);
                open = false;
              }}
              onkeydown={(e) => {
                if (e.key === 'Enter') {
                  onSelectConversation(c.id);
                  open = false;
                }
              }}
              class="w-full text-left p-2.5 rounded-xl border transition-all cursor-pointer flex items-start justify-between gap-2 {activeConvId === c.id ? 'bg-[var(--accent-soft)] border-[var(--accent-border)]' : 'bg-[var(--bg-input)] border-[var(--border-subtle)] hover:border-[var(--border-strong)]'}"
            >
              <div class="min-w-0 flex-1">
                <div class="text-xs font-medium text-[var(--text-primary)] truncate">{c.title || 'Sem título'}</div>
                <div class="flex items-center gap-1.5 mt-1 text-[10px] text-[var(--text-faint)]">
                  <span class="px-1.5 py-0.5 rounded bg-[var(--bg-chip)] text-[var(--text-secondary)] font-mono">{c.engine === 'ollama' ? 'Local' : 'AGY'}</span>
                  <span>{c.updated_at ? new Date(c.updated_at).toLocaleDateString('pt-BR', { day: '2-digit', month: '2-digit', hour: '2-digit', minute: '2-digit' }) : ''}</span>
                </div>
              </div>
              {#if c.engine === 'ollama'}
                <button
                  type="button"
                  onclick={(e) => handleDelete(e, c.id)}
                  class="p-1 rounded text-[var(--text-faint)] hover:text-rose-400 shrink-0 transition-colors"
                  title="Excluir"
                >
                  <Icon name="trash" size={14} />
                </button>
              {/if}
            </div>
          {/each}

          {#if hasMore}
            <button
              type="button"
              onclick={() => load(false)}
              disabled={loading}
              class="w-full py-2 rounded-lg bg-[var(--bg-chip)] hover:bg-[var(--bg-chip-hover)] text-xs text-[var(--text-secondary)] font-medium transition-colors"
            >
              {loading ? 'Carregando...' : 'Carregar mais'}
            </button>
          {/if}
        {/if}
      </div>
    {:else}
      <!-- Settings Tab -->
      <div class="flex-1 overflow-y-auto p-3.5 space-y-5">
        <!-- Appearance: Mode & Color Palette -->
        <div class="space-y-3">
          <div class="flex items-center justify-between">
            <span class="text-xs font-semibold text-[var(--text-secondary)]">Aparência</span>
            <span class="text-[10px] font-mono text-[var(--text-faint)] uppercase">{themeMode === 'dark' ? 'Escuro' : 'Claro'}</span>
          </div>

          <div class="grid grid-cols-2 gap-2">
            <button
              type="button"
              onclick={() => onThemeChange('dark', themePalette)}
              class="p-2 rounded-xl border text-xs font-semibold flex items-center justify-center gap-1.5 transition-all active:scale-[0.98] {themeMode === 'dark' ? 'bg-[var(--accent-soft)] border-[var(--accent-border)] text-[var(--accent-fg)]' : 'bg-[var(--bg-input)] border-[var(--border-subtle)] text-[var(--text-muted)] hover:border-[var(--border-strong)]'}"
            >
              <Icon name="moon" size={14} />
              <span>Escuro</span>
            </button>
            <button
              type="button"
              onclick={() => onThemeChange('light', themePalette)}
              class="p-2 rounded-xl border text-xs font-semibold flex items-center justify-center gap-1.5 transition-all active:scale-[0.98] {themeMode === 'light' ? 'bg-[var(--accent-soft)] border-[var(--accent-border)] text-[var(--accent-fg)]' : 'bg-[var(--bg-input)] border-[var(--border-subtle)] text-[var(--text-muted)] hover:border-[var(--border-strong)]'}"
            >
              <Icon name="sun" size={14} />
              <span>Claro</span>
            </button>
          </div>

          <div class="grid grid-cols-3 gap-1.5">
            {#each THEME_PALETTES as p}
              <button
                type="button"
                onclick={() => onThemeChange(themeMode, p.id)}
                class="px-2 py-1.5 rounded-xl border text-[11px] font-medium flex items-center gap-1.5 transition-all active:scale-[0.97] {themePalette === p.id ? 'bg-[var(--accent-soft)] border-[var(--accent-border)] text-[var(--text-primary)]' : 'bg-[var(--bg-input)] border-[var(--border-subtle)] text-[var(--text-muted)] hover:border-[var(--border-strong)]'}"
              >
                <span class="w-3 h-3 rounded-full shrink-0 shadow-sm" style="background-color: {p.swatch};"></span>
                <span class="truncate">{p.label}</span>
              </button>
            {/each}
          </div>
        </div>

        <div>
          <div class="block text-xs font-semibold text-[var(--text-secondary)] mb-2">Modo de Permissão Padrão</div>
          <div class="space-y-2">
            {#each permOptions as opt}
              <button
                type="button"
                onclick={() => saveSettingsPatch({ default_permission_mode: opt.value })}
                class="w-full text-left p-2.5 rounded-xl border transition-all flex items-start gap-2.5 active:scale-[0.99] {settings.default_permission_mode === opt.value ? 'bg-[var(--accent-soft)] border-[var(--accent-border)] text-[var(--text-primary)]' : 'bg-[var(--bg-input)] border-[var(--border-subtle)] text-[var(--text-muted)] hover:border-[var(--border-strong)]'}"
              >
                <Icon name={opt.icon} size={16} class={settings.default_permission_mode === opt.value ? 'text-[var(--accent-fg)] mt-0.5' : 'text-[var(--text-faint)] mt-0.5'} />
                <div class="min-w-0 flex-1">
                  <div class="text-xs font-semibold flex items-center justify-between">
                    <span>{opt.label}</span>
                    {#if settings.default_permission_mode === opt.value}
                      <Icon name="check" size={14} class="text-[var(--accent-fg)]" />
                    {/if}
                  </div>
                  <div class="text-[11px] text-[var(--text-faint)] mt-0.5">{opt.desc}</div>
                </div>
              </button>
            {/each}
          </div>
        </div>

        <div>
          <label for="cfg-default-model" class="block text-xs font-semibold text-[var(--text-secondary)] mb-1.5">Modelo Padrão</label>
          <select
            id="cfg-default-model"
            value={settings.default_model}
            onchange={(e) => saveSettingsPatch({ default_model: (e.target as HTMLSelectElement).value })}
            class="w-full bg-[var(--bg-input)] border border-[var(--border-subtle)] rounded-xl px-3 py-2 text-xs text-[var(--text-primary)] focus:outline-none focus:border-[var(--accent-fg)]"
          >
            {#each models as m}
              <option value={m.id}>{m.name} ({m.badge})</option>
            {/each}
          </select>
        </div>

        {#if settingsSavedMsg}
          <div
            transition:fade={{ duration: 150 }}
            class="p-2.5 rounded-xl bg-emerald-500/15 border border-emerald-500/35 text-emerald-500 text-xs flex items-center gap-2"
          >
            <Icon name="check" size={14} />
            <span>{settingsSavedMsg}</span>
          </div>
        {/if}

        <div class="pt-2 border-t border-[var(--border-subtle)] text-[11px] text-[var(--text-faint)] space-y-1.5">
          <p>As configurações ficam salvas no SQLite local do Gateway e sincronizam automaticamente entre sessões.</p>
          {#if savingSettings}
            <p class="text-[var(--accent-fg)]">Salvando...</p>
          {/if}
        </div>

        {#if watchdogEnabled && watchdog}
          <div class="p-3 rounded-xl bg-[var(--bg-input)] border border-[var(--border-subtle)] space-y-1.5 theme-surface">
            <div class="flex items-center justify-between text-xs font-semibold text-[var(--text-primary)]">
              <span>Watchdog</span>
              <span class="px-1.5 py-0.5 rounded text-[10px] {watchdog.online ? 'bg-emerald-500/15 text-emerald-500 border border-emerald-500/35' : 'bg-rose-500/15 text-rose-400 border border-rose-500/35'}">
                {watchdog.online ? 'Online' : 'Offline'}
              </span>
            </div>
            <div class="text-[11px] text-[var(--text-faint)] flex justify-between">
              <span>Verificações: {watchdog.uptime_checks}</span>
              <span>Quedas: {watchdog.downtime_events}</span>
            </div>
            {#if watchdog.last_seen_online}
              <div class="text-[10px] text-[var(--text-faint)]">
                Visto por último: {new Date(watchdog.last_seen_online).toLocaleTimeString('pt-BR')}
              </div>
            {/if}
          </div>
        {/if}

        {#if telemetryEnabled && telemetry && telemetry.enabled}
          <div class="p-3 rounded-xl bg-[var(--bg-input)] border border-[var(--border-subtle)] space-y-1.5 theme-surface">
            <div class="flex items-center justify-between text-xs font-semibold text-[var(--text-primary)]">
              <span>Telemetria de Hardware</span>
              <Icon name="cpu" size={14} class="text-[var(--accent-fg)]" />
            </div>
            {#if telemetry.gpu}
              <div class="text-[11px] text-[var(--text-secondary)] flex justify-between">
                <span class="truncate pr-2">{telemetry.gpu.name}</span>
                <span class="font-mono text-[var(--accent-fg)] shrink-0">{telemetry.gpu.temp_c}°C</span>
              </div>
              <div class="text-[11px] text-[var(--text-faint)] flex justify-between">
                <span>VRAM</span>
                <span class="font-mono">{telemetry.gpu.vram_used_mb} / {telemetry.gpu.vram_total_mb} MB</span>
              </div>
            {/if}
            {#if telemetry.ram}
              <div class="text-[11px] text-[var(--text-faint)] flex justify-between">
                <span>RAM</span>
                <span class="font-mono">{Math.round(telemetry.ram.used_mb / 1024)} / {Math.round(telemetry.ram.total_mb / 1024)} GB</span>
              </div>
            {/if}
            {#if telemetry.cpu_temp !== null}
              <div class="text-[11px] text-[var(--text-faint)] flex justify-between">
                <span>CPU Temp</span>
                <span class="font-mono">{telemetry.cpu_temp}°C</span>
              </div>
            {/if}
          </div>
        {/if}
      </div>
    {/if}
  </aside>
{/if}
