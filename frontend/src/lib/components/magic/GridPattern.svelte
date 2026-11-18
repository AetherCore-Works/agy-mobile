<script lang="ts">
  import { cn } from '../../utils';
  import type { SVGAttributes } from 'svelte/elements';

  type Props = SVGAttributes<SVGSVGElement> & {
    width?: number;
    height?: number;
    x?: number;
    y?: number;
    strokeDashArray?: string;
    squares?: Array<[x: number, y: number]>;
    fillColor?: string;
    strokeWidth?: number;
  };

  let {
    width = 36,
    height = 36,
    x = -1,
    y = -1,
    strokeDashArray = '',
    squares = [[1, 1], [3, 2], [2, 4]],
    class: className = '',
    fillColor = 'currentColor',
    strokeWidth = 1,
    ...restProps
  }: Props = $props();

  const uid = $props.id();
</script>

<svg
  aria-hidden="true"
  class={cn('pointer-events-none absolute inset-0 h-full w-full text-foreground/10', className)}
  {...restProps}
  stroke={fillColor}
  stroke-width={strokeWidth}
>
  <defs>
    <pattern id={uid} {width} {height} patternUnits="userSpaceOnUse" {x} {y}>
      <path d="M.5 {height}V.5H{width}" fill="none" stroke-dasharray={strokeDashArray} />
    </pattern>
  </defs>
  <rect width="100%" height="100%" stroke-width={0} fill="url(#{uid})" />
  {#if squares}
    <svg {x} {y} class="overflow-visible">
      {#each squares as sq}
        <rect
          stroke="none"
          fill={fillColor}
          fill-opacity="0.18"
          width={width - 1}
          height={height - 1}
          x={sq[0] * width + 1}
          y={sq[1] * height + 1}
        />
      {/each}
    </svg>
  {/if}
</svg>
