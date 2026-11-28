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
Tele-Remote is a central remote control and telemetry bridge that connects a cluster of distributed backend components (microservices, supervisors, and autonomous agents) to a centralized Telegram Bot interface.

It provides an efficient two-way communication channel:

1. **Telemetry & Logging:** Connected components stream logs, alerts, and status messages to Tele-Remote, which queues them non-blockingly (`telemetryChan` capacity `256`), splits long payloads for Telegram's 4096-character limit (`4000` rune chunks), and broadcasts them to authorized Telegram chats.
2. **Command & Control:** Operators issue commands via Telegram (including *Power Off* and dynamic microservice submenus) protected by strict sender authorization (`chat_id` + `allowed_users`), bounded sliding-window rate limiting (`MaxTrackedClients = 4096`, default burst `60` / `2s`), pending-input log redaction, and 2-step emergency confirmation with parallel per-component delivery reporting.
3. **Hardened gRPC Transport:** Supports runtime-toggleable Option A X.509 TLS/mTLS (`tls_enabled`, `tls_mode: auto|strict|disabled`, `local.tele_remote.tls.{ca_file,cert_file,key_file}`), `x-tele-auth` shared token verification, strict first-frame component registration gating (`codes.PermissionDenied: "⛔ Unauthorized access"`), stalled-stream write eviction, and bounded 5s graceful shutdown.

## Communication Architecture (Pub/Sub)

Tele-Remote uses a transport-agnostic **`Publisher` / `Subscriber`** interface model (`src/interfaces/`):

- **gRPC (Bidirectional Streams):** High-performance, typed bidirectional streaming (`teleremote.TeleRemoteService/Connect`).

Each connection is wrapped as an internal `interfaces.Publisher` with bounded write deadlines and automatic stalled-stream eviction, ensuring commands are routed to the active session and stale disconnects never clobber reconnected sessions.

## Shared Infrastructure

Tele-Remote integrates with the ecosystem's shared Go libraries:

- **[microservice-toolbox](https://github.com/Bastien-Antigravity/microservice-toolbox):** Handles service bootstrap (`BootstrapService`), `AppConfig` & `GetVersion()`, Option A dynamic TLS/mTLS (`DynamicTransportCredentials`), WireTap interceptors, OpenMFE `ServiceSpec` registration, and the polyglot `TeleClient` SDK (Go, Python, Rust).
- **[universal-logger](https://github.com/Bastien-Antigravity/universal-logger):** Unified structured logging across the 12 canonical log levels (`INFO`, `LOGON`, `LOGOUT`, `WARNING`, `ERROR`, `CRITICAL`, etc.).
- **distributed-config:** Global configuration and live config hot-reload (`OnLiveConfUpdate`).

## Configuration & Setup

### Unified Ecosystem Integration

`tele-remote` bootstraps using `microservice-toolbox` (`toolbox_bootstrap.BootstrapService("tele-remote")`), backed by `distributed-config`.

Configuration is loaded from `standalone.yaml` (symlinked to `../docker-deployment/modes/local/config/native.yaml`).

**Ecosystem Capability & Service-Local TLS (`standalone.yaml`):**

```yaml
local:
  tele_remote:
    tls:
      ca_file: /etc/bastien/tls/ca.crt
      cert_file: /etc/bastien/tls/tele-remote.crt
      key_file: /etc/bastien/tls/tele-remote.key

capabilities:
  tele_remote:
    token: "ENC(...) or your_bot_token"
    chat_id: "ENC(...) or your_chat_id"
    allowed_users: "ENC(...) or comma_separated_user_ids"
    url: "https://api.telegram.org"
    ip: "127.0.0.1"
    port: "1863" # Canonical gRPC binding port
    burst_limit: 60 # Optional sliding-window burst per 2s (default 60)
    tls_enabled: false # Runtime-toggleable via live config
    tls_mode: "auto" # auto | strict | disabled
    auth_token: "ENC(...) or optional_grpc_shared_token" # Verified via x-tele-auth
```

Secrets prefixed with `ENC(...)` are automatically decrypted on-demand via `appConfig.DecryptSecret()`.

**Running the Go Server:**

```bash
go build -o bin/tele-remote ./cmd/tele-remote
./bin/tele-remote
```

## Available Telegram Commands

All routes enforce sliding-window rate limiting (`RateLimitMiddleware`), strict sender authorization (`AuthMiddleware`, preventing unauthorized users in authorized group chats), and pending-input log redaction (`LoggingMiddleware`). When an authorized user sends `/start` or `/menu` to the Bot, it replies with an interactive Reply Keyboard:

* **🆘 power off !** : Requests 2-step confirmation (`✅ CONFIRM POWER OFF` / `❌ Cancel`) before broadcasting `BotCommand.POWER_OFF` in parallel across all connected components with per-component delivery reporting.
* **📂 / ⚙️ [Component Name]** : Interactive control menus provided dynamically by the microservices themselves, ordered by OpenMFE priority.

### Dynamic UI (Stateless In-Memory Live Registry)

Tele-Remote operates a stateless, in-memory live registry:

1. **Instant Navigation & Direct Button Dispatch:** Menu buttons are matched directly in the user's current submenu tree (scoped by `ConversationKey{ChatID, UserID}`) with path-qualified payload disambiguation for duplicate leaf labels across submenus.
2. **Atomic Background Sync & Payload Guards:** Microservices push fresh UI JSON trees in the background; payloads are bounded (**64 KB** / **8** nesting levels), sorted by numeric row/button order (with duplicate-order tolerance), and validated before atomically updating the in-memory registry.
3. **Manual Refresh:** A `🔄 refresh` button is always available in the footer to request an immediate sync from the microservice.
4. **Scoped Input Prompts & Revision Safety:** The bot prompts for text, redacts the reply from logs, and routes it back to the originating `ClientID` only if `PendingInput.MenuRevision` still matches the active session.
