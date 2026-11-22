---
microservice: tele-remote
type: note
status: active
tags:
- '#service/tele-remote'
- '#type/note'
- '#state/active'
- '#zone/3-fleet'
---
Tele-Remote is a central remote control and telemetry bridge that connects a cluster of distributed backend components (like trading bots, worker processes, or microservices) to a centralized Telegram Bot interface.

It provides an efficient two-way communication channel:

1. **Telemetry & Logging:** Connected components can stream logs, alerts, and structured messages to Tele-Remote, which queues them non-blockingly and routes them to a designated Telegram chat.
2. **Command & Control:** You can issue commands via Telegram (e.g., *Power Off*, *Close All Positions*) which are protected by `ChatID` authorization and 2-step emergency confirmation before broadcasting to connected components.

## Communication Architecture (Pub/Sub)

Tele-Remote uses a transport-agnostic **IPublisher/ISubscriber** interface model. This allows the system to seamlessly handle multiple connection protocols without changing the Telegram Bot logic:

- **gRPC (Streams):** Modern, high-performance, typed bidirectional streaming.

Each connection is wrapped as an internal `interfaces.IPublisher`, ensuring that commands are routed precisely to the correct client and that disconnects only clean up the session if the disconnecting stream's publisher matches the active session.

## Shared Infrastructure

Tele-Remote integrates with a suite of centralized Go libraries maintained across the ecosystem:

- **[microservice-toolbox](https://github.com/Bastien-Antigravity/microservice-toolbox):** Handles dynamic UI registration and automatic command routing.
- **[universal-logger](https://github.com/Bastien-Antigravity/universal-logger):** Unified structured logging with I-prefix standards.
- **distributed-config:** Global configuration for clustered deployments.

## Configuration & Setup

### Unified Ecosystem Integration

`tele-remote` bootstraps using `microservice-toolbox` (`toolbox_bootstrap.BootstrapService("tele-remote")`), backed by `distributed-config`.

Configuration is loaded from `standalone.yaml` (symlinked to the active profile, e.g. `native.yaml`).

**Ecosystem Capability (`standalone.yaml`):**

```yaml
capabilities:
  tele_remote:
    token: "ENC(...) or your_bot_token"
    chat_id: "ENC(...) or your_chat_id"
    ip: "127.0.0.1"
    port: "1863" # Standard gRPC binding port
```

Secrets prefixed with `ENC(...)` are automatically decrypted on-demand via the ecosystem's secure master key.

Alternatively, environment variables `TR_TOKEN`, `TR_CHATID`, `TR_IP`, and `TR_PORT` can be supplied.

**Running the Go Server:**

```bash
go build -o bin/tele-remote ./cmd/tele-remote/main.go
./bin/tele-remote
```

## Available Telegram Commands

All routes enforce `ChatID` authorization middleware (`AuthMiddleware`). When an authorized user sends `/start` to the Bot, it replies with an interactive Reply Keyboard:

* **🆘 power off !** : Requests 2-step confirmation (`✅ CONFIRM POWER OFF` / `❌ Cancel`) before signaling all connected components to power off.
* **⏏️ close all positions** : Requests 2-step confirmation (`✅ CONFIRM CLOSE ALL POSITIONS` / `❌ Cancel`) before signaling connected systems to halt strategies and close open positions.
* **⚙️ [Component Name]** : Interactive control menus provided dynamically by the microservices themselves.

### Dynamic UI (Stateless In-Memory Live Registry)

Tele-Remote operates a stateless, in-memory live registry:

1. **Instant Navigation & Direct Button Dispatch:** Menu buttons are matched directly in the user's current submenu tree (scoped by `ConversationKey{ChatID, UserID}`) without unbounded callback maps.
2. **Atomic Background Sync:** Microservices push fresh UI JSON trees in the background; payloads are validated before atomically updating the in-memory registry.
3. **Manual Refresh:** A `🔄 refresh` button is always available in the footer to request an immediate sync from the microservice.
4. **Scoped Input Prompts**: The bot prompts for text (e.g., "Enter new IP") and routes the reply directly back to the originating `ClientID` recorded on the `PendingInput`.
