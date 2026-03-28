package grpc_control

import (
	"fmt"
)

// -----------------------------------------------------------------------------
// Connect provides a bi-directional streaming connection
func (s *Server) Connect(stream TeleRemoteService_ConnectServer) error {
	var clientID string

	// Recv loop
	for {
		msg, err := stream.Recv()
		if err != nil {
			s.log.Warning("stream.Recv error or client disconnected", "client", clientID, "err", err)
			s.removeClient(clientID)
			return err
		}

		if clientID == "" && msg.ComponentName != "" {
			clientID = fmt.Sprintf("%s:%s:%d", msg.ComponentName, msg.Host, msg.Port)
			s.addClient(clientID, stream)
			s.log.Info("New client connected", "id", clientID)
		}

		switch payload := msg.Payload.(type) {
		case *ComponentMessage_Registration:
			s.log.Debug("Registration payload received", "id", clientID)
			if s.callbacks.OnRegistration != nil {
				reg := payload.Registration
				menuJSON := ""
				if reg != nil {
					menuJSON = reg.MenuJson
				}
				s.callbacks.OnRegistration(clientID, msg.ComponentName, menuJSON)
			}
		case *ComponentMessage_Telemetry:
			if s.callbacks.OnTelemetry != nil {
				s.callbacks.OnTelemetry(payload.Telemetry)
			}
		case *ComponentMessage_Qmsg:
			if s.callbacks.OnTelemetry != nil {
				q := payload.Qmsg
				s.callbacks.OnTelemetry(fmt.Sprintf("%s => %s: %s", q.FromAddr, q.ToAddr, q.Msg))
			}
		}
	}
}
