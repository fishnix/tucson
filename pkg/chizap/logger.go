package chizap

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"os"
	"runtime/debug"
	"strings"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"go.uber.org/zap"
)

// Config is the configuration for logger/recover
type Config struct {
	timeFormat   string
	utc          bool
	customFields []func(c context.Context, r *http.Request) zap.Field
}

// Option is a functional configuration option
type Option func(c *Config)

// Chizap returns http middleware that logs requests using uber-go/zap.
//
// It receives:
//   1. A time package format string (e.g. time.RFC3339).
//   2. A boolean stating whether to use UTC time zone or local.
func Chizap(logger *zap.Logger, timeFormat string, utc bool) func(http.Handler) http.Handler {
	return Logger(logger, WithTimeFormat(timeFormat), WithUTC(utc))
}

// RecoveryWithZap returns http middleware that recovers from any panics
// and logs requests using uber-go/zap. Errors are logged using zap.Error().
// The stack parameter enables/disables output of the stack info.  stack
// info can be very large.
func RecoveryWithZap(logger *zap.Logger, stack bool) func(http.Handler) http.Handler {
	return Recovery(logger, stack)
}

// WithTimeFormat allows optionally passing a time package format string (e.g. time.RFC3339).
// (default time.RFC3339Nano).
func WithTimeFormat(layout string) Option {
	return func(c *Config) {
		c.timeFormat = layout
	}
}

// WithUTC toggles between UTC or local time. (default local).
func WithUTC(b bool) Option {
	return func(c *Config) {
		c.utc = b
	}
}

// WithCustomFields appends optional custom fields to be logged.
func WithCustomFields(fields ...func(c context.Context, r *http.Request) zap.Field) Option {
	return func(c *Config) {
		c.customFields = fields
	}
}

// Logger returns http middleware that logs requests using uber-go/zap.
//
// Default option:
//   1. A time package format string (e.g. time.RFC3339).(default time.RFC3339Nano)
//   2. Use time zone.(e.g. utc time zone).(default local).
//   3. Custom fields.(default nil)
func Logger(logger *zap.Logger, opts ...Option) func(next http.Handler) http.Handler {
	cfg := Config{
		time.RFC3339Nano,
		false,
		nil,
	}

	for _, opt := range opts {
		opt(&cfg)
	}

	return func(next http.Handler) http.Handler {
		fn := func(w http.ResponseWriter, r *http.Request) {
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

			start := time.Now()
			path := r.URL.Path
			query := r.URL.RawQuery

			// TODO context isn't overloaded with a list of errors like
			// with gin.  Should we determine those from the response?
			// if len(c.Errors) > 0 {
			// 	// Append error field if this is an erroneous request.
			// 	for _, e := range c.Errors.Errors() {
			// 		logger.Error(e)
			// 	}
			// } else {

			defer func() {
				end := time.Now()
				if cfg.utc {
					end = end.UTC()
				}

				fields := make([]zap.Field, 0, 8+len(cfg.customFields)) //nolint:gomnd
				fields = append(fields,
					zap.Int("status", ww.Status()),
					zap.String("method", r.Method),
					zap.String("path", path),
					zap.String("query", query),
					zap.String("ip", r.RemoteAddr),
					zap.String("user-agent", r.UserAgent()),
					zap.String("time", end.Format(cfg.timeFormat)),
					zap.Duration("latency", time.Since(start)),
				)

				for _, field := range cfg.customFields {
					fields = append(fields, field(r.Context(), r))
				}

				logger.Info(path, fields...)
			}()

			next.ServeHTTP(ww, r)
		}

		return http.HandlerFunc(fn)
	}
}

// Recovery returns http middleware that recovers from any panics
// and logs requests using uber-go/zap. Errors are logged using zap.Error().
// The stack parameter enables/disables output of the stack info.  stack
// info can be very large.
func Recovery(logger *zap.Logger, stack bool, opts ...Option) func(next http.Handler) http.Handler {
	cfg := Config{
		time.RFC3339Nano,
		false,
		nil,
	}

	for _, opt := range opts {
		opt(&cfg)
	}

	if stack {
		cfg.customFields = append(cfg.customFields, func(c context.Context, r *http.Request) zap.Field {
			return zap.ByteString("stack", debug.Stack())
		})
	}

	return func(next http.Handler) http.Handler {
		fn := func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if err := recover(); err != nil {
					var brokenPipe bool

					// Check for a broken connection, as it is not really a
					// condition that warrants a panic stack trace.
					if ne, ok := err.(*net.OpError); ok {
						if se, ok := ne.Err.(*os.SyscallError); ok {
							if strings.Contains(strings.ToLower(se.Error()), "broken pipe") ||
								strings.Contains(strings.ToLower(se.Error()), "connection reset by peer") {
								brokenPipe = true
							}
						}
					}

					httpRequest, _ := httputil.DumpRequest(r, false)
					if brokenPipe {
						logger.Error(r.URL.Path,
							zap.Any("error", err),
							zap.ByteString("request", httpRequest),
						)

						fmt.Println("broke pipe")

						// If the connection is dead, we can't write a status to it.
						return
					}

					now := time.Now()
					if cfg.utc {
						now = now.UTC()
					}

					fields := make([]zap.Field, 0, 3+len(cfg.customFields)) //nolint:gomnd
					fields = append(fields,
						zap.String("time", now.Format(cfg.timeFormat)),
						zap.Any("error", err),
						zap.ByteString("request", httpRequest),
					)

					for _, field := range cfg.customFields {
						fields = append(fields, field(r.Context(), r))
					}

					logger.Error("[Recovery from panic]", fields...)

					w.WriteHeader(http.StatusInternalServerError)
				}
			}()

			next.ServeHTTP(w, r)
		}

		return http.HandlerFunc(fn)
	}
}
