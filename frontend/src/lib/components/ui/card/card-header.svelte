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
  data-slot="card-header"
  class={cn('flex flex-col gap-1 p-4 pb-2', className)}
  {...restProps}
>
  {@render children?.()}
</div>
