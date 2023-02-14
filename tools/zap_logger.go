package tools

import (
	"github.com/blendle/zapdriver"
	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func NewZapLogger(logLevel zapcore.Level) (logr.Logger, zap.AtomicLevel, error) {
	cfg := zapdriver.NewProductionConfig()
	cfg.EncoderConfig.EncodeDuration = zapcore.StringDurationEncoder // we don't know why this is needed
	cfg.EncoderConfig.EncodeTime = zapcore.RFC3339NanoTimeEncoder
	cfg.Level.SetLevel(logLevel)
	logger, err := cfg.Build()
	if err != nil {
		return logr.Logger{}, zap.AtomicLevel{}, err
	}
	return zapr.NewLogger(logger), cfg.Level, nil
}
