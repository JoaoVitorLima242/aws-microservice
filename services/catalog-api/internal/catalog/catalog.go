// Package catalog holds the in-memory product store and its HTTP handlers.
package catalog

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/shopspring/decimal"

	"shop-aws/services/catalog-api/internal/httpx"
)

func init() {
	// Serialise prices as JSON numbers (19.9) instead of strings ("19.9").
	decimal.MarshalJSONWithoutQuotes = true
}

type Product struct {
	ID     string          `json:"id"`
	Name   string          `json:"name"`
	Price  decimal.Decimal `json:"price"`
	Active bool            `json:"active"`
}

// Store is a read-only, in-memory product catalog. It is safe for concurrent
// use because it is never modified after construction.
type Store struct {
	products []Product
	byID     map[string]Product
}

// NewStore parses and validates the seed JSON.
func NewStore(seedJSON []byte) (*Store, error) {
	var products []Product
	if err := json.Unmarshal(seedJSON, &products); err != nil {
		return nil, fmt.Errorf("parse product seed: %w", err)
	}

	byID := make(map[string]Product, len(products))
	for i, p := range products {
		if p.ID == "" || p.Name == "" {
			return nil, fmt.Errorf("product seed entry %d: id and name are required", i)
		}
		if p.Price.IsNegative() {
			return nil, fmt.Errorf("product %q: price must not be negative", p.ID)
		}
		if _, dup := byID[p.ID]; dup {
			return nil, fmt.Errorf("product %q: duplicate id", p.ID)
		}
		byID[p.ID] = p
	}
	return &Store{products: products, byID: byID}, nil
}

func (s *Store) List() []Product {
	return s.products
}

func (s *Store) Get(id string) (Product, bool) {
	p, ok := s.byID[id]
	return p, ok
}

func (s *Store) Len() int {
	return len(s.products)
}

// Handler serves the /catalog routes.
type Handler struct {
	store       *Store
	serviceName string
	version     string
}

func NewHandler(store *Store, serviceName, version string) *Handler {
	return &Handler{store: store, serviceName: serviceName, version: version}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /catalog/health", h.health)
	mux.HandleFunc("GET /catalog/version", h.versionInfo)
	mux.HandleFunc("GET /catalog/products", h.listProducts)
	mux.HandleFunc("GET /catalog/products/{id}", h.getProduct)
}

func (h *Handler) health(w http.ResponseWriter, _ *http.Request) {
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) versionInfo(w http.ResponseWriter, _ *http.Request) {
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"service": h.serviceName, "version": h.version})
}

func (h *Handler) listProducts(w http.ResponseWriter, _ *http.Request) {
	httpx.WriteJSON(w, http.StatusOK, h.store.List())
}

func (h *Handler) getProduct(w http.ResponseWriter, r *http.Request) {
	p, ok := h.store.Get(r.PathValue("id"))
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, "product not found")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, p)
}
