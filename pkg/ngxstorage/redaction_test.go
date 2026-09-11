package ngxstorage

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

const (
	redactionAPIKey       = "ngx-secret-api-key-0123456789"
	redactionCHAPPassword = "ngx-secret-chap-0123456789"
)

// captureLogger records every SDK log line for the redaction assertions.
type captureLogger struct {
	mu    sync.Mutex
	lines []string
}

func (l *captureLogger) append(level, format string, args ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.lines = append(l.lines, level+" "+fmt.Sprintf(format, args...))
}

func (l *captureLogger) Debugf(format string, args ...interface{}) {
	l.append("debug", format, args...)
}
func (l *captureLogger) Infof(format string, args ...interface{}) { l.append("info", format, args...) }
func (l *captureLogger) Warnf(format string, args ...interface{}) { l.append("warn", format, args...) }
func (l *captureLogger) Errorf(format string, args ...interface{}) {
	l.append("error", format, args...)
}

func (l *captureLogger) joined() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return strings.Join(l.lines, "\n")
}

func redactionClient(t *testing.T, logger Logger, transport http.RoundTripper) *Client {
	t.Helper()
	c, err := NewClient(Config{
		Controllers: []string{"10.0.0.1"},
		APIKey:      redactionAPIKey,
		PoolName:    "pool1",
		HTTPClient:  &http.Client{Transport: transport},
		Logger:      logger,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	c.publishControllerSelection(0)
	return c
}

func assertNoSecrets(t *testing.T, logs string) {
	t.Helper()
	for _, secret := range []string{redactionAPIKey, redactionCHAPPassword} {
		if strings.Contains(logs, secret) {
			t.Fatalf("SDK log leaked %q:\n%s", secret, logs)
		}
	}
}

// The adversarial case from the redaction audit: a CHAP password travels in
// the request body while the backend fails the call. The secret must reach
// the backend and must never appear in an SDK log line.
func TestSDKLogsNeverContainSecrets(t *testing.T) {
	secretSeenInRequest := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if strings.Contains(string(body), redactionCHAPPassword) {
			secretSeenInRequest = true
		}
		switch {
		case strings.Contains(r.URL.Path, "/status/cluster"):
			w.Write([]byte(`{"connected":1,"status":"Master"}`))
		case strings.HasSuffix(r.URL.Path, "/api/v2/pool"):
			w.Write([]byte(`[{"id":"pool1","name":"pool1","owner":"o"}]`))
		case strings.HasSuffix(r.URL.Path, "/api/v2/pool/pool1"):
			w.Write([]byte(`{"id":"pool1","name":"pool1","owner":"o"}`))
		case strings.Contains(r.URL.Path, "/api/v2/auth_group/"):
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"code":5000,"error":"backend rejected ` + redactionCHAPPassword + `"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	logger := &captureLogger{}
	client := redactionClient(t, logger, roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		req.URL.Scheme = "http"
		req.URL.Host = server.Listener.Addr().String()
		return http.DefaultTransport.RoundTrip(req)
	}))

	if err := client.AuthGroups().AddCHAP(context.Background(), "ag-1", "user", redactionCHAPPassword); err == nil {
		t.Fatal("AddCHAP succeeded, want backend error")
	}
	if !secretSeenInRequest {
		t.Fatal("test setup: CHAP password never reached the backend")
	}
	assertNoSecrets(t, logger.joined())
}

// The transport-failure path is the one place the SDK logs request context.
// The logged line must stay limited to the method (and URL, which never
// carries credentials) even when the request body held secrets.
func TestSDKTransportFailureLogsNeverContainSecrets(t *testing.T) {
	logger := &captureLogger{}
	client := redactionClient(t, logger, roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		return nil, fmt.Errorf("write tcp: EOF")
	}))

	if err := client.AuthGroups().AddCHAP(context.Background(), "ag-1", "user", redactionCHAPPassword); err == nil {
		t.Fatal("AddCHAP succeeded, want transport error")
	}
	logs := logger.joined()
	if !strings.Contains(logs, "suppressing unsafe transport replay") {
		t.Fatalf("expected the unsafe-replay log line, got:\n%s", logs)
	}
	assertNoSecrets(t, logs)
}
