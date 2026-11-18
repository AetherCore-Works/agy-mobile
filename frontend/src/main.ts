import './app.css';
import { mount } from 'svelte';
import App from './App.svelte';

// Global Gemini-style tactile ripple on touch/click
document.addEventListener(
  'pointerdown',
  (ev) => {
    const target = (ev.target as HTMLElement | null)?.closest('button, a[download], .tap-surface') as HTMLElement | null;
    if (!target || target.hasAttribute('disabled') || target.dataset.noRipple !== undefined) return;
    const rect = target.getBoundingClientRect();
    if (rect.width === 0 || rect.height === 0) return;

    const size = Math.max(rect.width, rect.height) * 2.1;
    const ripple = document.createElement('span');
    ripple.className = 'agy-ripple';
    ripple.style.width = `${size}px`;
    ripple.style.height = `${size}px`;
    ripple.style.left = `${ev.clientX - rect.left - size / 2}px`;
    ripple.style.top = `${ev.clientY - rect.top - size / 2}px`;

    target.appendChild(ripple);
    ripple.addEventListener('animationend', () => ripple.remove(), { once: true });
    setTimeout(() => ripple.remove(), 600);
  },
  { passive: true },
);

mount(App, {
  target: document.getElementById('app')!,
});
