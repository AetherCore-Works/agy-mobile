<script lang="ts">
  import { onDestroy, onMount, tick } from 'svelte';
  import { fade, fly, scale } from 'svelte/transition';
  import { cubicOut } from 'svelte/easing';
  import { marked } from 'marked';
  import Icon from './components/Icon.svelte';
  import AiSpinner from './components/AiSpinner.svelte';
  import Sidebar from './components/Sidebar.svelte';
  import TerminalModal from './components/TerminalModal.svelte';
  import OutboxModal from './components/OutboxModal.svelte';
  import {
    cancelJob,
    createJob,
    fetchConversation,
    fetchFeatures,
    fetchModels,
    fetchOutbox,
    fetchSettings,
    fetchStatus,
    fetchWatchdog,
    triggerWol,
    updateSettings,
    uploadFile,
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
  import { escapeHtml, formatLangLabel, highlightCode } from './lib/highlight';

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
      .catch(() => {
        // Keep local theme applied even if gateway is temporarily unreachable
      });
  }

  // Gateway & Chat State
  let targetOnline = $state<boolean | null>(null);
  let activeOS = $state<string>('linux');
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
  let selectedModel = $state('gemini-4-hydrogen-high');
  let permissionMode = $state<PermissionMode>('copilot');

  let conversationId = $state<string | null>(localStorage.getItem('agy_conv_id'));
  let messages = $state<ChatMessage[]>([]);
  let userInput = $state('');
  let attachedFiles = $state<{ name: string; path: string }[]>([]);
  let uploading = $state(false);

  let hostLabel = $derived(
    gatewayMode === 'standalone'
      ? 'AGY Local'
      : gatewayMode === 'ide-bridge'
        ? 'IDE Bridge'
        : 'CachyCore'
  );

  // Streaming / Job state
  let isGenerating = $state(false);
  let activeJobId = $state<string | null>(localStorage.getItem('agy_active_job'));
  let activeJobLastSeq = $state<number>(Number(localStorage.getItem('agy_active_seq') || '0'));
  let liveStatus = $state('');
  let liveElapsed = $state(0);
  let jobProgress = $state(0);
  let copiedMsgIdx = $state<number | null>(null);
  let streamAbort: AbortController | null = null;
  let elapsedTimer: ReturnType<typeof setInterval> | null = null;
  let finishTimer: ReturnType<typeof setTimeout> | null = null;
  let copyMsgTimer: ReturnType<typeof setTimeout> | null = null;
  let toolStepCount = 0;
  let deltaCharsCount = 0;

  function setProgressFloor(target: number) {
    const clamped = Math.max(0, Math.min(100, target));
    if (clamped > jobProgress) {
      jobProgress = clamped;
    }
  }

  // Modals & Drawers
  let sidebarOpen = $state(false);
  let terminalOpen = $state(false);
  let outboxOpen = $state(false);
  let outboxFiles = $state<OutboxFile[]>([]);

  // Voice Dictation & Slash Commands
  let isListening = $state(false);
  let recognition = $state<any>(null);
  let slashMenuOpen = $state(false);
  let requestOutbox = $state(false);
  let recentOutboxDeliveries = $state<string[]>([]);

  // Viewport & Scroll State (iOS Safari / iPhone Pro Max keyboard & resume support)
  let keyboardOpen = $state(false);
  let isNearBottom = $state(true);
  let showScrollBottomBtn = $state(false);
  let lastViewportHeight = 0;
  let viewportSyncRaf: number | null = null;
  let viewportSyncUntil = 0;

  const slashCommands = [
    { cmd: '/outbox', desc: 'Pedir para a IA salvar arquivo no Outbox p/ download' },
    { cmd: '/usage', desc: 'Cota restante de tokens/modelos' },
    { cmd: '/compact', desc: 'Compactar histórico da sessão' },
    { cmd: '/diff', desc: 'Exibir git status e diff atual' },
    { cmd: '/mem', desc: 'Consultar memória persistente (ai-memory)' },
    { cmd: '/plan', desc: 'Ativar modo Planejamento (Leitura)' },
    { cmd: '/clear', desc: 'Limpar e iniciar nova conversa' },
    { cmd: '/status', desc: 'Status do Watchdog & recursos' },
  ];

  let approvalNeeded = $state<{ tool: string; command: string } | null>(null);

  async function runSlashCommand(cmd: string) {
    slashMenuOpen = false;
    userInput = '';
    if (cmd === '/clear') {
      newConversation();
      return;
    }
    if (cmd === '/outbox') {
      requestOutbox = true;
      userInput = 'Gere/copie o arquivo solicitado para o Outbox (/home/ai/agy-outbox) para download no celular: ';
      textareaEl?.focus();
      return;
    }
    if (cmd === '/plan') {
      permissionMode = 'readonly';
      messages = [...messages, { role: 'agent', content: '**Modo Planejamento (`readonly`) ativado.** Nenhuma alteração será aplicada sem revisão.' }];
      scrollToBottom(true);
      return;
    }
    if (cmd === '/diff') {
      userInput = 'Execute git status -s e git diff no workspace atual e exiba a saída dentro de um bloco ```diff.';
      await handleSubmit();
      return;
    }
    if (cmd === '/mem') {
      userInput = 'Consulte no ai-memory: ';
      textareaEl?.focus();
      return;
    }
    if (cmd === '/status') {
      const [wd, feat] = await Promise.all([fetchWatchdog(), fetchFeatures()]);
      const summary = [
        `### Status do Gateway (v${feat.version})`,
        `- **Host:** ${hostLabel} (${wd.online === false ? 'Offline' : 'Online'})`,
        `- **Verificações Watchdog:** ${wd.uptime_checks} (Quedas: ${wd.downtime_events})`,
        `- **Recursos:** ${Object.entries(feat.features).filter(([, v]) => v === true).map(([k]) => `\`${k}\``).join(', ')}`,
      ].join('\n');
      messages = [...messages, { role: 'agent', content: summary }];
      scrollToBottom(true);
      return;
    }
    if (cmd === '/compact' && !conversationId) {
      messages = [...messages, { role: 'agent', content: '> Não há conversa ativa para compactar. Envie uma mensagem ou carregue uma sessão primeiro.' }];
      scrollToBottom(true);
      return;
    }
    userInput = cmd;
    await handleSubmit();
  }

  // DOM Refs
  let shellEl: HTMLElement | undefined = $state();
  let chatContainer: HTMLElement | undefined = $state();
  let fileInputEl: HTMLInputElement | undefined = $state();
  let textareaEl: HTMLTextAreaElement | undefined = $state();

  function handleChatScroll() {
    if (!chatContainer) return;
    const dist = chatContainer.scrollHeight - chatContainer.scrollTop - chatContainer.clientHeight;
    isNearBottom = dist < 110;
    showScrollBottomBtn = dist > 140 && messages.length > 0;
  }

  async function scrollToBottom(force = false, smooth = false) {
    await tick();
    if (!chatContainer) return;
    if (!force && !isNearBottom) return;
    const applyScroll = () => {
      if (!chatContainer) return;
      if (smooth && typeof chatContainer.scrollTo === 'function') {
        chatContainer.scrollTo({ top: chatContainer.scrollHeight, behavior: 'smooth' });
      } else {
        chatContainer.scrollTop = chatContainer.scrollHeight;
      }
      isNearBottom = true;
      showScrollBottomBtn = false;
    };
    applyScroll();
    requestAnimationFrame(applyScroll);
  }

  function applyViewportNow() {
    if (window.scrollY !== 0 || window.scrollX !== 0) {
      window.scrollTo(0, 0);
    }
    const vv = window.visualViewport;
    if (vv) {
      const prevH = lastViewportHeight || vv.height;
      const changed = Math.abs(prevH - vv.height) > 1;
      lastViewportHeight = vv.height;
      keyboardOpen = window.innerHeight - vv.height > 110;

      if (shellEl) {
        shellEl.style.height = `${vv.height}px`;
        shellEl.style.transform = `translate3d(0, ${vv.offsetTop}px, 0)`;
      }

      if (changed && (prevH - vv.height > 20 || document.activeElement === textareaEl || isNearBottom)) {
        if (chatContainer && isNearBottom) {
          chatContainer.scrollTop = chatContainer.scrollHeight;
          showScrollBottomBtn = false;
        }
      }
    } else if (shellEl) {
      shellEl.style.height = '100dvh';
      shellEl.style.transform = 'translate3d(0, 0, 0)';
    }
  }

  function startViewportSync(durationMs = 460) {
    viewportSyncUntil = Math.max(viewportSyncUntil, performance.now() + durationMs);
    applyViewportNow();
    if (viewportSyncRaf !== null) return;

    const tickFrame = (now: number) => {
      applyViewportNow();
      if (now < viewportSyncUntil) {
        viewportSyncRaf = requestAnimationFrame(tickFrame);
      } else {
        viewportSyncRaf = null;
      }
    };
    viewportSyncRaf = requestAnimationFrame(tickFrame);
  }

  function updateAppViewport() {
    startViewportSync(260);
  }

  function scheduleKeyboardScroll() {
    startViewportSync(480);
    if (isNearBottom) {
      scrollToBottom(true);
    }
  }

  function resizeTextarea(el: HTMLTextAreaElement) {
    const prevHeight = el.offsetHeight;
    el.style.height = 'auto';
    const nextHeight = Math.min(Math.max(el.scrollHeight, 40), 120);
    el.style.height = `${nextHeight}px`;
    if (nextHeight !== prevHeight) {
      applyViewportNow();
      if (isNearBottom && chatContainer) {
        chatContainer.scrollTop = chatContainer.scrollHeight;
      }
    }
  }

  const COPY_ICON_SVG = '<svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.85" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M16 4h2a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2h2"></path><rect x="8" y="2" width="8" height="4" rx="1" ry="1"></rect></svg>';
  const CHECK_ICON_SVG = '<svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.1" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><polyline points="20 6 9 17 4 12"></polyline></svg>';

  const mdRenderer = new marked.Renderer();
  mdRenderer.code = ({ text, lang }: { text: string; lang?: string }) => {
    const isDiff = lang === 'diff' || (/^(---|\+\+\+|@@ )/m.test(text) && /^[+-]/m.test(text));
    if (isDiff) {
      const linesHtml = text
        .split('\n')
        .map((line) => {
          const esc = escapeHtml(line) || ' ';
          if (line.startsWith('+') && !line.startsWith('+++')) {
            return `<div class="diff-add px-3 whitespace-pre">${esc}</div>`;
          }
          if (line.startsWith('-') && !line.startsWith('---')) {
            return `<div class="diff-del px-3 whitespace-pre">${esc}</div>`;
          }
          if (line.startsWith('@@')) {
            return `<div class="diff-hunk px-3 font-semibold whitespace-pre">${esc}</div>`;
          }
          return `<div class="diff-ctx px-3 whitespace-pre">${esc}</div>`;
        })
        .join('');
      return `<details open class="code-block diff-box"><summary class="code-block-header cursor-pointer"><span class="code-lang">Diff Visual</span><button type="button" class="code-copy-btn" data-copy-code title="Copiar diff">${COPY_ICON_SVG}<span>Copiar</span></button></summary><div class="diff-lines overflow-x-auto text-[11.5px] font-mono py-2 leading-5">${linesHtml}</div></details>`;
    }
    const label = escapeHtml(formatLangLabel(lang));
    const highlighted = highlightCode(text, lang);
    return `<div class="code-block"><div class="code-block-header"><span class="code-lang">${label}</span><button type="button" class="code-copy-btn" data-copy-code title="Copiar código">${COPY_ICON_SVG}<span>Copiar</span></button></div><pre><code class="language-${escapeHtml(lang || '')}">${highlighted}</code></pre></div>`;
  };

  async function copyTextSafe(text: string): Promise<boolean> {
    try {
      if (navigator.clipboard?.writeText) {
        await navigator.clipboard.writeText(text);
        return true;
      }
    } catch {
      // Fallback below for non-HTTPS LAN origins
    }
    try {
      const ta = document.createElement('textarea');
      ta.value = text;
      ta.style.position = 'fixed';
      ta.style.opacity = '0';
      document.body.appendChild(ta);
      ta.focus();
      ta.select();
      const ok = document.execCommand('copy');
      document.body.removeChild(ta);
      return ok;
    } catch {
      return false;
    }
  }

  async function handleDocumentClick(e: MouseEvent) {
    const target = e.target as HTMLElement | null;
    if (!target || !chatContainer?.contains(target)) return;
    const btn = target.closest('button[data-copy-code]') as HTMLButtonElement | null;
    if (!btn) return;
    e.preventDefault();
    e.stopPropagation();

    const block = btn.closest('.code-block');
    if (!block) return;

    let rawText = '';
    const codeEl = block.querySelector('pre code');
    if (codeEl) {
      rawText = codeEl.textContent ?? '';
    } else {
      const diffEl = block.querySelector('.diff-lines');
      if (diffEl) {
        rawText = Array.from(diffEl.children)
          .map((line) => line.textContent ?? '')
          .join('\n');
      }
    }

    if (!rawText) return;
    const ok = await copyTextSafe(rawText);
    if (!ok) return;

    btn.dataset.copied = 'true';
    btn.innerHTML = `${CHECK_ICON_SVG}<span>Copiado</span>`;
    setTimeout(() => {
      if (!btn.isConnected) return;
      delete btn.dataset.copied;
      btn.innerHTML = `${COPY_ICON_SVG}<span>Copiar</span>`;
    }, 1600);
  }

  async function copyAgentMessage(idx: number, content: string) {
    if (!content.trim()) return;
    const ok = await copyTextSafe(content);
    if (!ok) return;
    copiedMsgIdx = idx;
    if (copyMsgTimer) clearTimeout(copyMsgTimer);
    copyMsgTimer = setTimeout(() => {
      if (copiedMsgIdx === idx) copiedMsgIdx = null;
    }, 1600);
  }

  function renderMarkdown(text: string): string {
    return marked.parse(text || '', { async: false, renderer: mdRenderer }) as string;
  }

  async function handleCancelActiveJob() {
    if (!activeJobId) return;
    const jid = activeJobId;
    liveStatus = 'Cancelando execução...';
    try {
      await cancelJob(jid);
    } catch {
      // ignore
    }
  }

  function startElapsedTimer(initialProgress = 8) {
    stopElapsedTimer();
    if (finishTimer) {
      clearTimeout(finishTimer);
      finishTimer = null;
    }
    liveElapsed = 0;
    toolStepCount = 0;
    deltaCharsCount = 0;
    jobProgress = initialProgress;
    elapsedTimer = setInterval(() => {
      liveElapsed += 1;
      if (deltaCharsCount === 0) {
        // Gentle asymptotic creep while waiting for tools/model startup (never exceeds 82% before completion)
        setProgressFloor(jobProgress + (82 - jobProgress) * 0.045);
      } else {
        setProgressFloor(jobProgress + (96 - jobProgress) * 0.03);
      }
    }, 1000);
  }

  function stopElapsedTimer() {
    if (elapsedTimer) {
      clearInterval(elapsedTimer);
      elapsedTimer = null;
    }
  }

  function clearActiveJob() {
    if (finishTimer) {
      clearTimeout(finishTimer);
      finishTimer = null;
    }
    activeJobId = null;
    activeJobLastSeq = 0;
    localStorage.removeItem('agy_active_job');
    localStorage.removeItem('agy_active_seq');
    if (streamAbort) {
      streamAbort.abort();
      streamAbort = null;
    }
    stopElapsedTimer();
    liveStatus = '';
    jobProgress = 0;
    approvalNeeded = null;
    isGenerating = false;
  }

  async function streamJobEvents(jobId: string, afterSeq = 0) {
    if (streamAbort) streamAbort.abort();
    const controller = new AbortController();
    streamAbort = controller;

    let jobFinished = false;
    let currentTools: string[] = [];

    if (messages.length === 0 || messages[messages.length - 1].role !== 'agent') {
      messages = [...messages, { role: 'agent', content: '' }];
    }
    const agentIdx = messages.length - 1;

    try {
      const response = await fetch(`/api/jobs/${jobId}/events?after=${afterSeq}`, {
        signal: controller.signal,
        credentials: 'same-origin',
      });

      if (!response.ok || !response.body) {
        throw new Error(`HTTP ${response.status}`);
      }

      const reader = response.body.getReader();
      const decoder = new TextDecoder('utf-8');
      let buffer = '';

      while (true) {
        const { done, value } = await reader.read();
        if (done) break;

        buffer += decoder.decode(value, { stream: true });
        const lines = buffer.split('\n');
        buffer = lines.pop() ?? '';

        for (const rawLine of lines) {
          const line = rawLine.trim();
          if (!line || line.startsWith(':')) continue;

          if (line.startsWith('id: ')) {
            const parsedId = parseInt(line.slice(4), 10);
            if (!Number.isNaN(parsedId)) {
              activeJobLastSeq = parsedId;
              localStorage.setItem('agy_active_seq', String(parsedId));
            }
            continue;
          }

          if (!line.startsWith('data: ')) continue;
          const dataStr = line.slice(6);
          if (dataStr === '[DONE]') {
            jobFinished = true;
            break;
          }

          try {
            const ev = JSON.parse(dataStr);
            if (ev.type === 'init') {
              if (ev.conversation_id) {
                conversationId = ev.conversation_id;
                localStorage.setItem('agy_conv_id', ev.conversation_id);
              }
              setProgressFloor(18);
            } else if (ev.type === 'delta') {
              messages[agentIdx].content += ev.text;
              messages = messages;
              deltaCharsCount += String(ev.text || '').length;
              setProgressFloor(56 + 40 * (1 - Math.exp(-deltaCharsCount / 520)));
              liveStatus = 'Digitando resposta...';
              scrollToBottom();
            } else if (ev.type === 'tool') {
              const tName = ev.name || 'ferramenta';
              if (ev.name && !currentTools.includes(ev.name)) {
                currentTools.push(ev.name);
              }
              toolStepCount += ev.state === 'ACTIVE' ? 1 : 0.6;
              setProgressFloor(20 + 54 * (1 - Math.exp(-toolStepCount * 0.42)));
              const preview = ev.preview ? ` (${String(ev.preview).slice(0, 28)}...)` : '';
              liveStatus = ev.state === 'ACTIVE' ? `Executando: ${tName}${preview}` : `Concluído: ${tName}`;
              scrollToBottom();
            } else if (ev.type === 'approval_needed') {
              approvalNeeded = {
                tool: ev.tool || 'permissão',
                command: ev.command || '',
              };
              setProgressFloor(68);
              liveStatus = 'Aguardando aprovação humana (HITL)...';
              scrollToBottom();
            } else if (ev.type === 'done') {
              jobFinished = true;
              jobProgress = 100;
              liveStatus = 'Concluído';
              if (ev.conversation_id) {
                conversationId = ev.conversation_id;
                localStorage.setItem('agy_conv_id', ev.conversation_id);
              }
              if (!messages[agentIdx].content.trim()) {
                if (ev.response && String(ev.response).trim()) {
                  messages[agentIdx].content = String(ev.response).trim();
                } else {
                  const tools = (ev.executed_tools && ev.executed_tools.length) ? ev.executed_tools : currentTools;
                  messages[agentIdx].content = tools.length > 0
                    ? `*Instruções executadas em ${hostLabel} com sucesso.*\n\n**Ações executadas:**\n` + tools.map((t: string) => `- \`${t}\``).join('\n')
                    : `*Instrução concluída com sucesso em ${hostLabel}.*`;
                }
                messages = messages;
              }
              if (Array.isArray(ev.outbox_files) && ev.outbox_files.length > 0) {
                recentOutboxDeliveries = ev.outbox_files;
              }
              refreshStatusAndOutbox();
            } else if (ev.type === 'error') {
              jobFinished = true;
              messages[agentIdx].content += `\n\n> **Erro:** ${ev.error}`;
              messages = messages;
            }
          } catch {
            // ignore malformed JSON chunk
          }
        }
      }
    } catch (err: any) {
      if (err?.name === 'AbortError') return;
      if (document.visibilityState === 'visible' && activeJobId === jobId) {
        liveStatus = 'Reconectando ao stream...';
        setTimeout(() => {
          if (activeJobId === jobId && document.visibilityState === 'visible') {
            streamJobEvents(jobId, activeJobLastSeq);
          }
        }, 1500);
      }
      return;
    }

    if (jobFinished && activeJobId === jobId) {
      if (!messages[agentIdx].content.trim()) {
        messages[agentIdx].content = `*Instrução concluída com sucesso em ${hostLabel}.*`;
        messages = messages;
      }
      jobProgress = 100;
      liveStatus = 'Concluído';
      stopElapsedTimer();
      refreshStatusAndOutbox();
      scrollToBottom(true);
      finishTimer = setTimeout(() => {
        if (activeJobId === jobId) {
          clearActiveJob();
        }
      }, 280);
    }
  }

  async function handleSubmit(e?: Event) {
    if (e) e.preventDefault();
    const rawText = userInput.trim();
    if ((!rawText && attachedFiles.length === 0) || isGenerating) return;

    const wantsOutbox = requestOutbox;
    const text = wantsOutbox
      ? `${rawText || 'Analise os arquivos anexados.'}\n\n[ Importante: Salve ou copie o arquivo final gerado no diretório Outbox (/home/ai/agy-outbox) para download imediato no celular e informe o nome exato do arquivo. ]`
      : rawText;

    const displayPrompt = rawText || `[${attachedFiles.length} arquivo(s) enviado(s)]`;
    const filesToSend = attachedFiles.map((f) => f.path);

    userInput = '';
    requestOutbox = false;
    recentOutboxDeliveries = [];
    attachedFiles = [];
    if (textareaEl) textareaEl.style.height = 'auto';

    messages = [
      ...messages,
      { role: 'user', content: displayPrompt },
      { role: 'agent', content: '' },
    ];
    isGenerating = true;
    liveStatus = gatewayMode === 'standalone' ? 'Iniciando localmente...' : `Iniciando em ${hostLabel}...`;
    startElapsedTimer();
    scrollToBottom(true);

    try {
      const job = await createJob({
        prompt: text || 'Analise os arquivos anexados.',
        conversation_id: conversationId,
        model: selectedModel,
        permission_mode: permissionMode,
        attached_files: filesToSend.length > 0 ? filesToSend : undefined,
      });

      activeJobId = job.job_id;
      activeJobLastSeq = 0;
      localStorage.setItem('agy_active_job', job.job_id);
      localStorage.setItem('agy_active_seq', '0');

      if (job.conversation_id) {
        conversationId = job.conversation_id;
        localStorage.setItem('agy_conv_id', job.conversation_id);
      }

      await streamJobEvents(job.job_id, 0);
    } catch (err: any) {
      const errText = `> **Erro:** ${err?.message || err}`;
      if (messages.length > 0 && messages[messages.length - 1].role === 'agent') {
        messages[messages.length - 1].content = errText;
        messages = messages;
      } else {
        messages = [...messages, { role: 'agent', content: errText }];
      }
      clearActiveJob();
    }
  }

  async function loadConversation(id: string) {
    clearActiveJob();
    recentOutboxDeliveries = [];
    try {
      const data = await fetchConversation(id);
      conversationId = id;
      localStorage.setItem('agy_conv_id', id);
      messages = data.messages || [];
      await scrollToBottom(true);
      setTimeout(() => scrollToBottom(true), 80);

      if (data.active_job_id) {
        activeJobId = data.active_job_id;
        activeJobLastSeq = 0;
        isGenerating = true;
        liveStatus = 'Retomando execução...';
        startElapsedTimer();
        await streamJobEvents(data.active_job_id, 0);
      }
    } catch {
      // ignore if deleted
    }
  }

  function newConversation() {
    clearActiveJob();
    recentOutboxDeliveries = [];
    conversationId = null;
    localStorage.removeItem('agy_conv_id');
    messages = [];
  }

  async function handleFilesSelected(e: Event) {
    const input = e.target as HTMLInputElement;
    if (!input.files || input.files.length === 0) return;
    uploading = true;
    try {
      for (const file of Array.from(input.files)) {
        const res = await uploadFile(file);
        attachedFiles = [...attachedFiles, { name: res.filename, path: res.path }];
      }
    } finally {
      uploading = false;
      input.value = '';
    }
  }

  function toggleVoice() {
    if (!recognition) return;
    if (isListening) recognition.stop();
    else recognition.start();
  }

  function handleVisibilityChange() {
    if (document.visibilityState === 'visible') {
      updateAppViewport();
      refreshStatusAndOutbox();
      if (activeJobId && !streamAbort) {
        liveStatus = 'Retomando stream...';
        streamJobEvents(activeJobId, activeJobLastSeq);
      }
      scrollToBottom(true);
      setTimeout(() => scrollToBottom(true), 120);
    }
  }

  async function refreshStatusAndOutbox() {
    try {
      const st: any = await fetchStatus();
      targetOnline = st.online;
      if (st.active_os) activeOS = st.active_os;
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
    document.addEventListener('visibilitychange', handleVisibilityChange);
    window.addEventListener('pageshow', handleVisibilityChange);
    window.addEventListener('focus', handleVisibilityChange);
    window.addEventListener('scroll', applyViewportNow, { passive: true });
    window.visualViewport?.addEventListener('resize', updateAppViewport);
    window.visualViewport?.addEventListener('scroll', updateAppViewport);
    applyViewportNow();

    // Setup Speech Recognition
    const SpeechRec = (window as any).SpeechRecognition || (window as any).webkitSpeechRecognition;
    if (SpeechRec) {
      recognition = new SpeechRec();
      recognition.lang = 'pt-BR';
      recognition.continuous = true;
      recognition.interimResults = true;
      recognition.onstart = () => (isListening = true);
      recognition.onend = () => (isListening = false);
      recognition.onerror = () => recognition.stop();
      recognition.onresult = (ev: any) => {
        let transcript = '';
        for (let i = ev.resultIndex; i < ev.results.length; i++) {
          transcript += ev.results[i][0].transcript;
        }
        userInput = transcript;
      };
    }

    // Load settings, models, features, status, outbox in parallel
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

    selectedModel = loadedSettings.default_model || 'gemini-4-hydrogen-high';
    permissionMode = loadedSettings.default_permission_mode || 'copilot';

    if (conversationId) {
      await loadConversation(conversationId);
    } else if (activeJobId) {
      isGenerating = true;
      liveStatus = 'Retomando sessão ativa...';
      startElapsedTimer();
      await streamJobEvents(activeJobId, activeJobLastSeq);
    }

    statusInterval = setInterval(refreshStatusAndOutbox, 15000);
  });

  onDestroy(() => {
    document.removeEventListener('visibilitychange', handleVisibilityChange);
    window.removeEventListener('pageshow', handleVisibilityChange);
    window.removeEventListener('focus', handleVisibilityChange);
    window.removeEventListener('scroll', applyViewportNow);
    window.visualViewport?.removeEventListener('resize', updateAppViewport);
    window.visualViewport?.removeEventListener('scroll', updateAppViewport);
    if (viewportSyncRaf !== null) cancelAnimationFrame(viewportSyncRaf);
    if (statusInterval) clearInterval(statusInterval);
    if (finishTimer) clearTimeout(finishTimer);
    if (copyMsgTimer) clearTimeout(copyMsgTimer);
    stopElapsedTimer();
    streamAbort?.abort();
  });
</script>

<svelte:window
  onkeydown={(e) => {
    if (e.key === 'Escape' && themeMenuOpen) themeMenuOpen = false;
  }}
/>
<svelte:document onclick={handleDocumentClick} />

<div
  bind:this={shellEl}
  style="height: 100dvh; will-change: transform, height;"
  class="fixed inset-x-0 top-0 w-full max-w-[100vw] overflow-x-hidden flex flex-col bg-[var(--bg-app)] text-[var(--text-primary)] theme-surface"
>
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
        {#if isGenerating}
          <AiSpinner size={14} class="text-[var(--accent-fg)]" />
        {:else}
          <span class="w-2 h-2 rounded-full shrink-0 transition-colors {targetOnline === true ? 'bg-emerald-500' : targetOnline === false ? 'bg-rose-500' : 'bg-amber-500'}"></span>
        {/if}
        <span class="text-xs font-bold tracking-tight truncate">{hostLabel}</span>
        {#if targetOnline === true && features.dual_boot}
          <span class="px-1.5 py-0.5 rounded bg-[var(--bg-input)] border border-[var(--border-subtle)] text-[10px] font-mono text-[var(--accent-fg)] shrink-0">
            {activeOS === 'windows' ? 'Win' : 'Linux'}
          </span>
        {/if}
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
        onclick={newConversation}
        class="p-2 rounded-xl bg-[var(--accent-soft)] text-[var(--accent-fg)] border border-[var(--accent-border)] hover:opacity-90 transition-all active:scale-95"
        title="Nova Conversa"
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

        <!-- Light / Dark Toggle -->
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

        <!-- Color Palettes -->
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

  <!-- Chat Area -->
  <div class="relative flex-1 min-h-0 flex flex-col">
  <main bind:this={chatContainer} onscroll={handleChatScroll} class="flex-1 overflow-y-auto px-3 sm:px-6 py-4 sm:py-6 overscroll-contain">
    <div class="max-w-3xl lg:max-w-[820px] mx-auto w-full min-h-full flex flex-col space-y-4 sm:space-y-5">
      {#if messages.length === 0}
        <div
          in:fade={{ duration: 220 }}
          class="flex-1 flex flex-col items-center justify-center text-center px-4 py-8 text-[var(--text-faint)] select-none"
        >
          <div class="p-2 rounded-3xl bg-[var(--accent-soft)] border border-[var(--accent-border)] mb-3 shadow-sm">
            <img src="/static/icon.svg" class="w-14 h-14 rounded-2xl" alt="AGY Logo" />
          </div>
          <h1 class="text-sm font-bold text-[var(--text-primary)]">Antigravity Mobile Gateway</h1>
          <p class="text-xs text-[var(--text-muted)] mt-1 max-w-xs leading-relaxed">
            Svelte 5 + SSE resumível. Suporta Gemini 4 Hydrogen, Extra, Ollama local, Inbox/Outbox e Terminal Web.
          </p>
        </div>
      {:else}
        {#each messages as msg, i}
          {@const isActiveBubble = isGenerating && i === messages.length - 1 && msg.role === 'agent'}
          <div class="flex {msg.role === 'user' ? 'justify-end' : 'justify-start'} min-w-0 animate-msg-in">
            {#if msg.role === 'user'}
              <div
                class="max-w-[85%] sm:max-w-[76%] rounded-2xl rounded-br-sm bg-[var(--accent-solid)] text-[var(--accent-on-solid)] px-4 py-2.5 text-sm leading-relaxed whitespace-pre-wrap break-words theme-surface"
                style="box-shadow: var(--shadow-card);"
              >
                {msg.content}
              </div>
            {:else}
              <div
                class="max-w-[96%] sm:max-w-[93%] min-w-0 rounded-2xl rounded-bl-sm bg-[var(--bg-elevated)] border border-[var(--border-subtle)] text-[var(--text-primary)] px-4 py-3.5 sm:px-6 sm:py-5 theme-surface"
                style="box-shadow: var(--shadow-card);"
              >
                {#if isActiveBubble && !msg.content.trim()}
                  <div class="flex items-center gap-2.5 py-0.5 min-w-[205px] sm:min-w-[240px]">
                    <AiSpinner size={20} class="text-[var(--accent-fg)]" />
                    <div class="min-w-0 flex-1 space-y-1.5">
                      <div class="flex items-center justify-between gap-2.5">
                        <span class="text-xs font-medium text-[var(--text-secondary)] truncate">
                          {liveStatus || 'Processando solicitação...'}
                        </span>
                        <span class="text-[10px] font-mono font-semibold text-[var(--accent-fg)] shrink-0">
                          {Math.round(jobProgress)}%
                        </span>
                      </div>
                      <div class="w-full h-1.5 rounded-full progress-track overflow-hidden">
                        <div
                          class="h-full rounded-full progress-fill"
                          style="width: {jobProgress}%;"
                        ></div>
                      </div>
                    </div>
                  </div>
                {:else}
                  <div class="markdown-body {isActiveBubble ? 'typing-cursor' : ''}">
                    {@html renderMarkdown(msg.content)}
                  </div>
                  {#if isActiveBubble}
                    <div class="mt-2.5 pt-2 border-t border-[var(--border-subtle)] space-y-1.5 text-[11px] text-[var(--text-muted)]">
                      <div class="flex items-center justify-between gap-3">
                        <div class="flex items-center gap-2 min-w-0">
                          <AiSpinner size={14} class="text-[var(--accent-fg)]" />
                          <span class="truncate">{liveStatus || 'Gerando resposta...'}</span>
                        </div>
                        <span class="font-mono text-[var(--text-faint)] shrink-0">
                          {Math.round(jobProgress)}% · {liveElapsed}s
                        </span>
                      </div>
                      <div class="w-full h-1 rounded-full progress-track overflow-hidden">
                        <div
                          class="h-full rounded-full progress-fill"
                          style="width: {jobProgress}%;"
                        ></div>
                      </div>
                    </div>
                  {:else if msg.content.trim()}
                    <div class="mt-4 pt-2.5 border-t border-[var(--border-subtle)]/70 flex items-center justify-end">
                      <button
                        type="button"
                        onclick={() => copyAgentMessage(i, msg.content)}
                        class="px-2 py-0.5 rounded-lg border text-[11px] font-medium flex items-center gap-1.5 transition-all active:scale-95 {copiedMsgIdx === i ? 'bg-[var(--accent-soft)] border-[var(--accent-border)] text-[var(--accent-fg)]' : 'bg-[var(--bg-input)] border-[var(--border-subtle)] text-[var(--text-muted)] hover:text-[var(--accent-fg)] hover:border-[var(--border-strong)]'}"
                        title="Copiar resposta"
                      >
                        <Icon name={copiedMsgIdx === i ? 'check' : 'clipboard'} size={12} />
                        <span>{copiedMsgIdx === i ? 'Copiado' : 'Copiar'}</span>
                      </button>
                    </div>
                  {/if}
                {/if}
              </div>
            {/if}
          </div>
        {/each}
      {/if}
    </div>
  </main>

    <!-- Floating Auto-Move / Scroll-to-Bottom Button -->
    {#if showScrollBottomBtn}
      <button
        type="button"
        transition:fly={{ y: 12, duration: 180, easing: cubicOut }}
        onclick={() => scrollToBottom(true, true)}
        class="absolute bottom-3 right-3 z-20 px-3 py-1.5 rounded-full bg-[var(--bg-elevated)] hover:bg-[var(--bg-input)] border border-[var(--accent-border)] text-[var(--accent-fg)] shadow-lg backdrop-blur-sm text-xs font-medium flex items-center gap-1.5"
        title="Ir para o fim da conversa"
      >
        <Icon name="chevron-down" size={15} />
        <span>Fim da conversa</span>
      </button>
    {/if}
  </div>

  <!-- Inline Outbox Ready Banner -->
  {#if recentOutboxDeliveries.length > 0}
    <div
      transition:fly={{ y: 12, duration: 200, easing: cubicOut }}
      class="mx-2.5 sm:mx-auto sm:max-w-3xl lg:max-w-[820px] sm:w-full mb-2 p-2.5 rounded-xl bg-[var(--accent-soft)] border border-[var(--accent-border)] text-[var(--text-primary)] space-y-1.5 shrink-0"
    >
      <div class="flex items-center justify-between gap-2 text-xs font-semibold text-[var(--accent-fg)]">
        <div class="flex items-center gap-1.5">
          <Icon name="outbox" size={14} />
          <span>Arquivo(s) pronto(s) no Outbox para download</span>
        </div>
        <button
          type="button"
          onclick={() => (recentOutboxDeliveries = [])}
          class="text-[var(--text-muted)] hover:text-[var(--text-primary)] p-0.5"
          aria-label="Fechar aviso"
        >
          <Icon name="close" size={13} />
        </button>
      </div>
      <div class="flex flex-wrap gap-1.5">
        {#each recentOutboxDeliveries as fname}
          <a
            href={`/api/outbox/${encodeURIComponent(fname)}`}
            download={fname}
            class="px-2.5 py-1 rounded-lg bg-[var(--accent-solid)] hover:bg-[var(--accent-hover)] text-[var(--accent-on-solid)] text-xs font-semibold flex items-center gap-1.5"
          >
            <Icon name="download" size={13} />
            <span class="truncate max-w-[200px]">{fname}</span>
          </a>
        {/each}
      </div>
    </div>
  {/if}

  <!-- Centered Bottom Dock (HITL Card + Slash Palette + Composer Card) -->
  <div class="w-full px-2.5 sm:px-6 pb-2.5 sm:pb-4 pt-1 shrink-0">
    <div class="max-w-3xl lg:max-w-[820px] mx-auto w-full space-y-2">
      <!-- Interactive HITL Approval Card -->
      {#if approvalNeeded}
        <div
          transition:fly={{ y: 12, duration: 180, easing: cubicOut }}
          class="p-3 rounded-2xl bg-amber-500/12 border border-amber-500/40 text-[var(--text-primary)] space-y-2"
        >
          <div class="flex items-center justify-between gap-2 text-xs font-semibold text-amber-500">
            <div class="flex items-center gap-1.5">
              <Icon name="shield" size={15} />
              <span>Aprovação HITL Necessária ({approvalNeeded.tool})</span>
            </div>
          </div>
          {#if approvalNeeded.command}
            <pre class="bg-[var(--bg-input)] border border-[var(--border-subtle)] rounded-lg p-2 text-[11px] font-mono text-[var(--text-primary)] overflow-x-auto">{approvalNeeded.command}</pre>
          {/if}
          <div class="flex items-center gap-2">
            {#if features.terminal}
              <button
                type="button"
                onclick={async () => {
                  if (approvalNeeded?.command) {
                    await navigator.clipboard.writeText(approvalNeeded.command).catch(() => {});
                  }
                  terminalOpen = true;
                }}
                class="flex-1 py-1.5 px-2.5 rounded-lg bg-emerald-600 hover:bg-emerald-500 text-white text-xs font-semibold flex items-center justify-center gap-1.5 transition-all active:scale-95"
              >
                <Icon name="terminal" size={14} />
                <span>Copiar & Abrir Terminal</span>
              </button>
            {/if}
            <button
              type="button"
              onclick={handleCancelActiveJob}
              class="py-1.5 px-3 rounded-lg bg-rose-500/20 hover:bg-rose-500/30 text-rose-400 border border-rose-500/40 text-xs font-semibold transition-all active:scale-95"
            >
              Rejeitar
            </button>
          </div>
        </div>
      {/if}

      <!-- Slash Commands Palette -->
      {#if slashMenuOpen || userInput.startsWith('/')}
        <div
          transition:fly={{ y: 8, duration: 150, easing: cubicOut }}
          class="bg-[var(--bg-elevated)] border border-[var(--border-subtle)] rounded-2xl p-1.5 space-y-1 max-h-48 overflow-y-auto"
          style="box-shadow: var(--shadow-popover);"
        >
          {#each slashCommands.filter((c) => !userInput.startsWith('/') || c.cmd.startsWith(userInput.trim().toLowerCase())) as item}
            <button
              type="button"
              onclick={() => runSlashCommand(item.cmd)}
              class="w-full text-left px-2.5 py-1.5 rounded-xl hover:bg-[var(--bg-chip)] flex items-center justify-between gap-2 text-xs transition-colors"
            >
              <span class="font-mono font-semibold text-[var(--accent-fg)]">{item.cmd}</span>
              <span class="text-[var(--text-muted)] text-[11px] truncate">{item.desc}</span>
            </button>
          {/each}
        </div>
      {/if}

      <!-- Composer Card -->
      <footer
        class="rounded-2xl bg-[var(--bg-elevated)] border border-[var(--border-subtle)] overflow-hidden theme-surface"
        style="box-shadow: var(--shadow-card);"
      >
        <!-- Live Status Bar (integrated into top of composer card) -->
        {#if isGenerating && liveStatus}
          <div
            transition:fly={{ y: 6, duration: 150, easing: cubicOut }}
            class="bg-[var(--bg-input)] border-b border-[var(--border-subtle)] theme-surface"
          >
            <div class="w-full h-[3px] progress-track overflow-hidden">
              <div
                class="h-full progress-fill"
                style="width: {jobProgress}%;"
              ></div>
            </div>
            <div class="px-3 py-1.5 flex items-center justify-between text-[11px] text-[var(--accent-fg)] font-mono">
              <div class="flex items-center gap-2 min-w-0">
                <AiSpinner size={14} class="text-[var(--accent-fg)]" />
                <span class="truncate">{liveStatus}</span>
              </div>
              <div class="flex items-center gap-2 shrink-0">
                <span class="text-[var(--text-secondary)] font-semibold">{Math.round(jobProgress)}%</span>
                <span class="text-[var(--text-faint)]">{liveElapsed}s</span>
                <button
                  type="button"
                  onclick={handleCancelActiveJob}
                  class="px-2 py-0.5 rounded-md bg-rose-500/15 text-rose-400 border border-rose-500/35 hover:bg-rose-500/25 text-[10px] font-semibold flex items-center gap-1 transition-all active:scale-95"
                >
                  <Icon name="stop" size={10} />
                  <span>Cancelar</span>
                </button>
              </div>
            </div>
          </div>
        {/if}

        <div class="p-2 sm:p-2.5 space-y-2">
          <!-- Compact Model, Permission & Quick Actions Bar -->
          <div class="flex flex-wrap items-center justify-between gap-1.5">
            <div class="flex items-center gap-1.5 min-w-0 flex-1 sm:flex-initial">
              <select
                bind:value={selectedModel}
                class="min-w-0 flex-1 sm:flex-none sm:w-52 bg-[var(--bg-input)] border border-[var(--border-subtle)] rounded-lg px-2 py-1 text-[11px] text-[var(--text-primary)] focus:outline-none focus:border-[var(--accent-fg)] truncate transition-colors"
              >
                {#each models as m}
                  <option value={m.id}>{m.name} ({m.badge})</option>
                {/each}
              </select>

              <select
                bind:value={permissionMode}
                class="bg-[var(--bg-input)] border border-[var(--border-subtle)] rounded-lg px-2 py-1 text-[11px] font-medium focus:outline-none focus:border-[var(--accent-fg)] shrink-0 transition-colors {permissionMode === 'autonomous' ? 'text-amber-500 border-amber-500/40' : permissionMode === 'readonly' ? 'text-[var(--accent-fg)]' : 'text-emerald-500'}"
              >
                <option value="copilot">Copiloto</option>
                <option value="autonomous">Autônomo</option>
                <option value="readonly">Leitura</option>
              </select>
            </div>

            <div class="flex items-center gap-1 shrink-0">
              <button
                type="button"
                onclick={() => (slashMenuOpen = !slashMenuOpen)}
                class="px-2 py-1 rounded-lg border text-[11px] font-medium flex items-center gap-1 transition-all active:scale-95 {slashMenuOpen ? 'bg-[var(--accent-solid)] border-[var(--accent-solid)] text-[var(--accent-on-solid)]' : 'bg-[var(--bg-input)] border-[var(--border-subtle)] text-[var(--accent-fg)] hover:border-[var(--border-strong)]'}"
                title="Comandos Slash (/)"
              >
                <span class="font-mono font-semibold">/</span>
                <span>Comandos</span>
              </button>

              <input
                bind:this={fileInputEl}
                type="file"
                multiple
                onchange={handleFilesSelected}
                class="hidden"
              />
              <button
                type="button"
                onclick={() => fileInputEl?.click()}
                disabled={uploading}
                class="px-2 py-1 rounded-lg bg-[var(--bg-input)] border border-[var(--border-subtle)] text-[var(--text-secondary)] hover:text-[var(--accent-fg)] hover:border-[var(--border-strong)] text-[11px] font-medium flex items-center gap-1 transition-all active:scale-95"
                title="Anexar arquivo ou foto"
              >
                <Icon name="paperclip" size={13} />
                <span>Anexar</span>
              </button>

              {#if recognition}
                <button
                  type="button"
                  onclick={toggleVoice}
                  class="px-2 py-1 rounded-lg border text-[11px] font-medium flex items-center gap-1 transition-all active:scale-95 {isListening ? 'bg-rose-600 border-rose-600 text-white animate-pulse' : 'bg-[var(--bg-input)] border-[var(--border-subtle)] text-[var(--text-secondary)] hover:text-[var(--accent-fg)] hover:border-[var(--border-strong)]'}"
                  title="Ditado por voz"
                >
                  <Icon name="mic" size={13} />
                  <span>{isListening ? 'Ouvindo...' : 'Voz'}</span>
                </button>
              {/if}
            </div>
          </div>

          <!-- Attached File Chips -->
          {#if attachedFiles.length > 0}
            <div class="flex flex-wrap gap-1.5">
              {#each attachedFiles as f, idx}
                <span
                  transition:fade={{ duration: 140 }}
                  class="inline-flex items-center gap-1 px-2 py-0.5 rounded-full bg-[var(--accent-soft)] border border-[var(--accent-border)] text-[11px] text-[var(--accent-fg)]"
                >
                  <Icon name="file" size={11} />
                  <span class="max-w-[130px] truncate">{f.name}</span>
                  <button
                    type="button"
                    onclick={() => (attachedFiles = attachedFiles.filter((_, k) => k !== idx))}
                    class="text-[var(--text-muted)] hover:text-rose-400 transition-colors"
                  >
                    <Icon name="close" size={11} />
                  </button>
                </span>
              {/each}
            </div>
          {/if}

          <!-- Main Input Form -->
          <form onsubmit={handleSubmit} class="flex items-end gap-1.5">
            <textarea
              bind:this={textareaEl}
              bind:value={userInput}
              rows={1}
              placeholder={uploading ? 'Enviando arquivo...' : gatewayMode === 'standalone' ? 'Digite uma instrução...' : `Instrução para ${hostLabel}...`}
              onfocus={scheduleKeyboardScroll}
              oninput={(e) => {
                const el = e.currentTarget;
                el.style.height = 'auto';
                el.style.height = `${Math.min(el.scrollHeight, 120)}px`;
              }}
              onkeydown={(e) => {
                if (e.key === 'Enter' && !e.shiftKey) {
                  e.preventDefault();
                  handleSubmit();
                }
              }}
              class="flex-1 bg-[var(--bg-input)] border border-[var(--border-subtle)] rounded-xl px-3 py-2 text-base sm:text-sm text-[var(--text-primary)] placeholder-[var(--text-faint)] focus:outline-none focus:border-[var(--accent-fg)] resize-none max-h-[120px] transition-colors"
            ></textarea>

            {#if isGenerating && activeJobId}
              <button
                type="button"
                onclick={handleCancelActiveJob}
                class="p-2 sm:p-2.5 rounded-xl bg-rose-600 hover:bg-rose-500 text-white shrink-0 transition-all active:scale-95"
                title="Cancelar execução"
                aria-label="Cancelar"
              >
                <Icon name="stop" size={18} />
              </button>
            {:else}
              <button
                type="submit"
                disabled={isGenerating || (!userInput.trim() && attachedFiles.length === 0)}
                class="p-2 sm:p-2.5 rounded-xl bg-[var(--accent-solid)] hover:bg-[var(--accent-hover)] disabled:opacity-40 text-[var(--accent-on-solid)] shrink-0 transition-all active:scale-95"
                aria-label="Enviar"
              >
                <Icon name="send" size={18} />
              </button>
            {/if}
          </form>
        </div>
      </footer>
    </div>
  </div>
</div>

<Sidebar
  bind:open={sidebarOpen}
  activeConvId={conversationId}
  {models}
  watchdogEnabled={features.watchdog !== false}
  telemetryEnabled={features.telemetry === true}
  {themeMode}
  {themePalette}
  bind:settings={settings}
  onSelectConversation={loadConversation}
  onNewConversation={newConversation}
  onThemeChange={setTheme}
  onSettingsSaved={(s) => {
    permissionMode = s.default_permission_mode;
    selectedModel = s.default_model;
  }}
/>

<TerminalModal
  bind:open={terminalOpen}
  hostLabel={gatewayMode === 'standalone' ? 'Terminal Local' : 'ryanslx@CachyCore'}
/>
<OutboxModal bind:open={outboxOpen} bind:files={outboxFiles} {outboxDir} />
