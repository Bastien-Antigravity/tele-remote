package publishers

import (
	"context"

	"tele-remote/src/interfaces"
	"tele-remote/src/serializers"

	// "github.com/Bastien-Antigravity/safe-socket/pkg/safesocket"
)

// SafeSocketPublisher defines the contract for sending commands over the safe-socket library
type SafeSocketPublisher struct {
	// ss *safesocket.SafeSocket
	serializer interfaces.ISerializer
}

func NewSafeSocketPublisher(ser *serializers.BinSerializer /*, ss *safesocket.SafeSocket */) interfaces.Publisher {
	return &SafeSocketPublisher{
		// ss: ss,
		serializer: ser,
	}
}

func (p *SafeSocketPublisher) PublishCommand(ctx context.Context, cmdType int32, payload string) error {
	// 1. Construct map or struct
	// 2. data, err := p.serializer.Marshal(obj)
	// 3. return p.ss.Send(data)
	return nil
}

func (p *SafeSocketPublisher) Close() error {
	return nil
}
