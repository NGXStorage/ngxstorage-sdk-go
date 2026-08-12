package ngxsdk

// Logger is the pluggable logging interface used by the SDK. It defaults to a
// no-op logger; drivers inject their own (zap, slog, logrus, ...) via
// Config.Logger. The SDK never logs secret values (API keys, CHAP passwords,
// S3 secret keys) through this interface — only booleans and non-sensitive
// metadata.
type Logger interface {
	Debugf(format string, args ...any)
	Infof(format string, args ...any)
	Warnf(format string, args ...any)
	Errorf(format string, args ...any)
}

// NopLogger discards all log output. It is the default when a driver does not
// inject a logger.
type NopLogger struct{}

func (NopLogger) Debugf(string, ...any) {}
func (NopLogger) Infof(string, ...any)  {}
func (NopLogger) Warnf(string, ...any)  {}
func (NopLogger) Errorf(string, ...any) {}
