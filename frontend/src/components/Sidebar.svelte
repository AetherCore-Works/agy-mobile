<script lang="ts">
  import { untrack, type Component } from "svelte";
  import { fade, fly } from "svelte/transition";
  import { cubicOut } from "svelte/easing";
  import Icon from "./Icon.svelte";
  import { Button } from "../lib/components/ui/button/index.js";
  import { Card } from "../lib/components/ui/card/index.js";
  import { Badge } from "../lib/components/ui/badge/index.js";
  import { Input } from "../lib/components/ui/input/index.js";
  import { Tabs, TabsList, TabsTrigger } from "../lib/components/ui/tabs/index.js";
  import { loadOptionalCockpitPanel } from "../lib/optionalCockpit";
  import {
    deleteConversation,
    fetchConversations,
    fetchTelemetry,
    fetchWatchdog,
    updateConversationMeta,
    updateSettings,
    type ConversationSummary,
    type GatewaySettings,
    type ModelInfo,
    type PermissionMode,
    type TelemetryData,
    type WatchdogState,
  } from "../lib/api";
  import {
    THEME_PALETTES,
    type ThemeMode,
    type ThemePalette,
  } from "../lib/theme";

  let {
    open = $bindable(false),
    activeConvId,
    models,
    watchdogEnabled = true,
    telemetryEnabled = false,
    cockpitEnabled = false,
    unreadAlerts = 0,
    themeMode,
    themePalette,
    settings = $bindable<GatewaySettings>(),
    onSelectConversation,
    onNewConversation,
    onConversationDeleted,
    onOpenAutomations,
    onSettingsSaved,
    onThemeChange,
  }: {
    open: boolean;
    activeConvId: string | null;
    models: ModelInfo[];
    watchdogEnabled?: boolean;
    telemetryEnabled?: boolean;
    cockpitEnabled?: boolean;
    unreadAlerts?: number;
    themeMode: ThemeMode;
    themePalette: ThemePalette;
    settings: GatewaySettings;
    onSelectConversation: (id: string, title?: string) => void;
    onNewConversation: () => void;
    onConversationDeleted?: (id: string) => void;
    onOpenAutomations?: () => void;
    onSettingsSaved: (s: GatewaySettings) => void;
    onThemeChange: (mode: ThemeMode, palette: ThemePalette) => void;
  } = $props();

  let CockpitComp = $state<Component<{ active?: boolean }> | null>(null);
  let cockpitLoadAttempted = $state(false);

  let activeTab = $state<"chats" | "settings">("chats");
  let conversations = $state<ConversationSummary[]>([]);
  let availableTags = $state<string[]>([]);
  let searchQuery = $state("");
  let engineFilter = $state<"" | "agy" | "ollama">("");
  let tagFilter = $state("");
  let loading = $state(false);
  let hasMore = $state(false);
  let offset = $state(0);
  let savingSettings = $state(false);
  let settingsSavedMsg = $state("");
  let watchdog = $state<WatchdogState | null>(null);
  let telemetry = $state<TelemetryData | null>(null);

  // Inline session organization editor state
  let editingId = $state<string | null>(null);
  let editTitle = $state("");
  let editTag = $state("");
  let editTagColor = $state("");
  let savingMeta = $state(false);
  let confirmDeleteId = $state<string | null>(null);
  let deletingId = $state<string | null>(null);
  let deleteError = $state("");

  const PAGE_SIZE = 30;

  function resolveTagSwatch(colorId?: string): string {
    return THEME_PALETTES.find((p) => p.id === colorId)?.swatch ?? "#71717a";
  }

  async function load(reset = true) {
    loading = true;
    try {
      const nextOffset = reset ? 0 : offset;
      const data = await fetchConversations(
        searchQuery,
        PAGE_SIZE,
        nextOffset,
        engineFilter,
        tagFilter,
      );
      if (reset) {
        conversations = data.conversations;
      } else {
        conversations = [...conversations, ...data.conversations];
      }
      if (data.tags) {
        availableTags = data.tags;
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

  function setEngineFilter(next: "" | "agy" | "ollama") {
    engineFilter = next;
    load(true);
  }

  function toggleTagFilter(tag: string) {
    tagFilter = tagFilter.toLowerCase() === tag.toLowerCase() ? "" : tag;
    load(true);
  }

  async function togglePin(e: MouseEvent, c: ConversationSummary) {
    e.stopPropagation();
    await updateConversationMeta(c.id, { pinned: !c.pinned });
    await load(true);
  }

  function startEdit(e: MouseEvent, c: ConversationSummary) {
    e.stopPropagation();
    confirmDeleteId = null;
    deleteError = "";
    if (editingId === c.id) {
      editingId = null;
      return;
    }
    editingId = c.id;
    editTitle = c.custom_title || c.title || "";
    editTag = c.tag || "";
    editTagColor = c.tag_color || "";
  }

  async function saveEdit(e: Event, c: ConversationSummary) {
    e.stopPropagation();
    savingMeta = true;
    try {
      await updateConversationMeta(c.id, {
        custom_title: editTitle.trim(),
        tag: editTag.trim(),
        tag_color: editTag.trim() ? editTagColor || "sky" : "",
      });
      editingId = null;
      await load(true);
    } finally {
      savingMeta = false;
    }
  }

  function requestDelete(e: MouseEvent, id: string) {
    e.stopPropagation();
    editingId = null;
    deleteError = "";
    confirmDeleteId = confirmDeleteId === id ? null : id;
  }

  async function confirmDelete(e: MouseEvent, id: string) {
    e.stopPropagation();
    deletingId = id;
    deleteError = "";
    try {
      await deleteConversation(id);
      confirmDeleteId = null;
      conversations = conversations.filter((c) => c.id !== id);
      onConversationDeleted?.(id);
      await load(true);
    } catch (err: any) {
      deleteError = err?.message || "Falha ao apagar sessão";
    } finally {
      deletingId = null;
    }
  }

  async function saveSettingsPatch(patch: Partial<GatewaySettings>) {
    savingSettings = true;
    settingsSavedMsg = "";
    try {
      const updated = await updateSettings(patch);
      settings = updated;
      onSettingsSaved(updated);
      settingsSavedMsg = "Salvo permanentemente";
      setTimeout(() => (settingsSavedMsg = ""), 2000);
    } finally {
      savingSettings = false;
    }
  }

  $effect(() => {
    const isOpen = open;
    const tab = activeTab;
    untrack(() => {
      if (!isOpen) {
        confirmDeleteId = null;
        deleteError = "";
        return;
      }
      if (tab === "chats") {
        load(true);
      } else if (tab === "settings") {
        if (watchdogEnabled) {
          fetchWatchdog()
            .then((w) => (watchdog = w))
            .catch(() => {});
        }
        if (cockpitEnabled && !CockpitComp && !cockpitLoadAttempted) {
          cockpitLoadAttempted = true;
          loadOptionalCockpitPanel()
            .then((comp) => {
              CockpitComp = comp;
            })
            .catch(() => {});
        } else if (telemetryEnabled && !cockpitEnabled) {
          fetchTelemetry()
            .then((t) => (telemetry = t))
            .catch(() => {});
        }
      }
    });
  });

  const permOptions: {
    value: PermissionMode;
    label: string;
    desc: string;
    icon: "shield" | "zap" | "eye";
  }[] = [
    {
      value: "copilot",
      label: "Copiloto",
      desc: "Aprova edições (--mode accept-edits)",
      icon: "shield",
    },
    {
      value: "autonomous",
      label: "Autônomo",
      desc: "Execução total sem bloqueios",
      icon: "zap",
    },
    {
      value: "readonly",
      label: "Leitura",
      desc: "Somente planejamento (--mode plan)",
      icon: "eye",
    },
  ];
</script>

{#if open}
  <!-- Backdrop -->
  <button
    transition:fade={{ duration: 180 }}
    type="button"
    aria-label="Fechar menu"
    onclick={() => (open = false)}
    class="fixed inset-0 bg-[var(--bg-backdrop)] backdrop-blur-sm z-40"
  ></button>

  <!-- Drawer -->
  <aside
    transition:fly={{ x: -320, duration: 220, easing: cubicOut }}
    class="fixed inset-y-0 left-0 w-80 max-w-[88vw] pt-[env(safe-area-inset-top)] pb-[env(safe-area-inset-bottom)] bg-[var(--bg-elevated)] border-r border-[var(--border-subtle)] z-50 flex flex-col shadow-2xl select-none theme-surface"
  >
    <!-- Header -->
    <div
      class="p-3.5 border-b border-[var(--border-subtle)] flex items-center justify-between"
    >
      <div class="flex items-center gap-2">
        <Icon name="clock" size={17} class="text-[var(--accent-fg)]" />
        <h2 class="text-sm font-bold text-[var(--text-primary)]">
          AGY Gateway
        </h2>
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
    <Tabs
      value={activeTab}
      onValueChange={(v) => (activeTab = v === "settings" ? "settings" : "chats")}
      class="border-b border-border bg-muted/60 p-1.5 theme-surface"
    >
      <TabsList class="grid grid-cols-2 w-full bg-muted border-border">
        <TabsTrigger value="chats">
          <Icon name="clock" size={14} />
          <span>Sessões</span>
        </TabsTrigger>
        <TabsTrigger value="settings">
          <Icon name="settings" size={14} />
          <span>Configs</span>
        </TabsTrigger>
      </TabsList>
    </Tabs>

    {#if activeTab === "chats"}
      <div class="p-3 border-b border-border space-y-2">
        <Button
          variant="default"
          onclick={() => {
            onNewConversation();
            open = false;
          }}
          class="w-full"
        >
          <Icon name="plus" size={15} />
          <span>Nova Conversa</span>
        </Button>

        <div class="relative flex items-center">
          <Icon
            name="search"
            size={14}
            class="absolute left-2.5 text-muted-foreground pointer-events-none"
          />
          <Input
            type="text"
            bind:value={searchQuery}
            oninput={handleSearchInput}
            placeholder="Buscar (FTS5)..."
            class="pl-8 pr-2.5 rounded-full"
          />
        </div>

        <!-- Engine Filter Pills -->
        <div
          class="grid grid-cols-3 gap-1 p-0.5 rounded-xl bg-muted border border-border"
        >
          <button
            type="button"
            onclick={() => setEngineFilter("")}
            class="py-1 rounded-lg text-[11px] font-semibold transition-all {engineFilter ===
            ''
              ? 'bg-card text-accent-foreground border border-border shadow-xs'
              : 'text-muted-foreground hover:text-foreground'}"
          >
            Todos
          </button>
          <button
            type="button"
            onclick={() => setEngineFilter("agy")}
            class="py-1 rounded-lg text-[11px] font-semibold transition-all {engineFilter ===
            'agy'
              ? 'bg-card text-accent-foreground border border-border shadow-xs'
              : 'text-muted-foreground hover:text-foreground'}"
          >
            AGY
          </button>
          <button
            type="button"
            onclick={() => setEngineFilter("ollama")}
            class="py-1 rounded-lg text-[11px] font-semibold transition-all {engineFilter ===
            'ollama'
              ? 'bg-card text-accent-foreground border border-border shadow-xs'
              : 'text-muted-foreground hover:text-foreground'}"
          >
            Local
          </button>
        </div>

        <!-- Project Tag Filter Pills -->
        {#if availableTags.length > 0}
          <div class="flex items-center gap-1 overflow-x-auto pb-0.5">
            {#each availableTags as t}
              <button
                type="button"
                onclick={() => toggleTagFilter(t)}
                class="px-2.5 py-0.5 rounded-full border text-[10px] font-medium shrink-0 flex items-center gap-1 transition-all {tagFilter.toLowerCase() ===
                t.toLowerCase()
                  ? 'bg-accent border-[var(--accent-border)] text-accent-foreground'
                  : 'bg-muted border-border text-muted-foreground hover:border-input'}"
              >
                <Icon name="tag" size={10} />
                <span>{t}</span>
              </button>
            {/each}
          </div>
        {/if}
      </div>

      <div class="flex-1 overflow-y-auto p-2 space-y-1.5">
        {#if loading && conversations.length === 0}
          <div class="text-center py-8 text-xs text-muted-foreground">
            Sincronizando sessões...
          </div>
        {:else if conversations.length === 0}
          <div class="text-center py-8 text-xs text-muted-foreground">
            Nenhuma sessão encontrada.
          </div>
        {:else}
          {#each conversations as c}
            <Card
              class="rounded-xl transition-all {activeConvId === c.id
                ? 'bg-accent/70 border-[var(--accent-border)]'
                : 'bg-muted/70 hover:border-input'}"
            >
              <div
                role="button"
                tabindex="0"
                onclick={() => {
                  onSelectConversation(c.id, c.title);
                  open = false;
                }}
                onkeydown={(e) => {
                  if (e.key === "Enter") {
                    onSelectConversation(c.id, c.title);
                    open = false;
                  }
                }}
                class="w-full text-left p-2.5 cursor-pointer flex items-start justify-between gap-2"
              >
                <div class="min-w-0 flex-1">
                  <div class="flex items-center gap-1.5">
                    {#if c.pinned}
                      <Icon
                        name="pin"
                        size={12}
                        class="text-accent-foreground shrink-0"
                      />
                    {/if}
                    <span
                      class="text-xs font-medium text-foreground truncate"
                      >{c.title || "Sem título"}</span
                    >
                  </div>
                  <div
                    class="flex flex-wrap items-center gap-1.5 mt-1 text-[10px] text-muted-foreground"
                  >
                    <Badge
                      variant={c.engine === "ollama" ? "indigo" : "slate"}
                      size="xs"
                      class="font-mono"
                    >
                      {c.engine === "ollama" ? "Local" : "AGY"}
                    </Badge>
                    {#if c.tag}
                      <Badge variant="outline" size="xs" class="gap-1">
                        <span
                          class="w-1.5 h-1.5 rounded-full"
                          style="background-color: {resolveTagSwatch(
                            c.tag_color,
                          )};"
                        ></span>
                        <span class="truncate max-w-[80px]">{c.tag}</span>
                      </Badge>
                    {/if}
                    <span
                      >{c.updated_at
                        ? new Date(c.updated_at).toLocaleDateString("pt-BR", {
                            day: "2-digit",
                            month: "2-digit",
                            hour: "2-digit",
                            minute: "2-digit",
                          })
                        : ""}</span
                    >
                  </div>
                </div>

                <div class="flex items-center gap-0.5 shrink-0">
                  <button
                    type="button"
                    onclick={(e) => togglePin(e, c)}
                    class="p-1 rounded transition-colors {c.pinned
                      ? 'text-[var(--accent-fg)]'
                      : 'text-[var(--text-faint)] hover:text-[var(--accent-fg)]'}"
                    title={c.pinned ? "Desafixar" : "Fixar no topo"}
                  >
                    <Icon name="pin" size={13} />
                  </button>
                  <button
                    type="button"
                    onclick={(e) => startEdit(e, c)}
                    class="p-1 rounded transition-colors {editingId === c.id
                      ? 'text-[var(--accent-fg)]'
                      : 'text-[var(--text-faint)] hover:text-[var(--accent-fg)]'}"
                    title="Renomear / Etiqueta"
                  >
                    <Icon name="edit" size={13} />
                  </button>
                  {#if c.engine === "ollama" || c.engine === "agy"}
                    <button
                      type="button"
                      onclick={(e) => requestDelete(e, c.id)}
                      class="p-1 rounded transition-colors {confirmDeleteId ===
                      c.id
                        ? 'text-rose-400 bg-rose-500/15'
                        : 'text-[var(--text-faint)] hover:text-rose-400 hover:bg-rose-500/10'}"
                      title="Apagar chat"
                    >
                      <Icon name="trash" size={13} />
                    </button>
                  {/if}
                </div>
              </div>

              {#if confirmDeleteId === c.id}
                <div
                  class="px-2.5 pb-2.5 pt-1.5 border-t border-rose-500/30 bg-rose-500/5 space-y-2 rounded-b-xl"
                >
                  <div
                    class="text-[11px] text-[var(--text-secondary)] leading-snug"
                  >
                    Apagar este chat permanentemente?
                  </div>
                  {#if deleteError}
                    <div class="text-[11px] text-rose-400">{deleteError}</div>
                  {/if}
                  <div class="flex items-center justify-end gap-1.5">
                    <button
                      type="button"
                      disabled={deletingId === c.id}
                      onclick={(e) => {
                        e.stopPropagation();
                        confirmDeleteId = null;
                        deleteError = "";
                      }}
                      class="px-2 py-1 rounded-md text-[11px] text-[var(--text-muted)] hover:text-[var(--text-primary)]"
                    >
                      Cancelar
                    </button>
                    <button
                      type="button"
                      disabled={deletingId === c.id}
                      onclick={(e) => confirmDelete(e, c.id)}
                      class="px-2.5 py-1 rounded-md bg-rose-600 hover:bg-rose-500 disabled:opacity-50 text-white text-[11px] font-semibold transition-colors"
                    >
                      {deletingId === c.id ? "Apagando..." : "Apagar"}
                    </button>
                  </div>
                </div>
              {/if}

              {#if editingId === c.id}
                <form
                  onsubmit={(e) => {
                    e.preventDefault();
                    saveEdit(e, c);
                  }}
                  class="px-2.5 pb-2.5 pt-1.5 border-t border-[var(--border-subtle)] space-y-2"
                >
                  <input
                    type="text"
                    bind:value={editTitle}
                    placeholder="Nome personalizado da sessão..."
                    class="w-full bg-[var(--bg-elevated)] border border-[var(--border-subtle)] rounded-lg px-2 py-1 text-xs text-[var(--text-primary)] focus:outline-none focus:border-[var(--accent-fg)]"
                  />
                  <div class="flex items-center gap-1.5">
                    <input
                      type="text"
                      bind:value={editTag}
                      placeholder="Etiqueta (ex: Backend)"
                      maxlength={32}
                      class="flex-1 min-w-0 bg-[var(--bg-elevated)] border border-[var(--border-subtle)] rounded-lg px-2 py-1 text-[11px] text-[var(--text-primary)] focus:outline-none focus:border-[var(--accent-fg)]"
                    />
                    <div class="flex items-center gap-1 shrink-0">
                      {#each THEME_PALETTES as p}
                        <button
                          type="button"
                          onclick={() => (editTagColor = p.id)}
                          aria-label={p.label}
                          class="w-4 h-4 rounded-full flex items-center justify-center transition-transform {editTagColor ===
                          p.id
                            ? 'scale-110 ring-2 ring-[var(--text-primary)]'
                            : 'opacity-75 hover:opacity-100'}"
                          style="background-color: {p.swatch};"
                        ></button>
                      {/each}
                    </div>
                  </div>
                  <div class="flex items-center justify-end gap-1.5">
                    <button
                      type="button"
                      onclick={() => (editingId = null)}
                      class="px-2 py-1 rounded-md text-[11px] text-[var(--text-muted)] hover:text-[var(--text-primary)]"
                    >
                      Cancelar
                    </button>
                    <button
                      type="submit"
                      disabled={savingMeta}
                      class="px-2.5 py-1 rounded-md bg-[var(--accent-solid)] text-[var(--accent-on-solid)] text-[11px] font-semibold"
                    >
                      {savingMeta ? "Salvando..." : "Salvar"}
                    </button>
                  </div>
                </form>
              {/if}
            </Card>
          {/each}

          {#if hasMore}
            <button
              type="button"
              onclick={() => load(false)}
              disabled={loading}
              class="w-full py-2 rounded-lg bg-[var(--bg-chip)] hover:bg-[var(--bg-chip-hover)] text-xs text-[var(--text-secondary)] font-medium transition-colors"
            >
              {loading ? "Carregando..." : "Carregar mais"}
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
            <span class="text-xs font-semibold text-[var(--text-secondary)]"
              >Aparência</span
            >
            <span
              class="text-[10px] font-mono text-[var(--text-faint)] uppercase"
              >{themeMode === "dark" ? "Escuro" : "Claro"}</span
            >
          </div>

          <div class="grid grid-cols-2 gap-2">
            <button
              type="button"
              onclick={() => onThemeChange("dark", themePalette)}
              class="p-2 rounded-xl border text-xs font-semibold flex items-center justify-center gap-1.5 transition-all active:scale-[0.98] {themeMode ===
              'dark'
                ? 'bg-[var(--accent-soft)] border-[var(--accent-border)] text-[var(--accent-fg)]'
                : 'bg-[var(--bg-input)] border-[var(--border-subtle)] text-[var(--text-muted)] hover:border-[var(--border-strong)]'}"
            >
              <Icon name="moon" size={14} />
              <span>Escuro</span>
            </button>
            <button
              type="button"
              onclick={() => onThemeChange("light", themePalette)}
              class="p-2 rounded-xl border text-xs font-semibold flex items-center justify-center gap-1.5 transition-all active:scale-[0.98] {themeMode ===
              'light'
                ? 'bg-[var(--accent-soft)] border-[var(--accent-border)] text-[var(--accent-fg)]'
                : 'bg-[var(--bg-input)] border-[var(--border-subtle)] text-[var(--text-muted)] hover:border-[var(--border-strong)]'}"
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
                class="px-2 py-1.5 rounded-xl border text-[11px] font-medium flex items-center gap-1.5 transition-all active:scale-[0.97] {themePalette ===
                p.id
                  ? 'bg-[var(--accent-soft)] border-[var(--accent-border)] text-[var(--text-primary)]'
                  : 'bg-[var(--bg-input)] border-[var(--border-subtle)] text-[var(--text-muted)] hover:border-[var(--border-strong)]'}"
              >
                <span
                  class="w-3 h-3 rounded-full shrink-0 shadow-sm"
                  style="background-color: {p.swatch};"
                ></span>
                <span class="truncate">{p.label}</span>
              </button>
            {/each}
          </div>
        </div>

        <div>
          <div
            class="block text-xs font-semibold text-[var(--text-secondary)] mb-2"
          >
            Modo de Permissão Padrão
          </div>
          <div class="space-y-2">
            {#each permOptions as opt}
              <button
                type="button"
                onclick={() =>
                  saveSettingsPatch({ default_permission_mode: opt.value })}
                class="w-full text-left p-2.5 rounded-xl border transition-all flex items-start gap-2.5 active:scale-[0.99] {settings.default_permission_mode ===
                opt.value
                  ? 'bg-[var(--accent-soft)] border-[var(--accent-border)] text-[var(--text-primary)]'
                  : 'bg-[var(--bg-input)] border-[var(--border-subtle)] text-[var(--text-muted)] hover:border-[var(--border-strong)]'}"
              >
                <Icon
                  name={opt.icon}
                  size={16}
                  class={settings.default_permission_mode === opt.value
                    ? "text-[var(--accent-fg)] mt-0.5"
                    : "text-[var(--text-faint)] mt-0.5"}
                />
                <div class="min-w-0 flex-1">
                  <div
                    class="text-xs font-semibold flex items-center justify-between"
                  >
                    <span>{opt.label}</span>
                    {#if settings.default_permission_mode === opt.value}
                      <Icon
                        name="check"
                        size={14}
                        class="text-[var(--accent-fg)]"
                      />
                    {/if}
                  </div>
                  <div class="text-[11px] text-[var(--text-faint)] mt-0.5">
                    {opt.desc}
                  </div>
                </div>
              </button>
            {/each}
          </div>
        </div>

        <div>
          <label
            for="cfg-default-model"
            class="block text-xs font-semibold text-[var(--text-secondary)] mb-1.5"
            >Modelo Padrão</label
          >
          <select
            id="cfg-default-model"
            value={settings.default_model}
            onchange={(e) =>
              saveSettingsPatch({
                default_model: (e.target as HTMLSelectElement).value,
              })}
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

        <div
          class="pt-2 border-t border-[var(--border-subtle)] text-[11px] text-[var(--text-faint)] space-y-1.5"
        >
          <p>
            As configurações ficam salvas no SQLite local do Gateway e
            sincronizam automaticamente entre sessões.
          </p>
          {#if savingSettings}
            <p class="text-[var(--accent-fg)]">Salvando...</p>
          {/if}
        </div>

        {#if onOpenAutomations}
          <button
            type="button"
            onclick={() => {
              open = false;
              onOpenAutomations?.();
            }}
            class="w-full p-2.5 rounded-xl bg-[var(--accent-soft)] border border-[var(--accent-border)] text-[var(--accent-fg)] text-xs font-semibold flex items-center justify-between gap-2 transition-all active:scale-[0.98]"
          >
            <div class="flex items-center gap-2">
              <Icon name="bell" size={15} />
              <span>Automações Cron & Alertas</span>
            </div>
            {#if unreadAlerts > 0}
              <span
                class="px-1.5 py-0.5 rounded-full bg-rose-600 text-white text-[10px] font-bold"
              >
                {unreadAlerts}
              </span>
            {:else}
              <Icon name="chevron-down" size={14} class="-rotate-90" />
            {/if}
          </button>
        {/if}

        {#if watchdogEnabled && watchdog}
          <div
            class="p-3 rounded-xl bg-[var(--bg-input)] border border-[var(--border-subtle)] space-y-1.5 theme-surface"
          >
            <div
              class="flex items-center justify-between text-xs font-semibold text-[var(--text-primary)]"
            >
              <span>Watchdog</span>
              <span
                class="px-1.5 py-0.5 rounded text-[10px] {watchdog.online
                  ? 'bg-emerald-500/15 text-emerald-500 border border-emerald-500/35'
                  : 'bg-rose-500/15 text-rose-400 border border-rose-500/35'}"
              >
                {watchdog.online ? "Online" : "Offline"}
              </span>
            </div>
            <div
              class="text-[11px] text-[var(--text-faint)] flex justify-between"
            >
              <span>Verificações: {watchdog.uptime_checks}</span>
              <span>Quedas: {watchdog.downtime_events}</span>
            </div>
            {#if watchdog.last_seen_online}
              <div class="text-[10px] text-[var(--text-faint)]">
                Visto por último: {new Date(
                  watchdog.last_seen_online,
                ).toLocaleTimeString("pt-BR")}
              </div>
            {/if}
          </div>
        {/if}

        {#if cockpitEnabled}
          {#if CockpitComp}
            <CockpitComp active={open && activeTab === "settings"} />
          {:else if cockpitLoadAttempted}
            <div
              class="p-3 rounded-xl bg-[var(--bg-input)] border border-[var(--border-subtle)] text-[11px] text-[var(--text-faint)]"
            >
              Pacote opcional Cockpit habilitado, mas módulo UI não encontrado.
            </div>
          {:else}
            <div
              class="p-3 rounded-xl bg-[var(--bg-input)] border border-[var(--border-subtle)] text-[11px] text-[var(--text-faint)]"
            >
              Carregando painel Cockpit...
            </div>
          {/if}
        {:else if telemetryEnabled && telemetry && telemetry.enabled}
          <div
            class="p-3 rounded-xl bg-[var(--bg-input)] border border-[var(--border-subtle)] space-y-1.5 theme-surface"
          >
            <div
              class="flex items-center justify-between text-xs font-semibold text-[var(--text-primary)]"
            >
              <span>Telemetria de Hardware</span>
              <Icon name="cpu" size={14} class="text-[var(--accent-fg)]" />
            </div>
            {#if telemetry.gpu}
              <div
                class="text-[11px] text-[var(--text-secondary)] flex justify-between"
              >
                <span class="truncate pr-2">{telemetry.gpu.name}</span>
                <span class="font-mono text-[var(--accent-fg)] shrink-0"
                  >{telemetry.gpu.temp_c}°C</span
                >
              </div>
              <div
                class="text-[11px] text-[var(--text-faint)] flex justify-between"
              >
                <span>VRAM</span>
                <span class="font-mono"
                  >{telemetry.gpu.vram_used_mb} / {telemetry.gpu.vram_total_mb} MB</span
                >
              </div>
            {/if}
            {#if telemetry.ram}
              <div
                class="text-[11px] text-[var(--text-faint)] flex justify-between"
              >
                <span>RAM</span>
                <span class="font-mono"
                  >{Math.round(telemetry.ram.used_mb / 1024)} / {Math.round(
                    telemetry.ram.total_mb / 1024,
                  )} GB</span
                >
              </div>
            {/if}
            {#if telemetry.cpu_temp !== null}
              <div
                class="text-[11px] text-[var(--text-faint)] flex justify-between"
              >
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
