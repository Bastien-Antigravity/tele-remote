
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

Tele-Remote is the primary user interface gateway for the Bastien-Antigravity ecosystem, providing a dynamic command-and-control bridge via Telegram.

## 1. High-Level Data Flow

```text
         [ TELEGRAM APP ]           [ TELE-REMOTE SERVER (GO) ]        [ CLIENT COMPONENTS ]
                |                             |                             |
                | (1) Command (Reply Button)  |                             |
                |---------------------------->| [RateLimit -> Auth -> Log]  |
                |                             | (2) PublishCommand (ctx)    |
                |                             |---------------------------->|
                |                             | [TLS/mTLS + x-tele-auth +   |
                |                             |  1st-Frame Reg Gate]        |
                |                             | (3) Telemetry (chan cap 256)|
                |<----------------------------|<----------------------------|
```

## 2. Component Registration (Atomic Menu-on-the-Fly)

When a client (Trading Bot, Ingestor, etc.) connects, it performs a **Registration** handshake on the first gRPC stream frame. It sends a structured JSON payload containing its:
- Component Name & Port
- Registration Metadata (optional `priority` / `order` / `component_priority`)
- **Menu JSON**: A recursive tree of rows, buttons, commands, and sub-menus (capped at **64 KB** / `maxMenuJSONBytes = 65536` and **8 nesting levels** / `maxMenuDepth = 8`).

Tele-Remote validates size, nesting depth, and JSON structure before acquiring `bot.Mu.Lock()`, ensuring malformed JSON never clobbers an existing valid menu, increments `ComponentMenu.Revision`, maps the component session directly to its stream `IPublisher`, and orders components (OpenMFE priority + deterministic tie-breaking) and rows/buttons numerically with duplicate-order tolerance.

## 3. Standardized Toolbox Integration

Tele-Remote follows the ecosystem's **Unified Microservice Architecture**:

- **Configuration**: Uses `microservice-toolbox/go/pkg/config` (`AppConfig`) with live config reload (`OnLiveConfUpdate`) for runtime TLS/mTLS and rate-limit updates.
- **Lifecycle**: Managed via `toolbox_lifecycle` with a bounded **5s** (`gracefulTimeout = 5s`) gRPC `GracefulStop` before forcing `Stop`.
- **Logging**: Integrated with `universal-logger` for centralized telemetry.
- **Connectivity**: Uses gRPC bidirectional streaming (`teleremote.TeleRemoteService/Connect`) with `dynamicTransportCreds` and WireTap interceptors.

## 4. Package Structure

- `cmd/tele-remote/`: Main entry point, secret decryption, live-reload wiring, and `ExchangeContract` builder (`contracts.go`).
- `src/telegram/core/`: Telegram Bot state machine, bounded sliding-window `RateLimiter`, strict `AuthMiddleware`, pending-input redacting `LoggingMiddleware`, chunked telemetry worker, and session-safe disconnect cleanup.
- `src/telegram/routes/`: Registers `RateLimitMiddleware`, `AuthMiddleware`, `LoggingMiddleware`, and Telegram command handlers (`/start`, emergency confirmation triggers, etc.).
- `src/telegram/ui/`: Parses/guards JSON menu trees, sorts OpenMFE priorities and numeric row/button orders, renders dynamic `ReplyKeyboardMarkup` menus, executes parallel emergency broadcasts, and dispatches button actions directly.
- `src/subscribers/`: gRPC Transport listener (`GrpcSubscriber`) with runtime-toggleable TLS/mTLS, `x-tele-auth` token verification, strict first-frame registration gating, and bounded 5s shutdown.
- `src/publishers/`: Context-aware gRPC Transport sender that dispatches commands and refresh requests.
- `src/models/`: Shared in-memory UI types (`ComponentMenu` with `Priority`/`Revision`), `ConversationKey`, `PendingInput` (with `MenuRevision`), and `PendingConfirmation` structures.

## 5. Stateless In-Memory Registry & Security Hardening

- **Purely In-Memory Live Registry**: Tele-Remote is stateless by design and does not persist registry state to disk. The UI state is built strictly from active gRPC streams (`DynamicMenus` and `Publishers`).
- **Runtime-Toggleable & Local/Remote-Aware gRPC TLS/mTLS**: `dynamicTransportCreds` peeks the first byte (`0x16`) to detect TLS `ClientHello` and evaluates `tls_enabled`, `tls_mode` (`auto` | `strict` | `disabled`), and `tls_verify_client` live via `OnLiveConfUpdate`. In `auto` mode, remote peers must use TLS while loopback/local peers may use plaintext; when explicit cert/key files are omitted, Tele-Remote falls back to `private_key_path` (default `/etc/bastien/private.pem`) to synthesize a self-signed server certificate from the global RSA key.
- **gRPC Token & Strict First-Frame Registration Gate**: When `auth_token` is configured, streams must present matching `x-tele-auth` (or `authorization` bearer) metadata; the first frame must be a valid `Registration` message (non-empty component name ≤ 64 chars, valid port `0–65535`) and component identity is pinned for the stream lifetime, rejecting violations with `codes.PermissionDenied` (`"⛔ Unauthorized access: ..."`).
- **Strict Telegram Sender Authorization**: `IsAuthorized` checks `c.Sender().ID` against `AuthorizedIDs` (`chat_id` + decrypted `allowed_users`) before falling back to private `c.Chat().ID > 0`, preventing unauthorized users inside authorized group chats from issuing commands.
- **Pending-Input Log Redaction**: `LoggingMiddleware` replaces message text with `[REDACTED_PENDING_INPUT]` whenever `PendingInputs` is active for that `ConversationKey`, preventing secrets entered at prompts from leaking to logs.
- **Bounded Sliding-Window Rate Limiter**: `RateLimitMiddleware` enforces per-`ConversationKey` sliding-window flood protection (default burst **60** requests / **2s**, configurable via `burst_limit`, 3s warning cooldown) with LRU/stale eviction capped at `MaxTrackedClients = 4096`.
- **2-Step Emergency Confirmation & Parallel Broadcast**: Critical fleet broadcast (`🆘 power off !`) requires explicit confirmation within 60 seconds via a dedicated confirmation Reply Keyboard (`✅ CONFIRM ...` / `❌ Cancel`) and fans out `PublishCommand` in parallel across connected components with per-component delivery status reporting.
- **Direct Button Dispatch & Session-Safe Reconnect Guards**: Buttons resolve directly from the in-memory `CommandMenu` tree. `OnDisconnect(clientID, dynPub)` only removes a session if `bot.Publishers[clientID] == dynPub`, cleans up `UserStates`/`UserPaths`/`PendingInputs`, and `PendingInput.MenuRevision` invalidates stale prompts if the component reconnects or pushes a newer menu revision.
- **Non-Blocking Telemetry & 4096-Char Chunking**: `OnTelemetry` enqueues messages into a bounded channel (`telemetryChan`, capacity 256); `Broadcast` splits payloads via `splitTelegramMessage` (`maxTelegramMessageLen = 4000`, newline-aware) to stay safely under Telegram's 4096-character limit.
- **Bounded Graceful Shutdown**: `GrpcSubscriber.Shutdown` closes `stopCh` to unblock active `Recv()` loops and runs `GracefulStop` with a **5s** deadline before calling `Stop()`.

## 6. Advanced UI Features

### Optimistic UI & Background Sync
1. Navigation is handled locally by the bot using a per-`ConversationKey{ChatID, UserID}` path stack, rendering submenus immediately.
2. When a service executes an action, it pushes a fresh JSON tree to the bot in the background.
3. Pressing `🔄 refresh` dispatches a non-blocking `RequestRefresh` to the microservice and re-renders the current menu immediately.

### Scoped Input State Machine
1. Client defines a `CommandButton` with an `InputPrompt`.
2. User clicks the button; Tele-Remote stores `PendingInput{ClientID, Button, Path, ExpiresAt}` under `ConversationKey{ChatID, UserID}` and prompts the user.
3. When the user replies, Tele-Remote verifies the prompt has not expired and routes the input directly to `pending.ClientID` regardless of any intervening `UserStates` changes.
