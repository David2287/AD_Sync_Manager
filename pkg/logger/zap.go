package logger

import (
	"fmt"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"ad-sync-manager/internal/domain/interfaces"
)

// zapLogger adapts go.uber.org/zap to the interfaces.Logger port.
type zapLogger struct {
	sugar *zap.SugaredLogger
}

// New constructs a production-ready zap logger.
//
//	level:  "debug" | "info" | "warn" | "error"
//	format: "json"  | "console"
func New(level, format string) (interfaces.Logger, error) {
	var lvl zapcore.Level
	if err := lvl.UnmarshalText([]byte(level)); err != nil {
		return nil, fmt.Errorf("logger: unknown level %q: %w", level, err)
	}

	cfg := zap.NewProductionConfig()
	cfg.Level = zap.NewAtomicLevelAt(lvl)
	if format == "console" {
		cfg.Encoding = "console"
		cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	}

	z, err := cfg.Build(zap.AddCallerSkip(1))
	if err != nil {
		return nil, fmt.Errorf("logger: build failed: %w", err)
	}

	return &zapLogger{sugar: z.Sugar()}, nil
}

func (l *zapLogger) Debug(msg string, kv ...any) { l.sugar.Debugw(msg, kv...) }
func (l *zapLogger) Info(msg string, kv ...any)  { l.sugar.Infow(msg, kv...) }
func (l *zapLogger) Warn(msg string, kv ...any)  { l.sugar.Warnw(msg, kv...) }
func (l *zapLogger) Error(msg string, kv ...any) { l.sugar.Errorw(msg, kv...) }
func (l *zapLogger) Sync() error                 { return l.sugar.Sync() }

func (l *zapLogger) With(kv ...any) interfaces.Logger {
	return &zapLogger{sugar: l.sugar.With(kv...)}
}
