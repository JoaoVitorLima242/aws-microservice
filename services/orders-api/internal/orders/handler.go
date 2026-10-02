package orders

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"shop-aws/services/orders-api/internal/httpx"
)

const maxBodyBytes = 1 << 20

// Handler serves the /orders routes.
type Handler struct {
	svc         *Service
	logger      *slog.Logger
	serviceName string
	version     string
}

func NewHandler(svc *Service, logger *slog.Logger, serviceName, version string) *Handler {
	return &Handler{svc: svc, logger: logger, serviceName: serviceName, version: version}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /orders/health", h.health)
	mux.HandleFunc("GET /orders/version", h.versionInfo)
	mux.HandleFunc("POST /orders", h.create)
	mux.HandleFunc("GET /orders", h.list)
	mux.HandleFunc("GET /orders/{id}", h.get)
}

func (h *Handler) health(w http.ResponseWriter, _ *http.Request) {
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) versionInfo(w http.ResponseWriter, _ *http.Request) {
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"service": h.serviceName, "version": h.version})
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var in CreateOrderInput
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	if err := dec.Decode(&in); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "request body must be a JSON object: "+err.Error())
		return
	}
	if dec.More() {
		httpx.WriteError(w, http.StatusBadRequest, "request body must contain a single JSON object")
		return
	}

	order, err := h.svc.Create(r.Context(), in)
	if err != nil {
		var verr *ValidationError
		switch {
		case errors.As(err, &verr):
			httpx.WriteJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid request", "details": verr.Problems})
		case errors.Is(err, ErrProductNotFound), errors.Is(err, ErrProductInactive):
			httpx.WriteError(w, http.StatusUnprocessableEntity, err.Error())
		case errors.Is(err, ErrCatalogUnavailable):
			h.logger.WarnContext(r.Context(), "catalog unavailable", slog.Any("error", err))
			httpx.WriteError(w, http.StatusServiceUnavailable, "catalog service unavailable, try again later")
		default:
			h.logger.ErrorContext(r.Context(), "failed to create order", slog.Any("error", err))
			httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		}
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, order)
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	order, err := h.svc.Get(r.Context(), r.PathValue("id"))
	switch {
	case errors.Is(err, ErrNotFound):
		httpx.WriteError(w, http.StatusNotFound, "order not found")
	case err != nil:
		h.logger.ErrorContext(r.Context(), "failed to get order", slog.Any("error", err))
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
	default:
		httpx.WriteJSON(w, http.StatusOK, order)
	}
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	list, err := h.svc.List(r.Context())
	if err != nil {
		h.logger.ErrorContext(r.Context(), "failed to list orders", slog.Any("error", err))
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, list)
}
