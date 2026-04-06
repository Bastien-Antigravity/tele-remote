package subscribers

import (
	"context"

	"tele-remote/src/config"
	"tele-remote/src/interfaces"

	unilogger "github.com/Bastien-Antigravity/universal-logger/src/logger"
)

// SafeSocketSubscriber abstracts a safe-socket TCP/Unix listener for Component payloads
type SafeSocketSubscriber struct {
	cfg *config.Config
	log *unilogger.UniLog
}

func NewSafeSocketSubscriber(c *config.Config, l *unilogger.UniLog) interfaces.Subscriber {
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
