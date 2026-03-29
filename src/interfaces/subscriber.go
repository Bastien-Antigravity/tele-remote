package interfaces

import "context"

// SubscriberCallbacks defines how subscribers notify the unified Bot logic
type SubscriberCallbacks struct {
	OnTelemetry    func(msg string)
	OnRegistration func(clientID string, componentName string, menuJSON string, dynamicPublisher Publisher)
	OnDisconnect   func(clientID string)
}

// Subscriber abstracts an incoming connection listener (gRPC server, NATS loop, etc)
type Subscriber interface {
	StartListen(ctx context.Context, cbs SubscriberCallbacks) error
	Close() error
}
