<script lang="ts">
  import { cn, type WithElementRef, type WithoutChildren } from '../../../utils';
  import type { HTMLTextareaAttributes } from 'svelte/elements';

  let {
    ref = $bindable(),
    value = $bindable(),
    class: className,
    'data-slot': dataSlot = 'textarea',
    ...restProps
  }: WithoutChildren<WithElementRef<HTMLTextareaAttributes>> = $props();
</script>

<textarea
  bind:this={ref}
  data-slot={dataSlot}
  class={cn(
    'flex w-full rounded-xl border border-border bg-muted px-3 py-2 text-base sm:text-sm text-foreground placeholder:text-muted-foreground transition-colors outline-none focus-visible:border-ring focus-visible:ring-2 focus-visible:ring-ring/30 disabled:cursor-not-allowed disabled:opacity-50',
    className,
  )}
  bind:value
  {...restProps}
></textarea>
