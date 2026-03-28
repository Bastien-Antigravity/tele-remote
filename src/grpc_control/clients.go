package grpc_control

import (
	"fmt"
)

// -----------------------------------------------------------------------------
// addClient tracks a newly connected gRPC stream
func (s *Server) addClient(id string, stream TeleRemoteService_ConnectServer) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.clients[id] = stream
}

// -----------------------------------------------------------------------------
// removeClient unregisters a stream and triggers the OnDisconnect callback
func (s *Server) removeClient(id string) {
	if id == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.clients[id]; exists {
		delete(s.clients, id)
		if s.callbacks.OnDisconnect != nil {
			go s.callbacks.OnDisconnect(id)
		}
	}
}

// -----------------------------------------------------------------------------
// TargetedBroadcastCommand sends a BotCommand to a specific client
func (s *Server) TargetedBroadcastCommand(clientID string, cmdType BotCommand_CommandType, payload string) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stream, exists := s.clients[clientID]
	if !exists {
		return fmt.Errorf("client %s not connected", clientID)
	}

	cmd := &BotCommand{
		CommandType:   cmdType,
		CustomPayload: payload,
	}

	err := stream.Send(cmd)
	if err != nil {
		s.log.Error("failed to send command to client", "client", clientID, "err", err)
	} else {
		s.log.Debug("Sent command to client", "command", cmdType, "client", clientID)
	}
	return err
}

// -----------------------------------------------------------------------------
// BroadcastCommand sends a BotCommand to all connected clients
func (s *Server) BroadcastCommand(cmdType BotCommand_CommandType, payload string) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	cmd := &BotCommand{
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

// -----------------------------------------------------------------------------
// StopAllComponents gracefully signals all clients to close positions
func (s *Server) StopAllComponents() {
	s.BroadcastCommand(BotCommand_CLOSE_ALL_POSITIONS, "")
}

// -----------------------------------------------------------------------------
// PowerOffAll forcefully signals all clients to shut down
func (s *Server) PowerOffAll() {
	s.BroadcastCommand(BotCommand_POWER_OFF, "")
}
