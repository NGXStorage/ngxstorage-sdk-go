package ngxstorage

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

// These tests pin the caller-owned retry contract: the SDK performs bounded
// 725-busy attempts and at most one cross-controller replay for GET/DELETE;
// generic same-controller transport retry does not exist and mutations are
// never replayed. They exercise the real Client.request / sendRequest paths
// (only the network seam is substituted).

func retryContractClient(t *testing.T, controllers []string, rt RoundTripper) *Client {
	t.Helper()
	c, err := NewClient(Config{
		Controllers: controllers,
		APIKey:      "test-key",
		MaxRetries:  3,
		BaseDelay:   time.Millisecond,
		MaxDelay:    time.Millisecond,
		HTTPClient:  &http.Client{Transport: rt},
		Logger:      NopLogger{},
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	c.publishControllerSelection(0)
	return c
}

// eofHandler closes the connection without a response: a post-send transport
// failure (EOF) with no ambiguous success.
func eofHandler(w http.ResponseWriter, r *http.Request) {
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		panic("hijacking unsupported")
	}
	conn, _, err := hijacker.Hijack()
	if err != nil {
		panic(err)
	}
	conn.Close()
}

// TestBusyRetriesUseMaxRetriesBudget: 725 responses consume the MaxRetries
// budget (2 busy + 1 success within 3 attempts).
func TestBusyRetriesUseMaxRetriesBudget(t *testing.T) {
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hits.Add(1) <= 2 {
			w.Write([]byte(`{"code":725,"error":"busy"}`))
			return
		}
		w.Write([]byte(`[{"id":"l1"}]`))
	}))
	defer server.Close()

	c := retryContractClient(t, []string{"10.0.0.1"}, roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		req.URL.Scheme = "http"
		req.URL.Host = server.Listener.Addr().String()
		return http.DefaultTransport.RoundTrip(req)
	}))
	if _, _, err := c.request(context.Background(), "GetLUNList", nil, nil); err != nil {
		t.Fatalf("request after busy retry: %v", err)
	}
	if got := hits.Load(); got != 3 {
		t.Fatalf("hits = %d, want 3 (2 busy + 1 success within MaxRetries=3)", got)
	}
}

// TestBusyBudgetStopsAtMaxRetries: the busy loop stops at MaxRetries exactly,
// at the single-request layer (no failover can extend the budget).
func TestBusyBudgetStopsAtMaxRetries(t *testing.T) {
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Write([]byte(`{"code":725,"error":"busy"}`))
	}))
	defer server.Close()

	c := retryContractClient(t, []string{"10.0.0.1"}, roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		req.URL.Scheme = "http"
		req.URL.Host = server.Listener.Addr().String()
		return http.DefaultTransport.RoundTrip(req)
	}))
	c.cfg.MaxRetries = 2
	if _, _, err := c.requestController(context.Background(), http.MethodGet, "https://10.0.0.1/api/v2/lun/list", nil); err == nil {
		t.Fatal("request succeeded on perpetual busy, want error")
	}
	if got := hits.Load(); got != 2 {
		t.Fatalf("hits = %d, want exactly MaxRetries=2", got)
	}
}

// TestTransportFailureIsNotRetriedBySDK: a transport failure spends exactly
// one attempt for every method (including ECONNREFUSED and EOF) and never
// reuses the busy budget.
func TestTransportFailureIsNotRetriedBySDK(t *testing.T) {
	methods := []string{http.MethodGet, http.MethodDelete, http.MethodPost, http.MethodPut, http.MethodPatch}
	for _, method := range methods {
		t.Run(method+"/ECONNREFUSED", func(t *testing.T) {
			var attempts atomic.Int32
			c := retryContractClient(t, []string{"10.0.0.1"}, roundTripperFunc(func(req *http.Request) (*http.Response, error) {
				attempts.Add(1)
				return nil, &url.Error{Op: req.Method, URL: req.URL.String(), Err: syscall.ECONNREFUSED}
			}))
			c.cfg.MaxRetries = 5
			_, _, err := c.requestController(context.Background(), method, "https://10.0.0.1/api/v2/lun", nil)
			if !IsTransportError(err) {
				t.Fatalf("error = %v, want TransportError", err)
			}
			if got := attempts.Load(); got != 1 {
				t.Fatalf("attempts = %d, want 1 (no generic same-controller retry)", got)
			}
		})
	}

	t.Run("GET/EOF", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(eofHandler))
		defer server.Close()
		var attempts atomic.Int32
		addr := server.Listener.Addr().String()
		c := retryContractClient(t, []string{"10.0.0.1"}, roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			attempts.Add(1)
			req.URL.Scheme = "http"
			req.URL.Host = addr
			return http.DefaultTransport.RoundTrip(req)
		}))
		c.cfg.MaxRetries = 5
		_, _, err := c.requestController(context.Background(), http.MethodGet, "https://10.0.0.1/api/v2/lun/list", nil)
		if !IsTransportError(err) {
			t.Fatalf("error = %v, want TransportError", err)
		}
		if got := attempts.Load(); got != 1 {
			t.Fatalf("attempts = %d, want 1 (no same-controller retry on EOF)", got)
		}
	})
}

// failoverEnv: controller A is Ready and fails the operation path with EOF;
// controller B is Master and can serve it. Cluster probes stay reachable on
// both so the real RefreshController can re-resolve the work mode.
type failoverEnv struct {
	client *Client
	opA    atomic.Int32
	opB    atomic.Int32
	failB  atomic.Bool
}

func newFailoverEnv(t *testing.T, method, opPath, okBody string) *failoverEnv {
	t.Helper()
	env := &failoverEnv{}
	serverA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/status/cluster" {
			w.Write([]byte(`{"connected":1,"status":"Ready"}`))
			return
		}
		eofHandler(w, r)
	}))
	serverB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/status/cluster" {
			w.Write([]byte(`{"connected":1,"status":"Master"}`))
			return
		}
		if env.failB.Load() {
			eofHandler(w, r)
			return
		}
		w.Write([]byte(okBody))
	}))
	t.Cleanup(serverA.Close)
	t.Cleanup(serverB.Close)

	env.client = retryContractClient(t, []string{"10.0.0.1", "10.0.0.2"}, roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		isOp := req.URL.Path == opPath && req.Method == method
		switch req.URL.Host {
		case "10.0.0.1":
			if isOp {
				env.opA.Add(1)
			}
			req.URL.Scheme = "http"
			req.URL.Host = serverA.Listener.Addr().String()
			return http.DefaultTransport.RoundTrip(req)
		default:
			if isOp {
				env.opB.Add(1)
			}
			req.URL.Scheme = "http"
			req.URL.Host = serverB.Listener.Addr().String()
			return http.DefaultTransport.RoundTrip(req)
		}
	}))
	return env
}

// TestSafeMethodFailoverReplaysOnceThenSucceeds: GET fails on the selected
// controller, the real refresh re-resolves the work mode, and exactly one
// cross-controller replay runs.
func TestSafeMethodFailoverReplaysOnceThenSucceeds(t *testing.T) {
	env := newFailoverEnv(t, http.MethodGet, "/api/v2/lun", `[{"id":"l1"}]`)
	body, _, err := env.client.request(context.Background(), "GetLUNList", nil, nil)
	if err != nil {
		t.Fatalf("request after failover: %v", err)
	}
	if len(body) == 0 {
		t.Fatal("empty body after failover")
	}
	if env.opA.Load() != 1 || env.opB.Load() != 1 {
		t.Fatalf("op attempts A=%d B=%d, want exactly one replay (1,1)", env.opA.Load(), env.opB.Load())
	}
}

// TestSafeMethodFailoverPropagatesSecondFailure: the one replay's failure is
// final; no further refresh/replay loop exists.
func TestSafeMethodFailoverPropagatesSecondFailure(t *testing.T) {
	env := newFailoverEnv(t, http.MethodGet, "/api/v2/lun", `[{"id":"l1"}]`)
	env.failB.Store(true)
	_, _, err := env.client.request(context.Background(), "GetLUNList", nil, nil)
	if !IsTransportError(err) {
		t.Fatalf("error = %v, want TransportError", err)
	}
	if env.opA.Load() != 1 || env.opB.Load() != 1 {
		t.Fatalf("op attempts A=%d B=%d, want exactly one replay then propagate (1,1)", env.opA.Load(), env.opB.Load())
	}
}

// TestDeleteFailoverReplaysOnce: DELETE rides the same single-replay contract.
func TestDeleteFailoverReplaysOnce(t *testing.T) {
	env := newFailoverEnv(t, http.MethodDelete, "/api/v2/lun/lun-1", `{"success":true}`)
	_, _, err := env.client.request(context.Background(), "DeleteLUN", url.Values{"id": {"lun-1"}}, nil)
	if err != nil {
		t.Fatalf("request after failover: %v", err)
	}
	if env.opA.Load() != 1 || env.opB.Load() != 1 {
		t.Fatalf("op attempts A=%d B=%d, want exactly one replay (1,1)", env.opA.Load(), env.opB.Load())
	}
}

// TestBusyExhaustionReturnsBusyErrorWithoutFailover pins the busy-exhaustion
// contract at the request layer: the exhausted request falls through the
// busy branch into the semantic-error branch and returns *APIError with
// CodeBusy. It is not a transport failure, so the safe-method failover never
// runs: the second controller receives zero requests and the wire attempts
// equal MaxRetries exactly.
func TestBusyExhaustionReturnsBusyErrorWithoutFailover(t *testing.T) {
	var aCalls atomic.Int32
	var bCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"code":725,"error":"busy"}`))
	}))
	defer server.Close()

	c := retryContractClient(t, []string{"10.0.0.1", "10.0.0.2"}, roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host == "10.0.0.2" {
			bCalls.Add(1)
			return nil, &url.Error{Op: req.Method, URL: req.URL.String(), Err: syscall.ECONNREFUSED}
		}
		aCalls.Add(1)
		req.URL.Scheme = "http"
		req.URL.Host = server.Listener.Addr().String()
		return http.DefaultTransport.RoundTrip(req)
	}))
	c.cfg.MaxRetries = 3

	_, _, err := c.request(context.Background(), "GetLUNList", nil, nil)
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error type = %T (%v), want *APIError with CodeBusy", err, err)
	}
	if apiErr.Code != CodeBusy {
		t.Fatalf("code = %d, want CodeBusy=%d", apiErr.Code, CodeBusy)
	}
	if IsTransportError(err) {
		t.Fatal("busy exhaustion must not be classified as a transport failure")
	}
	if got := aCalls.Load(); got != 3 {
		t.Fatalf("wire attempts = %d, want exactly MaxRetries=3", got)
	}
	if got := bCalls.Load(); got != 0 {
		t.Fatalf("second controller requests = %d, want 0 (no failover on busy exhaustion)", got)
	}
}

// TestMutationsNeverReplayedAtRequestLayer: POST/PUT/PATCH get no
// same-controller retry and no cross-controller replay, even for a refused
// connection or EOF.
func TestMutationsNeverReplayedAtRequestLayer(t *testing.T) {
	cases := []struct {
		method string
		op     string
		vars   url.Values
	}{
		{http.MethodPost, "CreateLUN", nil},
		{http.MethodPut, "AddLunToFCTarget", url.Values{"id": {"t1"}}},
		{http.MethodPatch, "ChangeISCSITargetName", url.Values{"id": {"t1"}}},
	}
	for _, tc := range cases {
		t.Run(tc.method+"/EOF", func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(eofHandler))
			defer server.Close()
			var attempts atomic.Int32
			addr := server.Listener.Addr().String()
			c := retryContractClient(t, []string{"10.0.0.1"}, roundTripperFunc(func(req *http.Request) (*http.Response, error) {
				attempts.Add(1)
				req.URL.Scheme = "http"
				req.URL.Host = addr
				return http.DefaultTransport.RoundTrip(req)
			}))
			c.cfg.MaxRetries = 5
			_, _, err := c.request(context.Background(), tc.op, tc.vars, map[string]string{"name": "x"})
			if !IsTransportError(err) {
				t.Fatalf("error = %v, want TransportError", err)
			}
			if got := attempts.Load(); got != 1 {
				t.Fatalf("attempts = %d, want 1 (mutations never replay)", got)
			}
		})

		t.Run(tc.method+"/ECONNREFUSED", func(t *testing.T) {
			var attempts atomic.Int32
			c := retryContractClient(t, []string{"10.0.0.1"}, roundTripperFunc(func(req *http.Request) (*http.Response, error) {
				attempts.Add(1)
				return nil, &url.Error{Op: req.Method, URL: req.URL.String(), Err: syscall.ECONNREFUSED}
			}))
			c.cfg.MaxRetries = 5
			_, _, err := c.request(context.Background(), tc.op, tc.vars, map[string]string{"name": "x"})
			if err == nil {
				t.Fatal("mutation succeeded on refused connection, want error")
			}
			if got := attempts.Load(); got != 1 {
				t.Fatalf("attempts = %d, want 1 (refused mutations never retried)", got)
			}
		})
	}
}
