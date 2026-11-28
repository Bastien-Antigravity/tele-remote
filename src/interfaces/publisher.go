package interfaces

/*
ESSENTIAL PROCESS:
Defines the outbound command and menu-refresh publisher contract for connected
microservice streams in Tele-Remote.

DATA FLOW:
1. Telegram UI or route handlers invoke PublishCommand or RequestRefresh with a bounded context.
2. Concrete transport implementations serialize and transmit the command to the target client.
3. Close releases transport resources when the session terminates.

KEY PARAMETERS:
- ctx: Context governing deadline and cancellation for outbound command delivery.
- cmdType: Numeric command identifier mapped to target microservice actions.
- payload: Static command argument string defined by the menu button.
- input: Optional user-supplied text collected via an input prompt.
*/

import (
	"context"
)

// -----------------------------------------------------------------------------

// Publisher defines the contract for sending commands back to a connected component.
type Publisher interface {
	// PublishCommand sends an action (Power off, Custom command, etc.) to the target client.
	PublishCommand(ctx context.Context, cmdType int32, payload, input string) error

	// RequestRefresh asks the client to resend its menu definition.
	RequestRefresh(ctx context.Context) error

	// Close terminates any resources held by the publisher.
	Close() error
}
