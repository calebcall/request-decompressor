package request_decompressor

import (
	"bytes"
	"compress/bzip2"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
	"github.com/caddyserver/caddy/v2/caddyconfig/httpcaddyfile"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
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
	metrics *DecompressionMetrics
}

// DecompressionMetrics tracks various metrics about decompression operations
type DecompressionMetrics struct {
	TotalRequests         int64
	SuccessfulRequests    int64
	FailedRequests        int64
	DecompressionTimings  float64
	RequestsByCompression map[string]*int64
}

// CaddyModule returns the Caddy module information.
func (Middleware) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "http.handlers.request_decompressor",
		New: func() caddy.Module { return new(Middleware) },
	}
}

// Provision implements caddy.Provisioner.
func (m *Middleware) Provision(ctx caddy.Context) error {
	m.logger = ctx.Logger()
	m.metrics = &DecompressionMetrics{
		RequestsByCompression: make(map[string]*int64),
	}
	return nil
}

// Validate implements caddy.Validator.
func (m *Middleware) Validate() error {
	return nil
}

// ServeHTTP implements caddyhttp.MiddlewareHandler.
func (m *Middleware) ServeHTTP(w http.ResponseWriter, r *http.Request, next caddyhttp.Handler) error {
	if r.Header.Get("Content-Encoding") == "" {
		return next.ServeHTTP(w, r)
	}

	atomic.AddInt64(&m.metrics.TotalRequests, 1)

	encoding := strings.ToLower(r.Header.Get("Content-Encoding"))
	if _, exists := m.metrics.RequestsByCompression[encoding]; !exists {
		m.metrics.RequestsByCompression[encoding] = new(int64)
	}
	atomic.AddInt64(m.metrics.RequestsByCompression[encoding], 1)

	body, err := io.ReadAll(r.Body)
	if err != nil {
		atomic.AddInt64(&m.metrics.FailedRequests, 1)
		return caddyhttp.Error(http.StatusBadRequest, err)
	}

	var decompressed []byte
	switch encoding {
	case "gzip":
		reader, err := gzip.NewReader(bytes.NewReader(body))
		if err != nil {
			atomic.AddInt64(&m.metrics.FailedRequests, 1)
			return caddyhttp.Error(http.StatusBadRequest, err)
		}
		decompressed, err = io.ReadAll(reader)
		reader.Close()

	case "bz2":
		reader := bzip2.NewReader(bytes.NewReader(body))
		decompressed, err = io.ReadAll(reader)

	case "zstd":
		decoder, err := zstd.NewReader(bytes.NewReader(body))
		if err != nil {
			atomic.AddInt64(&m.metrics.FailedRequests, 1)
			return caddyhttp.Error(http.StatusBadRequest, err)
		}
		decompressed, err = io.ReadAll(decoder)
		decoder.Close()

	default:
		atomic.AddInt64(&m.metrics.FailedRequests, 1)
		return caddyhttp.Error(http.StatusBadRequest, 
			fmt.Errorf("unsupported Content-Encoding: %s", encoding))
	}

	if err != nil {
		atomic.AddInt64(&m.metrics.FailedRequests, 1)
		return caddyhttp.Error(http.StatusBadRequest, err)
	}

	atomic.AddInt64(&m.metrics.SuccessfulRequests, 1)
	r.Body = io.NopCloser(bytes.NewReader(decompressed))
	r.Header.Del("Content-Encoding")
	r.ContentLength = int64(len(decompressed))

	return next.ServeHTTP(w, r)
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
