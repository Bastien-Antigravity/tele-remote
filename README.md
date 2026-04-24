# Tele-Remote

Tele-Remote is a central remote control and telemetry bridge that connects a cluster of distributed backend components (like trading bots, worker processes, or microservices) to a centralized Telegram Bot interface. 

It provides an efficient two-way communication channel:
1. **Telemetry & Logging:** Connected components can stream logs, alerts, and structured messages (Qmsg) to Tele-Remote, which routes them directly to a designated Telegram chat.
2. **Command & Control:** You can issue commands via Telegram (e.g., *Power Off*, *Close All Positions*) which are instantly broadcast to all connected components.

## Communication Architecture (Pub/Sub)

Tele-Remote uses a transport-agnostic **Publisher/Subscriber** interface model. This allows the system to seamlessly handle multiple connection protocols without changing the Telegram Bot logic:

- **gRPC (Streams):** Modern, high-performance, typed bidirectional streaming.
- **NATS (Topics):** Distributed messaging using NATS Core or Jetstream.
- **SafeSocket (TCP/Unix):** Low-overhead, binary-safe socket communication.

Each connection is wrapped as an internal `interfaces.Publisher`, ensuring that commands are routed precisely to the correct client regardless of the protocol.

## Shared Infrastructure

Tele-Remote integrates with a suite of centralized Go libraries maintained across the ecosystem:

- **[message-serializers](https://github.com/Bastien-Antigravity/message-serializers):** High-efficiency JSON/Binary marshaling.
- **[flexible-logger](https://github.com/Bastien-Antigravity/flexible-logger):** Unified structured logging.
- **distributed-config:** Global configuration for clustered deployments.

## Configuration & Setup

### Go Service

The Go server utilizes `viper` and `distributed-config` for unified settings.

**Key Configuration (config.yaml):**
```yaml
TB_TOKEN: "your_bot_token"
TB_CHATID: "your_chat_id"
TB_IP: "0.0.0.0"
TB_PORT: 50051  # gRPC binding port
nats:
  servers: ["nats://localhost:4222"]
  subject_prefix: "teleremote"
safesocket:
  port: 6000
```

**Running the Go Server:**
```bash
go mod tidy
go build ./cmd/tele-remote/...
./tele-remote
```

### Python Service (Legacy)

The original Python implementation using raw TCP sockets is located in `legacy-py/`.

```bash
python3 legacy-py/tele_remote.py --name "tele_remote" --log_level 10
```

## Available Telegram Commands
When the user sends `/start` to the Bot, it replies with an interactive keyboard:
* **🆘 power off !** : Gracefully signals all components to power off.
* **⏏️ close all positions** : Signals connected systems to halt strategies and close open positions.
* **🔌 Connected Nodes** : Lists all active components and dynamically generates their command menus on-the-fly!

## Documentation
For a detailed diagram and resume of the data flow, please see:
- [doc/doc.doc](doc/doc.doc)
