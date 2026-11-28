# AGENTS.md: tele-remote

## Service Mission & Architecture Role
`tele-remote` is the central remote control and telemetry bridge connecting distributed microservices and worker daemons to a Telegram bot interface. It provides bidirectional communication: workers push telemetry/alerts to Telegram, and Telegram users issue remote commands (e.g., emergency power-off, configuration tweaks) with dynamic interactive UI menus.

- **Exposed Capability**: `tele_remote` (Port: `1863` gRPC streaming)
- **Protocol Contract**: gRPC bidirectional streams (`teleremote.TeleRemoteService/Connect`) with `Publisher` / `Subscriber` interfaces (`src/interfaces/`).
- **Key Dependencies**: `microservice-toolbox`, `universal-logger`, `distributed-config`
- **Configuration Link**: `standalone.yaml -> ../docker-deployment/modes/local/config/native.yaml`

## Key Build & Test Commands
```bash
# Build binary
go build -o bin/tele-remote ./cmd/tele-remote

# Run tests with race detector
go test -v -race ./...

# Run service
./bin/tele-remote
```

## AI Development & Integration Guidelines
1. **Dynamic UI Registration**: Client microservices send UI schemas over gRPC streams using `microservice-toolbox/teleremote` (`TeleClient`). `tele-remote` validates and caches menus in memory for instant navigation and updates asynchronously.
2. **OpenMFE `ServiceSpec` & Exchange Contracts**: Publishes its `ServiceSpec` (with embedded `teleremote.proto` and Telegram Bot API contracts via `cmd/tele-remote/contracts.go`) to `web-interface` (`PublishWithLifecycle`) so `<tele-remote-inspector>` displays authoritative contracts via `/api/v1/tele-remote/contracts`.
3. **Encrypted Bot Credentials & Option A TLS**: The Telegram bot token (`token`), admin chat ID (`chat_id`), `allowed_users`, and optional `auth_token` are decrypted at startup via `appConfig.DecryptSecret()`. X.509 TLS/mTLS certificates are loaded from `local.tele_remote.tls` via `appConfig.GetLocalTLSConfig("tele_remote")`.
4. **Canonical Port & Versioning**: Port `1863` is the authoritative gRPC port. Version is resolved exclusively via `appConfig.GetVersion()` (`VERSION.txt`).
5. **Header Ritual & Formatting**: All Go source files MUST begin with the standard Triple-Block header (`ESSENTIAL PROCESS`, `DATA FLOW`, `KEY PARAMETERS`), use 4-block imports, unprefixed interface/model nouns (`Publisher`, `Subscriber`, `ComponentMenu`), and 77-dash section dividers (`// -----------------------------------------------------------------------------`).
