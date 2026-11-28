---
microservice: tele-remote
type: spec
status: active
tags:
- '#service/tele-remote'
- '#type/spec'
- '#state/active'
- '#tech/go'
- '#tier/1-gateway'
- '#zone/3-fleet'
- '#ai/ignore'
---

# 📋 Features & Behavior: tele-remote

This document summarizes the core features and behavior expectations for `tele-remote`.

## 🌟 Key Features
* **Option A gRPC TLS/mTLS & Token Gate**: Validates `x-tele-auth` metadata, Option A X.509 certificates (`local.tele_remote.tls`), and first-frame component registration.
* **Dynamic In-Memory Menu Registry**: Parses and validates bounded JSON menu trees (`<= 64 KB`, `<= 8` depth) and orders components by OpenMFE priority.
* **Strict Telegram Authorization & Rate Limiting**: Enforces per-`ConversationKey` sliding-window flood control, `AuthorizedIDs` sender validation, 2-step `CmdPowerOff` confirmation, and pending-input log redaction.
