package server

import (
	"context"

	"github.com/seal-io/walrus/utils/gopool"

	"github.com/seal-io/hermitcrab/pkg/health"
)

// registerHealthCheckers registers the health checkers into the global health registry.
func (r *Server) registerHealthCheckers(ctx context.Context, opts initOptions) error {
	cs := health.Checkers{
		health.CheckerFunc("gopool", getGoPoolHealthChecker()),
	}

	return health.Register(ctx, cs)
}

func getGoPoolHealthChecker() health.Check {
	return func(_ context.Context) error {
		return gopool.IsHealthy()
	}
}
