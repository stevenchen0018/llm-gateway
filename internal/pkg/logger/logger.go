// Package logger provides a process-wide structured (zap) logger.
package logger

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// New builds a zap.Logger configured by level ("debug", "info", "warn",
// "error") and format ("json" or "console").
func New(level, format string) (*zap.Logger, error) {
	var zapLevel zapcore.Level
	if err := zapLevel.UnmarshalText([]byte(level)); err != nil {
		zapLevel = zapcore.InfoLevel
	}

	encoderCfg := zap.NewProductionEncoderConfig()
	encoderCfg.TimeKey = "ts"
	encoderCfg.EncodeTime = zapcore.ISO8601TimeEncoder

	cfg := zap.Config{
		Level:            zap.NewAtomicLevelAt(zapLevel),
		Development:      false,
		Encoding:         format,
		EncoderConfig:    encoderCfg,
		OutputPaths:      []string{"stdout"},
		ErrorOutputPaths: []string{"stderr"},
	}
	if format != "json" {
		cfg.Encoding = "console"
		encoderCfg.EncodeLevel = zapcore.CapitalColorLevelEncoder
	}

	return cfg.Build()
}
