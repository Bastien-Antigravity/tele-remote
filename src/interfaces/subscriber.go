package interfaces

import "context"

// -----------------------------------------------------------------------------

// SubscriberCallbacks defines the triggers from any transport layer to the Bot context
type SubscriberCallbacks struct {
	// OnTelemetry receives raw logs/events from the component
	OnTelemetry func(msg string)

	// OnRegistration receives a specific menuJSON from a component
	OnRegistration func(clientID, componentName, menuJSON string, pub Publisher)

	// OnDisconnect handles cleanup when a component drops
	OnDisconnect func(clientID string)
}

// -----------------------------------------------------------------------------

// Subscriber abstracts an incoming connection listener (gRPC server, NATS loop, etc)
type Subscriber interface {
	// StartListen blocks until the service closes or errors
	StartListen(ctx context.Context, cbs SubscriberCallbacks) error

	// Close terminates the listener
	Close() error
}
