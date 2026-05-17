---
microservice: tele-remote
type: guide
status: active
tags:
- '#service/tele-remote'
- '#type/guide'
- '#state/active'
- '#tech/go'
- '#tier/1-gateway'
- '#zone/3-fleet'
- '#ai/ignore'
---

# 🧪 Testing Playbook: tele-remote

Quality assurance and local verification playbook for the `tele-remote` edge interface.

## 🛠️ Automated Testing Suite
*   **Unit Tests**: Local Go tests verifying websocket message parsing logic.
    ```bash
    go test ./src/websocket/...
    ```
*   **Integration Tests**: Run in native sandbox environment via `sandbox-testing` repository commands.

## 📦 Manual Validation Steps
1.  Fire up the test suite in the local dev configuration profiles.
2.  Confirm that trace metrics flow seamlessly into the `log-server`.
