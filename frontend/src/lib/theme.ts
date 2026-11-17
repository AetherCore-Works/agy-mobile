export type ThemeMode = 'dark' | 'light';
export type ThemePalette = 'sky' | 'emerald' | 'violet' | 'indigo' | 'amber' | 'rose';

export interface PaletteOption {
  id: ThemePalette;
  label: string;
  swatch: string;
}

export const THEME_PALETTES: PaletteOption[] = [
  { id: 'sky', label: 'Ciano', swatch: '#0ea5e9' },
  { id: 'emerald', label: 'Esmeralda', swatch: '#10b981' },
  { id: 'violet', label: 'Violeta', swatch: '#8b5cf6' },
  { id: 'indigo', label: 'Índigo', swatch: '#6366f1' },
  { id: 'amber', label: 'Âmbar', swatch: '#f59e0b' },
  { id: 'rose', label: 'Rosa', swatch: '#f43f5e' },
];

const MODE_KEY = 'agy_theme_mode';
const PALETTE_KEY = 'agy_theme_palette';

export function normalizeThemeMode(val?: string | null): ThemeMode {
  return val === 'light' ? 'light' : 'dark';
}

export function normalizeThemePalette(val?: string | null): ThemePalette {
  return THEME_PALETTES.some((p) => p.id === val) ? (val as ThemePalette) : 'sky';
}

export function hasStoredTheme(): boolean {
  try {
    return Boolean(localStorage.getItem(MODE_KEY) || localStorage.getItem(PALETTE_KEY));
  } catch {
    return false;
  }
}

export function readStoredTheme(): { mode: ThemeMode; palette: ThemePalette } {
  try {
    return {
      mode: normalizeThemeMode(localStorage.getItem(MODE_KEY)),
      palette: normalizeThemePalette(localStorage.getItem(PALETTE_KEY)),
    };
  } catch {
    return { mode: 'dark', palette: 'sky' };
  }
}

export function applyTheme(mode: ThemeMode, palette: ThemePalette, persist = true): void {
  const root = document.documentElement;
  root.dataset.theme = mode;
  root.dataset.palette = palette;
  root.classList.toggle('dark', mode === 'dark');

  const meta = document.querySelector('meta[name="theme-color"]');
  if (meta) {
    meta.setAttribute('content', mode === 'dark' ? '#09090b' : '#f4f5f7');
  }

  if (persist) {
    try {
      localStorage.setItem(MODE_KEY, mode);
      localStorage.setItem(PALETTE_KEY, palette);
    } catch {
      // ignore storage failures in private mode
    }
  }
}

export function getActiveAccentColor(): string {
  const value = getComputedStyle(document.documentElement).getPropertyValue('--accent-fg').trim();
  return value || '#38bdf8';
}
