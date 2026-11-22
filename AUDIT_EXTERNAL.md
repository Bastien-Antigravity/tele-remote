Tele-Remote — architecture and reliability audit
Repository: Bastien-Antigravity/tele-remote
Active repository
Go
92 commits
Develop branch · 21 November 2026

This project is particularly interesting because it has a different responsibility from notif-server.
- notif-server sends notifications from microservices to external platforms.
- tele-remote provides a two-way control interface between your microservice fleet and Telegram.
In other words, Tele-Remote is closer to a centralized remote operations console than a notification service.

It is potentially one of the most sensitive services in the ecosystem because it can transmit operational commands, including powering down services and closing trading positions.

My priority would therefore be security, command execution guarantees, client isolation and consistency of the interactive interface.
1. What Tele-Remote actually does
The repository describes two principal communication directions:
Telegram Bot

Human operator · commands · interactive menus

Two-way communication

Tele-Remote

Central gRPC server · Telegram integration · routing · UI cache
Command routing

Broadcast / targeted commands


Telemetry

Logs / alerts / status





Trading services

gRPC stream client


Config server

gRPC stream client


Log server

gRPC stream client


Other microservices

gRPC stream client




The architecture uses a transport-independent IPublisher / ISubscriber abstraction, with gRPC bidirectional streaming as the documented transport.
Bastien-Antigravity/tele-remote · GitHub


This is a good architectural decision: Telegram should not need to know whether a connected microservice communicates through gRPC, TCP or another transport.
2. Understanding the internal components
The architecture document identifies these principal packages:
Package	Responsibility
cmd/tele-remote	Bootstrap and service initialization
src/telegram/core	Bot state machine and message dispatch
src/telegram/routes	Telegram commands such as /start
src/telegram/ui	Dynamic menu parsing and keyboard rendering
src/subscribers	Receiving messages from microservices
src/publishers	Sending commands and refresh requests
src/models	Shared UI and data structures
src/config	Configuration integration
src/store	Registry state management


GitHub+1
There are three particularly important concepts here.
A. Publisher / Subscriber abstraction
This is the heart of the transport architecture.

Think of it this way:
- A subscriber receives information from a microservice.
- A publisher sends information to a microservice.
- Telegram communicates with these abstractions without needing to know the underlying network protocol.
For example:
Telegram button click
       |
       v
    Telegram
       |
       v
  Command router
       |
       v
    Publisher
       |
       v
  gRPC stream
       |
       v
  Trading service
       |
       v
  Execute action


The reverse direction handles telemetry and menu updates.

My recommendation: retain these interfaces, but make sure they represent actual business responsibilities rather than merely wrapping gRPC methods.

I would also define a consistent message envelope:
type RemoteMessage struct {
    ID          string
    Source      string
    Target      string
    Type        MessageType
    Payload     []byte
    Timestamp   time.Time
    Correlation string
}


The important fields are ID, Target and Correlation.

They make it possible to track a command from the Telegram button click to the microservice response.
B. Dynamic menu registration
This is arguably one of the most interesting features.

A connected microservice provides a JSON menu definition. Tele-Remote converts that definition into Telegram keyboards.

For example, a trading service could expose:
Trading Engine
    |
    +-- Status
    |
    +-- Strategies
    |      |
    |      +-- Enable
    |      +-- Disable
    |
    +-- Configuration
    |      |
    |      +-- Risk limit
    |      +-- Trading pair
    |
    +-- Emergency stop


Tele-Remote doesn't need to know the internal implementation of each trading service.

It only needs to understand the menu definition and the action identifier.

This is a good form of extensibility: individual microservices can expose their own control interfaces without modifying the central Telegram bot.

However, there is a crucial distinction:

A menu is a user interface description, not an authorization mechanism.

A microservice must validate every received action independently.
C. Optimistic UI and background synchronization
The project uses a local in-memory menu cache.

The intended process is:
1. A microservice registers its menu.
2. Tele-Remote stores the menu in memory.
3. The user navigates locally.
4. A service can push an updated menu.
5. The user can request a manual refresh.
This is an excellent way to avoid a network round trip for every navigation operation.

However, I would investigate consistency carefully.

Imagine:
- A user opens the trading configuration menu.
- The trading service changes its available actions.
- The service pushes a new menu.
- The user clicks an old button.
What happens?

The old button should never execute an action that is no longer valid.

I would introduce a MenuVersion or revision number and associate it with each rendered keyboard.

When a button is clicked, Tele-Remote should verify that its menu revision is still valid. If not, it should refresh the interface instead of executing a stale action.

GitHub+1
3. The most important reliability and security risks
Here I would be considerably stricter than with notif-server.

Tele-Remote is capable of triggering actions that affect the operational state of the entire fleet.

Priority 1
 A. Command authorization and dangerous operations

The README describes two particularly sensitive commands:
- Power off all connected components.
- Close all trading positions.
These operations should not be treated as ordinary Telegram button callbacks.

I would introduce three command categories:
Category	Example	Policy
Read-only	Status, logs, configuration display	Authorized user
Operational	Change configuration, restart service	Authorization + validation
Critical	Power off, close positions	Strong authorization + explicit confirmation

For critical commands, I recommend a two-step interaction:
Example: Close all positions
EMERGENCY OPERATION

Close all positions across connected trading services?

Confirm


Cancel


Illustrative interface, not an existing repository screenshot.

I would additionally require:
- Explicit Telegram user-ID allowlisting.
- Chat-ID validation.
- No authorization based solely on possession of a callback button.
- Audit logs containing operator, target, command and timestamp.
- Protection against replayed callbacks.
- Server-side validation of every critical operation.
One further recommendation: a global emergency command should have a different permission level from ordinary component management.
B. Broadcast versus targeted command delivery
The documentation describes broadcasting commands to all connected components.

That is convenient, but potentially dangerous.

For example, POWER_OFF may be appropriate for every service, whereas CLOSE_ALL_POSITIONS should probably only reach services that manage positions.

I would introduce explicit command targeting:
Command
  |
  +-- Broadcast to all
  |
  +-- Broadcast to a group
  |
  +-- Target a specific service


The command should carry:
type CommandTarget struct {
    Component string
    Group     string
    Broadcast bool
}


The actual implementation should also define what happens when some targets are disconnected.

A broadcast is not successful simply because the server successfully wrote the command to its gRPC stream.

It should report which services acknowledged execution, which rejected the operation and which were unreachable.

For emergency actions, this distinction is essential.
C. gRPC stream lifecycle and reconnection
The project relies on bidirectional gRPC streams.

This is a good choice, but it creates several important lifecycle problems:
- A client disconnects unexpectedly.
- A client reconnects while the previous session is still registered.
- A network interruption occurs during command delivery.
- The server writes to a stream while the client is shutting down.
- Two sessions temporarily exist for the same component.
The architecture document mentions session guards to clean up old sessions after reconnection.

I would make session ownership explicit.

Each registration should have:
Component ID
Session ID
Connection generation
Connected timestamp
Last heartbeat
Connection state


The server should only allow the current session generation to publish or receive commands.

This prevents an older connection from accidentally remaining active after a newer one has registered.

Important: gRPC stream delivery is not the same thing as command execution acknowledgement.

For reliable remote control, I would use:
COMMAND_CREATED
       |
       v
COMMAND_SENT
       |
       v
COMMAND_RECEIVED
       |
       v
COMMAND_EXECUTING
       |
       +----> COMMAND_SUCCEEDED
       |
       +----> COMMAND_FAILED


A timeout should produce UNKNOWN or TIMED_OUT, not automatically FAILED: the operation may have completed even though the response was lost.

That is particularly important for financial operations.

Priority 1
 D. Pending input state and concurrent Telegram users

The architecture describes a pendingInput mechanism:
1. A user selects an action requiring text.
2. Tele-Remote saves the pending input state.
3. Telegram asks the user for a value.
4. The next message is forwarded to the corresponding microservice.
This is simple, but the implementation needs to handle concurrent conversations correctly.

Imagine two users:
- User A selects an IP configuration field.
- User B selects a risk-limit configuration field.
- User A submits a value.
- User B submits a value.
If pending input is stored globally rather than by conversation, values can be routed to the wrong service.

I would use a session key:
type ConversationKey struct {
    ChatID int64
    UserID int64
}

type PendingInput struct {
    ComponentID string
    ActionID    string
    PromptID    string
    ExpiresAt   time.Time
}


And maintain:
map[ConversationKey]PendingInput


The input state should also have:
- An expiration time.
- Explicit cancellation.
- Validation of the expected input type.
- Protection against simultaneous pending operations.
- Cleanup after successful completion.
- A correlation ID linking the response to the original request.
This is one of the first implementation details I would inspect in the actual source code.
E. In-memory registry and restart behaviour
The architecture explicitly states that the old registry_state.json persistence was removed.

The menu registry is reconstructed from live gRPC streams after restarting Tele-Remote.

This is a reasonable choice because the menu definitions belong to the microservices, not to the Telegram gateway.

However, it introduces a temporary availability problem.
Restart scenario
1. Tele-Remote starts
   No registered menus
2. Microservices reconnect

   Each service registers its component and menu.
3. Registry rebuilt
   Menus available

My recommendation is not necessarily to restore full menu persistence.

Instead, implement a connection-readiness mechanism:
- Track expected services.
- Track currently connected services.
- Expose a synchronization status.
- Display unavailable components clearly in Telegram.
- Trigger a registration refresh after reconnecting.
There is little reason to persist the entire UI if every component can reconstruct it. But the bot should make its incomplete state visible.
4. Telegram UI: where simplification could help
I particularly like the idea of dynamically generated menus.

However, Telegram's interface has limitations:
- Inline keyboard callback data has a strict size limit.
- Messages have editing and rate limits.
- Telegram requests can fail independently of gRPC.
- A callback can arrive after the underlying menu has changed.
- Text input and callback interactions can overlap.
I would therefore avoid encoding complete action definitions into callback data.

Instead, use a small opaque identifier:
callback_data = "act:7f3a..."


The server resolves this identifier against its internal action registry.

The action registry should include:
Action ID
Component ID
Action ID/version
Menu revision
Authorized user
Expiration


This provides better security and allows stale callbacks to be rejected.

One other simplification: keep Telegram-specific keyboard rendering inside src/telegram/ui. The core should work with generic menu definitions and generic action requests, not Telegram keyboard structures.

5. Shared infrastructure and integration
Tele-Remote depends on several of the other ecosystem projects.
Dependency relationships
microservice-toolbox

Bootstrap · lifecycle · service registration


distributed-config

Configuration · secrets · network settings


tele-remote

Central control plane


universal-logger

Logging


gRPC / Protobuf

Communication




The repository's go.mod reveals an important development consideration.

It uses local replacements for distributed-config and flexible-logger, while microservice-toolbox and universal-logger are declared as versioned dependencies.
GitHub


I would investigate whether this dependency arrangement is intentional.

In particular:
- Why is flexible-logger still needed if universal-logger is the main logging abstraction?
- Are the local replacements necessary for production builds?
- Are the protobuf contracts versioned independently?
- Can a client built against an older protocol still connect safely?
The most important improvement would be to version the remote-control protocol independently of the implementation repositories.

This prevents an update to a shared library from unexpectedly breaking communication with older microservices.
6. Testing: what is currently covered and what is missing
The repository has a TESTING.md describing mock-based tests using httptest.Server.

Two tests are specifically documented:
Test	Purpose
TestBot_MenuRegistration	Verify registration and dynamic menu parsing
TestBot_Broadcast	Verify forwarding telemetry to Telegram

The documented test command is:
go test ./src/telegram/... -v



GitHub
This is a useful foundation, but it is not sufficient for a remote-control service.

I would add the following test groups:
Test	Importance
Unauthorized Telegram user	Critical
Unauthorized chat	Critical
Concurrent users with pending inputs	Critical
Replayed callback	Critical
Stale menu revision	Critical
Broadcast with disconnected services	Critical
gRPC reconnect and duplicate registration	Critical
Command acknowledgement timeout	Critical
Concurrent stream writes	High
Telegram API rate limiting	High
Tele-Remote restart and registry reconstruction	High
Malformed menu JSON	High
Oversized input and callback payload	High
Graceful shutdown with active streams	High

I would also run:
go test -race ./...
go vet ./...
go test -count=100 ./src/...


The race detector is particularly important because this application combines:
- Telegram event handlers.
- gRPC streams.
- Shared component registries.
- Dynamic menus.
- Pending input states.
- Background synchronization.
7. A significant documentation inconsistency
I found something worth investigating before making architectural changes.

The repository's AI-Project-DNA.md mentions:
- gRPC bidirectional streaming.
- NATS pub/sub.
- Zero-loss state persistence and restoration.
However, the current ARCHITECTURE.md describes:
- gRPC as the active transport.
- In-memory menu state.
- No menu persistence after restart.
The TODO file is also extremely short, listing only initial structure and maintenance/compliance tasks.
GitHub+2


This does not necessarily mean the implementation is wrong. It could simply mean the project documentation has not evolved consistently.

But I would clarify three things:
1. Is NATS an actual planned transport or an abandoned idea?
2. Is zero-loss persistence a requirement or has the project deliberately moved to stateless reconstruction?
3. Is the registry responsible for only menu state, or does it also maintain operational session state?
I would consolidate these decisions into ARCHITECTURE.md and make the other project documents refer to it.

8. My proposed refactoring roadmap
I would organize the improvements into the following phases.
Tele-Remote refactoring roadmap
1. Security and authorization
   Critical
   User allowlisting, command authorization, dangerous-operation confirmation, callback validation and audit trails.
2. Command execution protocol
   Critical
   Command IDs, acknowledgements, execution status, timeouts, correlation and idempotency.
3. Session and registry management
   Critical
   Explicit session ownership, reconnection generations, stale-session removal and concurrent access protection.
4. Telegram interaction state
   Critical
   Per-user pending inputs, expiration, callback validation and menu revision tracking.
5. Transport abstraction
   High
   Clear separation of publishers, subscribers and the gRPC implementation. Protocol versioning.
6. Restart and recovery
   High
   Registry reconstruction, readiness monitoring and recovery of commands whose execution status is uncertain.
7. Observability
   High
   Connected services, stream health, command latency, acknowledgements and failed operations.
8. Testing and simplification
   Medium
   Race testing, failure injection, removal of redundant abstractions and documentation consolidation.

9. What I would change architecturally
I would aim for a slightly simpler separation of responsibilities.
Proposed internal architecture
Telegram Adapter

Commands · callbacks · text input · keyboards


Application Core

Command handling · authorization · orchestration

Session Manager

Clients · registry · menus


Command Manager

Routing · acknowledgement · state


UI Manager

Menu versions · input sessions


Authorization

Permissions · audit




Transport Interfaces

IPublisher · ISubscriber


gRPC


Future TCP / NATS




The advantage is that Telegram becomes just one interface to the application.

You could eventually add a REST dashboard or a web administration interface without rewriting command routing, authorization or session management.

This also fits the broader microservice architecture: transport implementation should be replaceable without rewriting the business logic.

10. Overall assessment
Tele-Remote
Interesting architectural foundation
Strong points
- Dynamic menus provided by individual microservices.
- Bidirectional communication.
- Separation between Telegram and transport.
- Optimistic UI with background synchronization.
- Centralized control interface for the ecosystem.
- Reconnection and session lifecycle are recognized architectural concerns.

Main concerns
- Security of critical commands.
- No clearly documented execution acknowledgement protocol.
- Potential concurrency issues with pending inputs.
- Stale menu actions.
- Session lifecycle and duplicate registrations.
- Unclear persistence and transport requirements.
- Insufficient documented failure-oriented testing.

My recommendation

Keep the general architecture. Concentrate on making remote commands deterministic, secure and traceable. Avoid adding unnecessary infrastructure or a large event-bus abstraction.

Important scope limitation: I could inspect the public README, architecture, testing guide, project DNA, TODO and dependency manifest, but the GitHub source-tree endpoints were not accessible through the current retrieval session. Therefore, the risks above are implementation questions and architectural findings, not a claim that every suspected bug has been confirmed in Go source code.
Bastien-Antigravity/tele-remote · GitHub+3

11. What I recommend investigating first
For a proper source-level review, I would follow this exact order:
Order	Source area	What to establish
1	src/models/	Actual message, action and menu contracts
2	src/publishers/	Command routing and gRPC stream writes
3	src/subscribers/	Registration, telemetry and reconnection
4	src/telegram/core/	Main state machine and concurrency
5	src/telegram/routes/	Telegram authorization and handlers
6	src/telegram/ui/	Dynamic menus and callback resolution
7	src/store/	Registry and persistence responsibilities
8	cmd/tele-remote/	Bootstrap and graceful shutdown

I would pay particular attention to src/telegram/core/, because it is likely where several of these responsibilities converge.

One final architectural point: I would keep notif-server and tele-remote independent. The former should handle outbound notification delivery; the latter should handle interactive remote operations. They can share logging, configuration and protocol conventions, but combining their business logic would create unnecessary coupling.

My next step would be a file-by-file source audit of tele-remote, starting with src/models, src/publishers and src/subscribers, including concrete replacement Go code for any problematic implementations.



12. Source-level investigation: establishing the actual execution path
I have confirmed the current develop branch still documents the same fundamental architecture, and the source repository exposes 92 commits. The most useful new detail is that ARCHITECTURE.md explicitly documents a 10-second command timeout and transport-level mutexes.
Bastien-Antigravity/tele-remote · GitHub+1


That gives us a concrete starting point: we need to understand whether those mutexes protect the correct resources, and whether the timeout covers the entire command lifecycle or only the transport write.
13. A concrete finding in the current dependency configuration
I have now checked the actual go.mod, rather than relying on the architecture documentation.
Confirmed configuration issue
The repository contains:
replace (
    github.com/Bastien-Antigravity/distributed-config => ../distributed-config
    github.com/Bastien-Antigravity/flexible-logger => ../flexible-logger
    github.com/Bastien-Antigravity/microservice-toolbox => ../microservice-toolbox
    github.com/Bastien-Antigravity/safe-socket => ../safe-socket
    github.com/Bastien-Antigravity/universal-logger => ../universal-logger
)


All five dependencies are redirected to sibling directories.
GitHub


This is fine for a coordinated local workspace, but it creates a reproducibility problem.

Even though the module versions are declared, Go will use the local directories instead.

I recommend:
- Keep a workspace configuration for local ecosystem development.
- Remove these replacements from the production module.
- Pin dependency versions.
- Test standalone builds in CI.
There is also an interesting dependency detail: safe-socket, Cap'n Proto and mmap libraries are indirect dependencies. I would investigate whether these are genuinely needed by Tele-Remote or simply inherited through the shared ecosystem libraries.
14. What I need to finish the actual code audit
I could retrieve the repository documentation and go.mod, but GitHub is currently refusing access to the source directory listings. I don't want to invent method names, claim concurrency bugs without seeing the code, or give you replacement implementations based on assumptions.

Please upload the tele-remote repository as a ZIP archive.

With the source available, I can continue directly with:
1. src/models/: exact data structures and protocol contracts.
2. src/publishers/: gRPC sending, mutexes, timeouts and command delivery.
3. src/subscribers/: registration, stream lifecycle and reconnection.
4. src/telegram/core/: state management and concurrency.
5. src/telegram/ui/: menu parsing, callback handling and pending inputs.
I will then produce a much more useful result than another architectural overview: a code-level audit with exact filenames, methods, identified problems, severity, proposed simplifications and corrected Go implementations.

I would start with publishers and subscribers, because their contracts determine whether the rest of Tele-Remote can operate reliably.


The ZIP is accessible. I can now move beyond the architectural documentation and inspect the actual Go implementation.

The archive contains the complete main execution path, including:
- Publisher and subscriber implementations.
- Shared interfaces and models.
- Telegram bot core, routes and UI.
- gRPC protocol definitions.
- Persistence implementation.
- Bootstrap and configuration.
- Existing unit tests.
This is enough to perform the source-level audit we discussed.

I will distinguish confirmed code issues from design improvements, and avoid treating a theoretical risk as an existing bug.
Tele-Remote — detailed source-code audit
Source inspected
Refactoring recommended
Archive dated 20–21 November 2026

I have now followed the actual execution path through the code. There are several findings that change my initial assessment.

The most important conclusion is that the overall architecture is reasonable, but the implementation currently contains some unnecessary complexity and several genuine reliability and security problems.

I would not rewrite this repository. I would simplify it.
1. Actual execution flow
This is the execution path implemented in the uploaded source.
Telegram Bot API

Messages · Reply Keyboard · commands


telegram/routes + telegram/ui

Input handling · navigation · action resolution


telegram/core.Bot

DynamicMenus · ActionMap · Publishers · UserStates


GrpcPublisher

Serialized stream.Send()


GrpcSubscriber.Connect

Bidirectional stream per microservice


Microservice commands

Receive BotCommand


Telemetry

Forward to Telegram




The code responsible for these operations is:
Operation	Actual implementation
Connection acceptance	src/subscribers/grpc.go
Registration	src/telegram/ui/menus.go
Menu parsing	parseMenuRow() / parseButton()
Action resolution	HandleDynamicText()
Command dispatch	createCommandAction()
Stream transmission	src/publishers/grpc.go
Telemetry forwarding	Bot.OnTelemetry()
Session cleanup	Bot.OnDisconnect()

This is a relatively straightforward architecture. The problems arise mainly from how state is managed between these components.

2. Critical findings — confirmed in the source
Critical
 Finding 1: There is no effective Telegram user authorization

File: src/telegram/routes/routes.go

The bot registers these handlers:
bot.B.Handle("/start", ...)
bot.B.Handle(&btnPowerOff, ...)
bot.B.Handle(&btnCloseAll, ...)
bot.B.Handle(tb.OnText, ...)
bot.B.Handle(tb.OnCallback, ...)


I could not find an authorization check in these handlers or in the Bot middleware.

The configured ChatID is used to determine where telemetry is sent. It is not used to authorize incoming commands.

Consequently, any Telegram user who can interact with the bot can potentially:
- Open the management interface.
- Execute dynamic component commands.
- Trigger the global power-off operation.
- Trigger the close-all-positions operation.
This is particularly serious when connected to trading services.

Recommended correction

Introduce a centralized authorization policy:
type AuthorizationPolicy interface {
    IsAuthorized(
        userID int64,
        chatID int64,
        operation string,
    ) bool
}


All command handlers must use the same policy.

I would distinguish at least:
- Read-only access.
- Component management.
- Critical operations.
For critical operations, require an additional confirmation.

The gRPC side also needs independent authentication. Telegram authorization does not authenticate microservices.

Critical
 Finding 2: Pending input is associated with the wrong identity

File: src/telegram/core/bot.go

Currently:
UserStates    map[int64]string
PendingInputs map[int64]*models.CommandButton
UserPaths     map[int64][]string


All three maps use ChatID as the key.

This is insufficient for group conversations, where several users share one chat ID.

There is another, more subtle problem.

When the user submits an input, the code does this:
clientID := bot.UserStates[c.Chat().ID]
pub := bot.Publishers[clientID]


It retrieves the current component from UserStates.

It does not retrieve the component that originally requested the input.

Consider this sequence:
1. User opens Config Server.
2. User selects an IP configuration field.
3. Tele-Remote stores the pending input.
4. User navigates to another component or the selected component disconnects.
5. User submits the IP address.
The input is now associated with the current user state rather than the original request.

This is a genuine routing defect.

Recommended correction

Store the destination and action directly in the pending operation.
type ConversationKey struct {
    ChatID int64
    UserID int64
}

type PendingInput struct {
    ClientID    string
    CommandType int32
    Payload     string
    ExpiresAt   time.Time
}


Use:
map[ConversationKey]PendingInput


Now the input always returns to the service that requested it.

I would also support cancellation and expiration.

Critical
 Finding 3: The gRPC identity is not authenticated

Files:
- src/subscribers/grpc.go
- src/grpc_control/teleremote.proto
- cmd/tele-remote/contracts.go
The server is initialized using:
s.grpcSrv = grpc.NewServer()


There are no TLS credentials or authentication interceptors in this implementation.

The initial component identity is constructed from:
clientID = fmt.Sprintf(
    "%s:%s:%d",
    msg.ComponentName,
    msg.Host,
    msg.Port,
)


These are client-supplied values.

They establish an identifier, not an authenticated identity.

The exchange contract currently describes this as:
Auth: Component identity registered on initial stream frame


That wording is misleading.

A registration frame does not authenticate the sender.

An unauthorized network client could potentially impersonate a component, register its own menu and receive commands intended for that component.

Recommended correction

Use authenticated gRPC connections:
- TLS for transport security.
- Mutual TLS or service credentials for client identity.
- Server-side identity-to-component mapping.
- Authorization of registration and telemetry operations.
- Explicit rejection of duplicate or unauthorized identities.
Do not trust ComponentName, Host or Port as authentication credentials.

3. High-priority implementation defects
High
 Finding 4: ActionMap grows indefinitely

File: src/telegram/ui/menus.go

Every leaf button generates a new action:
uniqueID := registerAction(
    bot,
    createCommandAction(
        bot,
        clientID,
        cmdType,
        payload,
        label,
    ),
)


Then:
bot.ActionMap[id] = fn


The issue is that I could not find any deletion of entries from ActionMap.

Every menu registration creates new entries.

When a component refreshes its menu, the previous callbacks remain in memory.

This creates:
- Unbounded map growth.
- Retention of obsolete callbacks.
- Stale references to component IDs.
- Increasingly unnecessary synchronization.
There is an additional architectural problem: these generated IDs are not actually being used as Telegram inline callback data.

The implementation uses Reply Keyboard buttons and resolves actions through text.

The tb.OnCallback handler simply acknowledges callbacks.

My recommendation: remove ActionMap entirely.

The actual command information is already stored in CommandButton:
type CommandButton struct {
    Label       string
    CommandType int32
    Payload     string
    InputPrompt string
}


When a user selects a button, simply dispatch the selected button directly.

This eliminates:
- ActionMap.
- CbCounter.
- CallbackAction.
- registerAction().
- createCommandAction() as a separate closure factory.
It is a meaningful simplification.

High
 Finding 5: Duplicate button labels can resolve the wrong action metadata

File: src/telegram/ui/menus.go

The current implementation searches for buttons using:
findButtonByLabel(comp.Root, label)


This searches recursively and returns the first matching label.

Imagine a menu:
Configuration
    |
    +-- Network
    |     |
    |     +-- Change value
    |
    +-- Trading
          |
          +-- Change value


The two buttons have the same label but different commands and potentially different input prompts.

The search can return the wrong button.

This is especially problematic because the action closure retrieves InputPrompt using the label rather than using the exact button that generated the action.

Correction: use a unique action identifier supplied by the component, or use a stable identifier generated during menu parsing. Never use the display label as an action identifier.

The action should carry its own immutable command metadata.

High
 Finding 6: Menu registration is not atomic

File: src/telegram/ui/menus.go

The current sequence is:
1. Lock.
2. Delete old sessions with the same component name.
3. Register the new publisher.
4. Unlock.
5. Parse JSON.
6. Register the new menu.
If JSON parsing fails, the previous valid menu has already been removed.

The new publisher may also remain registered even though its menu was rejected.

This means a malformed menu update can leave the registry in an inconsistent state.

Recommended correction

Use a transactional registration sequence:
Receive registration
       |
       v
Validate identity
       |
       v
Parse JSON
       |
       v
Validate menu structure
       |
       v
Build complete menu
       |
       v
Atomic registry replacement


Only after all validation succeeds should the old registration be replaced.

This is one of the easiest improvements to implement without changing the overall architecture.

High
 Finding 7: The command timeout does not actually protect gRPC delivery

File: src/publishers/grpc.go

The interface defines:
PublishCommand(
    ctx context.Context,
    cmdType int32,
    payload, input string,
) error


The caller correctly creates a timeout:
dispatchCtx, cancel := context.WithTimeout(
    context.Background(),
    10*time.Second,
)
defer cancel()


However, the publisher implementation does:
p.mu.Lock()
defer p.mu.Unlock()

return p.stream.Send(cmd)


The ctx parameter is never used.

This is a real discrepancy between the advertised timeout and actual behaviour.

A gRPC server stream's Send() does not accept an individual context. Its lifetime is governed by the stream context.

Therefore, cancelling dispatchCtx does not interrupt an already blocked Send().

The mutex also remains held throughout the network operation.

Recommended correction

I would introduce a serialized outbound writer per stream, with:
- A bounded command queue.
- A single goroutine responsible for stream.Send().
- Context-aware queue admission.
- Stream cancellation handling.
- Explicit queue-full errors.
This preserves gRPC's single-writer requirement without making unrelated Telegram handlers wait on a network write.

An important limitation remains: an individual command already being written cannot be cancelled simply by cancelling the caller's context. Its status must be tracked separately.

High
 Finding 8: Telemetry blocks the gRPC receive loop

File: src/subscribers/grpc.go

The subscriber invokes:
s.cbs.OnTelemetry(payload.Telemetry)


The callback points to:
bot.OnTelemetry


Which eventually calls:
bot.B.Send(chat, msg)


This is a synchronous external HTTP request to Telegram.

Consequently, while Telegram is responding slowly, the gRPC subscriber cannot process the next incoming message on that stream.

This is undesirable for a bidirectional control channel.

A slow notification destination should not prevent the server from receiving registration updates or other telemetry.

Recommended correction

Decouple the two operations:
gRPC receiver
      |
      v
Bounded telemetry queue
      |
      v
Telegram delivery worker
      |
      v
Telegram API


I would use a bounded queue and explicit overflow policy rather than launching an unrestricted goroutine for every message.

Also introduce telemetry rate limiting and batching where appropriate.

High
 Finding 9: Session reconnection can produce stale cleanup

Files:
- src/subscribers/grpc.go
- src/telegram/core/bot.go
- src/telegram/ui/menus.go
The component ID is constructed from its name, host and port.

This is not a unique connection identifier.

Suppose:
1. Component A connects.
2. Component A reconnects using the same name, host and port.
3. The new connection replaces the old publisher.
4. The old stream subsequently disconnects.
5. OnDisconnect() executes for the old connection.
Because both connections have the same clientID, the old disconnect can remove the newly registered session.

The solution is to separate:
- ComponentID: stable logical service identity.
- SessionID: unique connection identity.
For example:
ComponentID = trading-engine
SessionID   = 8f2d...


On disconnect, cleanup should only occur if the session being disconnected is still the current session for that component.

This is a very important correction for a system with automatic reconnection.

High
 Finding 10: Disconnect does not clean up user state

File: src/telegram/core/bot.go

OnDisconnect() currently removes:
delete(bot.DynamicMenus, clientID)
delete(bot.Publishers, clientID)


But it does not invalidate:
- UserStates.
- UserPaths.
- PendingInputs.
A user can therefore retain a navigation path or pending input associated with a component that no longer exists.

The state should be invalidated when the component disconnects.

However, be careful not to cancel a pending operation silently without informing the user. A pending input should receive an explicit cancellation result.

4. Additional code-quality findings
These are not all critical individually, but together they create unnecessary complexity.
File	Finding	Recommendation
menus.go	JSON parsed into map[string]interface{}	Use typed structures
menus.go	Numeric command types arrive as float64	Validate integer ranges
menus.go	Recursive menus have no depth limit	Set maximum nesting
menus.go	Empty labels accepted	Validate required fields
menus.go	Menu rows are flattened	Preserve rows or simplify schema
menus.go	Fixed time.Sleep(200ms) after refresh	Use update acknowledgement
bot.go	User path slices returned after unlocking	Copy slices under lock
bot.go	Raw message content logged	Redact sensitive input
routes.go	Global command errors ignored	Aggregate and report failures
routes.go	Emergency operations have no confirmation	Add confirmation workflow
subscribers/grpc.go	No registration-first enforcement	Require valid handshake
subscribers/grpc.go	Client metadata can change after registration	Bind identity to session
subscribers/grpc.go	Graceful shutdown has no forced timeout	Add bounded shutdown
publishers/grpc.go	Close() is empty	Define ownership and lifecycle
store/persistence.go	Not integrated into runtime	Remove or properly integrate
main.go	Server startup errors are asynchronous	Add readiness signalling

A particularly unnecessary mechanism: the 200 ms sleep
In menus.go:
_ = pub.RequestRefresh(context.Background())
time.Sleep(200 * time.Millisecond)


The code assumes that the component will push an updated menu within 200 milliseconds.

That is not a synchronization guarantee.

If the component takes 250 milliseconds, the user may see stale data.

If it takes 2 seconds, the user still sees stale data.

I would introduce an explicit refresh correlation:
Refresh requested
       |
       v
Refresh ID generated
       |
       v
Component processes request
       |
       v
New menu registered
       |
       v
Refresh acknowledged
       |
       v
Telegram menu updated


If you do not want to complicate the protocol with acknowledgements yet, simply display a refreshing message and let the next menu update trigger a Telegram refresh.

A fixed sleep should not be used to coordinate asynchronous services.

5. A significant issue with the Telegram interface itself
The project documentation refers to inline keyboards and callback actions.

The actual implementation uses tb.ReplyMarkup and menuMarkup.Text(), which generate Reply Keyboard buttons.

This matters because Reply Keyboard buttons are ordinary text messages.

They are not signed action callbacks.

That creates several consequences:
1. A user can type the same text manually.
2. Duplicate labels are ambiguous.
3. A stale keyboard can still submit text.
4. The action must be resolved using current user state.
5. Telegram callback identifiers are not being used as intended.
The current OnCallback handler simply responds:
bot.B.Handle(tb.OnCallback, func(c tb.Context) error {
    return c.Respond()
})


There is no dynamic callback execution here.

My recommendation: consider migrating dynamic component menus to inline keyboards.










Current approach

Reply Keyboard · text-based routing











Recommended approach

Inline Keyboard · explicit callback identifiers


An inline callback could contain an opaque action ID:
action:7f3a2b


The server would resolve it against a registered action.

This removes the ambiguity of matching text labels.

There is one caveat: Telegram callback data has a strict 64-byte limit, so the identifier should remain compact.

I would retain Reply Keyboard for simple global navigation if useful, but use inline keyboards for dynamic service actions.
6. Persistence: an unused subsystem
Files:
- src/store/persistence.go
- src/models/persistence_types.go
There is a PersistenceManager that can load and save component registry state.

However, searching the entire Go source shows no runtime use of this manager.

The current architecture describes a stateless registry reconstructed from connected microservices.

This leaves two competing designs:
- Stateless registry reconstructed from live connections.
- Persisted registry restored from disk.
I would choose the first one for the current project.

The menus are owned by the microservices, so Tele-Remote should not be responsible for restoring potentially obsolete menus after a restart.

I would therefore remove:
src/store/persistence.go
src/models/persistence_types.go


And keep the runtime registry in memory.

If later you need persistence, I would persist command execution records and audit events, not necessarily the entire Telegram menu tree.

There is also a technical problem with the current Save() implementation.

It marshals a caller-owned map while holding only the persistence manager's mutex. That mutex does not protect the map against modifications by other owners.

Concurrent modification can therefore produce a data race or concurrent map iteration failure.

The file is also written directly rather than using an atomic temporary-file replacement.

If persistence is retained, those issues need fixing.
7. The configuration and documentation are out of sync
I found several inconsistencies across the uploaded files.
Documentation/configuration	Actual source
NATS is an available transport	Only gRPC subscriber/publisher implemented
NATS JetStream configuration	No NATS implementation in src
Dynamic inline keyboards	Reply Keyboard implementation
Callback-based actions	Dynamic actions resolved through text
Component identity provides authentication	No authentication in gRPC server
10-second command timeout	Publisher ignores the supplied context
Registry persistence structures	Persistence manager unused

There is also a discrepancy in the advertised configuration.

TeleRemoteCap declares:
type TeleRemoteCap struct {
    Token  string
    ChatID string
    URL    string
    IP     string
    Port   string
}


But main.go obtains the actual listening address through:
grpcAddr, err := appConfig.GetListenAddr("tele_remote")


The IP and Port fields are not used in the displayed startup implementation.

This should be clarified because it is easy to configure the wrong network endpoint when deploying the service.
My documentation recommendation
I would keep only three authoritative documents:
1. README.md: purpose, installation, configuration and operation.
2. ARCHITECTURE.md: actual runtime design and communication contracts.
3. TESTING.md: test strategy and commands.
The AI-specific project notes should not describe capabilities that are not implemented.

8. Proposed simplified data model
I would make the internal menu representation more explicit.

Currently the code parses arbitrary JSON into generic maps, then converts it into CommandButton objects.

I would introduce a typed input model.

For example:
type MenuDefinition struct {
    Version int          `json:"version"`
    Rows    []MenuRowDef `json:"rows"`
}

type MenuRowDef struct {
    Buttons []MenuButtonDef `json:"buttons"`
}

type MenuButtonDef struct {
    ID          string          `json:"id"`
    Label       string          `json:"label"`
    CommandType int32           `json:"cmd_type"`
    Payload     string          `json:"payload"`
    InputPrompt string          `json:"input_prompt"`
    Menu        *MenuDefinition `json:"menu,omitempty"`
}


This is an illustrative proposed contract, not a replacement that can be dropped into the current repository without migrating the connected microservices.

The advantages are significant:
- No interface{} assertions.
- No float64-to-int32 conversions.
- Explicit menu versioning.
- Explicit button identity.
- Easier recursive validation.
- Easier compatibility checks.
I would also add validation rules:
Field	Validation
Menu version	Supported version
Button ID	Required and unique within menu
Label	Required, non-empty
Command type	Known supported value
Payload	Maximum size
Input prompt	Maximum length
Menu nesting	Maximum depth
Number of buttons	Maximum allowed

This prevents malformed or excessively large menus from consuming unnecessary resources.
9. Proposed command execution contract
I think this is the most important architectural improvement for the entire service.

Currently, IPublisher.PublishCommand() returns an error, but the meaning of success is limited to whether the gRPC Send() operation returned successfully.

It does not prove that the remote component received or executed the command.

I would introduce a command envelope with an explicit ID:
type RemoteCommand struct {
    ID          string
    ComponentID string
    ActionID    string
    CommandType int32
    Payload     string
    Input       string
    IssuedAt    time.Time
}


And an execution response:
type CommandResult struct {
    CommandID string
    Status    CommandStatus
    Message   string
}


With these states:
1. CREATED

   Command generated by Tele-Remote.
2. SENT

   Command written to the gRPC stream.
3. RECEIVED

   Microservice confirms reception.
4. EXECUTING

   Microservice starts execution.
5. SUCCEEDED / FAILED / UNKNOWN

   Final result or unresolved execution state.

This is especially important for:
- Restart operations.
- Configuration changes.
- Emergency stops.
- Closing positions.
- Shutdown operations.
For commands that have side effects, I would also implement idempotency.

If Tele-Remote reconnects and retries a command, the receiving microservice should be able to recognize the same command ID and avoid executing the operation twice.

For example, a close-all-positions command must not accidentally be executed multiple times simply because an acknowledgement was lost.

10. Concrete refactoring priorities
Here is the order I would actually implement the changes.
Priority	File(s)	Action
P0	routes.go	Introduce Telegram authorization and critical-command confirmation
P0	subscribers/grpc.go	Authenticate microservice connections
P0	menus.go, bot.go	Fix pending input ownership and identity
P1	menus.go	Remove ActionMap and use explicit action IDs
P1	menus.go	Make registration atomic
P1	publishers/grpc.go	Correct timeout and outbound queue semantics
P1	subscribers/grpc.go	Add session IDs and safe reconnect handling
P1	bot.go	Clean disconnected user sessions
P1	subscribers/grpc.go	Decouple telemetry from stream reception
P2	models/, menus.go	Typed menu contract and validation
P2	store/	Remove unused persistence
P2	main.go	Startup readiness and lifecycle improvements
P2	Documentation	Synchronize implemented capabilities

11. Testing assessment
The existing tests are very limited compared with the responsibilities of this service.

I found:
- TestBot_Broadcast
- TestBot_MenuRegistration
- TestBuildTeleRemoteContracts
They test Telegram initialization, menu parsing and telemetry forwarding.

I did not find dedicated tests for:
- GrpcPublisher.
- GrpcSubscriber.
- Authentication.
- Concurrent registrations.
- Session replacement.
- Pending input routing.
- Command timeout.
- Concurrent stream writes.
- Graceful shutdown.
- Duplicate actions.
- Menu refresh races.
I would add the following tests first:
Recommended regression test suite
0/12 implemented

0%



[ ] Unauthorized Telegram user cannot execute commands
[ ] Unauthorized gRPC client cannot register
[ ] Two users in the same group have independent sessions
[ ] Pending input remains associated with its original component
[ ] Pending input expires and can be cancelled
[ ] Malformed registration does not replace an existing valid menu
[ ] Repeated menu refresh does not increase action registry size
[ ] Old session disconnect cannot delete a new session
[ ] Slow Telegram API does not block gRPC reception
[ ] Cancelled command is not reported as successfully delivered
[ ] Disconnected components are removed from all relevant user states
[ ] Concurrent registrations and navigation pass the race detector

I attempted to execute the test suite, but the environment has Go 1.23.2 while the repository requires Go 1.25.8. Downloading that toolchain was blocked by network restrictions. Therefore, the findings above are based on source inspection, not a successful compilation or test execution.

12. Final assessment: what should actually be simplified?
Tele-Remote
Refactoring required before production use
Area	Assessment
Overall architecture	Good
Transport abstraction	Good foundation
gRPC implementation	Needs improvement
Telegram integration	Functional, but simplistic
Menu architecture	More complicated than necessary
Concurrency	Several real issues
Security	Insufficient
Persistence	Unnecessary in current design
Testing	Insufficient
Documentation	Inconsistent


My central recommendation

Keep the microservice architecture, but simplify the Telegram interaction layer and strengthen the command protocol.

The biggest improvement is to stop treating a Telegram button click as a simple function call and instead treat it as a traceable, authenticated remote command with an explicit destination and execution result.

13. The implementation plan I would choose
I would split the actual code changes into four small iterations.
Iteration 1
Correctness and security
- Authorization.
- Pending input ownership.
- Atomic menu registration.
- Session IDs.
- Cleanup on disconnect.


Iteration 2
Simplification
- Remove ActionMap.
- Introduce typed menu definitions.
- Eliminate duplicate label resolution.
- Remove unused persistence.
- Separate UI state from command metadata.


Iteration 3
Transport reliability
- Correct command timeout semantics.
- Serialized outbound writer.
- Telemetry queue.
- Explicit acknowledgements.
- Reconnection handling.


Iteration 4
Verification and integration
- Race tests.
- Failure injection.
- Protocol versioning.
- Startup readiness.
- Documentation.


14. One architectural decision I would like to preserve
Given the direction of your broader microservice ecosystem, I would definitely preserve:
Business logic
      |
      v
IPublisher / ISubscriber
      |
      +---- gRPC
      |
      +---- Custom TCP (future)
      |
      +---- NATS (only if genuinely needed)


I would not introduce NATS simply because it appears in the configuration and project DNA.

The current gRPC bidirectional stream is perfectly reasonable for this particular control plane.

The important thing is to keep transport-specific details outside the Telegram application core.

That allows you to change the transport later without rewriting the command execution logic.

My preferred next step: implement Iteration 1 and Iteration 2 together, starting with bot.go and menus.go.

Those two files contain most of the unnecessary state management and several of the most important defects. Fixing them first should substantially reduce the complexity of the project before touching the transport layer.

I would aim for a smaller implementation, not a more sophisticated one.

