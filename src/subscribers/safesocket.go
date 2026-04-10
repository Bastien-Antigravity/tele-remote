package subscribers

import (
	"context"

	"github.com/Bastien-Antigravity/tele-remote/src/config"
	tele_interfaces "github.com/Bastien-Antigravity/tele-remote/src/interfaces"

	"github.com/Bastien-Antigravity/universal-logger/src/interfaces"
)

// SafeSocketSubscriber abstracts a safe-socket TCP/Unix listener for Component payloads
type SafeSocketSubscriber struct {
	cfg *config.Config
	log interfaces.Logger
}

func NewSafeSocketSubscriber(c *config.Config, l interfaces.Logger) tele_interfaces.Subscriber {
	return &SafeSocketSubscriber{cfg: c, log: l}
}

func (s *SafeSocketSubscriber) StartListen(ctx context.Context, cbs tele_interfaces.SubscriberCallbacks) error {
	s.log.Info("SafeSocket Subscriber initialized (Skeleton)")

	go func() {
		<-ctx.Done()
		s.Close()
	}()

	return nil
}

func (s *SafeSocketSubscriber) Close() error {
	return nil
}
