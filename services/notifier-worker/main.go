package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sqs"

	"shop-aws/services/notifier-worker/internal/config"
	"shop-aws/services/notifier-worker/internal/logging"
	"shop-aws/services/notifier-worker/internal/worker"
)

// shutdownTimeout is how long the current batch may take after SIGTERM
// before the process exits anyway (kept under 20 seconds).
const shutdownTimeout = 18 * time.Second

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
		logger.Error("worker stopped with error", slog.Any("error", err))
		os.Exit(1)
	}
	logger.Info("worker shut down cleanly")
}

func run(cfg config.Config, logger *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
	defer stop()

	// Credentials come from the SDK default chain (env, shared config,
	// ECS task role, EC2 instance profile...).
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(cfg.AWSRegion))
	if err != nil {
		return fmt.Errorf("load AWS config: %w", err)
	}
	client := sqs.NewFromConfig(awsCfg, func(o *sqs.Options) {
		if cfg.AWSEndpointURL != "" {
			o.BaseEndpoint = aws.String(cfg.AWSEndpointURL)
		}
	})

	w := worker.New(client, worker.Config{
		QueueURL:        cfg.QueueURL,
		MaxMessages:     cfg.MaxMessages,
		WaitTimeSeconds: cfg.WaitTimeSeconds,
		FailRate:        cfg.FailRate,
	}, logger)

	done := make(chan error, 1)
	go func() { done <- w.Run(ctx) }()

	select {
	case err := <-done:
		return err
	case <-ctx.Done():
	}

	logger.Info("shutdown signal received, finishing current batch")
	select {
	case err := <-done:
		return err
	case <-time.After(shutdownTimeout):
		return errors.New("shutdown timed out before the current batch finished")
	}
}
