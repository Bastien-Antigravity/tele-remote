package grpc_control

import (
	"context"
	"fmt"
	"net"
	"sync"

	"github.com/Bastien-Antigravity/flexible-logger/src/interfaces"
	"google.golang.org/grpc"
)

// -----------------------------------------------------------------------------
// Server manages the gRPC connections and callbacks
type Server struct {
	UnimplementedTeleRemoteServiceServer
	log      interfaces.Logger
	bindIP   string
	bindPort int

	mu      sync.RWMutex
	clients map[string]TeleRemoteService_ConnectServer

	callbacks ServerCallbacks
}

// -----------------------------------------------------------------------------
// ServerCallbacks groups event callbacks from the gRPC server to the Bot
type ServerCallbacks struct {
	OnTelemetry    func(string)
	OnRegistration func(clientID string, componentName string, menuJSON string)
	OnDisconnect   func(clientID string)
}

// -----------------------------------------------------------------------------
// NewServer initializes the gRPC service structures
func NewServer(l interfaces.Logger, ip string, port int, cbs ServerCallbacks) *Server {
	return &Server{
		log:       l,
		bindIP:    ip,
		bindPort:  port,
		clients:   make(map[string]TeleRemoteService_ConnectServer),
		callbacks: cbs,
	}
}

// -----------------------------------------------------------------------------
// Start initiates the gRPC listener and grpc.Server
func (s *Server) Start(ctx context.Context) error {
	addr := fmt.Sprintf("%s:%d", s.bindIP, s.bindPort)
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		s.log.Error("failed to listen", "err", err, "addr", addr)
		return err
	}

	grpcServer := grpc.NewServer()
	RegisterTeleRemoteServiceServer(grpcServer, s)

	go func() {
		<-ctx.Done()
		s.log.Info("Shutting down gRPC Server gently")
		grpcServer.GracefulStop()
	}()

	s.log.Info("Starting gRPC server", "addr", addr)
	return grpcServer.Serve(lis)
}
