# Tele-Remote

Tele-Remote is a central remote control and telemetry bridge that connects a cluster of distributed backend components (like trading bots, worker processes, or microservices) to a centralized Telegram Bot interface. 

It provides an efficient two-way communication channel:
1. **Telemetry & Logging:** Connected components can stream logs, alerts, and structured messages (Qmsg) to Tele-Remote, which routes them directly to a designated Telegram chat.
2. **Command & Control:** You can issue commands via Telegram (e.g., *Power Off*, *Close All Positions*) which are instantly broadcast to all connected components.

## Implementations

The project contains two distinct implementations for the server component:

### 1. Go Implementation (gRPC)
The modern, high-performance version is written in Go and utilizes **gRPC** for robust, typed, and bidirectional streaming between the server and the components. 

- **Codebase:** Located in `cmd/`, `src/grpc_control/`, `src/telegram/`, and `src/config/`.
- **Protocol:** Uses Protocol Buffers (`src/grpc_control/teleremote.proto`) natively, with `ComponentMessage` structure handling Registration, Telemetry, and Queue Messages (Qmsg).
- **Control Flow:** When the user clicks the "🆘 power off !" or "⏏️ close all positions" buttons on Telegram, a `BotCommand_POWER_OFF` or `BotCommand_CLOSE_ALL_POSITIONS` enum is broadcasted via gRPC stream to all connected components.

### 2. Python Implementation (Raw TCP Sockets)
The legacy/alternative version is written in Python and uses raw TCP sockets with an overarching custom messaging protocol. 

- **Codebase:** The primary entry point is `tele_remote.py`, alongside supporting `.py` files (`tele_button.py`, `tele_command.py`, `tele_funcs.py`).
- **Modes:** Supports both asynchronous loops (`asyncio`) and traditional threading architectures via the `TELE_REMOTE_SERVER_TYPE` toggle.
- **Dependencies:** Relies on a broader `common` library structure, looking for database, configuration, and thread queuing mechanics typically mapped to the parent directory.

## Configuration & Setup

### Go Service

The Go server utilizes `viper` to process configurations. You can configure it using a `config.yaml` file in the root directory, or via Environment Variables prefixed with `TELEREMOTE_`.

**Key Variables:**
* `TELEREMOTE_TB_TOKEN` (or `TB_TOKEN` in yaml): Your Telegram Bot Token.
* `TELEREMOTE_TB_CHATID` (or `TB_CHATID` in yaml): The Int64 ID of the target Telegram chat room/user.
* `TELEREMOTE_TB_IP` (or `TB_IP` in yaml): IP on which the gRPC server will bind (default: `0.0.0.0`).
* `TELEREMOTE_TB_PORT` (or `TB_PORT` in yaml): gRPC Port (default: `50051`).
* `TELEREMOTE_LOG_LEVEL` (or `LOG_LEVEL` in yaml): Logging verbosity, e.g., `DEBUG` or `INFO`.

**Running the Go Server:**
```bash
go mod tidy
go build ./cmd/teleremote/...
./teleremote
```

### Python Service

If running the Python implementation, rely on flags or the `common.config` configuration path loading approach:

```bash
python3 tele_remote.py --name "tele_remote" --log_level 10
```

## Available Telegram Commands
When the user sends `/start` to the Bot, it replies with a custom Reply Markup keyboard containing actionable buttons:
* **🆘 power off !** : Gracefully signals all components to power off.
* **⏏️ close all positions** : Signals connected systems to halt strategies and close open positions immediately.
* **🍀 running strategies** : (WIP) Poll active strategies.
* **📈📉 arbitrage** : (WIP) Execute/poll arbitrage details.

## Adding a New Client Component (Go)
To connect a new process to Tele-Remote, integrate a gRPC client using the `teleremote.pb.go` generated classes. Connect to the address specified in config, and stream `ComponentMessage` structures to register. Read incoming streams from the server to listen for `BotCommand` triggers.
