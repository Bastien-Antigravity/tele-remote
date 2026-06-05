---
microservice: obsidian-brain
type: note
status: active
tags:
- '#service/obsidian-brain'
- '#type/note'
- '#state/active'
- '#zone/3-fleet'
---# Tele-Remote

Tele-Remote is a central remote control and telemetry bridge that connects a cluster of distributed backend components (like trading bots, worker processes, or microservices) to a centralized Telegram Bot interface. 

It provides an efficient two-way communication channel:
1. **Telemetry & Logging:** Connected components can stream logs, alerts, and structured messages (Qmsg) to Tele-Remote, which routes them directly to a designated Telegram chat.
2. **Command & Control:** You can issue commands via Telegram (e.g., *Power Off*, *Close All Positions*) which are instantly broadcast to all connected components.

## Communication Architecture (Pub/Sub)

Tele-Remote uses a transport-agnostic **IPublisher/ISubscriber** interface model. This allows the system to seamlessly handle multiple connection protocols without changing the Telegram Bot logic:

- **gRPC (Streams):** Modern, high-performance, typed bidirectional streaming.
- **NATS (Topics):** Distributed messaging using NATS Core or Jetstream.

Each connection is wrapped as an internal `interfaces.IPublisher`, ensuring that commands are routed precisely to the correct client regardless of the protocol.

## Shared Infrastructure

Tele-Remote integrates with a suite of centralized Go libraries maintained across the ecosystem:

- **[message-serializers](https://github.com/Bastien-Antigravity/message-serializers):** High-efficiency JSON/Binary marshaling.
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
nats:
  servers: ["nats://localhost:4222"]
  subject_prefix: "teleremote"
```

**Running the Go Server:**
```bash
go mod tidy
go build ./cmd/tele-remote/...
./tele-remote
```

## Available Telegram Commands
When the user sends `/start` to the Bot, it replies with an interactive keyboard:
* **🆘 power off !** : Gracefully signals all components to power off.
* **⏏️ close all positions** : Signals connected systems to halt strategies and close open positions.
* **🔌 Connected Nodes** : Lists all active components and dynamically generates their command menus on-the-fly!

## Persistence
Tele-Remote persists the component registry to `src/assets/registry_state.json`. Upon restart, the bot restores dynamic menus and re-registers the associated command logic, allowing for seamless operation even if components are temporarily disconnected.
