package publishers

import (
	"context"

	"github.com/nats-io/nats.go"
	"github.com/Bastien-Antigravity/microservice-toolbox/go/pkg/serializers"
	"github.com/Bastien-Antigravity/tele-remote/src/interfaces"
)

// NatsPublisher uses NATS core to publish commands as JSON objects to components
type NatsPublisher struct {
	nc         *nats.Conn
	subject    string
	serializer serializers.Serializer
}

func NewNatsPublisher(nc *nats.Conn, subject string, ser serializers.Serializer) interfaces.Publisher {
	return &NatsPublisher{nc: nc, subject: subject, serializer: ser}
}

func (p *NatsPublisher) PublishCommand(ctx context.Context, cmdType int32, payload string) error {
	cmdMap := map[string]interface{}{
		"command_type":   cmdType,
		"custom_payload": payload,
	}
	b, err := p.serializer.Marshal(cmdMap)
	if err != nil {
		return err
	}
	return p.nc.Publish(p.subject, b)
}

func (p *NatsPublisher) Close() error {
	return nil
}
