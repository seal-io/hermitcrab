package apis

import (
	"context"
	"net/http"
	"time"

	"github.com/seal-io/hermitcrab/pkg/apis/debug"
	"github.com/seal-io/hermitcrab/pkg/apis/measure"
	providerapis "github.com/seal-io/hermitcrab/pkg/apis/provider"
	"github.com/seal-io/hermitcrab/pkg/apis/runtime"
	"github.com/seal-io/hermitcrab/pkg/provider"
)

type SetupOptions struct {
	// Configure from launching.
	ConnQPS               int
	ConnBurst             int
	WebsocketConnMaxPerIP int
	// Derived from configuration.
	ProviderService *provider.Service
	TlsCertified    bool
}

func (s *Server) Setup(ctx context.Context, opts SetupOptions) (http.Handler, error) {
	// Prepare middlewares.
	throttler := runtime.RequestThrottling(opts.ConnQPS, opts.ConnBurst)
	wsCounter := runtime.If(
		// Validate websocket connection.
		runtime.IsBidiStreamRequest,
		// Maximum 10 connection per ip.
		runtime.PerIP(func() runtime.Handle {
			return runtime.RequestCounting(opts.WebsocketConnMaxPerIP, 5*time.Second)
		}),
	)

	// Initial router.
	apisOpts := []runtime.RouterOption{
		runtime.WithDefaultWriter(s.logger),
		runtime.SkipLoggingPaths(
			"/",
			"/readyz",
			"/livez",
			"/metrics",
			"/debug/version"),
		runtime.ExposeOpenAPI(),
	}

	apis := runtime.NewRouter(apisOpts...)

	rootApis := apis.Group("/v1").
		Use(throttler, wsCounter)
	{
		r := rootApis
		r.Group("/providers").
			Routes(providerapis.Handle(opts.ProviderService))
	}

	measureApis := apis.Group("").
		Use(throttler)
	{
		r := measureApis
		r.Get("/readyz", measure.Readyz())
		r.Get("/livez", measure.Livez())
		r.Get("/metrics", measure.Metrics())
	}

	debugApis := apis.Group("/debug").
		Use(throttler)
	{
		r := debugApis
		r.Get("/version", debug.Version())
		r.Get("/flags", debug.GetFlags())
		r.Group("").
			Use(runtime.OnlyLocalIP()).
			Get("/pprof/*any", debug.PProf()).
			Put("/flags", debug.SetFlags())
	}

	return apis, nil
}
