package events

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/wyrd-company/gelato/pkg/messages"
	"github.com/wyrd-company/gelato/pkg/proto"
)

type contextKey string

const (
	publisherContextKey contextKey = "gelato-events-publisher"
	sourceContextKey    contextKey = "gelato-repository-source"
)

func ContextKey() interface{} {
	return publisherContextKey
}

// Publisher publishes Gelato repository events.
type Publisher interface {
	PublishRepositoryEvent(context.Context, messages.RepositoryEvent) error
	Close() error
}

type noopPublisher struct{}

func (noopPublisher) PublishRepositoryEvent(context.Context, messages.RepositoryEvent) error {
	return nil
}
func (noopPublisher) Close() error { return nil }

// RepositorySource records import/mirror origin details for repository-added events.
type RepositorySource struct {
	Remote string
	Mirror bool
}

func WithPublisher(ctx context.Context, publisher Publisher) context.Context {
	if publisher == nil {
		publisher = noopPublisher{}
	}
	return context.WithValue(ctx, publisherContextKey, publisher)
}

func FromContext(ctx context.Context) Publisher {
	if publisher, ok := ctx.Value(publisherContextKey).(Publisher); ok && publisher != nil {
		return publisher
	}
	return noopPublisher{}
}

func WithRepositorySource(ctx context.Context, source RepositorySource) context.Context {
	return context.WithValue(ctx, sourceContextKey, source)
}

func RepositorySourceFromContext(ctx context.Context) (RepositorySource, bool) {
	source, ok := ctx.Value(sourceContextKey).(RepositorySource)
	return source, ok
}

func PublishRepositoryEvent(ctx context.Context, event messages.RepositoryEvent) error {
	if event.ID == "" {
		event.ID = uuid.NewString()
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	}
	if event.Actor == "" {
		if user := proto.UserFromContext(ctx); user != nil {
			event.Actor = user.Username()
		}
	}
	return FromContext(ctx).PublishRepositoryEvent(ctx, event)
}
