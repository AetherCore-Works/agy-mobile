/// <reference types="vite/client" />
import type { Component } from 'svelte';

const cockpitLoaders = import.meta.glob('../../../packages/cockpit/ui/CockpitPanel.svelte');

export async function loadOptionalCockpitPanel(): Promise<Component<{ active?: boolean }> | null> {
  const loader = Object.values(cockpitLoaders)[0];
  if (!loader) return null;
  const mod = (await loader()) as { default?: Component<{ active?: boolean }> };
  return mod?.default ?? null;
}
