---
microservice: obsidian-brain
type: note
status: active
tags:
- '#service/obsidian-brain'
- '#type/note'
- '#state/active'
- '#zone/3-fleet'
---
Tele-Remote is a central remote control and telemetry bridge that connects a cluster of distributed backend components (like trading bots, worker processes, or microservices) to a centralized Telegram Bot interface.

It provides an efficient two-way communication channel:

1. **Telemetry & Logging:** Connected components can stream logs, alerts, and structured messages to Tele-Remote, which routes them directly to a designated Telegram chat.
2. **Command & Control:** You can issue commands via Telegram (e.g., *Power Off*, *Close All Positions*) which are instantly broadcast to all connected components.

## Communication Architecture (Pub/Sub)

Tele-Remote uses a transport-agnostic **IPublisher/ISubscriber** interface model. This allows the system to seamlessly handle multiple connection protocols without changing the Telegram Bot logic:

- **gRPC (Streams):** Modern, high-performance, typed bidirectional streaming.

Each connection is wrapped as an internal `interfaces.IPublisher`, ensuring that commands are routed precisely to the correct client.

## Shared Infrastructure

Tele-Remote integrates with a suite of centralized Go libraries maintained across the ecosystem:

- **[microservice-toolbox](https://github.com/Bastien-Antigravity/microservice-toolbox):** Handles dynamic UI registration and automatic command routing.
- **[universal-logger](https://github.com/Bastien-Antigravity/universal-logger):** Unified structured logging with I-prefix standards.
- **distributed-config:** Global configuration for clustered deployments.

## Configuration & Setup

### Go Service

The Go server utilizes `viper` (v1.21+) and `distributed-config` for unified settings.

**Key Configuration (config.yaml):**

```yaml
TB_TOKEN: "your_bot_token"
TB_CHATID: "your_chat_id"
TB_IP: "0.0.0.0"
TB_PORT: 50051  # gRPC binding port
```

**Running the Go Server:**

```bash
go build -o bin/tele-remote ./cmd/tele-remote/main.go
./bin/tele-remote
```

## Available Telegram Commands

When the user sends `/start` to the Bot, it replies with an interactive keyboard:

* **🆘 power off !** : Gracefully signals all connected components to power off.
* **⏏️ close all positions** : Signals connected systems to halt strategies and close open positions.
* **⚙️ [Component Name]** : Interactive control menus provided dynamically by the microservices themselves.

### Dynamic UI (Optimistic UI with Background Sync)

Tele-Remote operates a lightning-fast, stateless interface:

1. **Instant Navigation:** When you click a menu button, the bot instantly renders the next screen using its local memory cache (zero network latency).
2. **Background Sync:** Whenever a microservice's internal state changes (e.g., you update a setting), it immediately pushes a fresh UI definition to the bot in the background.
3. **Manual Refresh:** A `🔄 refresh` button is always available in the footer to force a hard sync with the microservice if needed.
4. **Input Prompts**: The bot can prompt for text (e.g., "Enter new IP") and route your reply back to the exact microservice Go function responsible for that setting.
