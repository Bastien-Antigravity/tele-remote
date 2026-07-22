---
microservice: obsidian-brain
type: note
status: active
tags:
- '#service/obsidian-brain'
- '#type/note'
- '#state/active'
- '#zone/3-fleet'
---# Tele-Remote Architecture

Tele-Remote is the primary user interface gateway for the Bastien-Antigravity ecosystem, providing a dynamic command-and-control bridge via Telegram.

## 1. High-Level Data Flow

```text
         [ TELEGRAM APP ]           [ TELE-REMOTE SERVER (GO) ]        [ CLIENT COMPONENTS ]
                |                             |                             |
                | (1) Command (Inline Button) |                             |
                |---------------------------->|                             |
                |                             | (2) PublishCommand (ctx)    |
                |---------------------------->|---------------------------->|
                |                             |                             |
                |                             | (3) Telemetry / Log Update  |
                |<----------------------------|<----------------------------|
```

## 2. Component Registration (Menu-on-the-fly)

When a client (Trading Bot, Ingestor, etc.) connects, it performs a **Registration** handshake. It sends a structured JSON payload containing its:
- Component Name
- Registration Metadata
- **Menu JSON**: A recursive tree of buttons, commands, and sub-menus.

Tele-Remote parses this on-the-fly, dynamically builds a Telegram inline keyboard, and maps every button click back to the specific `IPublisher` associated with that component.

## 3. Standardized Toolbox Integration

Tele-Remote follows the ecosystem's **Unified Microservice Architecture**:

- **Configuration**: Uses `microservice-toolbox/go/pkg/config` for network-aware settings.
- **Lifecycle**: Managed via `toolbox_lifecycle` for graceful shutdown of transport layers.
- **Logging**: Integrated with `universal-logger` for centralized telemetry.
- **Connectivity**: Uses gRPC for high-performance bidirectional component communication.

## 4. Package Structure

- `cmd/tele-remote/`: Main entry point and bootstrap logic.
- `src/telegram/core/`: Telegram Bot state machine and gRPC message dispatching.
- `src/telegram/routes/`: Registers Telegram command handlers (`/start`, etc.).
- `src/telegram/ui/`: Parses JSON actions and renders dynamic `ReplyKeyboardMarkup` menus.
- `src/subscribers/`: gRPC Transport listener that receives component data.
- `src/publishers/`: gRPC Transport sender that dispatches commands and refresh requests.
- `src/models/`: Shared UI types and persistence structures.
- `src/config/`: Ecosystem-aware configuration wrapper.
- `src/store/`: Persistence management for registry state.

## 5. Stateless Design & Reliability
- **Memory-Driven**: The bot does not persist any menu states to disk (`registry_state.json` was removed). The UI state is built purely from live gRPC streams. If the bot restarts, it waits for services to reconnect and push their menus.
- **Session Guards**: When a service reconnects (e.g., changing internal ports), the bot automatically detects and cleans up the old session to prevent duplicate UI buttons.
- **Resilient Commands**: Command dispatch uses `context.WithTimeout` (10s) and transport-level mutexes to ensure reliability under concurrent use.

## 6. Advanced UI Features

### Optimistic UI & Background Sync
To ensure the Telegram UI feels instantaneous while remaining perfectly accurate:
1. Navigation is handled locally by the bot using a "User Path" stack, rendering menus immediately with zero network latency.
2. When a service executes an action (e.g., changes a config value), it automatically calls `PushMenuUpdate()` to send a fresh JSON tree to the bot in the background.
3. The next time the user navigates, the new data is already present in RAM.
4. A manual `RequestRefresh` fallback is triggered if the user explicitly presses the `🔄 refresh` button.

### Input State Machine
The system supports collecting user parameters (e.g., editing a configuration key):
1. Client defines an `Action` with an `InputPrompt`.
2. User clicks the button; Tele-Remote saves the state as `pendingInput` and asks the user the prompt question.
3. User types text; Tele-Remote intercepts the next message and sends the text as the `input` field in a gRPC command back to the microservice.
4. The service's Go callback receives the text and executes the logic.
