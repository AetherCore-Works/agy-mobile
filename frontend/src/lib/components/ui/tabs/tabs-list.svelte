<script lang="ts">
  import { Tabs as TabsPrimitive } from 'bits-ui';
  import { cn } from '../../../utils';

  let {
    ref = $bindable(null),
    class: className,
    ...restProps
  }: TabsPrimitive.ListProps = $props();
</script>

<TabsPrimitive.List
  bind:ref
  data-slot="tabs-list"
  class={cn(
    'inline-flex items-center justify-center rounded-xl border border-border bg-muted p-1 text-muted-foreground',
    className,
  )}
  {...restProps}
/>
