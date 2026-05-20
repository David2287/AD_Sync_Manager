package audit

import (
	"encoding/json"
	"sync"

	"gopkg.in/natefinch/lumberjack.v2"
)

// FileLogger writes JSON Lines audit entries to a rotating log file.
//
// Rotation policy (via lumberjack):
//   - MaxSize    100 MB  — rotate when file exceeds this size
//   - MaxAge     30 days — delete rotated files older than this
//   - MaxBackups 10      — keep at most 10 archived files
//   - Compress   true    — gzip archives to save disk space
//
// Writes are serialised with a mutex so concurrent goroutines never interleave
// partial JSON objects. Wrap in AsyncLogger for non-blocking request flow.
type FileLogger struct {
	mu  sync.Mutex
	enc *json.Encoder
	lj  *lumberjack.Logger
}

// NewFileLogger creates a FileLogger that writes to path.
// The directory must already exist (or will be created by the caller).
func NewFileLogger(path string) (*FileLogger, error) {
	lj := &lumberjack.Logger{
		Filename:   path,
		MaxSize:    100, // MB
		MaxAge:     30,  // days
		MaxBackups: 10,
		Compress:   true,
	}
	return &FileLogger{enc: json.NewEncoder(lj), lj: lj}, nil
}

// Log appends entry as a single JSON Line.
func (f *FileLogger) Log(entry AuditLog) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.enc.Encode(entry)
}

// Rotate forces an immediate log file rotation regardless of size or age.
// Useful for the optional DELETE /api/v1/logs/rotate endpoint.
func (f *FileLogger) Rotate() error { return f.lj.Rotate() }

// Close flushes and releases the underlying file handle.
func (f *FileLogger) Close() error { return f.lj.Close() }
