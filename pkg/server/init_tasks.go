package server

import (
	"context"
	"fmt"

	"github.com/seal-io/walrus/utils/cron"

	"github.com/seal-io/hermitcrab/pkg/tasks/provider"
)

// startTasks starts the tasks by Cron Expression to do something periodically in background.
func (r *Server) startTasks(ctx context.Context, opts initOptions) (err error) {
	// Start cron scheduler.
	err = cron.Start(ctx, nil)
	if err != nil {
		return fmt.Errorf("error starting cron scheduler: %w", err)
	}

	// Register tasks.
	err = cron.Schedule(provider.SyncMetadata(ctx, opts.ProviderService))

	return
}
