<script lang="ts">
  import { onDestroy, onMount } from 'svelte';
  import { fade, fly } from 'svelte/transition';
  import { cubicOut } from 'svelte/easing';
  import Icon from './components/Icon.svelte';
  import AiSpinner from './components/AiSpinner.svelte';
  import ChatPane from './components/ChatPane.svelte';
  import Sidebar from './components/Sidebar.svelte';
  import TerminalModal from './components/TerminalModal.svelte';
  import OutboxModal from './components/OutboxModal.svelte';
  import ShareModal from './components/ShareModal.svelte';
  import {
    deleteConversation,
    fetchFeatures,
    fetchModels,
    fetchOutbox,
    fetchSettings,
    fetchStatus,
    triggerWol,
    updateSettings,
    type ChatMessage,
    type GatewaySettings,
    type ModelInfo,
    type OutboxFile,
    type PermissionMode,
  } from './lib/api';
  import {
    THEME_PALETTES,
    applyTheme,
    hasStoredTheme,
    normalizeThemeMode,
    normalizeThemePalette,
    readStoredTheme,
    type ThemeMode,
    type ThemePalette,
  } from './lib/theme';

  interface TabEntry {
    id: string;
    storageKeyPrefix: string;
    title: string;
    convId: string | null;
    isGenerating: boolean;
  }

  // Theme State (applied immediately to avoid FOUC)
  const initialTheme = readStoredTheme();
  let themeMode = $state<ThemeMode>(initialTheme.mode);
  let themePalette = $state<ThemePalette>(initialTheme.palette);
  let themeMenuOpen = $state(false);
  applyTheme(initialTheme.mode, initialTheme.palette, false);

  let activeSwatch = $derived(
    THEME_PALETTES.find((p) => p.id === themePalette)?.swatch ?? '#0ea5e9'
  );

  function setTheme(mode: ThemeMode, palette: ThemePalette) {
    themeMode = mode;
    themePalette = palette;
    applyTheme(mode, palette, true);
    updateSettings({ theme_mode: mode, theme_palette: palette })
      .then((updated) => {
        settings = updated;
      })
      .catch(() => {});
  }

  // Gateway Shared State
  let targetOnline = $state<boolean | null>(null);
  let gatewayMode = $state<'gateway' | 'standalone' | 'ide-bridge'>('gateway');
  let outboxDir = $state('');
  let features = $state<Record<string, any>>({
    terminal: true,
    wol: true,
    watchdog: true,
    ollama: true,
    inbox_outbox: true,
  });
  let models = $state<ModelInfo[]>([
    { id: 'gemini-4-hydrogen-high', name: 'Gemini 4 Hydrogen (High)', type: 'cloud', badge: 'Hydrogen' },
    { id: 'gemini-4-hydrogen-xhigh', name: 'Gemini 4 Hydrogen (Extra)', type: 'cloud', badge: 'Extra' },
    { id: 'gemini-4-hydrogen-medium', name: 'Gemini 4 Hydrogen (Medium)', type: 'cloud', badge: 'Hydrogen' },
  ]);
  let settings = $state<GatewaySettings>({
    default_permission_mode: 'copilot',
    default_model: 'gemini-4-hydrogen-high',
    default_effort: '',
    theme_mode: initialTheme.mode,
    theme_palette: initialTheme.palette,
  });
  let defaultModel = $state('gemini-4-hydrogen-high');
  let defaultPermission = $state<PermissionMode>('copilot');

  let hostLabel = $derived(
    gatewayMode === 'standalone'
      ? 'AGY Local'
      : gatewayMode === 'ide-bridge'
        ? 'IDE Bridge'
        : 'Remote Host'
  );

  // Tabs & Desktop Split View State
  let tabCounter = 1;
  let tabs = $state<TabEntry[]>([
    {
      id: 'tab-1',
      storageKeyPrefix: 'agy_',
      title: '',
      convId: localStorage.getItem('agy_conv_id'),
      isGenerating: false,
    },
  ]);
  let leftTabId = $state('tab-1');
  let rightTabId = $state('tab-1');
  let splitOpen = $state(false);
  let focusedSide = $state<'left' | 'right'>('left');
  let compareOpen = $state(false);
  let comparePrompt = $state('');
  let paneRefs: Record<string, ChatPane | undefined> = {};

  let focusedTabId = $derived(splitOpen && focusedSide === 'right' ? rightTabId : leftTabId);
  let focusedConvId = $derived(tabs.find((t) => t.id === focusedTabId)?.convId ?? null);
  let anyGenerating = $derived(tabs.some((t) => t.isGenerating));

  function createTab(activate = true): TabEntry {
    if (tabs.length >= 6) {
      return tabs[tabs.length - 1];
    }
    tabCounter += 1;
    const newId = `tab-${tabCounter}`;
    const entry: TabEntry = {
      id: newId,
      storageKeyPrefix: `agy_${newId}_`,
      title: '',
      convId: null,
      isGenerating: false,
    };
    tabs = [...tabs, entry];
    if (activate) {
      if (splitOpen && focusedSide === 'right') {
        rightTabId = newId;
      } else {
        leftTabId = newId;
      }
    }
    return entry;
  }

  function handleHeaderNewAction() {
    const curTab = tabs.find((t) => t.id === focusedTabId);
    const snap = paneRefs[focusedTabId]?.getSnapshotContext();
    if (curTab && snap && (snap.messages.length > 0 || curTab.convId)) {
      createTab(true);
    } else {
      paneRefs[focusedTabId]?.newConversation();
    }
  }

  function selectTab(tabId: string) {
    if (!splitOpen) {
      leftTabId = tabId;
      focusedSide = 'left';
      return;
    }
    if (focusedSide === 'left') {
      if (rightTabId === tabId) {
        rightTabId = leftTabId;
      }
      leftTabId = tabId;
    } else {
      if (leftTabId === tabId) {
        leftTabId = rightTabId;
      }
      rightTabId = tabId;
    }
  }

  function closeTab(e: MouseEvent, tabId: string) {
    e.stopPropagation();
    if (tabs.length <= 1) return;
    const remaining = tabs.filter((t) => t.id !== tabId);
    tabs = remaining;
    if (leftTabId === tabId) {
      leftTabId = remaining[0].id;
    }
    if (rightTabId === tabId) {
      rightTabId = remaining.find((t) => t.id !== leftTabId)?.id ?? remaining[0].id;
    }
    if (remaining.length < 2) {
      splitOpen = false;
      compareOpen = false;
      focusedSide = 'left';
    }
  }

  function toggleSplitView() {
    if (!splitOpen) {
      if (tabs.length < 2) {
        const second = createTab(false);
        rightTabId = second.id;
      } else if (rightTabId === leftTabId) {
        rightTabId = tabs.find((t) => t.id !== leftTabId)?.id ?? leftTabId;
      }
      splitOpen = true;
    } else {
      splitOpen = false;
      compareOpen = false;
      leftTabId = focusedTabId;
      focusedSide = 'left';
    }
  }

  async function handleCompareSubmit(e?: Event) {
    if (e) e.preventDefault();
    const prompt = comparePrompt.trim();
    if (!prompt || !splitOpen) return;
    comparePrompt = '';
    const left = paneRefs[leftTabId];
    const right = paneRefs[rightTabId];
    await Promise.all([
      left?.submitExternalPrompt(prompt),
      right && rightTabId !== leftTabId ? right.submitExternalPrompt(prompt) : Promise.resolve(),
    ]);
  }

  function handlePaneMetaChange(tabId: string, title: string, convId: string | null) {
    tabs = tabs.map((t) => (t.id === tabId ? { ...t, title, convId } : t));
  }

  function handlePaneGeneratingChange(tabId: string, generating: boolean) {
    tabs = tabs.map((t) => (t.id === tabId ? { ...t, isGenerating: generating } : t));
  }

  function visibilityForTab(tabId: string): 'active' | 'split-hidden-mobile' | 'hidden' {
    if (!splitOpen) {
      return tabId === leftTabId ? 'active' : 'hidden';
    }
    if (tabId === leftTabId && tabId === rightTabId) {
      return 'active';
    }
    if (tabId === leftTabId) {
      return focusedSide === 'left' ? 'active' : 'split-hidden-mobile';
    }
    if (tabId === rightTabId) {
      return focusedSide === 'right' ? 'active' : 'split-hidden-mobile';
    }
    return 'hidden';
  }

  function orderForTab(tabId: string): string {
    if (!splitOpen) return '';
    if (tabId === leftTabId) return 'md:order-1';
    if (tabId === rightTabId) return 'md:order-2';
    return '';
  }

  // Modals & Drawers
  let sidebarOpen = $state(false);
  let terminalOpen = $state(false);
  let outboxOpen = $state(false);
  let outboxFiles = $state<OutboxFile[]>([]);

  // Share Snapshot Modal
  let shareOpen = $state(false);
  let shareTitle = $state('Snapshot AGY');
  let shareSingleMD = $state('');
  let shareMessages = $state<ChatMessage[]>([]);

  function openShareModal(title: string, singleMarkdown: string, msgs: ChatMessage[]) {
    shareTitle = title || 'Snapshot AGY';
    shareSingleMD = singleMarkdown;
    shareMessages = msgs;
    shareOpen = true;
  }

  // Delete Chat Confirmation State
  let pendingDelete = $state<{ id: string; title: string } | null>(null);
  let deletingChat = $state(false);
  let deleteChatError = $state('');

  function requestDeleteConversation(convId: string, title: string) {
    deleteChatError = '';
    pendingDelete = { id: convId, title: title || 'Conversa atual' };
  }

  function handleConversationDeleted(deletedId: string) {
    for (const tab of tabs) {
      if (tab.convId === deletedId) {
        paneRefs[tab.id]?.newConversation();
      }
    }
  }

  async function confirmDeleteChat() {
    if (!pendingDelete || deletingChat) return;
    const targetId = pendingDelete.id;
    deletingChat = true;
    deleteChatError = '';
    try {
      await deleteConversation(targetId);
      pendingDelete = null;
      handleConversationDeleted(targetId);
    } catch (err: any) {
      deleteChatError = err?.message || 'Erro ao apagar chat';
    } finally {
      deletingChat = false;
    }
  }

  async function refreshStatusAndOutbox() {
    try {
      const st = await fetchStatus();
      targetOnline = st.online;
    } catch {
      targetOnline = false;
    }
    try {
      outboxFiles = await fetchOutbox();
    } catch {
      // ignore
    }
  }

  async function handleWakeOnLan() {
    await triggerWol();
    setTimeout(refreshStatusAndOutbox, 3000);
  }

  let statusInterval: ReturnType<typeof setInterval> | null = null;

  onMount(async () => {
    const [loadedSettings, loadedModels, loadedFeatures] = await Promise.all([
      fetchSettings().catch(() => settings),
      fetchModels().catch(() => models),
      fetchFeatures().catch(() => null),
      refreshStatusAndOutbox(),
    ]);

    settings = loadedSettings;
    if (!hasStoredTheme() && (loadedSettings.theme_mode || loadedSettings.theme_palette)) {
      const m = normalizeThemeMode(loadedSettings.theme_mode);
      const p = normalizeThemePalette(loadedSettings.theme_palette);
      themeMode = m;
      themePalette = p;
      applyTheme(m, p, true);
    }

    if (loadedModels.length > 0) models = loadedModels;
    if (loadedFeatures?.features) {
      features = loadedFeatures.features;
      if (loadedFeatures.mode === 'standalone' || loadedFeatures.mode === 'gateway' || loadedFeatures.mode === 'ide-bridge') {
        gatewayMode = loadedFeatures.mode;
      }
      if (loadedFeatures.dirs?.outbox) {
        outboxDir = loadedFeatures.dirs.outbox;
      }
    }

    defaultModel = loadedSettings.default_model || 'gemini-4-hydrogen-high';
    defaultPermission = loadedSettings.default_permission_mode || 'copilot';

    statusInterval = setInterval(refreshStatusAndOutbox, 15000);
  });

  onDestroy(() => {
    if (statusInterval) clearInterval(statusInterval);
  });
</script>

<svelte:window
  onkeydown={(e) => {
    if (e.key === 'Escape') {
      if (themeMenuOpen) themeMenuOpen = false;
      if (pendingDelete && !deletingChat) pendingDelete = null;
    }
  }}
/>

<div class="h-[100dvh] w-full max-w-[100vw] overflow-x-hidden flex flex-col bg-[var(--bg-app)] text-[var(--text-primary)] theme-surface">
  <!-- Header -->
  <header class="relative h-13 bg-[var(--bg-surface)] backdrop-blur-md border-b border-[var(--border-subtle)] px-2.5 sm:px-5 flex items-center justify-between gap-1.5 shrink-0 select-none z-30 theme-surface">
    <div class="flex items-center gap-2 min-w-0">
      <button
        type="button"
        onclick={() => (sidebarOpen = true)}
        class="p-2 rounded-xl text-[var(--text-secondary)] hover:bg-[var(--bg-chip)] transition-colors active:scale-95"
        aria-label="Menu"
      >
        <Icon name="menu" size={20} />
      </button>

      <div class="flex items-center gap-1.5 min-w-0">
        {#if anyGenerating}
          <AiSpinner size={14} class="text-[var(--accent-fg)]" />
        {:else}
          <span class="w-2 h-2 rounded-full shrink-0 transition-colors {targetOnline === true ? 'bg-emerald-500' : targetOnline === false ? 'bg-rose-500' : 'bg-amber-500'}"></span>
        {/if}
        <span class="text-xs font-bold tracking-tight truncate">{hostLabel}</span>
        {#if targetOnline === false && features.wol}
          <button
            type="button"
            onclick={handleWakeOnLan}
            class="px-2 py-0.5 rounded-md bg-amber-500/15 text-amber-500 border border-amber-500/35 text-[10px] font-semibold flex items-center gap-1 transition-transform active:scale-95"
          >
            <Icon name="power" size={11} />
            <span>Acordar</span>
          </button>
        {/if}
      </div>
    </div>

    <div class="flex items-center gap-1.5 shrink-0">
      <!-- Desktop Split View Toggle -->
      <button
        type="button"
        onclick={toggleSplitView}
        class="hidden md:inline-flex p-2 rounded-xl border transition-all active:scale-95 {splitOpen ? 'bg-[var(--accent-soft)] border-[var(--accent-border)] text-[var(--accent-fg)]' : 'bg-[var(--bg-chip)] border-transparent text-[var(--text-secondary)] hover:text-[var(--accent-fg)]'}"
        title={splitOpen ? 'Sair do Split View' : 'Split View (2 sessões lado a lado)'}
        aria-label="Alternar Split View"
      >
        <Icon name="columns" size={17} />
      </button>

      <!-- Share Active Conversation -->
      <button
        type="button"
        onclick={() => {
          const ctx = paneRefs[focusedTabId]?.getSnapshotContext();
          if (ctx && ctx.messages.length > 0) {
            openShareModal(ctx.title, '', ctx.messages);
          }
        }}
        class="hidden sm:inline-flex p-2 rounded-xl bg-[var(--bg-chip)] text-[var(--text-secondary)] hover:text-[var(--accent-fg)] transition-colors active:scale-95"
        title="Compartilhar / Exportar conversa"
        aria-label="Compartilhar conversa"
      >
        <Icon name="share" size={17} />
      </button>

      <!-- Delete Active Conversation -->
      {#if focusedConvId}
        <button
          type="button"
          onclick={() => {
            const focusedTab = tabs.find((t) => t.id === focusedTabId);
            const ctx = paneRefs[focusedTabId]?.getSnapshotContext();
            requestDeleteConversation(focusedConvId, ctx?.title || focusedTab?.title || 'Conversa atual');
          }}
          class="p-2 rounded-xl bg-[var(--bg-chip)] text-[var(--text-secondary)] hover:text-rose-400 transition-colors active:scale-95"
          title="Apagar chat atual"
          aria-label="Apagar chat atual"
        >
          <Icon name="trash" size={17} />
        </button>
      {/if}

      <!-- Theme & Color Palette Button -->
      <button
        type="button"
        onclick={() => (themeMenuOpen = !themeMenuOpen)}
        class="relative p-2 rounded-xl border transition-all active:scale-95 {themeMenuOpen ? 'bg-[var(--accent-soft)] border-[var(--accent-border)] text-[var(--accent-fg)]' : 'bg-[var(--bg-chip)] border-transparent text-[var(--text-secondary)] hover:text-[var(--accent-fg)]'}"
        title="Tema e paleta de cores"
        aria-label="Tema e paleta de cores"
      >
        <Icon name={themeMode === 'dark' ? 'moon' : 'sun'} size={17} />
        <span
          class="absolute bottom-1 right-1 w-2 h-2 rounded-full ring-1 ring-[var(--bg-surface)]"
          style="background-color: {activeSwatch};"
        ></span>
      </button>

      <button
        type="button"
        onclick={() => (outboxOpen = true)}
        class="relative p-2 rounded-xl bg-[var(--bg-chip)] text-[var(--text-secondary)] hover:text-[var(--accent-fg)] transition-colors active:scale-95"
        title="Outbox"
      >
        <Icon name="outbox" size={18} />
        {#if outboxFiles.length > 0}
          <span class="absolute -top-0.5 -right-0.5 min-w-[16px] h-4 px-1 rounded-full bg-[var(--accent-solid)] text-[var(--accent-on-solid)] text-[10px] font-bold flex items-center justify-center">
            {outboxFiles.length}
          </span>
        {/if}
      </button>

      {#if features.terminal}
        <button
          type="button"
          onclick={() => (terminalOpen = true)}
          class="p-2 rounded-xl bg-[var(--bg-chip)] text-[var(--text-secondary)] hover:text-emerald-500 transition-colors active:scale-95"
          title="Terminal HITL"
        >
          <Icon name="terminal" size={18} />
        </button>
      {/if}

      <button
        type="button"
        onclick={handleHeaderNewAction}
        class="p-2 rounded-xl bg-[var(--accent-soft)] text-[var(--accent-fg)] border border-[var(--accent-border)] hover:opacity-90 transition-all active:scale-95"
        title="Nova Aba / Conversa"
      >
        <Icon name="plus" size={18} />
      </button>
    </div>

    <!-- Quick Theme & Palette Popover -->
    {#if themeMenuOpen}
      <button
        type="button"
        aria-label="Fechar seletor de tema"
        onclick={() => (themeMenuOpen = false)}
        class="fixed inset-0 z-30 cursor-default"
      ></button>

      <div
        transition:fly={{ y: -8, duration: 160, easing: cubicOut }}
        class="absolute right-2.5 top-14 w-64 max-w-[calc(100vw-1.25rem)] bg-[var(--bg-elevated)] border border-[var(--border-subtle)] rounded-2xl p-3 z-40 space-y-3 theme-surface"
        style="box-shadow: var(--shadow-popover);"
      >
        <div class="flex items-center justify-between">
          <div class="flex items-center gap-1.5 text-xs font-semibold text-[var(--text-primary)]">
            <Icon name="palette" size={14} class="text-[var(--accent-fg)]" />
            <span>Tema & Cores</span>
          </div>
          <span class="text-[10px] font-mono text-[var(--text-faint)]">
            {themeMode === 'dark' ? 'Escuro' : 'Claro'}
          </span>
        </div>

        <div class="grid grid-cols-2 gap-1.5 p-1 rounded-xl bg-[var(--bg-input)] border border-[var(--border-subtle)]">
          <button
            type="button"
            onclick={() => setTheme('dark', themePalette)}
            class="py-1.5 px-2 rounded-lg text-xs font-semibold flex items-center justify-center gap-1.5 transition-all {themeMode === 'dark' ? 'bg-[var(--bg-elevated)] text-[var(--accent-fg)] shadow-sm border border-[var(--border-subtle)]' : 'text-[var(--text-muted)] hover:text-[var(--text-primary)]'}"
          >
            <Icon name="moon" size={13} />
            <span>Escuro</span>
          </button>
          <button
            type="button"
            onclick={() => setTheme('light', themePalette)}
            class="py-1.5 px-2 rounded-lg text-xs font-semibold flex items-center justify-center gap-1.5 transition-all {themeMode === 'light' ? 'bg-[var(--bg-elevated)] text-[var(--accent-fg)] shadow-sm border border-[var(--border-subtle)]' : 'text-[var(--text-muted)] hover:text-[var(--text-primary)]'}"
          >
            <Icon name="sun" size={13} />
            <span>Claro</span>
          </button>
        </div>

        <div class="space-y-1.5">
          <div class="text-[11px] font-medium text-[var(--text-muted)]">Paleta de destaque</div>
          <div class="grid grid-cols-3 gap-1.5">
            {#each THEME_PALETTES as p}
              <button
                type="button"
                onclick={() => setTheme(themeMode, p.id)}
                class="px-2 py-1.5 rounded-xl border text-[11px] font-medium flex items-center gap-1.5 transition-all active:scale-95 {themePalette === p.id ? 'bg-[var(--accent-soft)] border-[var(--accent-border)] text-[var(--text-primary)]' : 'bg-[var(--bg-input)] border-[var(--border-subtle)] text-[var(--text-muted)] hover:border-[var(--border-strong)]'}"
              >
                <span class="w-3 h-3 rounded-full shrink-0 flex items-center justify-center" style="background-color: {p.swatch};">
                  {#if themePalette === p.id}
                    <span class="w-1.5 h-1.5 rounded-full bg-white"></span>
                  {/if}
                </span>
                <span class="truncate">{p.label}</span>
              </button>
            {/each}
          </div>
        </div>
      </div>
    {/if}
  </header>

  <!-- Tabs & Split Bar (visible when multiple tabs or Split View active) -->
  {#if tabs.length > 1 || splitOpen}
    <div class="bg-[var(--bg-surface)] border-b border-[var(--border-subtle)] px-2.5 sm:px-5 py-1.5 flex items-center justify-between gap-2 shrink-0 select-none theme-surface">
      <div class="flex items-center gap-1.5 overflow-x-auto min-w-0 flex-1">
        {#each tabs as tab, idx (tab.id)}
          {@const isSelected = tab.id === focusedTabId}
          {@const isLeft = splitOpen && tab.id === leftTabId}
          {@const isRight = splitOpen && tab.id === rightTabId}
          <div
            class="inline-flex items-center gap-1.5 rounded-lg border px-2.5 py-1 text-[11px] font-medium shrink-0 transition-all {isSelected ? 'bg-[var(--accent-soft)] border-[var(--accent-border)] text-[var(--accent-fg)]' : 'bg-[var(--bg-input)] border-[var(--border-subtle)] text-[var(--text-muted)] hover:border-[var(--border-strong)]'}"
          >
            <button
              type="button"
              onclick={() => selectTab(tab.id)}
              class="flex items-center gap-1.5 max-w-[140px]"
            >
              {#if tab.isGenerating}
                <AiSpinner size={11} class="text-[var(--accent-fg)]" />
              {/if}
              {#if isLeft}
                <span class="hidden md:inline text-[9px] font-mono uppercase opacity-75">[Esq]</span>
              {:else if isRight}
                <span class="hidden md:inline text-[9px] font-mono uppercase opacity-75">[Dir]</span>
              {/if}
              <span class="truncate">{tab.title || `Sessão ${idx + 1}`}</span>
            </button>
            {#if tabs.length > 1}
              <button
                type="button"
                onclick={(e) => closeTab(e, tab.id)}
                class="text-[var(--text-faint)] hover:text-rose-400 transition-colors"
                title="Fechar aba"
              >
                <Icon name="close" size={11} />
              </button>
            {/if}
          </div>
        {/each}

        {#if tabs.length < 6}
          <button
            type="button"
            onclick={() => createTab(true)}
            class="px-2 py-1 rounded-lg bg-[var(--bg-input)] border border-[var(--border-subtle)] text-[var(--text-muted)] hover:text-[var(--accent-fg)] hover:border-[var(--border-strong)] text-[11px] font-medium flex items-center gap-1 shrink-0 transition-colors"
            title="Abrir nova aba"
          >
            <Icon name="plus" size={12} />
            <span class="hidden sm:inline">Nova Aba</span>
          </button>
        {/if}
      </div>

      {#if splitOpen}
        <div class="hidden md:flex items-center gap-1.5 shrink-0">
          <button
            type="button"
            onclick={() => (compareOpen = !compareOpen)}
            class="px-2.5 py-1 rounded-lg border text-[11px] font-semibold flex items-center gap-1.5 transition-all active:scale-95 {compareOpen ? 'bg-[var(--accent-solid)] border-[var(--accent-solid)] text-[var(--accent-on-solid)]' : 'bg-[var(--bg-input)] border-[var(--border-subtle)] text-[var(--text-secondary)] hover:border-[var(--accent-border)] hover:text-[var(--accent-fg)]'}"
            title="Enviar o mesmo prompt para os 2 modelos simultaneamente"
          >
            <Icon name="compare" size={12} />
            <span>Comparar Modelos</span>
          </button>
        </div>
      {/if}
    </div>
  {/if}

  <!-- Shared Compare Prompt Bar (Desktop Split View) -->
  {#if splitOpen && compareOpen}
    <div
      transition:fly={{ y: -8, duration: 150, easing: cubicOut }}
      class="hidden md:block bg-[var(--bg-surface)] border-b border-[var(--border-subtle)] px-4 py-2 shrink-0 theme-surface"
    >
      <form onsubmit={handleCompareSubmit} class="max-w-3xl mx-auto flex items-center gap-2">
        <Icon name="compare" size={15} class="text-[var(--accent-fg)] shrink-0" />
        <input
          type="text"
          bind:value={comparePrompt}
          placeholder="Comparar modelos: digite um prompt para enviar aos 2 painéis simultaneamente..."
          class="flex-1 bg-[var(--bg-input)] border border-[var(--border-subtle)] rounded-xl px-3 py-1.5 text-xs text-[var(--text-primary)] placeholder-[var(--text-faint)] focus:outline-none focus:border-[var(--accent-fg)]"
        />
        <button
          type="submit"
          disabled={!comparePrompt.trim()}
          class="px-3 py-1.5 rounded-xl bg-[var(--accent-solid)] hover:bg-[var(--accent-hover)] disabled:opacity-40 text-[var(--accent-on-solid)] text-xs font-semibold flex items-center gap-1.5 shrink-0 transition-all active:scale-95"
        >
          <Icon name="send" size={13} />
          <span>Enviar aos 2</span>
        </button>
      </form>
    </div>
  {/if}

  <!-- Workspace (Single View or Desktop Split View) -->
  <div class={splitOpen ? 'flex-1 min-h-0 flex flex-col md:grid md:grid-cols-2 md:gap-3 md:px-3 md:py-2.5' : 'flex-1 min-h-0 flex flex-col'}>
    {#each tabs as tab (tab.id)}
      <ChatPane
        bind:this={paneRefs[tab.id]}
        tabId={tab.id}
        storageKeyPrefix={tab.storageKeyPrefix}
        visibilityMode={visibilityForTab(tab.id)}
        paneOrderClass={orderForTab(tab.id)}
        isSplitView={splitOpen}
        isFocused={tab.id === focusedTabId}
        {models}
        {gatewayMode}
        {hostLabel}
        {features}
        {defaultModel}
        {defaultPermission}
        onOpenShare={openShareModal}
        onRequestDelete={requestDeleteConversation}
        onOpenTerminal={() => (terminalOpen = true)}
        onFocus={() => {
          if (splitOpen) {
            focusedSide = tab.id === rightTabId ? 'right' : 'left';
          }
        }}
        onMetaChange={handlePaneMetaChange}
        onGeneratingChange={handlePaneGeneratingChange}
      />
    {/each}
  </div>
</div>

<Sidebar
  bind:open={sidebarOpen}
  activeConvId={focusedConvId}
  {models}
  watchdogEnabled={features.watchdog !== false}
  telemetryEnabled={features.telemetry === true}
  cockpitEnabled={features.cockpit === true}
  {themeMode}
  {themePalette}
  bind:settings={settings}
  onSelectConversation={(id, title) => {
    paneRefs[focusedTabId]?.loadConversation(id, title);
  }}
  onNewConversation={() => {
    paneRefs[focusedTabId]?.newConversation();
  }}
  onConversationDeleted={handleConversationDeleted}
  onThemeChange={setTheme}
  onSettingsSaved={(s) => {
    defaultPermission = s.default_permission_mode;
    defaultModel = s.default_model;
  }}
/>

<TerminalModal
  bind:open={terminalOpen}
  hostLabel={gatewayMode === 'standalone' ? 'Terminal Local' : 'Remote Terminal'}
/>
<OutboxModal bind:open={outboxOpen} bind:files={outboxFiles} {outboxDir} />
<ShareModal
  bind:open={shareOpen}
  title={shareTitle}
  singleMarkdown={shareSingleMD}
  messages={shareMessages}
  {themeMode}
  {themePalette}
/>

{#if pendingDelete}
  <div
    transition:fade={{ duration: 150 }}
    class="fixed inset-0 bg-[var(--bg-backdrop)] backdrop-blur-sm z-50 flex items-end sm:items-center justify-center p-0 sm:p-4"
  >
    <button
      type="button"
      aria-label="Cancelar exclusão"
      onclick={() => {
        if (!deletingChat) pendingDelete = null;
      }}
      class="absolute inset-0 cursor-default"
    ></button>

    <div
      transition:fly={{ y: 20, duration: 200, easing: cubicOut }}
      class="relative w-full sm:max-w-sm bg-[var(--bg-elevated)] border-t sm:border border-[var(--border-subtle)] rounded-t-2xl sm:rounded-2xl p-4 space-y-3 shadow-2xl theme-surface"
    >
      <div class="flex items-start gap-3">
        <div class="p-2 rounded-xl bg-rose-500/15 border border-rose-500/30 text-rose-400 shrink-0">
          <Icon name="trash" size={18} />
        </div>
        <div class="min-w-0 flex-1 space-y-1">
          <h2 class="text-sm font-bold text-[var(--text-primary)]">Apagar chat?</h2>
          <p class="text-xs text-[var(--text-muted)] leading-relaxed break-words">
            A sessão <span class="font-semibold text-[var(--text-primary)]">"{pendingDelete.title}"</span> e seu histórico serão removidos permanentemente.
          </p>
        </div>
      </div>

      {#if deleteChatError}
        <div class="p-2 rounded-lg bg-rose-500/10 border border-rose-500/30 text-xs text-rose-400">
          {deleteChatError}
        </div>
      {/if}

      <div class="flex items-center justify-end gap-2 pt-1">
        <button
          type="button"
          disabled={deletingChat}
          onclick={() => (pendingDelete = null)}
          class="px-3 py-1.5 rounded-xl bg-[var(--bg-chip)] hover:bg-[var(--bg-chip-hover)] text-xs font-medium text-[var(--text-secondary)] transition-colors"
        >
          Cancelar
        </button>
        <button
          type="button"
          disabled={deletingChat}
          onclick={confirmDeleteChat}
          class="px-3.5 py-1.5 rounded-xl bg-rose-600 hover:bg-rose-500 disabled:opacity-50 text-white text-xs font-semibold flex items-center gap-1.5 transition-all active:scale-95"
        >
          <Icon name="trash" size={13} />
          <span>{deletingChat ? 'Apagando...' : 'Apagar chat'}</span>
        </button>
      </div>
    </div>
  </div>
{/if}
