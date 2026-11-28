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

Quality assurance and local verification playbook for `tele-remote`.

## 🛠️ Automated Testing Suite
* **Unit & Adversarial Tests**: Run all Go package tests with the race detector:
  ```bash
  go test -v -race ./...
  ```
* **Standalone Build Verification**:
  ```bash
  GOWORK=off go build ./...
  ```
* **Ecosystem Compliance Audit**:
  ```bash
  python3 ../obsidian-brain/08-Squad-Agent/main.py validate-compliance tele-remote
  ```
