const KEYWORDS = new Set([
  // JS / TS / Svelte
  'const', 'let', 'var', 'function', 'async', 'await', 'return', 'if', 'else',
  'for', 'while', 'do', 'switch', 'case', 'default', 'break', 'continue',
  'import', 'export', 'from', 'as', 'type', 'interface', 'enum', 'class',
  'extends', 'implements', 'new', 'try', 'catch', 'finally', 'throw', 'typeof',
  'instanceof', 'in', 'of', 'true', 'false', 'null', 'undefined',
  // Go / Rust / Python
  'package', 'func', 'struct', 'map', 'chan', 'go', 'defer', 'select', 'range',
  'nil', 'fn', 'pub', 'use', 'mod', 'impl', 'trait', 'match', 'mut', 'self',
  'def', 'elif', 'pass', 'with', 'yield', 'lambda', 'not', 'and', 'or', 'is',
  'True', 'False', 'None',
  // Shell / SQL
  'sudo', 'cd', 'echo', 'export', 'source', 'exit', 'git', 'docker', 'npm',
  'SELECT', 'FROM', 'WHERE', 'INSERT', 'INTO', 'VALUES', 'UPDATE', 'SET',
  'DELETE', 'CREATE', 'TABLE', 'IF', 'NOT', 'EXISTS', 'JOIN', 'ON', 'ORDER',
  'BY', 'GROUP', 'LIMIT', 'AND', 'OR', 'AS', 'DESC', 'ASC',
]);

const TYPES = new Set([
  'string', 'number', 'boolean', 'any', 'void', 'never', 'unknown', 'object',
  'int', 'int32', 'int64', 'uint', 'uint64', 'float32', 'float64', 'byte',
  'rune', 'bool', 'error', 'str', 'dict', 'list', 'tuple', 'set', 'Promise',
  'Record', 'Array', 'Map', 'Set', 'Error', 'HTMLElement', 'Event',
]);

const LANG_LABELS: Record<string, string> = {
  ts: 'TypeScript',
  typescript: 'TypeScript',
  js: 'JavaScript',
  javascript: 'JavaScript',
  svelte: 'Svelte',
  go: 'Go',
  golang: 'Go',
  py: 'Python',
  python: 'Python',
  bash: 'Bash',
  sh: 'Shell',
  zsh: 'Zsh',
  pwsh: 'PowerShell',
  powershell: 'PowerShell',
  ps1: 'PowerShell',
  json: 'JSON',
  yaml: 'YAML',
  yml: 'YAML',
  toml: 'TOML',
  css: 'CSS',
  html: 'HTML',
  xml: 'XML',
  sql: 'SQL',
  dockerfile: 'Dockerfile',
  diff: 'Diff',
  md: 'Markdown',
  markdown: 'Markdown',
  rust: 'Rust',
  rs: 'Rust',
};

export function formatLangLabel(lang?: string): string {
  const clean = (lang || '').trim().toLowerCase();
  if (!clean) return 'Código';
  return LANG_LABELS[clean] ?? clean.toUpperCase();
}

export function escapeHtml(str: string): string {
  return str
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;');
}

const TOKEN_RE =
  /<!--[\s\S]*?-->|\/\*[\s\S]*?\*\/|\/\/[^\n]*|#(?![0-9a-fA-F]{3,8}\b)[^\n]*|"(?:\\[\s\S]|[^"\\\n])*"|'(?:\\[\s\S]|[^'\\\n])*'|`(?:\\[\s\S]|[^`\\])*`|\$\{[^}\n]+\}|\$[A-Za-z_]\w*|\b(?:0x[0-9a-fA-F_]+|0b[01_]+|\d+(?:\.\d+)?)\b|[A-Za-z_$][\w$]*/g;

export function highlightCode(rawCode: string, lang?: string): string {
  const code = rawCode.replace(/\n$/, '');
  const cleanLang = (lang || '').trim().toLowerCase();
  if (!code) return '';
  if (cleanLang === 'text' || cleanLang === 'txt' || cleanLang === 'plain') {
    return escapeHtml(code);
  }

  let out = '';
  let lastIndex = 0;
  TOKEN_RE.lastIndex = 0;

  let m: RegExpExecArray | null;
  while ((m = TOKEN_RE.exec(code)) !== null) {
    if (m.index > lastIndex) {
      out += escapeHtml(code.slice(lastIndex, m.index));
    }

    const tok = m[0];
    const esc = escapeHtml(tok);
    const nextChar = code[TOKEN_RE.lastIndex] ?? '';

    if (tok.startsWith('<!--') || tok.startsWith('/*') || tok.startsWith('//') || tok.startsWith('#')) {
      out += `<span class="tok-cmt">${esc}</span>`;
    } else if (tok.startsWith('"') || tok.startsWith("'") || tok.startsWith('`')) {
      // Distinguish JSON/object property keys like `"model":`
      const afterSpaces = code.slice(TOKEN_RE.lastIndex).match(/^[ \t]*:/);
      if (tok.startsWith('"') && afterSpaces && (cleanLang === 'json' || cleanLang === 'jsonc')) {
        out += `<span class="tok-attr">${esc}</span>`;
      } else {
        out += `<span class="tok-str">${esc}</span>`;
      }
    } else if (tok.startsWith('$')) {
      out += `<span class="tok-var">${esc}</span>`;
    } else if (/^\d/.test(tok)) {
      out += `<span class="tok-num">${esc}</span>`;
    } else if (KEYWORDS.has(tok)) {
      out += `<span class="tok-kw">${esc}</span>`;
    } else if (TYPES.has(tok) || /^[A-Z][A-Za-z0-9_]*$/.test(tok)) {
      out += `<span class="tok-type">${esc}</span>`;
    } else if (nextChar === '(') {
      out += `<span class="tok-fn">${esc}</span>`;
    } else if (nextChar === ':' && (cleanLang === 'yaml' || cleanLang === 'yml' || cleanLang === 'css')) {
      out += `<span class="tok-attr">${esc}</span>`;
    } else {
      out += esc;
    }

    lastIndex = TOKEN_RE.lastIndex;
  }

  if (lastIndex < code.length) {
    out += escapeHtml(code.slice(lastIndex));
  }
  return out;
}
