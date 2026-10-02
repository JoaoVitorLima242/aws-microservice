package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/sqs"

	"shop-aws/services/orders-api/internal/catalog"
	"shop-aws/services/orders-api/internal/config"
	"shop-aws/services/orders-api/internal/events"
	"shop-aws/services/orders-api/internal/httpx"
	"shop-aws/services/orders-api/internal/logging"
	"shop-aws/services/orders-api/internal/orders"
	"shop-aws/services/orders-api/internal/storage"
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

	var awsCfg aws.Config
	if cfg.NeedsAWS() {
		var err error
		// Credentials come from the SDK default chain (env, shared config,
		// ECS task role, EC2 instance profile...).
		awsCfg, err = awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(cfg.AWSRegion))
		if err != nil {
			return fmt.Errorf("load AWS config: %w", err)
		}
	}

	var repo orders.Repository
	switch cfg.Storage {
	case config.StorageDynamoDB:
		client := dynamodb.NewFromConfig(awsCfg, func(o *dynamodb.Options) {
			if cfg.AWSEndpointURL != "" {
				o.BaseEndpoint = aws.String(cfg.AWSEndpointURL)
			}
		})
		repo = storage.NewDynamoDB(client, cfg.OrdersTable)
	default:
		repo = storage.NewMemory()
	}

	var publisher orders.Publisher
	if cfg.QueueURL != "" {
		client := sqs.NewFromConfig(awsCfg, func(o *sqs.Options) {
			if cfg.AWSEndpointURL != "" {
				o.BaseEndpoint = aws.String(cfg.AWSEndpointURL)
			}
		})
		publisher = events.NewSQSPublisher(client, cfg.QueueURL)
	} else {
		logger.Warn("QUEUE_URL is not set: OrderCreated events will not be published")
		publisher = events.UnconfiguredPublisher{}
	}

	logger.Info("orders-api starting",
		slog.String("storage", cfg.Storage),
		slog.String("catalogUrl", cfg.CatalogURL),
		slog.Bool("queueConfigured", cfg.QueueURL != ""),
		slog.Bool("customAwsEndpoint", cfg.AWSEndpointURL != ""),
	)

	svc := orders.NewService(repo, catalog.NewClient(cfg.CatalogURL, logger), publisher, logger)

	mux := http.NewServeMux()
	orders.NewHandler(svc, logger, cfg.ServiceName, cfg.AppVersion).Register(mux)
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		httpx.WriteError(w, http.StatusNotFound, "route not found")
	})

	var handler http.Handler = mux
	handler = httpx.Recover(logger)(handler)
	handler = httpx.AccessLog(logger, "/orders/health")(handler)
	handler = httpx.RequestID(handler)

	addr := net.JoinHostPort("0.0.0.0", strconv.Itoa(cfg.Port))
	return httpx.Serve(ctx, logger, addr, handler)
}
