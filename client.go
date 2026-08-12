package ngxsdk

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

const defaultTimeout = 60 * time.Second

// RoundTripper is the middleware hook drivers use to wrap SDK HTTP requests.
// It matches net/http's http.RoundTripper so standard middleware (tracing,
// metrics, custom retry) composes naturally.
type RoundTripper interface {
	RoundTrip(*http.Request) (*http.Response, error)
}

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
	stateMu     chan struct{} // one-slot refresh gate via select
	lastRefresh time.Time
	refreshMu   chan struct{}
	refreshOnce bool
}

// NewClient validates configuration and constructs a Client. It performs no
// network I/O; callers should invoke RefreshController before serving traffic
// when pool-scoped placement matters.
func NewClient(cfg Config) (*Client, error) {
	if len(cfg.Controllers) == 0 {
		return nil, errors.New("ngx sdk: at least one controller is required")
	}
	if len(cfg.Controllers) > 2 {
		return nil, errors.New("ngx sdk: at most 2 controllers supported")
	}
	if cfg.APIKey == "" {
		return nil, errors.New("ngx sdk: api key is required")
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

// newHTTPClient builds the SDK HTTP transport with TLS and middleware wiring.
func newHTTPClient(cfg Config) *http.Client {
	tlsCfg := &tls.Config{
		InsecureSkipVerify: cfg.InsecureSkipVerify, //nolint:gosec // self-signed NGX appliances
	}
	if cfg.RootCAs != nil {
		tlsCfg.RootCAs = cfg.RootCAs
	}

	transport := &http.Transport{TLSClientConfig: tlsCfg}

	var rt RoundTripper = transport
	// Wrap middleware outermost-first: iterate in reverse so cfg order runs
	// outer-to-inner.
	for i := len(cfg.RoundTrippers) - 1; i >= 0; i-- {
		inner := rt
		outer := cfg.RoundTrippers[i]
		rt = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			return outer.RoundTrip(withInner(req, inner))
		})
	}

	return &http.Client{Transport: rt, Timeout: cfg.Timeout}
}

// roundTripperFunc adapts a func to RoundTripper.
type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

// withInner is a context-based escape hatch so a middleware can delegate to
// the next hop. Standard middleware wraps the RoundTripper directly; this
// helper is retained for advanced middleware that needs the inner transport.
func withInner(req *http.Request, inner RoundTripper) *http.Request {
	return req.WithContext(context.WithValue(req.Context(), innerKey{}, inner))
}

type innerKey struct{}

// CurrentController returns the currently selected controller IP.
func (c *Client) CurrentController() string {
	return c.controllers[c.nodeIndex]
}

// retryConfig bounds the 725-busy retry behavior.
type retryConfig struct {
	MaxRetries int
	BaseDelay  time.Duration
	MaxDelay   time.Duration
}

// sendRequest performs one bounded request with 725-busy retry and returns
// the raw body, HTTP status, and a typed error for semantic failures.
func sendRequest(
	ctx context.Context,
	client *http.Client,
	logger Logger,
	method, urlStr, apiKey string,
	body interface{},
	cfg retryConfig,
) ([]byte, int, error) {
	var bodyBytes []byte
	if body != nil {
		var err error
		bodyBytes, err = json.Marshal(body)
		if err != nil {
			return nil, 0, fmt.Errorf("ngx sdk: marshal request body: %w", err)
		}
	}

	for attempt := 1; attempt <= cfg.MaxRetries; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, 0, err
		}
		var reqBody io.Reader
		if bodyBytes != nil {
			reqBody = bytes.NewReader(bodyBytes)
		}
		req, err := http.NewRequestWithContext(ctx, method, urlStr, reqBody)
		if err != nil {
			return nil, 0, fmt.Errorf("ngx sdk: build request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+apiKey)

		resp, err := client.Do(req)
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return nil, 0, ctxErr
			}
			return nil, 0, NewTransportError(method, urlStr, err)
		}

		respBody, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			return nil, 0, NewTransportError(method, urlStr, readErr)
		}

		code, msg := responseError(respBody)
		if code == CodeBusy && attempt < cfg.MaxRetries {
			logger.Warnf("ngx sdk: pool busy (725), attempt %d/%d", attempt, cfg.MaxRetries)
			delay := cfg.BaseDelay
			for r := 1; r < attempt && delay < cfg.MaxDelay; r++ {
				delay *= 2
				if delay > cfg.MaxDelay {
					delay = cfg.MaxDelay
				}
			}
			if err := sleepContext(ctx, delay); err != nil {
				return respBody, resp.StatusCode, err
			}
			continue
		}

		// Released NGX endpoints return JSON. Empty/malformed bodies are
		// protocol failures.
		if len(bytes.TrimSpace(respBody)) == 0 || !json.Valid(respBody) {
			return respBody, resp.StatusCode, &APIError{
				StatusCode: resp.StatusCode,
				Method:     method,
				Endpoint:   urlStr,
				Body:       append([]byte(nil), respBody...),
			}
		}

		if resp.StatusCode >= 400 || code != 0 {
			return respBody, resp.StatusCode, &APIError{
				StatusCode: resp.StatusCode,
				Code:       code,
				Message:    msg,
				Method:     method,
				Endpoint:   urlStr,
				Body:       append([]byte(nil), respBody...),
			}
		}

		return respBody, resp.StatusCode, nil
	}
	return nil, 0, NewTransportError(method, urlStr, errors.New("ngx sdk: retries exhausted"))
}

// responseError extracts the canonical {code, error} fields from a body.
func responseError(body []byte) (int, string) {
	var resp struct {
		Code  json.Number `json:"code"`
		Error string      `json:"error"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return 0, ""
	}
	code, _ := resp.Code.Int64()
	return int(code), resp.Error
}

// sleepContext waits for delay or returns when ctx ends.
func sleepContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
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
		c.logger.Warnf("ngx sdk: suppressing unsafe transport replay for %s", method)
		return resp, code, err
	}
	// Safe methods (GET/DELETE) replay after a controller refresh.
	if refreshErr := c.RefreshController(ctx); refreshErr != nil {
		c.logger.Warnf("ngx sdk: refresh after transport failure: %v", refreshErr)
		return resp, code, fmt.Errorf("ngx sdk: refresh after transport failure: %v: %w", refreshErr, err)
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
