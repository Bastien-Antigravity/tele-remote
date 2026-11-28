---
microservice: tele-remote
type: reference
status: active
tags:
- '#service/tele-remote'
- '#type/reference'
- '#state/active'
- '#tech/go'
- '#tier/1-gateway'
- '#zone/3-fleet'
- '#ai/ignore'
---

# 📚 General Misc: tele-remote

Reference parameters and ecosystem links for `tele-remote`.

## ⚙️ Configuration Keys (`capabilities.tele_remote`)
* `ip` / `port`: gRPC stream bind address (Canonical port: `1863`).
* `token` / `chat_id` / `allowed_users`: `ENC(...)` encrypted Telegram Bot token and authorized operator IDs.
* `tls_enabled` / `tls_mode` / `auth_token`: Runtime-toggleable gRPC TLS mode (`auto`, `strict`, `disabled`) and shared `x-tele-auth` secret.
* `burst_limit`: Sliding-window request limit per 2s window (default `60`).
