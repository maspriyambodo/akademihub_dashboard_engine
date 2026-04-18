package handler

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/sekolahpintar/dashboard-engine/internal/middleware"
	"github.com/sekolahpintar/dashboard-engine/internal/model"
	"github.com/sekolahpintar/dashboard-engine/internal/service"
)

type DashboardHandler struct {
	svc *service.DashboardService
}

func NewDashboardHandler(svc *service.DashboardService) *DashboardHandler {
	return &DashboardHandler{svc: svc}
}

// ─── helpers ──────────────────────────────────────────────────────────────

func queryInt64(r *http.Request, key string) *int64 {
	raw := strings.TrimSpace(r.URL.Query().Get(key))
	if raw == "" {
		return nil
	}
	v, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return nil
	}
	return &v
}

func buildFilter(r *http.Request) model.DashboardFilter {
	return model.DashboardFilter{
		TahunAjaranID: queryInt64(r, "tahun_ajaran_id"),
		KelasID:       queryInt64(r, "mst_kelas_id"),
	}
}

// ─── GET /api/v1/dashboard ────────────────────────────────────────────────

func (h *DashboardHandler) Index(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		jsonError(w, http.StatusUnauthorized, "Unauthenticated")
		return
	}

	result, err := h.svc.GetIndex(r.Context(), claims, buildFilter(r))
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			jsonNotFound(w, err.Error())
			return
		}
		jsonServerError(w, "Failed to retrieve dashboard data: "+err.Error())
		return
	}

	jsonOK(w, result, "Dashboard data retrieved successfully")
}

// ─── GET /api/v1/dashboard/summary-cards ──────────────────────────────────

func (h *DashboardHandler) SummaryCards(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		jsonError(w, http.StatusUnauthorized, "Unauthenticated")
		return
	}

	result, err := h.svc.GetSummaryCards(r.Context(), claims, buildFilter(r))
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			jsonNotFound(w, err.Error())
			return
		}
		jsonServerError(w, "Failed to retrieve summary cards: "+err.Error())
		return
	}

	jsonOK(w, result, "Summary cards retrieved successfully")
}

// ─── GET /api/v1/dashboard/financial-analytics ────────────────────────────

func (h *DashboardHandler) FinancialAnalytics(w http.ResponseWriter, r *http.Request) {
	result, err := h.svc.GetFinancialAnalytics(r.Context(), buildFilter(r))
	if err != nil {
		jsonServerError(w, "Failed to retrieve financial analytics: "+err.Error())
		return
	}
	jsonOK(w, result, "Financial analytics retrieved successfully")
}

// ─── GET /api/v1/dashboard/academic-attendance ────────────────────────────

func (h *DashboardHandler) AcademicAttendanceAnalytics(w http.ResponseWriter, r *http.Request) {
	result, err := h.svc.GetAcademicAttendanceAnalytics(r.Context(), buildFilter(r))
	if err != nil {
		jsonServerError(w, "Failed to retrieve academic analytics: "+err.Error())
		return
	}
	jsonOK(w, result, "Academic and attendance analytics retrieved successfully")
}

// ─── GET /api/v1/dashboard/counseling-insights ────────────────────────────

func (h *DashboardHandler) CounselingInsights(w http.ResponseWriter, r *http.Request) {
	result, err := h.svc.GetCounselingInsights(r.Context(), buildFilter(r))
	if err != nil {
		jsonServerError(w, "Failed to retrieve counseling insights: "+err.Error())
		return
	}
	jsonOK(w, result, "Counseling insights retrieved successfully")
}

// ─── GET /api/v1/dashboard/ppdb-insights ──────────────────────────────────

func (h *DashboardHandler) PpdbInsights(w http.ResponseWriter, r *http.Request) {
	result, err := h.svc.GetPpdbInsights(r.Context())
	if err != nil {
		jsonServerError(w, "Failed to retrieve PPDB insights: "+err.Error())
		return
	}
	jsonOK(w, result, "PPDB insights retrieved successfully")
}
