package requestdecompressor

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"strings"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
	"github.com/caddyserver/caddy/v2/caddyconfig/httpcaddyfile"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
	"github.com/klauspost/compress/brotli"
	"github.com/klauspost/compress/zstd"
	"go.uber.org/zap"
)

func init() {
	caddy.RegisterModule(Middleware{})
	httpcaddyfile.RegisterHandlerDirective("request_decompress", parseCaddyfile)
}

// Middleware implements an HTTP handler that decompresses request bodies
type Middleware struct {
	logger *zap.Logger
	next   http.Handler
	metrics *DecompressionMetrics
}

// DecompressionMetrics tracks various metrics about decompression operations
type DecompressionMetrics struct {
	TotalRequests         caddy.Counter
	SuccessfulRequests    caddy.Counter
	FailedRequests        caddy.Counter
	DecompressionTimings  caddy.Float64Counter
	RequestsByCompression map[string]caddy.Counter
}

// CaddyModule returns the Caddy module information.
func (Middleware) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "http.handlers.request_decompress",
		New: func() caddy.Module { return new(Middleware) },
	}
}

// Provision implements caddy.Provisioner.
func (m *Middleware) Provision(ctx caddy.Context) error {
	m.logger = ctx.Logger()
	m.metrics = &DecompressionMetrics{
		TotalRequests:         caddy.NewCounter(),
		SuccessfulRequests:    caddy.NewCounter(),
		FailedRequests:        caddy.NewCounter(),
		DecompressionTimings:  caddy.NewFloat64Counter(),
		RequestsByCompression: make(map[string]caddy.Counter),
	}
	return nil
}

// Validate implements caddy.Validator.
func (m *Middleware) Validate() error {
	return nil
}

// ServeHTTP implements caddyhttp.MiddlewareHandler.
func (m *Middleware) ServeHTTP(w http.ResponseWriter, r *http.Request) error {
	if r.Header.Get("Content-Encoding") == "" {
		return m.next.ServeHTTP(w, r)
	}

	m.metrics.TotalRequests.Add(1)

	encoding := strings.ToLower(r.Header.Get("Content-Encoding"))
	if _, exists := m.metrics.RequestsByCompression[encoding]; !exists {
		m.metrics.RequestsByCompression[encoding] = caddy.NewCounter()
	}
	m.metrics.RequestsByCompression[encoding].Add(1)

	body, err := io.ReadAll(r.Body)
	if err != nil {
		m.metrics.FailedRequests.Add(1)
		return caddyhttp.Error(http.StatusBadRequest, err)
	}

	var decompressed []byte
	switch encoding {
	case "gzip":
		reader, err := gzip.NewReader(bytes.NewReader(body))
		if err != nil {
			m.metrics.FailedRequests.Add(1)
			return caddyhttp.Error(http.StatusBadRequest, err)
		}
		decompressed, err = io.ReadAll(reader)
		reader.Close()

	case "br":
		reader := brotli.NewReader(bytes.NewReader(body))
		decompressed, err = io.ReadAll(reader)

	case "zstd":
		decoder, err := zstd.NewReader(bytes.NewReader(body))
		if err != nil {
			m.metrics.FailedRequests.Add(1)
			return caddyhttp.Error(http.StatusBadRequest, err)
		}
		decompressed, err = io.ReadAll(decoder)
		decoder.Close()

	default:
		m.metrics.FailedRequests.Add(1)
		return caddyhttp.Error(http.StatusBadRequest, 
			fmt.Errorf("unsupported Content-Encoding: %s", encoding))
	}

	if err != nil {
		m.metrics.FailedRequests.Add(1)
		return caddyhttp.Error(http.StatusBadRequest, err)
	}

	m.metrics.SuccessfulRequests.Add(1)
	r.Body = io.NopCloser(bytes.NewReader(decompressed))
	r.Header.Del("Content-Encoding")
	r.ContentLength = int64(len(decompressed))

	return m.next.ServeHTTP(w, r)
}

// UnmarshalCaddyfile implements caddyfile.Unmarshaler.
func (m *Middleware) UnmarshalCaddyfile(d *caddyfile.Dispenser) error {
	return nil
}

// parseCaddyfile parses the request_decompress directive
func parseCaddyfile(h httpcaddyfile.Helper) (caddyhttp.MiddlewareHandler, error) {
	var m Middleware
	return &m, nil
}
