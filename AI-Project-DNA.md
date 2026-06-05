---
microservice: obsidian-brain
type: note
status: active
tags:
- '#service/obsidian-brain'
- '#type/note'
- '#state/active'
- '#zone/3-fleet'
---# 🧬 Project DNA: tele-remote

## 🎯 High-Level Intent (BDD)
- **Goal**: Remote execution and management of ecosystem components via Telegram or custom remote protocols.
- **Key Pattern**: **Remote Procedure Call (RPC) / Bot Pattern**.

## 🛠 Technical Constraints
- **Language**: Go
- **Architecture Standard**: Adheres to the ecosystem-wide standards (I-prefix interfaces).
- **Transports**: gRPC (bidirectional), NATS (pub/sub).

## 👥 Roles & Responsibilities
- **Architect**: 
    - Implement robust authentication for remote commands.
    - Ensure zero-loss state persistence and logic restoration.
- **Developer**:
    - Use I-prefix for all interfaces (`IPublisher`, `ISubscriber`).
    - Maintain thread-safety in transport layers.
