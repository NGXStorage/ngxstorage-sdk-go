package ngxstorage

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
)

// semanticServer answers the refresh endpoints and counts probes separately
// from object requests so tests can prove whether a refresh actually ran.
func semanticServer(t *testing.T, object http.HandlerFunc) (*httptest.Server, *int32) {
	t.Helper()
	probes := new(int32)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/status/cluster"):
			atomic.AddInt32(probes, 1)
			w.Write([]byte(`{"connected":1,"status":"Master"}`))
		case strings.HasSuffix(r.URL.Path, "/api/v2/pool"):
			atomic.AddInt32(probes, 1)
			w.Write([]byte(`[{"id":"pool1","name":"pool1"}]`))
		default:
			object(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server, probes
}

func semanticClient(t *testing.T, logger Logger, transport http.RoundTripper) *Client {
	t.Helper()
	c, err := NewClient(Config{
		Controllers: []string{"10.0.0.1"},
		APIKey:      "test-key",
		PoolName:    "pool1",
		MaxRetries:  1,
		HTTPClient:  &http.Client{Transport: transport},
		Logger:      logger,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	c.publishControllerSelection(0)
	return c
}

func forwardTo(server *httptest.Server) http.RoundTripper {
	return roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		req.URL.Scheme = "http"
		req.URL.Host = server.Listener.Addr().String()
		return http.DefaultTransport.RoundTrip(req)
	})
}

// The request layer must not refresh-and-replay a semantic not-found: a
// mutation that reached the wrong controller is the owning driver
// operation's recovery decision, not the SDK's.
func TestSemantic5011ReturnedWithoutReplay(t *testing.T) {
	var objectCalls int32
	server, probes := semanticServer(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&objectCalls, 1)
		w.Write([]byte(`{"code":5011,"error":"no share here"}`))
	})
	c := semanticClient(t, NopLogger{}, forwardTo(server))

	err := c.AuthGroups().AddCHAP(context.Background(), "ag-1", "user", "pass")
	if err == nil {
		t.Fatal("AddCHAP succeeded, want the backend 5011")
	}
	if !IsCanonicalNotFoundError(err) {
		t.Fatalf("err = %v, want a canonical 5011", err)
	}
	if got := atomic.LoadInt32(&objectCalls); got != 1 {
		t.Fatalf("object requests = %d, want 1 (no replay)", got)
	}
	if got := atomic.LoadInt32(probes); got != 0 {
		t.Fatalf("refresh probes = %d, want 0 (the SDK must not refresh on a semantic error)", got)
	}
}

// A code-less HTTP 404 is a routing/object error and must never be turned
// into a write on another controller.
func TestCodeless404NeverReplaysWrite(t *testing.T) {
	var objectCalls int32
	server, probes := semanticServer(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&objectCalls, 1)
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error":"no route"}`))
	})
	c := semanticClient(t, NopLogger{}, forwardTo(server))

	err := c.AuthGroups().AddCHAP(context.Background(), "ag-1", "user", "pass")
	if err == nil {
		t.Fatal("AddCHAP succeeded, want the code-less 404")
	}
	if IsCanonicalNotFoundError(err) {
		t.Fatalf("err = %v, a code-less 404 is not a canonical 5011", err)
	}
	if got := atomic.LoadInt32(&objectCalls); got != 1 {
		t.Fatalf("object requests = %d, want 1", got)
	}
	if got := atomic.LoadInt32(probes); got != 0 {
		t.Fatalf("refresh probes = %d, want 0", got)
	}
}

// The safe-method transport failover is unchanged: a GET that failed with a
// transport error re-resolves the controller and is replayed once.
func TestTransportFailoverStillReplaysSafeMethods(t *testing.T) {
	var objectAttempts, objectCalls int32
	server, probes := semanticServer(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&objectCalls, 1)
		w.Write([]byte(`{"ok":true}`))
	})
	failing := new(atomic.Bool)
	failing.Store(true)
	transport := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		if strings.Contains(req.URL.Path, "/status/cluster") || strings.HasSuffix(req.URL.Path, "/api/v2/pool") {
			req.URL.Scheme = "http"
			req.URL.Host = server.Listener.Addr().String()
			return http.DefaultTransport.RoundTrip(req)
		}
		atomic.AddInt32(&objectAttempts, 1)
		if failing.CompareAndSwap(true, false) {
			return nil, fmt.Errorf("read tcp: EOF")
		}
		req.URL.Scheme = "http"
		req.URL.Host = server.Listener.Addr().String()
		return http.DefaultTransport.RoundTrip(req)
	})
	c := semanticClient(t, NopLogger{}, transport)

	body, err := c.get(context.Background(), "GetShare", url.Values{"id": {"sh-1"}})
	if err != nil {
		t.Fatalf("get after transport failover: %v", err)
	}
	if !strings.Contains(string(body), "ok") {
		t.Fatalf("body = %s, want the replayed response", body)
	}
	if got := atomic.LoadInt32(&objectAttempts); got != 2 {
		t.Fatalf("object attempts = %d, want 2 (failed + replayed)", got)
	}
	if got := atomic.LoadInt32(&objectCalls); got != 1 {
		t.Fatalf("server object calls = %d, want 1", got)
	}
	if got := atomic.LoadInt32(probes); got == 0 {
		t.Fatal("refresh probes = 0, the failover path must re-resolve the controller")
	}
}

// Unsafe methods are still never replayed after a transport failure.
func TestWritesNotReplayedAfterTransportFailure(t *testing.T) {
	var objectAttempts int32
	logger := &captureLogger{}
	transport := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method == http.MethodPatch && strings.Contains(req.URL.Path, "/api/v2/auth_group/") {
			atomic.AddInt32(&objectAttempts, 1)
			return nil, fmt.Errorf("write tcp: EOF")
		}
		return nil, fmt.Errorf("unexpected %s %s", req.Method, req.URL.Path)
	})
	c := semanticClient(t, logger, transport)

	if err := c.AuthGroups().AddCHAP(context.Background(), "ag-1", "user", "pass"); err == nil {
		t.Fatal("AddCHAP succeeded, want the transport error")
	}
	if got := atomic.LoadInt32(&objectAttempts); got != 1 {
		t.Fatalf("object attempts = %d, want 1 (no replay for an unsafe method)", got)
	}
	if !strings.Contains(logger.joined(), "suppressing unsafe transport replay") {
		t.Fatalf("expected the unsafe-replay suppression log, got:\n%s", logger.joined())
	}
}
