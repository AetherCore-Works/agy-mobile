# Windows Onboarding Guide — AGY Mobile Gateway

This guide describes how to connect a Windows workstation running `agy` to your **AGY Mobile Gateway** (standalone or remote gateway mode) and optional local services such as Ollama or MCP servers.

## 1. Architecture Overview

| Component | Where it Runs | Responsibility |
| :--- | :--- | :--- |
| **AGY Mobile Gateway** | Server/SBC (e.g. Raspberry Pi) or standalone Windows binary (`agy-gateway-windows-amd64.exe`) | Mobile-first Web/PWA control plane, SSE streaming, Inbox/Outbox, Wake-on-LAN, and terminal proxy. |
| **AGY CLI Runtime** | Target workstation (`%USERPROFILE%\.gemini\config\` on Windows or `~/.gemini/config/` on Linux) | Executes prompts, loads configured MCP servers, and applies workspace rules. |

## 2. SSH Key Setup (Gateway Mode Only)

If running the gateway on a separate host (such as a Raspberry Pi) targeting your Windows machine:

```powershell
ssh-keygen -t ed25519 -f "$HOME\.ssh\id_ed25519"
```

Add the public key to the gateway/target `authorized_keys` and configure `~/.ssh/config`:

```sshconfig
Host agy-gateway-host
    HostName 192.0.2.10
    User your-user
    IdentityFile ~/.ssh/id_ed25519
```

## 3. Standalone Mode on Windows (Quickest Setup)

Download `agy-gateway-windows-amd64.exe` from [GitHub Releases](https://github.com/AetherCore-Works/agy-mobile/releases/latest) and start it in PowerShell:

```powershell
$env:GATEWAY_MODE = "standalone"
$env:ENABLE_OLLAMA = "true"
$env:OLLAMA_URL = "http://127.0.0.1:11434"
.\agy-gateway-windows-amd64.exe
```

Open `http://127.0.0.1:8088` in your browser.

## 4. Optional MCP Servers

Configure your preferred MCP servers in `%USERPROFILE%\.gemini\config\mcp_config.json` using your own local or remote endpoints:

```json
{
  "mcpServers": {
    "example-memory": {
      "url": "http://127.0.0.1:49374/mcp"
    }
  }
}
```

## 5. Security Checklist

- Keep `.env`, private SSH keys, and API tokens out of Git.
- Restrict gateway exposure behind a VPN (WireGuard/Tailscale) or reverse proxy with authentication.
- Set `WEBHOOK_SECRET` before enabling optional webhook or bot integrations.
