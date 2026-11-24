
Tele-Remote — Technical Audit
Interesting architecture
Production hardening needed
Critical security risks
Repository: Bastien-Antigravity/tele-remote
Audited version: develop, commit 80e4bd483b67efce7f3a7e465bbe9b1bb59e99f6, 22 November 2026.
Technology: Go 1.25, gRPC, Protocol Buffers, Telegram Bot API, microservice-toolbox.
My overall assessment: Tele-Remote is a reasonably well-structured project with good architectural foundations, but its operational security and command-delivery guarantees are not yet strong enough for a production remote-control service.
This is particularly important because it is not simply a notification service. It can send commands to your microservices, including stopping services and closing trading positions.
I reviewed the architecture, Go implementation, transport interfaces, authentication, menu management, rate limiter, tests, configuration, Dockerfile and CI configuration.
This is a static code audit. I have not executed the service or performed live network, load or penetration tests.

1. Overall scores
   Overall engineering assessment
   6.8/10
   Good foundations, but significant reliability and security work remains.
   Category
   Score
   Assessment
   Architecture
   8.5/10
   Very good
   Code organization
   8/10
   Good
   Go implementation
   7.5/10
   Good
   Performance potential
   8/10
   Good
   Actual performance evidence
   5/10
   Insufficient benchmarks
   Reliability
   6/10
   Important gaps
   Security
   4.5/10
   Major weaknesses
   Error handling
   5.5/10
   Inconsistent
   Testing
   6.5/10
   Good beginnings
   Documentation
   8/10
   Extensive
   Deployment
   6.5/10
   Some integration constraints
   Usability
   7/10
   Functional, limited UX
   These scores represent code-review judgments, not measured benchmark results.
2. What is particularly good
   There are several genuinely good engineering decisions.
   A. Transport abstraction
   The separation between:
   IPublisher
   ISubscriber
   GrpcPublisher
   GrpcSubscriber
   is one of the strongest architectural decisions.
   The Telegram interface doesn't directly depend on gRPC.
   Telegram Bot / UI
   Commands, menus, operator interactions
   Publisher / Subscriber interfaces
   Good abstraction
   Transport-independent communication contracts
   gRPC
   Implemented
   TCP
   Future option
   NATS
   Future option
   Conceptual architecture; only gRPC is implemented as the active transport.
   This is a good foundation for your broader microservice ecosystem.
   The abstraction is not excessively complicated, and the Telegram business logic is reasonably separated from the transport implementation.
   B. Concurrency and session management
   The developer has addressed several real concurrency problems:
   Mutex protection around shared state.
   Read/write locks for the component registry.
   Dedicated mutex for concurrent gRPC stream writes.
   Session identity verification during disconnect.
   Per-user conversation state.
   Asynchronous telemetry delivery.
   One particularly good improvement is OnDisconnect(clientID, pub).
   It prevents an old gRPC connection from accidentally deleting a newly reconnected session.
   The test TestOnDisconnect_StaleStreamDoesNotDeleteNewSession verifies this behavior.
   This is not superficial engineering. It demonstrates an understanding of real distributed-system lifecycle problems.
   C. Dynamic menus
   The dynamic menu system is interesting.
   A microservice can register its own menu structure, which Tele-Remote then renders inside Telegram.
   This avoids hardcoding the user interface for every service.
   For example:
   Config Server exposes configuration controls.
   Trading services expose trading controls.
   Notification services expose notification settings.
   Watchdog exposes service-management controls.
   This is a useful approach for a growing microservice fleet.
   D. Rate limiting
   The latest commit introduces a sliding-window limiter.
   It has:
   Five requests per two seconds by default.
   Per-conversation tracking.
   Warning cooldown.
   Lazy cleanup.
   The implementation is lightweight and avoids external dependencies.
   The design is appropriate for this workload, although I found some issues with how it is integrated.
3. Problems and bugs identified
   This is the most important part of the audit.
   I distinguish between confirmed implementation weaknesses and scenarios that require runtime testing.
   Finding 1 — gRPC authentication and encryption
   Critical
   Files: src/subscribers/grpc.go, cmd/tele-remote/main.go
   The gRPC server is instantiated using:
   grpcSrv := grpc.NewServer()

There is no TLS configuration, mutual TLS, or application-level client authentication in this server implementation.
The initial registration identifies a component using:
clientID := fmt.Sprintf(
    "%s:%s:%d",
    msg.ComponentName,
    msg.Host,
    msg.Port,
)

These values are supplied by the connecting client.
Consequently, a client that can reach the gRPC port can potentially impersonate a registered component or register a malicious component.
Why this matters: an unauthorized connection could potentially inject telemetry, publish malicious menus or receive operational commands.
Recommendation:
Implement mTLS or equivalent authenticated client identity.
Authorize component identities against an explicit registry.
Do not trust client-provided hostnames or component names as authentication.
Validate registration before accepting telemetry or commands.
If the service is strictly bound to loopback, exposure is reduced, but this does not solve the problem for distributed deployments.
Finding 2 — Emergency broadcast reports success without confirming delivery
Critical
File: src/telegram/ui/menus.go
The emergency broadcast implementation ignores publishing errors:
for _, pub := range pubs {
    _ = pub.PublishCommand(
        dispatchCtx, cmdType, "", "",
    )
}

It then sends:
Powering off all components...
This creates a serious discrepancy between what the operator sees and what actually happened.
Imagine five connected services:
Service A receives the command.
Service B receives the command.
Service C disconnects.
Service D returns an error.
Service E never receives the command.
The Telegram interface still reports the operation as initiated without providing the delivery results.
There is also no execution acknowledgement protocol.
Recommendation: implement command tracking:
COMMAND_CREATED
     |
COMMAND_DISPATCHED
     |
COMMAND_RECEIVED
     |
COMMAND_EXECUTING
     |
     +---- SUCCESS
     |
     +---- FAILED
     |
     +---- TIMEOUT / UNKNOWN
Each command should have a unique ID, target, timestamp and execution status.
This is particularly important for trading operations. Sending a command is not equivalent to successfully closing positions.
Finding 3 — Telegram authorization has a group-chat weakness
High
File: src/telegram/core/bot.go
The current authorization logic accepts a request if either the sender ID or the chat ID appears in the authorized ID set.
Conceptually:
if authorized(senderID) {
    return true
}

if authorized(chatID) {
    return true
}

This is problematic when the bot is configured to authorize a Telegram group.
If the group chat ID is authorized, every participant in that group can potentially use the bot, even if their individual Telegram user IDs are not authorized.
This is especially dangerous for emergency operations.
Recommendation: separate three concepts:
Authorized private users.
Authorized group chats.
Authorized users within each group.
For critical operations, require an explicitly authorized individual sender, regardless of group membership.
Finding 4 — Stale menu and pending-input state
High
File: src/telegram/ui/menus.go
The system replaces component menus when services reconnect or update their registrations.
However, pending input and navigation state are not consistently invalidated when the same client ID registers a new menu.
Example:
An operator opens a configuration menu.
The operator selects an input action.
The component reconnects and changes its menu.
The operator submits the old input.
The old action can be sent to the newly registered publisher.
Another problem is getMenuByPath().
When a navigation path becomes invalid, it returns the last valid parent menu instead of explicitly rejecting the stale path.
This can cause an old keyboard to interact with a different menu structure.
Recommendation:
Add MenuRevision to every component registration.
Bind pending inputs to the session generation and menu revision.
Invalidate pending actions when a component reconnects or changes its menu.
Reject invalid navigation paths instead of silently falling back.
Finding 5 — Context timeout does not necessarily cancel gRPC sending
High
File: src/publishers/grpc.go
The publisher checks the context before acquiring the mutex and immediately before sending.
However, the actual operation is:
return p.stream.Send(cmd)

The supplied context is not passed into Send().
Consequently, the ten-second timeout used by the UI does not guarantee that a blocked stream write will stop after ten seconds.
This matters because emergency broadcasts are sequential.
A problematic publisher could delay the remaining recipients.
Recommendation:
Associate each publisher with a cancellable stream lifecycle.
Make stream shutdown reliably interrupt pending writes.
Avoid sequential blocking dispatch for fleet-wide operations.
Use bounded parallel dispatch with per-target results.
Finding 6 — gRPC shutdown can hang
High
File: src/subscribers/grpc.go
The shutdown implementation uses:
s.grpcSrv.GracefulStop()

GracefulStop waits for active RPCs to finish.
Because Tele-Remote uses long-lived bidirectional streams, a client that does not disconnect can prevent shutdown from completing.
There is no bounded fallback to Stop().
Recommendation: implement graceful shutdown with a deadline, followed by forced shutdown if active streams remain.
Finding 7 — Telemetry can be silently lost
High
File: src/telegram/core/bot.go
The telemetry channel has a capacity of 256 messages.
When full, messages are discarded:
select {
case bot.telemetryChan <- msg:
default:
    bot.Log.Warning(
        "Telemetry queue full; dropping message",
    )
}

This is a valid performance-oriented design for best-effort telemetry.
But it is unsuitable if the service is expected to guarantee delivery of operational alerts.
There is also only one worker, which sends messages sequentially.
A slow Telegram API response can therefore delay the entire queue.
Recommendation:
Separate disposable telemetry from important alerts.
Add message priority.
Add delivery/retry handling for critical alerts.
Measure queue depth and message drops.
4. Additional problems
These are less immediately critical but worth addressing.
Problem
Impact
Priority
Unauthorized messages are logged before authentication
Privacy and unnecessary log exposure
High
Raw user text and callback data are logged
Potentially exposes sensitive configuration
High
No command execution acknowledgement
Uncertain operational state
High
No stale-menu revision checks
Incorrect action dispatch
High
No explicit menu depth or payload limits
Resource exhaustion risk
High
Component identity based on client-provided fields
Session spoofing/collision risk
High
Telegram message length not handled
Long telemetry messages can fail
Medium
No retry strategy for Telegram failures
Lost notifications
Medium
Component ordering uses Go map iteration
Unstable menu ordering
Low
No connection-readiness indicator
Operator may see incomplete fleet state
Medium
No input-type validation at the gateway
Invalid configuration input can be forwarded
Medium
An important middleware ordering issue
In routes.go:
bot.B.Use(
    bot.RateLimitMiddleware(),
    bot.LoggingMiddleware(),
    bot.AuthMiddleware(),
)

This means logging happens before authorization.
Unauthorized users can therefore generate logged messages and callback data.
I would change the order so authentication precedes normal application logging, while retaining a separate, deliberately redacted security audit log.
The rate limiter also runs before authentication. Its map is keyed by user/chat identity and can accumulate entries for unique incoming identities until periodic cleanup. A global protection layer and a bounded state strategy would improve resilience.
5. Performance assessment
The architecture has good performance potential.
Go, gRPC bidirectional streams, Protocol Buffers and in-memory registries are appropriate technologies for this workload.
However, there is an important distinction between performance potential and demonstrated performance.
Component
Assessment
gRPC transport
Very good choice
Protobuf serialization
Efficient
Publisher mutex
Correctly serializes stream writes
In-memory menu registry
Efficient
Menu navigation
No network round trip required
Telemetry queue
Lightweight, but lossy
Rate limiter
Low overhead for normal traffic
Emergency broadcast
Sequential bottleneck
Telegram delivery
External bottleneck
Large fleet behavior
Not benchmarked
My recommendation: do not optimize the current implementation prematurely.
The biggest performance improvement would come from improving dispatch concurrency and isolating slow Telegram delivery from the rest of the system.
I would benchmark:
10, 50, 100 and 500 connected components.
10, 50 and 100 concurrent Telegram users.
100, 1,000 and 10,000 telemetry messages per minute.
Simultaneous component reconnections.
Slow and disconnected gRPC clients.
Telegram API throttling.
Measure p50, p95 and p99 command-dispatch latency, queue depth, dropped messages and memory consumption.
6. Testing quality
The project has a reasonable testing foundation.
Existing tests cover:
Bot initialization.
Telegram API mocking.
Telemetry broadcasting.
Authorization.
Rate limiting.
Menu registration.
Invalid JSON preservation.
Reconnection and stale disconnect protection.
Emergency confirmation.
Concurrent users with independent pending inputs.
I particularly appreciate the tests for reconnection and concurrent conversations.
However, there are significant gaps.
Missing test
Importance
Unauthorized user in authorized group
Critical
gRPC authentication and spoofed registration
Critical
Blocked publisher during emergency broadcast
Critical
Partial emergency broadcast failure
Critical
Reconnection with pending input
Critical
Stale menu revision
Critical
Command acknowledgement timeout
Critical
Graceful shutdown with active streams
Critical
Telegram message exceeding size limit
Medium
Telemetry queue saturation
High
Race detector under concurrent operations
High
Large menu / deeply nested JSON
High
I would introduce these mandatory CI checks:
go test ./...
go test -race ./...
go vet ./...
go test -count=50 ./src/...

There is also an important CI limitation: the repository delegates its main CI workflow to fleet-operation-brain. The actual reusable workflow is external, so its complete validation behavior cannot be established from this repository alone.
I have not independently verified that the latest commit passes these commands.
7. Deployment and maintainability
There is one integration issue worth highlighting.
The go.mod contains local replacements:
replace (
    github.com/Bastien-Antigravity/distributed-config => ../distributed-config
    github.com/Bastien-Antigravity/flexible-logger => ../flexible-logger
    github.com/Bastien-Antigravity/microservice-toolbox => ../microservice-toolbox
    github.com/Bastien-Antigravity/safe-socket => ../safe-socket
    github.com/Bastien-Antigravity/universal-logger => ../universal-logger
)

This means the repository is not independently buildable from an arbitrary directory without its sibling repositories.
The Dockerfile addresses this by cloning those repositories into a shared workspace.
That works as an ecosystem-oriented deployment strategy, but there are drawbacks:
Dependencies are cloned from mutable develop branches.
A change in another repository can unexpectedly affect this build.
Reproducing an old deployment becomes more difficult.
The versions declared in go.mod do not fully describe the actual source dependencies.
Recommendation: use pinned commits or immutable releases for production builds, and provide a reproducible workspace development setup.
There is also a minor startup reliability problem in main.go: the gRPC listener starts in a goroutine, and listener errors are logged without terminating or marking the service unhealthy.
The application can therefore report that it is ready even if the gRPC port failed to bind.
That should be corrected.
8. Usability assessment
The Telegram interface is functional, but it is more of an operational prototype than a mature fleet-management interface.
TeleWoW - Telegram bot to monitor and control your WoW MoP server from your phone | EmuCoach | WoW Emulation Resources | Cataclysm, MOP, Wotlk, TBC
GitHub - ErROR97/v2ray_x-ui_telegramBot · GitHub
Putting a SaaS admin panel entirely inside a Telegram bot - DEV Community
Good points
Dynamic service menus.
Main menu navigation.
Back navigation.
Refresh mechanism.
Text input prompts.
Two-step emergency confirmations.
Independent conversation state.
Areas for improvement

1. No fleet overview
   The operator needs to know:
   Which services are connected.
   Which services are disconnected.
   Which services are restarting.
   Which services are unhealthy.
   When each service last communicated.
2. No command execution feedback
   A message saying that a command was sent is insufficient.
   The operator should see its actual execution status.
3. Large menu scalability
   As the number of microservices increases, the main menu becomes increasingly cluttered.
   Pagination, service categories and search would improve usability.
4. Input validation
   The gateway forwards arbitrary text to components.
   The individual microservice must validate the actual value, but the gateway could provide basic validation metadata, input types and error feedback.
5. Emergency operations
   I would distinguish clearly between:
   Stop one service.
   Stop a service group.
   Stop the entire fleet.
   Close positions.
   Disable trading.
   These should not be treated as interchangeable broadcast operations.
6. Recommended remediation roadmap
   I would avoid rewriting this project. Its architecture is worth keeping.
   The following sequence would provide the greatest improvement for the least development effort.
   Phase 1 — Security
   Must do first
   Estimated effort: 2–4 days.

Implement authenticated gRPC connections.

Introduce explicit component identity validation.

Separate authorized users from authorized groups.

Fix middleware ordering.

Redact sensitive user input from logs.

Validate incoming registration payloads.
Phase 2 — Command reliability
Must do
Estimated effort: 3–5 days.

Add unique command IDs.

Introduce acknowledgements.

Track execution status.

Implement bounded parallel dispatch.

Handle partial broadcast failures.

Add idempotency for critical operations.
Phase 3 — State consistency
High priority
Estimated effort: 1–3 days.

Introduce menu revisions.

Bind pending inputs to session generations.

Invalidate stale actions.

Add menu size/depth limits.

Improve registration and reconnection cleanup.
Phase 4 — Operational reliability
High priority
Estimated effort: 2–4 days.

Bounded graceful shutdown.

Startup health checks.

Telemetry delivery priorities.

Retry strategy for critical notifications.

Queue and connection monitoring.

Automated race and integration tests.
Phase 5 — Usability
Later
Estimated effort: 2–3 days.

Fleet status overview.

Component grouping.

Better emergency command interface.

Execution history.

Pagination and search.
Effort estimates are approximate engineering estimates, not measured implementation times. Some work can be performed in parallel.
10. Final verdict: developer and project quality
There is something interesting about this repository when considering the broader Bastien-Antigravity ecosystem.
The developer is demonstrating several capabilities:
Competency
Evidence
Go development
Good
Distributed systems
Good
Concurrency management
Good
Software architecture
Very good
Transport abstraction
Very good
Protocol design
Good
Testing methodology
Good foundations
Security engineering
Needs improvement
Production reliability
Needs improvement
Operational UX
Intermediate
The most interesting aspect is that the developer is not simply writing a Telegram bot.
He is attempting to build a reusable control-plane component for an interconnected microservice architecture.
That is a more substantial engineering challenge.
However, the project also illustrates a recurring distinction: implementing architectural mechanisms is not the same as guaranteeing their operational properties.
For example:
Having a timeout does not guarantee cancellation.
Having authentication middleware does not guarantee correct authorization boundaries.
Having a publisher does not guarantee command execution.
Having a bounded queue does not guarantee message delivery.
Having graceful shutdown code does not guarantee bounded shutdown time.
These are precisely the areas I would want to see improved before considering the project production-ready.
11. My assessment for your candidate portfolio
Candidate assessment — Tele-Remote
Worth keeping
7.5/10
Assessment of demonstrated engineering potential, distinct from production readiness.
Strengths
System-level thinking.
Good abstraction and modularity.
Understanding of asynchronous communication.
Awareness of concurrency and lifecycle problems.
Ability to build interconnected services.
Development areas
Security threat modeling.
Distributed command-delivery guarantees.
Failure handling.
Production observability.
Integration and stress testing.
Suitable positions
Backend Go Developer.
Distributed Systems Developer.
Microservices Engineer.
Backend / Infrastructure Engineer.
Junior-to-mid-level Platform Engineer, depending on demonstrated independent production experience.
I would not classify this as senior-level production engineering evidence yet. The architectural ambition is good, but the missing operational guarantees are important.
12. One broader observation
I would pay particular attention to the consistency between this repository and the other projects in the ecosystem:
microservice-toolbox
safe-socket
distributed-config
universal-logger
notif-server
log-server
Tele-Remote is a particularly useful integration test of the entire architecture.
If these services can communicate reliably, reconnect safely, authenticate one another and maintain consistent command contracts, that would be stronger evidence of engineering maturity than any individual repository's architecture document.
My recommendation: keep the current architecture, fix security and command reliability first, and then perform a cross-repository integration audit.
The next most valuable exercise would be to audit the complete command lifecycle across Tele-Remote and its connected microservices, especially safe-socket, microservice-toolbox and the services that actually execute emergency commands. That would establish whether a command can be securely delivered, acknowledged, executed exactly once where required, and correctly reported to the operator.
