<script lang="ts">
  import { fade, fly } from 'svelte/transition';
  import { cubicOut } from 'svelte/easing';
  import Icon from './Icon.svelte';
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
      class="relative w-full sm:max-w-md bg-[var(--bg-elevated)] border-t sm:border border-[var(--border-subtle)] rounded-t-2xl sm:rounded-2xl p-4 space-y-4 shadow-2xl theme-surface"
    >
      <div class="flex items-center justify-between gap-2">
        <div class="flex items-center gap-2 min-w-0">
          <Icon name="share" size={17} class="text-[var(--accent-fg)]" />
          <h2 class="text-sm font-bold text-[var(--text-primary)] truncate">Compartilhar Snapshot</h2>
        </div>
        <button
          type="button"
          onclick={() => (open = false)}
          class="p-1.5 rounded-lg text-[var(--text-muted)] hover:text-[var(--text-primary)]"
        >
          <Icon name="close" size={18} />
        </button>
      </div>

      {#if singleMarkdown.trim() && messages.length > 0}
        <div class="grid grid-cols-2 gap-1.5 p-1 rounded-xl bg-[var(--bg-input)] border border-[var(--border-subtle)]">
          <button
            type="button"
            onclick={() => { scope = 'response'; snapshotUrl = ''; }}
            class="py-1.5 rounded-lg text-xs font-semibold transition-all {scope === 'response' ? 'bg-[var(--bg-elevated)] text-[var(--accent-fg)] border border-[var(--border-subtle)] shadow-sm' : 'text-[var(--text-muted)]'}"
          >
            Esta resposta
          </button>
          <button
            type="button"
            onclick={() => { scope = 'conversation'; snapshotUrl = ''; }}
            class="py-1.5 rounded-lg text-xs font-semibold transition-all {scope === 'conversation' ? 'bg-[var(--bg-elevated)] text-[var(--accent-fg)] border border-[var(--border-subtle)] shadow-sm' : 'text-[var(--text-muted)]'}"
          >
            Conversa toda
          </button>
        </div>
      {/if}

      <div class="grid grid-cols-1 sm:grid-cols-2 gap-2">
        <button
          type="button"
          onclick={handleDownloadHTML}
          class="p-2.5 rounded-xl bg-[var(--accent-soft)] border border-[var(--accent-border)] text-[var(--accent-fg)] text-xs font-semibold flex items-center justify-center gap-1.5 hover:opacity-90 transition-all active:scale-95"
        >
          <Icon name="download" size={15} />
          <span>Baixar HTML Formatado</span>
        </button>

        <button
          type="button"
          onclick={handleDownloadMD}
          class="p-2.5 rounded-xl bg-[var(--bg-input)] border border-[var(--border-subtle)] text-[var(--text-primary)] hover:border-[var(--border-strong)] text-xs font-semibold flex items-center justify-center gap-1.5 transition-all active:scale-95"
        >
          <Icon name="file" size={15} />
          <span>Baixar Markdown (.md)</span>
        </button>

        <button
          type="button"
          onclick={handleCopyMarkdown}
          class="p-2.5 rounded-xl bg-[var(--bg-input)] border border-[var(--border-subtle)] text-[var(--text-secondary)] hover:border-[var(--border-strong)] text-xs font-semibold flex items-center justify-center gap-1.5 transition-all active:scale-95"
        >
          <Icon name="clipboard" size={15} />
          <span>Copiar Markdown</span>
        </button>

        <button
          type="button"
          onclick={handleCreateLink}
          disabled={creatingLink}
          class="p-2.5 rounded-xl bg-[var(--bg-input)] border border-[var(--border-subtle)] text-[var(--text-secondary)] hover:border-[var(--accent-border)] hover:text-[var(--accent-fg)] text-xs font-semibold flex items-center justify-center gap-1.5 transition-all active:scale-95"
        >
          <Icon name="link" size={15} />
          <span>{creatingLink ? 'Gerando...' : 'Link Local (/s/...)'}</span>
        </button>
      </div>

      {#if snapshotUrl}
        <div
          transition:fade={{ duration: 140 }}
          class="p-2.5 rounded-xl bg-[var(--bg-input)] border border-[var(--accent-border)] space-y-1.5"
        >
          <div class="text-[11px] font-medium text-[var(--text-muted)]">Link somente leitura (sem acesso à API/terminal):</div>
          <div class="flex items-center gap-2">
            <input
              type="text"
              readonly
              value={snapshotUrl}
              class="flex-1 bg-transparent text-xs font-mono text-[var(--accent-fg)] focus:outline-none truncate"
            />
            <a
              href={snapshotUrl}
              target="_blank"
              rel="noopener noreferrer"
              class="px-2 py-1 rounded-lg bg-[var(--accent-solid)] text-[var(--accent-on-solid)] text-[11px] font-semibold shrink-0"
            >
              Abrir
            </a>
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
    </div>
  </div>
{/if}
