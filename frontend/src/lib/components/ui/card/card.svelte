<script module lang="ts">
  import type { WithElementRef } from 'bits-ui';
  import type { HTMLAttributes } from 'svelte/elements';
  import { type VariantProps, tv } from 'tailwind-variants';

  export const cardVariants = tv({
    base: 'rounded-2xl bg-card text-card-foreground theme-surface',
    variants: {
      variant: {
        default: 'border border-border shadow-sm ring-1 ring-foreground/[0.04]',
        soft: 'border border-border/60 bg-muted/65',
        mixed: 'border border-border/80 bg-card/80 backdrop-blur-md shadow-md',
      },
    },
    defaultVariants: {
      variant: 'default',
    },
  });

  export type CardVariant = VariantProps<typeof cardVariants>['variant'];

  export type CardProps = WithElementRef<HTMLAttributes<HTMLDivElement>> & {
    variant?: CardVariant;
  };
</script>

<script lang="ts">
  import { cn } from '../../../utils';

  let {
    ref = $bindable(),
    class: className,
    children,
    variant = 'default',
    ...restProps
  }: CardProps = $props();
</script>

<div
  bind:this={ref}
  data-slot="card"
  class={cn(cardVariants({ variant }), className)}
  {...restProps}
>
  {@render children?.()}
</div>
