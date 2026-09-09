package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/oglenyaboss/sing-box-agent/internal/auth"
	"github.com/oglenyaboss/sing-box-agent/internal/client"
	"github.com/oglenyaboss/sing-box-agent/internal/config"
	"github.com/oglenyaboss/sing-box-agent/internal/handlers"
	"github.com/oglenyaboss/sing-box-agent/internal/middleware"
	"github.com/oglenyaboss/sing-box-agent/internal/singbox"
	"github.com/oglenyaboss/sing-box-agent/internal/store"
	syncpkg "github.com/oglenyaboss/sing-box-agent/internal/sync"
)

const (
	// DefaultReadTimeout is the default read timeout for HTTP connections.
	DefaultReadTimeout = 15 * time.Second
	// DefaultWriteTimeout is the default write timeout for HTTP connections.
	DefaultWriteTimeout = 30 * time.Second
	// DefaultIdleTimeout is the default idle timeout for HTTP connections.
	DefaultIdleTimeout = 60 * time.Second
	// DefaultShutdownTimeout is the default timeout for graceful shutdown.
	DefaultShutdownTimeout = 30 * time.Second
)

// Server wraps http.Server with graceful shutdown and signal handling.
type Server struct {
	httpServer    *http.Server
	metricsServer *http.Server
	logger        *slog.Logger
	config        *config.Config
	startTime     time.Time
	shutdownMu    sync.Mutex
	shutdown      bool
	syncEngine    *syncpkg.Engine
	version       string
	lifecycle     *Lifecycle
}

// ServerOptions holds optional dependencies for the server.
type ServerOptions struct {
	Version       string
	FastifyClient *client.Client
	StatsClient   *singbox.V2RayStatsClient
}

// New creates a new HTTP server with the given config and logger.
func New(cfg *config.Config, logger *slog.Logger, opts ...ServerOptions) *Server {
	startTime := time.Now()
	var version string
	var fastifyClient *client.Client
	var statsClient *singbox.V2RayStatsClient
	if len(opts) > 0 {
		version = opts[0].Version
		fastifyClient = opts[0].FastifyClient
		statsClient = opts[0].StatsClient
	}
	if version == "" {
		version = "dev"
	}

	mux := http.NewServeMux()

	nonceCache := auth.NewNonceCache()
	authMiddleware := middleware.AuthMiddleware(middleware.AuthConfig{
		Token:      cfg.Token,
		Secret:     cfg.Secret,
		NonceCache: nonceCache,
		Logger:     logger,
	})

	// Idempotency protection for mutating protected routes (no-op for GET
	// and for requests without an Idempotency-Key header).
	idemStore := store.NewIdempotencyStore()
	idemMiddleware := middleware.IdempotencyMiddleware(idemStore)

	configClient := singbox.NewConfigClient(cfg.SingBoxConfigPath)
	reloader, err := buildReloader(cfg)
	if err != nil {
		logger.Error("invalid reload configuration, falling back to systemctl",
			slog.String("error", err.Error()))
		reloader = syncpkg.NewSystemctlReloader("sing-box")
	}
	// Inject the reloader into ConfigClient so that REST CRUD paths
	// (POST/PUT/DELETE /inbounds/{tag}/users) respect reload_strategy
	// and benefit from debounced reloads — same as /sync/desired-state.
	configClient = configClient.WithReloader(reloader)
	configManager := syncpkg.NewConfigManagerWithReloader(cfg.SingBoxConfigPath, reloader)
	syncEngine := syncpkg.NewEngineWithConfigManager(configClient, configManager)

	inboundHandler := handlers.NewInboundHandler(configClient, logger)
	userHandler := handlers.NewUserHandlerWithClient(configClient)
	subscriptionHandler := handlers.NewSubscriptionHandler(logger, configClient)
	syncHandler := handlers.NewSyncHandler(syncEngine, configClient, logger)

	coreService := singbox.NewCoreServiceAdapter(cfg.SingBoxConfigPath)
	if strings.EqualFold(cfg.ReloadStrategy, "signal") && cfg.ReloadTarget != "" {
		coreService = coreService.WithPIDFile(cfg.ReloadTarget)
	}
	// Inject the reloader into CoreService so POST /core/reload respects
	// reload_strategy instead of the hardcoded systemctl path.
	coreService = coreService.WithReloader(reloader)
	coreHandler := handlers.NewCoreHandler(coreService, logger)

	statsProvider := singbox.NewStatsProviderAdapter(configClient)
	if statsClient != nil {
		statsProvider = statsProvider.WithStatsClient(statsClient)
	}
	statsHandler := handlers.NewStatsHandler(statsProvider, logger)

	// Public endpoints (no auth)
	mux.HandleFunc("/healthz", HealthHandler)
	mux.HandleFunc("/readyz", ReadyzHandler(syncEngine, coreService))
	mux.HandleFunc("/status", StatusHandler(startTime, version, syncEngine))

	mux.Handle("/metrics", MetricsHandler(cfg.MetricsUsername, cfg.MetricsPassword))

	// Protected endpoints (auth required)
	mux.Handle("/inbounds", authMiddleware(idemMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			inboundHandler.ListInbounds(w, r)
		case http.MethodPost:
			inboundHandler.CreateInbound(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))))
	mux.Handle("/inbounds/", authMiddleware(idemMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rest := strings.TrimPrefix(r.URL.Path, "/inbounds/")
		if rest == "" {
			http.NotFound(w, r)
			return
		}

		parts := strings.Split(rest, "/")
		switch {
		case len(parts) == 1:
			switch r.Method {
			case http.MethodGet:
				inboundHandler.GetInbound(w, r)
			case http.MethodPut:
				inboundHandler.UpdateInbound(w, r)
			case http.MethodDelete:
				inboundHandler.DeleteInbound(w, r)
			default:
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			}
		case len(parts) == 2 && parts[1] == "users":
			switch r.Method {
			case http.MethodGet:
				userHandler.ListUsers(w, r)
			case http.MethodPost:
				userHandler.CreateUser(w, r)
			default:
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			}
		case len(parts) == 3 && parts[1] == "users":
			switch r.Method {
			case http.MethodGet:
				userHandler.GetUser(w, r)
			case http.MethodPut:
				userHandler.UpdateUser(w, r)
			case http.MethodDelete:
				userHandler.DeleteUser(w, r)
			default:
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			}
		default:
			http.NotFound(w, r)
		}
	}))))

	// Stats endpoints
	mux.Handle("/stats/traffic", authMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		statsHandler.GetTrafficStats(w, r)
	})))
	mux.Handle("/stats/online", authMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		statsHandler.GetOnlineUsers(w, r)
	})))

	// Sync endpoints
	mux.Handle("/sync/desired-state", authMiddleware(idemMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		syncHandler.ApplyDesiredState(w, r)
	}))))
	mux.Handle("/sync/status", authMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		syncHandler.GetSyncStatus(w, r)
	})))

	// Core endpoints
	mux.Handle("/core/reload", authMiddleware(idemMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		coreHandler.ReloadConfig(w, r)
	}))))
	mux.Handle("/core/restart", authMiddleware(idemMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		coreHandler.RestartCore(w, r)
	}))))
	mux.Handle("/core/config", authMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		coreHandler.GetConfig(w, r)
	})))

	// Subscription endpoints
	mux.Handle("/subscription/generate", authMiddleware(idemMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		subscriptionHandler.GenerateSubscription(w, r)
	}))))
	mux.Handle("/subscription/", authMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		subscriptionHandler.GetSubscription(w, r)
	})))

	handler := LoggingMiddleware(logger)(mux)

	lifecycle := NewLifecycle(fastifyClient, syncEngine, logger, version)

	var metricsServer *http.Server
	if cfg.MetricsPort > 0 {
		metricsServer = &http.Server{
			Addr:         fmt.Sprintf(":%d", cfg.MetricsPort),
			Handler:      MetricsHandler(cfg.MetricsUsername, cfg.MetricsPassword),
			ReadTimeout:  DefaultReadTimeout,
			WriteTimeout: DefaultWriteTimeout,
			IdleTimeout:  DefaultIdleTimeout,
		}
	}

	return &Server{
		httpServer: &http.Server{
			Addr:         fmt.Sprintf(":%d", cfg.APIPort),
			Handler:      handler,
			ReadTimeout:  DefaultReadTimeout,
			WriteTimeout: DefaultWriteTimeout,
			IdleTimeout:  DefaultIdleTimeout,
		},
		metricsServer: metricsServer,
		logger:        logger,
		config:        cfg,
		startTime:     startTime,
		syncEngine:    syncEngine,
		version:       version,
		lifecycle:     lifecycle,
	}
}

// Start starts the HTTP server with graceful shutdown support.
// It blocks until the server is stopped or an error occurs.
func (s *Server) Start() error {
	s.logger.Info("starting server",
		slog.Int("port", s.config.APIPort),
		slog.String("address", s.httpServer.Addr),
	)

	listener, err := net.Listen("tcp", s.httpServer.Addr)
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}

	// Setup signal handling for graceful shutdown
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Channel for server errors
	serverErr := make(chan error, 1)

	// Start lifecycle tasks (heartbeat, drift check)
	if s.lifecycle != nil {
		s.lifecycle.Start(ctx)
	}

	go func() {
		// Start serving
		if s.config.TLSCertPath != "" && s.config.TLSKeyPath != "" {
			s.logger.Info("serving with TLS",
				slog.String("cert", s.config.TLSCertPath),
				slog.String("key", s.config.TLSKeyPath),
			)
			serverErr <- s.httpServer.ServeTLS(listener, s.config.TLSCertPath, s.config.TLSKeyPath)
		} else {
			s.logger.Info("serving without TLS")
			serverErr <- s.httpServer.Serve(listener)
		}
	}()

	// Start the dedicated metrics server in the background. A bind/serve
	// failure must never take the agent down — log a warning and continue.
	if s.metricsServer != nil {
		go func() {
			if err := s.metricsServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				s.logger.Warn("metrics server failed", slog.Any("error", err))
			}
		}()
	}

	// Wait for shutdown signal or server error
	select {
	case <-ctx.Done():
		s.logger.Info("shutdown signal received")
		return s.shutdownServer()
	case err := <-serverErr:
		if errors.Is(err, http.ErrServerClosed) {
			s.logger.Info("server closed")
			return nil
		}
		return fmt.Errorf("server error: %w", err)
	}
}

// shutdownServer performs graceful shutdown with timeout.
func (s *Server) shutdownServer() error {
	s.shutdownMu.Lock()
	defer s.shutdownMu.Unlock()

	if s.shutdown {
		return nil
	}
	s.shutdown = true

	shutdownCtx, cancel := context.WithTimeout(context.Background(), DefaultShutdownTimeout)
	defer cancel()

	// Stop lifecycle tasks first
	if s.lifecycle != nil {
		s.lifecycle.Stop()
	}

	s.logger.Info("shutting down server", slog.Duration("timeout", DefaultShutdownTimeout))

	if err := s.httpServer.Shutdown(shutdownCtx); err != nil {
		s.logger.Error("shutdown error", slog.Any("error", err))
		return fmt.Errorf("shutdown error: %w", err)
	}

	// Shut down the metrics server with the same timeout. Failures are
	// logged but must not mask a successful API server shutdown.
	if s.metricsServer != nil {
		if err := s.metricsServer.Shutdown(shutdownCtx); err != nil {
			s.logger.Warn("metrics server shutdown error", slog.Any("error", err))
		}
	}

	s.logger.Info("server shutdown complete")
	return nil
}

// Shutdown performs manual shutdown of the server.
func (s *Server) Shutdown() error {
	return s.shutdownServer()
}

// Addr returns the server's listening address.
func (s *Server) Addr() string {
	return s.httpServer.Addr
}

// buildReloader constructs the sing-box reloader selected by the configuration.
// Defaults to systemctl when the strategy is unset, matching legacy behaviour.
func buildReloader(cfg *config.Config) (syncpkg.Reloader, error) {
	strategy := strings.ToLower(strings.TrimSpace(cfg.ReloadStrategy))
	target := strings.TrimSpace(cfg.ReloadTarget)

	switch strategy {
	case "", "systemctl":
		if target == "" {
			target = "sing-box"
		}
		return syncpkg.NewSystemctlReloader(target), nil
	case "signal":
		if target == "" {
			return nil, fmt.Errorf("reload_strategy=signal requires reload_target to point at a PID file")
		}
		return syncpkg.NewSignalReloader(target, syscall.SIGHUP), nil
	case "command":
		if strings.TrimSpace(cfg.ReloadCommand) == "" {
			return nil, fmt.Errorf("reload_strategy=command requires reload_command to be set")
		}
		return syncpkg.NewCommandReloader(cfg.ReloadCommand), nil
	default:
		return nil, fmt.Errorf("unknown reload_strategy %q (expected systemctl|signal|command)", cfg.ReloadStrategy)
	}
}
