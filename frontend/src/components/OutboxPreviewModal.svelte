<script lang="ts">
  import { fade, fly } from "svelte/transition";
  import { cubicOut } from "svelte/easing";
  import Icon from "./Icon.svelte";
  import { Button } from "../lib/components/ui/button/index.js";
  import { Badge } from "../lib/components/ui/badge/index.js";
  import { Card } from "../lib/components/ui/card/index.js";
  import { outboxFileUrl } from "../lib/api";

  export interface PreviewTarget {
    name: string;
    kind: "image" | "pdf";
    size?: number;
  }

  let {
    target = $bindable<PreviewTarget | null>(null),
  }: {
    target: PreviewTarget | null;
  } = $props();

  let zoomFull = $state(false);

  function close() {
    zoomFull = false;
    target = null;
  }

  function formatSize(bytes?: number): string {
    if (typeof bytes !== "number" || bytes <= 0) return "";
    if (bytes < 1024) return `${bytes} B`;
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
    return `${(bytes / (1024 * 1024)).toFixed(2)} MB`;
  }
</script>

<svelte:window
  onkeydown={(e) => {
    if (target && e.key === "Escape") {
      close();
    }
  }}
/>

{#if target}
  <div
    transition:fade={{ duration: 150 }}
    class="fixed inset-0 bg-[var(--bg-backdrop)] backdrop-blur-md z-50 flex items-center justify-center p-2 sm:p-5"
  >
    <button
      type="button"
      aria-label="Fechar visualização"
      onclick={close}
      class="absolute inset-0 cursor-default"
    ></button>

    <div
      transition:fly={{ y: 16, duration: 190, easing: cubicOut }}
      class="relative w-full max-w-4xl max-h-[92dvh] flex flex-col"
    >
      <Card class="rounded-2xl overflow-hidden flex flex-col max-h-[92dvh] shadow-2xl">
        <div class="px-3.5 py-2.5 border-b border-border flex items-center justify-between gap-2 shrink-0">
          <div class="flex items-center gap-2 min-w-0">
            <Badge variant="accent" size="xs" class="font-mono uppercase shrink-0">
              {target.kind === "pdf" ? "PDF" : "Imagem"}
            </Badge>
            <span class="text-xs sm:text-sm font-semibold text-foreground truncate">
              {target.name}
            </span>
            {#if formatSize(target.size)}
              <span class="text-[10px] font-mono text-muted-foreground shrink-0 hidden sm:inline">
                {formatSize(target.size)}
              </span>
            {/if}
          </div>

          <div class="flex items-center gap-1 shrink-0">
            {#if target.kind === "image"}
              <Button
                variant={zoomFull ? "default" : "outline"}
                size="xs"
                onclick={() => (zoomFull = !zoomFull)}
                title={zoomFull ? "Ajustar à tela" : "Tamanho original"}
              >
                <Icon name="search" size={12} />
                <span class="hidden sm:inline">{zoomFull ? "Ajustar" : "100%"}</span>
              </Button>
            {/if}
            <Button
              variant="outline"
              size="xs"
              href={outboxFileUrl(target.name, true)}
              target="_blank"
              rel="noopener noreferrer"
              title="Abrir em nova aba"
            >
              <Icon name="link" size={12} />
              <span class="hidden sm:inline">Abrir</span>
            </Button>
            <Button
              variant="default"
              size="xs"
              href={outboxFileUrl(target.name, false)}
              download={target.name}
              title="Baixar arquivo"
            >
              <Icon name="download" size={12} />
              <span>Baixar</span>
            </Button>
            <Button
              variant="ghost"
              size="icon-sm"
              onclick={close}
              aria-label="Fechar"
            >
              <Icon name="close" size={16} />
            </Button>
          </div>
        </div>

        <div class="flex-1 min-h-0 overflow-auto bg-muted/40 flex items-center justify-center p-2 sm:p-4">
          {#if target.kind === "image"}
            <img
              src={outboxFileUrl(target.name, true)}
              alt={target.name}
              class={zoomFull
                ? "max-w-none max-h-none rounded-lg shadow-lg"
                : "max-w-full max-h-[76dvh] object-contain rounded-lg shadow-md"}
            />
          {:else}
            <iframe
              title={`Preview ${target.name}`}
              src={outboxFileUrl(target.name, true)}
              class="w-full h-[74dvh] min-h-[380px] rounded-xl border border-border bg-white"
            ></iframe>
          {/if}
        </div>
      </Card>
    </div>
  </div>
{/if}
