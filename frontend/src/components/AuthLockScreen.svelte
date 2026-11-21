<script lang="ts">
  import { fade, fly } from "svelte/transition";
  import { cubicOut } from "svelte/easing";
  import Icon from "./Icon.svelte";
  import { Button } from "../lib/components/ui/button/index.js";
  import { Card } from "../lib/components/ui/card/index.js";
  import { Input } from "../lib/components/ui/input/index.js";
  import { Badge } from "../lib/components/ui/badge/index.js";
  import {
    loginWithSecret,
    loginWithWebAuthn,
  } from "../lib/api";

  let {
    webauthnCount = 0,
    onUnlocked,
  }: {
    webauthnCount?: number;
    onUnlocked: () => void;
  } = $props();

  let secretInput = $state("");
  let showNumericPad = $state(true);
  let loading = $state(false);
  let errorMsg = $state("");

  const canUseWebAuthn = $derived(
    typeof window !== "undefined" &&
      window.isSecureContext &&
      "PublicKeyCredential" in window &&
      webauthnCount > 0,
  );

  async function submitSecret(e?: Event) {
    e?.preventDefault();
    const clean = secretInput.trim();
    if (!clean || loading) return;
    loading = true;
    errorMsg = "";
    try {
      await loginWithSecret(clean);
      secretInput = "";
      onUnlocked();
    } catch (err: any) {
      errorMsg = err?.message || "PIN ou token inválido";
    } finally {
      loading = false;
    }
  }

  async function handleBiometric() {
    if (loading || !canUseWebAuthn) return;
    loading = true;
    errorMsg = "";
    try {
      await loginWithWebAuthn();
      onUnlocked();
    } catch (err: any) {
      errorMsg = err?.message || "Falha na autenticação biométrica";
    } finally {
      loading = false;
    }
  }

  function appendDigit(d: string) {
    if (loading || secretInput.length >= 32) return;
    secretInput += d;
    errorMsg = "";
  }

  function backspace() {
    if (loading || !secretInput) return;
    secretInput = secretInput.slice(0, -1);
  }
</script>

<div
  transition:fade={{ duration: 180 }}
  class="fixed inset-0 z-50 bg-background/95 backdrop-blur-2xl flex items-center justify-center p-4 pt-[env(safe-area-inset-top)] pb-[env(safe-area-inset-bottom)] select-none"
>
  <div
    transition:fly={{ y: 18, duration: 220, easing: cubicOut }}
    class="w-full max-w-sm"
  >
    <Card class="p-5 sm:p-6 space-y-5 shadow-2xl border-border/90">
      <div class="flex flex-col items-center text-center space-y-2">
        <div
          class="w-12 h-12 rounded-2xl bg-[var(--accent-soft)] border border-[var(--accent-border)] flex items-center justify-center shadow-sm"
        >
          <Icon name="shield" size={22} class="text-accent-foreground" />
        </div>
        <div class="space-y-0.5">
          <div class="flex items-center justify-center gap-2">
            <h1 class="text-base font-bold tracking-tight text-foreground">
              AGY Gateway Protegido
            </h1>
            <Badge variant="accent" size="xs" class="font-mono">LOCK</Badge>
          </div>
          <p class="text-xs text-muted-foreground">
            Digite seu PIN, Token de acesso ou use biometria.
          </p>
        </div>
      </div>

      <form onsubmit={submitSecret} class="space-y-3">
        <Input
          type="password"
          bind:value={secretInput}
          placeholder={showNumericPad ? "PIN ou Token..." : "GATEWAY_AUTH_TOKEN..."}
          autocomplete="current-password"
          inputmode={showNumericPad ? "numeric" : "text"}
          class="text-center tracking-widest font-mono h-10 text-base"
          disabled={loading}
        />

        {#if errorMsg}
          <div
            class="px-3 py-2 rounded-xl bg-rose-500/12 border border-rose-500/35 text-rose-400 text-xs text-center"
          >
            {errorMsg}
          </div>
        {/if}

        {#if showNumericPad}
          <div class="grid grid-cols-3 gap-2 pt-1">
            {#each ["1", "2", "3", "4", "5", "6", "7", "8", "9"] as digit}
              <Button
                type="button"
                variant="outline"
                onclick={() => appendDigit(digit)}
                disabled={loading}
                class="h-11 text-base font-mono font-semibold"
              >
                {digit}
              </Button>
            {/each}
            <Button
              type="button"
              variant="ghost"
              onclick={() => (showNumericPad = false)}
              disabled={loading}
              class="h-11 text-[11px] font-mono text-muted-foreground"
              title="Digitar token completo"
            >
              ABC
            </Button>
            <Button
              type="button"
              variant="outline"
              onclick={() => appendDigit("0")}
              disabled={loading}
              class="h-11 text-base font-mono font-semibold"
            >
              0
            </Button>
            <Button
              type="button"
              variant="ghost"
              onclick={backspace}
              disabled={loading || !secretInput}
              class="h-11 text-xs font-mono text-muted-foreground"
              aria-label="Apagar"
            >
              ⌫
            </Button>
          </div>
        {:else}
          <div class="flex justify-end">
            <Button
              type="button"
              variant="link"
              size="xs"
              onclick={() => (showNumericPad = true)}
              class="px-0 h-auto text-[11px]"
            >
              Usar teclado numérico
            </Button>
          </div>
        {/if}

        <div class="flex flex-col gap-2 pt-1">
          <Button
            type="submit"
            disabled={loading || !secretInput.trim()}
            class="w-full h-10 font-semibold"
          >
            <Icon name="check" size={15} />
            <span>{loading ? "Verificando..." : "Desbloquear"}</span>
          </Button>

          {#if canUseWebAuthn}
            <Button
              type="button"
              variant="outline"
              onclick={handleBiometric}
              disabled={loading}
              class="w-full h-10 border-[var(--accent-border)] text-accent-foreground"
            >
              <Icon name="shield" size={15} />
              <span>Desbloquear com Biometria / Passkey</span>
            </Button>
          {/if}
        </div>
      </form>
    </Card>
  </div>
</div>
