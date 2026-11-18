<script lang="ts">
  import Icon from "./Icon.svelte";
  import { Badge } from "../lib/components/ui/badge/index.js";
  import { Button } from "../lib/components/ui/button/index.js";
  import { Card } from "../lib/components/ui/card/index.js";
  import { outboxFileUrl, type OutboxFile } from "../lib/api";
  import type { PreviewTarget } from "./OutboxPreviewModal.svelte";

  const IMAGE_EXTS = new Set([".png", ".jpg", ".jpeg", ".webp", ".gif", ".svg"]);

  let {
    content = "",
    messageFiles = [],
    outboxFiles = [],
    outboxLoaded = false,
    onOpenPreview,
  }: {
    content?: string;
    messageFiles?: string[];
    outboxFiles?: OutboxFile[];
    outboxLoaded?: boolean;
    onOpenPreview?: (target: PreviewTarget) => void;
  } = $props();

  let expandedPdf = $state<Record<string, boolean>>({});
  let failedImages = $state<Record<string, boolean>>({});

  function extOf(name: string): string {
    const idx = name.lastIndexOf(".");
    return idx >= 0 ? name.slice(idx).toLowerCase() : "";
  }

  function classifyFile(name: string): "image" | "pdf" | "other" {
    const ext = extOf(name);
    if (IMAGE_EXTS.has(ext)) return "image";
    if (ext === ".pdf") return "pdf";
    return "other";
  }

  function escapeRegex(raw: string): string {
    return raw.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
  }

  function formatSize(bytes?: number): string {
    if (typeof bytes !== "number" || bytes <= 0) return "";
    if (bytes < 1024) return `${bytes} B`;
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
    return `${(bytes / (1024 * 1024)).toFixed(2)} MB`;
  }

  let outboxMap = $derived(new Map(outboxFiles.map((f) => [f.name, f])));

  let items = $derived.by(() => {
    const seen = new Set<string>();
    const result: {
      name: string;
      kind: "image" | "pdf" | "other";
      size?: number;
      available: boolean;
      fromJobArtifact: boolean;
    }[] = [];

    for (const rawName of messageFiles || []) {
      const clean = String(rawName || "").trim();
      if (!clean || seen.has(clean)) continue;
      seen.add(clean);
      const match = outboxMap.get(clean);
      result.push({
        name: clean,
        kind: classifyFile(clean),
        size: match?.size,
        available: !outboxLoaded || Boolean(match),
        fromJobArtifact: true,
      });
    }

    if (content && outboxFiles.length > 0) {
      for (const f of outboxFiles) {
        if (seen.has(f.name)) continue;
        const kind = classifyFile(f.name);
        if (kind === "other") continue;
        const re = new RegExp(`(^|[^a-zA-Z0-9_-])${escapeRegex(f.name)}(?=$|[^a-zA-Z0-9_-])`);
        if (re.test(content)) {
          seen.add(f.name);
          result.push({
            name: f.name,
            kind,
            size: f.size,
            available: true,
            fromJobArtifact: false,
          });
        }
      }
    }

    return result;
  });

  let mediaItems = $derived(
    items.filter((x) => x.kind === "image" || x.kind === "pdf"),
  );
  let otherItems = $derived(items.filter((x) => x.kind === "other"));

  function isPdfExpanded(name: string, totalPdfs: number, index: number): boolean {
    if (name in expandedPdf) return expandedPdf[name];
    return totalPdfs <= 2 && index === 0;
  }
</script>

{#if mediaItems.length > 0 || otherItems.length > 0}
  <div class="mt-3.5 pt-3 border-t border-border/70 space-y-2.5">
    <div class="flex items-center justify-between gap-2">
      <div class="flex items-center gap-1.5 text-[11px] font-semibold text-muted-foreground">
        <Icon name="outbox" size={13} class="text-accent-foreground" />
        <span>Anexos gerados no Outbox</span>
      </div>
      <Badge variant="outline" size="xs" class="font-mono">
        {mediaItems.length + otherItems.length}
      </Badge>
    </div>

    {#if mediaItems.filter((x) => x.kind === "image").length > 0}
      {@const imgs = mediaItems.filter((x) => x.kind === "image")}
      <div
        class="grid gap-2 {imgs.length === 1 ? 'grid-cols-1' : 'grid-cols-1 sm:grid-cols-2'}"
      >
        {#each imgs as img (img.name)}
          {#if img.available && !failedImages[img.name]}
            <Card variant="soft" class="overflow-hidden border-border/80">
              <button
                type="button"
                onclick={() =>
                  onOpenPreview?.({
                    name: img.name,
                    kind: "image",
                    size: img.size,
                  })}
                class="w-full block bg-muted/50 hover:bg-muted/70 transition-colors text-left group relative overflow-hidden"
                title="Toque para ampliar"
              >
                <img
                  src={outboxFileUrl(img.name, true)}
                  alt={img.name}
                  loading="lazy"
                  onerror={() => {
                    failedImages = { ...failedImages, [img.name]: true };
                  }}
                  class="w-full max-h-60 object-contain mx-auto transition-transform duration-200 group-hover:scale-[1.015]"
                />
                <span
                  class="absolute top-2 right-2 px-2 py-0.5 rounded-full bg-background/85 backdrop-blur-sm border border-border text-[10px] font-medium text-foreground flex items-center gap-1 shadow-sm"
                >
                  <Icon name="eye" size={11} class="text-accent-foreground" />
                  <span>Ampliar</span>
                </span>
              </button>
              <div
                class="px-2.5 py-2 border-t border-border/60 flex items-center justify-between gap-2"
              >
                <div class="min-w-0">
                  <div class="text-xs font-medium text-foreground truncate">
                    {img.name}
                  </div>
                  {#if formatSize(img.size)}
                    <div class="text-[10px] font-mono text-muted-foreground">
                      {formatSize(img.size)}
                    </div>
                  {/if}
                </div>
                <Button
                  variant="outline"
                  size="xs"
                  href={outboxFileUrl(img.name, false)}
                  download={img.name}
                  class="shrink-0"
                  title="Baixar imagem"
                >
                  <Icon name="download" size={12} />
                  <span>Baixar</span>
                </Button>
              </div>
            </Card>
          {:else if img.fromJobArtifact}
            <div
              class="px-2.5 py-2 rounded-xl border border-dashed border-border text-[11px] text-muted-foreground flex items-center gap-1.5"
            >
              <Icon name="file" size={12} />
              <span class="truncate">{img.name} (removido do Outbox)</span>
            </div>
          {/if}
        {/each}
      </div>
    {/if}

    {#if mediaItems.filter((x) => x.kind === "pdf").length > 0}
      {@const pdfs = mediaItems.filter((x) => x.kind === "pdf")}
      <div class="space-y-2">
        {#each pdfs as pdf, pdfIdx (pdf.name)}
          {#if pdf.available}
            {@const isOpen = isPdfExpanded(pdf.name, pdfs.length, pdfIdx)}
            <Card variant="soft" class="overflow-hidden border-border/80">
              <div class="px-3 py-2 flex flex-wrap items-center justify-between gap-2">
                <div class="flex items-center gap-2 min-w-0 flex-1">
                  <Badge variant="amber" size="xs" class="font-mono shrink-0">PDF</Badge>
                  <div class="min-w-0">
                    <div class="text-xs font-medium text-foreground truncate">
                      {pdf.name}
                    </div>
                    {#if formatSize(pdf.size)}
                      <div class="text-[10px] font-mono text-muted-foreground">
                        {formatSize(pdf.size)}
                      </div>
                    {/if}
                  </div>
                </div>
                <div class="flex items-center gap-1 shrink-0">
                  <Button
                    variant="outline"
                    size="xs"
                    onclick={() => {
                      expandedPdf = { ...expandedPdf, [pdf.name]: !isOpen };
                    }}
                    title={isOpen ? "Recolher preview" : "Mostrar preview"}
                  >
                    <Icon name={isOpen ? "chevron-up" : "chevron-down"} size={12} />
                    <span>{isOpen ? "Recolher" : "Preview"}</span>
                  </Button>
                  <Button
                    variant="outline"
                    size="xs"
                    onclick={() =>
                      onOpenPreview?.({
                        name: pdf.name,
                        kind: "pdf",
                        size: pdf.size,
                      })}
                    title="Expandir em tela cheia"
                  >
                    <Icon name="eye" size={12} />
                  </Button>
                  <Button
                    variant="outline"
                    size="xs"
                    href={outboxFileUrl(pdf.name, false)}
                    download={pdf.name}
                    title="Baixar PDF"
                  >
                    <Icon name="download" size={12} />
                  </Button>
                </div>
              </div>
              {#if isOpen}
                <div class="border-t border-border/60 bg-muted/40 p-1.5">
                  <iframe
                    title={`PDF ${pdf.name}`}
                    src={outboxFileUrl(pdf.name, true)}
                    loading="lazy"
                    class="w-full h-56 sm:h-72 rounded-lg border border-border bg-white"
                  ></iframe>
                </div>
              {/if}
            </Card>
          {:else if pdf.fromJobArtifact}
            <div
              class="px-2.5 py-2 rounded-xl border border-dashed border-border text-[11px] text-muted-foreground flex items-center gap-1.5"
            >
              <Icon name="file" size={12} />
              <span class="truncate">{pdf.name} (removido do Outbox)</span>
            </div>
          {/if}
        {/each}
      </div>
    {/if}

    {#if otherItems.length > 0}
      <div class="flex flex-wrap gap-1.5">
        {#each otherItems as item (item.name)}
          {#if item.available}
            <Button
              variant="outline"
              size="xs"
              href={outboxFileUrl(item.name, false)}
              download={item.name}
            >
              <Icon name="download" size={12} />
              <span class="truncate max-w-[180px]">{item.name}</span>
              {#if formatSize(item.size)}
                <span class="font-mono text-[10px] text-muted-foreground">
                  {formatSize(item.size)}
                </span>
              {/if}
            </Button>
          {/if}
        {/each}
      </div>
    {/if}
  </div>
{/if}
