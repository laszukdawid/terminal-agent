package utils

import (
	"go.uber.org/zap"
)

var Logger *zap.Logger
var Loglevel string

func InitLogger(loglevel *string) (*zap.Logger, error) {
	var err error
	Loglevel = *loglevel

	switch *loglevel {
	case zap.DebugLevel.String():
		cfg := zap.NewDevelopmentConfig()
		cfg.Level.SetLevel(zap.DebugLevel)
		cfg.DisableStacktrace = true
		Logger, err = cfg.Build()

	case zap.WarnLevel.String():
		cfg := zap.NewProductionConfig()
		cfg.Level.SetLevel(zap.WarnLevel)
		cfg.DisableStacktrace = true
		Logger, err = cfg.Build()
	case zap.InfoLevel.String():
		cfg := zap.NewProductionConfig()
		cfg.Level.SetLevel(zap.InfoLevel)
		cfg.DisableStacktrace = true
		Logger, err = cfg.Build()
	default:
		Logger, err = zap.NewDevelopment()
	}
	defer Logger.Sync()

	return Logger, err
}

func GetLogger() *zap.Logger {
	if Logger == nil {
		if Logger == nil {
			// Default log level
			Loglevel = zap.DebugLevel.String()
		}
		Logger, _ := InitLogger(&Loglevel)
		return Logger
	}
	return Logger
}

func Sugar() *zap.SugaredLogger {
	return GetLogger().Sugar()
}

func Debug(msg string, fields ...zap.Field) {
	GetLogger().Debug(msg, fields...)
}

func Info(msg string, fields ...zap.Field) {
	GetLogger().Info(msg, fields...)
}

func Warn(msg string, fields ...zap.Field) {
	GetLogger().Warn(msg, fields...)
}

func Error(msg string, fields ...zap.Field) {
	GetLogger().Error(msg, fields...)
}

func Debugw(msg string, keysAndValues ...any) {
	Sugar().Debugw(msg, keysAndValues...)
}

func Infow(msg string, keysAndValues ...any) {
	Sugar().Infow(msg, keysAndValues...)
}

func Warnw(msg string, keysAndValues ...any) {
	Sugar().Warnw(msg, keysAndValues...)
}

func Errorw(msg string, keysAndValues ...any) {
	Sugar().Errorw(msg, keysAndValues...)
}
