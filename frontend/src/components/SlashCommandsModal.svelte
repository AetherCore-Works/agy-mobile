<script lang="ts">
  import { untrack } from "svelte";
  import { fade, fly } from "svelte/transition";
  import { cubicOut } from "svelte/easing";
  import Icon from "./Icon.svelte";
  import { Button } from "../lib/components/ui/button/index.js";
  import { Card } from "../lib/components/ui/card/index.js";
  import { Badge } from "../lib/components/ui/badge/index.js";
  import { Input } from "../lib/components/ui/input/index.js";
  import { Textarea } from "../lib/components/ui/textarea/index.js";
  import { Kbd } from "../lib/components/ui/kbd/index.js";
  import {
    createSlashCommand,
    deleteSlashCommand,
    fetchSlashCommands,
    resetSlashCommands,
    updateSlashCommand,
    type ModelInfo,
    type SlashCommandItem,
  } from "../lib/api";

  const RESERVED_BUILTINS = [
    { cmd: "/usage", desc: "Cota restante de tokens/modelos" },
    { cmd: "/compact", desc: "Compactar histórico da sessão" },
    { cmd: "/diff", desc: "Exibir git status e diff atual" },
    { cmd: "/mem", desc: "Consultar memória persistente (ai-memory)" },
    { cmd: "/plan", desc: "Ativar modo Planejamento (Leitura)" },
    { cmd: "/clear", desc: "Limpar e iniciar nova conversa" },
    { cmd: "/status", desc: "Status do Watchdog & recursos" },
  ];

  let {
    open = $bindable(false),
    commands = $bindable<SlashCommandItem[]>([]),
    models = [],
  }: {
    open: boolean;
    commands: SlashCommandItem[];
    models: ModelInfo[];
  } = $props();

  let loading = $state(false);
  let saving = $state(false);
  let errorMsg = $state("");
  let feedback = $state("");
  let filterText = $state("");

  let showForm = $state(false);
  let editingId = $state<string | null>(null);
  let formCommand = $state("");
  let formDescription = $state("");
  let formPrompt = $state("");
  let formModel = $state("");
  let formAutoSubmit = $state(true);
  let confirmDeleteId = $state<string | null>(null);
  let promptEl = $state<HTMLTextAreaElement | undefined>(undefined);

  let filteredCommands = $derived.by(() => {
    const q = filterText.trim().toLowerCase();
    if (!q) return commands;
    return commands.filter(
      (c) =>
        c.command.toLowerCase().includes(q) ||
        c.description.toLowerCase().includes(q) ||
        c.prompt_template.toLowerCase().includes(q),
    );
  });

  $effect(() => {
    if (open) {
      untrack(() => refreshList());
    }
  });

  async function refreshList() {
    loading = true;
    errorMsg = "";
    try {
      commands = await fetchSlashCommands();
    } catch (err: any) {
      errorMsg = err?.message || "Erro ao carregar comandos.";
    } finally {
      loading = false;
    }
  }

  function startCreate() {
    editingId = null;
    formCommand = "";
    formDescription = "";
    formPrompt = "";
    formModel = "";
    formAutoSubmit = true;
    errorMsg = "";
    feedback = "";
    confirmDeleteId = null;
    showForm = true;
  }

  function startEdit(item: SlashCommandItem) {
    editingId = item.id;
    formCommand = item.command;
    formDescription = item.description;
    formPrompt = item.prompt_template;
    formModel = item.model || "";
    formAutoSubmit = item.auto_submit;
    errorMsg = "";
    feedback = "";
    confirmDeleteId = null;
    showForm = true;
  }

  function insertPlaceholder() {
    const token = "{{input}}";
    if (!promptEl) {
      formPrompt = formPrompt ? `${formPrompt} ${token}` : token;
      return;
    }
    const start = promptEl.selectionStart ?? formPrompt.length;
    const end = promptEl.selectionEnd ?? formPrompt.length;
    const before = formPrompt.slice(0, start);
    const after = formPrompt.slice(end);
    const sepBefore = before && !before.endsWith(" ") && !before.endsWith("\n") ? " " : "";
    formPrompt = `${before}${sepBefore}${token}${after}`;
    requestAnimationFrame(() => {
      promptEl?.focus();
    });
  }

  async function handleSubmit(e: Event) {
    e.preventDefault();
    if (saving) return;
    saving = true;
    errorMsg = "";
    feedback = "";
    try {
      const payload = {
        command: formCommand.trim(),
        description: formDescription.trim(),
        prompt_template: formPrompt.trim(),
        model: formModel.trim(),
        auto_submit: formAutoSubmit,
      };
      if (editingId) {
        const updated = await updateSlashCommand(editingId, payload);
        commands = commands.map((c) => (c.id === updated.id ? updated : c));
        feedback = `Comando ${updated.command} atualizado.`;
      } else {
        const created = await createSlashCommand(payload);
        commands = [...commands, created].sort(
          (a, b) => a.sort_order - b.sort_order || a.command.localeCompare(b.command),
        );
        feedback = `Comando ${created.command} criado.`;
      }
      showForm = false;
      editingId = null;
    } catch (err: any) {
      errorMsg = err?.message || "Erro ao salvar comando.";
    } finally {
      saving = false;
    }
  }

  async function handleDelete(id: string) {
    errorMsg = "";
    feedback = "";
    try {
      await deleteSlashCommand(id);
      commands = commands.filter((c) => c.id !== id);
      confirmDeleteId = null;
      feedback = "Comando removido.";
    } catch (err: any) {
      errorMsg = err?.message || "Erro ao remover comando.";
    }
  }

  async function handleResetDefaults() {
    errorMsg = "";
    feedback = "";
    loading = true;
    try {
      commands = await resetSlashCommands();
      feedback = "Atalhos padrão restaurados (comandos existentes preservados).";
    } catch (err: any) {
      errorMsg = err?.message || "Erro ao restaurar padrões.";
    } finally {
      loading = false;
    }
  }

  function modelLabel(modelId: string): string {
    if (!modelId) return "";
    const found = models.find((m) => m.id === modelId);
    return found ? found.name : modelId;
  }
</script>

{#if open}
  <div
    transition:fade={{ duration: 150 }}
    class="fixed inset-0 bg-[var(--bg-backdrop)] backdrop-blur-sm z-50 flex items-end sm:items-center justify-center p-0 sm:p-4"
  >
    <button
      type="button"
      aria-label="Fechar Comandos Slash"
      onclick={() => (open = false)}
      class="absolute inset-0 cursor-default"
    ></button>

    <div
      transition:fly={{ y: 20, duration: 200, easing: cubicOut }}
      class="relative w-full sm:max-w-2xl max-h-[90dvh] flex flex-col"
    >
      <Card class="rounded-t-2xl sm:rounded-2xl flex flex-col max-h-[90dvh] shadow-2xl overflow-hidden">
        <!-- Header -->
        <div class="p-3.5 border-b border-border flex items-center justify-between gap-2 shrink-0">
          <div class="flex items-center gap-2 min-w-0">
            <div class="p-1.5 rounded-lg bg-accent/70 border border-[var(--accent-border)] text-accent-foreground shrink-0">
              <Icon name="terminal" size={16} />
            </div>
            <div class="min-w-0">
              <div class="flex items-center gap-2">
                <h2 class="text-sm font-bold text-foreground truncate">
                  Comandos Slash (/)
                </h2>
                <Badge variant="accent" size="xs" class="font-mono">
                  {commands.length}
                </Badge>
              </div>
              <p class="text-[11px] text-muted-foreground truncate">
                Atalhos persistidos em SQLite para Skills, MCPs e prompts no celular
              </p>
            </div>
          </div>

          <div class="flex items-center gap-1 shrink-0">
            {#if !showForm}
              <Button variant="default" size="xs" onclick={startCreate}>
                <Icon name="plus" size={13} />
                <span>Novo</span>
              </Button>
            {/if}
            <Button
              variant="ghost"
              size="icon-sm"
              onclick={() => (open = false)}
              aria-label="Fechar"
            >
              <Icon name="close" size={17} />
            </Button>
          </div>
        </div>

        <!-- Body -->
        <div class="flex-1 overflow-y-auto p-3.5 space-y-3">
          {#if errorMsg}
            <div class="p-2.5 rounded-xl bg-rose-500/10 border border-rose-500/35 text-xs text-rose-400">
              {errorMsg}
            </div>
          {/if}

          {#if feedback}
            <div class="p-2.5 rounded-xl bg-emerald-500/15 border border-emerald-500/35 text-xs text-emerald-500 flex items-center gap-1.5">
              <Icon name="check" size={14} />
              <span>{feedback}</span>
            </div>
          {/if}

          {#if showForm}
            <Card variant="soft" class="p-3.5 space-y-3">
              <div class="flex items-center justify-between gap-2">
                <span class="text-xs font-semibold text-foreground">
                  {editingId ? "Editar comando slash" : "Novo comando slash"}
                </span>
                <Button
                  variant="ghost"
                  size="xs"
                  onclick={() => {
                    showForm = false;
                    editingId = null;
                    errorMsg = "";
                  }}
                >
                  Cancelar
                </Button>
              </div>

              <form onsubmit={handleSubmit} class="space-y-3">
                <div class="grid grid-cols-1 sm:grid-cols-2 gap-2.5">
                  <div class="space-y-1">
                    <label
                      for="slash-cmd-input"
                      class="block text-[11px] font-medium text-secondary-foreground"
                    >
                      Atalho (ex: /publish)
                    </label>
                    <Input
                      id="slash-cmd-input"
                      bind:value={formCommand}
                      placeholder="/meu_comando"
                      class="font-mono text-xs"
                      required
                    />
                  </div>
                  <div class="space-y-1">
                    <label
                      for="slash-desc-input"
                      class="block text-[11px] font-medium text-secondary-foreground"
                    >
                      Descrição curta
                    </label>
                    <Input
                      id="slash-desc-input"
                      bind:value={formDescription}
                      placeholder="Ex: Analisar ledger ou rodar skill"
                      class="text-xs"
                    />
                  </div>
                </div>

                <div class="space-y-1">
                  <div class="flex items-center justify-between gap-2">
                    <label
                      for="slash-prompt-input"
                      class="text-[11px] font-medium text-secondary-foreground"
                    >
                      Template do Prompt
                    </label>
                    <button
                      type="button"
                      onclick={insertPlaceholder}
                      class="px-2 py-0.5 rounded-md bg-accent/70 hover:bg-accent border border-[var(--accent-border)] text-[10px] font-mono text-accent-foreground transition-colors"
                    >
                      + Inserir {"{{input}}"}
                    </button>
                  </div>
                  <Textarea
                    id="slash-prompt-input"
                    bind:ref={promptEl}
                    bind:value={formPrompt}
                    rows={3}
                    placeholder={'Ex: Use a skill X para executar: {{input}}'}
                    class="text-xs leading-relaxed"
                    required
                  />
                  <p class="text-[10px] text-muted-foreground">
                    Use <code class="font-mono">{"{{input}}"}</code> onde deseja inserir o texto digitado após o comando.
                  </p>
                </div>

                <div class="grid grid-cols-1 sm:grid-cols-2 gap-2.5">
                  <div class="space-y-1">
                    <label
                      for="slash-model-select"
                      class="block text-[11px] font-medium text-secondary-foreground"
                    >
                      Modelo específico (opcional)
                    </label>
                    <select
                      id="slash-model-select"
                      bind:value={formModel}
                      class="w-full h-9 rounded-md border border-input bg-muted px-2.5 text-xs text-foreground focus:outline-none focus:border-ring"
                    >
                      <option value="">Usar modelo ativo da aba</option>
                      {#each models as m}
                        <option value={m.id}>{m.name} ({m.badge})</option>
                      {/each}
                    </select>
                  </div>

                  <div class="space-y-1">
                    <span class="block text-[11px] font-medium text-secondary-foreground">
                      Ao tocar no menu /
                    </span>
                    <div class="grid grid-cols-2 gap-1.5">
                      <button
                        type="button"
                        onclick={() => (formAutoSubmit = true)}
                        class="h-9 px-2 rounded-md border text-[11px] font-medium transition-colors {formAutoSubmit
                          ? 'bg-accent border-[var(--accent-border)] text-accent-foreground'
                          : 'bg-muted/60 border-border text-muted-foreground hover:text-foreground'}"
                      >
                        Enviar direto
                      </button>
                      <button
                        type="button"
                        onclick={() => (formAutoSubmit = false)}
                        class="h-9 px-2 rounded-md border text-[11px] font-medium transition-colors {!formAutoSubmit
                          ? 'bg-accent border-[var(--accent-border)] text-accent-foreground'
                          : 'bg-muted/60 border-border text-muted-foreground hover:text-foreground'}"
                      >
                        Preencher /cmd
                      </button>
                    </div>
                  </div>
                </div>

                <div class="flex items-center justify-end gap-2 pt-1">
                  <Button
                    type="submit"
                    variant="default"
                    size="sm"
                    disabled={saving}
                  >
                    <Icon name="check" size={14} />
                    <span>{saving ? "Salvando..." : editingId ? "Salvar alterações" : "Criar comando"}</span>
                  </Button>
                </div>
              </form>
            </Card>
          {:else}
            <div class="flex items-center gap-2">
              <div class="relative flex-1">
                <Input
                  bind:value={filterText}
                  placeholder="Filtrar comandos (/publish, status, review)..."
                  class="text-xs pl-8"
                />
                <Icon
                  name="search"
                  size={13}
                  class="absolute left-2.5 top-1/2 -translate-y-1/2 text-muted-foreground pointer-events-none"
                />
              </div>
              <Button
                variant="outline"
                size="sm"
                onclick={handleResetDefaults}
                disabled={loading}
                title="Recriar atalhos padrão ausentes"
              >
                <Icon name="refresh" size={13} class={loading ? "animate-spin" : ""} />
                <span class="hidden sm:inline">Restaurar padrões</span>
              </Button>
            </div>

            {#if loading && commands.length === 0}
              <div class="py-8 text-center text-xs text-muted-foreground">
                Carregando comandos...
              </div>
            {:else if filteredCommands.length === 0}
              <Card variant="soft" class="p-6 text-center space-y-2">
                <p class="text-xs text-muted-foreground">
                  Nenhum comando personalizado encontrado.
                </p>
                <Button variant="outline" size="xs" onclick={startCreate}>
                  <Icon name="plus" size={12} />
                  <span>Criar primeiro comando</span>
                </Button>
              </Card>
            {:else}
              <div class="space-y-2">
                {#each filteredCommands as item (item.id)}
                  <Card variant="soft" class="p-3 space-y-2">
                    <div class="flex items-start justify-between gap-2">
                      <div class="min-w-0 flex-1 space-y-1">
                        <div class="flex flex-wrap items-center gap-1.5">
                          <Kbd class="px-2 py-0.5 text-xs font-semibold text-accent-foreground border-[var(--accent-border)] bg-accent/60">
                            {item.command}
                          </Kbd>
                          <Badge variant={item.auto_submit ? "emerald" : "sky"} size="xs">
                            {item.auto_submit ? "Envio direto" : "Preencher"}
                          </Badge>
                          {#if item.model}
                            <Badge variant="outline" size="xs" class="font-mono max-w-[160px] truncate">
                              {modelLabel(item.model)}
                            </Badge>
                          {/if}
                        </div>
                        {#if item.description}
                          <div class="text-xs font-medium text-foreground break-words">
                            {item.description}
                          </div>
                        {/if}
                        <p class="text-[11px] text-muted-foreground font-mono break-words line-clamp-2">
                          {item.prompt_template}
                        </p>
                      </div>

                      <div class="flex items-center gap-1 shrink-0">
                        <Button
                          variant="ghost"
                          size="icon-xs"
                          onclick={() => startEdit(item)}
                          title="Editar comando"
                        >
                          <Icon name="edit" size={13} />
                        </Button>
                        <Button
                          variant="ghost"
                          size="icon-xs"
                          onclick={() =>
                            (confirmDeleteId =
                              confirmDeleteId === item.id ? null : item.id)}
                          class={confirmDeleteId === item.id
                            ? "text-rose-400 bg-rose-500/15"
                            : "text-muted-foreground hover:text-rose-400"}
                          title="Apagar comando"
                        >
                          <Icon name="trash" size={13} />
                        </Button>
                      </div>
                    </div>

                    {#if confirmDeleteId === item.id}
                      <div class="pt-2 border-t border-rose-500/30 flex items-center justify-between gap-2 text-[11px]">
                        <span class="text-muted-foreground">
                          Remover <code class="font-mono text-foreground">{item.command}</code>?
                        </span>
                        <div class="flex items-center gap-1.5">
                          <Button
                            variant="outline"
                            size="xs"
                            onclick={() => (confirmDeleteId = null)}
                          >
                            Cancelar
                          </Button>
                          <Button
                            variant="destructiveSolid"
                            size="xs"
                            onclick={() => handleDelete(item.id)}
                          >
                            Remover
                          </Button>
                        </div>
                      </div>
                    {/if}
                  </Card>
                {/each}
              </div>
            {/if}

            <!-- Built-in Reserved Commands Reference -->
            <div class="pt-2 border-t border-border/70 space-y-1.5">
              <div class="text-[11px] font-semibold text-muted-foreground">
                Comandos nativos do Gateway (sempre ativos)
              </div>
              <div class="flex flex-wrap gap-1.5">
                {#each RESERVED_BUILTINS as b}
                  <div
                    class="inline-flex items-center gap-1.5 px-2 py-1 rounded-lg bg-muted/60 border border-border/70 text-[10px]"
                    title={b.desc}
                  >
                    <code class="font-mono font-semibold text-foreground">{b.cmd}</code>
                    <span class="text-muted-foreground hidden sm:inline">· {b.desc}</span>
                  </div>
                {/each}
              </div>
            </div>
          {/if}
        </div>
      </Card>
    </div>
  </div>
{/if}
