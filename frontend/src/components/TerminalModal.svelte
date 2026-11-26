<script lang="ts">
  import { onDestroy, onMount } from 'svelte';
  import { fade, fly } from 'svelte/transition';
  import { cubicOut } from 'svelte/easing';
  import { Terminal } from '@xterm/xterm';
  import { FitAddon } from '@xterm/addon-fit';
  import Icon from './Icon.svelte';
  import { getActiveAccentColor } from '../lib/theme';

  let {
    open = $bindable(false),
    hostLabel = 'Terminal Local',
  }: {
    open: boolean;
    hostLabel?: string;
  } = $props();

  let termContainer: HTMLDivElement | undefined = $state();
  let termInputVal = $state('');
  let connected = $state(false);
  let statusMsg = $state('Desconectado');
  let modalHeight = $state('');
  let modalTop = $state('0px');
  let modalBottom = $state('0px');
  let fontSize = $state(13);
  let termCols = $state(80);
  let termRows = $state(24);

  let term: Terminal | null = null;
  let fitAddon: FitAddon | null = null;
  let ws: WebSocket | null = null;
  const encoder = new TextEncoder();
  const decoder = new TextDecoder('utf-8');

  function sendTextToTty(text: string) {
    if (!ws || ws.readyState !== WebSocket.OPEN) return;
    const payload = encoder.encode('0' + text);
    ws.send(payload);
  }

  function sendResize(cols: number, rows: number) {
    termCols = cols;
    termRows = rows;
    if (!ws || ws.readyState !== WebSocket.OPEN) return;
    const msg = '1' + JSON.stringify({ columns: cols, rows: rows });
    ws.send(encoder.encode(msg));
  }

  function changeFontSize(delta: number) {
    const next = Math.min(20, Math.max(10, fontSize + delta));
    if (next === fontSize) return;
    fontSize = next;
    if (term) {
      term.options.fontSize = next;
      setTimeout(() => {
        if (fitAddon && term) {
          fitAddon.fit();
          sendResize(term.cols, term.rows);
        }
      }, 30);
    }
  }

  function connectTerminal() {
    disconnectTerminal();
    if (!termContainer) return;

    const accent = getActiveAccentColor();
    term = new Terminal({
      cursorBlink: true,
      cursorStyle: 'block',
      fontSize,
      lineHeight: 1.16,
      fontFamily: '"JetBrains Mono", ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace',
      theme: {
        background: '#09090b',
        foreground: '#e4e4e7',
        cursor: accent,
        selectionBackground: 'rgba(56, 189, 248, 0.24)',
        black: '#18181b',
        red: '#f43f5e',
        green: '#10b981',
        yellow: '#f59e0b',
        blue: '#38bdf8',
        magenta: '#a855f7',
        cyan: '#06b6d4',
        white: '#d4d4d8',
        brightBlack: '#52525b',
        brightRed: '#fb7185',
        brightGreen: '#34d399',
        brightYellow: '#fbbf24',
        brightBlue: '#7dd3fc',
        brightMagenta: '#c084fc',
        brightCyan: '#22d3ee',
        brightWhite: '#fafafa',
      },
      scrollback: 3000,
    });
    fitAddon = new FitAddon();
    term.loadAddon(fitAddon);
    term.open(termContainer);

    setTimeout(() => {
      fitAddon?.fit();
    }, 50);

    const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const wsUrl = `${proto}//${window.location.host}/terminal/ws`;
    statusMsg = 'Conectando...';

    ws = new WebSocket(wsUrl, ['tty']);
    ws.binaryType = 'arraybuffer';

    ws.onopen = () => {
      connected = true;
      statusMsg = `Conectado (${hostLabel})`;
      ws?.send(encoder.encode(JSON.stringify({ AuthToken: '', columns: term?.cols ?? 80, rows: term?.rows ?? 24 })));
      if (term) {
        fitAddon?.fit();
        sendResize(term.cols, term.rows);
      }
    };

    ws.onmessage = (ev) => {
      if (!(ev.data instanceof ArrayBuffer)) return;
      const bytes = new Uint8Array(ev.data);
      if (bytes.length === 0) return;
      const cmd = String.fromCharCode(bytes[0]);
      if (cmd === '0') {
        const text = decoder.decode(bytes.subarray(1));
        term?.write(text);
        const oauthUrl = extractGoogleOAuthUrl(text);
        if (oauthUrl) {
          detectedOAuthUrl = oauthUrl;
        }
      }
    };

    ws.onclose = () => {
      connected = false;
      statusMsg = 'Sessão encerrada';
    };

    ws.onerror = () => {
      connected = false;
      statusMsg = 'Erro de conexão WebSocket';
    };

    term.onData((data) => {
      sendTextToTty(data);
    });

    term.onResize(({ cols, rows }) => {
      sendResize(cols, rows);
    });
  }

  function disconnectTerminal() {
    detectedOAuthUrl = "";
    if (ws) {
      ws.close();
      ws = null;
    }
    if (term) {
      term.dispose();
      term = null;
    }
    fitAddon = null;
    connected = false;
  }

  function handleKey(key: 'ESC' | 'TAB' | 'CTRL_C' | 'CTRL_L' | 'UP' | 'DOWN') {
    const map: Record<string, string> = {
      ESC: '\x1b',
      TAB: '\t',
      CTRL_C: '\x03',
      CTRL_L: '\x0c',
      UP: '\x1b[A',
      DOWN: '\x1b[B',
    };
    sendTextToTty(map[key]);
  }

  function insertText(txt: string) {
    termInputVal += txt;
  }

  let detectedOAuthUrl = $state("");

  function extractGoogleOAuthUrl(raw: string): string {
    const clean = raw.replace(/\x1b\[[0-9;]*[A-Za-z]/g, '');
    const m = clean.match(/https:\/\/accounts\.google\.com\/[^\s"'<>]+/);
    if (!m) return '';
    return m[0].replace(/[.,;:)\]}]+$/, '');
  }

  async function pasteClipboard() {
    try {
      const text = await navigator.clipboard.readText();
      if (text) {
        termInputVal += text;
      }
    } catch {
      // Fallback: user can paste directly in the input field
    }
  }

  function submitInput() {
    const val = termInputVal;
    sendTextToTty(val + '\r');
    termInputVal = '';
  }

  function updateViewport() {
    if (!open) return;
    const vv = window.visualViewport;
    const isKbOpen = Boolean(vv && vv.height < window.innerHeight - 80);
    document.documentElement.dataset.kbOpen = isKbOpen ? 'true' : 'false';
    if (vv && isKbOpen) {
      modalHeight = `${vv.height}px`;
      modalTop = `${vv.offsetTop}px`;
      modalBottom = 'auto';
    } else {
      modalHeight = '';
      modalTop = '0px';
      modalBottom = '0px';
    }
    setTimeout(() => {
      if (fitAddon && term) {
        fitAddon.fit();
        sendResize(term.cols, term.rows);
      }
    }, 60);
  }

  $effect(() => {
    if (open && termContainer && !term) {
      updateViewport();
      connectTerminal();
    } else if (!open && term) {
      disconnectTerminal();
    }
  });

  onMount(() => {
    window.visualViewport?.addEventListener('resize', updateViewport);
    window.visualViewport?.addEventListener('scroll', updateViewport);
  });

  onDestroy(() => {
    window.visualViewport?.removeEventListener('resize', updateViewport);
    window.visualViewport?.removeEventListener('scroll', updateViewport);
    disconnectTerminal();
  });
</script>

{#if open}
  <div
    transition:fade={{ duration: 150 }}
    style="top: {modalTop}; bottom: {modalBottom}; {modalHeight ? `height: ${modalHeight};` : ''}"
    class="fixed inset-x-0 bg-zinc-950 z-50 flex flex-col select-none"
  >
    <!-- Safe-Area Aware Terminal Header -->
    <div
      transition:fly={{ y: -12, duration: 190, easing: cubicOut }}
      class="h-[calc(3.25rem+env(safe-area-inset-top))] pt-[env(safe-area-inset-top)] bg-[var(--bg-surface)]/95 backdrop-blur-xl border-b border-[var(--border-subtle)] px-3 sm:px-4 flex items-center justify-between gap-2 shrink-0 theme-surface"
    >
      <div class="flex items-center gap-2 min-w-0">
        <div class="flex items-center gap-1.5 px-2 py-0.5 rounded-full bg-zinc-900/90 border border-zinc-800">
          <span class="w-2 h-2 rounded-full shrink-0 {connected ? 'bg-emerald-500 shadow-[0_0_8px_rgba(16,185,129,0.5)]' : 'bg-amber-500 animate-pulse'}"></span>
          <span class="text-[11px] font-mono font-bold text-zinc-100 truncate">{hostLabel}</span>
        </div>
        <span class="hidden xs:inline-flex text-[10px] font-mono text-zinc-400 bg-zinc-900/70 border border-zinc-800/80 px-1.5 py-0.5 rounded-md shrink-0">
          {termCols}×{termRows}
        </span>
      </div>
      <div class="flex items-center gap-1.5 shrink-0">
        <div class="flex items-center rounded-lg bg-[var(--bg-chip)] border border-[var(--border-subtle)] overflow-hidden">
          <button
            type="button"
            onclick={() => changeFontSize(-1)}
            title="Diminuir fonte"
            class="px-2 py-1 text-[11px] font-mono text-[var(--text-muted)] hover:text-[var(--text-primary)] active:bg-zinc-800 transition-colors"
          >
            A-
          </button>
          <div class="w-px h-3.5 bg-[var(--border-subtle)]"></div>
          <button
            type="button"
            onclick={() => changeFontSize(1)}
            title="Aumentar fonte"
            class="px-2 py-1 text-[11px] font-mono text-[var(--text-muted)] hover:text-[var(--text-primary)] active:bg-zinc-800 transition-colors"
          >
            A+
          </button>
        </div>
        <button
          type="button"
          onclick={connectTerminal}
          title="Reconectar ({statusMsg})"
          class="p-1.5 rounded-lg bg-[var(--bg-chip)] border border-[var(--border-subtle)] text-[var(--text-muted)] hover:text-[var(--text-primary)] transition-colors active:scale-95"
        >
          <Icon name="refresh" size={15} />
        </button>
        <button
          type="button"
          onclick={() => (open = false)}
          class="px-2.5 py-1 rounded-lg bg-rose-500/15 text-rose-400 border border-rose-500/30 hover:bg-rose-500/25 text-xs font-semibold transition-all active:scale-95"
        >
          Fechar
        </button>
      </div>
    </div>

    <!-- Native xterm.js Canvas (always dark high-contrast with breathing padding) -->
    {#if detectedOAuthUrl}
      <div class="bg-amber-500/10 border-b border-amber-500/30 px-3 py-2 flex flex-wrap items-center justify-between gap-2 shrink-0">
        <div class="min-w-0 flex-1">
          <p class="text-[11px] font-semibold text-amber-300">URL oficial de login Google detectada (Remote SSH OAuth)</p>
          <p class="text-[10px] text-[var(--text-muted)] truncate">Abra o link, copie o código descartável e toque em <strong>Colar</strong> abaixo.</p>
        </div>
        <div class="flex items-center gap-1.5 shrink-0">
          <a
            href={detectedOAuthUrl}
            target="_blank"
            rel="noopener noreferrer"
            class="px-2.5 py-1 rounded-lg bg-amber-500/20 hover:bg-amber-500/30 text-amber-200 text-[11px] font-semibold border border-amber-500/40"
          >
            Abrir Login Google
          </a>
          <button
            type="button"
            onclick={() => (detectedOAuthUrl = "")}
            class="px-2 py-1 rounded-lg text-[11px] text-[var(--text-muted)] hover:text-[var(--text-primary)]"
            aria-label="Fechar aviso OAuth"
          >
            ×
          </button>
        </div>
      </div>
    {/if}
    <div class="flex-1 min-h-0 w-full bg-zinc-950 px-2.5 py-1.5 flex flex-col overflow-hidden">
      <div bind:this={termContainer} class="flex-1 min-h-0 w-full overflow-hidden"></div>
    </div>

    <!-- Mobile Quick Keys & Input Bar (Dynamic Safe-Area Bottom) -->
    <div class="bg-[var(--bg-surface)]/95 backdrop-blur-xl border-t border-[var(--border-subtle)] px-2.5 pt-2 pb-[max(0.5rem,var(--safe-bottom-dynamic))] flex flex-col gap-1.5 shrink-0 theme-surface">
      <div class="flex items-center gap-1.5 overflow-x-auto pb-0.5 no-scrollbar">
        <button type="button" onclick={() => handleKey('ESC')} class="px-2.5 py-1 rounded-lg bg-[var(--bg-chip)] hover:bg-[var(--bg-chip-hover)] text-[var(--text-primary)] text-[11px] font-mono font-medium border border-[var(--border-strong)] shrink-0 transition-transform active:scale-95">Esc</button>
        <button type="button" onclick={() => handleKey('TAB')} class="px-2.5 py-1 rounded-lg bg-[var(--bg-chip)] hover:bg-[var(--bg-chip-hover)] text-[var(--text-primary)] text-[11px] font-mono font-medium border border-[var(--border-strong)] shrink-0 transition-transform active:scale-95">Tab</button>
        <button type="button" onclick={() => handleKey('CTRL_C')} class="px-2.5 py-1 rounded-lg bg-rose-500/15 hover:bg-rose-500/25 text-rose-400 text-[11px] font-mono font-medium border border-rose-500/35 shrink-0 transition-transform active:scale-95">Ctrl+C</button>
        <button type="button" onclick={() => handleKey('CTRL_L')} class="px-2.5 py-1 rounded-lg bg-[var(--bg-chip)] hover:bg-[var(--bg-chip-hover)] text-[var(--text-muted)] text-[11px] font-mono border border-[var(--border-strong)] shrink-0 transition-transform active:scale-95">Limpar</button>
        <button type="button" onclick={() => handleKey('UP')} class="px-2 py-1 rounded-lg bg-[var(--bg-chip)] hover:bg-[var(--bg-chip-hover)] text-[var(--text-primary)] border border-[var(--border-strong)] shrink-0 transition-transform active:scale-95" aria-label="Cima">
          <Icon name="chevron-up" size={14} />
        </button>
        <button type="button" onclick={() => handleKey('DOWN')} class="px-2 py-1 rounded-lg bg-[var(--bg-chip)] hover:bg-[var(--bg-chip-hover)] text-[var(--text-primary)] border border-[var(--border-strong)] shrink-0 transition-transform active:scale-95" aria-label="Baixo">
          <Icon name="chevron-down" size={14} />
        </button>
        <button type="button" onclick={() => insertText('sudo ')} class="px-2.5 py-1 rounded-lg bg-emerald-500/15 hover:bg-emerald-500/25 text-emerald-400 text-[11px] font-mono font-medium border border-emerald-500/35 shrink-0 transition-transform active:scale-95">sudo</button>
        <button type="button" onclick={pasteClipboard} class="px-2.5 py-1 rounded-lg bg-[var(--accent-soft)] hover:opacity-90 text-[var(--accent-fg)] text-[11px] font-mono font-medium border border-[var(--accent-border)] shrink-0 flex items-center gap-1 transition-transform active:scale-95">
          <Icon name="clipboard" size={12} />
          <span>Colar</span>
        </button>
      </div>
      <div class="flex items-center gap-1.5">
        <input
          type="text"
          bind:value={termInputVal}
          autocapitalize="off"
          autocorrect="off"
          spellcheck="false"
          placeholder="Comando rápido / colar texto..."
          class="flex-1 bg-[var(--bg-input)] border border-[var(--border-subtle)] rounded-xl px-3 py-1.5 text-xs font-mono text-[var(--text-primary)] placeholder-[var(--text-faint)] focus:outline-none focus:border-[var(--accent-fg)]"
          onkeydown={(e) => {
            if (e.key === 'Enter') {
              e.preventDefault();
              submitInput();
            }
          }}
        />
        <button
          type="button"
          onclick={submitInput}
          class="px-3.5 py-1.5 rounded-xl bg-[var(--accent-solid)] hover:bg-[var(--accent-hover)] text-[var(--accent-on-solid)] text-xs font-mono font-semibold shrink-0 transition-all active:scale-95"
        >
          Enviar
        </button>
      </div>
    </div>
  </div>
{/if}
