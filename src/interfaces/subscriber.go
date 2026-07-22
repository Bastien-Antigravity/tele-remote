package interfaces

import (
	"context"
)

// -----------------------------------------------------------------------------

// ISubscriberCallbacks defines the triggers from any transport layer to the Bot context
type ISubscriberCallbacks struct {
	// OnTelemetry receives raw logs/events from the component
	OnTelemetry func(msg string)

	// OnRegistration receives a specific menu protobuf from a component
	OnRegistration func(clientID, componentName, menuJSON string, pub IPublisher)

	// OnDisconnect handles cleanup when a component drops
	OnDisconnect func(clientID string)
}

// -----------------------------------------------------------------------------

// ISubscriber abstracts an incoming connection listener (gRPC server, NATS loop, etc)
type ISubscriber interface {
	// StartListen blocks until the service closes or errors
	StartListen(ctx context.Context, cbs ISubscriberCallbacks) error

	// Close terminates the listener
	Close() error
}
