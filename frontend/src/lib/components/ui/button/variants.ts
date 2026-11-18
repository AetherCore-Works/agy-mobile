import { type VariantProps, tv } from 'tailwind-variants';
import type { HTMLAnchorAttributes, HTMLButtonAttributes } from 'svelte/elements';
import type { WithElementRef } from '../../../utils';

export const buttonVariants = tv({
  base: "group/button inline-flex shrink-0 cursor-pointer items-center justify-center gap-1.5 rounded-full border border-transparent bg-clip-padding text-xs font-medium whitespace-nowrap transition-[color,background-color,border-color,box-shadow,transform,filter] duration-200 outline-none select-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50 active:not-aria-[haspopup]:scale-[0.97] disabled:pointer-events-none disabled:opacity-45 [&_svg]:pointer-events-none [&_svg]:shrink-0",
  variants: {
    variant: {
      default:
        'border-primary/80 bg-radial-[at_52%_-52%] from-primary/85 to-primary text-primary-foreground shadow-sm inset-shadow-2xs inset-shadow-white/25 hover:brightness-110 dark:border-primary/40 dark:from-primary dark:to-primary/80 dark:inset-shadow-white/20',
      outline:
        'border-border bg-card/90 text-foreground shadow-xs hover:bg-muted/80 hover:border-input hover:text-accent-foreground',
      secondary:
        'bg-secondary text-secondary-foreground hover:bg-secondary/80',
      ghost:
        'text-muted-foreground hover:bg-secondary/80 hover:text-foreground',
      destructive:
        'bg-destructive/12 text-rose-400 border-destructive/35 hover:bg-destructive/22 focus-visible:ring-destructive/30',
      destructiveSolid:
        'bg-destructive text-destructive-foreground shadow-sm hover:bg-destructive/90',
      link: 'text-accent-foreground underline-offset-4 hover:underline',
    },
    size: {
      default: 'h-9 px-4 text-xs',
      xs: 'h-6 px-2 text-[11px]',
      sm: 'h-7.5 px-3 text-[11px]',
      lg: 'h-10 px-5 text-sm',
      icon: 'size-9',
      'icon-xs': 'size-6',
      'icon-sm': 'size-8',
    },
  },
  defaultVariants: {
    variant: 'default',
    size: 'default',
  },
});

export type ButtonVariant = VariantProps<typeof buttonVariants>['variant'];
export type ButtonSize = VariantProps<typeof buttonVariants>['size'];

export type ButtonProps = WithElementRef<HTMLButtonAttributes> &
  WithElementRef<HTMLAnchorAttributes> & {
    variant?: ButtonVariant;
    size?: ButtonSize;
  };
