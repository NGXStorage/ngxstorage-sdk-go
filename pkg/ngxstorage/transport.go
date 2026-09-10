package ngxstorage

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"syscall"
	"time"
)

const defaultTimeout = 60 * time.Second

// RoundTripper is the middleware hook drivers use to wrap SDK HTTP requests.
// It matches net/http's http.RoundTripper so standard middleware (tracing,
// metrics, custom retry) composes naturally.
type RoundTripper interface {
	RoundTrip(*http.Request) (*http.Response, error)
}

// newHTTPClient builds the SDK HTTP transport with TLS and middleware wiring.
func newHTTPClient(cfg Config) *http.Client {
	tlsCfg := &tls.Config{
		InsecureSkipVerify: cfg.InsecureSkipVerify, //nolint:gosec // self-signed NGX Storage Arrays
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

// retryConfig bounds the 725-busy retry behavior.
type retryConfig struct {
	MaxRetries int
	BaseDelay  time.Duration
	MaxDelay   time.Duration
	// TransportBaseDelay and TransportMaxDelay bound the retry of transient
	// transport failures: refused connections (any method) and post-send
	// failures such as EOF (idempotent methods only).
	TransportBaseDelay time.Duration
	TransportMaxDelay  time.Duration
}

const (
	defaultTransportBaseDelay = time.Second
	defaultTransportMaxDelay  = 5 * time.Second
)

// transportRetryable reports whether a failed request may be retried. A
// refused connection never reached the backend, so any method is safe; a
// post-send failure (EOF, reset) could have been processed and is only
// replayed for idempotent methods.
func transportRetryable(method string, err error) bool {
	if errors.Is(err, syscall.ECONNREFUSED) {
		return true
	}
	return method == http.MethodGet || method == http.MethodDelete
}

func transportRetryDelay(attempt int, base, max time.Duration) time.Duration {
	delay := time.Duration(attempt) * base
	if delay > max {
		delay = max
	}
	return delay
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
			return nil, 0, fmt.Errorf("ngxstorage: marshal request body: %w", err)
		}
	}
	transportBase := cfg.TransportBaseDelay
	if transportBase <= 0 {
		transportBase = defaultTransportBaseDelay
	}
	transportMax := cfg.TransportMaxDelay
	if transportMax <= 0 {
		transportMax = defaultTransportMaxDelay
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
			return nil, 0, fmt.Errorf("ngxstorage: build request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+apiKey)

		resp, err := client.Do(req)
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return nil, 0, ctxErr
			}
			if attempt < cfg.MaxRetries && transportRetryable(method, err) {
				logger.Warnf("ngxstorage: transient transport error, attempt %d/%d: %v", attempt, cfg.MaxRetries, err)
				if serr := sleepContext(ctx, transportRetryDelay(attempt, transportBase, transportMax)); serr != nil {
					return nil, 0, serr
				}
				continue
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
			logger.Warnf("ngxstorage: pool busy (725), attempt %d/%d", attempt, cfg.MaxRetries)
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
	return nil, 0, NewTransportError(method, urlStr, errors.New("ngxstorage: retries exhausted"))
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
