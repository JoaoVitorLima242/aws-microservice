// Package catalog is the HTTP client for catalog-api.
package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"shop-aws/services/orders-api/internal/httpx"
	"shop-aws/services/orders-api/internal/logging"
	"shop-aws/services/orders-api/internal/orders"
)

const (
	attemptTimeout = 2 * time.Second
	maxAttempts    = 2 // first try + 1 retry
	retryBackoff   = 100 * time.Millisecond
)

type Client struct {
	baseURL string
	http    *http.Client
	logger  *slog.Logger
}

func NewClient(baseURL string, logger *slog.Logger) *Client {
	return &Client{baseURL: baseURL, http: &http.Client{}, logger: logger}
}

// GetProduct fetches a product. It returns orders.ErrProductNotFound on 404
// and wraps orders.ErrCatalogUnavailable on network errors, timeouts or
// unexpected responses (after one retry).
func (c *Client) GetProduct(ctx context.Context, id string) (orders.Product, error) {
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		p, retryable, err := c.fetch(ctx, id)
		if err == nil || errors.Is(err, orders.ErrProductNotFound) {
			return p, err
		}
		lastErr = err
		if !retryable || ctx.Err() != nil {
			break
		}
		c.logger.WarnContext(ctx, "catalog request failed",
			slog.String("productId", id),
			slog.Int("attempt", attempt),
			slog.Any("error", err),
		)
		if attempt < maxAttempts {
			select {
			case <-time.After(retryBackoff):
			case <-ctx.Done():
			}
		}
	}
	return orders.Product{}, fmt.Errorf("%w: %v", orders.ErrCatalogUnavailable, lastErr)
}

func (c *Client) fetch(ctx context.Context, id string) (p orders.Product, retryable bool, err error) {
	ctx, cancel := context.WithTimeout(ctx, attemptTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/catalog/products/"+url.PathEscape(id), nil)
	if err != nil {
		return p, false, err
	}
	req.Header.Set("Accept", "application/json")
	if rid := logging.RequestID(ctx); rid != "" {
		req.Header.Set(httpx.RequestIDHeader, rid)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return p, true, err
	}
	defer func() {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
		resp.Body.Close()
	}()

	switch {
	case resp.StatusCode == http.StatusOK:
		if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&p); err != nil {
			return p, true, fmt.Errorf("decode catalog response: %w", err)
		}
		return p, false, nil
	case resp.StatusCode == http.StatusNotFound:
		return p, false, orders.ErrProductNotFound
	case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500:
		return p, true, fmt.Errorf("catalog returned status %d", resp.StatusCode)
	default:
		return p, false, fmt.Errorf("catalog returned unexpected status %d", resp.StatusCode)
	}
}
