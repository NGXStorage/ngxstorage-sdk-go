package ngxstorage

import (
	"errors"
	"fmt"
)

// Backend error codes returned by the NGX Storage Manager API v2. These are
// the canonical numeric codes observed in live responses (see the Cinder FC
// driver's api.py and the NFS CSI driver for the authoritative list).
const (
	// CodeBusy marks the pool as busy under heavy I/O. Retried with bounded
	// exponential backoff.
	CodeBusy = 725
	// CodeNotFound marks a referenced resource as absent. Idempotent delete
	// operations treat it as success.
	CodeNotFound = 5011
	// CodeAlreadyExists marks a create of a name that already exists.
	CodeAlreadyExists = 710
	// CodeAttachedBusy marks a resource that cannot be mutated while attached.
	CodeAttachedBusy = 534
	// CodeIQNNotFound / CodeIQNExists / CodeTargetDeleted are iSCSI-specific
	// ignorable codes used by the Cinder driver.
	CodeIQNNotFound   = 521
	CodeIQNExists     = 522
	CodeTargetDeleted = 611
	// CodeUnauthorized is returned when the API key is rejected.
	CodeUnauthorized = 5000
)

// APIError describes a semantic backend error returned by the NGX API v2.
// It carries the HTTP status, the canonical NGX error code, and the raw
// response body (never logged by the SDK itself).
type APIError struct {
	StatusCode int
	Code       int
	Message    string
	Method     string
	Endpoint   string
	Body       []byte
}

func (e *APIError) Error() string {
	if e.Code != 0 {
		return fmt.Sprintf("ngxstorage api error: method=%s endpoint=%s http_status=%d code=%d message=%q", e.Method, e.Endpoint, e.StatusCode, e.Code, e.Message)
	}
	return fmt.Sprintf("ngxstorage api error: method=%s endpoint=%s http_status=%d message=%q", e.Method, e.Endpoint, e.StatusCode, e.Message)
}

// TransportError describes a network-level failure reaching the backend.
type TransportError struct {
	Method   string
	Endpoint string
	Err      error
}

func (e *TransportError) Error() string {
	return fmt.Sprintf("ngxstorage transport error: method=%s endpoint=%s: %v", e.Method, e.Endpoint, e.Err)
}

func (e *TransportError) Unwrap() error { return e.Err }

// NewTransportError constructs a TransportError for a request-level failure.
func NewTransportError(method, endpoint string, err error) error {
	return &TransportError{Method: method, Endpoint: endpoint, Err: err}
}

// Sentinel errors resolved during controller/pool refresh.
var (
	// ErrPoolNotFound means every required pool list confirmed the configured
	// pool is absent.
	ErrPoolNotFound = errors.New("ngx: pool not found")
	// ErrClusterNotReady means controller roles do not form a serviceable work
	// mode, or placement is ambiguous.
	ErrClusterNotReady = errors.New("ngx: cluster not ready")
)

// Classify maps an APIError to a driver-friendly kind. Drivers use this to
// translate backend errors into CSI/Cinder status codes without string
// matching.
func Classify(err error) (kind string, code int, ok bool) {
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		return "", 0, false
	}
	return ClassifyCode(apiErr.Code), apiErr.Code, true
}

// ClassifyCode maps a canonical NGX error code to a stable kind string.
func ClassifyCode(code int) string {
	switch code {
	case CodeBusy:
		return "busy"
	case CodeNotFound, CodeIQNNotFound, CodeTargetDeleted:
		return "not_found"
	case CodeAlreadyExists, CodeIQNExists:
		return "already_exists"
	case CodeAttachedBusy:
		return "attached_busy"
	case CodeUnauthorized:
		return "unauthorized"
	default:
		return "unknown"
	}
}

// IsNotFound reports whether the error carries an idempotent not-found code.
func IsNotFound(err error) bool {
	kind, _, ok := Classify(err)
	return ok && kind == "not_found"
}

// IsAlreadyExists reports whether the error carries an already-exists code.
func IsAlreadyExists(err error) bool {
	kind, _, ok := Classify(err)
	return ok && kind == "already_exists"
}

// IsBusy reports whether the error carries the pool-busy code.
func IsBusy(err error) bool {
	kind, _, ok := Classify(err)
	return ok && kind == "busy"
}

// IsUnauthorized reports whether the error carries an auth failure.
func IsUnauthorized(err error) bool {
	kind, _, ok := Classify(err)
	return ok && kind == "unauthorized"
}

// IsTransportError reports whether err is (or wraps) a TransportError.
func IsTransportError(err error) bool {
	var t *TransportError
	return errors.As(err, &t)
}

// HTTPStatusFromError returns the HTTP status code carried by an APIError, or
// 0 for non-API errors.
func HTTPStatusFromError(err error) int {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode
	}
	return 0
}
