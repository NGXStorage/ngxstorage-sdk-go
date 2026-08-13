package ngxstorage

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// Config configures a Client. The API key is never logged by the SDK.
type Config struct {
	// Controllers is the list of 1-2 controller IPs (or hostnames).
	Controllers []string
	// APIKey is the bearer token. Never logged.
	APIKey string
	// PoolName is the canonical configured pool (optional; drivers that only
	// query status/hardware may leave it empty).
	PoolName string

	// InsecureSkipVerify defaults to true because NGX appliances use
	// self-signed certificates and most customers have no private CA/DNS.
	// Set false and provide RootCAs to enforce a customer trust chain.
	InsecureSkipVerify bool
	// RootCAs is an optional customer CA bundle. When set, TLS verification
	// uses it (and InsecureSkipVerify should be false).
	RootCAs *x509.CertPool

	// RoundTrippers is an optional middleware chain wrapped around the SDK
	// transport, outermost-first.
	RoundTrippers []RoundTripper

	// Logger is the pluggable logger. Defaults to NopLogger.
	Logger Logger

	// HTTPClient is an optional test/advanced transport override.
	HTTPClient *http.Client

	// Timeout is the per-request timeout. Defaults to 60s.
	Timeout time.Duration
}

// Client is the thread-safe entry point for all NGX API v2 operations. It
// owns the controller selection, bounded 725-busy retry, and transport
// failover. Service accessors return lazily-bound per-resource services.
type Client struct {
	cfg        Config
	apiKey     string
	poolName   string
	logger     Logger
	httpClient *http.Client

	// controller selection state
	controllers []string
	nodeIndex   int
	lastRefresh time.Time
	refreshMu   chan struct{}
	refreshOnce bool
}

// NewClient validates configuration and constructs a Client. It performs no
// network I/O; callers should invoke RefreshController before serving traffic
// when pool-scoped placement matters.
func NewClient(cfg Config) (*Client, error) {
	if len(cfg.Controllers) == 0 {
		return nil, errors.New("ngxstorage: at least one controller is required")
	}
	if len(cfg.Controllers) > 2 {
		return nil, errors.New("ngxstorage: at most 2 controllers supported")
	}
	if cfg.APIKey == "" {
		return nil, errors.New("ngxstorage: api key is required")
	}
	if cfg.Logger == nil {
		cfg.Logger = NopLogger{}
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = defaultTimeout
	}

	client := &Client{
		cfg:         cfg,
		apiKey:      cfg.APIKey,
		poolName:    cfg.PoolName,
		logger:      cfg.Logger,
		controllers: append([]string(nil), cfg.Controllers...),
	}

	if cfg.HTTPClient != nil {
		client.httpClient = cfg.HTTPClient
	} else {
		client.httpClient = newHTTPClient(cfg)
	}
	return client, nil
}

// CurrentController returns the currently selected controller IP.
func (c *Client) CurrentController() string {
	return c.controllers[c.nodeIndex]
}

// PoolName returns the configured canonical pool name.
func (c *Client) PoolName() string {
	return c.poolName
}

// requestController performs one request against the currently selected
// controller. It is separated so transport failover can re-run the request
// after a refresh.
func (c *Client) requestController(ctx context.Context, method, link string, body interface{}) ([]byte, int, error) {
	return sendRequest(ctx, c.httpClient, c.logger, method, link, c.apiKey, body, retryConfig{
		MaxRetries: 6,
		BaseDelay:  10 * time.Second,
		MaxDelay:   30 * time.Second,
	})
}

// request rewrites the endpoint to the current controller and performs the
// request with single transport-failover replay for safe methods.
func (c *Client) request(ctx context.Context, name string, pathVars url.Values, body interface{}) ([]byte, int, error) {
	link := endpointURL(name, c.CurrentController(), pathVars)
	method := endpointMethod(name)

	resp, code, err := c.requestController(ctx, method, link, body)
	if err == nil {
		return resp, code, nil
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return resp, code, ctxErr
	}
	if !IsTransportError(err) {
		return resp, code, err
	}
	// POST mutations are never replayed after an ambiguous transport failure.
	if method == http.MethodPost {
		c.logger.Warnf("ngxstorage: suppressing unsafe transport replay for %s", method)
		return resp, code, err
	}
	// Safe methods (GET/DELETE) replay after a controller refresh.
	if refreshErr := c.RefreshController(ctx); refreshErr != nil {
		c.logger.Warnf("ngxstorage: refresh after transport failure: %v", refreshErr)
		return resp, code, fmt.Errorf("ngxstorage: refresh after transport failure: %v: %w", refreshErr, err)
	}
	link = endpointURL(name, c.CurrentController(), pathVars)
	return c.requestController(ctx, method, link, body)
}

// get is a convenience wrapper for GET requests with no body.
func (c *Client) get(ctx context.Context, name string, pathVars url.Values) ([]byte, error) {
	body, _, err := c.request(ctx, name, pathVars, nil)
	return body, err
}

// mutation performs a POST/PUT/PATCH/DELETE with a body and returns the raw
// response and a success flag.
func (c *Client) mutation(ctx context.Context, name string, pathVars url.Values, body interface{}) ([]byte, error) {
	resp, _, err := c.request(ctx, name, pathVars, body)
	return resp, err
}
