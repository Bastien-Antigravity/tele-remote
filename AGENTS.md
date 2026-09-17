# AGENTS.md: tele-remote

## Service Mission & Architecture Role
`tele-remote` is the central remote control and telemetry bridge connecting distributed microservices and worker daemons to a Telegram bot interface. It provides bidirectional communication: workers push telemetry/alerts to Telegram, and Telegram users issue remote commands (e.g., emergency stops, configuration tweaks) with dynamic interactive UI menus.

- **Exposed Capability**: `tele_remote` (Port: `1863` gRPC streaming)
- **Protocol Contract**: gRPC streams with `IPublisher` / `ISubscriber` interface.
- **Key Dependencies**: `microservice-toolbox`, `universal-logger`, `distributed-config`
- **Configuration Link**: `standalone.yaml -> ../docker-deployment/modes/local/config/native.yaml`

## Key Build & Test Commands
```bash
# Build binary
go build -o bin/tele-remote ./cmd/tele-remote/main.go

# Run tests
go test -v ./...

# Run service
./bin/tele-remote
```

## AI Development & Integration Guidelines
1. **Dynamic UI Registration**: Client microservices send UI schemas over gRPC streams. `tele-remote` caches menus locally for instant navigation and updates asynchronously.
2. **Encrypted Bot Credentials**: The Telegram bot token (`token`) and admin chat ID (`chat_id`) are stored as `ENC(...)` tokens and decrypted at startup via `appConfig.DecryptSecret()`.
3. **Canonical Port**: Port `1863` is the authoritative gRPC port. Never hardcode legacy ports like `50051`.
4. **Header Ritual**: All Go source files MUST begin with the standard Triple-Block header (`ESSENTIAL PROCESS`, `DATA FLOW`, `KEY PARAMETERS`).
5. **Section Dividers**: Use `// -----------------------------------------------------------------------------` between exported methods and major sections.
