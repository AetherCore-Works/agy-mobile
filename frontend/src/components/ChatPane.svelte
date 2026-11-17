<script lang="ts">
  import { onDestroy, onMount, tick, untrack } from 'svelte';
  import { fade, fly } from 'svelte/transition';
  import { cubicOut } from 'svelte/easing';
  import Icon from './Icon.svelte';
  import AiSpinner from './AiSpinner.svelte';
  import {
    cancelJob,
    createJob,
    fetchConversation,
    fetchFeatures,
    fetchWatchdog,
    uploadFile,
    type ChatMessage,
    type ModelInfo,
    type PermissionMode,
  } from '../lib/api';
  import {
    CHECK_ICON_SVG,
    COPY_ICON_SVG,
    copyTextSafe,
    extractCodeFromCopyButton,
    renderMarkdown,
  } from '../lib/markdown';

  let {
    tabId,
    storageKeyPrefix = 'agy_',
    visibilityMode = 'active',
    paneOrderClass = '',
    isSplitView = false,
    isFocused = true,
    models,
    gatewayMode,
    hostLabel,
    features,
    defaultModel,
    defaultPermission,
    onOpenShare,
    onOpenTerminal,
    onFocus,
    onMetaChange,
    onGeneratingChange,
  }: {
    tabId: string;
    storageKeyPrefix?: string;
    visibilityMode?: 'active' | 'split-hidden-mobile' | 'hidden';
    paneOrderClass?: string;
    isSplitView?: boolean;
    isFocused?: boolean;
    models: ModelInfo[];
    gatewayMode: 'gateway' | 'standalone' | 'ide-bridge';
    hostLabel: string;
    features: Record<string, any>;
    defaultModel: string;
    defaultPermission: PermissionMode;
    onOpenShare: (title: string, singleMarkdown: string, messages: ChatMessage[]) => void;
    onOpenTerminal: () => void;
    onFocus: () => void;
    onMetaChange?: (tabId: string, title: string, convId: string | null) => void;
    onGeneratingChange?: (tabId: string, generating: boolean) => void;
  } = $props();

  const prefix = untrack(() => storageKeyPrefix);
  const convStorageKey = `${prefix}conv_id`;
  const jobStorageKey = `${prefix}active_job`;
  const seqStorageKey = `${prefix}active_seq`;

  let selectedModel = $state(untrack(() => defaultModel));
  let permissionMode = $state<PermissionMode>(untrack(() => defaultPermission));
  let userTouchedModel = false;
  let userTouchedPerm = false;

  $effect(() => {
    if (!userTouchedModel && defaultModel) selectedModel = defaultModel;
  });
  $effect(() => {
    if (!userTouchedPerm && defaultPermission) permissionMode = defaultPermission;
  });

  let conversationId = $state<string | null>(localStorage.getItem(convStorageKey));
  let sessionTitle = $state('');
  let messages = $state<ChatMessage[]>([]);
  let userInput = $state('');
  let attachedFiles = $state<{ name: string; path: string }[]>([]);
  let uploading = $state(false);

  let isGenerating = $state(false);
  let activeJobId = $state<string | null>(localStorage.getItem(jobStorageKey));
  let activeJobLastSeq = $state<number>(Number(localStorage.getItem(seqStorageKey) || '0'));
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

  let isListening = $state(false);
  let recognition = $state<any>(null);
  let slashMenuOpen = $state(false);
  let approvalNeeded = $state<{ tool: string; command: string } | null>(null);

  let paneEl: HTMLElement | undefined = $state();
  let chatContainer: HTMLElement | undefined = $state();
  let fileInputEl: HTMLInputElement | undefined = $state();
  let textareaEl: HTMLTextAreaElement | undefined = $state();

  const slashCommands = [
    { cmd: '/usage', desc: 'Cota restante de tokens/modelos' },
    { cmd: '/compact', desc: 'Compactar histórico da sessão' },
    { cmd: '/diff', desc: 'Exibir git status e diff atual' },
    { cmd: '/mem', desc: 'Consultar memória persistente (ai-memory)' },
    { cmd: '/plan', desc: 'Ativar modo Planejamento (Leitura)' },
    { cmd: '/clear', desc: 'Limpar e iniciar nova conversa' },
    { cmd: '/status', desc: 'Status do Watchdog & recursos' },
  ];

  $effect(() => {
    onGeneratingChange?.(tabId, isGenerating);
  });

  function deriveTitle(msgs: ChatMessage[], explicitTitle?: string): string {
    if (explicitTitle?.trim()) return explicitTitle.trim();
    const firstUser = msgs.find((m) => m.role === 'user')?.content.trim();
    if (!firstUser) return '';
    return firstUser.length > 44 ? firstUser.slice(0, 44) + '...' : firstUser;
  }

  function notifyMeta(explicitTitle?: string) {
    const t = deriveTitle(messages, explicitTitle ?? sessionTitle);
    sessionTitle = t;
    onMetaChange?.(tabId, t, conversationId);
  }

  function setProgressFloor(target: number) {
    const clamped = Math.max(0, Math.min(100, target));
    if (clamped > jobProgress) jobProgress = clamped;
  }

  async function scrollToBottom() {
    await tick();
    if (chatContainer) {
      chatContainer.scrollTop = chatContainer.scrollHeight;
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
    localStorage.removeItem(jobStorageKey);
    localStorage.removeItem(seqStorageKey);
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
              localStorage.setItem(seqStorageKey, String(parsedId));
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
                localStorage.setItem(convStorageKey, ev.conversation_id);
                notifyMeta();
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
                localStorage.setItem(convStorageKey, ev.conversation_id);
                notifyMeta();
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
      scrollToBottom();
      finishTimer = setTimeout(() => {
        if (activeJobId === jobId) {
          clearActiveJob();
        }
      }, 280);
    }
  }

  async function submitPromptInternal(promptText: string, filesToSend: string[] = []) {
    const text = promptText.trim();
    if ((!text && filesToSend.length === 0) || isGenerating) return;

    const displayPrompt = text || `[${filesToSend.length} arquivo(s) enviado(s)]`;
    messages = [
      ...messages,
      { role: 'user', content: displayPrompt },
      { role: 'agent', content: '' },
    ];
    notifyMeta();
    isGenerating = true;
    liveStatus = gatewayMode === 'standalone' ? 'Iniciando localmente...' : `Iniciando em ${hostLabel}...`;
    startElapsedTimer();
    scrollToBottom();

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
      localStorage.setItem(jobStorageKey, job.job_id);
      localStorage.setItem(seqStorageKey, '0');

      if (job.conversation_id) {
        conversationId = job.conversation_id;
        localStorage.setItem(convStorageKey, job.conversation_id);
        notifyMeta();
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

  async function handleSubmit(e?: Event) {
    if (e) e.preventDefault();
    const text = userInput.trim();
    if ((!text && attachedFiles.length === 0) || isGenerating) return;
    const filesToSend = attachedFiles.map((f) => f.path);

    userInput = '';
    attachedFiles = [];
    if (textareaEl) textareaEl.style.height = 'auto';

    await submitPromptInternal(text, filesToSend);
  }

  async function runSlashCommand(cmd: string) {
    slashMenuOpen = false;
    userInput = '';
    if (cmd === '/clear') {
      newConversation();
      return;
    }
    if (cmd === '/plan') {
      userTouchedPerm = true;
      permissionMode = 'readonly';
      messages = [...messages, { role: 'agent', content: '**Modo Planejamento (`readonly`) ativado.** Nenhuma alteração será aplicada sem revisão.' }];
      scrollToBottom();
      return;
    }
    if (cmd === '/diff') {
      await submitPromptInternal('Execute git status -s e git diff no workspace atual e exiba a saída dentro de um bloco ```diff.');
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
      scrollToBottom();
      return;
    }
    if (cmd === '/compact' && !conversationId) {
      messages = [...messages, { role: 'agent', content: '> Não há conversa ativa para compactar. Envie uma mensagem ou carregue uma sessão primeiro.' }];
      scrollToBottom();
      return;
    }
    await submitPromptInternal(cmd);
  }

  export async function loadConversation(id: string, titleHint?: string) {
    clearActiveJob();
    try {
      const data = await fetchConversation(id);
      conversationId = id;
      localStorage.setItem(convStorageKey, id);
      messages = data.messages || [];
      sessionTitle = data.title || titleHint || '';
      notifyMeta(sessionTitle);
      await scrollToBottom();

      if (data.active_job_id) {
        activeJobId = data.active_job_id;
        activeJobLastSeq = 0;
        isGenerating = true;
        liveStatus = 'Retomando execução...';
        startElapsedTimer(16);
        await streamJobEvents(data.active_job_id, 0);
      }
    } catch {
      // ignore if deleted
    }
  }

  export function newConversation() {
    clearActiveJob();
    conversationId = null;
    sessionTitle = '';
    localStorage.removeItem(convStorageKey);
    messages = [];
    notifyMeta('');
  }

  export async function submitExternalPrompt(promptText: string) {
    await submitPromptInternal(promptText);
  }

  export function getSnapshotContext(): { title: string; messages: ChatMessage[]; convId: string | null } {
    return {
      title: sessionTitle || deriveTitle(messages) || 'Conversa AGY',
      messages,
      convId: conversationId,
    };
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

  async function handleDocumentClick(e: MouseEvent) {
    const target = e.target as HTMLElement | null;
    if (!target || !chatContainer?.contains(target)) return;
    const btn = target.closest('button[data-copy-code]') as HTMLButtonElement | null;
    if (!btn) return;
    e.preventDefault();
    e.stopPropagation();

    const rawText = extractCodeFromCopyButton(btn);
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

  function handleVisibilityChange() {
    if (document.visibilityState === 'visible' && activeJobId && !streamAbort) {
      liveStatus = 'Retomando stream...';
      streamJobEvents(activeJobId, activeJobLastSeq);
    }
  }

  onMount(async () => {
    document.addEventListener('visibilitychange', handleVisibilityChange);

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

    if (conversationId) {
      await loadConversation(conversationId);
    } else if (activeJobId) {
      isGenerating = true;
      liveStatus = 'Retomando sessão ativa...';
      startElapsedTimer(16);
      await streamJobEvents(activeJobId, activeJobLastSeq);
    }
  });

  onDestroy(() => {
    document.removeEventListener('visibilitychange', handleVisibilityChange);
    if (finishTimer) clearTimeout(finishTimer);
    if (copyMsgTimer) clearTimeout(copyMsgTimer);
    stopElapsedTimer();
    streamAbort?.abort();
  });
</script>

<svelte:document
  onclick={handleDocumentClick}
  onpointerdown={(e) => {
    const target = e.target as HTMLElement | null;
    if (target && paneEl?.contains(target)) onFocus();
  }}
/>

<section
  bind:this={paneEl}
  onfocusin={onFocus}
  class="{visibilityMode === 'active' ? 'flex' : visibilityMode === 'split-hidden-mobile' ? 'hidden md:flex' : 'hidden'} flex-col flex-1 min-w-0 min-h-0 h-full {paneOrderClass} {isSplitView ? 'rounded-2xl border overflow-hidden bg-[var(--bg-app)] theme-surface ' + (isFocused ? 'border-[var(--accent-border)]' : 'border-[var(--border-subtle)]') : ''}"
>
  {#if isSplitView}
    <!-- Split Pane Mini Header -->
    <div class="h-9 px-3 bg-[var(--bg-surface)] border-b border-[var(--border-subtle)] flex items-center justify-between gap-2 shrink-0 select-none">
      <div class="flex items-center gap-2 min-w-0">
        {#if isGenerating}
          <AiSpinner size={12} class="text-[var(--accent-fg)]" />
        {:else}
          <span class="w-1.5 h-1.5 rounded-full shrink-0 {isFocused ? 'bg-[var(--accent-fg)]' : 'bg-[var(--text-faint)]'}"></span>
        {/if}
        <span class="text-[11px] font-semibold text-[var(--text-primary)] truncate">
          {sessionTitle || 'Nova Sessão'}
        </span>
        <span class="text-[10px] font-mono text-[var(--text-faint)] truncate hidden lg:inline">
          ({selectedModel.replace('ollama:', '')})
        </span>
      </div>

      <div class="flex items-center gap-1 shrink-0">
        {#if messages.length > 0}
          <button
            type="button"
            onclick={() => onOpenShare(sessionTitle || 'Conversa AGY', '', messages)}
            class="p-1 rounded-md text-[var(--text-muted)] hover:text-[var(--accent-fg)] transition-colors"
            title="Compartilhar conversa"
          >
            <Icon name="share" size={13} />
          </button>
        {/if}
        <button
          type="button"
          onclick={newConversation}
          class="p-1 rounded-md text-[var(--text-muted)] hover:text-[var(--accent-fg)] transition-colors"
          title="Limpar painel"
        >
          <Icon name="plus" size={13} />
        </button>
      </div>
    </div>
  {/if}

  <!-- Chat Area -->
  <main bind:this={chatContainer} class="flex-1 overflow-y-auto px-3 {isSplitView ? 'sm:px-4 py-3 sm:py-4' : 'sm:px-6 py-4 sm:py-6'} overscroll-contain">
    <div class="{isSplitView ? 'w-full' : 'max-w-3xl lg:max-w-[820px] mx-auto w-full'} min-h-full flex flex-col space-y-4 sm:space-y-5">
      {#if messages.length === 0}
        <div
          in:fade={{ duration: 220 }}
          class="flex-1 flex flex-col items-center justify-center text-center px-4 py-8 text-[var(--text-faint)] select-none"
        >
          <div class="p-2 rounded-3xl bg-[var(--accent-soft)] border border-[var(--accent-border)] mb-3 shadow-sm">
            <img src="/static/icon.svg" class="w-12 h-12 sm:w-14 sm:h-14 rounded-2xl" alt="AGY Logo" />
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
                class="max-w-[85%] sm:max-w-[78%] rounded-2xl rounded-br-sm bg-[var(--accent-solid)] text-[var(--accent-on-solid)] px-4 py-2.5 text-sm leading-relaxed whitespace-pre-wrap break-words theme-surface"
                style="box-shadow: var(--shadow-card);"
              >
                {msg.content}
              </div>
            {:else}
              <div
                class="max-w-[96%] sm:max-w-[93%] min-w-0 rounded-2xl rounded-bl-sm bg-[var(--bg-elevated)] border border-[var(--border-subtle)] text-[var(--text-primary)] px-4 py-3.5 {isSplitView ? 'sm:px-4 sm:py-3.5' : 'sm:px-6 sm:py-5'} theme-surface"
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
                    <div class="mt-4 pt-2.5 border-t border-[var(--border-subtle)]/70 flex flex-wrap items-center justify-end gap-1.5">
                      <button
                        type="button"
                        onclick={() => onOpenShare(sessionTitle || 'Snapshot AGY', msg.content, messages)}
                        class="px-2 py-0.5 rounded-lg border bg-[var(--bg-input)] border-[var(--border-subtle)] text-[var(--text-muted)] hover:text-[var(--accent-fg)] hover:border-[var(--border-strong)] text-[11px] font-medium flex items-center gap-1.5 transition-all active:scale-95"
                        title="Compartilhar / Exportar snapshot"
                      >
                        <Icon name="share" size={12} />
                        <span>Compartilhar</span>
                      </button>

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

  <!-- Centered Bottom Dock (HITL Card + Slash Palette + Composer Card) -->
  <div class="w-full px-2.5 {isSplitView ? 'sm:px-3 pb-2.5' : 'sm:px-6 pb-2.5 sm:pb-4'} pt-1 shrink-0">
    <div class="{isSplitView ? 'w-full' : 'max-w-3xl lg:max-w-[820px] mx-auto w-full'} space-y-2">
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
                  onOpenTerminal();
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
                value={selectedModel}
                onchange={(e) => {
                  userTouchedModel = true;
                  selectedModel = (e.target as HTMLSelectElement).value;
                }}
                class="min-w-0 flex-1 sm:flex-none {isSplitView ? 'sm:w-40' : 'sm:w-52'} bg-[var(--bg-input)] border border-[var(--border-subtle)] rounded-lg px-2 py-1 text-[11px] text-[var(--text-primary)] focus:outline-none focus:border-[var(--accent-fg)] truncate transition-colors"
              >
                {#each models as m}
                  <option value={m.id}>{m.name} ({m.badge})</option>
                {/each}
              </select>

              <select
                value={permissionMode}
                onchange={(e) => {
                  userTouchedPerm = true;
                  permissionMode = (e.target as HTMLSelectElement).value as PermissionMode;
                }}
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
              class="flex-1 bg-[var(--bg-input)] border border-[var(--border-subtle)] rounded-xl px-3 py-2 text-sm text-[var(--text-primary)] placeholder-[var(--text-faint)] focus:outline-none focus:border-[var(--accent-fg)] resize-none max-h-[120px] transition-colors"
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
</section>
