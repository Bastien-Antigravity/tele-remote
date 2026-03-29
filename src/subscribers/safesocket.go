package subscribers

import (
	"context"

	"tele-remote/src/config"
	"tele-remote/src/interfaces"
	// "tele-remote/src/publishers"
	// "tele-remote/src/serializers"

	flexlogger "github.com/Bastien-Antigravity/flexible-logger/src/interfaces"
	// "github.com/Bastien-Antigravity/safe-socket/pkg/safesocket"
)

// SafeSocketSubscriber abstracts a safe-socket TCP/Unix listener for Component payloads
type SafeSocketSubscriber struct {
	cfg *config.Config
	log flexlogger.Logger
}

func NewSafeSocketSubscriber(c *config.Config, l flexlogger.Logger) interfaces.Subscriber {
	return &SafeSocketSubscriber{cfg: c, log: l}
}

func (s *SafeSocketSubscriber) StartListen(ctx context.Context, cbs interfaces.SubscriberCallbacks) error {
	s.log.Info("SafeSocket Subscriber initialized (Skeleton)")

	// Future Implementation would:
	// 1. Initialise safe-socket server on s.cfg.SafeSocket.Port
	// 2. Wrap incoming sockets with &serializers.BinSerializer{}
	// 3. When messages arrive, decode to Registration or Telemetry map
	// 4. Extract Pub Config (if any) or default to the bidirectional socket
	// 5. Create dynPub := publishers.NewSafeSocketPublisher(ser, socket)
	// 6. Fire cbs.OnRegistration(clientID, componentName, menuJSON, dynPub)

	go func() {
		<-ctx.Done()
		s.Close()
	}()

	return nil
}

func (s *SafeSocketSubscriber) Close() error {
	return nil
}
