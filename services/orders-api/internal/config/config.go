// Package config reads the service configuration from environment variables.
package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
)

const (
	StorageMemory   = "memory"
	StorageDynamoDB = "dynamodb"
)

type Config struct {
	ServiceName    string
	LogLevel       string
	AppVersion     string
	Port           int
	CatalogURL     string
	Storage        string
	OrdersTable    string
	QueueURL       string
	AWSRegion      string
	AWSEndpointURL string
}

// NeedsAWS reports whether any AWS client has to be created.
func (c Config) NeedsAWS() bool {
	return c.Storage == StorageDynamoDB || c.QueueURL != ""
}

func Load() (Config, error) {
	cfg := Config{
		ServiceName:    getenv("SERVICE_NAME", "orders-api"),
		LogLevel:       getenv("LOG_LEVEL", "info"),
		AppVersion:     getenv("APP_VERSION", "dev"),
		CatalogURL:     strings.TrimRight(os.Getenv("CATALOG_URL"), "/"),
		Storage:        strings.ToLower(getenv("STORAGE", StorageMemory)),
		OrdersTable:    os.Getenv("ORDERS_TABLE"),
		QueueURL:       os.Getenv("QUEUE_URL"),
		AWSRegion:      os.Getenv("AWS_REGION"),
		AWSEndpointURL: os.Getenv("AWS_ENDPOINT_URL"),
	}

	var errs []error

	port, err := strconv.Atoi(getenv("PORT", "8080"))
	if err != nil || port < 1 || port > 65535 {
		errs = append(errs, fmt.Errorf("PORT must be a number between 1 and 65535, got %q", os.Getenv("PORT")))
	}
	cfg.Port = port

	if cfg.CatalogURL == "" {
		errs = append(errs, errors.New("CATALOG_URL is required"))
	} else if u, err := url.Parse(cfg.CatalogURL); err != nil || u.Scheme == "" || u.Host == "" {
		errs = append(errs, fmt.Errorf("CATALOG_URL must be an absolute URL, got %q", cfg.CatalogURL))
	}

	switch cfg.Storage {
	case StorageMemory:
	case StorageDynamoDB:
		if cfg.OrdersTable == "" {
			errs = append(errs, errors.New("ORDERS_TABLE is required when STORAGE=dynamodb"))
		}
	default:
		errs = append(errs, fmt.Errorf("STORAGE must be %q or %q, got %q", StorageMemory, StorageDynamoDB, cfg.Storage))
	}

	if cfg.NeedsAWS() && cfg.AWSRegion == "" {
		errs = append(errs, errors.New("AWS_REGION is required when using DynamoDB or SQS"))
	}

	return cfg, errors.Join(errs...)
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
