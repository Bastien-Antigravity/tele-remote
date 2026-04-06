package subscribers

import (
	"context"
	"fmt"
	"net"

	unilogger "github.com/Bastien-Antigravity/universal-logger/src/logger"
	"google.golang.org/grpc"

	"tele-remote/src/grpc_control"
	"tele-remote/src/interfaces"
	"tele-remote/src/publishers"
)

// GrpcSubscriber acts as the Tele-Remote generic gRPC Listener
type GrpcSubscriber struct {
	grpc_control.UnimplementedTeleRemoteServiceServer
	log      *unilogger.UniLog
	bindIP   string
	bindPort int
	cbs      interfaces.SubscriberCallbacks
	grpcSrv  *grpc.Server
}

func NewGrpcSubscriber(l *unilogger.UniLog, ip string, port int) interfaces.Subscriber {
	return &GrpcSubscriber{
		log:      l,
		bindIP:   ip,
		bindPort: port,
	}
}

func (s *GrpcSubscriber) StartListen(ctx context.Context, cbs interfaces.SubscriberCallbacks) error {
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
				go s.cbs.OnDisconnect(clientID)
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

func (s *GrpcSubscriber) Close() error {
	if s.grpcSrv != nil {
		s.grpcSrv.GracefulStop()
	}
	return nil
}
