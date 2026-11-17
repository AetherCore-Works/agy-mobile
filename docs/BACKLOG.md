# Backlog de Evolução — AGY Mobile Gateway

Itens planejados para os próximos ciclos de refinamento:

## 1. Atalhos de Skills & MCP no Menu `/` (Slash Commands)
- **`/publish`**: Template pronto para acionar a skill `publish-rpi-site` (build + deploy no Caddy do Raspberry Pi).
- **`/disbox`**: Template rápido para consultar/enfileirar jogos (FitGirl/OnlineFix/SteamRip), filmes, séries e animes via MCP do Disbox.
- **`/fin`**: Template para consultas rápidas no ledger financeiro Beancount v3 (`fin-vault` MCP).
- **Benefício:** Zero digitação longa no celular e roteamento determinístico para o modelo Cloud adequado.

## 2. Preview Inline de Mídia do Outbox no Chat
- Quando a IA gerar imagens (`.png`, `.jpg`, `.webp`) ou `.pdf` no Outbox, renderizar miniatura/preview interativo diretamente no card do chat antes mesmo de abrir o modal do Outbox.
