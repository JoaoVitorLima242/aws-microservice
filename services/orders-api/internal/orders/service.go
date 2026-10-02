package orders

import (
	"context"
	"fmt"
	"log/slog"
	"net/mail"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"shop-aws/services/orders-api/internal/logging"
)

// persistTimeout bounds saving the order and publishing its event. That work
// runs detached from the request context so a client disconnect does not
// leave an order half-processed.
const persistTimeout = 5 * time.Second

type CreateOrderInput struct {
	ProductID     string `json:"productId"`
	Quantity      int    `json:"quantity"`
	CustomerEmail string `json:"customerEmail"`
}

// ValidationError is returned when the input is invalid (HTTP 400).
type ValidationError struct {
	Problems []string
}

func (e *ValidationError) Error() string {
	return "invalid order: " + strings.Join(e.Problems, "; ")
}

func (in CreateOrderInput) validate() error {
	var problems []string
	if strings.TrimSpace(in.ProductID) == "" {
		problems = append(problems, "productId is required")
	}
	if in.Quantity < 1 {
		problems = append(problems, "quantity must be >= 1")
	}
	if !validEmail(in.CustomerEmail) {
		problems = append(problems, "customerEmail must be a valid e-mail address")
	}
	if len(problems) > 0 {
		return &ValidationError{Problems: problems}
	}
	return nil
}

// validEmail accepts a bare address (no display name) whose domain has a dot.
func validEmail(s string) bool {
	if s == "" || len(s) > 254 {
		return false
	}
	addr, err := mail.ParseAddress(s)
	if err != nil || addr.Address != s {
		return false
	}
	domain := s[strings.LastIndex(s, "@")+1:]
	return strings.Contains(domain, ".") && !strings.HasPrefix(domain, ".") && !strings.HasSuffix(domain, ".")
}

type Service struct {
	repo      Repository
	catalog   Catalog
	publisher Publisher
	logger    *slog.Logger
}

func NewService(repo Repository, catalog Catalog, publisher Publisher, logger *slog.Logger) *Service {
	return &Service{repo: repo, catalog: catalog, publisher: publisher, logger: logger}
}

// Create validates the input, prices the order using the catalog, saves it
// and publishes an OrderCreated event. A publish failure is logged but does
// not fail the operation: the order stays saved with eventPublished=false.
func (s *Service) Create(ctx context.Context, in CreateOrderInput) (Order, error) {
	in.ProductID = strings.TrimSpace(in.ProductID)
	in.CustomerEmail = strings.TrimSpace(in.CustomerEmail)
	if err := in.validate(); err != nil {
		return Order{}, err
	}

	product, err := s.catalog.GetProduct(ctx, in.ProductID)
	if err != nil {
		return Order{}, err
	}
	if !product.Active {
		return Order{}, ErrProductInactive
	}

	order := Order{
		ID:             uuid.NewString(),
		ProductID:      product.ID,
		Quantity:       in.Quantity,
		UnitPrice:      product.Price,
		Total:          product.Price.Mul(decimal.NewFromInt(int64(in.Quantity))),
		CustomerEmail:  in.CustomerEmail,
		Status:         StatusCreated,
		CreatedAt:      time.Now().UTC(),
		EventPublished: false,
	}

	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), persistTimeout)
	defer cancel()

	if err := s.repo.Create(ctx, order); err != nil {
		return Order{}, fmt.Errorf("save order: %w", err)
	}
	s.logger.InfoContext(ctx, "order created",
		slog.String("orderId", order.ID),
		slog.String("productId", order.ProductID),
		slog.Int("quantity", order.Quantity),
		slog.String("total", order.Total.String()),
	)

	evt := OrderCreatedEvent{
		EventType:  EventTypeOrderCreated,
		EventID:    uuid.NewString(),
		OccurredAt: time.Now().UTC(),
		RequestID:  logging.RequestID(ctx),
		Data: OrderCreatedData{
			OrderID:       order.ID,
			ProductID:     order.ProductID,
			Quantity:      order.Quantity,
			Total:         order.Total,
			CustomerEmail: order.CustomerEmail,
		},
	}
	if err := s.publisher.Publish(ctx, evt); err != nil {
		s.logger.ErrorContext(ctx, "failed to publish OrderCreated event",
			slog.String("orderId", order.ID),
			slog.String("eventId", evt.EventID),
			slog.Any("error", err),
		)
		return order, nil
	}

	if err := s.repo.MarkEventPublished(ctx, order.ID); err != nil {
		s.logger.ErrorContext(ctx, "event published but failed to set eventPublished=true",
			slog.String("orderId", order.ID),
			slog.String("eventId", evt.EventID),
			slog.Any("error", err),
		)
		return order, nil
	}
	order.EventPublished = true
	s.logger.InfoContext(ctx, "OrderCreated event published",
		slog.String("orderId", order.ID),
		slog.String("eventId", evt.EventID),
	)
	return order, nil
}

func (s *Service) Get(ctx context.Context, id string) (Order, error) {
	return s.repo.Get(ctx, id)
}

func (s *Service) List(ctx context.Context) ([]Order, error) {
	return s.repo.List(ctx)
}
