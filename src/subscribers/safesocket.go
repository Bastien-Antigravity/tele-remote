package subscribers

import (
	"context"

	"tele-remote/src/config"
	"tele-remote/src/interfaces"

	flexlogger "github.com/Bastien-Antigravity/flexible-logger/src/interfaces"
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

	go func() {
		<-ctx.Done()
		s.Close()
	}()

	return nil
}

func (s *SafeSocketSubscriber) Close() error {
	return nil
}
