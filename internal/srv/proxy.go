package srv

import (
	"context"
	"crypto/tls"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

var (
	sanitizeHeaders = []string{"www-authenticate", "server"}
)

type proxy struct {
	origin *Origin
	logger *zap.Logger
}

func (s *Server) newProxy(origin *Origin, logger *zap.Logger) *proxy {
	return &proxy{
		origin: origin,
		logger: logger,
	}
}

// proxyRequest proxies requests to a given backend
func (p *proxy) proxyRequest(w http.ResponseWriter, r *http.Request) {
	requestID, _ := uuid.NewUUID()
	ctx := context.WithValue(r.Context(), "requestID", requestID.String())

	logger := p.logger.With(
		zap.String("req.url", r.URL.String()),
		zap.String("http.method", r.Method),
		zap.String("request.id", requestID.String()),
	)

	u, err := url.Parse(p.origin.BaseUrl + r.URL.String())
	if err != nil {
		logger.Error("failed to parse backend url", zap.Error(err))
	}

	logger.Debug("proxying request", zap.String("backend.url", u.String()))

	req, err := http.NewRequestWithContext(ctx, r.Method, u.String(), r.Body)
	if err != nil {
		logger.Error("failed to generate backend request", zap.Error(err))
	}

	// clone headers from request
	// TODO sanitize headers for backend
	req.Header = r.Header.Clone()

	// override headers
	for k, v := range p.origin.SetHeaders {
		req.Header.Set(k, v)
	}

	// append headers
	for k, v := range p.origin.AddHeaders {
		req.Header.Add(k, v)
	}

	if p.origin.BasicAuth != nil {
		logger.Debug("setting basic auth")
		req.SetBasicAuth(p.origin.BasicAuth.Username, p.origin.BasicAuth.Password)
	}

	req.Header.Set("X-Forwarded-For", r.RemoteAddr)
	req.Header.Set("X-Forwarded-Proto", r.Proto)

	tr := &http.Transport{}
	if p.origin.Insecure {
		tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}

	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
		Timeout:   120 * time.Second,
		Transport: tr,
	}

	resp, err := client.Do(req)
	if err != nil {
		logger.Warn("failed to proxy request to backend", zap.Error(err))

		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte("backend unavailable"))

		return
	}
	defer resp.Body.Close()

	p.cloneHeaders(ctx, w, resp)

	logger.Debug("returning response code", zap.Int("code", resp.StatusCode))

	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

func (p *proxy) cloneHeaders(ctx context.Context, w http.ResponseWriter, resp *http.Response) {
	requestID, ok := ctx.Value("requestID").(string)
	if !ok {
		requestID = ""
	}

	logger := p.logger.With(
		zap.String("request.id", requestID),
	)

	for key, values := range resp.Header {
		for _, v := range values {
			var skip bool
			for _, k := range sanitizeHeaders {
				if strings.ToLower(k) == strings.ToLower(key) {
					skip = true
				}
			}

			if skip {
				logger.Debug("sanitizing header", zap.String("key", key), zap.String("value", v))
				continue
			}

			logger.Debug("cloning header", zap.String("key", key), zap.String("value", v))
			w.Header().Set(key, v)
		}
	}
}
