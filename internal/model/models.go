package model

// ─── Auth ─────────────────────────────────────────────────────────────────

// UserClaims holds authenticated user data loaded from DB after JWT validation.
type UserClaims struct {
	UserID  int64
	Roles   []string
	SiswaID *int64
	GuruID  *int64
	WaliID  *int64
}

func (c *UserClaims) HasRole(role string) bool {
	for _, r := range c.Roles {
		if r == role {
			return true
		}
	}
	return false
}

// ─── Filters ──────────────────────────────────────────────────────────────

type DashboardFilter struct {
	TahunAjaranID *int64
	KelasID       *int64
}

// ─── Summary Cards ────────────────────────────────────────────────────────

type SppTunggakan struct {
	Amount         float64 `json:"amount"`
	Formatted      string  `json:"formatted"`
	Month          string  `json:"month"`
	Year           int     `json:"year"`
	JumlahSiswa    int64   `json:"jumlah_siswa"`
}

type PpdbSummary struct {
	TotalPendaftar    int64 `json:"total_pendaftar"`
	PendaftarDiterima int64 `json:"pendaftar_diterima"`
}

type AdminSummaryCards struct {
	TotalSiswaAktif   int64        `json:"total_siswa_aktif"`
	TotalGuru         int64        `json:"total_guru"`
	TotalKelas        int64        `json:"total_kelas"`
	TotalTunggakanSpp SppTunggakan `json:"total_tunggakan_spp"`
	KasusBkProses     int64        `json:"kasus_bk_proses"`
	PpdbSummary       PpdbSummary  `json:"ppdb_summary"`
}

type SiswaSummaryCards struct {
	AttendanceThisMonth int64 `json:"attendance_this_month"`
	PendingSppCount     int64 `json:"pending_spp_count"`
	RecentGradeCount    int64 `json:"recent_grade_count"`
	UpcomingTaskCount   int64 `json:"upcoming_task_count"`
}

type GuruSummaryCards struct {
	TotalKelasWali   int64 `json:"total_kelas_wali"`
	TotalSiswaWali   int64 `json:"total_siswa_wali"`
	TotalMapel       int64 `json:"total_mapel"`
	TugasBelumDinilai int64 `json:"tugas_belum_dinilai"`
}

// ─── Financial Analytics ──────────────────────────────────────────────────

type SppDataset struct {
	Label         string   `json:"label"`
	Data          []int64  `json:"data"`
	FormattedData []string `json:"formatted_data,omitempty"`
	YAxisID       string   `json:"yAxisID,omitempty"`
}

type SppTrend struct {
	Labels   []string     `json:"labels"`
	Datasets []SppDataset `json:"datasets"`
}

type PaymentStatusDonut struct {
	Labels      []string  `json:"labels"`
	Data        []int64   `json:"data"`
	Percentages []float64 `json:"percentages"`
	Colors      []string  `json:"colors"`
}

type YearlySummary struct {
	TotalPendapatan    float64 `json:"total_pendapatan"`
	FormattedTotal     string  `json:"formatted_total"`
	Year               int     `json:"year"`
	TotalLunas         int64   `json:"total_lunas"`
	TotalBelumLunas    int64   `json:"total_belum_lunas"`
}

type FinancialAnalytics struct {
	SppTrend                  SppTrend           `json:"spp_trend"`
	PaymentStatusDistribution PaymentStatusDonut `json:"payment_status_distribution"`
	YearlySummary             YearlySummary      `json:"yearly_summary"`
}

// ─── Academic & Attendance Analytics ─────────────────────────────────────

type AttendanceDataset struct {
	Label string  `json:"label"`
	Data  []int64 `json:"data"`
	Color string  `json:"color"`
}

type Attendance7Days struct {
	Labels   []string            `json:"labels"`
	Datasets []AttendanceDataset `json:"datasets"`
}

type AttendanceSummary struct {
	RataRataKehadiran float64 `json:"rata_rata_kehadiran"`
	TotalHadir7Hari  int64   `json:"total_hadir_7_hari"`
	TotalRecords     int64   `json:"total_records"`
}

type NilaiDistribution struct {
	Labels      []string  `json:"labels"`
	Data        []int64   `json:"data"`
	Percentages []float64 `json:"percentages"`
	Colors      []string  `json:"colors"`
}

type NilaiSummary struct {
	RataRata       float64 `json:"rata_rata"`
	TotalUjian     int64   `json:"total_ujian"`
	NilaiTertinggi float64 `json:"nilai_tertinggi"`
	NilaiTerendah  float64 `json:"nilai_terendah"`
}

type AcademicAttendanceAnalytics struct {
	Attendance7Days   Attendance7Days   `json:"attendance_7_days"`
	AttendanceSummary AttendanceSummary `json:"attendance_summary"`
	NilaiDistribution NilaiDistribution `json:"nilai_distribution"`
	NilaiSummary      NilaiSummary      `json:"nilai_summary"`
}

// ─── Counseling Insights ──────────────────────────────────────────────────

type TopKategoriKasus struct {
	Labels []string `json:"labels"`
	Data   []int64  `json:"data"`
}

type StatusPenyelesaian struct {
	Labels      []string  `json:"labels"`
	Data        []int64   `json:"data"`
	Percentages []float64 `json:"percentages"`
	Colors      []string  `json:"colors"`
}

type KasusPerBulan struct {
	Labels []string `json:"labels"`
	Data   []int64  `json:"data"`
}

type BkRingkasan struct {
	TotalKasus              int64   `json:"total_kasus"`
	KasusSelesai            int64   `json:"kasus_selesai"`
	KasusProses             int64   `json:"kasus_proses"`
	KasusDibuka             int64   `json:"kasus_dibuka"`
	KasusDirujuk            int64   `json:"kasus_dirujuk"`
	PersentasePenyelesaian  float64 `json:"persentase_penyelesaian"`
}

type CounselingInsights struct {
	TopKategoriKasus   TopKategoriKasus   `json:"top_kategori_kasus"`
	StatusPenyelesaian StatusPenyelesaian `json:"status_penyelesaian"`
	KasusPerBulan      KasusPerBulan      `json:"kasus_per_bulan"`
	Ringkasan          BkRingkasan        `json:"ringkasan"`
}

// ─── PPDB Insights ────────────────────────────────────────────────────────

type StatusDistribution struct {
	Labels []string `json:"labels"`
	Data   []int64  `json:"data"`
	Colors []string `json:"colors"`
}

type RegistrationsPerMonth struct {
	Labels []string `json:"labels"`
	Data   []int64  `json:"data"`
}

type PpdbInsights struct {
	StatusDistribution    StatusDistribution    `json:"status_distribution"`
	RegistrationsPerMonth RegistrationsPerMonth `json:"registrations_per_month"`
}

// ─── Admin Dashboard (combined) ───────────────────────────────────────────

type AdminDashboard struct {
	Role               string                      `json:"role"`
	SummaryCards       AdminSummaryCards            `json:"summary_cards"`
	Financial          FinancialAnalytics           `json:"financial"`
	AcademicAttendance AcademicAttendanceAnalytics  `json:"academic_attendance"`
	Counseling         CounselingInsights           `json:"counseling"`
	Ppdb               PpdbInsights                 `json:"ppdb"`
	FiltersApplied     map[string]interface{}       `json:"filters_applied"`
	GeneratedAt        string                       `json:"generated_at"`
}

// ─── Guru Dashboard ───────────────────────────────────────────────────────

type GuruProfile struct {
	ID      int64  `json:"id"`
	MstGuruID int64 `json:"mst_guru_id"`
	Nama    string `json:"nama"`
	NIP     string `json:"nip"`
}

type GuruDashboard struct {
	Role        string       `json:"role"`
	Profile     GuruProfile  `json:"profile"`
	Summary     GuruSummaryCards `json:"summary"`
	GeneratedAt string       `json:"generated_at"`
}

// ─── Siswa Dashboard ──────────────────────────────────────────────────────

type SiswaProfile struct {
	ID        int64  `json:"id"`
	MstSiswaID int64 `json:"mst_siswa_id"`
	Nama      string `json:"nama"`
	NIS       string `json:"nis"`
	Kelas     string `json:"kelas"`
}

type AttendanceItem struct {
	Status      int64  `json:"status" db:"status"`
	Total       int64  `json:"total" db:"total"`
	StatusLabel string `json:"status_label"`
}

type UnpaidSppItem struct {
	ID         int64   `json:"id" db:"id"`
	Bulan      int     `json:"bulan" db:"bulan"`
	BulanNama  string  `json:"bulan_nama"`
	Tahun      int     `json:"tahun" db:"tahun"`
	JumlahBayar float64 `json:"jumlah_bayar" db:"jumlah_bayar"`
	Status     int64   `json:"status" db:"status"`
}

type RecentGrade struct {
	ID          int64   `json:"id" db:"id"`
	Nilai       float64 `json:"nilai" db:"nilai"`
	NamaUjian   string  `json:"nama_ujian" db:"nama_ujian"`
	NamaMapel   string  `json:"nama_mapel" db:"nama_mapel"`
}

type UpcomingTask struct {
	ID           int64  `json:"id" db:"id"`
	Judul        string `json:"judul" db:"judul"`
	TenggatWaktu string `json:"tenggat_waktu" db:"tenggat_waktu"`
}

type SiswaDashboard struct {
	Role           string           `json:"role"`
	Profile        SiswaProfile     `json:"profile"`
	AttendanceSummary []AttendanceItem `json:"attendance_summary"`
	UnpaidSpp      []UnpaidSppItem  `json:"unpaid_spp"`
	RecentGrades   []RecentGrade    `json:"recent_grades"`
	UpcomingTasks  []UpcomingTask   `json:"upcoming_tasks"`
	GeneratedAt    string           `json:"generated_at"`
}

// ─── Wali Dashboard ───────────────────────────────────────────────────────

type WaliProfile struct {
	Nama string `json:"nama"`
}

type JadwalItem struct {
	Mapel      string `json:"mapel"`
	Guru       string `json:"guru"`
	JamMulai   string `json:"jam_mulai"`
	JamSelesai string `json:"jam_selesai"`
	Ruangan    string `json:"ruangan"`
}

type ChildData struct {
	ID              int64        `json:"id"`
	Nama            string       `json:"nama"`
	Kelas           string       `json:"kelas"`
	AbsensiHariIni  string       `json:"absensi_hari_ini"`
	TunggakanSppCount int64      `json:"tunggakan_spp_count"`
	JadwalHariIni   []JadwalItem `json:"jadwal_hari_ini"`
}

type WaliDashboard struct {
	Role        string      `json:"role"`
	Profile     WaliProfile `json:"profile"`
	Children    []ChildData `json:"children"`
	GeneratedAt string      `json:"generated_at"`
}
