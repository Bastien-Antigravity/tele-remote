# Testing Tele-Remote

Tele-Remote uses a deterministic, zero-network mock infrastructure so the entire unit and adversarial test suite runs offline without a real Telegram Bot Token or external network access.

## 1. Running Unit Tests

To run the full suite of unit tests with the race detector:

```bash
go test -v -race ./...
```

## 2. Test Suite Details

### A. gRPC Transport, mTLS & Stalled-Stream Eviction (`src/subscribers/grpc_test.go`)
- **First-Frame Registration & Identity Pinning**: Verifies telemetry frames before registration and mid-stream component identity changes are rejected with `codes.PermissionDenied` (`"⛔ Unauthorized access"`).
- **Bearer Token Authentication**: Verifies `x-tele-auth` / `authorization` metadata enforcement.
- **Option A TLS/mTLS & Live Reload**: Generates ephemeral X.509 CA, server, and client certificates (`t.TempDir()`) and tests `auto` vs `strict` mode handshakes and atomic policy hot-reload rollbacks.
- **Stalled-Stream Write Timeout & Eviction**: Verifies `GrpcPublisher.PublishCommand` unblocks immediately on context deadline expiration, invokes the stream abort hook, and fails fast with `ErrStreamAborted` on subsequent calls without wedging.

### B. Telegram Core & Rate Limiter (`src/telegram/core/bot_test.go`, `rate_limiter_test.go`)
- **In-Memory Telegram Transport**: Uses `core.SetTestRoundTripper` to intercept `/getMe` and `/sendMessage` HTTP calls in memory without opening local TCP listeners.
- **Sender Authorization**: Verifies `AuthMiddleware` allows configured `chat_id` and `allowed_users` while rejecting unauthorized group members and unknown chats.
- **Pending-Input Log Redaction & 4000-Rune Chunking**: Verifies `LoggingMiddleware` redacts secret prompt replies (`[REDACTED_PENDING_INPUT]`) and `splitTelegramMessage` splits long payloads cleanly on newline boundaries.
- **Sliding-Window Rate Limiter**: Verifies burst allowance, throttle warning cooldowns, per-`ConversationKey` isolation, and `MaxTrackedClients` (`4096`) eviction.

### C. Dynamic UI Menus & Emergency Broadcasts (`src/telegram/ui/menus_test.go`)
- **Atomic Menu Parsing & Payload Guards**: Verifies malformed, oversized (`> 64 KB`), or deeply nested (`> 8` levels) JSON updates preserve the existing valid component session.
- **OpenMFE Priority & Duplicate-Order Sorting**: Verifies deterministic component, row, and button ordering.
- **2-Step Emergency Confirmation & Scoped Prompts**: Verifies `CmdPowerOff` requires confirmation within 60s, reports partial failures accurately, and isolates concurrent users' `PendingInput` states.
