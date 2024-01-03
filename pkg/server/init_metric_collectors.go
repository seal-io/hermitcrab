package server

import (
	"context"

	"github.com/seal-io/walrus/utils/cron"
	"github.com/seal-io/walrus/utils/gopool"

	"github.com/seal-io/hermitcrab/pkg/apis/runtime"
	"github.com/seal-io/hermitcrab/pkg/database"
	"github.com/seal-io/hermitcrab/pkg/metric"
)

// registerMetricCollectors registers the metric collectors into the global metric registry.
func (r *Server) registerMetricCollectors(ctx context.Context, opts initOptions) error {
	cs := metric.Collectors{
		database.NewStatsCollectorWith(opts.BoltDriver),
		gopool.NewStatsCollector(),
		cron.NewStatsCollector(),
		runtime.NewStatsCollector(),
	}

	return metric.Register(ctx, cs)
}
