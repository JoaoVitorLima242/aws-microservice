// Package config reads the service configuration from environment variables.
package config

import (
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	ServiceName string
	LogLevel    string
	AppVersion  string
	Port        int
}

func Load() (Config, error) {
	cfg := Config{
		ServiceName: getenv("SERVICE_NAME", "catalog-api"),
		LogLevel:    getenv("LOG_LEVEL", "info"),
		AppVersion:  getenv("APP_VERSION", "dev"),
	}

	port, err := strconv.Atoi(getenv("PORT", "8080"))
	if err != nil || port < 1 || port > 65535 {
		return Config{}, fmt.Errorf("PORT must be a number between 1 and 65535, got %q", os.Getenv("PORT"))
	}
	cfg.Port = port

	return cfg, nil
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
