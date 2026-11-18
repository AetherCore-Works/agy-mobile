<script lang="ts">
  import { untrack } from 'svelte';
  import { fade, fly } from 'svelte/transition';
  import { cubicOut } from 'svelte/easing';
  import Icon from './Icon.svelte';
  import { Button } from '../lib/components/ui/button/index.js';
  import { Card } from '../lib/components/ui/card/index.js';
  import { Badge } from '../lib/components/ui/badge/index.js';
  import { deleteOutboxFile, fetchOutbox, type OutboxFile } from '../lib/api';

  let {
    open = $bindable(false),
    files = $bindable<OutboxFile[]>([]),
    outboxDir = "",
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
    const isOpen = open;
    if (isOpen) untrack(() => refresh());
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
      class="relative w-full sm:max-w-md"
    >
      <Card class="rounded-t-2xl sm:rounded-2xl max-h-[82vh] flex flex-col shadow-2xl overflow-hidden">
        <div class="p-4 border-b border-border flex items-center justify-between gap-2">
          <div class="flex items-center gap-2 min-w-0">
            <Icon name="outbox" size={18} class="text-accent-foreground" />
            <div class="min-w-0">
              <div class="flex items-center gap-2">
                <h2 class="text-sm font-bold text-foreground truncate">Outbox</h2>
                {#if files.length > 0}
                  <Badge variant="accent" size="xs" class="font-mono">{files.length}</Badge>
                {/if}
              </div>
              {#if outboxDir}
                <p class="text-[10px] font-mono text-muted-foreground truncate">{outboxDir}</p>
              {/if}
            </div>
          </div>
          <div class="flex items-center gap-1 shrink-0">
            <Button
              variant="ghost"
              size="icon-sm"
              onclick={refresh}
              title="Atualizar"
            >
              <Icon name="refresh" size={15} class={loading ? 'animate-spin' : ''} />
            </Button>
            <Button
              variant="ghost"
              size="icon-sm"
              onclick={() => (open = false)}
              title="Fechar"
            >
              <Icon name="close" size={17} />
            </Button>
          </div>
        </div>

        <div class="p-3 overflow-y-auto flex-1 space-y-2">
          {#if loading}
            <p class="text-xs text-muted-foreground text-center py-8">Carregando arquivos...</p>
          {:else if files.length === 0}
            <p class="text-xs text-muted-foreground text-center py-8">Nenhum arquivo disponível no outbox.</p>
          {:else}
            {#each files as f}
              <Card variant="soft" class="flex items-center justify-between gap-2 p-2.5 rounded-xl">
                <div class="min-w-0 flex-1">
                  <div class="text-xs font-medium text-foreground truncate">{f.name}</div>
                  <div class="text-[11px] font-mono text-muted-foreground">{formatSize(f.size)}</div>
                </div>
                <div class="flex items-center gap-1 shrink-0">
                  <Button
                    href={`/api/outbox/${encodeURIComponent(f.name)}`}
                    download={f.name}
                    variant="outline"
                    size="xs"
                    class="text-accent-foreground border-[var(--accent-border)] bg-accent"
                  >
                    <Icon name="download" size={13} />
                    <span>Baixar</span>
                  </Button>
                  <Button
                    variant="ghost"
                    size="icon-xs"
                    onclick={() => remove(f.name)}
                    class="hover:text-rose-400 hover:bg-rose-500/10"
                    title="Excluir"
                  >
                    <Icon name="trash" size={14} />
                  </Button>
                </div>
              </Card>
            {/each}
          {/if}
        </div>
      </Card>
    </div>
  </div>
{/if}
