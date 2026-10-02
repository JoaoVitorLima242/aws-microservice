// Package orders contains the order domain model, the use case that creates
// orders and the interfaces of its dependencies.
package orders

import (
	"context"
	"errors"
	"time"

	"github.com/shopspring/decimal"
)

func init() {
	// Serialise money as JSON numbers (19.9) instead of strings ("19.9").
	decimal.MarshalJSONWithoutQuotes = true
}

const StatusCreated = "CREATED"

type Order struct {
	ID             string          `json:"id"`
	ProductID      string          `json:"productId"`
	Quantity       int             `json:"quantity"`
	UnitPrice      decimal.Decimal `json:"unitPrice"`
	Total          decimal.Decimal `json:"total"`
	CustomerEmail  string          `json:"customerEmail"`
	Status         string          `json:"status"`
	CreatedAt      time.Time       `json:"createdAt"`
	EventPublished bool            `json:"eventPublished"`
}

// ErrNotFound is returned by a Repository when the order does not exist.
var ErrNotFound = errors.New("order not found")

// Repository persists orders. Implementations live in the storage package.
type Repository interface {
	Create(ctx context.Context, o Order) error
	Get(ctx context.Context, id string) (Order, error)
	List(ctx context.Context) ([]Order, error)
	MarkEventPublished(ctx context.Context, id string) error
}

// Product is the subset of catalog data the orders service needs.
type Product struct {
	ID     string          `json:"id"`
	Name   string          `json:"name"`
	Price  decimal.Decimal `json:"price"`
	Active bool            `json:"active"`
}

var (
	// ErrProductNotFound means the catalog answered 404 for the product.
	ErrProductNotFound = errors.New("product not found")
	// ErrProductInactive means the product exists but is not for sale.
	ErrProductInactive = errors.New("product is not active")
	// ErrCatalogUnavailable means the catalog could not be reached in time.
	ErrCatalogUnavailable = errors.New("catalog service unavailable")
)

// Catalog looks up products in the catalog service.
type Catalog interface {
	GetProduct(ctx context.Context, id string) (Product, error)
}

const EventTypeOrderCreated = "OrderCreated"

// OrderCreatedEvent is the message published to the queue.
type OrderCreatedEvent struct {
	EventType  string           `json:"eventType"`
	EventID    string           `json:"eventId"`
	OccurredAt time.Time        `json:"occurredAt"`
	RequestID  string           `json:"requestId"`
	Data       OrderCreatedData `json:"data"`
}

type OrderCreatedData struct {
	OrderID       string          `json:"orderId"`
	ProductID     string          `json:"productId"`
	Quantity      int             `json:"quantity"`
	Total         decimal.Decimal `json:"total"`
	CustomerEmail string          `json:"customerEmail"`
}

// Publisher sends events to the message queue.
type Publisher interface {
	Publish(ctx context.Context, evt OrderCreatedEvent) error
}
