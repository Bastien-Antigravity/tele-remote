package publishers

/*
ESSENTIAL PROCESS:
Wraps an active bidirectional gRPC server stream into a thread-safe IPublisher
that enforces bounded caller context cancellation and deadlines during sends.

DATA FLOW:
1. Callers invoke PublishCommand or RequestRefresh with a context.
2. sendWithContext checks ctx.Err(), acquires the 1-slot sendSem respecting ctx.Done(), and dispatches stream.Send in a goroutine.
3. Callers unblock immediately if ctx expires even when the underlying gRPC flow-control window stalls.

KEY PARAMETERS:
- stream: Active gRPC bidirectional ConnectServer stream for a connected microservice.
- sendSem: 1-slot buffered channel serializing concurrent Send calls without blocking forever on ctx cancellation.
*/

import (
	"context"

	"github.com/Bastien-Antigravity/tele-remote/src/grpc_control"
	"github.com/Bastien-Antigravity/tele-remote/src/interfaces"
)

// -----------------------------------------------------------------------------

// GrpcPublisher wraps a bidirectional gRPC stream and implements interfaces.IPublisher
type GrpcPublisher struct {
	stream  grpc_control.TeleRemoteService_ConnectServer
	sendSem chan struct{}
}

// -----------------------------------------------------------------------------

// NewGrpcPublisher creates a thread-safe gRPC stream publisher.
func NewGrpcPublisher(stream grpc_control.TeleRemoteService_ConnectServer) interfaces.IPublisher {
	return &GrpcPublisher{
		stream:  stream,
		sendSem: make(chan struct{}, 1),
	}
}

// -----------------------------------------------------------------------------

func (p *GrpcPublisher) sendWithContext(ctx context.Context, cmd *grpc_control.BotCommand) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	select {
	case p.sendSem <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}

	if err := ctx.Err(); err != nil {
		<-p.sendSem
		return err
	}

	errCh := make(chan error, 1)
	go func() {
		defer func() { <-p.sendSem }()
		errCh <- p.stream.Send(cmd)
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// -----------------------------------------------------------------------------

// PublishCommand sends a command to the connected microservice if the context has not expired.
func (p *GrpcPublisher) PublishCommand(ctx context.Context, cmdType int32, payload, input string) error {
	cmd := &grpc_control.BotCommand{
		CommandType:   grpc_control.BotCommand_CommandType(cmdType),
		CustomPayload: payload,
		Input:         input,
	}
	return p.sendWithContext(ctx, cmd)
}

// -----------------------------------------------------------------------------

// RequestRefresh asks the connected microservice to push a fresh menu definition.
func (p *GrpcPublisher) RequestRefresh(ctx context.Context) error {
	cmd := &grpc_control.BotCommand{
		CommandType: grpc_control.BotCommand_REFRESH_MENU,
	}
	return p.sendWithContext(ctx, cmd)
}

// -----------------------------------------------------------------------------

// Close satisfies interfaces.IPublisher.
func (p *GrpcPublisher) Close() error {
	return nil
}
