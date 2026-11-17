# Playbook & Grande Lista de Afazeres: Onboarding da IA (AGY) no Windows

Este documento é o **guia mestre passo a passo** para você entregar à nova instância do **Google Antigravity (`agy`) no Windows** (dual boot no `CachyCore` ou outra máquina Windows na LAN) para que ela se conecte imediatamente a **todos os serviços pré-existentes** da sua infraestrutura (Raspberry Pi 5 `192.168.1.210`, `ai-memory` / `ai-hub`, `Disbox MCP`, `Fin-Vault MCP`, `Ollama CUDA` local, `ollama-worker`, `publish-rpi-site` e `AGY Mobile Gateway`).

> **Dica de Ouro:** Quando abrir o AGY no Windows pela primeira vez, basta colar o prompt da **Seção 7** abaixo — ele próprio lerá este checklist e configurará quase tudo sozinho!

---

## 1. Arquitetura: O que fica no AGY vs. O que fica no AGY Mobile Gateway?

Antes de configurar o Windows, vale fixar a divisão de responsabilidades:

| Camada | Onde Mora? | O que faz? | Exemplos |
| :--- | :--- | :--- | :--- |
| **AGY Mobile Gateway** (`agy-mobile-gateway`) | No Raspberry Pi (`192.168.1.210:8088`) ou binário standalone (`agy-gateway-windows-amd64.exe`) | É o **controle remoto Web/PWA** (interface celular, fila SSE, Inbox/Outbox, botão Wake-on-LAN, Terminal WebSocket). | Botão `Pedir Download`, `/outbox`, auto-scroll do iPhone, upload de fotos. |
| **Motor AGY ("AGY de verdade")** | No Host onde o CLI `agy` roda: **CachyCore Linux** (`/home/ai/.gemini/config/`) ou **Windows** (`%USERPROFILE%\.gemini\config\`) | É o **cérebro executor**: carrega os servidores **MCP**, lê as **Skills** (`SKILL.md`), aplica as regras (`AGENTS.md`) e executa ferramentas. | `ai-memory`, `disbox` MCP, `fin-vault` MCP, `@playwright/mcp`, skill `publish-rpi-site`. |

### Caso Prático: Como funciona a Skill `publish-rpi-site`?
- A skill `publish-rpi-site` fica instalada **do lado do AGY de verdade** (`~/.gemini/config/skills/publish-rpi-site/SKILL.md`).
- Quando você pede pelo celular (no **AGY Mobile**): *"Publica o projeto X em `app.marcosryan.com`"*, o AGY Mobile apenas transmite seu pedido para o `agy` rodando no computador.
- O `agy` do computador lê a skill local dele, faz o build (`npm run build`), envia via SSH/SCP para o Raspberry Pi (`192.168.1.210`), atualiza o `/opt/stacks/caddy/Caddyfile` e recarrega o Caddy!
- Portanto, para que o **AGY do Windows** também saiba publicar sites no Raspberry Pi, basta copiar a pasta `skills/publish-rpi-site` para `%USERPROFILE%\.gemini\config\skills\publish-rpi-site\`.

---

## 2. Fase 1 — Pré-Requisitos de Rede e SSH no Windows

Para que o AGY no Windows converse com o **Raspberry Pi 5 (`192.168.1.210`)** sem pedir senha:

- [ ] **1.1. Configurar o alias SSH `rpi` no Windows (`%USERPROFILE%\.ssh\config`):**
  ```sshconfig
  Host rpi
      HostName 192.168.1.210
      User ryserver
      Port 22
      IdentityFile ~/.ssh/id_ed25519
      ServerAliveInterval 30
      ServerAliveCountMax 3
  ```
- [ ] **1.2. Autorizar a chave SSH do Windows no Raspberry Pi:**
  No PowerShell do Windows:
  ```powershell
  if (!(Test-Path "$HOME\.ssh\id_ed25519.pub")) { ssh-keygen -t ed25519 -N '""' -f "$HOME\.ssh\id_ed25519" }
  type "$HOME\.ssh\id_ed25519.pub" | ssh ryserver@192.168.1.210 "mkdir -p ~/.ssh && cat >> ~/.ssh/authorized_keys"
  ```
- [ ] **1.3. Testar conectividade com o Raspberry Pi:**
  ```powershell
  ssh rpi "uname -a && docker ps --format 'table {{.Names}}\t{{.Status}}'"
  ```

---

## 3. Fase 2 — Conectar Todos os Servidores MCP no AGY do Windows

No Windows, o AGY lê os servidores MCP em `%USERPROFILE%\.gemini\config\mcp_config.json`.

- [ ] **2.1. Conectar `ai-memory` / `ai-hub` (Memória Persistente no Raspberry Pi):**
  - **Endpoint ativo na LAN:** `http://192.168.1.210:49374/mcp`
  - Permite que o AGY do Windows consulte **todo o histórico arquitetural do Linux e do RPi** (`memory_query`, `memory_read_page`, `memory_write_page`) em tempo real!
- [ ] **2.2. Conectar `disbox` (Servidor MCP Nativo — TorBox, Jogos, Filmes, Animes):**
  - **Opção A (Remoto via SSE):** `https://disbox.pousada.space/v1/mcp/sse`
  - **Opção B (Binário Go Local via Stdio):** compilar ou apontar para `disbox.exe --mcp` (repositório `Phoenix7290/disbox`, branch `feat/mcp-native-server`).
  - **10 Tools disponíveis:** `disbox_search_media`, `disbox_search_tmdb`, `disbox_search_anime`, `disbox_search_games`, `disbox_get_game_downloads`, `disbox_add_torrent`, `disbox_add_webdl`, `disbox_get_history`, `disbox_get_queue`, `disbox_send_ftp`.
- [ ] **2.3. Conectar `fin-vault` (Ledger Financeiro Beancount v3 no Raspberry Pi):**
  - **Endpoint MCP no RPi:** `http://192.168.1.210:8095/mcp` (ou `/sse`)
  - **8 Tools disponíveis:** `get_net_worth`, `audit_credit_cards`, `get_installments_forecast`, `analyze_marketplace_spending`, `audit_subscriptions_and_warranties`, `check_unmatched_transfers`, `generate_irpf_summary`, `export_anonymized_snapshot`.
- [ ] **2.4. Conectar `playwright` (Automação e QA de Browser):**
  - Requer Node.js instalado no Windows (`npx -y @playwright/mcp@latest`).

### Exemplo pronto de `%USERPROFILE%\.gemini\config\mcp_config.json` para o Windows:
```json
{
  "mcpServers": {
    "ai-memory": {
      "url": "http://192.168.1.210:49374/mcp"
    },
    "ai-hub": {
      "url": "http://192.168.1.210:49374/mcp"
    },
    "fin-vault": {
      "url": "http://192.168.1.210:8095/mcp"
    },
    "disbox": {
      "url": "https://disbox.pousada.space/v1/mcp/sse"
    },
    "playwright": {
      "command": "npx",
      "args": ["-y", "@playwright/mcp@latest"]
    }
  }
}
```

---

## 4. Fase 3 — Migrar Skills e Regras Globais (`AGENTS.md`) para o Windows

- [ ] **3.1. Copiar a Skill `publish-rpi-site`:**
  - Copiar `/home/ai/.gemini/config/skills/publish-rpi-site/` (contendo `SKILL.md` e `scripts/deploy_static.sh`) para `%USERPROFILE%\.gemini\config\skills\publish-rpi-site\`.
  - No Windows, o AGY pode rodar o `deploy_static.sh` via Git Bash (`bash.exe`) ou WSL.
- [ ] **3.2. Instalar o `AGENTS.md` Global no Windows (`%USERPROFILE%\AGENTS.md`):**
  - Manter as diretrizes de segurança (proibição de leitura de `.env`/chaves privadas), consulta obrigatória ao `ai-memory` antes de planejar, idioma espelhado com *English Coach* e delegação de leitura de arquivos grandes ao Ollama local.

---

## 5. Fase 4 — Ollama Local + `ollama-worker` na RTX 5070 Ti (Windows)

Como o dual boot roda no mesmo hardware (**Ryzen 9 7900 + RTX 5070 Ti 16GB VRAM**):

- [ ] **4.1. Instalar Ollama para Windows com variáveis de otimização de VRAM:**
  Definir variáveis de ambiente do usuário no Windows:
  - `OLLAMA_HOST=0.0.0.0:11434`
  - `OLLAMA_KEEP_ALIVE=5m` (libera os 16 GB de VRAM após 5 min ocioso para não afetar jogos como League of Legends!)
  - `OLLAMA_FLASH_ATTENTION=1`
- [ ] **4.2. Baixar os modelos equivalentes no Windows:**
  ```powershell
  ollama pull qwen3:8b
  ollama pull qwen3:14b
  ```
- [ ] **4.3. Disponibilizar o `ollama-worker` no Windows (via Git Bash / WSL ou wrapper PowerShell):**
  - Apontar para o script `/home/ryanslx/code/scripts/ollama-worker` (ou cópia em `C:\Users\<user>\bin\ollama-worker`) para que o AGY no Windows também economize franquia de tokens da nuvem usando `-M` (map) e `-e` (extract).

---

## 6. Fase 5 — Integrar o Windows ao `AGY Mobile Gateway`

Você tem **duas formas** de usar o AGY Mobile quando estiver no Windows:

- [ ] **Opção A — Rodar o Gateway Standalone direto no Windows (`agy-gateway-windows-amd64.exe`):**
  1. Baixar da Release `v3.3.0` (`agy-gateway-windows-amd64.exe`) ou compilar `backend-go`.
  2. Criar `.env` ao lado do `.exe`:
     ```env
     GATEWAY_MODE=standalone
     PORT=8088
     OLLAMA_URL=http://127.0.0.1:11434
     ```
  3. Pastas automáticas no Windows:
     - **Inbox:** `%USERPROFILE%\Documents\AGY-Gateway\inbox`
     - **Outbox:** `%USERPROFILE%\Documents\AGY-Gateway\outbox`
- [ ] **Opção B — Manter o Gateway no Raspberry Pi (`192.168.1.210:8088`) apontando para o Windows:**
  - Se o OpenSSH Server estiver ativo no Windows, o próprio RPi pode enviar comandos SSH para o usuário do Windows quando o CachyCore estiver bootado no Windows!

---

## 7. Prompt Pronto de Auto-Configuração (Cole no AGY do Windows)

Quando você abrir o AGY no Windows, cole o bloco abaixo:

```text
Você é a instância Windows do meu ambiente CachyCore (Dual Boot Ryzen 9 7900 + RTX 5070 Ti 16GB VRAM).
Execute o checklist de integração com a minha infraestrutura pré-existente:
1. Configure meu arquivo %USERPROFILE%\.gemini\config\mcp_config.json conectando:
   - ai-memory e ai-hub em http://192.168.1.210:49374/mcp
   - fin-vault em http://192.168.1.210:8095/mcp
   - disbox em https://disbox.pousada.space/v1/mcp/sse
   - playwright via npx -y @playwright/mcp@latest
2. Teste a conexão HTTP com http://192.168.1.210:49374/mcp e consulte no ai-memory (global=true) as páginas "projects/agy-mobile-gateway.md", "projects/disbox/mcp-native-server.md", "infra/rpi-fin-vault.md" e "stacks/ollama-open-webui.md" para absorver o contexto completo do meu ecossistema.
3. Verifique se o alias SSH "rpi" (ryserver@192.168.1.210) está configurado em %USERPROFILE%\.ssh\config.
4. Verifique se o Ollama local está respondendo em http://127.0.0.1:11434 com OLLAMA_KEEP_ALIVE=5m e OLLAMA_FLASH_ATTENTION=1.
5. Me apresente um relatório de status do que já está online e pronto para uso.
```
