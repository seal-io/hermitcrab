package server

import (
	"context"
	"fmt"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"

	"github.com/seal-io/walrus/utils/strs"

	"github.com/seal-io/hermitcrab/pkg/database"
	"github.com/seal-io/hermitcrab/pkg/provider"
)

type initOptions struct {
	ProviderService *provider.Service
	SkipTLSVerify   bool
	BoltDriver      database.BoltDriver
}

func (r *Server) init(ctx context.Context, opts initOptions) error {
	// Initialize data for system.
	inits := []initiation{
		r.registerHealthCheckers,
		r.registerMetricCollectors,
		r.startTasks,
	}

	for i := range inits {
		if err := inits[i](ctx, opts); err != nil {
			return fmt.Errorf("failed to %s: %w",
				loadInitiationName(inits[i]), err)
		}
	}

	return nil
}

type initiation func(context.Context, initOptions) error

func loadInitiationName(i initiation) string {
	n := runtime.FuncForPC(reflect.ValueOf(i).Pointer()).Name()
	n = strings.TrimPrefix(strings.TrimSuffix(filepath.Ext(n), "-fm"), ".")

	return strs.Decamelize(n, true)
}
