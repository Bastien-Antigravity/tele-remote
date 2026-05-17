---
microservice: tele-remote
type: architecture
status: active
tags:
- '#service/tele-remote'
- '#type/architecture'
- '#state/active'
- '#tech/go'
- '#tier/1-gateway'
- '#zone/3-fleet'
- '#ai/ignore'
---

# 🗼 Architecture Overview: tele-remote

The **tele-remote** microservice serves as the edge gateway layer for external command propagation in the Bastien Ecosystem.

## 🔗 Context & Placement
*   **Tier**: `#tier/1-gateway` (Edge interface)
*   **Primary Technologies**: `#tech/go` (Golang Systems Engine)
*   **Central Integration**: Communicates with downstream microservices via gRPC and handles inbound web socket payloads.

## 📐 Structural Overview
```mermaid
graph TD
    User([External Operator]) -->|WebSocket/HTTP| Tele[tele-remote Gateway]
    Tele -->|gRPC| Config[config-server]
    Tele -->|SafeSocket| Log[log-server]
```
