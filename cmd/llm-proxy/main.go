package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/omegaatt36/llm-proxy/app/server"
	"github.com/omegaatt36/llm-proxy/config"
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, nil)))

	config, err := config.Load()
	if err != nil {
		slog.Error(fmt.Sprintf("Failed to load configuration: %v", err))
		os.Exit(1)
	}

	if config.LogLevel != "" {
		var level slog.Level
		if err := level.UnmarshalText([]byte(config.LogLevel)); err != nil {
			slog.Error("Invalid log level", "level", config.LogLevel, "error", err)
			os.Exit(1)
		}

		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: level,
		})))
	}

	slog.Debug(fmt.Sprintf("Configuration loaded: %+v", config))
	slog.Info(fmt.Sprintf("Model mappings: %v", config.ModelMappings))

	proxyServer, err := server.NewProxyServer(config, nil)
	if err != nil {
		slog.Error(fmt.Sprintf("Failed to create proxy server: %v", err))
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := proxyServer.Start(ctx); err != nil {
		slog.Error(fmt.Sprintf("Failed to start proxy server: %v", err))
		os.Exit(1)
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	sig := <-sigChan
	slog.Info(fmt.Sprintf("Received signal: %v, shutting down...", sig))

	// Cancel context to gracefully shutdown
	cancel()
	<-time.After(time.Second * 3)
}
