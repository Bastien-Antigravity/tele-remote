package interfaces

/*
ESSENTIAL PROCESS:
Defines the inbound transport listener and callback contracts bridging connected
microservice streams to the Tele-Remote bot core.

DATA FLOW:
1. Concrete subscribers accept bidirectional gRPC streams from fleet microservices.
2. Registration payloads invoke OnRegistration with the component menu JSON and stream publisher.
3. Telemetry events invoke OnTelemetry for asynchronous broadcast to the authorized Telegram chat.
4. Stream termination invokes OnDisconnect with the disconnecting stream's publisher reference.

KEY PARAMETERS:
- clientID: Unique session identifier for the connected microservice instance.
- componentName: Human-readable display name of the microservice.
- menuJSON: Serialized dynamic menu tree definition.
- pub: Session-bound Publisher used to verify identity on disconnect and dispatch commands.
*/

import (
	"context"
)

// -----------------------------------------------------------------------------

// SubscriberCallbacks defines the triggers from any transport layer to the Bot context.
type SubscriberCallbacks struct {
	// OnTelemetry receives raw logs/events from the component.
	OnTelemetry func(msg string)

	// OnRegistration receives a menu registration payload from a component.
	OnRegistration func(clientID, componentName, menuJSON string, pub Publisher)

	// OnDisconnect handles cleanup when a component drops, passing the disconnecting stream publisher.
	OnDisconnect func(clientID string, pub ...Publisher)
}

// -----------------------------------------------------------------------------

// Subscriber abstracts an incoming connection listener (gRPC server, etc.).
type Subscriber interface {
	// StartListen blocks until the service closes or errors.
	StartListen(ctx context.Context, cbs SubscriberCallbacks) error

	// Close terminates the listener.
	Close() error
}
