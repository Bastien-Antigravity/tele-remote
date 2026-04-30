# Testing Tele-Remote

Tele-Remote uses a mockable infrastructure to allow full unit testing without requiring a real Telegram Bot Token or an internet connection.

## 1. Running Unit Tests

To run the full suite of unit tests:

```bash
go test ./src/telegram/... -v
```

## 2. Test Suite Details

The current test suite covers the core logic of the Telegram gateway using a mock API server.

### A. Bot Initialization & Registry (`TestBot_MenuRegistration`)
- **Objective**: Ensures the bot correctly handshakes with the Telegram API and processes incoming microservice registrations.
- **Logic**: 
    1. Spins up a `httptest.Server` mimicking the Telegram `/getMe` endpoint.
    2. Initializes a `Bot` instance using the `test` profile.
    3. Simulates a component connection with a complex `Menu JSON`.
    4. **Verification**: Checks that the menu is correctly parsed into `src/models` and mapped to the internal `dynamicMenus` registry.

### B. Telemetry Broadcasting (`TestBot_Broadcast`)
- **Objective**: Verifies that telemetry data received from the ecosystem is correctly forwarded to the admin chat.
- **Logic**:
    1. Mocks the Telegram `/sendMessage` endpoint.
    2. Triggers the `Broadcast()` method with a sample message.
    3. **Verification**: Inspects the outgoing HTTP request to ensure the `chat_id` matches and the `text` payload is intact.

## 3. Mocking Infrastructure

The testing suite relies on `httptest.Server` to mimic the Telegram API.

- **Profile**: Uses the `test` profile defined in `src/telegram/test.yaml`.
- **API Redirection**: The `TelegramURL` in the config is overridden at runtime to point to the local mock server.
- **Authentication**: A dummy token (`12345:TEST_TOKEN`) is used to verify that the bot correctly constructs the API paths.

## 4. Local Development (Standalone)

For manual testing in standalone mode:

1. Ensure `TR_TOKEN` and `TR_CHATID` are set in your environment.
2. Run the service:
   ```bash
   go run ./cmd/tele-remote/main.go
   ```
3. Use the `standalone` profile to skip Config Server synchronization.
