package worker

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const eventTypeOrderCreated = "OrderCreated"

// OrderCreatedEvent mirrors the message published by orders-api.
type OrderCreatedEvent struct {
	EventType  string           `json:"eventType"`
	EventID    string           `json:"eventId"`
	OccurredAt time.Time        `json:"occurredAt"`
	RequestID  string           `json:"requestId"`
	Data       OrderCreatedData `json:"data"`
}

type OrderCreatedData struct {
	OrderID       string      `json:"orderId"`
	ProductID     string      `json:"productId"`
	Quantity      int         `json:"quantity"`
	Total         json.Number `json:"total"`
	CustomerEmail string      `json:"customerEmail"`
}

func (e OrderCreatedEvent) validate() error {
	var problems []string
	if e.EventType != eventTypeOrderCreated {
		problems = append(problems, fmt.Sprintf("unsupported eventType %q", e.EventType))
	}
	if e.EventID == "" {
		problems = append(problems, "eventId is required")
	}
	if e.Data.OrderID == "" {
		problems = append(problems, "data.orderId is required")
	}
	if e.Data.CustomerEmail == "" {
		problems = append(problems, "data.customerEmail is required")
	}
	if len(problems) > 0 {
		return errors.New(strings.Join(problems, "; "))
	}
	return nil
}
