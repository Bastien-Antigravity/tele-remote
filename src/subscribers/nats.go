package subscribers

import (
	"context"
	"encoding/json"
	"fmt"

	"tele-remote/src/config"
	"tele-remote/src/interfaces"
	"tele-remote/src/publishers"

	flexlogger "github.com/Bastien-Antigravity/flexible-logger/src/interfaces"
	msg_serializers "github.com/Bastien-Antigravity/message-serializers/src/serializers"
	"github.com/nats-io/nats.go"
)

// NatsSubscriber connects to a NATS cluster and listens for Component messages
type NatsSubscriber struct {
	cfg *config.Config
	log flexlogger.Logger
	nc  *nats.Conn
	sub *nats.Subscription
}

func NewNatsSubscriber(c *config.Config, l flexlogger.Logger) interfaces.Subscriber {
	return &NatsSubscriber{cfg: c, log: l}
}

func (s *NatsSubscriber) StartListen(ctx context.Context, cbs interfaces.SubscriberCallbacks) error {
	if len(s.cfg.Nats.Servers) == 0 {
		s.log.Warning("NATS disabled: no servers configured")
		return nil
	}

	nc, err := nats.Connect(s.cfg.Nats.Servers[0], nats.Name(s.cfg.Nats.ClientID))
	if err != nil {
		return fmt.Errorf("nats connect failed: %v", err)
	}
	s.nc = nc

	// Use the shared JSON serializer
	ser := msg_serializers.NewJSONSerializer()

	s.log.Info("Starting NATS Subscriber", "subject", s.cfg.Nats.SubjectPrefix+".>")

	sub, err := nc.Subscribe(s.cfg.Nats.SubjectPrefix+".>", func(m *nats.Msg) {
		var msgMap map[string]interface{}
		if err := ser.Unmarshal(m.Data, &msgMap); err != nil {
			s.log.Warning("Failed to parse incoming nats msg: ", err)
			return
		}

		compNameAny, ok := msgMap["component_name"]
		if !ok {
			return // invalid message
		}
		clientID := compNameAny.(string)

		if regAny, ok := msgMap["registration"]; ok {
			regMap := regAny.(map[string]interface{})
			menuJSON := ""
			if m, ok := regMap["menu_json"].(string); ok {
				menuJSON = m
			}
			
			// Extract Pub Config supplied by the client
			pubSubject := ""
			if pubConfAny, ok := regMap["pub_config"]; ok {
				pubConfigJSON := pubConfAny.(string)
				var pubSubjMap map[string]string
				if err := json.Unmarshal([]byte(pubConfigJSON), &pubSubjMap); err == nil {
					pubSubject = pubSubjMap["subject"]
				}
			}

			if pubSubject == "" {
				s.log.Warning("NATS Registration rejected, required 'pub_config' -> 'subject' is missing")
				return
			}

			dynPub := publishers.NewNatsPublisher(s.nc, pubSubject, ser)
			cbs.OnRegistration(clientID, clientID, menuJSON, dynPub)

		} else if telAny, ok := msgMap["telemetry"]; ok {
			cbs.OnTelemetry(telAny.(string))
		}
	})

	if err != nil {
		return err
	}
	s.sub = sub

	// Listen until context cancellation
	go func() {
		<-ctx.Done()
		s.Close()
	}()

	return nil
}

func (s *NatsSubscriber) Close() error {
	if s.sub != nil {
		_ = s.sub.Unsubscribe()
	}
	if s.nc != nil {
		s.nc.Close()
	}
	return nil
}
