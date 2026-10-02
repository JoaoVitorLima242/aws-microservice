package main

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"shop-aws/services/catalog-api/internal/catalog"
	"shop-aws/services/catalog-api/internal/config"
	"shop-aws/services/catalog-api/internal/httpx"
	"shop-aws/services/catalog-api/internal/logging"
	"shop-aws/services/catalog-api/seed"
)

func main() {
	cfg, cfgErr := config.Load()

	logger, levelOK := logging.New(cfg.ServiceName, cfg.AppVersion, cfg.LogLevel)
	slog.SetDefault(logger)
	if cfgErr != nil {
		logger.Error("invalid configuration", slog.Any("error", cfgErr))
		os.Exit(1)
	}
	if !levelOK {
		logger.Warn("unknown LOG_LEVEL, falling back to info", slog.String("logLevel", cfg.LogLevel))
	}

	if err := run(cfg, logger); err != nil {
		logger.Error("service stopped with error", slog.Any("error", err))
		os.Exit(1)
	}
}

func run(cfg config.Config, logger *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
	defer stop()

	store, err := catalog.NewStore(seed.Products)
	if err != nil {
		return err
	}
	logger.Info("product catalog loaded", slog.Int("products", store.Len()))

	mux := http.NewServeMux()
	catalog.NewHandler(store, cfg.ServiceName, cfg.AppVersion).Register(mux)
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		httpx.WriteError(w, http.StatusNotFound, "route not found")
	})

	var handler http.Handler = mux
	handler = httpx.Recover(logger)(handler)
	handler = httpx.AccessLog(logger, "/catalog/health")(handler)
	handler = httpx.RequestID(handler)

	addr := net.JoinHostPort("0.0.0.0", strconv.Itoa(cfg.Port))
	return httpx.Serve(ctx, logger, addr, handler)
}
