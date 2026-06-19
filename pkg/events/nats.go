package events

import (
	"context"
	"fmt"

	nats "github.com/nats-io/nats.go"
	"github.com/wyrd-company/gelato/pkg/config"
	"github.com/wyrd-company/gelato/pkg/messages"
)

type NATSPublisher struct {
	conn   *nats.Conn
	prefix string
}

func NewNATSPublisher(cfg *config.Config) (*NATSPublisher, error) {
	if cfg == nil || !cfg.NATS.Enabled {
		return nil, nil
	}

	conn, err := nats.Connect(cfg.NATS.URL, nats.Name("gelato-events"))
	if err != nil {
		return nil, fmt.Errorf("connect to NATS: %w", err)
	}

	return &NATSPublisher{
		conn:   conn,
		prefix: cfg.NATS.SubjectPrefix,
	}, nil
}

func (p *NATSPublisher) PublishRepositoryEvent(_ context.Context, event messages.RepositoryEvent) error {
	if p == nil || p.conn == nil {
		return nil
	}

	data, err := messages.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}

	for _, subject := range messages.EventSubjects(p.prefix, event.Type, event.Repository) {
		msg := &nats.Msg{
			Subject: subject,
			Data:    data,
			Header:  nats.Header{},
		}
		msg.Header.Set(messages.HeaderContentType, messages.MediaTypeMessagePack)
		if err := p.conn.PublishMsg(msg); err != nil {
			return fmt.Errorf("publish event to %s: %w", subject, err)
		}
	}

	return nil
}

func (p *NATSPublisher) Close() error {
	if p == nil || p.conn == nil {
		return nil
	}

	p.conn.Drain() //nolint:errcheck
	p.conn.Close()
	return nil
}
