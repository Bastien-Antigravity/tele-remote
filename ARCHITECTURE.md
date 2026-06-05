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
- **Connectivity**: Uses NATS and gRPC for bidirectional component communication.

## 4. Package Structure

- `cmd/tele-remote/`: Main entry point and bootstrap logic.
- `src/telegram/`: Telegram bot logic, routing, and dynamic UI building.
- `src/subscribers/`: Transport listeners (`ISubscriber`) that receive component data.
- `src/publishers/`: Transport senders (`IPublisher`) that dispatch commands back to components.
- `src/models/`: Shared UI types and persistence structures.
- `src/config/`: Ecosystem-aware configuration wrapper.
- `src/store/`: Persistence management for registry state.

## 5. Persistence & Reliability
- **State Recovery**: The component registry is persisted to disk. Upon restart, the `Bot` restores the dynamic menu structures and re-registers callback actions.
- **Resilient Commands**: Command dispatch uses `context.WithTimeout` (10s) and transport-level mutexes (e.g., in `GrpcPublisher`) to ensure reliability under concurrent use.
