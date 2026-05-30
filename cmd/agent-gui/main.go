package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"fyne.io/fyne/v2"
	"github.com/laszukdawid/terminal-agent/internal/app"
	"github.com/laszukdawid/terminal-agent/internal/config"
	"github.com/laszukdawid/terminal-agent/internal/gui"
	"github.com/laszukdawid/terminal-agent/internal/platform"
	u "github.com/laszukdawid/terminal-agent/internal/utils"
	"go.uber.org/zap"
)

const guiAppID = "terminal-agent-gui"
const fyneAppID = "com.terminal-agent.popup"

func main() {
	show := flag.Bool("show", false, "show an existing popup or start a visible primary instance")
	newInstance := flag.Bool("new", false, "start a new isolated GUI instance")
	devMode := flag.Bool("dev", false, "enable developer tools (adds a Test button for rendering checks)")
	flag.Parse()

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
	runtimeAppID := guiAppID
	windowAppID := fyneAppID
	if *newInstance {
		suffix := strconv.Itoa(os.Getpid())
		runtimeAppID = guiAppID + "-" + suffix
		windowAppID = fyneAppID + "." + suffix
	}

	instance, err := platform.Acquire(runtimeAppID)
	if err != nil {
		if *newInstance {
			logger.Error("failed to acquire GUI instance lock for new instance", zap.Error(err))
			fmt.Println("Failed to start new GUI instance")
			os.Exit(1)
		}
		if errors.Is(err, platform.ErrAlreadyRunning) {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			if err := platform.Send(ctx, filepath.Join(platform.RuntimeDir(guiAppID), "ipc.sock"), platform.CommandShow); err != nil {
				logger.Error("failed to signal existing GUI instance", zap.Error(err))
				fmt.Println("Failed to signal existing GUI instance")
				os.Exit(1)
			}
			return
		}
		logger.Error("failed to acquire GUI instance lock", zap.Error(err))
		fmt.Println("Failed to start GUI")
		os.Exit(1)
	}
	defer instance.Close()

	if err := platform.PrepareSocket(instance.SocketPath); err != nil {
		logger.Error("failed to prepare GUI socket", zap.Error(err))
		fmt.Println("Failed to start GUI")
		os.Exit(1)
	}
	defer os.Remove(instance.SocketPath)

	guiApp := gui.NewApp(service, cfg, windowAppID, *devMode)
	server, err := platform.Listen(instance.SocketPath, func(command string) error {
		switch command {
		case platform.CommandShow:
			fyne.Do(guiApp.Show)
			return nil
		case platform.CommandHide:
			fyne.Do(guiApp.Hide)
			return nil
		default:
			return fmt.Errorf("unsupported command: %s", command)
		}
	})
	if err != nil {
		logger.Error("failed to start GUI IPC server", zap.Error(err))
		fmt.Println("Failed to start GUI")
		os.Exit(1)
	}
	defer server.Close()

	sigCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-sigCtx.Done()
		server.Close()
		fyne.Do(guiApp.Hide)
	}()

	if *show {
		logger.Debug("starting visible GUI from --show")
	}
	guiApp.Run()
}
