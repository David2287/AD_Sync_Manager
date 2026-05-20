package audit

import (
	"fmt"
	"os"
)

// MultiLogger fans out every Log and Close call to a list of AuditLoggers.
// The first error encountered is returned; the remaining loggers still run.
type MultiLogger struct {
	loggers []AuditLogger
}

// NewMultiLogger returns a logger that writes to every provided logger.
func NewMultiLogger(loggers ...AuditLogger) *MultiLogger {
	return &MultiLogger{loggers: loggers}
}

// Log calls Log on every inner logger. Returns the first non-nil error.
func (m *MultiLogger) Log(entry AuditLog) error {
	var first error
	for _, l := range m.loggers {
		if err := l.Log(entry); err != nil && first == nil {
			first = err
		}
	}
	return first
}

// Close calls Close on every inner logger. Returns the first non-nil error.
func (m *MultiLogger) Close() error {
	var first error
	for _, l := range m.loggers {
		if err := l.Close(); err != nil && first == nil {
			first = err
		}
	}
	return first
}

// AsyncLogger wraps any AuditLogger and makes Log non-blocking by queuing
// entries in a buffered channel drained by a single background goroutine.
//
// Trade-offs:
//   - Pro:  Log never blocks the HTTP request, even during slow disk or DB writes.
//   - Con:  If the buffer fills faster than the worker drains it, excess entries
//     are dropped with a stderr warning (no retry, no persistence). This is
//     an extreme edge case under normal load with a buffer of ~512 entries.
//   - Con:  A hard crash before the worker flushes can lose buffered entries.
//     For guaranteed delivery, use synchronous writes or an external queue.
//
// Call Close at application shutdown to drain the queue before exit.
type AsyncLogger struct {
	inner AuditLogger
	ch    chan AuditLog
	done  chan struct{}
}

// NewAsyncLogger wraps inner with a channel of the given buffer size.
// The background drain goroutine starts immediately.
func NewAsyncLogger(inner AuditLogger, bufSize int) *AsyncLogger {
	al := &AsyncLogger{
		inner: inner,
		ch:    make(chan AuditLog, bufSize),
		done:  make(chan struct{}),
	}
	go al.drain()
	return al
}

// Log enqueues entry without blocking. If the buffer is full the entry is
// dropped and a warning is written to stderr.
func (a *AsyncLogger) Log(entry AuditLog) error {
	select {
	case a.ch <- entry:
	default:
		fmt.Fprintf(os.Stderr,
			"audit: buffer full, dropping %s event for operator %q\n",
			entry.Action, entry.Operator)
	}
	return nil
}

// Close signals the worker to stop after draining remaining entries, waits for
// it to finish, then closes the inner logger. Must be called before process exit.
func (a *AsyncLogger) Close() error {
	close(a.ch)
	<-a.done
	return a.inner.Close()
}

func (a *AsyncLogger) drain() {
	defer close(a.done)
	for entry := range a.ch {
		if err := a.inner.Log(entry); err != nil {
			fmt.Fprintf(os.Stderr, "audit: write error: %v\n", err)
		}
	}
}
