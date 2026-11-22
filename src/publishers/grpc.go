package publishers

/*
ESSENTIAL PROCESS:
Wraps an active bidirectional gRPC server stream into a thread-safe IPublisher
that respects caller context cancellation and deadlines before sending.

DATA FLOW:
1. Callers invoke PublishCommand or RequestRefresh with a context.
2. Context validity is verified via ctx.Err() prior to acquiring the stream mutex.
3. serialized BotCommand protobuf frames are transmitted over the gRPC stream.

KEY PARAMETERS:
- stream: Active gRPC bidirectional ConnectServer stream for a connected microservice.
- mu: Mutex serializing concurrent Send calls on the underlying gRPC stream.
*/

import (
	"context"
	"sync"

	"github.com/Bastien-Antigravity/tele-remote/src/grpc_control"
	"github.com/Bastien-Antigravity/tele-remote/src/interfaces"
)

// -----------------------------------------------------------------------------

// GrpcPublisher wraps a bidirectional gRPC stream and implements interfaces.IPublisher
type GrpcPublisher struct {
	stream grpc_control.TeleRemoteService_ConnectServer
	mu     sync.Mutex
}

// -----------------------------------------------------------------------------

// NewGrpcPublisher creates a thread-safe gRPC stream publisher.
func NewGrpcPublisher(stream grpc_control.TeleRemoteService_ConnectServer) interfaces.IPublisher {
	return &GrpcPublisher{stream: stream}
}

// -----------------------------------------------------------------------------

// PublishCommand sends a command to the connected microservice if the context has not expired.
func (p *GrpcPublisher) PublishCommand(ctx context.Context, cmdType int32, payload, input string) error {
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return err
		}
	}

	cmd := &grpc_control.BotCommand{
		CommandType:   grpc_control.BotCommand_CommandType(cmdType),
		CustomPayload: payload,
		Input:         input,
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return err
		}
	}
	return p.stream.Send(cmd)
}

// -----------------------------------------------------------------------------

// RequestRefresh asks the connected microservice to push a fresh menu definition.
func (p *GrpcPublisher) RequestRefresh(ctx context.Context) error {
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return err
		}
	}

	cmd := &grpc_control.BotCommand{
		CommandType: grpc_control.BotCommand_REFRESH_MENU,
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return err
		}
	}
	return p.stream.Send(cmd)
}

// -----------------------------------------------------------------------------

// Close satisfies interfaces.IPublisher.
func (p *GrpcPublisher) Close() error {
	return nil
}
