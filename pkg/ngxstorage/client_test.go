package ngxstorage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

// newTestClient builds a Client pointed at an httptest server and returns both.
func newTestClient(t *testing.T, handler http.HandlerFunc) (*Client, *httptest.Server) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	// The httptest server URL has no scheme in Controllers; strip the http://
	// prefix because the SDK builds https:// URLs itself. Instead, we inject a
	// custom HTTP client that rewrites https to the test server scheme.
	client := &http.Client{
		Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			req.URL.Scheme = "http"
			req.URL.Host = server.Listener.Addr().String()
			req.Host = server.Listener.Addr().String()
			return http.DefaultTransport.RoundTrip(req)
		}),
	}

	cfg := Config{
		Controllers: []string{"10.0.0.1"},
		APIKey:      "test-key",
		PoolName:    "pool1",
		HTTPClient:  client,
		Logger:      NopLogger{},
	}
	c, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	// Simulate a completed initial RefreshController (as the drivers perform
	// at startup) so per-request ensureRefresh does not fire during tests.
	c.publishControllerSelection(0)
	return c, server
}

func TestNewClientValidation(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{"no controllers", Config{APIKey: "k"}, true},
		{"blank controller", Config{Controllers: []string{" "}, APIKey: "k"}, true},
		{"too many controllers", Config{Controllers: []string{"a", "b", "c"}, APIKey: "k"}, true},
		{"duplicate controllers", Config{Controllers: []string{"a", " a "}, APIKey: "k"}, true},
		{"no api key", Config{Controllers: []string{"a"}}, true},
		{"valid", Config{Controllers: []string{"a"}, APIKey: "k"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewClient(tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Fatalf("NewClient() err=%v wantErr=%v", err, tt.wantErr)
			}
		})
	}
	c, err := NewClient(Config{Controllers: []string{" 10.0.0.1 "}, APIKey: "k"})
	if err != nil {
		t.Fatalf("trimmed controller: %v", err)
	}
	if got := c.CurrentController(); got != "10.0.0.1" {
		t.Fatalf("controller = %q, want trimmed address", got)
	}
	if c.cfg.MaxRetries != 6 || c.cfg.BaseDelay != 10*time.Second || c.cfg.MaxDelay != 30*time.Second {
		t.Fatalf("retry defaults = %d/%s/%s", c.cfg.MaxRetries, c.cfg.BaseDelay, c.cfg.MaxDelay)
	}
}

func TestLUNCreate(t *testing.T) {
	var gotBody map[string]interface{}
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("auth header = %q", r.Header.Get("Authorization"))
		}
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.Write([]byte(`{"luns":[{"id":"lun-1","name":"vol1"}]}`))
	})

	lun, err := c.LUNs().Create(context.Background(), "vol1", 100)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if lun.ID != "lun-1" {
		t.Fatalf("lun ID = %s, want lun-1", lun.ID)
	}
	if gotBody["pool_name"] != "pool1" {
		t.Errorf("pool_name = %v, want pool1", gotBody["pool_name"])
	}
	if gotBody["size"] != float64(100) {
		t.Errorf("size = %v, want 100", gotBody["size"])
	}
}

func TestLUNCreateValidation(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("invalid create must not reach the backend")
	})
	if _, err := c.LUNs().Create(context.Background(), "", 1); err == nil {
		t.Fatal("empty LUN name should fail")
	}
	if _, err := c.LUNs().Create(context.Background(), "vol1", 0); err == nil {
		t.Fatal("non-positive LUN size should fail")
	}
}

func TestLUNExpandSendsAdditiveIncrement(t *testing.T) {
	var gotBody map[string]interface{}
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Errorf("method = %s, want PATCH", r.Method)
		}
		if r.URL.Path != "/api/v2/lun/lun-1" {
			t.Errorf("path = %q, want /api/v2/lun/lun-1", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode expand body: %v", err)
		}
		w.Write([]byte(`{"success":true}`))
	})

	// The backend contract is additive. A caller expanding 1 GiB -> 3 GiB
	// passes incrementGB=2, and the SDK must preserve that delta verbatim.
	if err := c.LUNs().Expand(context.Background(), "lun-1", 2); err != nil {
		t.Fatalf("Expand: %v", err)
	}
	if gotBody["size"] != float64(2) {
		t.Fatalf("size = %v, want additive increment 2", gotBody["size"])
	}
	if gotBody["sing"] != "G" {
		t.Fatalf("sing = %v, want G", gotBody["sing"])
	}
}

func TestListEndpointParity(t *testing.T) {
	tests := []struct {
		name string
		path string
		call func(context.Context, *Client) error
	}{
		{"FC list", "/api/v2/target/fc", func(ctx context.Context, c *Client) error { _, err := c.FCTargets().List(ctx); return err }},
		{"FC detail", "/api/v2/target/fc/list", func(ctx context.Context, c *Client) error { _, err := c.FCTargets().ListDetail(ctx); return err }},
		{"iSCSI list", "/api/v2/target/iscsi", func(ctx context.Context, c *Client) error { _, err := c.ISCSITargets().List(ctx); return err }},
		{"iSCSI detail", "/api/v2/target/iscsi/list", func(ctx context.Context, c *Client) error { _, err := c.ISCSITargets().ListDetail(ctx); return err }},
		{"pool names", "/api/v2/pool", func(ctx context.Context, c *Client) error { _, err := c.Pools().List(ctx); return err }},
		{"pool detail", "/api/v2/pool/list", func(ctx context.Context, c *Client) error { _, err := c.Pools().ListDetail(ctx); return err }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != tt.path {
					t.Fatalf("path = %q, want %q", r.URL.Path, tt.path)
				}
				w.Write([]byte(`[]`))
			})
			if err := tt.call(context.Background(), c); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestLUNDeleteIdempotent(t *testing.T) {
	calls := 0
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		// The semantic-404 path probes cluster status and the pool list to
		// rule out a stale controller selection before reporting the error.
		if strings.Contains(r.URL.Path, "/status/cluster") {
			w.Write([]byte(`{"connected":1,"status":"Master"}`))
			return
		}
		if strings.HasSuffix(r.URL.Path, "/api/v2/pool") {
			w.Write([]byte(`[{"id":"pool1","name":"pool1"}]`))
			return
		}
		calls++
		// NotFound code 5011 → idempotent success.
		w.Write([]byte(`{"code":5011,"error":"not found"}`))
	})

	if err := c.LUNs().Delete(context.Background(), "lun-1"); err != nil {
		t.Fatalf("Delete should be idempotent, got: %v", err)
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want 1", calls)
	}
}

func TestBusyRetry(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls < 3 {
			w.Write([]byte(`{"code":725,"error":"busy"}`))
			return
		}
		w.Write([]byte(`{"luns":[{"id":"lun-1"}]}`))
	}))
	defer server.Close()

	transport := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		req.URL.Scheme = "http"
		req.URL.Host = server.Listener.Addr().String()
		return http.DefaultTransport.RoundTrip(req)
	})

	// Use tiny delays via sendRequest directly (deterministic, fast).
	_, _, err := sendRequest(context.Background(), &http.Client{Transport: transport}, NopLogger{},
		http.MethodPost, "https://x/api/v2/lun", "test-key",
		map[string]interface{}{"pool_name": "p", "name": "v", "size": 1, "sing": "G"},
		retryConfig{MaxRetries: 6, BaseDelay: time.Millisecond, MaxDelay: 10 * time.Millisecond},
	)
	if err != nil {
		t.Fatalf("sendRequest after busy retry: %v", err)
	}
	if calls != 3 {
		t.Fatalf("calls = %d, want 3 (2 busy + 1 success)", calls)
	}
}

func TestSendRequestHonorsContextDeadlineDuringHTTPCall(t *testing.T) {
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, _, err := sendRequest(ctx, server.Client(), NopLogger{}, http.MethodGet,
		server.URL, "test-key", nil,
		retryConfig{MaxRetries: 3, BaseDelay: time.Second, MaxDelay: time.Second})

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("sendRequest error = %v, want context deadline exceeded", err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("request did not reach test server")
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("deadline cancellation took %v; want prompt cancellation", elapsed)
	}
}

func TestAPIClassify(t *testing.T) {
	err := &APIError{Code: CodeNotFound}
	if !IsNotFound(err) {
		t.Fatal("IsNotFound should be true for CodeNotFound")
	}
	err2 := &APIError{Code: CodeBusy}
	if !IsBusy(err2) {
		t.Fatal("IsBusy should be true for CodeBusy")
	}
	err3 := &APIError{Code: CodeUnauthorized}
	if !IsUnauthorized(err3) {
		t.Fatal("IsUnauthorized should be true for CodeUnauthorized")
	}
	wrapped404 := fmt.Errorf("wrapped get failure: %w", &APIError{StatusCode: http.StatusNotFound})
	if !IsNotFound(wrapped404) {
		t.Fatal("IsNotFound should be true for wrapped HTTP 404 without NGX code")
	}
}

func TestTransportReplaySafety(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodDelete} {
		if !isTransportReplaySafe(method) {
			t.Fatalf("%s should be replay-safe", method)
		}
	}
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch} {
		if isTransportReplaySafe(method) {
			t.Fatalf("%s must not be replayed after an ambiguous transport failure", method)
		}
	}
}

func TestFCTargetGetRaw(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{
			"id":"t1","name":"tgt-a","owner":"o1",
			"ports":[
				{"id":"p1","name":"fc0/1","wwpns":[{"wwpn":"naa.2002a","owner":"o","target_id":"t1"}],"initiators":[]}
			],
			"luns":[{"id":"lun-1","name":"v","number":"6","pool_name":"pool1","scsi_id":"3600"}]
		}`))
	})

	target, err := c.FCTargets().Get(context.Background(), "t1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	// SDK returns raw backend values: WWPN keeps the naa. prefix, LUN number
	// stays a string. Interpretation belongs to the driver.
	if len(target.Ports) != 1 || target.Ports[0].Wwpns[0].WWPN != "naa.2002a" {
		t.Fatalf("raw WWPN not preserved: %+v", target.Ports)
	}
	if target.Luns[0].Number != "6" {
		t.Fatalf("raw LUN number = %q, want string \"6\"", target.Luns[0].Number)
	}
}

func TestFCTargetGetNumericLUNNumber(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{
			"id":"t1","name":"tgt-a","owner":"o1","ports":[],
			"luns":[{"id":"lun-1","name":"v","number":6,"pool_name":"pool1","scsi_id":"3600"}]
		}`))
	})

	target, err := c.FCTargets().Get(context.Background(), "t1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if target.Luns[0].Number != "6" {
		t.Fatalf("numeric LUN number = %q, want string \"6\"", target.Luns[0].Number)
	}
}

func TestSnapshotClone(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		w.Write([]byte(`{"volume_id":"new-lun-1"}`))
	})

	volID, err := c.Snapshots().Clone(context.Background(), "snap-1", "clone-1")
	if err != nil {
		t.Fatalf("Clone: %v", err)
	}
	if volID != "new-lun-1" {
		t.Fatalf("volID = %s, want new-lun-1", volID)
	}
}

func TestPoolCapacity(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[{"name":"pool1","logical":{"available":1000,"reserved":500}}]`))
	})

	avail, reserved, err := c.Pools().GetConfiguredCapacity(context.Background())
	if err != nil {
		t.Fatalf("GetConfiguredCapacity: %v", err)
	}
	if avail != 1000 || reserved != 500 {
		t.Fatalf("capacity = %d/%d, want 1000/500", avail, reserved)
	}
}

func TestMalformedJSON(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`not json`))
	})
	_, err := c.LUNs().List(context.Background())
	if err == nil {
		t.Fatal("List should fail on malformed JSON")
	}
}

func TestErrorStringRedactsNothing(t *testing.T) {
	// APIError.String must never include the API key (it carries no secret).
	err := &APIError{StatusCode: 401, Code: CodeUnauthorized, Method: "GET", Endpoint: "https://x/api/v2/pool", Message: "bad key"}
	s := err.Error()
	if s == "" {
		t.Fatal("Error() should be non-empty")
	}
	_ = fmt.Sprint(s) // ensure no panic
}

func TestEnsureRefresh(t *testing.T) {
	var requests int
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		requests++
		switch r.URL.Path {
		case "/api/v2/status/cluster":
			w.Write([]byte(`{"connected":1,"status":"Master"}`))
		case "/api/v2/pool":
			w.Write([]byte(`[{"id":"p1","name":"pool1"}]`))
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	})

	// Fresh selection: ensureRefresh must not issue any request.
	c.publishControllerSelection(0)
	c.ensureRefresh(context.Background())
	if requests != 0 {
		t.Fatalf("fresh selection triggered %d requests, want 0", requests)
	}

	// Stale selection (>5m): ensureRefresh re-resolves the controller.
	c.selectionMu.Lock()
	c.lastRefresh = time.Now().Add(-6 * time.Minute)
	c.selectionMu.Unlock()
	c.ensureRefresh(context.Background())
	if requests == 0 {
		t.Fatal("stale selection did not trigger a refresh")
	}
	if c.CurrentController() != "10.0.0.1" {
		t.Fatalf("controller = %s, want 10.0.0.1", c.CurrentController())
	}
}

func TestEnsureRefreshZeroSelectionRefreshes(t *testing.T) {
	var requests int
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path == "/api/v2/status/cluster" {
			w.Write([]byte(`{"connected":1,"status":"Master"}`))
			return
		}
		if r.URL.Path == "/api/v2/pool" {
			w.Write([]byte(`[{"id":"p1","name":"pool1"}]`))
			return
		}
		t.Errorf("unexpected path %s", r.URL.Path)
	})

	// A never-refreshed client (zero timestamp) refreshes on first use.
	c.selectionMu.Lock()
	c.lastRefresh = time.Time{}
	c.selectionMu.Unlock()
	c.ensureRefresh(context.Background())
	if requests == 0 {
		t.Fatal("zero selection did not trigger a refresh")
	}
}

func TestTransportFailoverReplaysSafeMethod(t *testing.T) {
	var failed bool
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/status/cluster" {
			w.Write([]byte(`{"connected":1,"status":"Master"}`))
			return
		}
		if r.URL.Path == "/api/v2/pool" {
			w.Write([]byte(`[{"id":"p1","name":"pool1"}]`))
			return
		}
		if r.URL.Path == "/api/v2/lun/list" && !failed {
			failed = true
			// Simulate a transport failure by resetting the connection.
			hj, ok := w.(http.Hijacker)
			if !ok {
				t.Fatal("hijack unsupported in this Go version")
				return
			}
			conn, _, _ := hj.Hijack()
			conn.Close()
			return
		}
		w.Write([]byte(`[]`))
	})

	// Avoid ensureRefresh firing mid-test; the replay path exercises refresh.
	c.publishControllerSelection(0)

	luns, err := c.LUNs().List(context.Background())
	if err != nil {
		t.Fatalf("List failed after failover: %v", err)
	}
	if len(luns) != 0 {
		t.Fatalf("unexpected luns: %#v", luns)
	}
	if !failed {
		t.Fatal("expected a simulated transport failure")
	}
}

// A refused connection never reached the backend, so the retry is safe for
// every method (POST included).
func TestTransportRetryOnRefusedConnection(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	transport := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		if calls.Add(1) < 3 {
			return nil, fmt.Errorf("dial tcp: connect: %w", syscall.ECONNREFUSED)
		}
		req.URL.Scheme = "http"
		req.URL.Host = server.Listener.Addr().String()
		return http.DefaultTransport.RoundTrip(req)
	})

	_, _, err := sendRequest(context.Background(), &http.Client{Transport: transport}, NopLogger{},
		http.MethodPost, "https://x/api/v2/lun", "test-key", map[string]string{"name": "v"},
		retryConfig{MaxRetries: 4, BaseDelay: time.Millisecond, MaxDelay: time.Millisecond,
			TransportBaseDelay: time.Millisecond, TransportMaxDelay: time.Millisecond})
	if err != nil {
		t.Fatalf("sendRequest after refused retry: %v", err)
	}
	if got := calls.Load(); got != 3 {
		t.Fatalf("calls = %d, want 3 (2 refused + 1 success)", got)
	}
}

// A post-send failure (EOF) could have been processed, so unsafe methods must
// not be replayed automatically.
func TestTransportNoRetryOnEOFForUnsafeMethod(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		hijacker, ok := w.(http.Hijacker)
		if !ok {
			t.Fatal("hijacking unsupported")
		}
		conn, _, err := hijacker.Hijack()
		if err != nil {
			t.Fatal(err)
		}
		conn.Close()
	}))
	defer server.Close()

	transport := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		req.URL.Scheme = "http"
		req.URL.Host = server.Listener.Addr().String()
		return http.DefaultTransport.RoundTrip(req)
	})

	_, _, err := sendRequest(context.Background(), &http.Client{Transport: transport}, NopLogger{},
		http.MethodPost, "https://x/api/v2/lun", "test-key", map[string]string{"name": "v"},
		retryConfig{MaxRetries: 4, BaseDelay: time.Millisecond, MaxDelay: time.Millisecond,
			TransportBaseDelay: time.Millisecond, TransportMaxDelay: time.Millisecond})
	if err == nil {
		t.Fatal("sendRequest succeeded on EOF for POST, want transport error")
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("calls = %d, want 1 (no replay for unsafe method)", got)
	}
}

// EOF on an idempotent method is safe to replay and must ride out a brief
// backend restart.
func TestTransportRetryOnEOFForIdempotentMethod(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) < 3 {
			hijacker, ok := w.(http.Hijacker)
			if !ok {
				t.Fatal("hijacking unsupported")
			}
			conn, _, err := hijacker.Hijack()
			if err != nil {
				t.Fatal(err)
			}
			conn.Close()
			return
		}
		w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	transport := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		req.URL.Scheme = "http"
		req.URL.Host = server.Listener.Addr().String()
		return http.DefaultTransport.RoundTrip(req)
	})

	_, _, err := sendRequest(context.Background(), &http.Client{Transport: transport}, NopLogger{},
		http.MethodGet, "https://x/api/v2/lun/list", "test-key", nil,
		retryConfig{MaxRetries: 4, BaseDelay: time.Millisecond, MaxDelay: time.Millisecond,
			TransportBaseDelay: time.Millisecond, TransportMaxDelay: time.Millisecond})
	if err != nil {
		t.Fatalf("sendRequest after EOF retry: %v", err)
	}
	if got := calls.Load(); got != 3 {
		t.Fatalf("calls = %d, want 3 (2 EOF + 1 success)", got)
	}
}
