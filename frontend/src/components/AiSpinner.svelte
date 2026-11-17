<script lang="ts">
  let {
    size = 18,
    class: className = '',
  }: {
    size?: number;
    class?: string;
  } = $props();
</script>

<span
  class="inline-flex items-center justify-center shrink-0 {className}"
  style="width: {size}px; height: {size}px;"
  aria-hidden="true"
>
  <svg
    width={size}
    height={size}
    viewBox="0 0 24 24"
    fill="none"
    class="overflow-visible"
  >
    <!-- Subtle circular orbit track -->
    <circle
      cx="12"
      cy="12"
      r="8.5"
      stroke="currentColor"
      stroke-opacity="0.18"
      stroke-width="2"
    />

    <!-- Soft inner breathing core -->
    <circle
      cx="12"
      cy="12"
      r="2.4"
      fill="currentColor"
      class="ai-core-pulse"
    />

    <!-- Spinning orbital arc + leading ball -->
    <g class="ai-orb-spin">
      <circle
        cx="12"
        cy="12"
        r="8.5"
        stroke="currentColor"
        stroke-width="2.1"
        stroke-linecap="round"
        stroke-dasharray="22 36"
        stroke-opacity="0.72"
      />
      <circle
        cx="12"
        cy="3.5"
        r="2.55"
        fill="currentColor"
      />
    </g>
  </svg>
</span>
