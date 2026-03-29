package interfaces

import "context"

// Publisher abstracts sending commands back to a component.
type Publisher interface {
	PublishCommand(ctx context.Context, cmdType int32, payload string) error
	Close() error
}
