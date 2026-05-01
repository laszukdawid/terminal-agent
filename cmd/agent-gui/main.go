package main

import (
	"fmt"
	"os"

	"github.com/laszukdawid/terminal-agent/internal/app"
	"github.com/laszukdawid/terminal-agent/internal/config"
	"github.com/laszukdawid/terminal-agent/internal/gui"
	u "github.com/laszukdawid/terminal-agent/internal/utils"
	"go.uber.org/zap"
)

func main() {
	loglevel := zap.InfoLevel.String()
	logger, err := u.InitLogger(&loglevel)
	if err != nil {
		fmt.Println("Failed to initialize logger")
		os.Exit(1)
	}
	defer logger.Sync()

	cfg, err := config.LoadConfig()
	if err != nil {
		cfg = config.NewDefaultConfig()
	}

	service := app.NewService()
	guiApp := gui.NewApp(service, cfg)
	guiApp.Run()
}
