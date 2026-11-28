package publishers

/*
ESSENTIAL PROCESS:
Wraps an active bidirectional gRPC server stream into a thread-safe Publisher
that enforces bounded caller context cancellation, deadlines, and automatic
stream eviction when a stalled peer blocks outbound writes.

DATA FLOW:
1. Callers invoke PublishCommand or RequestRefresh with a bounded context.
2. sendWithContext fails fast if the stream is already closed/aborted or ctx has expired.
3. sendWithContext acquires the 1-slot sendSem respecting ctx.Done() and dispatches stream.Send.
4. If ctx expires while stream.Send is blocked, the publisher marks itself aborted and invokes onAbort to tear down the stalled stream so subsequent commands never wedge indefinitely.

KEY PARAMETERS:
- stream: Active gRPC bidirectional ConnectServer stream for a connected microservice.
- sendSem: 1-slot buffered channel serializing concurrent Send calls.
- onAbort: Optional callback triggered on write timeout to close the stalled gRPC stream.
*/

import (
	"context"
	"errors"
	"sync/atomic"

	"github.com/Bastien-Antigravity/tele-remote/src/grpc_control"
	"github.com/Bastien-Antigravity/tele-remote/src/interfaces"
)

// -----------------------------------------------------------------------------

// ErrStreamAborted indicates the publisher stream was terminated after a stalled write timeout.
var ErrStreamAborted = errors.New("gRPC stream aborted due to stalled send timeout")

// -----------------------------------------------------------------------------

// GrpcPublisher wraps a bidirectional gRPC stream and implements interfaces.Publisher.
type GrpcPublisher struct {
	stream  grpc_control.TeleRemoteService_ConnectServer
	sendSem chan struct{}
	aborted atomic.Bool
	onAbort func()
}

// -----------------------------------------------------------------------------

// NewGrpcPublisher creates a thread-safe gRPC stream publisher with an optional abort hook for stalled streams.
func NewGrpcPublisher(stream grpc_control.TeleRemoteService_ConnectServer, onAbort ...func()) interfaces.Publisher {
	var abortFn func()
	if len(onAbort) > 0 {
		abortFn = onAbort[0]
	}
	return &GrpcPublisher{
		stream:  stream,
		sendSem: make(chan struct{}, 1),
		onAbort: abortFn,
	}
}

// -----------------------------------------------------------------------------

func (p *GrpcPublisher) abort() {
	if p.aborted.CompareAndSwap(false, true) && p.onAbort != nil {
		p.onAbort()
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
	if p.aborted.Load() {
		return ErrStreamAborted
	}

	select {
	case p.sendSem <- struct{}{}:
	case <-ctx.Done():
		p.abort()
		return ctx.Err()
	}

	if p.aborted.Load() {
		<-p.sendSem
		return ErrStreamAborted
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
		if err != nil {
			p.abort()
		}
		return err
	case <-ctx.Done():
		p.abort()
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

// Close marks the publisher closed and aborts any stalled send.
func (p *GrpcPublisher) Close() error {
	p.abort()
	return nil
}
