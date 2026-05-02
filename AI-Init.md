# ⚡ AI Initialization: tele-remote

> [!IMPORTANT] MANDATORY INITIALIZATION
> Copy and paste this prompt when starting a new session in this repository:
> 
> *"1. Read the ecosystem map in **[[00-Master-MOC]]**."*
> *"2. Load project constraints from **[[AI-Project-DNA]]**."*
> *"3. Restore session state from **[[AI-Session-State]]**."*
> *"4. **Audit**: Run `git branch --show-current` and `cat VERSION.txt` to verify state matches the Session State."*
> *"5. **Spec Gate**: Before implementing any feature, you MUST read the behavioral spec in `business-bdd-brain`."*

Tele-Remote acts as a high-performance multiplexer for microservices monitoring and control. It translates heterogeneous backend signals (gRPC, NATS, SafeSocket) into a unified interactive Telegram experience.

## Architecture & Data Flow
1.  **Ingress Layer:**
    -   **gRPC (Stream):** Bidirectional streaming for high-frequency updates and instant command delivery.
    -   **NATS:** Topic-based telemetry and control for clustered services.
    -   **SafeSocket:** Custom TCP framing for lightweight/legacy connectivity.
2.  **Core Abstraction:**
    -   `interfaces.Subscriber`: Listens for incoming telemetry and registration.
    -   `interfaces.Publisher`: Routes commands back to specific clients.
3.  **UI Engine:**
    -   Dynamically renders Telegram menus based on JSON registrations from clients.
    -   Recursive menu support for complex command trees.

## Technical Stack
-   **Language:** Go 1.25+
-   **Frameworks:** `gopkg.in/telebot.v3`, `google.golang.org/grpc`
-   **Shared Libraries:**
    -   `github.com/Bastien-Antigravity/universal-logger`: Structured logging.
    -   `github.com/Bastien-Antigravity/distributed-config`: Clustered configuration.
    -   `github.com/Bastien-Antigravity/message-serializers`: Binary/JSON handling.

## Development Rules for AI Agents
-   **Documentation:** Maintain `AI-Session-State.md` to track task history.
-   **Coding Style:**
    -   Follow standard Go idiomatic patterns.
    -   Strictly use `universal-logger` for all output.
    -   Prefer `interfaces` for transport-agnostic logic.
-   **Security:** Never commit `.env` or sensitive configuration files.
-   **Git Hygiene:** Do not commit compiled binaries or local cache directories (`teleremote_cache/`).

## Contextual Knowledge Items
-   [Universal Logging](knowledge/universal_logging/artifacts/standardization.md)
-   [Microservice Toolbox](knowledge/microservice_toolbox/artifacts/connectivity_patterns.md)
