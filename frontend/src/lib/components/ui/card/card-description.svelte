<script lang="ts">
  import { cn, type WithElementRef } from '../../../utils';
  import type { HTMLAttributes } from 'svelte/elements';

  let {
    ref = $bindable(),
    class: className,
    children,
    ...restProps
  }: WithElementRef<HTMLAttributes<HTMLParagraphElement>> = $props();
</script>

<p
  bind:this={ref}
  data-slot="card-description"
  class={cn('text-xs text-muted-foreground leading-relaxed', className)}
  {...restProps}
>
  {@render children?.()}
</p>
