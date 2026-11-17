# AGY Cockpit (`packages/cockpit`)

Pacote **opcional fora do Core** para monitoramento vivo e controle remoto da workstation (gráficos SVG de histórico, top processos RAM/GPU, controle Docker `start/stop/logs` e ações de energia protegidas por confirmação).

> O Core (`backend-go/`) não compila nem executa nada deste pacote por padrão (`ENABLE_COCKPIT=false`). Quando habilitado no Gateway (`ENABLE_COCKPIT=true` + `COCKPIT_URL`), o Core atua apenas como proxy autenticado em `/api/cockpit/*` e carrega a interface do pacote sob demanda.

## Recursos

- **Histórico em tempo real (ring buffer):** coleta contínua configurável de **VRAM (%)**, **Temperatura da GPU (°C)**, **CPU (% / Temp)** e **RAM (%)**, servida para renderização leve em SVG.
- **Top Processos / Docker:** lista os processos com maior consumo de RAM/VRAM (`nvidia-smi` + SO nativo) e containers Docker (`docker ps -a`) com ações de `start`, `stop` e visualização de `logs`.
- **Ações de Energia protegidas:** **Bloquear tela** (`BLOQUEAR`), **Suspender** (`SUSPENDER`) e **Reiniciar** (`REINICIAR`) exigem confirmação explícita no payload e podem ser totalmente desativadas com `COCKPIT_ALLOW_POWER=false`.

## Executando na Workstation (Windows / Linux / macOS)

```bash
cd packages/cockpit
go build -ldflags="-s -w" -o agy-cockpit ./cmd/agy-cockpit
./agy-cockpit
```

No `agy-gateway` (Core), habilite o mod opcional:

```env
ENABLE_COCKPIT=true
COCKPIT_URL=http://127.0.0.1:8099
# Em modo Gateway (Pi -> Workstation na LAN), aponte para o IP do PC:
# COCKPIT_URL=http://192.168.1.190:8099
# COCKPIT_TOKEN=troque-por-um-segredo-forte
```

## Variáveis do `agy-cockpit`

| Variável | Padrão | Descrição |
| :--- | :--- | :--- |
| `COCKPIT_ADDR` | `127.0.0.1:8099` | Endereço de bind HTTP (use `0.0.0.0:8099` para acesso via LAN/Pi) |
| `COCKPIT_SAMPLE_INTERVAL_SEC` | `5` | Intervalo de amostragem do histórico em segundos |
| `COCKPIT_HISTORY_SAMPLES` | `72` | Quantidade máxima de amostras mantidas no ring buffer (72 * 5s = 6 min) |
| `COCKPIT_ALLOW_POWER` | `true` | Define se `lock`/`suspend`/`reboot` ficam permitidos |
| `COCKPIT_TOKEN` | *(vazio)* | Token Bearer opcional exigido pelo agente Cockpit |

## Perfil Docker Opcional

```bash
docker compose --profile cockpit up -d --build
```
