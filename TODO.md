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
- [x] ChatID authorization middleware and 2-step confirmation for critical emergency commands (`CmdPowerOff`, `CmdStop`)
- [x] Direct Reply Keyboard button dispatch (removed `ActionMap` leak), scoped `ConversationKey{ChatID, UserID}` state, and origin-bound `PendingInput` routing
- [x] Atomic menu JSON validation before session swap, publisher-verified `OnDisconnect`, non-blocking telemetry worker, and removal of dead persistence files
- [ ] Maintenance and continuous compliance checks
