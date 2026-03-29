package publishers

import (
	"context"

	"tele-remote/src/grpc_control"
	"tele-remote/src/interfaces"
)

// GrpcPublisher wraps a bidirectional gRPC stream and implements interfaces.Publisher
type GrpcPublisher struct {
	stream grpc_control.TeleRemoteService_ConnectServer
}

func NewGrpcPublisher(stream grpc_control.TeleRemoteService_ConnectServer) interfaces.Publisher {
	return &GrpcPublisher{stream: stream}
}

func (p *GrpcPublisher) PublishCommand(ctx context.Context, cmdType int32, payload string) error {
	cmd := &grpc_control.BotCommand{
		CommandType:   grpc_control.BotCommand_CommandType(cmdType),
		CustomPayload: payload,
	}
	// Note: gRPC streams are not safe for concurrent calling of SendMsg.
	// We rely on the Bot's mutex or sequential callback execution.
	return p.stream.Send(cmd)
}

func (p *GrpcPublisher) Close() error {
	// Let the gRPC server or client handle disconnection natively
	return nil
}
