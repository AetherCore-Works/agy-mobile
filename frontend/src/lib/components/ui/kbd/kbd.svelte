<script lang="ts">
  import { cn, type WithElementRef } from '../../../utils';
  import type { HTMLAttributes } from 'svelte/elements';

  let {
    ref = $bindable(),
    class: className,
    children,
    ...restProps
  }: WithElementRef<HTMLAttributes<HTMLElement>> = $props();
</script>

<kbd
  bind:this={ref}
  data-slot="kbd"
  class={cn(
    'pointer-events-none inline-flex h-4.5 min-w-4.5 items-center justify-center gap-1 rounded border border-border bg-muted px-1 font-mono text-[10px] font-medium text-muted-foreground select-none',
    className,
  )}
  {...restProps}
>
  {@render children?.()}
</kbd>
