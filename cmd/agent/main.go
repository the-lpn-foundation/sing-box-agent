package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/oglenyaboss/sing-box-agent/internal/client"
	"github.com/oglenyaboss/sing-box-agent/internal/config"
	"github.com/oglenyaboss/sing-box-agent/internal/metrics"
	"github.com/oglenyaboss/sing-box-agent/internal/server"
	"github.com/oglenyaboss/sing-box-agent/internal/singbox"
)

var (
	Version   = "dev"
	BuildTime = "unknown"
	GitCommit = "unknown"
)

func main() {
	configPath := flag.String("config", config.DefaultConfigPath, "path to config file")
	showVersion := flag.Bool("version", false, "show version")
	flag.Parse()

	if *showVersion {
		fmt.Printf("sing-box-agent: %s (%s) built %s\n", Version, GitCommit, BuildTime)
		return
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	if err := cfg.Validate(); err != nil {
		log.Fatalf("invalid config: %v", err)
	}

	logger := newLogger(cfg.LogLevel)
	logger.Info("starting sing-box-agent",
		slog.String("version", Version),
		slog.String("commit", GitCommit),
		slog.String("build_time", BuildTime),
		slog.String("config", *configPath),
	)

	// sing-box is managed by systemd externally. Do not start or stop it from the agent.
	wrapper := singbox.NewWrapper(cfg.SingBoxConfigPath)

	serverOpts := server.ServerOptions{Version: Version}

	// Attach a v2ray_api stats client to the /stats endpoints so they return
	// real per-inbound and per-user counters (best-effort; disabled if the
	// sing-box v2ray_api listener is unreachable).
	if cfg.StatsAPIAddress != "" {
		statsClient, err := singbox.NewV2RayStatsClient(cfg.StatsAPIAddress)
		if err != nil {
			logger.Warn("v2ray stats api unavailable, /stats endpoints report zeroes",
				slog.String("error", err.Error()))
		} else {
			serverOpts.StatsClient = statsClient
		}
	}

	// Create FastifyClient if central API is configured
	if cfg.FastifyBaseURL != "" {
		fastifyClient, err := client.NewFastifyClient(client.Options{
			BaseURL: cfg.FastifyBaseURL,
			Token:   cfg.Token,
			Secret:  cfg.Secret,
		})
		if err != nil {
			log.Fatalf("failed to create fastify client: %v", err)
		}
		serverOpts.FastifyClient = fastifyClient
		logger.Info("fastify integration enabled", slog.String("base_url", cfg.FastifyBaseURL))
	} else {
		logger.Info("running in standalone mode (no central API)")
	}

	srv := server.New(cfg, logger, wrapper, serverOpts)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start inbound traffic metrics collection (best-effort; disabled if the
	// sing-box v2ray_api is unreachable).
	go func() {
		configClient := singbox.NewConfigClient(cfg.SingBoxConfigPath, wrapper)
		if err := metrics.RunStatsLoop(ctx, logger, cfg.StatsAPIAddress, configClient); err != nil {
			logger.Warn("stats loop failed", slog.Any("error", err))
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	serverErrCh := make(chan error, 1)
	go func() {
		serverErrCh <- srv.Start()
	}()

	select {
	case sig := <-sigCh:
		logger.Info("received shutdown signal", slog.String("signal", sig.String()))
	case err := <-serverErrCh:
		if err != nil {
			logger.Error("server stopped with error", slog.Any("error", err))
		}
		cancel()
	case <-ctx.Done():
	}

	logger.Info("shutting down")
	if err := srv.Shutdown(); err != nil && !errors.Is(err, context.Canceled) {
		logger.Error("server shutdown failed", slog.Any("error", err))
	}

	// Do not stop sing-box here; systemd manages the service lifecycle.

	logger.Info("goodbye")
}

func newLogger(level string) *slog.Logger {
	logLevel := new(slog.LevelVar)
	switch level {
	case "debug":
		logLevel.Set(slog.LevelDebug)
	case "warn":
		logLevel.Set(slog.LevelWarn)
	case "error":
		logLevel.Set(slog.LevelError)
	default:
		logLevel.Set(slog.LevelInfo)
	}

	handler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel})
	return slog.New(handler)
}
