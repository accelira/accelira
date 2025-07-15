package util

import (
	"os"
	"sync"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	logger     *zap.Logger
	loggerOnce sync.Once
)

// GetLogger returns a singleton zap.Logger instance.
func GetLogger() *zap.Logger {
	loggerOnce.Do(func() {
		cfg := zap.NewProductionConfig()
		if os.Getenv("ACCELIRA_DEBUG") == "1" {
			cfg = zap.NewDevelopmentConfig()
		}
		cfg.EncoderConfig.TimeKey = "timestamp"
		cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
		l, err := cfg.Build()
		if err != nil {
			panic(err)
		}
		logger = l
	})
	return logger
}

// SyncLogger flushes any buffered log entries.
func SyncLogger() {
	if logger != nil {
		_ = logger.Sync()
	}
}
