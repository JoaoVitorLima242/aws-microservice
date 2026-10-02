// Package storage implements orders.Repository in memory and in DynamoDB.
package storage

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"shop-aws/services/orders-api/internal/orders"
)

// Memory keeps orders in a map. Data is lost on restart and is not shared
// between replicas; use it for local development only.
type Memory struct {
	mu     sync.RWMutex
	orders map[string]orders.Order
}

func NewMemory() *Memory {
	return &Memory{orders: make(map[string]orders.Order)}
}

func (m *Memory) Create(_ context.Context, o orders.Order) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.orders[o.ID]; exists {
		return fmt.Errorf("order %q already exists", o.ID)
	}
	m.orders[o.ID] = o
	return nil
}

func (m *Memory) Get(_ context.Context, id string) (orders.Order, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	o, ok := m.orders[id]
	if !ok {
		return orders.Order{}, orders.ErrNotFound
	}
	return o, nil
}

func (m *Memory) List(_ context.Context) ([]orders.Order, error) {
	m.mu.RLock()
	list := make([]orders.Order, 0, len(m.orders))
	for _, o := range m.orders {
		list = append(list, o)
	}
	m.mu.RUnlock()
	sortNewestFirst(list)
	return list, nil
}

func (m *Memory) MarkEventPublished(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	o, ok := m.orders[id]
	if !ok {
		return orders.ErrNotFound
	}
	o.EventPublished = true
	m.orders[id] = o
	return nil
}

func sortNewestFirst(list []orders.Order) {
	sort.Slice(list, func(i, j int) bool { return list[i].CreatedAt.After(list[j].CreatedAt) })
}
