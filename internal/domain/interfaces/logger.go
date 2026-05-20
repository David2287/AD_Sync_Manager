package interfaces

// Logger is the application-wide structured logging port.
// The production adapter wraps go.uber.org/zap's SugaredLogger.
// Callers pass key-value pairs: log.Info("user login", "user", uid, "ip", ip)
type Logger interface {
	Debug(msg string, keysAndValues ...any)
	Info(msg string, keysAndValues ...any)
	Warn(msg string, keysAndValues ...any)
	Error(msg string, keysAndValues ...any)

	// With returns a child logger with pre-bound key-value pairs.
	With(keysAndValues ...any) Logger

	// Sync flushes any buffered log entries. Call on shutdown.
	Sync() error
}
