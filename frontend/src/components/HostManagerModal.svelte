<script lang="ts">
  import { fade, fly } from "svelte/transition";
  import { cubicOut } from "svelte/easing";
  import Icon from "./Icon.svelte";
  import { Button } from "../lib/components/ui/button/index.js";
  import { Card } from "../lib/components/ui/card/index.js";
  import { Badge } from "../lib/components/ui/badge/index.js";
  import { Input } from "../lib/components/ui/input/index.js";
  import {
    activateHost,
    createHost,
    deleteHost,
    updateHost,
    type HostProfile,
  } from "../lib/api";

  let {
    open = $bindable(false),
    hosts = $bindable<HostProfile[]>([]),
    activeHostId = $bindable(""),
    onActivated,
  }: {
    open: boolean;
    hosts: HostProfile[];
    activeHostId: string;
    onActivated: () => void;
  } = $props();

  let editingId = $state<string | null>(null);
  let formOpen = $state(false);
  let name = $state("");
  let host = $state("");
  let sshUser = $state("ai");
  let sshPort = $state("22");
  let wolMac = $state("");
  let osHint = $state<"linux" | "windows" | "darwin" | "auto">("linux");
  let saving = $state(false);
  let errorMsg = $state("");

  function openNewForm() {
    editingId = null;
    name = "";
    host = "";
    sshUser = "ai";
    sshPort = "22";
    wolMac = "";
    osHint = "linux";
    errorMsg = "";
    formOpen = true;
  }

  function openEditForm(item: HostProfile) {
    editingId = item.id;
    name = item.name;
    host = item.host;
    sshUser = item.ssh_user;
    sshPort = item.ssh_port || "22";
    wolMac = item.wol_mac || "";
    osHint = (item.os_hint as any) || "linux";
    errorMsg = "";
    formOpen = true;
  }

  async function saveForm(e: Event) {
    e.preventDefault();
    if (saving) return;
    if (!host.trim() || !sshUser.trim()) {
      errorMsg = "IP/Host e Usuário SSH são obrigatórios.";
      return;
    }
    saving = true;
    errorMsg = "";
    try {
      const payload = {
        name: name.trim() || host.trim(),
        host: host.trim(),
        ssh_user: sshUser.trim(),
        ssh_port: sshPort.trim() || "22",
        wol_mac: wolMac.trim(),
        os_hint: osHint,
      };
      if (editingId) {
        const res = await updateHost(editingId, payload);
        hosts = hosts.map((h) => (h.id === editingId ? res.host : h));
      } else {
        const res = await createHost(payload);
        hosts = [...hosts, res.host];
      }
      formOpen = false;
      onActivated();
    } catch (err: any) {
      errorMsg = err?.message || "Erro ao salvar host";
    } finally {
      saving = false;
    }
  }

  async function handleActivate(id: string) {
    if (saving) return;
    saving = true;
    errorMsg = "";
    try {
      const res = await activateHost(id);
      hosts = res.hosts;
      activeHostId = res.active_host_id;
      onActivated();
      open = false;
    } catch (err: any) {
      errorMsg = err?.message || "Erro ao alternar host";
    } finally {
      saving = false;
    }
  }

  async function handleDelete(id: string) {
    if (saving) return;
    saving = true;
    errorMsg = "";
    try {
      const res = await deleteHost(id);
      hosts = res.hosts;
      activeHostId = res.active_host_id;
      onActivated();
    } catch (err: any) {
      errorMsg = err?.message || "Erro ao remover host";
    } finally {
      saving = false;
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
      aria-label="Fechar gerenciador de hosts"
      onclick={() => (open = false)}
      class="absolute inset-0 cursor-default"
    ></button>

    <div
      transition:fly={{ y: 20, duration: 200, easing: cubicOut }}
      class="relative w-full sm:max-w-lg max-h-[90dvh] flex flex-col"
    >
      <Card class="rounded-t-2xl sm:rounded-2xl max-h-[90dvh] flex flex-col shadow-2xl overflow-hidden">
        <div class="p-4 border-b border-border flex items-center justify-between gap-2 shrink-0">
          <div class="flex items-center gap-2 min-w-0">
            <Icon name="cpu" size={18} class="text-accent-foreground shrink-0" />
            <div>
              <h2 class="text-sm font-bold text-foreground">Perfis Multi-Host</h2>
              <p class="text-[11px] text-muted-foreground">
                Alterne entre PC Principal, Dell G15, Servidor Linux ou Notebook.
              </p>
            </div>
          </div>
          <div class="flex items-center gap-1.5 shrink-0">
            {#if !formOpen}
              <Button variant="default" size="xs" onclick={openNewForm}>
                <Icon name="plus" size={13} />
                <span>Novo Host</span>
              </Button>
            {/if}
            <Button
              variant="ghost"
              size="icon-sm"
              onclick={() => (open = false)}
              aria-label="Fechar"
            >
              <Icon name="close" size={16} />
            </Button>
          </div>
        </div>

        <div class="p-3.5 overflow-y-auto flex-1 space-y-3">
          {#if errorMsg}
            <div class="p-2.5 rounded-xl bg-rose-500/12 border border-rose-500/35 text-rose-400 text-xs">
              {errorMsg}
            </div>
          {/if}

          {#if formOpen}
            <form onsubmit={saveForm} class="space-y-3">
              <div class="grid grid-cols-1 sm:grid-cols-2 gap-2.5">
                <div>
                  <label for="host-name" class="block text-[11px] font-semibold text-muted-foreground mb-1">Nome / Apelido</label>
                  <Input id="host-name" bind:value={name} placeholder="Ex: Dell G15 / CachyCore" />
                </div>
                <div>
                  <label for="host-ip" class="block text-[11px] font-semibold text-muted-foreground mb-1">IP ou Hostname</label>
                  <Input id="host-ip" bind:value={host} placeholder="192.168.1.220" required />
                </div>
                <div>
                  <label for="host-user" class="block text-[11px] font-semibold text-muted-foreground mb-1">Usuário SSH</label>
                  <Input id="host-user" bind:value={sshUser} placeholder="ryanslx" required />
                </div>
                <div>
                  <label for="host-port" class="block text-[11px] font-semibold text-muted-foreground mb-1">Porta SSH</label>
                  <Input id="host-port" bind:value={sshPort} placeholder="22" />
                </div>
                <div>
                  <label for="host-mac" class="block text-[11px] font-semibold text-muted-foreground mb-1">MAC Wake-on-LAN (opcional)</label>
                  <Input id="host-mac" bind:value={wolMac} placeholder="aa:bb:cc:dd:ee:ff" class="font-mono" />
                </div>
                <div>
                  <label for="host-os" class="block text-[11px] font-semibold text-muted-foreground mb-1">Sistema</label>
                  <select
                    id="host-os"
                    bind:value={osHint}
                    class="w-full h-9 rounded-xl bg-[var(--bg-input)] border border-[var(--border-subtle)] px-3 text-xs text-foreground"
                  >
                    <option value="linux">Linux</option>
                    <option value="windows">Windows</option>
                    <option value="darwin">macOS</option>
                    <option value="auto">Auto</option>
                  </select>
                </div>
              </div>
              <div class="flex items-center justify-end gap-2 pt-1">
                <Button type="button" variant="outline" size="sm" onclick={() => (formOpen = false)}>
                  Cancelar
                </Button>
                <Button type="submit" size="sm" disabled={saving}>
                  <Icon name="check" size={14} />
                  <span>{saving ? "Salvando..." : editingId ? "Atualizar" : "Adicionar Host"}</span>
                </Button>
              </div>
            </form>
          {:else}
            <div class="space-y-2">
              {#each hosts as h (h.id)}
                <Card
                  variant="soft"
                  class="p-3 rounded-xl flex items-center justify-between gap-2.5 {h.id === activeHostId || h.is_active
                    ? 'border-[var(--accent-border)] bg-[var(--accent-soft)]'
                    : ''}"
                >
                  <button
                    type="button"
                    onclick={() => handleActivate(h.id)}
                    class="min-w-0 flex-1 text-left"
                  >
                    <div class="flex items-center gap-1.5">
                      <span class="text-xs font-bold text-foreground truncate">{h.name}</span>
                      {#if h.id === activeHostId || h.is_active}
                        <Badge variant="accent" size="xs">Ativo</Badge>
                      {/if}
                      <Badge variant="outline" size="xs" class="font-mono uppercase">{h.os_hint || "linux"}</Badge>
                    </div>
                    <div class="text-[11px] font-mono text-muted-foreground truncate mt-0.5">
                      {h.ssh_user}@{h.host}:{h.ssh_port || "22"}
                      {#if h.wol_mac}
                        · WoL {h.wol_mac}
                      {/if}
                    </div>
                  </button>
                  <div class="flex items-center gap-1 shrink-0">
                    {#if !(h.id === activeHostId || h.is_active)}
                      <Button variant="outline" size="xs" onclick={() => handleActivate(h.id)} disabled={saving}>
                        Usar
                      </Button>
                    {/if}
                    <Button
                      variant="ghost"
                      size="icon-xs"
                      onclick={() => openEditForm(h)}
                      title="Editar host"
                    >
                      <Icon name="edit" size={14} />
                    </Button>
                    {#if hosts.length > 1}
                      <Button
                        variant="ghost"
                        size="icon-xs"
                        onclick={() => handleDelete(h.id)}
                        class="hover:text-rose-400 hover:bg-rose-500/10"
                        title="Excluir host"
                      >
                        <Icon name="trash" size={14} />
                      </Button>
                    {/if}
                  </div>
                </Card>
              {/each}
            </div>
          {/if}
        </div>
      </Card>
    </div>
  </div>
{/if}
