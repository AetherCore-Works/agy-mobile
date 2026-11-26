<script lang="ts">
  import { onDestroy, onMount, tick, untrack } from "svelte";
  import { fade, fly } from "svelte/transition";
  import { cubicOut } from "svelte/easing";
  import Icon from "./Icon.svelte";
  import AiSpinner from "./AiSpinner.svelte";
  import OutboxInlinePreview from "./OutboxInlinePreview.svelte";
  import type { PreviewTarget } from "./OutboxPreviewModal.svelte";
  import { Button } from "../lib/components/ui/button/index.js";
  import { Card } from "../lib/components/ui/card/index.js";
  import { Badge } from "../lib/components/ui/badge/index.js";
  import { Textarea } from "../lib/components/ui/textarea/index.js";
  import { Kbd } from "../lib/components/ui/kbd/index.js";
  import GridPattern from "../lib/components/magic/GridPattern.svelte";
  import {
    cancelJob,
    createJob,
    fetchConversation,
    fetchFeatures,
    fetchWatchdog,
    submitFeedback,
    transcribeAudio,
    uploadFile,
    type ChatMessage,
    type EvolutionProposal,
    type ModelInfo,
    type OutboxFile,
    type PermissionMode,
    type SlashCommandItem,
  } from "../lib/api";
  import {
    CHECK_ICON_SVG,
    COPY_ICON_SVG,
    copyTextSafe,
    extractCodeFromCopyButton,
    renderMarkdown,
  } from "../lib/markdown";

  let {
    tabId,
    storageKeyPrefix = "agy_",
    visibilityMode = "active",
    paneOrderClass = "",
    isSplitView = false,
    isFocused = true,
    models,
    gatewayMode,
    hostLabel,
    features,
    defaultModel,
    defaultPermission,
    slashCommands = [],
    outboxFiles = [],
    outboxLoaded = false,
    pendingProposals = [],
    onOpenShare,
    onRequestDelete,
    onOpenTerminal,
    onOpenSlashManager,
    onOpenEvolution,
    onApproveProposal,
    onRejectProposal,
    onOutboxRefresh,
    onOpenMediaPreview,
    onFocus,
    onMetaChange,
    onGeneratingChange,
  }: {
    tabId: string;
    storageKeyPrefix?: string;
    visibilityMode?: "active" | "split-hidden-mobile" | "hidden";
    paneOrderClass?: string;
    isSplitView?: boolean;
    isFocused?: boolean;
    models: ModelInfo[];
    gatewayMode: "gateway" | "standalone" | "ide-bridge";
    hostLabel: string;
    features: Record<string, any>;
    defaultModel: string;
    defaultPermission: PermissionMode;
    slashCommands?: SlashCommandItem[];
    outboxFiles?: OutboxFile[];
    outboxLoaded?: boolean;
    pendingProposals?: EvolutionProposal[];
    onOpenShare: (
      title: string,
      singleMarkdown: string,
      messages: ChatMessage[],
    ) => void;
    onRequestDelete?: (convId: string, title: string) => void;
    onOpenTerminal: () => void;
    onOpenSlashManager?: () => void;
    onOpenEvolution?: () => void;
    onApproveProposal?: (id: string) => Promise<void>;
    onRejectProposal?: (id: string) => Promise<void>;
    onOutboxRefresh?: () => Promise<void> | void;
    onOpenMediaPreview?: (target: PreviewTarget) => void;
    onFocus: () => void;
    onMetaChange?: (
      tabId: string,
      title: string,
      convId: string | null,
    ) => void;
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
    if (!userTouchedPerm && defaultPermission)
      permissionMode = defaultPermission;
  });

  let conversationId = $state<string | null>(
    localStorage.getItem(convStorageKey),
  );
  let sessionTitle = $state("");
  let messages = $state<ChatMessage[]>([]);
  type AttachedFileItem = {
    name: string;
    path: string;
    previewUrl?: string;
    isImage?: boolean;
  };
  let userInput = $state("");
  let attachedFiles = $state<AttachedFileItem[]>([]);
  let uploading = $state(false);
  let dragOverComposer = $state(false);
  let attachError = $state("");
  let attachErrorTimer: ReturnType<typeof setTimeout> | undefined;
  let quotaExceededError = $state("");
  let quotaNotice = $state("");

  let isGenerating = $state(false);
  let activeJobId = $state<string | null>(localStorage.getItem(jobStorageKey));
  let activeJobLastSeq = $state<number>(
    Number(localStorage.getItem(seqStorageKey) || "0"),
  );
  let liveStatus = $state("");
  let liveElapsed = $state(0);
  let jobProgress = $state(0);
  let copiedMsgIdx = $state<number | null>(null);
  let correctingMsgIdx = $state<number | null>(null);
  let correctionDraft = $state("");
  let submittingFeedbackIdx = $state<number | null>(null);
  let evolutionDockDismissed = $state(false);
  let busyProposalId = $state<string | null>(null);
  let prevPendingCount = 0;

  $effect(() => {
    if (pendingProposals.length > prevPendingCount) {
      evolutionDockDismissed = false;
    }
    prevPendingCount = pendingProposals.length;
  });

  let streamAbort: AbortController | null = null;
  let elapsedTimer: ReturnType<typeof setInterval> | null = null;
  let finishTimer: ReturnType<typeof setTimeout> | null = null;
  let copyMsgTimer: ReturnType<typeof setTimeout> | null = null;
  let toolStepCount = 0;
  let deltaCharsCount = 0;

  let isListening = $state(false);
  let transcribingVoice = $state(false);
  let showVoiceButton = $state(false);
  let recognition = $state<any>(null);
  let hasMediaRecorder = $state(false);
  let voiceNotice = $state("");
  let voiceNoticeTimer: ReturnType<typeof setTimeout> | null = null;
  let voicePrefix = "";
  let voiceFinal = "";
  let mediaRecorder: MediaRecorder | null = null;
  let mediaStream: MediaStream | null = null;
  let mediaChunks: BlobPart[] = [];
  let slashMenuOpen = $state(false);
  let approvalNeeded = $state<{ tool: string; command: string } | null>(null);

  let paneEl: HTMLElement | undefined = $state();
  let chatContainer: HTMLElement | undefined = $state();
  let fileInputEl: HTMLInputElement | undefined = $state();
  let textareaEl = $state<HTMLTextAreaElement | null>(null);
  let showScrollBottomBtn = $state(false);

  function handleChatScroll() {
    if (!chatContainer) return;
    const distanceFromBottom =
      chatContainer.scrollHeight -
      chatContainer.scrollTop -
      chatContainer.clientHeight;
    showScrollBottomBtn = distanceFromBottom > 120;
  }

  const builtinSlashCommands = [
    { cmd: "/usage", desc: "Cota restante de tokens/modelos" },
    { cmd: "/compact", desc: "Compactar histórico da sessão" },
    { cmd: "/diff", desc: "Exibir git status e diff atual" },
    { cmd: "/mem", desc: "Consultar memória persistente (ai-memory)" },
    { cmd: "/evolve", desc: "Relatório de Evolução do Aegys (1 clique)" },
    { cmd: "/plan", desc: "Ativar modo Planejamento (Leitura)" },
    { cmd: "/clear", desc: "Limpar e iniciar nova conversa" },
    { cmd: "/status", desc: "Status do Watchdog & recursos" },
  ];

  type PaletteCommandEntry = {
    cmd: string;
    desc: string;
    custom?: SlashCommandItem;
  };

  let slashFilterToken = $derived.by(() => {
    const trimmed = userInput.trimStart();
    if (!trimmed.startsWith("/")) return "";
    return trimmed.split(/\s+/)[0].toLowerCase();
  });

  let isTypingSlashSelector = $derived.by(() => {
    const trimmed = userInput.trimStart();
    return trimmed.startsWith("/") && !/\s/.test(trimmed);
  });

  let paletteEntries = $derived.by((): PaletteCommandEntry[] => {
    const all: PaletteCommandEntry[] = [
      ...slashCommands.map((c) => ({
        cmd: c.command,
        desc: c.description || c.prompt_template,
        custom: c,
      })),
      ...builtinSlashCommands.map((b) => ({ cmd: b.cmd, desc: b.desc })),
    ];
    if (!slashFilterToken) return all;
    return all.filter((item) => item.cmd.toLowerCase().startsWith(slashFilterToken));
  });

  function expandSlashPrompt(template: string, rawArgs: string): string {
    const args = rawArgs.trim();
    if (/\{\{\s*input\s*\}\}/i.test(template)) {
      if (args) {
        return template.replace(/\{\{\s*input\s*\}\}/gi, args).trim();
      }
      return template
        .replace(/[:\s-]*\{\{\s*input\s*\}\}/gi, "")
        .replace(/\s{2,}/g, " ")
        .trim();
    }
    return args ? `${template.trim()}\n\n${args}` : template.trim();
  }

  $effect(() => {
    onGeneratingChange?.(tabId, isGenerating);
  });

  function deriveTitle(msgs: ChatMessage[], explicitTitle?: string): string {
    if (explicitTitle?.trim()) return explicitTitle.trim();
    const firstUser = msgs.find((m) => m.role === "user")?.content.trim();
    if (!firstUser) return "";
    return firstUser.length > 44 ? firstUser.slice(0, 44) + "..." : firstUser;
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
    liveStatus = "";
    jobProgress = 0;
    approvalNeeded = null;
    isGenerating = false;
  }

  async function handleCancelActiveJob() {
    if (!activeJobId) return;
    const jid = activeJobId;
    liveStatus = "Cancelando execução...";
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

    if (
      messages.length === 0 ||
      messages[messages.length - 1].role !== "agent"
    ) {
      messages = [...messages, { role: "agent", content: "", job_id: jobId }];
    } else if (!messages[messages.length - 1].job_id) {
      messages[messages.length - 1].job_id = jobId;
      messages = messages;
    }
    const agentIdx = messages.length - 1;

    try {
      const response = await fetch(
        `/api/jobs/${jobId}/events?after=${afterSeq}`,
        {
          signal: controller.signal,
          credentials: "same-origin",
        },
      );

      if (!response.ok || !response.body) {
        throw new Error(`HTTP ${response.status}`);
      }

      const reader = response.body.getReader();
      const decoder = new TextDecoder("utf-8");
      let buffer = "";

      while (true) {
        const { done, value } = await reader.read();
        if (done) break;

        buffer += decoder.decode(value, { stream: true });
        const lines = buffer.split("\n");
        buffer = lines.pop() ?? "";

        for (const rawLine of lines) {
          const line = rawLine.trim();
          if (!line || line.startsWith(":")) continue;

          if (line.startsWith("id: ")) {
            const parsedId = parseInt(line.slice(4), 10);
            if (!Number.isNaN(parsedId)) {
              activeJobLastSeq = parsedId;
              localStorage.setItem(seqStorageKey, String(parsedId));
            }
            continue;
          }

          if (!line.startsWith("data: ")) continue;
          const dataStr = line.slice(6);
          if (dataStr === "[DONE]") {
            jobFinished = true;
            break;
          }

          try {
            const ev = JSON.parse(dataStr);
            if (ev.type === "init") {
              if (ev.conversation_id) {
                conversationId = ev.conversation_id;
                localStorage.setItem(convStorageKey, ev.conversation_id);
                notifyMeta();
              }
              setProgressFloor(18);
            } else if (ev.type === "delta") {
              messages[agentIdx].content += ev.text;
              messages = messages;
              deltaCharsCount += String(ev.text || "").length;
              setProgressFloor(
                56 + 40 * (1 - Math.exp(-deltaCharsCount / 520)),
              );
              liveStatus = "Digitando resposta...";
              scrollToBottom();
            } else if (ev.type === "tool") {
              const tName = ev.name || "ferramenta";
              if (ev.name && !currentTools.includes(ev.name)) {
                currentTools.push(ev.name);
              }
              const tState = String(ev.state || "ACTIVE");
              const tPreview = ev.preview ? String(ev.preview) : "";
              const tl = [...(messages[agentIdx].tool_timeline ?? [])];
              if (tState === "ACTIVE") {
                const existingIdx = tl.findIndex((s) => s.name === tName && s.state === "ACTIVE");
                if (existingIdx >= 0) {
                  if (tPreview) tl[existingIdx] = { ...tl[existingIdx], preview: tPreview };
                } else {
                  tl.push({ name: tName, preview: tPreview, state: "ACTIVE" });
                }
              } else {
                let updated = false;
                for (let k = tl.length - 1; k >= 0; k--) {
                  if (tl[k].name === tName && tl[k].state === "ACTIVE") {
                    tl[k] = { ...tl[k], state: tState, preview: tPreview || tl[k].preview };
                    updated = true;
                    break;
                  }
                }
                if (!updated) tl.push({ name: tName, preview: tPreview, state: tState });
              }
              messages[agentIdx].tool_timeline = tl;
              messages = messages;
              toolStepCount += ev.state === "ACTIVE" ? 1 : 0.6;
              setProgressFloor(20 + 54 * (1 - Math.exp(-toolStepCount * 0.42)));
              const preview = ev.preview
                ? ` (${String(ev.preview).slice(0, 28)}...)`
                : "";
              liveStatus =
                ev.state === "ACTIVE"
                  ? `Executando: ${tName}${preview}`
                  : `Concluído: ${tName}`;
              scrollToBottom();
            } else if (ev.type === "approval_needed") {
              approvalNeeded = {
                tool: ev.tool || "permissão",
                command: ev.command || "",
              };
              setProgressFloor(68);
              liveStatus = "Aguardando aprovação humana (HITL)...";
              scrollToBottom();
            } else if (ev.type === "done") {
              jobFinished = true;
              jobProgress = 100;
              liveStatus = "Concluído";
              if (ev.conversation_id) {
                conversationId = ev.conversation_id;
                localStorage.setItem(convStorageKey, ev.conversation_id);
                notifyMeta();
              }
              if (Array.isArray(ev.outbox_files) && ev.outbox_files.length > 0) {
                messages[agentIdx].outbox_files = ev.outbox_files.map((x: any) =>
                  String(x),
                );
                messages = messages;
                void onOutboxRefresh?.();
              }
              if (Array.isArray(ev.tool_timeline) && ev.tool_timeline.length > 0) {
                messages[agentIdx].tool_timeline = ev.tool_timeline;
                messages = messages;
              }
              if (!messages[agentIdx].content.trim()) {
                if (ev.response && String(ev.response).trim()) {
                  messages[agentIdx].content = String(ev.response).trim();
                } else {
                  const tools =
                    ev.executed_tools && ev.executed_tools.length
                      ? ev.executed_tools
                      : currentTools;
                  messages[agentIdx].content =
                    tools.length > 0
                      ? `*Instruções executadas em ${hostLabel} com sucesso.*\n\n**Ações executadas:**\n` +
                        tools.map((t: string) => `- \`${t}\``).join("\n")
                      : `*Instrução concluída com sucesso em ${hostLabel}.*`;
                }
                messages = messages;
              }
            } else if (ev.type === "error") {
              jobFinished = true;
              const errText = String(ev.error || "");
              if (
                ev.error_code === "quota_exceeded" ||
                /429|resource_exhausted|quota exceeded|rate limit|weekly limit/i.test(errText)
              ) {
                quotaExceededError = errText || "Limite de cota atingido.";
              }
              messages[agentIdx].content += `\n\n> **Erro:** ${ev.error}`;
              messages = messages;
            }
          } catch {
            // ignore malformed JSON chunk
          }
        }
      }
    } catch (err: any) {
      if (err?.name === "AbortError") return;
      if (document.visibilityState === "visible" && activeJobId === jobId) {
        liveStatus = "Reconectando ao stream...";
        setTimeout(() => {
          if (activeJobId === jobId && document.visibilityState === "visible") {
            streamJobEvents(jobId, activeJobLastSeq);
          }
        }, 1500);
      }
      return;
    }

    if (jobFinished && activeJobId === jobId) {
      if (!messages[agentIdx].content.trim()) {
        messages[agentIdx].content =
          `*Instrução concluída com sucesso em ${hostLabel}.*`;
        messages = messages;
      }
      jobProgress = 100;
      liveStatus = "Concluído";
      stopElapsedTimer();
      scrollToBottom();
      finishTimer = setTimeout(() => {
        if (activeJobId === jobId) {
          clearActiveJob();
        }
      }, 280);
    }
  }

  async function submitPromptInternal(
    promptText: string,
    filesToSend: string[] = [],
    opts?: { modelOverride?: string; displayText?: string; slashCommand?: string },
  ) {
    const text = promptText.trim();
    if ((!text && filesToSend.length === 0) || isGenerating) return;

    const displayPrompt =
      opts?.displayText?.trim() ||
      text ||
      `[${filesToSend.length} arquivo(s) enviado(s)]`;
    messages = [
      ...messages,
      { role: "user", content: displayPrompt },
      { role: "agent", content: "" },
    ];
    notifyMeta();
    isGenerating = true;
    liveStatus =
      gatewayMode === "standalone"
        ? "Iniciando localmente..."
        : `Iniciando em ${hostLabel}...`;
    startElapsedTimer();
    scrollToBottom();

    try {
      const effectiveModel = opts?.modelOverride?.trim() || selectedModel;
      const job = await createJob({
        prompt: text || "Analise os arquivos anexados.",
        conversation_id: conversationId,
        model: effectiveModel,
        permission_mode: permissionMode,
        attached_files: filesToSend.length > 0 ? filesToSend : undefined,
        slash_command: opts?.slashCommand,
      });

      if (messages.length > 0 && messages[messages.length - 1].role === "agent") {
        messages[messages.length - 1].job_id = job.job_id;
        messages = messages;
      }

      activeJobId = job.job_id;
      activeJobLastSeq = 0;
      localStorage.setItem(jobStorageKey, job.job_id);
      localStorage.setItem(seqStorageKey, "0");

      if (job.conversation_id) {
        conversationId = job.conversation_id;
        localStorage.setItem(convStorageKey, job.conversation_id);
        notifyMeta();
      }

      await streamJobEvents(job.job_id, 0);
    } catch (err: any) {
      const errText = `> **Erro:** ${err?.message || err}`;
      if (
        messages.length > 0 &&
        messages[messages.length - 1].role === "agent"
      ) {
        messages[messages.length - 1].content = errText;
        messages = messages;
      } else {
        messages = [...messages, { role: "agent", content: errText }];
      }
      clearActiveJob();
    }
  }

  async function handleSubmit(e?: Event) {
    if (e) e.preventDefault();
    const text = userInput.trim();
    if ((!text && attachedFiles.length === 0) || isGenerating) return;
    const filesToSend = attachedFiles.map((f) => f.path);

    slashMenuOpen = false;
    userInput = "";
    clearAttachedFiles();
    if (textareaEl) textareaEl.style.height = "auto";

    if (text.startsWith("/")) {
      const spaceIdx = text.search(/\s/);
      const cmdToken = (spaceIdx === -1 ? text : text.slice(0, spaceIdx)).toLowerCase();
      const rawArgs = spaceIdx === -1 ? "" : text.slice(spaceIdx + 1).trim();

      const customCmd = slashCommands.find(
        (c) => c.command.toLowerCase() === cmdToken,
      );
      if (customCmd) {
        const expanded = expandSlashPrompt(customCmd.prompt_template, rawArgs);
        await submitPromptInternal(expanded, filesToSend, {
          modelOverride: customCmd.model || undefined,
          displayText: text,
          slashCommand: customCmd.command,
        });
        return;
      }

      if (
        !rawArgs &&
        filesToSend.length === 0 &&
        builtinSlashCommands.some((b) => b.cmd === cmdToken)
      ) {
        await runSlashCommand(cmdToken);
        return;
      }
    }

    await submitPromptInternal(text, filesToSend);
  }

  async function runSlashCommand(cmd: string) {
    const normalized = cmd.toLowerCase();
    const trimmedInput = userInput.trim();
    let inlineArgs = "";
    if (trimmedInput.toLowerCase().startsWith(normalized)) {
      inlineArgs = trimmedInput.slice(normalized.length).trim();
    }

    const customCmd = slashCommands.find(
      (c) => c.command.toLowerCase() === normalized,
    );
    if (customCmd) {
      slashMenuOpen = false;
      if (!customCmd.auto_submit && !inlineArgs) {
        userInput = `${customCmd.command} `;
        await tick();
        syncTextareaHeight();
        textareaEl?.focus();
        return;
      }
      userInput = "";
      if (textareaEl) textareaEl.style.height = "auto";
      const expanded = expandSlashPrompt(customCmd.prompt_template, inlineArgs);
      await submitPromptInternal(expanded, [], {
        modelOverride: customCmd.model || undefined,
        displayText: inlineArgs
          ? `${customCmd.command} ${inlineArgs}`
          : customCmd.command,
        slashCommand: customCmd.command,
      });
      return;
    }

    slashMenuOpen = false;
    userInput = "";
    if (textareaEl) textareaEl.style.height = "auto";
    if (normalized === "/clear") {
      newConversation();
      return;
    }
    if (normalized === "/evolve") {
      onOpenEvolution?.();
      return;
    }
    if (normalized === "/plan") {
      userTouchedPerm = true;
      permissionMode = "readonly";
      messages = [
        ...messages,
        {
          role: "agent",
          content:
            "**Modo Planejamento (`readonly`) ativado.** Nenhuma alteração será aplicada sem revisão.",
        },
      ];
      scrollToBottom();
      return;
    }
    if (normalized === "/diff") {
      await submitPromptInternal(
        "Execute git status -s e git diff no workspace atual e exiba a saída dentro de um bloco ```diff.",
        [],
        { displayText: "/diff", slashCommand: "/diff" },
      );
      return;
    }
    if (normalized === "/mem") {
      userInput = "Consulte no ai-memory: ";
      await tick();
      syncTextareaHeight();
      textareaEl?.focus();
      return;
    }
    if (normalized === "/status") {
      const [wd, feat] = await Promise.all([fetchWatchdog(), fetchFeatures()]);
      const summary = [
        `### Status do Gateway (v${feat.version})`,
        `- **Host:** ${hostLabel} (${wd.online === false ? "Offline" : "Online"})`,
        `- **Verificações Watchdog:** ${wd.uptime_checks} (Quedas: ${wd.downtime_events})`,
        `- **Recursos:** ${Object.entries(feat.features)
          .filter(([, v]) => v === true)
          .map(([k]) => `\`${k}\``)
          .join(", ")}`,
      ].join("\n");
      messages = [...messages, { role: "agent", content: summary }];
      scrollToBottom();
      return;
    }
    if (normalized === "/compact" && !conversationId) {
      messages = [
        ...messages,
        {
          role: "agent",
          content:
            "> Não há conversa ativa para compactar. Envie uma mensagem ou carregue uma sessão primeiro.",
        },
      ];
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
      sessionTitle = data.title || titleHint || "";
      notifyMeta(sessionTitle);
      await scrollToBottom();

      if (data.active_job_id) {
        activeJobId = data.active_job_id;
        activeJobLastSeq = 0;
        isGenerating = true;
        liveStatus = "Retomando execução...";
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
    sessionTitle = "";
    localStorage.removeItem(convStorageKey);
    messages = [];
    notifyMeta("");
  }

  export async function submitExternalPrompt(promptText: string) {
    await submitPromptInternal(promptText);
  }

  export function getSnapshotContext(): {
    title: string;
    messages: ChatMessage[];
    convId: string | null;
  } {
    return {
      title: sessionTitle || deriveTitle(messages) || "Conversa AGY",
      messages,
      convId: conversationId,
    };
  }

  const IMAGE_EXT_BY_MIME: Record<string, string> = {
    "image/png": "png",
    "image/jpeg": "jpg",
    "image/jpg": "jpg",
    "image/webp": "webp",
    "image/gif": "gif",
    "image/bmp": "bmp",
    "image/svg+xml": "svg",
    "image/avif": "avif",
  };

  function flashAttachError(msg: string) {
    attachError = msg;
    if (attachErrorTimer) clearTimeout(attachErrorTimer);
    attachErrorTimer = setTimeout(() => {
      attachError = "";
    }, 4500);
  }

  function revokePreviewUrl(item: AttachedFileItem) {
    if (item.previewUrl) URL.revokeObjectURL(item.previewUrl);
  }

  function removeAttachedFile(idx: number) {
    const item = attachedFiles[idx];
    if (item) revokePreviewUrl(item);
    attachedFiles = attachedFiles.filter((_, k) => k !== idx);
  }

  function clearAttachedFiles() {
    for (const item of attachedFiles) {
      revokePreviewUrl(item);
    }
    attachedFiles = [];
  }

  function normalizeClipboardImageFile(file: File, index: number): File {
    const trimmedName = file.name?.trim() ?? "";
    if (trimmedName && /\.[a-z0-9]{2,5}$/i.test(trimmedName)) {
      return file;
    }
    const mime = file.type.toLowerCase();
    const ext =
      IMAGE_EXT_BY_MIME[mime] ||
      mime.split("/")[1]?.split("+")[0]?.replace(/[^a-z0-9]/gi, "") ||
      "png";
    const filename = trimmedName
      ? `${trimmedName}.${ext}`
      : `clipboard-${Date.now()}-${index + 1}.${ext}`;
    return new File([file], filename, {
      type: file.type || `image/${ext}`,
      lastModified: file.lastModified || Date.now(),
    });
  }

  function extractClipboardImageFiles(dt: DataTransfer | null): File[] {
    if (!dt) return [];
    const fromItems: File[] = [];
    if (dt.items) {
      for (const item of Array.from(dt.items)) {
        if (item.kind === "file" && item.type.startsWith("image/")) {
          const f = item.getAsFile();
          if (f) {
            fromItems.push(normalizeClipboardImageFile(f, fromItems.length));
          }
        }
      }
    }
    if (fromItems.length > 0) return fromItems;

    const fromFiles: File[] = [];
    if (dt.files) {
      for (const f of Array.from(dt.files)) {
        if (f.type.startsWith("image/")) {
          fromFiles.push(normalizeClipboardImageFile(f, fromFiles.length));
        }
      }
    }
    return fromFiles;
  }

  async function uploadAttachedFiles(files: File[]) {
    if (files.length === 0) return;
    attachError = "";
    uploading = true;
    const errors: string[] = [];
    try {
      for (const file of files) {
        const isImage = file.type.startsWith("image/");
        const previewUrl = isImage ? URL.createObjectURL(file) : undefined;
        try {
          const res = await uploadFile(file);
          attachedFiles = [
            ...attachedFiles,
            { name: res.filename, path: res.path, previewUrl, isImage },
          ];
        } catch (err) {
          if (previewUrl) URL.revokeObjectURL(previewUrl);
          const reason =
            err instanceof Error ? err.message : "falha no upload";
          errors.push(`${file.name}: ${reason}`);
        }
      }
    } finally {
      uploading = false;
    }
    if (errors.length > 0) {
      flashAttachError(errors.join("; "));
    }
  }

  async function handleFilesSelected(e: Event) {
    const input = e.target as HTMLInputElement;
    if (!input.files || input.files.length === 0) return;
    try {
      await uploadAttachedFiles(Array.from(input.files));
    } finally {
      input.value = "";
    }
  }

  async function handleComposerPaste(e: ClipboardEvent) {
    const dt = e.clipboardData;
    const imageFiles = extractClipboardImageFiles(dt);
    if (imageFiles.length === 0) return;
    const plainText = dt?.getData("text/plain")?.trim() ?? "";
    if (!plainText) {
      e.preventDefault();
    }
    await uploadAttachedFiles(imageFiles);
  }

  async function handleDocumentPaste(e: ClipboardEvent) {
    if (visibilityMode !== "active" || !isFocused) return;
    const target = e.target as HTMLElement | null;
    if (target === textareaEl) return;
    if (target?.closest("input, textarea, select, [contenteditable='true']")) {
      return;
    }
    const imageFiles = extractClipboardImageFiles(e.clipboardData);
    if (imageFiles.length === 0) return;
    e.preventDefault();
    await uploadAttachedFiles(imageFiles);
  }

  function handleComposerDragOver(e: DragEvent) {
    if (!e.dataTransfer?.types?.includes("Files")) return;
    e.preventDefault();
    dragOverComposer = true;
  }

  function handleComposerDragLeave(e: DragEvent) {
    const current = e.currentTarget as HTMLElement | null;
    const related = e.relatedTarget as Node | null;
    if (current && related && current.contains(related)) return;
    dragOverComposer = false;
  }

  async function handleComposerDrop(e: DragEvent) {
    dragOverComposer = false;
    const files = e.dataTransfer?.files ? Array.from(e.dataTransfer.files) : [];
    if (files.length === 0) return;
    e.preventDefault();
    await uploadAttachedFiles(files);
  }

  function flashVoiceNotice(msg: string) {
    voiceNotice = msg;
    if (voiceNoticeTimer) clearTimeout(voiceNoticeTimer);
    voiceNoticeTimer = setTimeout(() => {
      if (voiceNotice === msg) voiceNotice = "";
    }, 6500);
  }

  function syncTextareaHeight() {
    tick().then(() => {
      if (!textareaEl) return;
      textareaEl.style.height = "auto";
      textareaEl.style.height = `${Math.min(textareaEl.scrollHeight, 120)}px`;
    });
  }

  function mapSpeechError(code: string): string {
    switch (code) {
      case "not-allowed":
      case "service-not-allowed":
        if (typeof window !== "undefined" && !window.isSecureContext) {
          return "Microfone bloqueado em HTTP local. Use HTTPS / domínio próprio ou o microfone do teclado.";
        }
        return "Permissão de microfone ou ditado negada no navegador.";
      case "audio-capture":
        return "Nenhum microfone disponível ou em uso por outro app.";
      case "no-speech":
        return "Nenhuma fala detectada. Tente falar mais perto do microfone.";
      case "network":
        return "Serviço de reconhecimento de voz do navegador indisponível.";
      case "aborted":
        return "";
      default:
        return `Erro no ditado por voz (${code || "desconhecido"}).`;
    }
  }

  function cleanupMediaRecording() {
    if (mediaStream) {
      mediaStream.getTracks().forEach((t) => t.stop());
      mediaStream = null;
    }
    mediaRecorder = null;
    mediaChunks = [];
  }

  async function startWhisperRecording() {
    try {
      const stream = await navigator.mediaDevices.getUserMedia({ audio: true });
      mediaStream = stream;
      mediaChunks = [];
      const candidates = [
        "audio/webm;codecs=opus",
        "audio/webm",
        "audio/mp4",
        "audio/ogg",
      ];
      const mimeType =
        candidates.find(
          (m) =>
            typeof MediaRecorder.isTypeSupported === "function" &&
            MediaRecorder.isTypeSupported(m),
        ) || "";
      const rec = mimeType
        ? new MediaRecorder(stream, { mimeType })
        : new MediaRecorder(stream);
      mediaRecorder = rec;

      rec.ondataavailable = (ev) => {
        if (ev.data && ev.data.size > 0) mediaChunks.push(ev.data);
      };
      rec.onstop = async () => {
        const blobType = rec.mimeType || mimeType || "audio/webm";
        const ext = blobType.includes("mp4")
          ? "m4a"
          : blobType.includes("ogg")
            ? "ogg"
            : "webm";
        const blob = new Blob(mediaChunks, { type: blobType });
        cleanupMediaRecording();
        isListening = false;
        if (blob.size < 256) return;
        transcribingVoice = true;
        try {
          const text = await transcribeAudio(blob, `voice.${ext}`);
          if (text) {
            userInput = userInput.trim() ? `${userInput.trim()} ${text}` : text;
            syncTextareaHeight();
          } else {
            flashVoiceNotice("Nenhuma fala reconhecida na gravação.");
          }
        } catch (err: any) {
          flashVoiceNotice(err?.message || "Falha ao transcrever áudio.");
        } finally {
          transcribingVoice = false;
        }
      };
      rec.onerror = () => {
        isListening = false;
        cleanupMediaRecording();
        flashVoiceNotice("Falha ao gravar áudio do microfone.");
      };
      rec.start();
      isListening = true;
      voiceNotice = "";
    } catch (err: any) {
      isListening = false;
      cleanupMediaRecording();
      if (
        err?.name === "NotAllowedError" ||
        err?.name === "PermissionDeniedError"
      ) {
        flashVoiceNotice(
          "Permissão de microfone negada nas configurações do navegador.",
        );
      } else {
        flashVoiceNotice(
          err?.message || "Não foi possível acessar o microfone.",
        );
      }
    }
  }

  function toggleVoice() {
    voiceNotice = "";
    if (typeof window !== "undefined" && !window.isSecureContext) {
      flashVoiceNotice(
        "O microfone no navegador exige HTTPS / domínio próprio (ou localhost). Em HTTP local, use o microfone do teclado do celular.",
      );
      return;
    }

    if (mediaRecorder && isListening) {
      mediaRecorder.stop();
      return;
    }

    if (recognition) {
      if (isListening) {
        try {
          recognition.stop();
        } catch {
          isListening = false;
        }
        return;
      }
      voicePrefix = userInput.trim();
      voiceFinal = "";
      try {
        recognition.start();
        return;
      } catch (err: any) {
        isListening = false;
        if (features.whisper && hasMediaRecorder) {
          void startWhisperRecording();
          return;
        }
        flashVoiceNotice(
          err?.message || "Não foi possível iniciar o reconhecimento de voz.",
        );
        return;
      }
    }

    if (features.whisper && hasMediaRecorder) {
      void startWhisperRecording();
      return;
    }

    flashVoiceNotice(
      "Ditado nativo indisponível neste navegador. Use o microfone do teclado ou ative HTTPS.",
    );
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

  async function sendAgentFeedback(idx: number, rating: "up" | "down", correction = "") {
    const target = messages[idx];
    if (!target || target.role !== "agent") return;
    submittingFeedbackIdx = idx;
    try {
      await submitFeedback({
        job_id: target.job_id,
        conversation_id: conversationId,
        rating,
        correction: correction.trim() || undefined,
        message_snippet: target.content.slice(0, 240),
      });
      messages[idx].feedback = rating;
      messages = messages;
      if (correctingMsgIdx === idx) {
        correctingMsgIdx = null;
        correctionDraft = "";
      }
    } catch {
      // ignore non-blocking feedback error
    } finally {
      submittingFeedbackIdx = null;
    }
  }

  async function handleQuickProposalDecision(id: string, action: "approve" | "reject") {
    busyProposalId = id;
    try {
      if (action === "approve") {
        await onApproveProposal?.(id);
      } else {
        await onRejectProposal?.(id);
      }
    } finally {
      busyProposalId = null;
    }
  }

  async function handleDocumentClick(e: MouseEvent) {
    const target = e.target as HTMLElement | null;
    if (!target || !chatContainer?.contains(target)) return;
    const btn = target.closest(
      "button[data-copy-code]",
    ) as HTMLButtonElement | null;
    if (!btn) return;
    e.preventDefault();
    e.stopPropagation();

    const rawText = extractCodeFromCopyButton(btn);
    if (!rawText) return;
    const ok = await copyTextSafe(rawText);
    if (!ok) return;

    btn.dataset.copied = "true";
    btn.innerHTML = `${CHECK_ICON_SVG}<span>Copiado</span>`;
    setTimeout(() => {
      if (!btn.isConnected) return;
      delete btn.dataset.copied;
      btn.innerHTML = `${COPY_ICON_SVG}<span>Copiar</span>`;
    }, 1600);
  }

  function handleVisibilityChange() {
    if (document.visibilityState === "visible" && activeJobId && !streamAbort) {
      liveStatus = "Retomando stream...";
      streamJobEvents(activeJobId, activeJobLastSeq);
    }
  }

  onMount(async () => {
    document.addEventListener("visibilitychange", handleVisibilityChange);

    hasMediaRecorder =
      typeof window !== "undefined" &&
      typeof MediaRecorder !== "undefined" &&
      Boolean(navigator.mediaDevices?.getUserMedia);

    const SpeechRec =
      (window as any).SpeechRecognition ||
      (window as any).webkitSpeechRecognition;
    showVoiceButton =
      typeof window !== "undefined" &&
      (!window.isSecureContext ||
        Boolean(SpeechRec) ||
        (Boolean(features.whisper) && hasMediaRecorder));
    if (SpeechRec) {
      const rec = new SpeechRec();
      const isMobile = /Android|iPhone|iPad|iPod/i.test(
        navigator.userAgent || "",
      );
      rec.lang = "pt-BR";
      // Chrome Android drops/duplicates interim streams when continuous=true
      rec.continuous = !isMobile;
      rec.interimResults = true;
      rec.onstart = () => {
        isListening = true;
        voiceNotice = "";
      };
      rec.onend = () => {
        isListening = false;
      };
      rec.onerror = (ev: any) => {
        isListening = false;
        const code = String(ev?.error || "");
        if (
          (code === "network" || code === "service-not-allowed") &&
          features.whisper &&
          hasMediaRecorder &&
          window.isSecureContext
        ) {
          void startWhisperRecording();
          return;
        }
        const msg = mapSpeechError(code);
        if (msg) flashVoiceNotice(msg);
      };
      rec.onresult = (ev: any) => {
        let interim = "";
        for (let i = ev.resultIndex; i < ev.results.length; i++) {
          const res = ev.results[i];
          const text = String(res?.[0]?.transcript || "").trim();
          if (!text) continue;
          if (res.isFinal) {
            voiceFinal = voiceFinal ? `${voiceFinal} ${text}` : text;
          } else {
            interim = interim ? `${interim} ${text}` : text;
          }
        }
        userInput = [voicePrefix, voiceFinal, interim]
          .filter(Boolean)
          .join(" ");
        syncTextareaHeight();
      };
      recognition = rec;
    }

    if (conversationId) {
      await loadConversation(conversationId);
    } else if (activeJobId) {
      isGenerating = true;
      liveStatus = "Retomando sessão ativa...";
      startElapsedTimer(16);
      await streamJobEvents(activeJobId, activeJobLastSeq);
    }
  });

  onDestroy(() => {
    document.removeEventListener("visibilitychange", handleVisibilityChange);
    if (finishTimer) clearTimeout(finishTimer);
    if (copyMsgTimer) clearTimeout(copyMsgTimer);
    if (voiceNoticeTimer) clearTimeout(voiceNoticeTimer);
    if (attachErrorTimer) clearTimeout(attachErrorTimer);
    clearAttachedFiles();
    try {
      recognition?.abort?.();
    } catch {
      // ignore
    }
    if (mediaRecorder && mediaRecorder.state !== "inactive") {
      try {
        mediaRecorder.stop();
      } catch {
        // ignore
      }
    }
    cleanupMediaRecording();
    stopElapsedTimer();
    streamAbort?.abort();
  });
</script>

<svelte:document
  onclick={handleDocumentClick}
  onpaste={handleDocumentPaste}
  onpointerdown={(e) => {
    const target = e.target as HTMLElement | null;
    if (target && paneEl?.contains(target)) onFocus();
  }}
/>

<section
  bind:this={paneEl}
  onfocusin={onFocus}
  class="{visibilityMode === 'active'
    ? 'flex'
    : visibilityMode === 'split-hidden-mobile'
      ? 'hidden md:flex'
      : 'hidden'} flex-col flex-1 min-w-0 min-h-0 h-full {paneOrderClass} {isSplitView
    ? 'rounded-2xl border overflow-hidden bg-[var(--bg-app)] theme-surface ' +
      (isFocused
        ? 'border-[var(--accent-border)]'
        : 'border-[var(--border-subtle)]')
    : ''}"
>
  {#if isSplitView}
    <!-- Split Pane Mini Header -->
    <div
      class="h-9 px-3 bg-[var(--bg-surface)] border-b border-[var(--border-subtle)] flex items-center justify-between gap-2 shrink-0 select-none"
    >
      <div class="flex items-center gap-2 min-w-0">
        {#if isGenerating}
          <AiSpinner size={12} class="text-[var(--accent-fg)]" />
        {:else}
          <span
            class="w-1.5 h-1.5 rounded-full shrink-0 {isFocused
              ? 'bg-[var(--accent-fg)]'
              : 'bg-[var(--text-faint)]'}"
          ></span>
        {/if}
        <span
          class="text-[11px] font-semibold text-[var(--text-primary)] truncate"
        >
          {sessionTitle || "Nova Sessão"}
        </span>
        <span
          class="text-[10px] font-mono text-[var(--text-faint)] truncate hidden lg:inline"
        >
          ({selectedModel.replace("ollama:", "")})
        </span>
      </div>

      <div class="flex items-center gap-1 shrink-0">
        {#if messages.length > 0}
          <button
            type="button"
            onclick={() =>
              onOpenShare(sessionTitle || "Conversa AGY", "", messages)}
            class="p-1 rounded-md text-[var(--text-muted)] hover:text-[var(--accent-fg)] transition-colors"
            title="Compartilhar conversa"
          >
            <Icon name="share" size={13} />
          </button>
        {/if}
        {#if conversationId && onRequestDelete}
          <button
            type="button"
            onclick={() =>
              conversationId &&
              onRequestDelete(conversationId, sessionTitle || "Conversa AGY")}
            class="p-1 rounded-md text-[var(--text-muted)] hover:text-rose-400 hover:bg-rose-500/10 transition-colors"
            title="Apagar chat"
          >
            <Icon name="trash" size={13} />
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
  <div class="relative flex-1 min-h-0 flex flex-col overflow-hidden">
    <main
      bind:this={chatContainer}
      onscroll={handleChatScroll}
      class="flex-1 overflow-y-auto px-3 {isSplitView
        ? 'sm:px-4 py-3 sm:py-4'
        : 'sm:px-6 py-4 sm:py-6'} overscroll-contain"
    >
      <div
        class="{isSplitView
          ? 'w-full'
          : 'max-w-3xl lg:max-w-[820px] mx-auto w-full'} min-h-full flex flex-col space-y-4 sm:space-y-5"
      >
        {#if messages.length === 0}
          <div
            in:fade={{ duration: 220 }}
            class="relative flex-1 flex flex-col items-center justify-center text-center px-3 sm:px-6 py-8 select-none overflow-hidden"
          >
            <div
              class="pointer-events-none absolute inset-0 overflow-hidden [mask-image:radial-gradient(ellipse_at_center,white_18%,transparent_72%)]"
            >
              <GridPattern width={32} height={32} />
            </div>

            <div class="relative z-10 flex flex-col items-center max-w-md mx-auto">
              <div
                class="inline-flex items-center gap-2 rounded-full border border-border bg-card/90 backdrop-blur-sm px-3 py-1 mb-5 shadow-xs"
              >
                <span class="size-1.5 rounded-full bg-emerald-500 animate-pulse"></span>
                <span class="text-[11px] font-semibold text-foreground"
                  >{gatewayMode === "standalone" ? "AGY Local" : hostLabel}</span
                >
                <span class="h-3 w-px bg-border"></span>
                <span class="text-[11px] text-muted-foreground font-mono"
                  >SSE Resumível</span
                >
              </div>

              <Card
                class="p-2.5 rounded-2xl mb-4 bg-card/95 shadow-md ring-1 ring-foreground/10"
              >
                <img
                  src="/static/icon.svg"
                  class="w-12 h-12 sm:w-14 sm:h-14 rounded-xl"
                  alt="AGY Logo"
                />
              </Card>

              <h1
                class="text-base sm:text-lg font-bold tracking-tight text-foreground text-balance"
              >
                Antigravity Mobile Gateway
              </h1>
              <p
                class="text-xs sm:text-[13px] text-muted-foreground mt-1.5 max-w-sm leading-relaxed text-balance"
              >
                Svelte 5 + SSE resumível. Controle Gemini 4 Hydrogen, Extra, Ollama
                local, Inbox/Outbox e Terminal Web no celular ou desktop.
              </p>

              <div
                class="mt-5 flex flex-wrap items-center justify-center gap-1.5 max-w-full"
              >
                <Button
                  variant="outline"
                  size="sm"
                  onclick={() => runSlashCommand("/status")}
                >
                  <Icon name="cpu" size={13} class="text-accent-foreground" />
                  <span>Status</span>
                  <Kbd class="hidden sm:inline-flex">/status</Kbd>
                </Button>
                <Button
                  variant="outline"
                  size="sm"
                  onclick={() => runSlashCommand("/diff")}
                >
                  <Icon name="columns" size={13} class="text-accent-foreground" />
                  <span>Git diff</span>
                  <Kbd class="hidden sm:inline-flex">/diff</Kbd>
                </Button>
                <Button
                  variant="outline"
                  size="sm"
                  onclick={() => runSlashCommand("/usage")}
                >
                  <Icon name="sparkles" size={13} class="text-accent-foreground" />
                  <span>Cota</span>
                </Button>
                {#if features.terminal}
                  <Button
                    variant="outline"
                    size="sm"
                    onclick={onOpenTerminal}
                  >
                    <Icon name="terminal" size={13} class="text-emerald-500" />
                    <span>Terminal</span>
                  </Button>
                {/if}
              </div>
            </div>
          </div>
        {:else}
          {#each messages as msg, i}
            {@const isActiveBubble =
              isGenerating && i === messages.length - 1 && msg.role === "agent"}
            <div
              class="flex {msg.role === 'user'
                ? 'justify-end'
                : 'justify-start'} min-w-0 animate-msg-in"
            >
              {#if msg.role === "user"}
                <div
                  class="max-w-[85%] sm:max-w-[78%] rounded-2xl rounded-br-sm bg-radial-[at_52%_-52%] from-primary/85 to-primary text-primary-foreground px-4 py-2.5 text-sm leading-relaxed whitespace-pre-wrap break-words shadow-sm ring-1 ring-primary/30 theme-surface"
                >
                  {msg.content}
                </div>
              {:else}
                <Card
                  class="max-w-[96%] sm:max-w-[93%] min-w-0 rounded-2xl rounded-bl-sm px-4 py-3.5 {isSplitView
                    ? 'sm:px-4 sm:py-3.5'
                    : 'sm:px-6 sm:py-5'}"
                >
                  {#if isActiveBubble && !msg.content.trim()}
                    <div
                      class="flex items-center gap-2.5 py-0.5 min-w-[205px] sm:min-w-[240px]"
                    >
                      <AiSpinner size={20} class="text-accent-foreground" />
                      <div class="min-w-0 flex-1 space-y-1.5">
                        <div class="flex items-center justify-between gap-2.5">
                          <span
                            class="text-xs font-medium text-secondary-foreground truncate"
                          >
                            {liveStatus || "Processando solicitação..."}
                          </span>
                          <span
                            class="text-[10px] font-mono font-semibold text-accent-foreground shrink-0"
                          >
                            {Math.round(jobProgress)}%
                          </span>
                        </div>
                        <div
                          class="w-full h-1.5 rounded-full progress-track overflow-hidden"
                        >
                          <div
                            class="h-full rounded-full progress-fill"
                            style="width: {jobProgress}%;"
                          ></div>
                        </div>
                      </div>
                    </div>
                  {:else}
                    {#if msg.tool_timeline && msg.tool_timeline.length > 0}
                      <details class="mb-2 rounded-lg border border-border/70 bg-muted/40 px-2.5 py-1.5 text-[11px]">
                        <summary class="cursor-pointer select-none font-mono text-muted-foreground hover:text-foreground">
                          {msg.tool_timeline.length} ferramenta{msg.tool_timeline.length === 1 ? "" : "s"} executada{msg.tool_timeline.length === 1 ? "" : "s"} — ver detalhes
                        </summary>
                        <ul class="mt-1.5 space-y-1 border-t border-border/50 pt-1.5 font-mono text-[10px]">
                          {#each msg.tool_timeline as step}
                            <li class="flex items-start justify-between gap-2">
                              <span class="min-w-0 truncate">
                                <span class={step.state === "ACTIVE" ? "text-amber-400" : "text-emerald-400"}>
                                  {step.state === "ACTIVE" ? "●" : "✓"}
                                </span>
                                <span class="font-semibold text-foreground">{step.name}</span>
                                {#if step.preview}
                                  <span class="text-muted-foreground"> · {step.preview}</span>
                                {/if}
                              </span>
                              {#if step.duration_ms && step.duration_ms > 0}
                                <span class="shrink-0 text-muted-foreground">
                                  {step.duration_ms >= 1000 ? `${(step.duration_ms / 1000).toFixed(1)}s` : `${step.duration_ms}ms`}
                                </span>
                              {/if}
                            </li>
                          {/each}
                        </ul>
                      </details>
                    {/if}
                    <div
                      class="markdown-body {isActiveBubble
                        ? 'typing-cursor'
                        : ''}"
                    >
                      {@html renderMarkdown(msg.content)}
                    </div>
                    {#if !isActiveBubble && (msg.content.trim() || (msg.outbox_files && msg.outbox_files.length > 0))}
                      <OutboxInlinePreview
                        content={msg.content}
                        messageFiles={msg.outbox_files}
                        {outboxFiles}
                        {outboxLoaded}
                        onOpenPreview={onOpenMediaPreview}
                      />
                    {/if}
                    {#if isActiveBubble}
                      <div
                        class="mt-2.5 pt-2 border-t border-border space-y-1.5 text-[11px] text-muted-foreground"
                      >
                        <div class="flex items-center justify-between gap-3">
                          <div class="flex items-center gap-2 min-w-0">
                            <AiSpinner
                              size={14}
                              class="text-accent-foreground"
                            />
                            <span class="truncate"
                              >{liveStatus || "Gerando resposta..."}</span
                            >
                          </div>
                          <span
                            class="font-mono text-muted-foreground/80 shrink-0"
                          >
                            {Math.round(jobProgress)}% · {liveElapsed}s
                          </span>
                        </div>
                        <div
                          class="w-full h-1 rounded-full progress-track overflow-hidden"
                        >
                          <div
                            class="h-full rounded-full progress-fill"
                            style="width: {jobProgress}%;"
                          ></div>
                        </div>
                      </div>
                    {:else if msg.content.trim()}
                      <div
                        class="mt-4 pt-2.5 border-t border-border/70 flex flex-wrap items-center justify-between gap-1.5"
                      >
                        <div class="flex flex-wrap items-center gap-1">
                          <Button
                            variant="outline"
                            size="xs"
                            disabled={submittingFeedbackIdx === i}
                            onclick={() => sendAgentFeedback(i, "up")}
                            class={msg.feedback === "up"
                              ? "border-emerald-500/45 bg-emerald-500/15 text-emerald-500"
                              : ""}
                            title="Resposta útil (registra na telemetria)"
                          >
                            <Icon name="thumbs-up" size={12} />
                            <span>{msg.feedback === "up" ? "Útil" : ""}</span>
                          </Button>
                          <Button
                            variant="outline"
                            size="xs"
                            disabled={submittingFeedbackIdx === i}
                            onclick={() => {
                              if (correctingMsgIdx === i) {
                                correctingMsgIdx = null;
                              } else {
                                correctingMsgIdx = i;
                                correctionDraft = "";
                              }
                            }}
                            class={msg.feedback === "down" || correctingMsgIdx === i
                              ? "border-amber-500/45 bg-amber-500/15 text-amber-400"
                              : ""}
                            title="Registrar falha ou correção para o Aegys aprender"
                          >
                            <Icon name="thumbs-down" size={12} />
                            <span>{msg.feedback === "down" ? "Ajuste salvo" : "Corrigir"}</span>
                          </Button>
                        </div>

                        <div class="flex flex-wrap items-center gap-1.5">
                          <Button
                            variant="outline"
                            size="xs"
                            onclick={() => copyAgentMessage(i, msg.content)}
                            title="Copiar resposta em Markdown"
                          >
                            <Icon name={copiedMsgIdx === i ? "check" : "clipboard"} size={12} />
                            <span>{copiedMsgIdx === i ? "Copiado" : "Copiar"}</span>
                          </Button>
                          <Button
                            variant="outline"
                            size="xs"
                            onclick={() =>
                              onOpenShare(
                                sessionTitle || "Snapshot AGY",
                                msg.content,
                                messages,
                              )}
                            title="Compartilhar / Exportar snapshot"
                          >
                            <Icon name="share" size={12} />
                            <span>Compartilhar</span>
                          </Button>
                        </div>
                      </div>

                      {#if correctingMsgIdx === i}
                        <form
                          onsubmit={(ev) => {
                            ev.preventDefault();
                            void sendAgentFeedback(i, "down", correctionDraft);
                          }}
                          class="mt-2 pt-2 border-t border-border/60 flex flex-col sm:flex-row items-stretch sm:items-center gap-1.5"
                        >
                          <input
                            type="text"
                            bind:value={correctionDraft}
                            placeholder="O que o Aegys errou ou deve melhorar?"
                            class="flex-1 min-w-0 h-8 rounded-lg border border-input bg-muted px-2.5 text-xs text-foreground focus:outline-none focus:border-ring"
                          />
                          <div class="flex items-center justify-end gap-1 shrink-0">
                            <Button
                              type="button"
                              variant="ghost"
                              size="xs"
                              onclick={() => (correctingMsgIdx = null)}
                            >
                              Cancelar
                            </Button>
                            <Button
                              type="submit"
                              variant="default"
                              size="xs"
                              disabled={submittingFeedbackIdx === i}
                            >
                              Registrar
                            </Button>
                          </div>
                        </form>
                      {/if}
                    {/if}
                  {/if}
                </Card>
              {/if}
            </div>
          {/each}
        {/if}
      </div>
    </main>

    {#if showScrollBottomBtn}
      <div
        transition:fly={{ y: 10, duration: 160, easing: cubicOut }}
        class="absolute bottom-3 right-3 max-w-[calc(100%-1.5rem)] z-20"
      >
        <Button
          variant="outline"
          size="sm"
          data-scroll-bottom
          onclick={() => {
            scrollToBottom();
            showScrollBottomBtn = false;
          }}
          class="border-[var(--accent-border)] text-accent-foreground shadow-lg backdrop-blur-md"
          title="Ir para o fim da conversa"
        >
          <Icon name="chevron-down" size={14} />
          <span>Fim da conversa</span>
        </Button>
      </div>
    {/if}
  </div>

  <!-- Centered Bottom Dock (HITL Card + Slash Palette + Composer Card) -->
  <div
    class="w-full px-2.5 {isSplitView
      ? 'sm:px-3 pb-2.5'
      : 'sm:px-6 pb-[max(0.25rem,var(--safe-bottom-dynamic))] sm:pb-4'} pt-1 shrink-0"
  >
    <div
      class="{isSplitView
        ? 'w-full'
        : 'max-w-3xl lg:max-w-[820px] mx-auto w-full'} space-y-2"
    >
      <!-- Aegys Evolution 1-Click Approval Card -->
      {#if pendingProposals.length > 0 && !evolutionDockDismissed}
        {@const topProp = pendingProposals[0]}
        <div transition:fly={{ y: 10, duration: 170, easing: cubicOut }}>
          <Card class="p-2.5 sm:p-3 bg-accent/35 border-[var(--accent-border)] space-y-2">
            <div class="flex items-start justify-between gap-2">
              <div class="flex items-center gap-1.5 min-w-0">
                <Icon name="trending-up" size={14} class="text-accent-foreground shrink-0" />
                <span class="text-xs font-bold text-foreground truncate">
                  Relatório de Evolução ({pendingProposals.length})
                </span>
                <Badge variant="accent" size="xs" class="shrink-0">
                  {topProp.kind === "fast_path_slash" ? "Fast-Path /" : "Melhoria"}
                </Badge>
              </div>
              <div class="flex items-center gap-1 shrink-0">
                {#if onOpenEvolution}
                  <Button
                    variant="ghost"
                    size="xs"
                    onclick={() => onOpenEvolution?.()}
                    class="h-6 px-2 text-[11px] text-accent-foreground"
                  >
                    Ver todas
                  </Button>
                {/if}
                <button
                  type="button"
                  onclick={() => (evolutionDockDismissed = true)}
                  class="text-muted-foreground hover:text-foreground p-0.5"
                  aria-label="Ocultar sugestão de evolução"
                >
                  <Icon name="close" size={13} />
                </button>
              </div>
            </div>

            <div class="text-xs font-semibold text-foreground break-words">
              {topProp.title}
            </div>
            {#if topProp.rationale}
              <p class="text-[11px] text-muted-foreground line-clamp-2 break-words">
                {topProp.rationale}
              </p>
            {/if}

            <div class="flex flex-wrap items-center justify-between gap-1.5 pt-0.5">
              {#if topProp.kind === "fast_path_slash" && topProp.payload?.command}
                <Kbd class="text-accent-foreground border-[var(--accent-border)] bg-accent/60 px-1.5">
                  {topProp.payload.command}
                </Kbd>
              {:else}
                <span></span>
              {/if}
              <div class="flex items-center gap-1.5">
                <Button
                  variant="outline"
                  size="xs"
                  disabled={busyProposalId === topProp.id}
                  onclick={() => handleQuickProposalDecision(topProp.id, "reject")}
                >
                  Dispensar
                </Button>
                <Button
                  variant="default"
                  size="xs"
                  disabled={busyProposalId === topProp.id}
                  onclick={() => handleQuickProposalDecision(topProp.id, "approve")}
                  class="bg-emerald-600 hover:bg-emerald-500 text-white border-emerald-500"
                >
                  <Icon name="check" size={12} />
                  <span>{busyProposalId === topProp.id ? "Aplicando..." : "Aprovar em 1 clique"}</span>
                </Button>
              </div>
            </div>
          </Card>
        </div>
      {/if}

      <!-- Interactive HITL Approval Card -->
      {#if approvalNeeded}
        <div transition:fly={{ y: 12, duration: 180, easing: cubicOut }}>
          <Card class="p-3 bg-amber-500/10 border-amber-500/40 space-y-2">
            <div
              class="flex items-center justify-between gap-2 text-xs font-semibold text-amber-500"
            >
              <div class="flex items-center gap-1.5">
                <Icon name="shield" size={15} />
                <span>Aprovação HITL Necessária ({approvalNeeded.tool})</span>
              </div>
              <Badge variant="amber" size="xs">HITL</Badge>
            </div>
            {#if approvalNeeded.command}
              <pre
                class="bg-muted border border-border rounded-xl p-2.5 text-[11px] font-mono text-foreground overflow-x-auto">{approvalNeeded.command}</pre>
            {/if}
            <div class="flex items-center gap-2">
              {#if features.terminal}
                <Button
                  size="sm"
                  onclick={async () => {
                    if (approvalNeeded?.command) {
                      await navigator.clipboard
                        .writeText(approvalNeeded.command)
                        .catch(() => {});
                    }
                    onOpenTerminal();
                  }}
                  class="flex-1 bg-emerald-600 hover:bg-emerald-500 text-white border-emerald-500"
                >
                  <Icon name="terminal" size={14} />
                  <span>Copiar & Abrir Terminal</span>
                </Button>
              {/if}
              <Button
                variant="destructive"
                size="sm"
                onclick={handleCancelActiveJob}
              >
                Rejeitar
              </Button>
            </div>
          </Card>
        </div>
      {/if}

      <!-- Slash Commands Palette -->
      {#if slashMenuOpen || isTypingSlashSelector}
        <div transition:fly={{ y: 8, duration: 150, easing: cubicOut }}>
          <Card
            variant="mixed"
            class="p-1.5 space-y-1 max-h-60 overflow-y-auto shadow-xl"
          >
            <div
              class="px-2 py-1 border-b border-border/60 flex items-center justify-between gap-2"
            >
              <span class="text-[10px] font-semibold uppercase tracking-wide text-muted-foreground">
                Comandos Slash (/)
              </span>
              {#if onOpenSlashManager}
                <Button
                  variant="ghost"
                  size="xs"
                  onclick={() => {
                    slashMenuOpen = false;
                    onOpenSlashManager?.();
                  }}
                  class="h-6 px-2 text-[11px] text-accent-foreground"
                >
                  <Icon name="settings" size={11} />
                  <span>Gerenciar</span>
                </Button>
              {/if}
            </div>
            {#if paletteEntries.length === 0}
              <div class="px-2.5 py-2 text-[11px] text-muted-foreground">
                Nenhum atalho correspondente a "{slashFilterToken}".
              </div>
            {:else}
              {#each paletteEntries as item (item.cmd)}
                <button
                  type="button"
                  onclick={() => runSlashCommand(item.cmd)}
                  class="w-full text-left px-2.5 py-1.5 rounded-xl hover:bg-secondary/80 flex items-center justify-between gap-2 text-xs transition-colors"
                >
                  <div class="flex items-center gap-1.5 shrink-0">
                    <Kbd class="text-accent-foreground border-[var(--accent-border)] bg-accent/60 px-1.5">{item.cmd}</Kbd>
                    {#if item.custom}
                      <Badge variant={item.custom.auto_submit ? "accent" : "outline"} size="xs">
                        {item.custom.model ? "Rota" : "SQL"}
                      </Badge>
                    {/if}
                  </div>
                  <span class="text-muted-foreground text-[11px] truncate"
                    >{item.desc}</span
                  >
                </button>
              {/each}
            {/if}
          </Card>
        </div>
      {/if}

      <!-- Composer Card -->
      <footer>
        <Card
          variant="mixed"
          class={`overflow-hidden shadow-lg transition-colors ${
            dragOverComposer
              ? "ring-2 ring-primary/70 border-primary/60 bg-primary/5"
              : ""
          }`}
          ondragover={handleComposerDragOver}
          ondragleave={handleComposerDragLeave}
          ondrop={handleComposerDrop}
        >
          {#if isGenerating && liveStatus}
            <div
              transition:fly={{ y: 6, duration: 150, easing: cubicOut }}
              class="bg-muted border-b border-border theme-surface"
            >
              <div class="w-full h-[3px] progress-track overflow-hidden">
                <div
                  class="h-full progress-fill"
                  style="width: {jobProgress}%;"
                ></div>
              </div>
              <div
                class="px-3 py-1.5 flex items-center justify-between text-[11px] text-accent-foreground font-mono"
              >
                <div class="flex items-center gap-2 min-w-0">
                  <AiSpinner size={14} class="text-accent-foreground" />
                  <span class="truncate">{liveStatus}</span>
                </div>
                <div class="flex items-center gap-2 shrink-0">
                  <span class="text-secondary-foreground font-semibold"
                    >{Math.round(jobProgress)}%</span
                  >
                  <span class="text-muted-foreground">{liveElapsed}s</span>
                  <Button
                    variant="destructive"
                    size="xs"
                    onclick={handleCancelActiveJob}
                    class="h-5 px-2 text-[10px]"
                  >
                    <Icon name="stop" size={10} />
                    <span>Cancelar</span>
                  </Button>
                </div>
              </div>
            </div>
          {/if}

          {#if quotaExceededError}
            {@const ollamaFallback = models.find((m) => m.id.startsWith("ollama:"))}
            <div class="bg-amber-500/10 border-b border-amber-500/30 px-3 py-2 flex flex-col gap-1.5">
              <div class="flex items-start justify-between gap-2">
                <div class="min-w-0">
                  <p class="text-[11px] font-semibold text-amber-300">Cota AGY atingida (ToS-Safe)</p>
                  <p class="text-[10px] text-muted-foreground truncate">{quotaExceededError}</p>
                </div>
                <button
                  type="button"
                  onclick={() => (quotaExceededError = "")}
                  class="text-xs text-muted-foreground hover:text-foreground px-1"
                  aria-label="Fechar aviso de cota"
                >
                  ×
                </button>
              </div>
              <div class="flex flex-wrap items-center gap-1.5">
                <Button
                  variant="outline"
                  size="xs"
                  onclick={() => {
                    quotaExceededError = "";
                    onOpenTerminal();
                  }}
                >
                  Trocar Conta Manualmente (Fluxo Oficial)
                </Button>
                {#if ollamaFallback}
                  <Button
                    variant="outline"
                    size="xs"
                    onclick={() => {
                      userTouchedModel = true;
                      selectedModel = ollamaFallback.id;
                      quotaExceededError = "";
                      quotaNotice = `Backend alterado para ${ollamaFallback.name}. Envie sua próxima mensagem.`;
                    }}
                  >
                    Usar Ollama Local ({ollamaFallback.name})
                  </Button>
                {/if}
                <Button
                  variant="ghost"
                  size="xs"
                  onclick={() => {
                    userInput = "/usage";
                    quotaExceededError = "";
                  }}
                >
                  Consultar /usage
                </Button>
              </div>
            </div>
          {/if}

          {#if quotaNotice}
            <div class="bg-emerald-500/10 border-b border-emerald-500/30 px-3 py-1.5 flex items-center justify-between gap-2 text-[11px] text-emerald-300">
              <span>{quotaNotice}</span>
              <button type="button" onclick={() => (quotaNotice = "")} class="text-xs px-1">×</button>
            </div>
          {/if}

          <div class="px-2 py-1.5 sm:p-2.5 space-y-1.5 sm:space-y-2">
            <!-- Compact Model, Permission & Quick Actions Bar -->
            <div class="flex flex-wrap items-center justify-between gap-1.5">
              <div
                class="flex items-center gap-1.5 min-w-[210px] flex-1 sm:flex-initial"
              >
                <select
                  value={selectedModel}
                  onchange={(e) => {
                    userTouchedModel = true;
                    selectedModel = (e.target as HTMLSelectElement).value;
                  }}
                  class="min-w-[120px] flex-1 sm:flex-none {isSplitView
                    ? 'sm:w-40'
                    : 'sm:w-52'} bg-muted border border-border rounded-full px-2.5 py-1 text-[11px] text-foreground focus:outline-none focus:border-ring truncate transition-colors"
                >
                  {#each models as m}
                    <option value={m.id}>{m.name} ({m.badge})</option>
                  {/each}
                </select>

                <select
                  value={permissionMode}
                  onchange={(e) => {
                    userTouchedPerm = true;
                    permissionMode = (e.target as HTMLSelectElement)
                      .value as PermissionMode;
                  }}
                  class="bg-muted border border-border rounded-full px-2.5 py-1 text-[11px] font-medium focus:outline-none focus:border-ring shrink-0 transition-colors {permissionMode ===
                  'autonomous'
                    ? 'text-amber-500 border-amber-500/40'
                    : permissionMode === 'readonly'
                      ? 'text-accent-foreground'
                      : 'text-emerald-500'}"
                >
                  <option value="copilot">Copiloto</option>
                  <option value="autonomous">Autônomo</option>
                  <option value="readonly">Leitura</option>
                </select>
              </div>

              <div class="flex items-center gap-1 shrink-0">
                <Button
                  variant={slashMenuOpen ? "default" : "outline"}
                  size="xs"
                  onclick={() => (slashMenuOpen = !slashMenuOpen)}
                  class={!slashMenuOpen ? "text-accent-foreground" : ""}
                  title="Comandos Slash (/)"
                >
                  <span class="font-mono font-semibold">/</span>
                  <span>Comandos</span>
                </Button>

                <input
                  bind:this={fileInputEl}
                  type="file"
                  multiple
                  tabindex={-1}
                  onchange={handleFilesSelected}
                  class="hidden"
                />
                <Button
                  variant="outline"
                  size="xs"
                  onclick={() => fileInputEl?.click()}
                  disabled={uploading}
                  title="Anexar arquivo ou foto"
                >
                  <Icon name="paperclip" size={12} />
                  <span>Anexar</span>
                </Button>

                {#if showVoiceButton || (Boolean(features.whisper) && hasMediaRecorder)}
                  <Button
                    variant={isListening ? "destructiveSolid" : "outline"}
                    size="xs"
                    onclick={toggleVoice}
                    disabled={transcribingVoice}
                    class={isListening ? "animate-pulse" : ""}
                    title="Ditado por voz (requer HTTPS / localhost)"
                  >
                    <Icon name="mic" size={12} />
                    <span
                      >{transcribingVoice
                        ? "Transcrevendo..."
                        : isListening
                          ? "Ouvindo..."
                          : "Voz"}</span
                    >
                  </Button>
                {/if}
              </div>
            </div>

            {#if voiceNotice}
              <div
                transition:fade={{ duration: 140 }}
                class="flex items-center justify-between gap-2 px-2.5 py-1.5 rounded-xl bg-amber-500/10 border border-amber-500/35 text-[11px] text-amber-300"
              >
                <span class="break-words">{voiceNotice}</span>
                <button
                  type="button"
                  onclick={() => (voiceNotice = "")}
                  class="text-amber-300/80 hover:text-amber-200 shrink-0"
                  aria-label="Fechar aviso de voz"
                >
                  <Icon name="close" size={12} />
                </button>
              </div>
            {/if}

            {#if attachError}
              <div
                class="flex items-center justify-between gap-2 rounded-lg border border-rose-500/35 bg-rose-500/10 px-2.5 py-1.5 text-xs text-rose-300"
              >
                <span class="truncate">{attachError}</span>
                <button
                  type="button"
                  onclick={() => (attachError = "")}
                  aria-label="Fechar erro de anexo"
                  class="shrink-0 rounded p-0.5 text-rose-300/80 hover:text-rose-200 transition-colors"
                >
                  <Icon name="close" size={12} />
                </button>
              </div>
            {/if}

            <!-- Attached File Chips -->
            {#if attachedFiles.length > 0}
              <div class="flex flex-wrap gap-1.5">
                {#each attachedFiles as f, idx}
                  {#if f.isImage && f.previewUrl}
                    <div
                      class="group relative flex items-center gap-1.5 rounded-lg border border-primary/35 bg-primary/10 p-1 pr-2 text-xs text-foreground max-w-[200px]"
                    >
                      <img
                        src={f.previewUrl}
                        alt={f.name}
                        class="h-9 w-9 shrink-0 rounded object-cover border border-border/60 bg-black/20"
                      />
                      <span class="min-w-0 flex-1 truncate">{f.name}</span>
                      <button
                        type="button"
                        onclick={() => removeAttachedFile(idx)}
                        aria-label="Remover imagem"
                        class="shrink-0 rounded-full p-0.5 text-muted-foreground hover:text-rose-400 transition-colors"
                      >
                        <Icon name="close" size={11} />
                      </button>
                    </div>
                  {:else}
                    <Badge variant="accent" size="sm" class="gap-1.5 pr-1">
                      <Icon name="file" size={11} />
                      <span class="max-w-[130px] truncate">{f.name}</span>
                      <button
                        type="button"
                        onclick={() => removeAttachedFile(idx)}
                        aria-label="Remover anexo"
                        class="rounded-full p-0.5 text-muted-foreground hover:text-rose-400 transition-colors"
                      >
                        <Icon name="close" size={10} />
                      </button>
                    </Badge>
                  {/if}
                {/each}
              </div>
            {/if}

            <!-- Main Input Form -->
            <form onsubmit={handleSubmit} class="flex items-end gap-1.5">
              <Textarea
                bind:ref={textareaEl}
                bind:value={userInput}
                rows={1}
                placeholder={uploading
                  ? "Enviando arquivo..."
                  : gatewayMode === "standalone"
                    ? "Digite uma instrução ou cole uma imagem (Ctrl+V)..."
                    : `Instrução para ${hostLabel} (Ctrl+V cola imagem)...`}
                onpaste={handleComposerPaste}
                onfocus={() => setTimeout(scrollToBottom, 180)}
                oninput={(e) => {
                  const el = e.currentTarget as HTMLTextAreaElement;
                  el.style.height = "auto";
                  el.style.height = `${Math.min(el.scrollHeight, 120)}px`;
                }}
                onkeydown={(e) => {
                  if (e.key === "Enter" && !e.shiftKey) {
                    if (window.matchMedia?.("(pointer: coarse)").matches) return;
                    e.preventDefault();
                    handleSubmit();
                  }
                }}
                class="flex-1 min-h-0 resize-none overflow-y-auto max-h-[120px] py-2 leading-5"
              />

              {#if isGenerating && activeJobId}
                <Button
                  variant="destructiveSolid"
                  size="icon"
                  onclick={handleCancelActiveJob}
                  class="rounded-xl shrink-0"
                  title="Cancelar execução"
                  aria-label="Cancelar"
                >
                  <Icon name="stop" size={17} />
                </Button>
              {:else}
                <Button
                  type="submit"
                  variant="default"
                  size="icon"
                  disabled={isGenerating ||
                    (!userInput.trim() && attachedFiles.length === 0)}
                  class="rounded-xl shrink-0"
                  aria-label="Enviar"
                >
                  <Icon name="send" size={17} />
                </Button>
              {/if}
            </form>
          </div>
        </Card>
      </footer>
    </div>
  </div>
</section>
