package adspot

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// Handler wires HTTP routes to the adspot repository.
type Handler struct {
	repo *Repository
}

// NewHandler creates a Handler backed by the given Repository.
func NewHandler(repo *Repository) *Handler {
	return &Handler{repo: repo}
}

// Routes returns a chi.Router with all adspot sub-routes registered.
func (h *Handler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Post("/", h.create)
	r.Get("/", h.listEligible)
	r.Get("/{id}", h.getByID)
	r.Post("/{id}/deactivate", h.deactivate)
	return r
}

// POST /adspots
func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var req CreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.Title == "" || req.ImageURL == "" || req.Placement == "" {
		writeError(w, "title, imageUrl and placement are required", http.StatusBadRequest)
		return
	}
	if !validPlacement(req.Placement) {
		writeError(w, "placement must be one of: home_screen, ride_summary, map_view", http.StatusBadRequest)
		return
	}

	spot, err := h.repo.Create(r.Context(), req)
	if err != nil {
		writeError(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, spot, http.StatusCreated)
}

// GET /adspots/{id}
func (h *Handler) getByID(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	spot, err := h.repo.GetByID(r.Context(), id)
	if err != nil {
		writeError(w, "internal error", http.StatusInternalServerError)
		return
	}
	if spot == nil {
		writeError(w, "adspot not found", http.StatusNotFound)
		return
	}
	writeJSON(w, spot, http.StatusOK)
}

// POST /adspots/{id}/deactivate
func (h *Handler) deactivate(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	spot, err := h.repo.Deactivate(r.Context(), id)
	if err != nil {
		writeError(w, "internal error", http.StatusInternalServerError)
		return
	}
	if spot == nil {
		writeError(w, "adspot not found", http.StatusNotFound)
		return
	}
	writeJSON(w, spot, http.StatusOK)
}

// GET /adspots?placement=...&status=active
func (h *Handler) listEligible(w http.ResponseWriter, r *http.Request) {
	placement := r.URL.Query().Get("placement")
	spots, err := h.repo.ListEligible(r.Context(), placement)
	if err != nil {
		writeError(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, spots, http.StatusOK)
}

// ── helpers ──────────────────────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, v any, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, msg string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
