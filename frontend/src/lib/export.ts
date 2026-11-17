import type { ChatMessage } from './api';
import { escapeHtml } from './highlight';
import type { ThemeMode, ThemePalette } from './theme';

export function buildConversationMarkdown(title: string, messages: ChatMessage[]): string {
  const header = `# ${title || 'Conversa AGY'}\n\n`;
  const body = messages
    .map((m) => `### ${m.role === 'user' ? 'Você' : 'AGY'}\n\n${m.content.trim()}`)
    .join('\n\n---\n\n');
  return header + body + '\n';
}

export function downloadTextFile(filename: string, content: string, mimeType: string): void {
  const blob = new Blob([content], { type: mimeType });
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = filename;
  document.body.appendChild(a);
  a.click();
  document.body.removeChild(a);
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}

const PALETTE_COLORS: Record<ThemePalette, { darkSolid: string; darkFg: string; lightSolid: string; lightFg: string }> = {
  sky: { darkSolid: '#0284c7', darkFg: '#38bdf8', lightSolid: '#0284c7', lightFg: '#0284c7' },
  emerald: { darkSolid: '#059669', darkFg: '#34d399', lightSolid: '#059669', lightFg: '#059669' },
  violet: { darkSolid: '#7c3aed', darkFg: '#a78bfa', lightSolid: '#7c3aed', lightFg: '#7c3aed' },
  indigo: { darkSolid: '#4f46e5', darkFg: '#818cf8', lightSolid: '#4f46e5', lightFg: '#4f46e5' },
  amber: { darkSolid: '#d97706', darkFg: '#fbbf24', lightSolid: '#d97706', lightFg: '#b45309' },
  rose: { darkSolid: '#e11d48', darkFg: '#fb7185', lightSolid: '#e11d48', lightFg: '#e11d48' },
};

export function buildStandaloneSnapshotHTML(
  title: string,
  renderedBodyHTML: string,
  rawMarkdown: string,
  themeMode: ThemeMode,
  themePalette: ThemePalette
): string {
  const pal = PALETTE_COLORS[themePalette] ?? PALETTE_COLORS.sky;
  const isDark = themeMode !== 'light';
  const accent = isDark ? pal.darkSolid : pal.lightSolid;
  const accentFg = isDark ? pal.darkFg : pal.lightFg;

  const vars = isDark
    ? `--bg-app:#09090b;--bg-card:#18181b;--code-bg:#0e0e12;--code-hdr:#16161c;--inline-bg:#27272a;--border:#27272a;--text:#f4f4f5;--muted:#a1a1aa;--tok-kw:#c084fc;--tok-str:#86efac;--tok-num:#fbbf24;--tok-cmt:#71717a;--tok-fn:#60a5fa;--tok-type:#2dd4bf;--tok-attr:#7dd3fc;--tok-var:#f472b6;`
    : `--bg-app:#f4f5f7;--bg-card:#ffffff;--code-bg:#f8fafc;--code-hdr:#eef2f6;--inline-bg:#e8edf3;--border:#e4e4e7;--text:#18181b;--muted:#52525b;--tok-kw:#7c3aed;--tok-str:#059669;--tok-num:#b45309;--tok-cmt:#64748b;--tok-fn:#2563eb;--tok-type:#0f766e;--tok-attr:#0284c7;--tok-var:#be185d;`;

  const safeJSON = JSON.stringify(rawMarkdown).replace(/</g, '\\u003c');
  const safeTitle = escapeHtml(title || 'Snapshot AGY');
  const createdStr = escapeHtml(new Date().toLocaleString('pt-BR'));

  return `<!DOCTYPE html>
<html lang="pt-BR">
<head>
  <meta charset="UTF-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1.0" />
  <title>${safeTitle} — AGY Snapshot</title>
  <style>
    :root{${vars}--accent:${accent};--accent-fg:${accentFg};}
    *{box-sizing:border-box}
    body{margin:0;font-family:system-ui,-apple-system,Segoe UI,Roboto,sans-serif;background:var(--bg-app);color:var(--text);line-height:1.7;padding:24px 16px 48px}
    .wrap{max-width:820px;margin:0 auto}
    .topbar{display:flex;flex-wrap:wrap;align-items:center;justify-content:space-between;gap:12px;margin-bottom:18px;padding-bottom:14px;border-bottom:1px solid var(--border)}
    .badge{display:inline-block;font-size:11px;font-weight:600;padding:2px 9px;border-radius:999px;background:color-mix(in srgb,var(--accent) 16%,transparent);color:var(--accent-fg);border:1px solid color-mix(in srgb,var(--accent-fg) 35%,transparent)}
    h1.doc-title{font-size:1.15rem;margin:6px 0 2px}
    .meta{font-size:12px;color:var(--muted)}
    .card{background:var(--bg-card);border:1px solid var(--border);border-radius:16px;padding:22px 24px;overflow-wrap:anywhere}
    .btn{background:var(--bg-card);color:var(--text);border:1px solid var(--border);border-radius:8px;padding:6px 12px;font-size:12px;cursor:pointer;font-weight:500}
    .btn:hover{border-color:var(--accent-fg);color:var(--accent-fg)}
    .card h1,.card h2,.card h3,.card h4{margin:1.3em 0 0.5em;line-height:1.35}
    .card>:first-child{margin-top:0}.card>:last-child{margin-bottom:0}
    .card p{margin:0 0 0.85em}
    .card hr{border:none;height:1px;background:var(--border);margin:1.35em 0}
    .card ul,.card ol{padding-left:1.4em;margin:0.6em 0 0.9em}.card li{margin-bottom:0.4em}
    .card blockquote{margin:0.9em 0;padding:8px 14px;border-left:3px solid var(--accent-fg);background:color-mix(in srgb,var(--accent) 10%,transparent);border-radius:0 8px 8px 0}
    .card code{font-family:ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;font-size:0.84em;background:var(--inline-bg);border:1px solid var(--border);padding:1px 6px;border-radius:6px}
    .code-block{margin:0.95em 0;border:1px solid var(--border);border-radius:12px;background:var(--code-bg);overflow:hidden}
    .code-block-header{display:flex;align-items:center;justify-content:space-between;padding:6px 12px;background:var(--code-hdr);border-bottom:1px solid var(--border);font-family:ui-monospace,monospace;font-size:11px;color:var(--muted);font-weight:600}
    .code-copy-btn{display:inline-flex;align-items:center;gap:4px;background:var(--bg-card);color:var(--text);border:1px solid var(--border);border-radius:6px;padding:3px 8px;font-size:11px;cursor:pointer}
    .code-block pre{margin:0;padding:14px 16px;overflow-x:auto;font-size:13px;line-height:1.65;white-space:pre}
    .code-block pre code{background:transparent;border:none;padding:0;font-size:inherit}
    .tok-kw{color:var(--tok-kw);font-weight:500}.tok-str{color:var(--tok-str)}.tok-num{color:var(--tok-num)}.tok-cmt{color:var(--tok-cmt);font-style:italic}.tok-fn{color:var(--tok-fn)}.tok-type{color:var(--tok-type)}.tok-attr{color:var(--tok-attr)}.tok-var{color:var(--tok-var)}
    table{width:100%;border-collapse:collapse;margin:1em 0;font-size:0.84rem;display:block;overflow-x:auto}
    th,td{border:1px solid var(--border);padding:6px 10px;text-align:left;white-space:nowrap}
    th{background:var(--code-hdr)}
    a{color:var(--accent-fg)}
  </style>
</head>
<body>
  <div class="wrap">
    <div class="topbar">
      <div>
        <span class="badge">AGY Snapshot Offline</span>
        <h1 class="doc-title">${safeTitle}</h1>
        <div class="meta">Gerado em ${createdStr}</div>
      </div>
      <button type="button" class="btn" onclick="copyMd(this)">Copiar Markdown</button>
    </div>
    <article class="card">${renderedBodyHTML}</article>
  </div>
  <script type="application/json" id="raw-md">${safeJSON}</script>
  <script>
    function copyStr(txt,btn,orig){
      if(navigator.clipboard&&navigator.clipboard.writeText){navigator.clipboard.writeText(txt).then(done).catch(fb)}else{fb()}
      function fb(){var t=document.createElement('textarea');t.value=txt;t.style.position='fixed';t.style.opacity='0';document.body.appendChild(t);t.select();document.execCommand('copy');document.body.removeChild(t);done()}
      function done(){btn.textContent='Copiado';setTimeout(function(){btn.textContent=orig},1500)}
    }
    document.addEventListener('click',function(e){
      var btn=e.target.closest&&e.target.closest('button[data-copy-code]');
      if(!btn)return;
      var block=btn.closest('.code-block');
      var code=block&&block.querySelector('pre code');
      if(code)copyStr(code.textContent||'',btn,'Copiar');
    });
    function copyMd(btn){copyStr(JSON.parse(document.getElementById('raw-md').textContent),btn,'Copiar Markdown')}
  </script>
</body>
</html>`;
}
