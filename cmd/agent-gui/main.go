package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/debug"
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

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

const guiAppID = "terminal-agent-gui"
const fyneAppID = "com.terminal-agent.popup"

func resolveVersion() string {
	buildInfo, ok := debug.ReadBuildInfo()
	if ok {
		return selectVersion(version, buildInfo.Main.Version)
	}
	return selectVersion(version, "")
}

func displayVersion() string {
	resolved := resolveVersion()
	if len(resolved) > 0 && resolved[0] >= '0' && resolved[0] <= '9' {
		return "v" + resolved
	}
	return resolved
}

func selectVersion(linkerVersion, buildInfoVersion string) string {
	if linkerVersion != "" && linkerVersion != "dev" && linkerVersion != "(devel)" {
		return linkerVersion
	}
	if buildInfoVersion != "" && buildInfoVersion != "(devel)" {
		return buildInfoVersion
	}
	return "unknown"
}

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
			if err := signalExistingGUI(ctx); err != nil {
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

	guiApp := gui.NewApp(service, cfg, gui.AppOptions{
		AppID:   windowAppID,
		DevMode: *devMode,
		Version: displayVersion(),
	})
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
	guiApp.LoadInitialEnvironment()

	sigCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-sigCtx.Done()
		server.Close()
		os.Exit(0)
	}()

	if *show {
		logger.Debug("starting visible GUI from --show")
	}
	guiApp.Run()
}

func signalExistingGUI(ctx context.Context) error {
	socketPath := filepath.Join(platform.RuntimeDir(guiAppID), "ipc.sock")
	for {
		err := platform.Send(ctx, socketPath, platform.CommandShow)
		if err == nil {
			return nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		select {
		case <-ctx.Done():
			return err
		case <-time.After(50 * time.Millisecond):
		}
	}
}
