// Package config reads the worker configuration from environment variables.
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	ServiceName     string
	LogLevel        string
	AppVersion      string
	QueueURL        string
	MaxMessages     int32
	WaitTimeSeconds int32
	FailRate        float64
	AWSRegion       string
	AWSEndpointURL  string
}

func Load() (Config, error) {
	cfg := Config{
		ServiceName:    getenv("SERVICE_NAME", "notifier-worker"),
		LogLevel:       getenv("LOG_LEVEL", "info"),
		AppVersion:     getenv("APP_VERSION", "dev"),
		QueueURL:       os.Getenv("QUEUE_URL"),
		AWSRegion:      os.Getenv("AWS_REGION"),
		AWSEndpointURL: os.Getenv("AWS_ENDPOINT_URL"),
	}

	var errs []error
	if cfg.QueueURL == "" {
		errs = append(errs, errors.New("QUEUE_URL is required"))
	}
	if cfg.AWSRegion == "" {
		errs = append(errs, errors.New("AWS_REGION is required"))
	}

	maxMessages, err := strconv.Atoi(getenv("MAX_MESSAGES", "10"))
	if err != nil || maxMessages < 1 || maxMessages > 10 {
		errs = append(errs, fmt.Errorf("MAX_MESSAGES must be an integer between 1 and 10, got %q", os.Getenv("MAX_MESSAGES")))
	}
	cfg.MaxMessages = int32(maxMessages)

	waitTime, err := strconv.Atoi(getenv("WAIT_TIME_SECONDS", "20"))
	if err != nil || waitTime < 0 || waitTime > 20 {
		errs = append(errs, fmt.Errorf("WAIT_TIME_SECONDS must be an integer between 0 and 20, got %q", os.Getenv("WAIT_TIME_SECONDS")))
	}
	cfg.WaitTimeSeconds = int32(waitTime)

	failRate, err := strconv.ParseFloat(getenv("FAIL_RATE", "0"), 64)
	if err != nil || failRate < 0 || failRate > 1 {
		errs = append(errs, fmt.Errorf("FAIL_RATE must be a number between 0 and 1, got %q", os.Getenv("FAIL_RATE")))
	}
	cfg.FailRate = failRate

	return cfg, errors.Join(errs...)
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
