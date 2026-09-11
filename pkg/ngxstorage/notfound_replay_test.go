package ngxstorage

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
)

// A semantic NotFound against a stale controller selection must re-resolve the
// controller (pool ownership) and replay once when the selection changes.
func TestNotFoundRefreshesControllerAndReplays(t *testing.T) {
	ready := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/status/cluster"):
			w.Write([]byte(`{"connected":1,"status":"Ready"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"code":5011,"error":"no share with id"}`))
		}
	}))
	defer ready.Close()
	master := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/status/cluster"):
			w.Write([]byte(`{"connected":1,"status":"Master"}`))
		case strings.HasSuffix(r.URL.Path, "/api/v2/pool"):
			w.Write([]byte(`[{"id":"pool1","name":"pool1"}]`))
		default:
			w.Write([]byte(`{"ok":true}`))
		}
	}))
	defer master.Close()

	transport := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host == "10.0.0.1" {
			req.URL.Scheme = "http"
			req.URL.Host = ready.Listener.Addr().String()
			return http.DefaultTransport.RoundTrip(req)
		}
		req.URL.Scheme = "http"
		req.URL.Host = master.Listener.Addr().String()
		return http.DefaultTransport.RoundTrip(req)
	})
	c, err := NewClient(Config{
		Controllers: []string{"10.0.0.1", "10.0.0.2"},
		APIKey:      "test-key",
		PoolName:    "pool1",
		HTTPClient:  &http.Client{Transport: transport},
		Logger:      NopLogger{},
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	c.publishControllerSelection(0) // stale selection: the Ready controller

	body, err := c.get(context.Background(), "GetShare", url.Values{"id": {"share-1"}})
	if err != nil {
		t.Fatalf("expected replay against the master, got %v", err)
	}
	if !strings.Contains(string(body), "ok") {
		t.Fatalf("body = %s, want the master response", body)
	}
}

// When the refresh keeps the same controller, the NotFound is returned without
// a replay loop.
func TestNotFoundWithoutSelectionChangeReturnsNotFound(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/status/cluster"):
			w.Write([]byte(`{"connected":1,"status":"Master"}`))
		case strings.HasSuffix(r.URL.Path, "/api/v2/pool"):
			w.Write([]byte(`[{"id":"pool1","name":"pool1"}]`))
		default:
			calls.Add(1)
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"code":5011,"error":"no share with id"}`))
		}
	}))
	defer server.Close()

	transport := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		req.URL.Scheme = "http"
		req.URL.Host = server.Listener.Addr().String()
		return http.DefaultTransport.RoundTrip(req)
	})
	c, err := NewClient(Config{
		Controllers: []string{"10.0.0.1", "10.0.0.2"},
		APIKey:      "test-key",
		PoolName:    "pool1",
		HTTPClient:  &http.Client{Transport: transport},
		Logger:      NopLogger{},
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	c.publishControllerSelection(0)

	if _, err := c.get(context.Background(), "GetShare", url.Values{"id": {"share-1"}}); !IsNotFound(err) {
		t.Fatalf("err = %v, want NotFound", err)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("object requests = %d, want 1 (no replay without a selection change)", got)
	}
}
