<script lang="ts">
  import { fade, fly } from "svelte/transition";
  import { cubicOut } from "svelte/easing";
  import Icon from "./Icon.svelte";
  import AiSpinner from "./AiSpinner.svelte";
  import { Button } from "../lib/components/ui/button/index.js";
  import { Badge } from "../lib/components/ui/badge/index.js";
  import { Kbd } from "../lib/components/ui/kbd/index.js";
  import { Tabs, TabsList, TabsTrigger } from "../lib/components/ui/tabs/index.js";
  import {
    deleteAegysRule,
    fetchEvolutionReport,
    runEvolutionCycle,
    updateAegysRule,
    type EvolutionProposal,
    type EvolutionReport,
  } from "../lib/api";

  let {
    open = $bindable(false),
    report = $bindable<EvolutionReport | null>(null),
    onApproveProposal,
    onRejectProposal,
    onRefreshReport,
  }: {
    open: boolean;
    report: EvolutionReport | null;
    onApproveProposal: (id: string) => Promise<void>;
    onRejectProposal: (id: string) => Promise<void>;
    onRefreshReport?: () => Promise<void>;
  } = $props();

  type EvoTab = "proposals" | "telemetry" | "rules";

  let tab = $state<EvoTab>("proposals");
  let loading = $state(false);
  let runningCycle = $state(false);
  let busyProposalId = $state<string | null>(null);
  let busyRuleId = $state<string | null>(null);
  let feedback = $state("");
  let errorMsg = $state("");
  let feedbackTimer: ReturnType<typeof setTimeout> | null = null;

  let pendingProposals = $derived(
    (report?.proposals ?? []).filter((p) => p.status === "pending"),
  );
  let decidedProposals = $derived(
    (report?.proposals ?? []).filter((p) => p.status !== "pending").slice(0, 12),
  );
  let rules = $derived(report?.rules ?? []);
  let cycles = $derived(report?.cycles ?? []);

  $effect(() => {
    if (open) {
      void reloadReport();
    }
  });

  function flash(msg: string) {
    feedback = msg;
    if (feedbackTimer) clearTimeout(feedbackTimer);
    feedbackTimer = setTimeout(() => {
      if (feedback === msg) feedback = "";
    }, 4500);
  }

  async function reloadReport() {
    loading = true;
    errorMsg = "";
    try {
      report = await fetchEvolutionReport();
    } catch (err: any) {
      errorMsg = err?.message || "Falha ao carregar relatório de evolução.";
    } finally {
      loading = false;
    }
  }

  async function handleRunNow() {
    runningCycle = true;
    errorMsg = "";
    try {
      const res = await runEvolutionCycle(true);
      if (res.report) {
        report = res.report;
      } else {
        await reloadReport();
      }
      await onRefreshReport?.();
      const count = res.outcome?.proposals?.length ?? 0;
      if (res.outcome?.skipped) {
        flash(res.outcome.reason || "Sem eventos novos para consolidar.");
      } else if (count > 0) {
        flash(`Ciclo concluído: ${count} nova(s) melhoria(s) pronta(s) para aprovação.`);
        tab = "proposals";
      } else {
        flash("Consolidação concluída sem novas sugestões pendentes.");
      }
    } catch (err: any) {
      errorMsg = err?.message || "Erro ao rodar consolidação.";
    } finally {
      runningCycle = false;
    }
  }

  async function handleApprove(p: EvolutionProposal) {
    busyProposalId = p.id;
    errorMsg = "";
    try {
      await onApproveProposal(p.id);
      flash(
        p.kind === "fast_path_slash"
          ? `Atalho ${p.payload?.command || ""} criado no menu /!`
          : `Melhoria "${p.title}" aplicada às regras do Aegys.`,
      );
    } catch (err: any) {
      errorMsg = err?.message || "Erro ao aprovar proposta.";
    } finally {
      busyProposalId = null;
    }
  }

  async function handleReject(p: EvolutionProposal) {
    busyProposalId = p.id;
    errorMsg = "";
    try {
      await onRejectProposal(p.id);
      flash("Sugestão dispensada (não será repetida).");
    } catch (err: any) {
      errorMsg = err?.message || "Erro ao dispensar proposta.";
    } finally {
      busyProposalId = null;
    }
  }

  async function handleToggleRule(id: string, nextEnabled: boolean) {
    busyRuleId = id;
    errorMsg = "";
    try {
      await updateAegysRule(id, nextEnabled);
      await reloadReport();
      await onRefreshReport?.();
    } catch (err: any) {
      errorMsg = err?.message || "Erro ao atualizar regra.";
    } finally {
      busyRuleId = null;
    }
  }

  async function handleDeleteRule(id: string) {
    busyRuleId = id;
    errorMsg = "";
    try {
      await deleteAegysRule(id);
      await reloadReport();
      await onRefreshReport?.();
    } catch (err: any) {
      errorMsg = err?.message || "Erro ao remover regra.";
    } finally {
      busyRuleId = null;
    }
  }

  function kindLabel(kind: string): string {
    if (kind === "fast_path_slash") return "Fast-Path /";
    if (kind === "preference") return "Preferência";
    return "Regra de Prompt";
  }

  function fmtDate(iso?: string): string {
    if (!iso) return "";
    const d = new Date(iso);
    if (Number.isNaN(d.getTime())) return iso;
    return d.toLocaleString("pt-BR", {
      day: "2-digit",
      month: "2-digit",
      hour: "2-digit",
      minute: "2-digit",
    });
  }
</script>

{#if open}
  <div
    transition:fade={{ duration: 150 }}
    class="fixed inset-0 bg-[var(--bg-backdrop)] backdrop-blur-sm z-50 flex items-end sm:items-center justify-center p-0 sm:p-4"
  >
    <button
      type="button"
      aria-label="Fechar Relatório de Evolução"
      onclick={() => (open = false)}
      class="absolute inset-0 cursor-default"
    ></button>

    <div
      transition:fly={{ y: 20, duration: 200, easing: cubicOut }}
      class="relative w-full sm:max-w-2xl max-h-[90dvh] flex flex-col bg-card border border-border rounded-t-2xl sm:rounded-2xl shadow-2xl overflow-hidden"
    >
      <!-- Header -->
      <div class="p-3.5 border-b border-border flex items-center justify-between gap-2 shrink-0">
        <div class="flex items-center gap-2 min-w-0">
          <div class="p-1.5 rounded-lg bg-accent/70 border border-[var(--accent-border)] text-accent-foreground shrink-0">
            <Icon name="trending-up" size={16} />
          </div>
          <div class="min-w-0">
            <div class="flex flex-wrap items-center gap-1.5">
              <h2 class="text-sm font-bold text-foreground truncate">
                Evolução Contínua do Aegys
              </h2>
              {#if pendingProposals.length > 0}
                <Badge variant="accent" size="xs" class="font-mono">
                  {pendingProposals.length} pendente{pendingProposals.length > 1 ? "s" : ""}
                </Badge>
              {/if}
            </div>
            <p class="text-[11px] text-muted-foreground truncate">
              Consolidação local (ollama-worker + ai-memory · Cron {report?.cron_expr || "0 3 * * *"})
            </p>
          </div>
        </div>

        <div class="flex items-center gap-1 shrink-0">
          <Button
            variant="default"
            size="xs"
            disabled={runningCycle}
            onclick={handleRunNow}
            title="Analisar telemetria e gerar melhorias agora"
          >
            {#if runningCycle}
              <AiSpinner size={12} />
            {:else}
              <Icon name="sparkles" size={12} />
            {/if}
            <span>{runningCycle ? "Consolidando..." : "Consolidar Agora"}</span>
          </Button>
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

      <!-- Tabs & Status Pills -->
      <div class="px-3.5 pt-2.5 pb-2 border-b border-border/70 flex flex-wrap items-center justify-between gap-2 shrink-0 bg-muted/30">
        <Tabs
          value={tab}
          onValueChange={(v) => {
            tab = v as EvoTab;
          }}
        >
          <TabsList>
            <TabsTrigger value="proposals">
              Melhorias ({pendingProposals.length})
            </TabsTrigger>
            <TabsTrigger value="telemetry">
              Telemetria & Ciclos
            </TabsTrigger>
            <TabsTrigger value="rules">
              Regras ({rules.length})
            </TabsTrigger>
          </TabsList>
        </Tabs>

        <div class="flex items-center gap-1.5 text-[10px] font-mono">
          <Badge variant={report?.ollama_enabled ? "emerald" : "outline"} size="xs">
            Ollama {report?.ollama_enabled ? "ON" : "OFF"}
          </Badge>
          <Badge variant={report?.ai_memory_enabled ? "emerald" : "outline"} size="xs">
            ai-memory {report?.ai_memory_enabled ? "ON" : "OFF"}
          </Badge>
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

        {#if loading && !report}
          <div class="py-10 text-center text-xs text-muted-foreground">
            Carregando relatório de evolução...
          </div>
        {:else if tab === "proposals"}
          {#if pendingProposals.length === 0}
            <div class="p-5 rounded-xl bg-muted/40 border border-border text-center space-y-2">
              <p class="text-xs font-semibold text-foreground">
                Nenhuma sugestão pendente no momento.
              </p>
              <p class="text-[11px] text-muted-foreground max-w-md mx-auto leading-relaxed">
                O Aegys monitora comandos usados, falhas e correções e roda consolidação off-peak para propor novos atalhos Fast-Path (/) e regras melhores.
              </p>
              <div class="pt-1">
                <Button variant="outline" size="xs" disabled={runningCycle} onclick={handleRunNow}>
                  <Icon name="sparkles" size={12} />
                  <span>Executar análise agora</span>
                </Button>
              </div>
            </div>
          {:else}
            <div class="space-y-2.5">
              {#each pendingProposals as p (p.id)}
                <div class="p-3 rounded-xl bg-muted/40 border border-[var(--accent-border)] space-y-2">
                  <div class="flex items-start justify-between gap-2">
                    <div class="min-w-0 flex-1 space-y-1">
                      <div class="flex flex-wrap items-center gap-1.5">
                        <Badge variant="accent" size="xs">
                          {kindLabel(p.kind)}
                        </Badge>
                        {#if p.kind === "fast_path_slash" && p.payload?.command}
                          <Kbd class="text-accent-foreground border-[var(--accent-border)] bg-accent/60 px-1.5">
                            {p.payload.command}
                          </Kbd>
                        {/if}
                        {#if p.payload?.model}
                          <Badge variant="outline" size="xs" class="font-mono">
                            {p.payload.model}
                          </Badge>
                        {/if}
                      </div>
                      <h3 class="text-xs font-bold text-foreground break-words">
                        {p.title}
                      </h3>
                      {#if p.rationale}
                        <p class="text-[11px] text-muted-foreground leading-relaxed break-words">
                          {p.rationale}
                        </p>
                      {/if}
                    </div>
                  </div>

                  {#if p.kind === "fast_path_slash" && p.payload?.prompt_template}
                    <div class="p-2 rounded-lg bg-background/80 border border-border/80 text-[11px] font-mono text-secondary-foreground break-words">
                      {p.payload.prompt_template}
                    </div>
                  {:else if p.payload?.content}
                    <div class="p-2 rounded-lg bg-background/80 border border-border/80 text-[11px] text-secondary-foreground break-words leading-relaxed">
                      {p.payload.content}
                    </div>
                  {/if}

                  <div class="flex flex-wrap items-center justify-end gap-1.5 pt-1">
                    <Button
                      variant="outline"
                      size="xs"
                      disabled={busyProposalId === p.id}
                      onclick={() => handleReject(p)}
                    >
                      <Icon name="close" size={12} />
                      <span>Dispensar</span>
                    </Button>
                    <Button
                      variant="default"
                      size="xs"
                      disabled={busyProposalId === p.id}
                      onclick={() => handleApprove(p)}
                      class="bg-emerald-600 hover:bg-emerald-500 text-white border-emerald-500"
                    >
                      <Icon name="check" size={12} />
                      <span>{busyProposalId === p.id ? "Aplicando..." : "Aprovar em 1 clique"}</span>
                    </Button>
                  </div>
                </div>
              {/each}
            </div>
          {/if}

          {#if decidedProposals.length > 0}
            <div class="pt-2 space-y-1.5">
              <div class="text-[11px] font-semibold text-muted-foreground uppercase tracking-wide">
                Decisões recentes
              </div>
              {#each decidedProposals as dp (dp.id)}
                <div class="px-2.5 py-2 rounded-xl bg-muted/25 border border-border/70 flex items-center justify-between gap-2 text-[11px]">
                  <div class="min-w-0 flex items-center gap-2">
                    <Badge variant={dp.status === "approved" ? "emerald" : "outline"} size="xs">
                      {dp.status === "approved" ? "Aprovada" : "Dispensada"}
                    </Badge>
                    <span class="text-foreground truncate">{dp.title}</span>
                  </div>
                  <span class="text-[10px] font-mono text-muted-foreground shrink-0">
                    {fmtDate(dp.decided_at || dp.created_at)}
                  </span>
                </div>
              {/each}
            </div>
          {/if}
        {:else if tab === "telemetry"}
          {@const st = report?.stats}
          <div class="grid grid-cols-2 sm:grid-cols-4 gap-2">
            <div class="p-2.5 rounded-xl bg-muted/40 border border-border space-y-0.5">
              <div class="text-[10px] text-muted-foreground uppercase">Execuções OK</div>
              <div class="text-base font-bold font-mono text-emerald-500">{st?.total_jobs_ok ?? 0}</div>
            </div>
            <div class="p-2.5 rounded-xl bg-muted/40 border border-border space-y-0.5">
              <div class="text-[10px] text-muted-foreground uppercase">Falhas</div>
              <div class="text-base font-bold font-mono text-rose-400">{st?.total_job_errors ?? 0}</div>
            </div>
            <div class="p-2.5 rounded-xl bg-muted/40 border border-border space-y-0.5">
              <div class="text-[10px] text-muted-foreground uppercase">Feedbacks +/-</div>
              <div class="text-base font-bold font-mono text-foreground">
                {st?.total_feedback_up ?? 0} / {st?.total_feedback_down ?? 0}
              </div>
            </div>
            <div class="p-2.5 rounded-xl bg-muted/40 border border-border space-y-0.5">
              <div class="text-[10px] text-muted-foreground uppercase">Correções</div>
              <div class="text-base font-bold font-mono text-amber-400">{st?.total_corrections ?? 0}</div>
            </div>
          </div>

          {#if st?.top_commands && st.top_commands.length > 0}
            <div class="p-3 rounded-xl bg-muted/30 border border-border space-y-2">
              <div class="text-xs font-semibold text-foreground">Comandos mais usados</div>
              <div class="flex flex-wrap gap-1.5">
                {#each st.top_commands as cmd}
                  <Badge variant="accent" size="sm" class="font-mono gap-1.5">
                    <span>{cmd.name}</span>
                    <span class="opacity-75">×{cmd.count}</span>
                  </Badge>
                {/each}
              </div>
            </div>
          {/if}

          {#if st?.repeated_prompts && st.repeated_prompts.length > 0}
            <div class="p-3 rounded-xl bg-muted/30 border border-border space-y-2">
              <div class="text-xs font-semibold text-foreground">Prompts frequentes (candidatos Fast-Path)</div>
              <div class="space-y-1.5">
                {#each st.repeated_prompts as rep}
                  <div class="p-2 rounded-lg bg-background/70 border border-border/70 flex items-center justify-between gap-2 text-[11px]">
                    <span class="text-foreground truncate">{rep.prompt_preview}</span>
                    <Badge variant="outline" size="xs" class="font-mono shrink-0">{rep.count}x</Badge>
                  </div>
                {/each}
              </div>
            </div>
          {/if}

          <div class="space-y-1.5">
            <div class="text-xs font-semibold text-foreground">Ciclos de Consolidação</div>
            {#if cycles.length === 0}
              <p class="text-[11px] text-muted-foreground">Nenhum ciclo executado ainda.</p>
            {:else}
              {#each cycles as c (c.id)}
                <div class="p-2.5 rounded-xl bg-muted/30 border border-border/80 space-y-1 text-[11px]">
                  <div class="flex flex-wrap items-center justify-between gap-1.5">
                    <div class="flex items-center gap-1.5">
                      <Badge variant={c.status === "completed" ? "emerald" : c.status === "partial" ? "amber" : "outline"} size="xs">
                        {c.status}
                      </Badge>
                      <span class="font-mono text-muted-foreground">{c.analyzer_backend || "local"}</span>
                      <span class="font-mono text-muted-foreground">ai-memory: {c.ai_memory_status || "-"}</span>
                    </div>
                    <span class="text-[10px] font-mono text-muted-foreground">{fmtDate(c.started_at)}</span>
                  </div>
                  <p class="text-secondary-foreground break-words">{c.summary}</p>
                </div>
              {/each}
            {/if}
          </div>
        {:else}
          {#if rules.length === 0}
            <div class="p-5 rounded-xl bg-muted/40 border border-border text-center text-xs text-muted-foreground">
              Nenhuma regra ou preferência aprendida aprovada ainda.
            </div>
          {:else}
            <div class="space-y-2">
              {#each rules as rule (rule.id)}
                <div class="p-3 rounded-xl bg-muted/35 border border-border space-y-2">
                  <div class="flex items-start justify-between gap-2">
                    <div class="min-w-0 flex-1 space-y-1">
                      <div class="flex items-center gap-1.5">
                        <Badge variant={rule.enabled ? "emerald" : "outline"} size="xs">
                          {rule.enabled ? "Ativa" : "Pausada"}
                        </Badge>
                        <span class="text-xs font-bold text-foreground truncate">{rule.title}</span>
                      </div>
                      <p class="text-[11px] text-secondary-foreground break-words leading-relaxed">
                        {rule.content}
                      </p>
                    </div>
                    <div class="flex items-center gap-1 shrink-0">
                      <Button
                        variant="outline"
                        size="xs"
                        disabled={busyRuleId === rule.id}
                        onclick={() => handleToggleRule(rule.id, !rule.enabled)}
                      >
                        {rule.enabled ? "Pausar" : "Ativar"}
                      </Button>
                      <Button
                        variant="ghost"
                        size="icon-sm"
                        disabled={busyRuleId === rule.id}
                        onclick={() => handleDeleteRule(rule.id)}
                        class="text-muted-foreground hover:text-rose-400"
                        title="Remover regra"
                      >
                        <Icon name="trash" size={14} />
                      </Button>
                    </div>
                  </div>
                </div>
              {/each}
            </div>
          {/if}
        {/if}
      </div>
    </div>
  </div>
{/if}
