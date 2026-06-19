package backend

import (
	"context"

	"github.com/wyrd-company/gelato/pkg/events"
	"github.com/wyrd-company/gelato/pkg/messages"
)

func (d *Backend) publishRepositoryEvent(ctx context.Context, event messages.RepositoryEvent) {
	if err := events.PublishRepositoryEvent(ctx, event); err != nil {
		d.logger.Error("failed to publish repository event", "type", event.Type, "repo", event.Repository, "err", err)
	}
}
