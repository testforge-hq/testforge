package admin

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Handlers provides admin API handlers
type Handlers struct {
	dashboard *DashboardService
	logger    *zap.Logger
}

// NewHandlers creates new admin handlers
func NewHandlers(dashboard *DashboardService, logger *zap.Logger) *Handlers {
	return &Handlers{
		dashboard: dashboard,
		logger:    logger,
	}
}

// GetOverview returns platform overview stats
func (h *Handlers) GetOverview(w http.ResponseWriter, r *http.Request) {
	stats, err := h.dashboard.GetOverview(r.Context())
	if err != nil {
		h.logger.Error("failed to get overview", zap.Error(err))
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	h.jsonResponse(w, stats)
}

// ListTenants returns paginated tenant list
func (h *Handlers) ListTenants(w http.ResponseWriter, r *http.Request) {
	params := h.parseListParams(r)

	tenants, total, err := h.dashboard.ListTenants(r.Context(), params)
	if err != nil {
		h.logger.Error("failed to list tenants", zap.Error(err))
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	h.jsonResponse(w, map[string]interface{}{
		"data":  tenants,
		"total": total,
		"limit": params.Limit,
		"offset": params.Offset,
	})
}

// GetTenant returns details for a specific tenant
func (h *Handlers) GetTenant(w http.ResponseWriter, r *http.Request) {
	tenantIDStr := r.PathValue("id")
	tenantID, err := uuid.Parse(tenantIDStr)
	if err != nil {
		http.Error(w, "Invalid tenant ID", http.StatusBadRequest)
		return
	}

	tenants, _, err := h.dashboard.ListTenants(r.Context(), ListParams{Limit: 1, Offset: 0})
	if err != nil {
		h.logger.Error("failed to get tenant", zap.Error(err))
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	for _, t := range tenants {
		if t.ID == tenantID {
			h.jsonResponse(w, t)
			return
		}
	}

	http.Error(w, "Tenant not found", http.StatusNotFound)
}

// ListUsers returns paginated user list
func (h *Handlers) ListUsers(w http.ResponseWriter, r *http.Request) {
	params := h.parseListParams(r)

	users, total, err := h.dashboard.ListUsers(r.Context(), params)
	if err != nil {
		h.logger.Error("failed to list users", zap.Error(err))
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	h.jsonResponse(w, map[string]interface{}{
		"data":  users,
		"total": total,
		"limit": params.Limit,
		"offset": params.Offset,
	})
}

// GetRunningTests returns currently running tests
func (h *Handlers) GetRunningTests(w http.ResponseWriter, r *http.Request) {
	runs, err := h.dashboard.GetRunningTests(r.Context())
	if err != nil {
		h.logger.Error("failed to get running tests", zap.Error(err))
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	h.jsonResponse(w, map[string]interface{}{
		"data":  runs,
		"count": len(runs),
	})
}

// GetCostAnalytics returns cost analytics
func (h *Handlers) GetCostAnalytics(w http.ResponseWriter, r *http.Request) {
	daysStr := r.URL.Query().Get("days")
	days := 30
	if daysStr != "" {
		if d, err := strconv.Atoi(daysStr); err == nil && d > 0 && d <= 90 {
			days = d
		}
	}

	costs, err := h.dashboard.GetCostAnalytics(r.Context(), days)
	if err != nil {
		h.logger.Error("failed to get cost analytics", zap.Error(err))
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	h.jsonResponse(w, map[string]interface{}{
		"data": costs,
		"days": days,
	})
}

// GetBillingOverview returns billing overview
func (h *Handlers) GetBillingOverview(w http.ResponseWriter, r *http.Request) {
	overview, err := h.dashboard.GetBillingOverview(r.Context())
	if err != nil {
		h.logger.Error("failed to get billing overview", zap.Error(err))
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	h.jsonResponse(w, overview)
}

// GetSystemHealth returns system health status
func (h *Handlers) GetSystemHealth(w http.ResponseWriter, r *http.Request) {
	health, err := h.dashboard.GetSystemHealth(r.Context())
	if err != nil {
		h.logger.Error("failed to get system health", zap.Error(err))
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	h.jsonResponse(w, health)
}

// GetRecentActivity returns recent platform activity
func (h *Handlers) GetRecentActivity(w http.ResponseWriter, r *http.Request) {
	limitStr := r.URL.Query().Get("limit")
	limit := 50
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 200 {
			limit = l
		}
	}

	activity, err := h.dashboard.GetRecentActivity(r.Context(), limit)
	if err != nil {
		h.logger.Error("failed to get recent activity", zap.Error(err))
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	h.jsonResponse(w, map[string]interface{}{
		"data": activity,
	})
}

// Helpers

func (h *Handlers) parseListParams(r *http.Request) ListParams {
	params := ListParams{
		Limit:  20,
		Offset: 0,
		Sort:   "created_at",
		Order:  "desc",
	}

	if l := r.URL.Query().Get("limit"); l != "" {
		if limit, err := strconv.Atoi(l); err == nil && limit > 0 && limit <= 100 {
			params.Limit = limit
		}
	}

	if o := r.URL.Query().Get("offset"); o != "" {
		if offset, err := strconv.Atoi(o); err == nil && offset >= 0 {
			params.Offset = offset
		}
	}

	if s := r.URL.Query().Get("sort"); s != "" {
		params.Sort = s
	}

	if o := r.URL.Query().Get("order"); o == "asc" || o == "desc" {
		params.Order = o
	}

	params.Search = r.URL.Query().Get("search")

	return params
}

func (h *Handlers) jsonResponse(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}
