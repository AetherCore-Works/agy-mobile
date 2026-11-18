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

<div
  bind:this={ref}
  data-slot="card-footer"
  class={cn('flex items-center p-4 pt-0', className)}
  {...restProps}
>
  {@render children?.()}
</div>
