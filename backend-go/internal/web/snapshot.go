package web

import (
	"encoding/json"
	"fmt"
	"html"
	"regexp"
	"strings"
)

var (
	inlineCodeRe   = regexp.MustCompile("`([^`\n]+)`")
	inlineBoldRe   = regexp.MustCompile(`\*\*([^*]+)\*\*`)
	inlineItalicRe = regexp.MustCompile(`\*([^*\n]+)\*`)
	inlineLinkRe   = regexp.MustCompile(`\[([^\]]+)\]\((https?://[^)\s]+)\)`)
)

func renderInlineMD(line string) string {
	var codeSlots []string
	masked := inlineCodeRe.ReplaceAllStringFunc(line, func(m string) string {
		inner := strings.TrimSuffix(strings.TrimPrefix(m, "`"), "`")
		idx := len(codeSlots)
		codeSlots = append(codeSlots, "<code>"+html.EscapeString(inner)+"</code>")
		return fmt.Sprintf("\x00CODE%d\x00", idx)
	})

	var linkSlots []string
	masked = inlineLinkRe.ReplaceAllStringFunc(masked, func(m string) string {
		parts := inlineLinkRe.FindStringSubmatch(m)
		if len(parts) != 3 {
			return html.EscapeString(m)
		}
		idx := len(linkSlots)
		linkSlots = append(linkSlots, fmt.Sprintf(`<a href="%s" target="_blank" rel="noopener noreferrer">%s</a>`, html.EscapeString(parts[2]), html.EscapeString(parts[1])))
		return fmt.Sprintf("\x00LINK%d\x00", idx)
	})

	escaped := html.EscapeString(masked)
	escaped = inlineBoldRe.ReplaceAllString(escaped, "<strong>$1</strong>")
	escaped = inlineItalicRe.ReplaceAllString(escaped, "<em>$1</em>")

	for i, codeHTML := range codeSlots {
		escaped = strings.ReplaceAll(escaped, fmt.Sprintf("\x00CODE%d\x00", i), codeHTML)
	}
	for i, linkHTML := range linkSlots {
		escaped = strings.ReplaceAll(escaped, fmt.Sprintf("\x00LINK%d\x00", i), linkHTML)
	}
	return escaped
}

func renderMarkdownSafe(md string) string {
	lines := strings.Split(strings.ReplaceAll(md, "\r\n", "\n"), "\n")
	var b strings.Builder

	inCode := false
	codeLang := ""
	var codeBuf []string

	listTag := ""
	closeList := func() {
		if listTag != "" {
			b.WriteString("</" + listTag + ">\n")
			listTag = ""
		}
	}

	flushCode := func() {
		langLabel := strings.ToUpper(strings.TrimSpace(codeLang))
		if langLabel == "" {
			langLabel = "CÓDIGO"
		}
		b.WriteString(`<div class="code-block"><div class="code-hdr"><span>` + html.EscapeString(langLabel) + `</span><button type="button" class="copy-btn" onclick="copyCode(this)">Copiar</button></div><pre><code>`)
		b.WriteString(html.EscapeString(strings.Join(codeBuf, "\n")))
		b.WriteString("</code></pre></div>\n")
		codeBuf = nil
		codeLang = ""
	}

	for _, raw := range lines {
		trimmed := strings.TrimSpace(raw)
		if strings.HasPrefix(trimmed, "```") {
			if inCode {
				flushCode()
				inCode = false
			} else {
				closeList()
				inCode = true
				codeLang = strings.TrimSpace(strings.TrimPrefix(trimmed, "```"))
			}
			continue
		}
		if inCode {
			codeBuf = append(codeBuf, raw)
			continue
		}

		if trimmed == "" {
			closeList()
			continue
		}
		if trimmed == "---" || trimmed == "***" || trimmed == "___" {
			closeList()
			b.WriteString("<hr />\n")
			continue
		}
		if strings.HasPrefix(trimmed, "#### ") {
			closeList()
			b.WriteString("<h4>" + renderInlineMD(strings.TrimPrefix(trimmed, "#### ")) + "</h4>\n")
			continue
		}
		if strings.HasPrefix(trimmed, "### ") {
			closeList()
			b.WriteString("<h3>" + renderInlineMD(strings.TrimPrefix(trimmed, "### ")) + "</h3>\n")
			continue
		}
		if strings.HasPrefix(trimmed, "## ") {
			closeList()
			b.WriteString("<h2>" + renderInlineMD(strings.TrimPrefix(trimmed, "## ")) + "</h2>\n")
			continue
		}
		if strings.HasPrefix(trimmed, "# ") {
			closeList()
			b.WriteString("<h1>" + renderInlineMD(strings.TrimPrefix(trimmed, "# ")) + "</h1>\n")
			continue
		}
		if strings.HasPrefix(trimmed, "> ") {
			closeList()
			b.WriteString("<blockquote>" + renderInlineMD(strings.TrimPrefix(trimmed, "> ")) + "</blockquote>\n")
			continue
		}
		if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") {
			if listTag != "ul" {
				closeList()
				listTag = "ul"
				b.WriteString("<ul>\n")
			}
			b.WriteString("<li>" + renderInlineMD(trimmed[2:]) + "</li>\n")
			continue
		}
		if dot := strings.Index(trimmed, ". "); dot > 0 && dot <= 3 {
			allDigits := true
			for _, ch := range trimmed[:dot] {
				if ch < '0' || ch > '9' {
					allDigits = false
					break
				}
			}
			if allDigits {
				if listTag != "ol" {
					closeList()
					listTag = "ol"
					b.WriteString("<ol>\n")
				}
				b.WriteString("<li>" + renderInlineMD(trimmed[dot+2:]) + "</li>\n")
				continue
			}
		}

		closeList()
		b.WriteString("<p>" + renderInlineMD(trimmed) + "</p>\n")
	}
	if inCode {
		flushCode()
	}
	closeList()
	return b.String()
}

func paletteTokens(mode, palette string) (accentSolid, accentFG string) {
	dark := mode != "light"
	switch palette {
	case "emerald":
		if dark {
			return "#059669", "#34d399"
		}
		return "#059669", "#059669"
	case "violet":
		if dark {
			return "#7c3aed", "#a78bfa"
		}
		return "#7c3aed", "#7c3aed"
	case "indigo":
		if dark {
			return "#4f46e5", "#818cf8"
		}
		return "#4f46e5", "#4f46e5"
	case "amber":
		if dark {
			return "#d97706", "#fbbf24"
		}
		return "#d97706", "#b45309"
	case "rose":
		if dark {
			return "#e11d48", "#fb7185"
		}
		return "#e11d48", "#e11d48"
	default:
		if dark {
			return "#0284c7", "#38bdf8"
		}
		return "#0284c7", "#0284c7"
	}
}

// RenderSnapshotHTML produces a self-contained read-only HTML document for a shared snapshot.
func RenderSnapshotHTML(title, contentMD, createdAt, themeMode, themePalette string) string {
	if themeMode != "light" {
		themeMode = "dark"
	}
	accentSolid, accentFG := paletteTokens(themeMode, themePalette)
	bgApp, bgCard, bgCode, bgHdr, border, textMain, textMuted :=
		"#09090b", "#18181b", "#0e0e12", "#16161c", "#27272a", "#f4f4f5", "#a1a1aa"
	if themeMode == "light" {
		bgApp, bgCard, bgCode, bgHdr, border, textMain, textMuted =
			"#f4f5f7", "#ffffff", "#f8fafc", "#eef2f6", "#e4e4e7", "#18181b", "#52525b"
	}

	rawJSON, _ := json.Marshal(contentMD)
	safeJSON := strings.ReplaceAll(string(rawJSON), "<", `\u003c`)
	safeTitle := html.EscapeString(title)
	if strings.TrimSpace(safeTitle) == "" {
		safeTitle = "Snapshot AGY"
	}
	renderedBody := renderMarkdownSafe(contentMD)

	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="pt-BR">
<head>
  <meta charset="UTF-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1.0" />
  <title>%s — AGY Snapshot</title>
  <style>
    :root {
      --bg-app: %s; --bg-card: %s; --bg-code: %s; --bg-hdr: %s;
      --border: %s; --text: %s; --muted: %s;
      --accent: %s; --accent-fg: %s;
    }
    * { box-sizing: border-box; }
    body {
      margin: 0; font-family: system-ui, -apple-system, Segoe UI, Roboto, sans-serif;
      background: var(--bg-app); color: var(--text); line-height: 1.7; padding: 24px 16px 48px;
    }
    .wrap { max-width: 820px; margin: 0 auto; }
    .topbar {
      display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: 12px;
      margin-bottom: 18px; padding-bottom: 14px; border-bottom: 1px solid var(--border);
    }
    .badge {
      display: inline-block; font-size: 11px; font-weight: 600; padding: 2px 8px; border-radius: 999px;
      background: color-mix(in srgb, var(--accent) 16%%, transparent); color: var(--accent-fg);
      border: 1px solid color-mix(in srgb, var(--accent-fg) 35%%, transparent);
    }
    h1.doc-title { font-size: 1.15rem; margin: 4px 0 0; }
    .meta { font-size: 12px; color: var(--muted); }
    .card {
      background: var(--bg-card); border: 1px solid var(--border); border-radius: 16px;
      padding: 22px 24px; overflow-wrap: anywhere;
    }
    .copy-btn {
      background: var(--bg-card); color: var(--text); border: 1px solid var(--border);
      border-radius: 8px; padding: 5px 10px; font-size: 12px; cursor: pointer; font-weight: 500;
    }
    .copy-btn:hover { border-color: var(--accent-fg); color: var(--accent-fg); }
    .card h1, .card h2, .card h3, .card h4 { margin: 1.3em 0 0.5em; line-height: 1.35; }
    .card > :first-child { margin-top: 0; }
    .card > :last-child { margin-bottom: 0; }
    .card p { margin: 0 0 0.85em; }
    .card hr { border: none; height: 1px; background: var(--border); margin: 1.3em 0; }
    .card ul, .card ol { padding-left: 1.4em; margin: 0.6em 0 0.9em; }
    .card li { margin-bottom: 0.4em; }
    .card blockquote {
      margin: 0.9em 0; padding: 8px 14px; border-left: 3px solid var(--accent-fg);
      background: color-mix(in srgb, var(--accent) 10%%, transparent); border-radius: 0 8px 8px 0;
    }
    .card code {
      font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace; font-size: 0.85em;
      background: var(--bg-hdr); border: 1px solid var(--border); padding: 1px 5px; border-radius: 5px;
    }
    .code-block {
      margin: 0.95em 0; border: 1px solid var(--border); border-radius: 12px;
      background: var(--bg-code); overflow: hidden;
    }
    .code-hdr {
      display: flex; align-items: center; justify-content: space-between;
      padding: 6px 12px; background: var(--bg-hdr); border-bottom: 1px solid var(--border);
      font-family: ui-monospace, monospace; font-size: 11px; color: var(--muted); font-weight: 600;
    }
    .code-block pre { margin: 0; padding: 14px 16px; overflow-x: auto; font-size: 13px; line-height: 1.6; }
    .code-block pre code { background: transparent; border: none; padding: 0; }
    a { color: var(--accent-fg); }
  </style>
</head>
<body>
  <div class="wrap">
    <div class="topbar">
      <div>
        <span class="badge">Snapshot Somente Leitura</span>
        <h1 class="doc-title">%s</h1>
        <div class="meta">%s</div>
      </div>
      <button type="button" class="copy-btn" onclick="copyMarkdown(this)">Copiar Markdown</button>
    </div>
    <article class="card">%s</article>
  </div>
  <script type="application/json" id="raw-md">%s</script>
  <script>
    function copyText(txt, btn, orig) {
      if (navigator.clipboard && navigator.clipboard.writeText) {
        navigator.clipboard.writeText(txt).then(function() { done(); }).catch(fallback);
      } else { fallback(); }
      function fallback() {
        var ta = document.createElement('textarea'); ta.value = txt; ta.style.position = 'fixed'; ta.style.opacity = '0';
        document.body.appendChild(ta); ta.select(); document.execCommand('copy'); document.body.removeChild(ta); done();
      }
      function done() { btn.textContent = 'Copiado'; setTimeout(function() { btn.textContent = orig; }, 1500); }
    }
    function copyCode(btn) {
      var code = btn.closest('.code-block').querySelector('pre code').textContent || '';
      copyText(code, btn, 'Copiar');
    }
    function copyMarkdown(btn) {
      var md = JSON.parse(document.getElementById('raw-md').textContent);
      copyText(md, btn, 'Copiar Markdown');
    }
  </script>
</body>
</html>`, safeTitle, bgApp, bgCard, bgCode, bgHdr, border, textMain, textMuted, accentSolid, accentFG, safeTitle, html.EscapeString(createdAt), renderedBody, safeJSON)
}
