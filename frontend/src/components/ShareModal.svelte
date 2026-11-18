<script lang="ts">
  import { fade, fly } from 'svelte/transition';
  import { cubicOut } from 'svelte/easing';
  import Icon from './Icon.svelte';
  import { Button } from '../lib/components/ui/button/index.js';
  import { Card } from '../lib/components/ui/card/index.js';
  import { Tabs, TabsList, TabsTrigger } from '../lib/components/ui/tabs/index.js';
  import { createSnapshot, type ChatMessage } from '../lib/api';
  import { buildConversationMarkdown, buildStandaloneSnapshotHTML, downloadTextFile } from '../lib/export';
  import { copyTextSafe, renderMarkdown } from '../lib/markdown';
  import type { ThemeMode, ThemePalette } from '../lib/theme';

  let {
    open = $bindable(false),
    title = 'Snapshot AGY',
    singleMarkdown = '',
    messages = [],
    themeMode,
    themePalette,
  }: {
    open: boolean;
    title?: string;
    singleMarkdown?: string;
    messages?: ChatMessage[];
    themeMode: ThemeMode;
    themePalette: ThemePalette;
  } = $props();

  let scope = $state<'response' | 'conversation'>('response');
  let creatingLink = $state(false);
  let snapshotUrl = $state('');
  let linkError = $state('');
  let feedbackMsg = $state('');

  $effect(() => {
    if (open) {
      scope = singleMarkdown.trim() ? 'response' : 'conversation';
      snapshotUrl = '';
      linkError = '';
      feedbackMsg = '';
    }
  });

  let activeMarkdown = $derived(
    scope === 'response' && singleMarkdown.trim()
      ? singleMarkdown
      : buildConversationMarkdown(title, messages)
  );

  let activeFilenameBase = $derived(
    (title || 'agy-snapshot')
      .toLowerCase()
      .replace(/[^a-z0-9]+/g, '-')
      .replace(/^-|-$/g, '')
      .slice(0, 42) || 'agy-snapshot'
  );

  function flash(msg: string) {
    feedbackMsg = msg;
    setTimeout(() => {
      if (feedbackMsg === msg) feedbackMsg = '';
    }, 2000);
  }

  function handleDownloadHTML() {
    const rendered = renderMarkdown(activeMarkdown);
    const html = buildStandaloneSnapshotHTML(title, rendered, activeMarkdown, themeMode, themePalette);
    downloadTextFile(`${activeFilenameBase}.html`, html, 'text/html;charset=utf-8');
    flash('HTML offline baixado');
  }

  function handleDownloadMD() {
    downloadTextFile(`${activeFilenameBase}.md`, activeMarkdown, 'text/markdown;charset=utf-8');
    flash('Markdown baixado');
  }

  async function handleCopyMarkdown() {
    if (await copyTextSafe(activeMarkdown)) {
      flash('Markdown copiado');
    }
  }

  async function handleCreateLink() {
    creatingLink = true;
    linkError = '';
    try {
      const res = await createSnapshot({
        title: title || 'Snapshot AGY',
        content_md: activeMarkdown,
        theme_mode: themeMode,
        theme_palette: themePalette,
      });
      snapshotUrl = `${window.location.origin}${res.url}`;
      await copyTextSafe(snapshotUrl);
      flash('Link somente leitura copiado');
    } catch (err: any) {
      linkError = err?.message || 'Erro ao gerar link';
    } finally {
      creatingLink = false;
    }
  }
</script>

{#if open}
  <div
    transition:fade={{ duration: 150 }}
    class="fixed inset-0 bg-[var(--bg-backdrop)] backdrop-blur-sm z-50 flex items-end sm:items-center justify-center p-0 sm:p-4"
  >
    <button
      type="button"
      aria-label="Fechar compartilhamento"
      onclick={() => (open = false)}
      class="absolute inset-0 cursor-default"
    ></button>

    <div
      transition:fly={{ y: 20, duration: 200, easing: cubicOut }}
      class="relative w-full sm:max-w-md"
    >
      <Card class="rounded-t-2xl sm:rounded-2xl p-4 space-y-4 shadow-2xl">
        <div class="flex items-center justify-between gap-2">
          <div class="flex items-center gap-2 min-w-0">
            <Icon name="share" size={17} class="text-accent-foreground" />
            <h2 class="text-sm font-bold text-foreground truncate">Compartilhar Snapshot</h2>
          </div>
          <Button
            variant="ghost"
            size="icon-sm"
            onclick={() => (open = false)}
          >
            <Icon name="close" size={17} />
          </Button>
        </div>

        {#if singleMarkdown.trim() && messages.length > 0}
          <Tabs
            value={scope}
            onValueChange={(v) => {
              scope = v === 'conversation' ? 'conversation' : 'response';
              snapshotUrl = '';
            }}
          >
            <TabsList class="grid grid-cols-2 w-full">
              <TabsTrigger value="response">Esta resposta</TabsTrigger>
              <TabsTrigger value="conversation">Conversa toda</TabsTrigger>
            </TabsList>
          </Tabs>
        {/if}

        <div class="grid grid-cols-1 sm:grid-cols-2 gap-2">
          <Button
            variant="default"
            onclick={handleDownloadHTML}
            class="h-10 rounded-xl"
          >
            <Icon name="download" size={15} />
            <span>Baixar HTML Formatado</span>
          </Button>

          <Button
            variant="outline"
            onclick={handleDownloadMD}
            class="h-10 rounded-xl"
          >
            <Icon name="file" size={15} />
            <span>Baixar Markdown (.md)</span>
          </Button>

          <Button
            variant="outline"
            onclick={handleCopyMarkdown}
            class="h-10 rounded-xl"
          >
            <Icon name="clipboard" size={15} />
            <span>Copiar Markdown</span>
          </Button>

          <Button
            variant="outline"
            onclick={handleCreateLink}
            disabled={creatingLink}
            class="h-10 rounded-xl"
          >
            <Icon name="link" size={15} />
            <span>{creatingLink ? 'Gerando...' : 'Link Local (/s/...)'}</span>
          </Button>
        </div>

        {#if snapshotUrl}
          <div
            transition:fade={{ duration: 140 }}
            class="p-2.5 rounded-xl bg-muted border border-[var(--accent-border)] space-y-1.5"
          >
            <div class="text-[11px] font-medium text-muted-foreground">Link somente leitura (sem acesso à API/terminal):</div>
            <div class="flex items-center gap-2">
              <input
                type="text"
                readonly
                value={snapshotUrl}
                class="flex-1 bg-transparent text-xs font-mono text-accent-foreground focus:outline-none truncate"
              />
              <Button
                href={snapshotUrl}
                target="_blank"
                rel="noopener noreferrer"
                size="xs"
              >
                Abrir
              </Button>
            </div>
          </div>
        {/if}

        {#if linkError}
          <p class="text-xs text-rose-400">{linkError}</p>
        {/if}

        {#if feedbackMsg}
          <div
            transition:fade={{ duration: 140 }}
            class="p-2 rounded-xl bg-emerald-500/15 border border-emerald-500/35 text-emerald-500 text-xs flex items-center justify-center gap-1.5"
          >
            <Icon name="check" size={14} />
            <span>{feedbackMsg}</span>
          </div>
        {/if}
      </Card>
    </div>
  </div>
{/if}
