import { type VariantProps, tv } from 'tailwind-variants';

export const badgeVariants = tv({
  base: 'inline-flex items-center justify-center gap-1 rounded-full font-medium whitespace-nowrap leading-none transition-colors select-none',
  variants: {
    variant: {
      default: 'bg-primary text-primary-foreground',
      secondary: 'bg-secondary text-secondary-foreground',
      accent: 'bg-accent text-accent-foreground border border-[var(--accent-border)]',
      outline: 'border border-border bg-card/60 text-muted-foreground',
      destructive: 'bg-rose-500/15 text-rose-400 border border-rose-500/35',
      sky: 'bg-sky-500/15 text-sky-400 border border-sky-500/30',
      emerald: 'bg-emerald-500/15 text-emerald-500 border border-emerald-500/30',
      violet: 'bg-violet-500/15 text-violet-400 border border-violet-500/30',
      indigo: 'bg-indigo-500/15 text-indigo-400 border border-indigo-500/30',
      amber: 'bg-amber-500/15 text-amber-500 border border-amber-500/35',
      rose: 'bg-rose-500/15 text-rose-400 border border-rose-500/30',
      slate: 'bg-zinc-500/15 text-muted-foreground border border-border',
    },
    size: {
      default: 'h-5 px-2 text-[11px]',
      xs: 'h-4 px-1.5 text-[10px]',
      sm: 'h-6 px-2.5 text-xs',
    },
  },
  defaultVariants: {
    variant: 'default',
    size: 'default',
  },
});

export type BadgeVariant = VariantProps<typeof badgeVariants>['variant'];
export type BadgeSize = VariantProps<typeof badgeVariants>['size'];
