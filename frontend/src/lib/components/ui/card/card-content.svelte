<script lang="ts">
  import { cn, type WithElementRef } from '../../../utils';
  import type { HTMLAttributes } from 'svelte/elements';

  let {
    ref = $bindable(),
    class: className,
    children,
    ...restProps
  }: WithElementRef<HTMLAttributes<HTMLDivElement>> = $props();
</script>

<div bind:this={ref} data-slot="card-content" class={cn('p-4 pt-2', className)} {...restProps}>
  {@render children?.()}
</div>
