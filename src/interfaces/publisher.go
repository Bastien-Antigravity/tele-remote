package interfaces

import "context"

// -----------------------------------------------------------------------------

// IPublisher defines the contract for sending commands back to a component.
type IPublisher interface {
	// PublishCommand sends an action (Close, Power off, etc.) to the target client.
	PublishCommand(ctx context.Context, cmdType int32, payload, input string) error

	// RequestRefresh asks the client to resend its menu definition
	RequestRefresh(ctx context.Context) error

	// Close terminates any resources held by the publisher.
	Close() error
}
