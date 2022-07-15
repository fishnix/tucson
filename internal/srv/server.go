package srv

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/fishnix/tucson/pkg/chizap"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/jwtauth/v5"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	metrics "github.com/slok/go-http-metrics/metrics/prometheus"
	mm "github.com/slok/go-http-metrics/middleware"
	"github.com/slok/go-http-metrics/middleware/std"
	"go.uber.org/zap"
	"golang.org/x/oauth2"
)

// Server implements the HTTP and scaling server
type Server struct {
	defaultOrigin *Origin
	matchers      []*Matcher
	origins       map[string]*Origin
	debug         bool
	enableOIDC    bool
	listen        string
	logger        *zap.Logger
	oidcProvider  *oidc.Provider
	oauth2Config  oauth2.Config
	signingKey    string
}

// Origin defines a backend
type Origin struct {
	BaseUrl    string            `mapstructure:"url"`
	Insecure   bool              `mapstructure:"insecure"`
	SetHeaders map[string]string `mapstructure:"set_headers"`
	AddHeaders map[string]string `mapstructure:"add_headers"`
	Prefix     string            `mapstructure:"prefix"`
	Oidc       bool              `mapstructure:"oidc"`
	BasicAuth  *BasicAuth        `mapstructure:"basicauth"`
}

type BasicAuth struct {
	Username string `mapstructure:"username"`
	Password string `mapstructure:"password"`
}

// Matcher links a request to an origin
type Matcher struct {
	Path   string `mapstructure:"path"`
	Origin string `mapstructure:"origin"`
}

type Option func(s *Server)

var (
	readTimeout     = 10 * time.Second
	writeTimeout    = 20 * time.Second
	shutdownTimeout = 5 * time.Second

	tokenAuth *jwtauth.JWTAuth
)

func New(opts ...Option) *Server {
	s := &Server{
		logger: zap.NewNop(),
	}

	for _, o := range opts {
		o(s)
	}

	return s
}

// WithDebug sets the debug flag
func WithDebug(d bool) Option {
	return func(s *Server) {
		s.debug = d
	}
}

// WithListen sets the listen
func WithListen(l string) Option {
	return func(s *Server) {
		s.listen = l
	}
}

// WithLogger sets the logger
func WithLogger(l *zap.Logger) Option {
	return func(s *Server) {
		s.logger = l
	}
}

// WithOrigins sets the map of backends
func WithOrigins(o map[string]*Origin) Option {
	return func(s *Server) {
		s.origins = o
	}
}

// WithDefaultOrigin sets the default origin to use if there's no match
func WithDefaultOrigin(o *Origin) Option {
	return func(s *Server) {
		s.defaultOrigin = o
	}
}

// WithMatchers sets the slice of matchers
func WithMatchers(m []*Matcher) Option {
	return func(s *Server) {
		s.matchers = m
	}
}

// WithSigningKey sets the JWT signing key
func WithSigningKey(k string) Option {
	return func(s *Server) {
		s.signingKey = k
	}
}

// WithOidcProvider sets the OIDC provider
func WithOidcProvider(p *oidc.Provider) Option {
	return func(s *Server) {
		s.oidcProvider = p
	}
}

// WithOauth2Config sets the config for oauth2
func WithOauth2Config(c oauth2.Config) Option {
	return func(s *Server) {
		s.oauth2Config = c
	}
}

// setup sets up the router, middlewares and routes
func (s *Server) setup() *chi.Mux {
	r := chi.NewRouter()

	// avoid registering on the global prom registry
	reg := prometheus.NewRegistry()
	reg.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)

	// metrics middleware
	r.Use(std.HandlerProvider("", mm.New(mm.Config{
		Recorder: metrics.NewRecorder(metrics.Config{
			Registry: reg,
			Prefix:   "tucson",
		}),
	})))

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(chizap.Logger(s.logger.With(zap.String("component", "srv")),
		chizap.WithTimeFormat(time.RFC3339),
		chizap.WithUTC(true),
	))
	r.Use(chizap.RecoveryWithZap(s.logger.With(zap.String("component", "httpsrv")), true))

	// metrics endpoint
	r.Method(http.MethodGet, "/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))

	// health endpoints
	r.Get("/healthz", s.livenessCheck)
	r.Get("/healthz/liveness", s.livenessCheck)
	r.Get("/healthz/readiness", s.readinessCheck)

	r.Get("/auth/login", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, s.oauth2Config.AuthCodeURL("foobar"), http.StatusFound)
	})

	r.Get("/auth/callback", s.handleOAuth2Callback)

	tokenAuth := jwtauth.New("HS256", []byte(s.signingKey), nil)

	for _, m := range s.matchers {
		r.Group(func(r chi.Router) {
			origin, ok := s.origins[m.Origin]
			if !ok {
				s.logger.Warn("origin not found for matcher", zap.String("origin", m.Origin), zap.Any("matcher", m))
				return
			}

			if origin.Oidc {
				r.Use(s.Authenticator(tokenAuth))
			}

			// TODO handle more than GET
			r.Get(m.Path, s.proxyOriginHandler(origin))
			r.Post(m.Path, s.proxyOriginHandler(origin))
			r.Put(m.Path, s.proxyOriginHandler(origin))
			r.Patch(m.Path, s.proxyOriginHandler(origin))
			r.Delete(m.Path, s.proxyOriginHandler(origin))
		})
	}

	// Default Backend Routes
	r.Group(func(r chi.Router) {
		if s.defaultOrigin.Oidc {
			r.Use(s.Authenticator(tokenAuth))
		}

		r.NotFound(s.proxyOriginHandler(s.defaultOrigin))
	})

	return r
}

// NewServer returns a configured server
func (s *Server) NewServer() *http.Server {
	return &http.Server{
		Handler:      s.setup(),
		Addr:         s.listen,
		ReadTimeout:  readTimeout,
		WriteTimeout: writeTimeout,
	}
}

// Run starts the scaler and the http server
func (s *Server) Run(ctx context.Context) error {
	var wg sync.WaitGroup
	httpsrv := s.NewServer()

	go func() {
		if err := httpsrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			panic(err)
		}
	}()

	<-ctx.Done()

	ctxShutDown, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer func() {
		cancel()
	}()

	if err := httpsrv.Shutdown(ctxShutDown); err != nil {
		return err
	}

	// wait for scaler to shutdown
	wg.Wait()

	s.logger.Info("server shutdown cleanly", zap.String("time", time.Now().UTC().Format(time.RFC3339)))

	return nil
}
