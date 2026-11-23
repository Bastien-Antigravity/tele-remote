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

1. **Telemetry & Logging:** Connected components can stream logs, alerts, and structured messages to Tele-Remote, which queues them non-blockingly, splits long payloads for Telegram's 4096-character limit (`4000` rune chunks), and routes them to authorized Telegram chats.
2. **Command & Control:** You can issue commands via Telegram (e.g., *Power Off*) protected by strict sender authorization (`chat_id` + `allowed_users`), bounded sliding-window rate limiting (`MaxTrackedClients = 4096`, default burst `60` / `2s`), pending-input log redaction, and 2-step emergency confirmation with parallel per-component delivery reporting.
3. **Hardened gRPC Transport:** Supports runtime-toggleable & local/remote-aware TLS/mTLS (`tls_enabled`, `tls_mode: auto|strict|disabled`, `tls_verify_client`, `private_key_path` global RSA key fallback), `x-tele-auth` shared token verification, strict first-frame component registration gating (`codes.PermissionDenied: "⛔ Unauthorized access"`), and bounded 5s graceful shutdown.

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
    allowed_users: "ENC(...) or comma_separated_user_ids"
    ip: "127.0.0.1"
    port: "1863" # Standard gRPC binding port
    burst_limit: 60 # Optional sliding-window burst per 2s (default 60)
    tls_enabled: true # Runtime-toggleable via live config
    tls_mode: "auto" # auto | strict | disabled
    tls_verify_client: false # Enable mTLS client cert verification
    private_key_path: "/etc/bastien/private.pem" # Global RSA key fallback
    auth_token: "ENC(...) or optional_grpc_shared_token" # Verified via x-tele-auth
```

Secrets prefixed with `ENC(...)` are automatically decrypted on-demand via the ecosystem's secure master key.

Alternatively, environment variables `TR_TOKEN`, `TR_CHATID`, `TR_IP`, and `TR_PORT` can be supplied.

**Running the Go Server:**

```bash
go build -o bin/tele-remote ./cmd/tele-remote
./bin/tele-remote
```

## Available Telegram Commands

All routes enforce sliding-window rate limiting (`RateLimitMiddleware`), strict sender authorization (`AuthMiddleware`, preventing unauthorized users in authorized group chats), and pending-input log redaction (`LoggingMiddleware`). When an authorized user sends `/start` to the Bot, it replies with an interactive Reply Keyboard:

* **🆘 power off !** : Requests 2-step confirmation (`✅ CONFIRM POWER OFF` / `❌ Cancel`) before broadcasting in parallel across all connected components with per-component delivery reporting.
* **⚙️ [Component Name]** : Interactive control menus provided dynamically by the microservices themselves, ordered by OpenMFE priority.

### Dynamic UI (Stateless In-Memory Live Registry)

Tele-Remote operates a stateless, in-memory live registry:

1. **Instant Navigation & Direct Button Dispatch:** Menu buttons are matched directly in the user's current submenu tree (scoped by `ConversationKey{ChatID, UserID}`) without unbounded callback maps.
2. **Atomic Background Sync & Payload Guards:** Microservices push fresh UI JSON trees in the background; payloads are bounded (**64 KB** / **8** nesting levels), sorted by numeric row/button order (with duplicate-order tolerance), and validated before atomically updating the in-memory registry.
3. **Manual Refresh:** A `🔄 refresh` button is always available in the footer to request an immediate sync from the microservice.
4. **Scoped Input Prompts & Revision Safety:** The bot prompts for text (e.g., "Enter new IP"), redacts the reply from logs, and routes it back to the originating `ClientID` only if the `PendingInput.MenuRevision` still matches the active session (invalidating stale prompts on reconnect).
