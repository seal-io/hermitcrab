package provider

import (
	"context"

	"github.com/seal-io/walrus/utils/cron"

	"github.com/seal-io/hermitcrab/pkg/provider"
)

// SyncMetadata creates a Cron task to sync the metadata from remote to local 30 minutes.
func SyncMetadata(_ context.Context, providerService *provider.Service) (name string, expr cron.Expr, task cron.Task) {
	name = "tasks.provider.sync_metadata"
	expr = cron.ImmediateExpr("0 */30 * ? * *")
	task = cron.TaskFunc(func(ctx context.Context, args ...any) error {
		return providerService.Metadata.Sync(ctx)
	})

	return
}
