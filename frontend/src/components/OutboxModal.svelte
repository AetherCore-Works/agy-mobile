<script lang="ts">
  import { fade, fly } from 'svelte/transition';
  import { cubicOut } from 'svelte/easing';
  import Icon from './Icon.svelte';
  import { deleteOutboxFile, fetchOutbox, type OutboxFile } from '../lib/api';

  let {
    open = $bindable(false),
    files = $bindable<OutboxFile[]>([]),
    outboxDir = '',
  }: {
    open: boolean;
    files: OutboxFile[];
    outboxDir?: string;
  } = $props();

  let loading = $state(false);

  async function refresh() {
    loading = true;
    try {
      files = await fetchOutbox();
    } finally {
      loading = false;
    }
  }

  async function remove(name: string) {
    await deleteOutboxFile(name);
    await refresh();
  }

  function formatSize(bytes: number): string {
    if (bytes < 1024) return `${bytes} B`;
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
    return `${(bytes / (1024 * 1024)).toFixed(2)} MB`;
  }

  $effect(() => {
    if (open) refresh();
  });
</script>

{#if open}
  <div
    transition:fade={{ duration: 160 }}
    class="fixed inset-0 bg-[var(--bg-backdrop)] backdrop-blur-sm z-50 flex items-end sm:items-center justify-center p-0 sm:p-4"
  >
    <button
      type="button"
      aria-label="Fechar Outbox"
      onclick={() => (open = false)}
      class="absolute inset-0 cursor-default"
    ></button>

    <div
      transition:fly={{ y: 24, duration: 210, easing: cubicOut }}
      class="relative w-full sm:max-w-md bg-[var(--bg-elevated)] border-t sm:border border-[var(--border-subtle)] rounded-t-2xl sm:rounded-2xl max-h-[82vh] flex flex-col shadow-2xl theme-surface"
    >
      <div class="p-4 border-b border-[var(--border-subtle)] flex items-center justify-between gap-2">
        <div class="flex items-center gap-2 min-w-0">
          <Icon name="outbox" size={18} class="text-[var(--accent-fg)]" />
          <div class="min-w-0">
            <h2 class="text-sm font-bold text-[var(--text-primary)] truncate">Outbox</h2>
            {#if outboxDir}
              <p class="text-[10px] font-mono text-[var(--text-faint)] truncate">{outboxDir}</p>
            {/if}
          </div>
        </div>
        <div class="flex items-center gap-1 shrink-0">
          <button
            type="button"
            onclick={refresh}
            class="p-1.5 rounded-lg text-[var(--text-muted)] hover:text-[var(--accent-fg)] transition-colors"
            title="Atualizar"
          >
            <Icon name="refresh" size={16} class={loading ? 'animate-spin' : ''} />
          </button>
          <button
            type="button"
            onclick={() => (open = false)}
            class="p-1.5 rounded-lg text-[var(--text-muted)] hover:text-[var(--text-primary)] transition-colors"
            title="Fechar"
          >
            <Icon name="close" size={18} />
          </button>
        </div>
      </div>

      <div class="p-3 overflow-y-auto flex-1 space-y-2">
        {#if loading}
          <p class="text-xs text-[var(--text-faint)] text-center py-8">Carregando arquivos...</p>
        {:else if files.length === 0}
          <p class="text-xs text-[var(--text-faint)] text-center py-8">Nenhum arquivo disponível no outbox.</p>
        {:else}
          {#each files as f}
            <div class="flex items-center justify-between gap-2 p-2.5 rounded-xl bg-[var(--bg-input)] border border-[var(--border-subtle)] theme-surface">
              <div class="min-w-0 flex-1">
                <div class="text-xs font-medium text-[var(--text-primary)] truncate">{f.name}</div>
                <div class="text-[11px] text-[var(--text-faint)]">{formatSize(f.size)}</div>
              </div>
              <div class="flex items-center gap-1.5 shrink-0">
                <a
                  href={`/api/outbox/${encodeURIComponent(f.name)}`}
                  download={f.name}
                  class="px-2.5 py-1.5 rounded-lg bg-[var(--accent-soft)] text-[var(--accent-fg)] border border-[var(--accent-border)] hover:opacity-90 text-xs font-medium flex items-center gap-1 transition-all active:scale-95"
                >
                  <Icon name="download" size={14} />
                  <span>Baixar</span>
                </a>
                <button
                  type="button"
                  onclick={() => remove(f.name)}
                  class="p-1.5 rounded-lg text-[var(--text-faint)] hover:text-rose-400 hover:bg-rose-500/10 transition-colors"
                  title="Excluir"
                >
                  <Icon name="trash" size={15} />
                </button>
              </div>
            </div>
          {/each}
        {/if}
      </div>
    </div>
  </div>
{/if}
