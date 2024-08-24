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
		Logger, err = zap.NewDevelopment()
	case zap.WarnLevel.String():
		cfg := zap.NewProductionConfig()
		cfg.Level.SetLevel(zap.WarnLevel)
		Logger, err = cfg.Build()
	case zap.InfoLevel.String():
		Logger, err = zap.NewProduction()
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
