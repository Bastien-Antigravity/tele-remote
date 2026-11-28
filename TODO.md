---
microservice: tele-remote
type: task
status: active
tags:
- '#service/tele-remote'
- '#type/task'
- '#state/active'
- '#zone/3-fleet'
---

# TODO List: tele-remote

- [x] Initial service structure and configuration
- [x] ChatID/Sender authorization middleware and 2-step confirmation for critical emergency commands (`CmdPowerOff`)
- [x] Direct Reply Keyboard button dispatch, scoped `ConversationKey{ChatID, UserID}` state, and origin-bound `PendingInput` routing
- [x] Atomic menu JSON validation before session swap, publisher-verified `OnDisconnect`, stalled-stream write eviction, and non-blocking telemetry worker with graceful shutdown drain
- [x] Option A X.509 TLS/mTLS + `x-tele-auth` enforcement and polyglot `TeleClient` path-qualified duplicate leaf label disambiguation
- [ ] Maintenance and continuous compliance checks
