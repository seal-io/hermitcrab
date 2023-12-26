package server

import (
	"context"
	"errors"
	"flag"
	"fmt"
	stdlog "log"
	"net"
	"os"
	"path/filepath"
	"strconv"

	"github.com/seal-io/walrus/utils/clis"
	"github.com/seal-io/walrus/utils/files"
	"github.com/seal-io/walrus/utils/gopool"
	"github.com/seal-io/walrus/utils/log"
	"github.com/seal-io/walrus/utils/runtimex"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/klog/v2"

	"github.com/seal-io/hermitcrab/pkg/consts"
	"github.com/seal-io/hermitcrab/pkg/database"
	"github.com/seal-io/hermitcrab/pkg/provider"
)

type Server struct {
	Logger clis.Logger

	BindAddress           string
	BindWithDualStack     bool
	EnableTls             bool
	TlsCertFile           string
	TlsPrivateKeyFile     string
	TlsCertDir            string
	TlsAutoCertDomains    []string
	ConnQPS               int
	ConnBurst             int
	WebsocketConnMaxPerIP int
	GopoolWorkerFactor    int

	DataSourceDir string
}

func New() *Server {
	return &Server{
		BindAddress:           "0.0.0.0",
		BindWithDualStack:     true,
		EnableTls:             true,
		TlsCertDir:            filepath.Join(consts.DataDir, "tls"),
		ConnQPS:               100,
		ConnBurst:             200,
		WebsocketConnMaxPerIP: 25,
		GopoolWorkerFactor:    100,

		DataSourceDir: filepath.Join(consts.DataDir, "data"),
	}
}

func (r *Server) Flags(cmd *cli.Command) {
	flags := [...]cli.Flag{
		&cli.StringFlag{
			Name:        "bind-address",
			Usage:       "The IP address on which to listen.",
			Destination: &r.BindAddress,
			Value:       r.BindAddress,
			Action: func(c *cli.Context, s string) error {
				if s != "" && net.ParseIP(s) == nil {
					return errors.New("--bind-address: invalid IP address")
				}
				return nil
			},
		},
		&cli.BoolFlag{
			Name:        "bind-with-dual-stack",
			Usage:       "Enable dual stack socket listening.",
			Destination: &r.BindWithDualStack,
			Value:       r.BindWithDualStack,
		},
		&cli.BoolFlag{
			Name:        "enable-tls",
			Usage:       "Enable HTTPs.",
			Destination: &r.EnableTls,
			Value:       r.EnableTls,
		},
		&cli.StringFlag{
			Name: "tls-cert-file",
			Usage: "The file containing the default x509 certificate for HTTPS. " +
				"If any CA certs, concatenated after server cert file. ",
			Destination: &r.TlsCertFile,
			Value:       r.TlsCertFile,
			Action: func(c *cli.Context, s string) error {
				if s != "" &&
					!files.Exists(s) {
					return errors.New("--tls-cert-file: file is not existed")
				}
				return nil
			},
		},
		&cli.StringFlag{
			Name:        "tls-private-key-file",
			Usage:       "The file containing the default x509 private key matching --tls-cert-file.",
			Destination: &r.TlsPrivateKeyFile,
			Value:       r.TlsPrivateKeyFile,
			Action: func(c *cli.Context, s string) error {
				if s != "" &&
					!files.Exists(s) {
					return errors.New("--tls-private-key-file: file is not existed")
				}
				return nil
			},
		},
		&cli.StringFlag{
			Name: "tls-cert-dir",
			Usage: "The directory where the TLS certs are located. " +
				"If --tls-cert-file and --tls-private-key-file are provided, this flag will be ignored. " +
				"If --tls-cert-file and --tls-private-key-file are not provided, " +
				"the certificate and key of auto-signed or self-signed are saved to where this flag specified. ",
			Destination: &r.TlsCertDir,
			Value:       r.TlsCertDir,
			Action: func(c *cli.Context, s string) error {
				if c.String("tls-cert-file") != "" && c.String("tls-private-key-file") != "" {
					return nil
				}

				if s == "" {
					return errors.New(
						"--tls-cert-dir: must be filled if --tls-cert-file and --tls-private-key-file are not provided")
				}

				if !filepath.IsAbs(s) {
					return errors.New("--tls-cert-dir: must be absolute path")
				}

				return nil
			},
		},
		&cli.StringSliceFlag{
			Name: "tls-auto-cert-domains",
			Usage: "The domains to accept ACME HTTP-01 or TLS-ALPN-01 challenge to " +
				"generate HTTPS x509 certificate and private key, " +
				"and saved to the directory specified by --tls-cert-dir. " +
				"If --tls-cert-file and --tls-key-file are provided, this flag will be ignored.",
			Action: func(c *cli.Context, v []string) error {
				f := field.NewPath("--tls-auto-cert-domains")
				for i := range v {
					if err := validation.IsFullyQualifiedDomainName(f, v[i]).ToAggregate(); err != nil {
						return err
					}
				}
				if len(v) != 0 &&
					(c.String("tls-cert-dir") == "" &&
						(c.String("tls-cert-file") == "" || c.String("tls-private-key-file") == "")) {
					return errors.New("--tls-cert-dir: must be filled")
				}
				r.TlsAutoCertDomains = v
				return nil
			},
			Value: cli.NewStringSlice(r.TlsAutoCertDomains...),
		},
		&cli.IntFlag{
			Name:        "conn-qps",
			Usage:       "The qps(maximum average number per second) when dialing the server.",
			Destination: &r.ConnQPS,
			Value:       r.ConnQPS,
		},
		&cli.IntFlag{
			Name:        "conn-burst",
			Usage:       "The burst(maximum number at the same moment) when dialing the server.",
			Destination: &r.ConnBurst,
			Value:       r.ConnBurst,
		},
		&cli.IntFlag{
			Name:        "websocket-conn-max-per-ip",
			Usage:       "The maximum number of websocket connections per IP.",
			Destination: &r.WebsocketConnMaxPerIP,
			Value:       r.WebsocketConnMaxPerIP,
		},
		&cli.IntFlag{
			Name: "gopool-worker-factor",
			Usage: "The gopool worker factor determines the number of tasks of the goroutine worker pool," +
				"it is calculated by the number of CPU cores multiplied by this factor.",
			Action: func(c *cli.Context, i int) error {
				if i < 100 {
					return errors.New("too small --gopool-worker-factor: must be greater than 100")
				}
				return nil
			},
			Destination: &r.GopoolWorkerFactor,
			Value:       r.GopoolWorkerFactor,
		},
		&cli.StringFlag{
			Name:  "data-source-dir",
			Usage: "The directory where the data are stored.",
			Action: func(c *cli.Context, s string) error {
				if s == "" {
					return errors.New("--data-source-dir: must be filled")
				}

				if !filepath.IsAbs(s) {
					return errors.New("--data-source-dir: must be absolute path")
				}

				return nil
			},
			Destination: &r.DataSourceDir,
			Value:       r.DataSourceDir,
		},
	}
	for i := range flags {
		cmd.Flags = append(cmd.Flags, flags[i])
	}

	r.Logger.Flags(cmd)
}

func (r *Server) Before(cmd *cli.Command) {
	pb := cmd.Before
	cmd.Before = func(c *cli.Context) error {
		l := log.GetLogger()

		// Sink the output of standard logger to util logger.
		stdlog.SetOutput(l)

		// Turn on the logrus logger
		// and sink the output to util logger.
		logrus.SetLevel(logrus.TraceLevel)
		logrus.SetFormatter(log.AsLogrusFormatter(l))

		// Turn on klog logger according to the verbosity,
		// and sink the output to util logger.
		{
			var flags flag.FlagSet

			klog.InitFlags(&flags)
			_ = flags.Set("v", strconv.FormatUint(log.GetVerbosity(), 10))
			_ = flags.Set("skip_headers", "true")
		}
		klog.SetLogger(log.AsLogr(l))

		if pb != nil {
			return pb(c)
		}

		// Init set GOMAXPROCS.
		runtimex.Init()

		return nil
	}

	r.Logger.Before(cmd)
}

func (r *Server) Action(cmd *cli.Command) {
	cmd.Action = func(c *cli.Context) error {
		return r.Run(c.Context)
	}
}

func (r *Server) Run(c context.Context) error {
	if err := r.configure(); err != nil {
		return fmt.Errorf("error configuring: %w", err)
	}

	g, ctx := gopool.GroupWithContext(c)

	// Load database driver.
	var bolt database.Bolt

	g.Go(func() error {
		log.Info("running database")

		err := bolt.Run(ctx, r.DataSourceDir)
		if err != nil {
			log.Errorf("error running database: %v", err)
		}

		return err
	})

	// Create service clients.
	boltDriver := bolt.GetDriver()

	providerService, err := provider.NewService(boltDriver, r.DataSourceDir)
	if err != nil {
		return fmt.Errorf("error creating provider service: %w", err)
	}

	// Initialize some resources.
	log.Info("initializing")

	initOpts := initOptions{
		ProviderService: providerService,
		SkipTLSVerify:   len(r.TlsAutoCertDomains) != 0,
	}

	if err := r.init(ctx, initOpts); err != nil {
		log.Errorf("error initializing: %v", err)
		return fmt.Errorf("error initializing: %w", err)
	}

	// Run apis.
	startApisOpts := startApisOptions{
		ProviderService: providerService,
	}

	g.Go(func() error {
		log.Info("starting apis")

		err := r.startApis(ctx, startApisOpts)
		if err != nil {
			log.Errorf("error starting apis: %v", err)
		}

		return err
	})

	return g.Wait()
}

func (r *Server) configure() error {
	// Configure gopool.
	gopool.Reset(r.GopoolWorkerFactor)

	// Configure data source dir.
	if err := os.MkdirAll(r.DataSourceDir, 0o700); err != nil {
		if !os.IsExist(err) {
			return fmt.Errorf("--data-source-dir: %w", err)
		}

		i, _ := os.Stat(r.DataSourceDir)
		if !i.IsDir() {
			return errors.New("--data-source-dir: not directory")
		}
	}

	return nil
}
