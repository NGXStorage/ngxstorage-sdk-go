package ngxsdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
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
	return c, server
}

func TestNewClientValidation(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{"no controllers", Config{APIKey: "k"}, true},
		{"too many controllers", Config{Controllers: []string{"a", "b", "c"}, APIKey: "k"}, true},
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

	lun, err := c.LUNs().Create(context.Background(), "vol1", 100, "owner1")
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

func TestLUNDeleteIdempotent(t *testing.T) {
	calls := 0
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
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
