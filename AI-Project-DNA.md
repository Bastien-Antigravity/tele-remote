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

# 🧬 Project DNA: tele-remote

## Metadata
- Version: Managed via root `VERSION.txt` (`appConfig.GetVersion()`)
- Classification: Level 1 Base Fleet Microservice

## 🎯 High-Level Intent (BDD)
- **Goal**: Bidirectional remote command execution, dynamic menu projection, and telemetry broadcasting for ecosystem microservices via Telegram.
- **Key Pattern**: **Bidirectional gRPC Stream Gateway / Dynamic Reply-Keyboard Bot Pattern**.

## 🛠 Technical Constraints
- **Language**: Go
- **Architecture Standard**: Adheres to Bastien-Antigravity standards (unprefixed interfaces `Publisher` and `Subscriber`, 4-block imports, Triple-Block headers, 77-dash dividers, canonical `universal-logger` levels).
- **Transports**: gRPC bidirectional streaming (`teleremote.TeleRemoteService/Connect`, port `1863`) and Telegram Bot API (HTTPS long-polling).

## 👥 Roles & Responsibilities
- **Architect**:
  - Enforce Option A TLS/mTLS (`DynamicTransportCredentials`), `x-tele-auth` token verification, first-frame registration gating, and Telegram `AuthorizedIDs` sender validation.
  - Maintain stateless in-memory registry semantics with session-safe reconnect and stalled-stream eviction.
- **Developer**:
  - Use unprefixed nouns for interfaces (`Publisher`, `Subscriber`, `SubscriberCallbacks`) and models (`ComponentMenu`, `CommandMenu`).
  - Maintain deterministic concurrency, bounded timeouts, and `lifecycle.Manager` shutdown coordination.
