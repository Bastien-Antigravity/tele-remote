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

# Tele-Remote Architecture

Tele-Remote is the primary operator gateway for the Bastien-Antigravity ecosystem, providing a dynamic command-and-control and telemetry bridge via Telegram.

## 1. High-Level Data Flow

```text
         [ TELEGRAM APP ]           [ TELE-REMOTE SERVER (GO) ]        [ CLIENT COMPONENTS ]
                |                             |                             |
                | (1) Command (Reply Button)  |                             |
                |---------------------------->| [RateLimit -> Auth -> Log]  |
                |                             | (2) PublishCommand (ctx)    |
                |                             |---------------------------->|
                |                             | [Option A TLS/mTLS +        |
                |                             |  x-tele-auth + 1st-Frame]   |
                |                             | (3) Telemetry (chan cap 256)|
                |<----------------------------|<----------------------------|
```

## 2. Component Registration (Atomic Menu-on-the-Fly)

When a client microservice connects via `microservice-toolbox/teleremote` (`TeleClient`), it performs a **Registration** handshake on the first gRPC stream frame:
- Component Name, Host & Port
- Registration Metadata (optional `priority` / `order` / `component_priority`)
- **Menu JSON**: A recursive tree of rows, buttons, commands, and sub-menus (capped at **64 KB** / `maxMenuJSONBytes = 65536` and **8 nesting levels** / `maxMenuDepth = 8`).

Tele-Remote validates size, nesting depth, and JSON structure before acquiring `bot.Mu.Lock()`, ensuring malformed JSON never clobbers an existing valid menu, increments `ComponentMenu.Revision`, maps the component session directly to its stream `interfaces.Publisher`, and orders components (OpenMFE priority + deterministic tie-breaking) and rows/buttons numerically with duplicate-order tolerance.

## 3. Standardized Toolbox Integration

Tele-Remote follows the ecosystem's **Unified Microservice Architecture**:

- **Configuration & Versioning**: Uses `microservice-toolbox/go/pkg/config` (`AppConfig`), `appConfig.GetVersion()` (`VERSION.txt` SSoT), and live config reload (`OnLiveConfUpdate`) for runtime TLS/mTLS, `auth_token`, and rate-limit updates.
- **Cold-Path Fail-Fast & Deterministic Lifecycle**: Synchronously binds the gRPC TCP socket (`grpcSub.Bind()`) on the cold path before announcing readiness or publishing its `ServiceSpec`, and registers both `gRPC_Subscriber` (5s bounded `GracefulStop`) and `Telegram_Bot` (stops polling and drains `telemetryChan`) with `toolbox_lifecycle.Manager`.
- **Logging**: Uses `universal-logger` with canonical levels (`INFO` for lifecycle checkpoints, `LOGON`/`LOGOUT` for gRPC peer sessions, `WARNING` for recoverable anomalies, `ERROR`/`CRITICAL` for failures).
- **Connectivity**: Uses gRPC bidirectional streaming (`teleremote.TeleRemoteService/Connect`) with `DynamicTransportCredentials` and WireTap interceptors.

## 4. Package Structure

- `cmd/tele-remote/`: Main entrypoint, secret decryption, synchronous listener bind, live-reload wiring, and `ExchangeContract` builder (`contracts.go`).
- `src/telegram/core/`: Telegram Bot state machine, bounded sliding-window `RateLimiter`, strict `AuthMiddleware`, pending-input redacting `LoggingMiddleware`, chunked telemetry worker with graceful drain, and session-safe disconnect cleanup.
- `src/telegram/routes/`: Registers `RateLimitMiddleware`, `AuthMiddleware`, `LoggingMiddleware`, and Telegram command handlers (`/start`, `/menu`, emergency confirmation triggers).
- `src/telegram/ui/`: Parses/guards JSON menu trees, sorts OpenMFE priorities and numeric row/button orders, renders dynamic `ReplyKeyboardMarkup` menus, executes parallel emergency broadcasts, evicts expired prompts/confirmations, and dispatches button actions directly.
- `src/subscribers/`: gRPC Transport listener (`GrpcSubscriber`) with Option A runtime-toggleable TLS/mTLS, `x-tele-auth` token verification, strict first-frame registration gating, stalled-stream abort handling, and bounded 5s shutdown.
- `src/publishers/`: Context-aware gRPC Transport sender (`GrpcPublisher`) that serializes commands/refreshes and aborts stalled streams on write timeout so semaphores never wedge.
- `src/interfaces/`: Unprefixed transport contracts (`Publisher`, `Subscriber`, `SubscriberCallbacks`).
- `src/models/`: Shared in-memory UI types (`ComponentMenu` with `Priority`/`Revision`), `ConversationKey`, `PendingInput` (with `MenuRevision`), and `PendingConfirmation` structures.

## 5. Stateless In-Memory Registry & Security Hardening

- **Purely In-Memory Live Registry**: Tele-Remote is stateless by design and builds UI state strictly from active gRPC streams (`DynamicMenus` and `Publishers`).
- **Option A Runtime-Toggleable & Local/Remote-Aware gRPC TLS/mTLS**: `DynamicTransportCredentials` peeks the first byte (`0x16`) to detect TLS `ClientHello` and evaluates `tls_enabled`, `tls_mode` (`auto` | `strict` | `disabled`), and service-private `local.tele_remote.tls` (`ca_file`, `cert_file`, `key_file`, `verify_client`) live via `OnLiveConfUpdate`.
- **gRPC Token & Strict First-Frame Registration Gate**: When `auth_token` is configured, streams must present matching `x-tele-auth` (or `authorization` bearer) metadata; the first frame must be a valid `Registration` message (non-empty component name ≤ 64 chars, valid port `0–65535`) and component identity is pinned for the stream lifetime, rejecting violations with `codes.PermissionDenied` (`"⛔ Unauthorized access: ..."`).
- **Strict Telegram Sender Authorization**: `IsAuthorized` checks `c.Sender().ID` against `AuthorizedIDs` (`chat_id` + decrypted `allowed_users`) before falling back to private `c.Chat().ID > 0`, preventing unauthorized users inside authorized group chats from issuing commands.
- **Pending-Input Log Redaction**: `LoggingMiddleware` replaces message text with `[REDACTED_PENDING_INPUT]` whenever `PendingInputs` is active for that `ConversationKey`.
- **Bounded Sliding-Window Rate Limiter**: `RateLimitMiddleware` enforces per-`ConversationKey` sliding-window flood protection (default burst **60** requests / **2s**, configurable via `burst_limit`, 3s warning cooldown) capped at `MaxTrackedClients = 4096`.
- **2-Step Emergency Confirmation & Parallel Broadcast**: Critical fleet broadcast (`🆘 power off !`) requires explicit confirmation within 60 seconds (`✅ CONFIRM POWER OFF` / `❌ Cancel`) and fans out `PublishCommand` in parallel across connected components with per-component delivery reporting.
- **Stalled-Stream Write Eviction**: If `GrpcPublisher.PublishCommand` or `RequestRefresh` exceeds its context deadline on a stalled HTTP/2 stream, `GrpcPublisher` marks itself aborted and triggers `GrpcSubscriber`'s stream abort hook, immediately evicting the dead session and failing fast on subsequent writes.
