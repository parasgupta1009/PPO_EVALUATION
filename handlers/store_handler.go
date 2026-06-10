package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/PPO_EVALUATION/models"
	"github.com/PPO_EVALUATION/services"
)

type StoreHandler struct {
	store services.Store
}

func NewStoreHandler(store services.Store) *StoreHandler {
	return &StoreHandler{store: store}
}

func (h *StoreHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /keys", h.Set)
	mux.HandleFunc("GET /keys/{key}", h.Get)
	mux.HandleFunc("DELETE /keys/{key}", h.Delete)
}

type SetRequest struct {
	Key        string `json:"key"`
	Value      string `json:"value"`
	TTLSeconds int    `json:"ttl_seconds"`
}

type GetResponse struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type ErrorResponse struct {
	Error string `json:"error"`
}

func (h *StoreHandler) Set(w http.ResponseWriter, r *http.Request) {
	var req SetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid request body"})
		return
	}

	if req.Key == "" {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "key is required"})
		return
	}

	if req.TTLSeconds <= 0 {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "ttl_seconds must be positive"})
		return
	}

	h.store.Set(req.Key, req.Value, time.Duration(req.TTLSeconds)*time.Second)
	writeJSON(w, http.StatusCreated, map[string]string{
		"message": "key set successfully",
		"key":     req.Key,
	})
}

func (h *StoreHandler) Get(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")

	val, err := h.store.Get(key)
	if err != nil {
		if errors.Is(err, models.ErrKeyNotFound) {
			writeJSON(w, http.StatusNotFound, ErrorResponse{Error: "key not found"})
			return
		}
		if errors.Is(err, models.ErrKeyExpired) {
			writeJSON(w, http.StatusGone, ErrorResponse{Error: "key expired"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "internal error"})
		return
	}

	writeJSON(w, http.StatusOK, GetResponse{Key: key, Value: val})
}

func (h *StoreHandler) Delete(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	err := h.store.Delete(key)
	if err != nil {
		if errors.Is(err, models.ErrKeyNotFound) {
			writeJSON(w, http.StatusNotFound, ErrorResponse{Error: "key not found"})
			return
		}
		if errors.Is(err, models.ErrKeyExpired) {
			writeJSON(w, http.StatusGone, ErrorResponse{Error: "key expired"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "internal error"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "deleted successfully", "key": key})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
