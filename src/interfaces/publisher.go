package interfaces

import "context"

// -----------------------------------------------------------------------------

// Publisher defines the contract for sending commands back to a component.
type Publisher interface {
	// PublishCommand sends an action (Close, Power off, etc.) to the target client.
	PublishCommand(ctx context.Context, cmdType int32, payload string) error

	// Close terminates any resources held by the publisher.
	Close() error
}
