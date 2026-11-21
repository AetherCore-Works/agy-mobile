import { marked } from 'marked';
import { escapeHtml, formatLangLabel, highlightCode } from './highlight';

export const COPY_ICON_SVG =
  '<svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.85" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M16 4h2a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2h2"></path><rect x="8" y="2" width="8" height="4" rx="1" ry="1"></rect></svg>';

export const CHECK_ICON_SVG =
  '<svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.1" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><polyline points="20 6 9 17 4 12"></polyline></svg>';

const mdRenderer = new marked.Renderer();
mdRenderer.code = ({ text, lang }: { text: string; lang?: string }) => {
  const isDiff = lang === 'diff' || (/^(---|\+\+\+|@@ )/m.test(text) && /^[+-]/m.test(text));
  if (isDiff) {
    const linesHtml = text
      .split('\n')
      .map((line) => {
        const esc = escapeHtml(line) || ' ';
        if (line.startsWith('+') && !line.startsWith('+++')) {
          return `<div class="diff-add px-3 whitespace-pre">${esc}</div>`;
        }
        if (line.startsWith('-') && !line.startsWith('---')) {
          return `<div class="diff-del px-3 whitespace-pre">${esc}</div>`;
        }
        if (line.startsWith('@@')) {
          return `<div class="diff-hunk px-3 font-semibold whitespace-pre">${esc}</div>`;
        }
        return `<div class="diff-ctx px-3 whitespace-pre">${esc}</div>`;
      })
      .join('');
    return `<details open class="code-block diff-box"><summary class="code-block-header cursor-pointer"><span class="code-lang">Diff Visual</span><button type="button" class="code-copy-btn" data-copy-code title="Copiar diff">${COPY_ICON_SVG}<span>Copiar</span></button></summary><div class="diff-lines overflow-x-auto text-[11.5px] font-mono py-2 leading-5">${linesHtml}</div></details>`;
  }
  const label = escapeHtml(formatLangLabel(lang));
  const highlighted = highlightCode(text, lang);
  return `<div class="code-block"><div class="code-block-header"><span class="code-lang">${label}</span><button type="button" class="code-copy-btn" data-copy-code title="Copiar código">${COPY_ICON_SVG}<span>Copiar</span></button></div><pre><code class="language-${escapeHtml(lang || '')}">${highlighted}</code></pre></div>`;
};

const INLINE_LATEX_SYMBOLS: Record<string, string> = {
  '\\rightarrow': '→',
  '\\to': '→',
  '\\leftarrow': '←',
  '\\leftrightarrow': '↔',
  '\\Rightarrow': '⇒',
  '\\Leftarrow': '⇐',
  '\\Leftrightarrow': '⇔',
  '\\uparrow': '↑',
  '\\downarrow': '↓',
  '\\times': '×',
  '\\approx': '≈',
  '\\neq': '≠',
  '\\le': '≤',
  '\\leq': '≤',
  '\\ge': '≥',
  '\\geq': '≥',
  '\\pm': '±',
  '\\cdot': '·',
  '\\infty': '∞',
  '\\checkmark': '✓',
  '\\bullet': '•',
};

/**
 * Replaces standalone inline LaTeX symbol tokens (e.g. $\rightarrow$ -> →)
 * outside fenced code blocks and inline code spans.
 */
function normalizeInlineLatexSymbols(text: string): string {
  return text
    .split(/(```[\s\S]*?```|`[^`\n]+`)/g)
    .map((segment, idx) =>
      idx % 2 === 1
        ? segment
        : segment.replace(/\$\s*(\\[a-zA-Z]+)\s*\$/g, (full, cmd) => INLINE_LATEX_SYMBOLS[cmd] ?? full),
    )
    .join('');
}

export function renderMarkdown(text: string): string {
  const normalized = normalizeInlineLatexSymbols(text || '');
  return marked.parse(normalized, { async: false, renderer: mdRenderer }) as string;
}

export async function copyTextSafe(text: string): Promise<boolean> {
  try {
    if (navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(text);
      return true;
    }
  } catch {
    // Fallback below for non-HTTPS LAN origins
  }
  try {
    const ta = document.createElement('textarea');
    ta.value = text;
    ta.style.position = 'fixed';
    ta.style.opacity = '0';
    document.body.appendChild(ta);
    ta.focus();
    ta.select();
    const ok = document.execCommand('copy');
    document.body.removeChild(ta);
    return ok;
  } catch {
    return false;
  }
}

export function extractCodeFromCopyButton(btn: HTMLButtonElement): string {
  const block = btn.closest('.code-block');
  if (!block) return '';
  const codeEl = block.querySelector('pre code');
  if (codeEl) return codeEl.textContent ?? '';
  const diffEl = block.querySelector('.diff-lines');
  if (diffEl) {
    return Array.from(diffEl.children)
      .map((line) => line.textContent ?? '')
      .join('\n');
  }
  return '';
}
