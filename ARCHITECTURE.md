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
                |---------------------------->| [AuthMiddleware (ChatID)]   |
                |                             | (2) PublishCommand (ctx)    |
                |                             |---------------------------->|
                |                             |                             |
                |                             | (3) Telemetry (chan cap 256)|
                |<----------------------------|<----------------------------|
```

## 2. Component Registration (Atomic Menu-on-the-Fly)

When a client (Trading Bot, Ingestor, etc.) connects, it performs a **Registration** handshake. It sends a structured JSON payload containing its:
- Component Name
- Registration Metadata
- **Menu JSON**: A recursive tree of buttons, commands, and sub-menus.

Tele-Remote validates and unmarshals the JSON tree before acquiring `bot.Mu.Lock()`, ensuring malformed JSON never clobbers an existing valid menu, and maps the component session directly to its stream `IPublisher`.

## 3. Standardized Toolbox Integration

Tele-Remote follows the ecosystem's **Unified Microservice Architecture**:

- **Configuration**: Uses `microservice-toolbox/go/pkg/config` for network-aware settings.
- **Lifecycle**: Managed via `toolbox_lifecycle` for graceful shutdown of transport layers.
- **Logging**: Integrated with `universal-logger` for centralized telemetry.
- **Connectivity**: Uses gRPC for high-performance bidirectional component communication.

## 4. Package Structure

- `cmd/tele-remote/`: Main entry point and bootstrap logic.
- `src/telegram/core/`: Telegram Bot state machine, `ChatID` `AuthMiddleware`, non-blocking telemetry worker, and session-safe disconnect cleanup.
- `src/telegram/routes/`: Registers global middlewares and Telegram command handlers (`/start`, emergency confirmation triggers, etc.).
- `src/telegram/ui/`: Parses JSON menu trees, renders dynamic `ReplyKeyboardMarkup` menus, and dispatches button actions directly.
- `src/subscribers/`: gRPC Transport listener that receives component registrations and telemetry.
- `src/publishers/`: Context-aware gRPC Transport sender that dispatches commands and refresh requests.
- `src/models/`: Shared in-memory UI types, `ConversationKey`, `PendingInput`, and `PendingConfirmation` structures.

## 5. Stateless In-Memory Registry & Security Hardening

- **Purely In-Memory Live Registry**: Tele-Remote is stateless by design and does not persist registry state to disk. The UI state is built strictly from active gRPC streams (`DynamicMenus` and `Publishers`).
- **Telegram `ChatID` Authorization**: `AuthMiddleware` verifies every incoming message and callback against the configured `ChatID`, rejecting unauthorized callers before handler execution.
- **2-Step Emergency Confirmation**: Critical fleet broadcasts (`🆘 power off !` and `⏏️ close all positions`) require explicit confirmation within 60 seconds via a dedicated confirmation Reply Keyboard (`✅ CONFIRM ...` / `❌ Cancel`).
- **Direct Button Dispatch**: Buttons in the active submenu are resolved directly from the in-memory `CommandMenu` tree without an unbounded `ActionMap`.
- **Session-Safe Reconnect & Disconnect Guards**: `OnDisconnect(clientID, dynPub)` checks that `bot.Publishers[clientID] == dynPub` before deleting the session so an old closing stream never deletes a newly reconnected session, and cleans up any `UserStates`, `UserPaths`, and `PendingInputs` referencing a disconnected `clientID`.
- **Non-Blocking Telemetry & Context-Guarded Publish**: `OnTelemetry` enqueues messages into a bounded channel (`telemetryChan`, capacity 256) serviced by a background worker, and `GrpcPublisher` checks `ctx.Err()` prior to locking and sending.

## 6. Advanced UI Features

### Optimistic UI & Background Sync
1. Navigation is handled locally by the bot using a per-`ConversationKey{ChatID, UserID}` path stack, rendering submenus immediately.
2. When a service executes an action, it pushes a fresh JSON tree to the bot in the background.
3. Pressing `🔄 refresh` dispatches a non-blocking `RequestRefresh` to the microservice and re-renders the current menu immediately.

### Scoped Input State Machine
1. Client defines a `CommandButton` with an `InputPrompt`.
2. User clicks the button; Tele-Remote stores `PendingInput{ClientID, Button, Path, ExpiresAt}` under `ConversationKey{ChatID, UserID}` and prompts the user.
3. When the user replies, Tele-Remote verifies the prompt has not expired and routes the input directly to `pending.ClientID` regardless of any intervening `UserStates` changes.
