package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/glebovdev/somafm-cli/internal/api"
	"github.com/glebovdev/somafm-cli/internal/cache"
	"github.com/glebovdev/somafm-cli/internal/config"
	"github.com/glebovdev/somafm-cli/internal/player"
	"github.com/glebovdev/somafm-cli/internal/service"
	"github.com/glebovdev/somafm-cli/internal/ui"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

var (
	versionFlag = flag.Bool("version", false, "Show version information")
	debugFlag   = flag.Bool("debug", false, "Enable debug logging")
	randomFlag  = flag.Bool("random", false, "Start with a random station")
)

func init() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "%s v%s - %s\n\n", config.AppName, config.AppVersion, config.AppDescription)
		fmt.Fprintf(os.Stderr, "Usage: %s [options]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()

		configPath, err := config.GetConfigPath()
		if err == nil {
			if _, statErr := os.Stat(configPath); statErr == nil {
				fmt.Fprintf(os.Stderr, "\nConfig file: %s\n", configPath)
			} else {
				fmt.Fprintf(os.Stderr, "\nConfig file will be created on first use.\n")
			}
		}
	}
}

func main() {
	flag.Parse()

	if *versionFlag {
		fmt.Printf("%s v%s\n", config.AppName, config.AppVersion)
		fmt.Println(config.AppDescription)
		os.Exit(0)
	}

	if *debugFlag {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)

		cacheDir, err := cache.GetCacheDir()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not get cache dir: %v\n", err)
			cacheDir = os.TempDir()
		}
		if err := os.MkdirAll(cacheDir, 0755); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not create log dir: %v\n", err)
		}
		logPath := filepath.Join(cacheDir, "debug.log")
		logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not create log file: %v\n", err)
			logFile = os.Stderr
		}
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: logFile, TimeFormat: "15:04:05"})
		fmt.Printf("Debug log: %s\n", logPath)
		log.Info().Msgf("Starting %s v%s (debug mode)", config.AppName, config.AppVersion)
	} else {
		// Avoid TUI corruption by only logging errors to /dev/null
		zerolog.SetGlobalLevel(zerolog.ErrorLevel)
		logFile, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0644)
		if err == nil {
			log.Logger = log.Output(logFile)
		}
	}

	if *debugFlag {
		if configPath, err := config.GetConfigPath(); err == nil {
			log.Debug().Msgf("Config: %s", configPath)
		}
		if cacheDir, err := cache.GetCacheDir(); err == nil {
			log.Debug().Msgf("Cache: %s", cacheDir)
		}
	}

	apiClient := api.NewSomaFMClient()
	stationService := service.NewStationService(apiClient)
	somaPlayer := player.NewPlayer()
	somaUi := ui.NewUI(somaPlayer, stationService, *randomFlag)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	uiDone := make(chan error, 1)

	go func() {
		<-sigChan
		if *debugFlag {
			log.Info().Msg("Received shutdown signal, cleaning up...")
		}
		somaUi.Shutdown()
	}()

	if *debugFlag {
		log.Info().Msg("Starting UI...")
	}

	// Run UI in a goroutine so we can handle signals properly
	go func() {
		uiDone <- somaUi.Run()
	}()

	if err := <-uiDone; err != nil {
		if *debugFlag {
			log.Error().Err(err).Msg("Error running UI")
		}
		somaPlayer.Stop()
		os.Exit(1)
	}

	// Ensure player is fully stopped before exiting
	somaPlayer.Stop()
	if *debugFlag {
		log.Info().Msg("SomaFM CLI stopped")
	}
}
