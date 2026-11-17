<script lang="ts">
  import { onDestroy } from 'svelte';
  import {
    fetchCockpitDockerLogs,
    fetchCockpitOverview,
    runCockpitDockerAction,
    triggerCockpitPower,
    type CockpitOverview,
    type CockpitPowerAction,
  } from './cockpitApi';
  import { buildSparklineGeometry, formatMB } from './telemetrySvg';

  let { active = true }: { active?: boolean } = $props();

  type CockpitTab = 'charts' | 'processes' | 'docker' | 'power';

  let subTab = $state<CockpitTab>('charts');
  let expanded = $state(false);
  let loading = $state(false);
  let errorMsg = $state('');
  let overview = $state<CockpitOverview | null>(null);

  let dockerBusyId = $state<string | null>(null);
  let activeLogContainer = $state<{ id: string; name: string } | null>(null);
  let logsText = $state('');
  let loadingLogs = $state(false);

  let pendingPower = $state<{ action: CockpitPowerAction; label: string; word: string; desc: string } | null>(null);
  let powerConfirmInput = $state('');
  let powerBusy = $state(false);
  let powerFeedback = $state('');

  let pollTimer: ReturnType<typeof setInterval> | null = null;

  async function refreshOverview(force = false) {
    if (!active) return;
    if (!overview) loading = true;
    try {
      const data = await fetchCockpitOverview(force);
      overview = data;
      errorMsg = '';
    } catch (err: any) {
      errorMsg = err?.message || 'Cockpit optional package indisponível';
    } finally {
      loading = false;
    }
  }

  function stopPolling() {
    if (pollTimer) {
      clearInterval(pollTimer);
      pollTimer = null;
    }
  }

  $effect(() => {
    if (active) {
      refreshOverview(false);
      stopPolling();
      pollTimer = setInterval(() => {
        refreshOverview(false);
      }, 5000);
    } else {
      stopPolling();
      expanded = false;
      pendingPower = null;
      powerConfirmInput = '';
    }
  });

  onDestroy(() => {
    stopPolling();
  });

  let vramChart = $derived(
    buildSparklineGeometry(
      (overview?.history ?? []).map((p) => (p.vram_total_mb > 0 ? p.vram_pct : null)),
      0,
      100
    )
  );
  let gpuTempChart = $derived(
    buildSparklineGeometry(
      (overview?.history ?? []).map((p) => p.gpu_temp_c),
      30,
      95
    )
  );
  let cpuChart = $derived(
    buildSparklineGeometry(
      (overview?.history ?? []).map((p) => p.cpu_pct),
      0,
      100
    )
  );
  let ramChart = $derived(
    buildSparklineGeometry(
      (overview?.history ?? []).map((p) => (p.ram_total_mb > 0 ? p.ram_pct : null)),
      0,
      100
    )
  );

  async function handleDockerAction(id: string, action: 'start' | 'stop') {
    dockerBusyId = id;
    errorMsg = '';
    try {
      await runCockpitDockerAction(id, action);
      await refreshOverview(true);
    } catch (err: any) {
      errorMsg = err?.message || `Erro ao executar docker ${action}`;
    } finally {
      dockerBusyId = null;
    }
  }

  async function openDockerLogs(id: string, name: string) {
    if (activeLogContainer?.id === id) {
      activeLogContainer = null;
      logsText = '';
      return;
    }
    activeLogContainer = { id, name };
    loadingLogs = true;
    logsText = '';
    try {
      logsText = await fetchCockpitDockerLogs(id, 80);
    } catch (err: any) {
      logsText = err?.message || 'Não foi possível carregar logs.';
    } finally {
      loadingLogs = false;
    }
  }

  const powerSpecs: { action: CockpitPowerAction; label: string; word: string; desc: string; danger: boolean }[] = [
    {
      action: 'lock',
      label: 'Bloquear Tela',
      word: 'BLOQUEAR',
      desc: 'Trava a sessão gráfica da workstation imediatamente.',
      danger: false,
    },
    {
      action: 'suspend',
      label: 'Suspender PC',
      word: 'SUSPENDER',
      desc: 'Coloca o computador em suspensão (pode ser acordado depois via WoL).',
      danger: true,
    },
    {
      action: 'reboot',
      label: 'Reiniciar PC',
      word: 'REINICIAR',
      desc: 'Reinicia o sistema operacional e encerra sessões ativas.',
      danger: true,
    },
  ];

  function selectPowerAction(spec: (typeof powerSpecs)[number]) {
    powerFeedback = '';
    powerConfirmInput = '';
    pendingPower = spec;
  }

  let canSubmitPower = $derived(
    pendingPower !== null &&
      (pendingPower.action === 'lock' ||
        powerConfirmInput.trim().toUpperCase() === pendingPower.word)
  );

  async function submitPowerAction() {
    if (!pendingPower || powerBusy || !canSubmitPower) return;
    const action = pendingPower.action;
    const confirmWord = action === 'lock' ? pendingPower.word : powerConfirmInput.trim().toUpperCase();
    powerBusy = true;
    powerFeedback = '';
    errorMsg = '';
    try {
      await triggerCockpitPower(action, confirmWord);
      powerFeedback = `Comando "${pendingPower.label}" agendado com sucesso.`;
      pendingPower = null;
      powerConfirmInput = '';
    } catch (err: any) {
      errorMsg = err?.message || 'Erro ao executar ação de energia';
    } finally {
      powerBusy = false;
    }
  }
</script>

{#snippet sparkCard(
  gradId: string,
  title: string,
  valueText: string,
  subText: string,
  geom: ReturnType<typeof buildSparklineGeometry>,
  strokeColor: string
)}
  <div class="cp-metric-card">
    <div class="cp-metric-head">
      <span class="cp-metric-title">{title}</span>
      <span class="cp-metric-val" style="color: {strokeColor};">{valueText}</span>
    </div>
    <svg viewBox="0 0 120 36" class="cp-svg" preserveAspectRatio="none" aria-hidden="true">
      <defs>
        <linearGradient id={gradId} x1="0" y1="0" x2="0" y2="1">
          <stop offset="0%" stop-color={strokeColor} stop-opacity="0.32" />
          <stop offset="100%" stop-color={strokeColor} stop-opacity="0.02" />
        </linearGradient>
      </defs>
      <line x1="0" y1="18" x2="120" y2="18" stroke="var(--border-subtle)" stroke-dasharray="2 2" stroke-width="0.7" />
      {#if geom.hasData}
        <path d={geom.areaPath} fill={`url(#${gradId})`} />
        <path d={geom.linePath} fill="none" stroke={strokeColor} stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round" />
        <circle cx={geom.lastX} cy={geom.lastY} r="2.2" fill={strokeColor} />
      {/if}
    </svg>
    <div class="cp-metric-sub">{subText}</div>
  </div>
{/snippet}

{#snippet cockpitBody()}
  <div class="cp-root">
    <!-- Header -->
    <div class="cp-header">
      <div class="cp-title-row">
        <span class="cp-dot" class:cp-dot-on={overview !== null && !errorMsg}></span>
        <span class="cp-title">Cockpit</span>
        {#if overview?.hostname}
          <span class="cp-host">{overview.hostname}</span>
        {/if}
      </div>

      <div class="cp-head-actions">
        <button
          type="button"
          class="cp-icon-btn"
          title="Atualizar métricas"
          onclick={() => refreshOverview(true)}
          disabled={loading}
        >
          <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <polyline points="23 4 23 10 17 10" />
            <polyline points="1 20 1 14 7 14" />
            <path d="M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15" />
          </svg>
        </button>
        <button
          type="button"
          class="cp-icon-btn"
          title={expanded ? 'Recolher painel' : 'Expandir painel'}
          onclick={() => (expanded = !expanded)}
        >
          {#if expanded}
            <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <polyline points="4 14 10 14 10 20" />
              <polyline points="20 10 14 10 14 4" />
              <line x1="14" y1="10" x2="21" y2="3" />
              <line x1="3" y1="21" x2="10" y2="14" />
            </svg>
          {:else}
            <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <polyline points="15 3 21 3 21 9" />
              <polyline points="9 21 3 21 3 15" />
              <line x1="21" y1="3" x2="14" y2="10" />
              <line x1="3" y1="21" x2="10" y2="14" />
            </svg>
          {/if}
        </button>
      </div>
    </div>

    <!-- Navigation Pills -->
    <div class="cp-tabs">
      <button type="button" class="cp-tab" class:cp-tab-active={subTab === 'charts'} onclick={() => (subTab = 'charts')}>Gráficos</button>
      <button type="button" class="cp-tab" class:cp-tab-active={subTab === 'processes'} onclick={() => (subTab = 'processes')}>Processos</button>
      <button type="button" class="cp-tab" class:cp-tab-active={subTab === 'docker'} onclick={() => (subTab = 'docker')}>Docker</button>
      <button type="button" class="cp-tab" class:cp-tab-active={subTab === 'power'} onclick={() => (subTab = 'power')}>Energia</button>
    </div>

    {#if errorMsg}
      <div class="cp-alert cp-alert-err">{errorMsg}</div>
    {/if}

    {#if loading && !overview}
      <div class="cp-empty">Conectando ao pacote opcional Cockpit...</div>
    {:else if overview}
      {#if subTab === 'charts'}
        {@const cur = overview.current}
        <div class="cp-chart-grid" class:cp-chart-grid-wide={expanded}>
          {@render sparkCard(
            'cp-grad-vram',
            'VRAM',
            cur.vram_total_mb > 0 ? `${cur.vram_pct.toFixed(0)}%` : 'N/D',
            cur.vram_total_mb > 0 ? `${formatMB(cur.vram_used_mb)} / ${formatMB(cur.vram_total_mb)}` : 'Sem GPU NVIDIA',
            vramChart,
            'var(--accent-fg)'
          )}
          {@render sparkCard(
            'cp-grad-gputemp',
            'GPU Temp',
            cur.gpu_temp_c !== null ? `${cur.gpu_temp_c}°C` : 'N/D',
            cur.gpu_name || 'Sem sensor GPU',
            gpuTempChart,
            cur.gpu_temp_c !== null && cur.gpu_temp_c >= 80 ? '#f43f5e' : cur.gpu_temp_c !== null && cur.gpu_temp_c >= 68 ? '#f59e0b' : '#10b981'
          )}
          {@render sparkCard(
            'cp-grad-cpu',
            'CPU',
            cur.cpu_pct !== null ? `${cur.cpu_pct.toFixed(0)}%` : 'N/D',
            cur.cpu_temp_c !== null ? `Temp: ${cur.cpu_temp_c}°C` : `${overview.history.length} amostras`,
            cpuChart,
            '#38bdf8'
          )}
          {@render sparkCard(
            'cp-grad-ram',
            'RAM',
            cur.ram_total_mb > 0 ? `${cur.ram_pct.toFixed(0)}%` : 'N/D',
            cur.ram_total_mb > 0 ? `${formatMB(cur.ram_used_mb)} / ${formatMB(cur.ram_total_mb)}` : 'Sem dados RAM',
            ramChart,
            '#a78bfa'
          )}
        </div>
      {:else if subTab === 'processes'}
        {#if overview.processes.length === 0}
          <div class="cp-empty">Nenhum processo coletado ainda.</div>
        {:else}
          <div class="cp-list" class:cp-list-tall={expanded}>
            {#each overview.processes as proc}
              <div class="cp-row">
                <div class="cp-row-main">
                  <span class="cp-row-name">{proc.name}</span>
                  <span class="cp-row-meta">PID {proc.pid}</span>
                </div>
                <div class="cp-row-badges">
                  {#if proc.vram_mb && proc.vram_mb > 0}
                    <span class="cp-badge cp-badge-gpu">GPU {formatMB(proc.vram_mb)}</span>
                  {/if}
                  <span class="cp-badge">{formatMB(proc.ram_mb)}</span>
                </div>
              </div>
            {/each}
          </div>
        {/if}
      {:else if subTab === 'docker'}
        {#if !overview.docker_available}
          <div class="cp-empty">Docker CLI / daemon indisponível nesta máquina.</div>
        {:else if overview.containers.length === 0}
          <div class="cp-empty">Nenhum container Docker encontrado.</div>
        {:else}
          <div class="cp-list" class:cp-list-tall={expanded}>
            {#each overview.containers as ctr}
              <div class="cp-docker-card">
                <div class="cp-docker-top">
                  <div class="cp-row-main">
                    <div class="cp-title-row">
                      <span class="cp-dot" class:cp-dot-on={ctr.state === 'running'}></span>
                      <span class="cp-row-name">{ctr.name}</span>
                    </div>
                    <span class="cp-row-meta">{ctr.image} · {ctr.status}</span>
                  </div>
                  <div class="cp-docker-actions">
                    {#if ctr.state === 'running'}
                      <button
                        type="button"
                        class="cp-mini-btn cp-mini-danger"
                        disabled={dockerBusyId === ctr.id}
                        onclick={() => handleDockerAction(ctr.id, 'stop')}
                      >
                        Stop
                      </button>
                    {:else}
                      <button
                        type="button"
                        class="cp-mini-btn cp-mini-ok"
                        disabled={dockerBusyId === ctr.id}
                        onclick={() => handleDockerAction(ctr.id, 'start')}
                      >
                        Start
                      </button>
                    {/if}
                    <button
                      type="button"
                      class="cp-mini-btn"
                      class:cp-mini-active={activeLogContainer?.id === ctr.id}
                      onclick={() => openDockerLogs(ctr.id, ctr.name)}
                    >
                      Logs
                    </button>
                  </div>
                </div>

                {#if activeLogContainer?.id === ctr.id}
                  <div class="cp-logs-box">
                    <div class="cp-logs-head">
                      <span>Logs: {activeLogContainer.name}</span>
                      <button type="button" class="cp-link-btn" onclick={() => openDockerLogs(ctr.id, ctr.name)}>Fechar</button>
                    </div>
                    <pre class="cp-logs-pre">{loadingLogs ? 'Carregando logs...' : (logsText || '(sem logs recentes)')}</pre>
                  </div>
                {/if}
              </div>
            {/each}
          </div>
        {/if}
      {:else}
        <!-- Power Controls -->
        {#if powerFeedback}
          <div class="cp-alert cp-alert-ok">{powerFeedback}</div>
        {/if}

        {#if !overview.allow_power}
          <div class="cp-empty">Ações de energia desativadas (`COCKPIT_ALLOW_POWER=false`).</div>
        {:else}
          <div class="cp-power-grid">
            {#each powerSpecs as spec}
              <button
                type="button"
                class="cp-power-btn"
                class:cp-power-danger={spec.danger}
                class:cp-power-selected={pendingPower?.action === spec.action}
                onclick={() => selectPowerAction(spec)}
              >
                <span>{spec.label}</span>
              </button>
            {/each}
          </div>

          {#if pendingPower}
            <div class="cp-confirm-box">
              <div class="cp-confirm-title">Confirmar: {pendingPower.label}</div>
              <div class="cp-confirm-desc">{pendingPower.desc}</div>
              {#if pendingPower.action !== 'lock'}
                <input
                  type="text"
                  bind:value={powerConfirmInput}
                  placeholder={`Digite ${pendingPower.word} para confirmar`}
                  class="cp-confirm-input"
                />
              {/if}
              <div class="cp-confirm-actions">
                <button
                  type="button"
                  class="cp-mini-btn"
                  disabled={powerBusy}
                  onclick={() => {
                    pendingPower = null;
                    powerConfirmInput = '';
                  }}
                >
                  Cancelar
                </button>
                <button
                  type="button"
                  class="cp-mini-btn cp-mini-danger"
                  disabled={!canSubmitPower || powerBusy}
                  onclick={submitPowerAction}
                >
                  {powerBusy ? 'Executando...' : `Executar (${pendingPower.word})`}
                </button>
              </div>
            </div>
          {/if}
        {/if}
      {/if}
    {/if}
  </div>
{/snippet}

{#if expanded}
  <div class="cp-modal-backdrop" role="dialog" aria-modal="true" aria-label="AGY Cockpit">
    <button type="button" class="cp-modal-scrim" aria-label="Fechar Cockpit" onclick={() => (expanded = false)}></button>
    <div class="cp-modal-dialog">
      {@render cockpitBody()}
    </div>
  </div>
{:else}
  {@render cockpitBody()}
{/if}

<style>
  .cp-root {
    background: var(--bg-input);
    border: 1px solid var(--border-subtle);
    border-radius: 0.75rem;
    padding: 0.75rem;
    display: flex;
    flex-direction: column;
    gap: 0.65rem;
    color: var(--text-primary);
  }
  .cp-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.5rem;
  }
  .cp-title-row {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    min-width: 0;
  }
  .cp-dot {
    width: 0.45rem;
    height: 0.45rem;
    border-radius: 999px;
    background: var(--text-faint);
    flex-shrink: 0;
  }
  .cp-dot-on {
    background: #10b981;
  }
  .cp-title {
    font-size: 0.75rem;
    font-weight: 700;
    color: var(--text-primary);
  }
  .cp-host {
    font-size: 0.65rem;
    font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
    color: var(--text-faint);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .cp-head-actions {
    display: flex;
    align-items: center;
    gap: 0.25rem;
    flex-shrink: 0;
  }
  .cp-icon-btn {
    padding: 0.28rem;
    border-radius: 0.45rem;
    background: var(--bg-elevated);
    border: 1px solid var(--border-subtle);
    color: var(--text-muted);
    cursor: pointer;
    display: inline-flex;
    align-items: center;
    justify-content: center;
  }
  .cp-icon-btn:hover {
    color: var(--accent-fg);
    border-color: var(--border-strong);
  }
  .cp-tabs {
    display: grid;
    grid-template-columns: repeat(4, minmax(0, 1fr));
    gap: 0.25rem;
    background: var(--bg-elevated);
    border: 1px solid var(--border-subtle);
    border-radius: 0.55rem;
    padding: 0.2rem;
  }
  .cp-tab {
    border: 1px solid transparent;
    background: transparent;
    color: var(--text-muted);
    font-size: 0.66rem;
    font-weight: 600;
    padding: 0.3rem 0.2rem;
    border-radius: 0.4rem;
    cursor: pointer;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .cp-tab-active {
    background: var(--accent-soft);
    border-color: var(--accent-border);
    color: var(--accent-fg);
  }
  .cp-chart-grid {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 0.5rem;
  }
  .cp-chart-grid-wide {
    grid-template-columns: repeat(auto-fit, minmax(155px, 1fr));
  }
  .cp-metric-card {
    background: var(--bg-elevated);
    border: 1px solid var(--border-subtle);
    border-radius: 0.6rem;
    padding: 0.45rem 0.55rem;
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
    min-width: 0;
  }
  .cp-metric-head {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: 0.25rem;
  }
  .cp-metric-title {
    font-size: 0.66rem;
    font-weight: 600;
    color: var(--text-secondary);
  }
  .cp-metric-val {
    font-size: 0.72rem;
    font-weight: 700;
    font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  }
  .cp-svg {
    width: 100%;
    height: 36px;
    display: block;
  }
  .cp-metric-sub {
    font-size: 0.62rem;
    color: var(--text-faint);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  }
  .cp-list {
    display: flex;
    flex-direction: column;
    gap: 0.35rem;
    max-height: 220px;
    overflow-y: auto;
  }
  .cp-list-tall {
    max-height: 56vh;
  }
  .cp-row {
    background: var(--bg-elevated);
    border: 1px solid var(--border-subtle);
    border-radius: 0.5rem;
    padding: 0.4rem 0.55rem;
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.5rem;
  }
  .cp-row-main {
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 0.1rem;
  }
  .cp-row-name {
    font-size: 0.72rem;
    font-weight: 600;
    color: var(--text-primary);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .cp-row-meta {
    font-size: 0.62rem;
    color: var(--text-faint);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .cp-row-badges {
    display: flex;
    align-items: center;
    gap: 0.25rem;
    flex-shrink: 0;
  }
  .cp-badge {
    font-size: 0.63rem;
    font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
    padding: 0.12rem 0.38rem;
    border-radius: 0.35rem;
    background: var(--bg-chip);
    color: var(--text-secondary);
  }
  .cp-badge-gpu {
    background: var(--accent-soft);
    border: 1px solid var(--accent-border);
    color: var(--accent-fg);
  }
  .cp-docker-card {
    background: var(--bg-elevated);
    border: 1px solid var(--border-subtle);
    border-radius: 0.55rem;
    padding: 0.45rem 0.55rem;
    display: flex;
    flex-direction: column;
    gap: 0.4rem;
  }
  .cp-docker-top {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.4rem;
  }
  .cp-docker-actions {
    display: flex;
    align-items: center;
    gap: 0.25rem;
    flex-shrink: 0;
  }
  .cp-mini-btn {
    padding: 0.22rem 0.48rem;
    border-radius: 0.4rem;
    font-size: 0.66rem;
    font-weight: 600;
    border: 1px solid var(--border-subtle);
    background: var(--bg-chip);
    color: var(--text-secondary);
    cursor: pointer;
  }
  .cp-mini-btn:disabled {
    opacity: 0.45;
    cursor: not-allowed;
  }
  .cp-mini-ok {
    color: #10b981;
    border-color: rgba(16, 185, 129, 0.35);
  }
  .cp-mini-danger {
    color: #fb7185;
    border-color: rgba(244, 63, 94, 0.38);
  }
  .cp-mini-active {
    background: var(--accent-soft);
    border-color: var(--accent-border);
    color: var(--accent-fg);
  }
  .cp-logs-box {
    background: var(--code-bg);
    border: 1px solid var(--border-subtle);
    border-radius: 0.45rem;
    overflow: hidden;
  }
  .cp-logs-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 0.25rem 0.5rem;
    background: var(--code-header-bg);
    font-size: 0.62rem;
    color: var(--text-muted);
  }
  .cp-link-btn {
    background: transparent;
    border: none;
    color: var(--accent-fg);
    font-size: 0.62rem;
    cursor: pointer;
  }
  .cp-logs-pre {
    margin: 0;
    padding: 0.45rem;
    font-size: 0.64rem;
    line-height: 1.45;
    max-height: 150px;
    overflow: auto;
    color: var(--text-secondary);
    white-space: pre-wrap;
    word-break: break-all;
  }
  .cp-power-grid {
    display: grid;
    grid-template-columns: repeat(3, minmax(0, 1fr));
    gap: 0.35rem;
  }
  .cp-power-btn {
    padding: 0.45rem 0.3rem;
    border-radius: 0.5rem;
    border: 1px solid var(--border-subtle);
    background: var(--bg-elevated);
    color: var(--text-primary);
    font-size: 0.68rem;
    font-weight: 600;
    cursor: pointer;
    text-align: center;
  }
  .cp-power-danger {
    color: #fb7185;
    border-color: rgba(244, 63, 94, 0.3);
  }
  .cp-power-selected {
    background: rgba(244, 63, 94, 0.12);
    border-color: rgba(244, 63, 94, 0.55);
  }
  .cp-confirm-box {
    background: var(--bg-elevated);
    border: 1px solid rgba(244, 63, 94, 0.38);
    border-radius: 0.55rem;
    padding: 0.55rem;
    display: flex;
    flex-direction: column;
    gap: 0.4rem;
  }
  .cp-confirm-title {
    font-size: 0.72rem;
    font-weight: 700;
    color: #fb7185;
  }
  .cp-confirm-desc {
    font-size: 0.66rem;
    color: var(--text-muted);
    line-height: 1.4;
  }
  .cp-confirm-input {
    width: 100%;
    background: var(--bg-input);
    border: 1px solid var(--border-subtle);
    border-radius: 0.45rem;
    padding: 0.35rem 0.5rem;
    font-size: 0.7rem;
    color: var(--text-primary);
  }
  .cp-confirm-actions {
    display: flex;
    justify-content: flex-end;
    gap: 0.35rem;
  }
  .cp-alert {
    font-size: 0.68rem;
    padding: 0.4rem 0.55rem;
    border-radius: 0.5rem;
  }
  .cp-alert-err {
    background: rgba(244, 63, 94, 0.12);
    border: 1px solid rgba(244, 63, 94, 0.32);
    color: #fb7185;
  }
  .cp-alert-ok {
    background: rgba(16, 185, 129, 0.12);
    border: 1px solid rgba(16, 185, 129, 0.32);
    color: #10b981;
  }
  .cp-empty {
    font-size: 0.68rem;
    color: var(--text-faint);
    text-align: center;
    padding: 0.75rem 0.25rem;
  }
  .cp-modal-backdrop {
    position: fixed;
    inset: 0;
    z-index: 60;
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 1rem;
  }
  .cp-modal-scrim {
    position: fixed;
    inset: 0;
    background: var(--bg-backdrop);
    backdrop-filter: blur(4px);
    border: none;
    cursor: default;
  }
  .cp-modal-dialog {
    position: relative;
    width: 100%;
    max-width: 44rem;
    max-height: 88vh;
    overflow-y: auto;
    border-radius: 0.9rem;
    box-shadow: var(--shadow-popover);
  }
</style>
