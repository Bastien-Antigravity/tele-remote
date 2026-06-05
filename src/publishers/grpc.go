package publishers

import (
	"context"
	"sync"

	"github.com/Bastien-Antigravity/tele-remote/src/grpc_control"
	"github.com/Bastien-Antigravity/tele-remote/src/interfaces"
)

// GrpcPublisher wraps a bidirectional gRPC stream and implements interfaces.IPublisher
type GrpcPublisher struct {
	stream grpc_control.TeleRemoteService_ConnectServer
	mu     sync.Mutex
}

func NewGrpcPublisher(stream grpc_control.TeleRemoteService_ConnectServer) interfaces.IPublisher {
	return &GrpcPublisher{stream: stream}
}

func (p *GrpcPublisher) PublishCommand(ctx context.Context, cmdType int32, payload string) error {
	cmd := &grpc_control.BotCommand{
		CommandType:   grpc_control.BotCommand_CommandType(cmdType),
		CustomPayload: payload,
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	return p.stream.Send(cmd)
}

func (p *GrpcPublisher) Close() error {
	return nil
}
