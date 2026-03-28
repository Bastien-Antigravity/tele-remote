package grpc

import (
	"context"
	"fmt"
	"net"
	"sync"
	pb "github.com/Bastien-Antigravity/TeleRemote/pb"
	"github.com/Bastien-Antigravity/flexible-logger/src/interfaces"
	"google.golang.org/grpc"
)

type Server struct {
	pb.UnimplementedTeleRemoteServiceServer
	log      interfaces.Logger
	bindIP   string
	bindPort int

	mu      sync.RWMutex
	clients map[string]pb.TeleRemoteService_ConnectServer

	// Telemetry callback
	onTelemetry func(string)
}

func NewServer(l interfaces.Logger, ip string, port int, teleCb func(string)) *Server {
	return &Server{
		log:         l,
		bindIP:      ip,
		bindPort:    port,
		clients:     make(map[string]pb.TeleRemoteService_ConnectServer),
		onTelemetry: teleCb,
	}
}

// Start initiates the gRPC listener.
func (s *Server) Start(ctx context.Context) error {
	addr := fmt.Sprintf("%s:%d", s.bindIP, s.bindPort)
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		s.log.Error("failed to listen", "err", err, "addr", addr)
		return err
	}

	grpcServer := grpc.NewServer()
	pb.RegisterTeleRemoteServiceServer(grpcServer, s)

	go func() {
		<-ctx.Done()
		s.log.Info("Shutting down gRPC Server gently")
		grpcServer.GracefulStop()
	}()

	s.log.Info("Starting gRPC server", "addr", addr)
	return grpcServer.Serve(lis)
}

// Connect provides a bi-directional streaming connection.
func (s *Server) Connect(stream pb.TeleRemoteService_ConnectServer) error {
	var clientID string

	// Recv loop
	for {
		msg, err := stream.Recv()
		if err != nil {
			s.log.Warn("stream.Recv error or client disconnected", "client", clientID, "err", err)
			s.removeClient(clientID)
			return err
		}

		if clientID == "" && msg.ComponentName != "" {
			clientID = fmt.Sprintf("%s:%s:%d", msg.ComponentName, msg.Host, msg.Port)
			s.addClient(clientID, stream)
			s.log.Info("New client connected", "id", clientID)
		}

		switch payload := msg.Payload.(type) {
		case *pb.ComponentMessage_Registration:
			s.log.Debug("Registration payload received", "id", clientID)
		case *pb.ComponentMessage_Telemetry:
			if s.onTelemetry != nil {
				s.onTelemetry(payload.Telemetry)
			}
		case *pb.ComponentMessage_Qmsg:
			if s.onTelemetry != nil {
				q := payload.Qmsg
				s.onTelemetry(fmt.Sprintf("%s => %s: %s", q.FromAddr, q.ToAddr, q.Msg))
			}
		}
	}
}

func (s *Server) addClient(id string, stream pb.TeleRemoteService_ConnectServer) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.clients[id] = stream
}

func (s *Server) removeClient(id string) {
	if id == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.clients, id)
}

// BroadcastCommand sends a BotCommand to all connected clients.
func (s *Server) BroadcastCommand(cmdType pb.BotCommand_CommandType, payload string) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	cmd := &pb.BotCommand{
		CommandType:   cmdType,
		CustomPayload: payload,
	}

	for id, stream := range s.clients {
		err := stream.Send(cmd)
		if err != nil {
			s.log.Error("failed to send command to client", "client", id, "err", err)
		} else {
			s.log.Debug("Sent command to client", "command", cmdType, "client", id)
		}
	}
}

// Graceful helpers for Stop all
func (s *Server) StopAllComponents() {
	s.BroadcastCommand(pb.BotCommand_CLOSE_ALL_POSITIONS, "")
}

func (s *Server) PowerOffAll() {
	s.BroadcastCommand(pb.BotCommand_POWER_OFF, "")
}
