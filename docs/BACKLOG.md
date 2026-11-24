# Backlog de Evolução — AGY Mobile Gateway

Itens planejados para os próximos ciclos de refinamento:

## 1. Atalhos de Skills & MCP no Menu `/` (Slash Commands)
- **`/publish`**: Template pronto para acionar deploy automatizado de aplicações estáticas/web.
- **`/status`**: Template rápido para consultar saúde do host, containers e recursos.
- **`/mcp`**: Templates rápidos para consultas em servidores MCP conectados ao ambiente.
- **Benefício:** Zero digitação longa no celular e roteamento determinístico para o modelo adequado.

## 2. Preview Inline de Mídia do Outbox no Chat
- Quando a IA gerar imagens (`.png`, `.jpg`, `.webp`) ou `.pdf` no Outbox, renderizar miniatura/preview interativo diretamente no card do chat antes mesmo de abrir o modal do Outbox.

---

## 3. Plano de Evolução — AGY Gateway (Lado Servidor / Backend & PWA)
Responsabilidades do núcleo Go (`agy-gateway`), banco SQLite, Scheduler e integrações de infraestrutura:

- [ ] **Ponte Telegram no `botbridge` (`POST /api/bot/telegram/webhook`):**
  - Allowlist por `chat_id` / `user_id`, envio com chunking (limite de 4096 chars) e formatação segura.
- [ ] **Push de Automações no `Notifier.Dispatch`:**
  - Adicionar flags `notify_telegram` e `notify_discord` nas automações (`automations.go` / `notifier.go`) para entregar resultados de crons e alertas de watchers diretamente nos bots.
- [ ] **Mapeamento de Sessão por Chat/Tópico no SQLite:**
  - Persistir vínculo `platform:chat_id` -> `conversation_id` (com suporte a reset `/new`), mantendo contexto contínuo entre mensagens do bot e a PWA.
- [ ] **Entrega de Arquivos do Outbox via Bot:**
  - Expor metadados/links de `outbox_files` na resposta do `botbridge` para que o bot envie imagens, PDFs e arquivos gerados diretamente no chat.
- [ ] **Cron "Morning Briefing":**
  - Task agendada no `Scheduler` consolidando status do host, alertas de containers Docker e pendências.
- [ ] **Conector IoT (Home Assistant REST Proxy / Webhook):**
  - Integrar comandos/ações rápidas via `HOME_ASSISTANT_URL` e `HOME_ASSISTANT_TOKEN` para controlar dispositivos inteligentes via REST API.

---

## 4. Ecossistema Bot — Aegys Bot (Repositório Dedicado)
A camada conversacional multi-canal (Discord, Telegram, WhatsApp via providers plugáveis, roteamento híbrido *Fast-Path/AI-Path* e RBAC em 2 níveis) evolui no repositório dedicado **[AetherCore-Works/aegys-bot](https://github.com/AetherCore-Works/aegys-bot)**, consumindo a API do `agy-gateway` ou backends LLM standalone.

---

## 5. Próximos Refinamentos de Terminal, Multi-Host & UX Mobile (Planejamento)
Itens aprovados para planejamento e execução em ciclos futuros:

- [ ] **Login SSH por Chave no `agy-ttyd`:**
  - Montar `./ssh:/root/.ssh:ro` (com chave dedicada opcional) no serviço `agy-ttyd` do `compose.yaml` e suporte a flag para abrir o shell remoto sem pedir senha no celular a cada sessão.
- [ ] **Seletor Multi-Host no Topo, Terminal e Cockpit (`CachyCore` ↔ `Dell G15` ↔ `RPi`):**
  - Permitir alternar o nó alvo do homelab diretamente pela PWA para abrir sessões SSH e inspecionar telemetria/containers em múltiplos hosts.
- [ ] **Code Splitting / Lazy-Loading do `@xterm/xterm` e Modais:**
  - Carregar `TerminalModal` (e o motor `xterm.js`), highlighters de código e modais secundários sob demanda via `import()`, reduzindo o chunk principal (~696 kB) e acelerando o carregamento em 4G/5G.
- [ ] **Timeline Visual de Tool Calls & Diffs no Chat:**
  - Adicionar um acordeão discreto e colapsável nas respostas do agente exibindo as ferramentas executadas, comandos rodados e arquivos modificados sem poluir o texto principal.
- [x] **CI & CD Completo (GitHub Actions + Self-Hosted Runner no Homelab):**
  - **CI:** `.github/workflows/ci.yml` (`go vet` + `go test -race` em `backend-go` e `packages/cockpit`, `npx tsc --noEmit && npm run build`, Gitleaks e matriz cross-OS em todo PR/push).
  - **CD:** `.github/workflows/release.yml` (releases por tag `v*` com binários multi-OS + `SHA256SUMS.txt`) e `.github/workflows/deploy-rpi.yml` (deploy no RPi via Self-Hosted Runner na LAN com `scripts/deploy-rpi.sh` + `/api/health`; ativável com `ENABLE_RPI_AUTO_DEPLOY=true`).

---

## 6. Visibilidade de Cota, Troca Manual Oficial (ToS-Safe) & Fallback de Provedor
> **Guardrail de Compliance (Google Antigravity ToS):** Proibido implementar rotação automática de contas, replay automático em `429`, proxies OAuth ou armazenamento multi-conta em banco/SQLite (`accounts.db`). Tudo aqui opera estritamente no modelo **1 conta ativa por vez**, com ação manual do usuário pelo fluxo oficial do `agy`.

- [ ] **Detecção Limpa de `429 / RESOURCE_EXHAUSTED` + Notificação na PWA/Bots:**
  - Reconhecer erro de cota esgotada no `runner.go`, interromper o job sem retry/failover automático entre contas e exibir card de ação manual na interface (e alerta opcional nos bots).
- [ ] **Painel/Atalhos de Cota (`/usage` e `/credits`):**
  - Expor atalho rápido para consultar uso de cota e AI Credits do `agy` na conta atualmente autenticada.
- [ ] **Assistente Guiado de Troca Manual via *Remote SSH OAuth* (`TerminalModal`):**
  - Quando o usuário optar por trocar manualmente entre contas pagas legítimas, abrir sessão interativa no terminal do host para o próprio usuário rodar `/logout` + `agy` (modo Remote SSH OAuth), destacando a URL oficial de login clicável no celular e facilitando colar o *authorization code* descartável direto no `stdin` do `agy` (sem capturar/armazenar tokens no gateway).
- [ ] **Fallback Explícito de Backend (Ollama Local ou `GEMINI_API_KEY`):**
  - Permitir ao usuário alternar manualmente a conversa/próximo turno para um provedor alternativo legítimo (**Ollama local** ou **`modelProvider: "gemini"` + `GEMINI_API_KEY`** própria) ao atingir o teto da janela de 5h.
