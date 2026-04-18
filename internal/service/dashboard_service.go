package service

import (
	"context"
	"fmt"
	"time"

	"github.com/sekolahpintar/dashboard-engine/internal/model"
	"github.com/sekolahpintar/dashboard-engine/internal/repository"
)

type DashboardService struct {
	repo *repository.DashboardRepo
}

func NewDashboardService(repo *repository.DashboardRepo) *DashboardService {
	return &DashboardService{repo: repo}
}

// ─── Role-dispatched index ────────────────────────────────────────────────

func (s *DashboardService) GetIndex(ctx context.Context, claims *model.UserClaims, f model.DashboardFilter) (interface{}, error) {
	switch {
	case claims.HasRole("admin") || claims.HasRole("staff"):
		return s.GetAdminDashboard(ctx, claims, f)
	case claims.HasRole("guru"):
		return s.GetGuruDashboard(ctx, claims)
	case claims.HasRole("siswa"):
		return s.GetSiswaDashboard(ctx, claims)
	case claims.HasRole("wali"):
		return s.GetWaliDashboard(ctx, claims)
	default:
		return s.GetAdminDashboard(ctx, claims, f)
	}
}

// ─── Admin Dashboard ──────────────────────────────────────────────────────

func (s *DashboardService) GetAdminDashboard(ctx context.Context, _ *model.UserClaims, f model.DashboardFilter) (*model.AdminDashboard, error) {
	// Run all sub-fetches sequentially (each already parallelises internally)
	summary, err := s.repo.GetAdminSummaryCards(ctx, f)
	if err != nil {
		return nil, fmt.Errorf("summary cards: %w", err)
	}

	financial, err := s.repo.GetFinancialAnalytics(ctx, f)
	if err != nil {
		return nil, fmt.Errorf("financial analytics: %w", err)
	}

	academic, err := s.repo.GetAcademicAttendanceAnalytics(ctx, f)
	if err != nil {
		return nil, fmt.Errorf("academic attendance: %w", err)
	}

	counseling, err := s.repo.GetCounselingInsights(ctx, f)
	if err != nil {
		return nil, fmt.Errorf("counseling insights: %w", err)
	}

	ppdb, err := s.repo.GetPpdbInsights(ctx)
	if err != nil {
		return nil, fmt.Errorf("ppdb insights: %w", err)
	}

	filtersApplied := map[string]interface{}{
		"tahun_ajaran_id": f.TahunAjaranID,
		"mst_kelas_id":    f.KelasID,
	}

	return &model.AdminDashboard{
		Role:               "admin",
		SummaryCards:       *summary,
		Financial:          *financial,
		AcademicAttendance: *academic,
		Counseling:         *counseling,
		Ppdb:               *ppdb,
		FiltersApplied:     filtersApplied,
		GeneratedAt:        time.Now().Format(time.RFC3339),
	}, nil
}

// ─── Summary Cards (role-aware) ───────────────────────────────────────────

func (s *DashboardService) GetSummaryCards(ctx context.Context, claims *model.UserClaims, f model.DashboardFilter) (interface{}, error) {
	switch {
	case claims.HasRole("siswa"):
		if claims.SiswaID == nil {
			return nil, fmt.Errorf("siswa profile not found")
		}
		siswa, err := s.repo.GetSiswaByUserID(ctx, claims.UserID)
		if err != nil || siswa == nil {
			return nil, fmt.Errorf("siswa profile not found")
		}
		return s.repo.GetSiswaSummaryCards(ctx, siswa.ID, siswa.KelasID)

	case claims.HasRole("guru"):
		if claims.GuruID == nil {
			return nil, fmt.Errorf("guru profile not found")
		}
		return s.repo.GetGuruSummaryCards(ctx, *claims.GuruID)

	default:
		return s.repo.GetAdminSummaryCards(ctx, f)
	}
}

// ─── Financial Analytics ──────────────────────────────────────────────────

func (s *DashboardService) GetFinancialAnalytics(ctx context.Context, f model.DashboardFilter) (*model.FinancialAnalytics, error) {
	return s.repo.GetFinancialAnalytics(ctx, f)
}

// ─── Academic & Attendance Analytics ─────────────────────────────────────

func (s *DashboardService) GetAcademicAttendanceAnalytics(ctx context.Context, f model.DashboardFilter) (*model.AcademicAttendanceAnalytics, error) {
	return s.repo.GetAcademicAttendanceAnalytics(ctx, f)
}

// ─── Counseling Insights ──────────────────────────────────────────────────

func (s *DashboardService) GetCounselingInsights(ctx context.Context, f model.DashboardFilter) (*model.CounselingInsights, error) {
	return s.repo.GetCounselingInsights(ctx, f)
}

// ─── PPDB Insights ────────────────────────────────────────────────────────

func (s *DashboardService) GetPpdbInsights(ctx context.Context) (*model.PpdbInsights, error) {
	return s.repo.GetPpdbInsights(ctx)
}

// ─── Guru Dashboard ───────────────────────────────────────────────────────

func (s *DashboardService) GetGuruDashboard(ctx context.Context, claims *model.UserClaims) (*model.GuruDashboard, error) {
	if claims.GuruID == nil {
		return nil, fmt.Errorf("guru profile not found")
	}

	guru, err := s.repo.GetGuruByUserID(ctx, claims.UserID)
	if err != nil || guru == nil {
		return nil, fmt.Errorf("guru profile not found")
	}

	summaryCards, err := s.repo.GetGuruSummaryCards(ctx, guru.ID)
	if err != nil {
		return nil, err
	}

	return &model.GuruDashboard{
		Role: "guru",
		Profile: model.GuruProfile{
			ID:        guru.ID,
			MstGuruID: guru.ID,
			Nama:      guru.Nama,
			NIP:       guru.NIP,
		},
		Summary:     *summaryCards,
		GeneratedAt: time.Now().Format(time.RFC3339),
	}, nil
}

// ─── Siswa Dashboard ──────────────────────────────────────────────────────

func (s *DashboardService) GetSiswaDashboard(ctx context.Context, claims *model.UserClaims) (*model.SiswaDashboard, error) {
	siswa, err := s.repo.GetSiswaByUserID(ctx, claims.UserID)
	if err != nil || siswa == nil {
		return nil, fmt.Errorf("siswa profile not found")
	}

	attendanceSummary, unpaidSpp, recentGrades, upcomingTasks, err :=
		s.repo.GetSiswaDashboardData(ctx, siswa.ID, siswa.KelasID)
	if err != nil {
		return nil, err
	}

	// Enrich unpaid SPP with month names
	for i := range unpaidSpp {
		// namaBulan is local to repo; re-derive here inline
		months := []string{
			"Januari", "Februari", "Maret", "April", "Mei", "Juni",
			"Juli", "Agustus", "September", "Oktober", "November", "Desember",
		}
		if unpaidSpp[i].Bulan >= 1 && unpaidSpp[i].Bulan <= 12 {
			unpaidSpp[i].BulanNama = months[unpaidSpp[i].Bulan-1]
		}
	}

	if attendanceSummary == nil {
		attendanceSummary = []model.AttendanceItem{}
	}
	if unpaidSpp == nil {
		unpaidSpp = []model.UnpaidSppItem{}
	}
	if recentGrades == nil {
		recentGrades = []model.RecentGrade{}
	}
	if upcomingTasks == nil {
		upcomingTasks = []model.UpcomingTask{}
	}

	return &model.SiswaDashboard{
		Role: "siswa",
		Profile: model.SiswaProfile{
			ID:         siswa.ID,
			MstSiswaID: siswa.ID,
			Nama:       siswa.Nama,
			NIS:        siswa.NIS,
			Kelas:      siswa.NamaKelas,
		},
		AttendanceSummary: attendanceSummary,
		UnpaidSpp:         unpaidSpp,
		RecentGrades:      recentGrades,
		UpcomingTasks:     upcomingTasks,
		GeneratedAt:       time.Now().Format(time.RFC3339),
	}, nil
}

// ─── Wali Dashboard ───────────────────────────────────────────────────────

func (s *DashboardService) GetWaliDashboard(ctx context.Context, claims *model.UserClaims) (*model.WaliDashboard, error) {
	if claims.WaliID == nil {
		return nil, fmt.Errorf("wali profile not found")
	}

	wali, err := s.repo.GetWaliByUserID(ctx, claims.UserID)
	if err != nil || wali == nil {
		return nil, fmt.Errorf("wali profile not found")
	}

	children, err := s.repo.GetWaliChildren(ctx, *claims.WaliID)
	if err != nil {
		return nil, fmt.Errorf("get wali children: %w", err)
	}

	// Fetch per-child data (sequential – usually only 1-3 children)
	childrenData := make([]model.ChildData, 0, len(children))
	for _, child := range children {
		absensiLabel, err := s.repo.GetChildTodayAbsensi(ctx, child.ID)
		if err != nil {
			return nil, err
		}

		// Re-use cached refCode via repo
		statusBayarLunas, err := s.repo.GetStatusBayarLunas(ctx)
		if err != nil {
			return nil, err
		}

		tunggakanCount, err := s.repo.GetChildTunggakanSppCount(ctx, child.ID, statusBayarLunas)
		if err != nil {
			return nil, err
		}

		jadwalRows, err := s.repo.GetJadwalHariIni(ctx, child.KelasID)
		if err != nil {
			return nil, err
		}

		jadwal := make([]model.JadwalItem, 0, len(jadwalRows))
		for _, j := range jadwalRows {
			jadwal = append(jadwal, model.JadwalItem{
				Mapel:      j.NamaMapel,
				Guru:       j.NamaGuru,
				JamMulai:   j.JamMulai,
				JamSelesai: j.JamSelesai,
				Ruangan:    j.Ruangan,
			})
		}

		childrenData = append(childrenData, model.ChildData{
			ID:                child.ID,
			Nama:              child.Nama,
			Kelas:             child.NamaKelas,
			AbsensiHariIni:    absensiLabel,
			TunggakanSppCount: tunggakanCount,
			JadwalHariIni:     jadwal,
		})
	}

	return &model.WaliDashboard{
		Role:        "wali",
		Profile:     model.WaliProfile{Nama: wali.Nama},
		Children:    childrenData,
		GeneratedAt: time.Now().Format(time.RFC3339),
	}, nil
}
