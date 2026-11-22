package subscribers

/*
ESSENTIAL PROCESS:
Implements the gRPC transport listener for Tele-Remote, managing bidirectional
component streams, menu registration callbacks, telemetry forwarding, and session-safe disconnects.

DATA FLOW:
1. StartListen binds the configured TCP address and serves TeleRemoteServiceServer.
2. Each connected microservice opens a bidirectional Connect stream wrapped in a dedicated GrpcPublisher.
3. Registration and telemetry messages are forwarded to the bot callbacks.
4. When a stream closes, OnDisconnect is invoked with both clientID and the stream's specific dynPub so stale stream closures never evict a newer reconnected session.

KEY PARAMETERS:
- bindIP: Configured IP/hostname for the gRPC server listener.
- bindPort: Configured TCP port for the gRPC server listener.
- cbs: Callback hooks linking transport events to the Telegram bot core.
*/

import (
	"context"
	"fmt"
	"net"

	"github.com/Bastien-Antigravity/tele-remote/src/grpc_control"
	tele_interfaces "github.com/Bastien-Antigravity/tele-remote/src/interfaces"
	"github.com/Bastien-Antigravity/tele-remote/src/publishers"

	"github.com/Bastien-Antigravity/universal-logger/src/interfaces"

	"google.golang.org/grpc"
)

// -----------------------------------------------------------------------------

// GrpcSubscriber acts as the Tele-Remote generic gRPC Listener
type GrpcSubscriber struct {
	grpc_control.UnimplementedTeleRemoteServiceServer
	log      interfaces.Logger
	bindIP   string
	bindPort int
	cbs      tele_interfaces.ISubscriberCallbacks
	grpcSrv  *grpc.Server
}

// -----------------------------------------------------------------------------

// NewGrpcSubscriber instantiates a gRPC transport listener bound to ip:port.
func NewGrpcSubscriber(l interfaces.Logger, ip string, port int) tele_interfaces.ISubscriber {
	return &GrpcSubscriber{
		log:      l,
		bindIP:   ip,
		bindPort: port,
	}
}

// -----------------------------------------------------------------------------

// StartListen binds the TCP socket and serves incoming gRPC component streams.
func (s *GrpcSubscriber) StartListen(ctx context.Context, cbs tele_interfaces.ISubscriberCallbacks) error {
	s.cbs = cbs
	addr := fmt.Sprintf("%s:%d", s.bindIP, s.bindPort)
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		s.log.Error("gRPC Subscriber failed to listen", "err", err, "addr", addr)
		return err
	}

	s.grpcSrv = grpc.NewServer()
	grpc_control.RegisterTeleRemoteServiceServer(s.grpcSrv, s)

	go func() {
		<-ctx.Done()
		s.log.Info("Shutting down gRPC Subscriber gently")
		s.grpcSrv.GracefulStop()
	}()

	s.log.Info("Starting gRPC Subscriber", "addr", addr)
	return s.grpcSrv.Serve(lis)
}

// -----------------------------------------------------------------------------

// Connect implements the grpc_control.TeleRemoteServiceServer stream loop
func (s *GrpcSubscriber) Connect(stream grpc_control.TeleRemoteService_ConnectServer) error {
	var clientID string

	// Wrap stream as publisher immediately
	dynPub := publishers.NewGrpcPublisher(stream)

	for {
		msg, err := stream.Recv()
		if err != nil {
			s.log.Warning("stream.Recv error or client disconnected", "client", clientID, "err", err)
			if clientID != "" && s.cbs.OnDisconnect != nil {
				go s.cbs.OnDisconnect(clientID, dynPub)
			}
			return err
		}

		if clientID == "" && msg.ComponentName != "" {
			clientID = fmt.Sprintf("%s:%s:%d", msg.ComponentName, msg.Host, msg.Port)
			s.log.Info("New client connected via gRPC", "id", clientID)
		}

		switch payload := msg.Payload.(type) {
		case *grpc_control.ComponentMessage_Registration:
			if s.cbs.OnRegistration != nil {
				reg := payload.Registration
				menuJSON := ""
				if reg != nil {
					menuJSON = reg.MenuJson
				}
				s.cbs.OnRegistration(clientID, msg.ComponentName, menuJSON, dynPub)
			}
		case *grpc_control.ComponentMessage_Telemetry:
			if s.cbs.OnTelemetry != nil {
				s.cbs.OnTelemetry(payload.Telemetry)
			}
		case *grpc_control.ComponentMessage_Qmsg:
			if s.cbs.OnTelemetry != nil {
				q := payload.Qmsg
				s.cbs.OnTelemetry(fmt.Sprintf("%s => %s: %s", q.FromAddr, q.ToAddr, q.Msg))
			}
		}
	}
}

// -----------------------------------------------------------------------------

// Close gracefully stops the gRPC server.
func (s *GrpcSubscriber) Close() error {
	if s.grpcSrv != nil {
		s.grpcSrv.GracefulStop()
	}
	return nil
}
