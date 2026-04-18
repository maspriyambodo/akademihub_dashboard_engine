package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/sekolahpintar/dashboard-engine/internal/model"
	"golang.org/x/sync/errgroup"
)

// ─── Reference code cache (TTL 60 min) ───────────────────────────────────

type refEntry struct {
	value     string
	expiresAt time.Time
}

type DashboardRepo struct {
	db      *sqlx.DB
	refMu   sync.RWMutex
	refCache map[string]refEntry
}

func NewDashboardRepo(db *sqlx.DB) *DashboardRepo {
	return &DashboardRepo{
		db:       db,
		refCache: make(map[string]refEntry),
	}
}

// refCode returns the kode for a given kategori+nama from sys_references,
// using an in-memory TTL cache (60 minutes).
func (r *DashboardRepo) refCode(ctx context.Context, kategori, nama string) (string, error) {
	key := kategori + ":" + nama

	r.refMu.RLock()
	entry, ok := r.refCache[key]
	r.refMu.RUnlock()
	if ok && time.Now().Before(entry.expiresAt) {
		return entry.value, nil
	}

	var kode string
	err := r.db.QueryRowContext(ctx,
		`SELECT kode FROM sys_references WHERE kategori = $1 AND nama = $2 AND deleted_at IS NULL LIMIT 1`,
		kategori, nama,
	).Scan(&kode)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("refCode(%s/%s): %w", kategori, nama, err)
	}

	r.refMu.Lock()
	r.refCache[key] = refEntry{value: kode, expiresAt: time.Now().Add(60 * time.Minute)}
	r.refMu.Unlock()

	return kode, nil
}

// ─── helpers ──────────────────────────────────────────────────────────────

func countQuery(ctx context.Context, db *sqlx.DB, query string, args ...interface{}) (int64, error) {
	var n int64
	err := db.QueryRowContext(ctx, query, args...).Scan(&n)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return n, err
}

func namaBulan(m int) string {
	months := []string{
		"Januari", "Februari", "Maret", "April", "Mei", "Juni",
		"Juli", "Agustus", "September", "Oktober", "November", "Desember",
	}
	if m < 1 || m > 12 {
		return "-"
	}
	return months[m-1]
}

func namaBulanShort(m int) string {
	months := []string{
		"Jan", "Feb", "Mar", "Apr", "Mei", "Jun",
		"Jul", "Agu", "Sep", "Okt", "Nov", "Des",
	}
	if m < 1 || m > 12 {
		return ""
	}
	return months[m-1]
}

func round2(f float64) float64 {
	return float64(int(f*100+0.5)) / 100
}

func formatRupiah(amount float64) string {
	s := fmt.Sprintf("%.0f", amount)
	n := len(s)
	var b strings.Builder
	for i, c := range s {
		if i > 0 && (n-i)%3 == 0 {
			b.WriteByte('.')
		}
		b.WriteRune(c)
	}
	return "Rp " + b.String()
}

// statusAbsensiLabel mirrors the PHP getStatusAbsensiLabel helper.
// The PHP controller hard-codes kode values 15-18 directly. This function
// maps those integer codes to human-readable labels.
func statusAbsensiLabel(status int64) string {
	switch status {
	case 15:
		return "Hadir"
	case 16:
		return "Izin"
	case 17:
		return "Sakit"
	case 18:
		return "Alpha"
	default:
		return "Belum Absen"
	}
}

// ─── Admin Summary Cards ──────────────────────────────────────────────────

func (r *DashboardRepo) GetAdminSummaryCards(ctx context.Context, f model.DashboardFilter) (*model.AdminSummaryCards, error) {
	now := time.Now()
	month := int(now.Month())
	year := now.Year()

	statusSiswaAktif, err := r.refCode(ctx, "status_siswa", "Aktif")
	if err != nil {
		return nil, err
	}
	statusBayarLunas, err := r.refCode(ctx, "status_bayar", "Lunas")
	if err != nil {
		return nil, err
	}
	statusBkProses, err := r.refCode(ctx, "status_bk", "proses")
	if err != nil {
		return nil, err
	}

	var (
		totalSiswa        int64
		totalGuru         int64
		totalKelas        int64
		tunggakanAmount   float64
		jumlahSiswaTungg  int64
		kasusBkProses     int64
		totalPendaftar    int64
		pendaftarDiterima int64
	)

	g, gctx := errgroup.WithContext(ctx)

	// Total siswa aktif
	g.Go(func() error {
		q := `SELECT COUNT(*) FROM mst_siswa s WHERE s.status = $1 AND s.deleted_at IS NULL`
		args := []interface{}{statusSiswaAktif}
		if f.KelasID != nil {
			q += ` AND s.mst_kelas_id = $2`
			args = append(args, *f.KelasID)
		}
		if f.TahunAjaranID != nil {
			q += fmt.Sprintf(` AND EXISTS (SELECT 1 FROM mst_kelas k WHERE k.id = s.mst_kelas_id AND k.tahun_ajaran_id = $%d AND k.deleted_at IS NULL)`, len(args)+1)
			args = append(args, *f.TahunAjaranID)
		}
		var err error
		totalSiswa, err = countQuery(gctx, r.db, q, args...)
		return err
	})

	// Total guru
	g.Go(func() error {
		var err error
		totalGuru, err = countQuery(gctx, r.db, `SELECT COUNT(*) FROM mst_guru WHERE deleted_at IS NULL`)
		return err
	})

	// Total kelas
	g.Go(func() error {
		q := `SELECT COUNT(*) FROM mst_kelas WHERE deleted_at IS NULL`
		args := []interface{}{}
		if f.TahunAjaranID != nil {
			q += ` AND tahun_ajaran_id = $1`
			args = append(args, *f.TahunAjaranID)
		}
		if f.KelasID != nil {
			q += fmt.Sprintf(` AND id = $%d`, len(args)+1)
			args = append(args, *f.KelasID)
		}
		var err error
		totalKelas, err = countQuery(gctx, r.db, q, args...)
		return err
	})

	// Tunggakan SPP bulan berjalan
	g.Go(func() error {
		q := `SELECT COALESCE(SUM(p.jumlah_bayar), 0), COUNT(DISTINCT p.mst_siswa_id)
			  FROM trx_pembayaran_spp p
			  WHERE p.bulan = $1 AND p.tahun = $2 AND p.status != $3`
		args := []interface{}{month, year, statusBayarLunas}
		if f.KelasID != nil {
			q += ` AND EXISTS (SELECT 1 FROM mst_siswa s WHERE s.id = p.mst_siswa_id AND s.mst_kelas_id = $4 AND s.deleted_at IS NULL)`
			args = append(args, *f.KelasID)
		}
		if f.TahunAjaranID != nil {
			q += fmt.Sprintf(` AND EXISTS (SELECT 1 FROM mst_siswa s JOIN mst_kelas k ON k.id = s.mst_kelas_id WHERE s.id = p.mst_siswa_id AND k.tahun_ajaran_id = $%d AND k.deleted_at IS NULL AND s.deleted_at IS NULL)`, len(args)+1)
			args = append(args, *f.TahunAjaranID)
		}
		return r.db.QueryRowContext(gctx, q, args...).Scan(&tunggakanAmount, &jumlahSiswaTungg)
	})

	// Kasus BK proses
	g.Go(func() error {
		q := `SELECT COUNT(*) FROM trx_bk_kasus bk WHERE bk.status = $1 AND bk.deleted_at IS NULL`
		args := []interface{}{statusBkProses}
		if f.KelasID != nil {
			q += ` AND EXISTS (SELECT 1 FROM mst_siswa s WHERE s.id = bk.mst_siswa_id AND s.mst_kelas_id = $2 AND s.deleted_at IS NULL)`
			args = append(args, *f.KelasID)
		}
		if f.TahunAjaranID != nil {
			q += fmt.Sprintf(` AND EXISTS (SELECT 1 FROM mst_siswa s JOIN mst_kelas k ON k.id = s.mst_kelas_id WHERE s.id = bk.mst_siswa_id AND k.tahun_ajaran_id = $%d AND k.deleted_at IS NULL AND s.deleted_at IS NULL)`, len(args)+1)
			args = append(args, *f.TahunAjaranID)
		}
		var err error
		kasusBkProses, err = countQuery(gctx, r.db, q, args...)
		return err
	})

	// PPDB
	g.Go(func() error {
		var err error
		totalPendaftar, err = countQuery(gctx, r.db, `SELECT COUNT(*) FROM ppdb_pendaftaran WHERE deleted_at IS NULL`)
		return err
	})
	g.Go(func() error {
		var err error
		pendaftarDiterima, err = countQuery(gctx, r.db, `SELECT COUNT(*) FROM ppdb_pendaftaran WHERE status_pendaftaran = 'diterima' AND deleted_at IS NULL`)
		return err
	})

	if err := g.Wait(); err != nil {
		return nil, fmt.Errorf("admin summary cards: %w", err)
	}

	return &model.AdminSummaryCards{
		TotalSiswaAktif: totalSiswa,
		TotalGuru:       totalGuru,
		TotalKelas:      totalKelas,
		TotalTunggakanSpp: model.SppTunggakan{
			Amount:      tunggakanAmount,
			Formatted:   formatRupiah(tunggakanAmount),
			Month:       namaBulan(month),
			Year:        year,
			JumlahSiswa: jumlahSiswaTungg,
		},
		KasusBkProses: kasusBkProses,
		PpdbSummary: model.PpdbSummary{
			TotalPendaftar:    totalPendaftar,
			PendaftarDiterima: pendaftarDiterima,
		},
	}, nil
}

// ─── Financial Analytics ──────────────────────────────────────────────────

func (r *DashboardRepo) GetFinancialAnalytics(ctx context.Context, f model.DashboardFilter) (*model.FinancialAnalytics, error) {
	statusBayarLunas, err := r.refCode(ctx, "status_bayar", "Lunas")
	if err != nil {
		return nil, err
	}

	now := time.Now()
	currentYear := now.Year()
	startDate := now.AddDate(0, -11, 0)
	startDate = time.Date(startDate.Year(), startDate.Month(), 1, 0, 0, 0, 0, startDate.Location())
	endDate := time.Date(now.Year(), now.Month()+1, 0, 23, 59, 59, 0, now.Location())

	// ── SPP trend (last 12 months) ──
	type trendRow struct {
		Year             int     `db:"year"`
		Month            int     `db:"month"`
		TotalPendapatan  float64 `db:"total_pendapatan"`
		JumlahTransaksi  int64   `db:"jumlah_transaksi"`
	}

	trendQ := `
		SELECT EXTRACT(YEAR FROM p.tanggal_bayar)::int AS year,
		       EXTRACT(MONTH FROM p.tanggal_bayar)::int AS month,
		       COALESCE(SUM(p.jumlah_bayar), 0) AS total_pendapatan,
		       COUNT(*) AS jumlah_transaksi
		FROM trx_pembayaran_spp p
		WHERE p.status = $1
		  AND p.tanggal_bayar BETWEEN $2 AND $3`
	trendArgs := []interface{}{statusBayarLunas, startDate, endDate}
	trendQ += r.buildSppJoinFilters(&trendArgs, f)
	trendQ += ` GROUP BY year, month ORDER BY year, month`

	var trendRows []trendRow
	if err := r.db.SelectContext(ctx, &trendRows, trendQ, trendArgs...); err != nil {
		return nil, fmt.Errorf("spp trend: %w", err)
	}

	// Build 12-month chart
	months12 := make([]string, 12)
	pendapatan12 := make([]int64, 12)
	transaksi12 := make([]int64, 12)
	for i := 0; i < 12; i++ {
		d := now.AddDate(0, -(11 - i), 0)
		months12[i] = namaBulanShort(int(d.Month())) + " " + fmt.Sprintf("%d", d.Year())
		for _, row := range trendRows {
			if row.Year == d.Year() && row.Month == int(d.Month()) {
				pendapatan12[i] = int64(row.TotalPendapatan)
				transaksi12[i] = row.JumlahTransaksi
				break
			}
		}
	}

	// ── Payment status distribution ──
	type statusRow struct {
		Status string  `db:"status"`
		Total  int64   `db:"total"`
		Amount float64 `db:"amount"`
	}
	statusQ := `
		SELECT p.status, COUNT(*) AS total, COALESCE(SUM(p.jumlah_bayar), 0) AS amount
		FROM trx_pembayaran_spp p
		WHERE p.tahun = $1`
	statusArgs := []interface{}{currentYear}
	statusQ += r.buildSppJoinFilters(&statusArgs, f)
	statusQ += ` GROUP BY p.status`

	var statusRows []statusRow
	if err := r.db.SelectContext(ctx, &statusRows, statusQ, statusArgs...); err != nil {
		return nil, fmt.Errorf("payment status: %w", err)
	}

	var lunasCount, belumLunasCount int64
	for _, sr := range statusRows {
		if sr.Status == statusBayarLunas {
			lunasCount = sr.Total
		} else {
			belumLunasCount += sr.Total
		}
	}
	totalCount := lunasCount + belumLunasCount

	pct := func(n int64) float64 {
		if totalCount == 0 {
			return 0
		}
		return round2(float64(n) / float64(totalCount) * 100)
	}

	// ── Yearly total income ──
	yearlyQ := `SELECT COALESCE(SUM(p.jumlah_bayar), 0) FROM trx_pembayaran_spp p WHERE p.tahun = $1 AND p.status = $2`
	yearlyArgs := []interface{}{currentYear, statusBayarLunas}
	yearlyQ += r.buildSppJoinFilters(&yearlyArgs, f)

	var yearlyTotal float64
	if err := r.db.QueryRowContext(ctx, yearlyQ, yearlyArgs...).Scan(&yearlyTotal); err != nil {
		return nil, fmt.Errorf("yearly total: %w", err)
	}

	formattedPendapatan := make([]string, 12)
	for i, v := range pendapatan12 {
		formattedPendapatan[i] = formatRupiah(float64(v))
	}

	return &model.FinancialAnalytics{
		SppTrend: model.SppTrend{
			Labels: months12,
			Datasets: []model.SppDataset{
				{
					Label:         "Pendapatan SPP",
					Data:          pendapatan12,
					FormattedData: formattedPendapatan,
				},
				{
					Label:   "Jumlah Transaksi",
					Data:    transaksi12,
					YAxisID: "y1",
				},
			},
		},
		PaymentStatusDistribution: model.PaymentStatusDonut{
			Labels:      []string{"Lunas", "Belum Lunas"},
			Data:        []int64{lunasCount, belumLunasCount},
			Percentages: []float64{pct(lunasCount), pct(belumLunasCount)},
			Colors:      []string{"#10B981", "#EF4444"},
		},
		YearlySummary: model.YearlySummary{
			TotalPendapatan: yearlyTotal,
			FormattedTotal:  formatRupiah(yearlyTotal),
			Year:            currentYear,
			TotalLunas:      lunasCount,
			TotalBelumLunas: belumLunasCount,
		},
	}, nil
}

// buildSppJoinFilters appends WHERE clauses for kelas/tahun_ajaran filters on
// trx_pembayaran_spp (aliased as p) and grows the args slice.
func (r *DashboardRepo) buildSppJoinFilters(args *[]interface{}, f model.DashboardFilter) string {
	var sb strings.Builder
	if f.KelasID != nil {
		*args = append(*args, *f.KelasID)
		sb.WriteString(fmt.Sprintf(
			` AND EXISTS (SELECT 1 FROM mst_siswa s WHERE s.id = p.mst_siswa_id AND s.mst_kelas_id = $%d AND s.deleted_at IS NULL)`,
			len(*args),
		))
	}
	if f.TahunAjaranID != nil {
		*args = append(*args, *f.TahunAjaranID)
		sb.WriteString(fmt.Sprintf(
			` AND EXISTS (SELECT 1 FROM mst_siswa s JOIN mst_kelas k ON k.id = s.mst_kelas_id WHERE s.id = p.mst_siswa_id AND k.tahun_ajaran_id = $%d AND k.deleted_at IS NULL AND s.deleted_at IS NULL)`,
			len(*args),
		))
	}
	return sb.String()
}

// ─── Academic & Attendance Analytics ─────────────────────────────────────

func (r *DashboardRepo) GetAcademicAttendanceAnalytics(ctx context.Context, f model.DashboardFilter) (*model.AcademicAttendanceAnalytics, error) {
	statusHadir, err := r.refCode(ctx, "status_absensi", "hadir")
	if err != nil {
		return nil, err
	}
	statusIzin, err := r.refCode(ctx, "status_absensi", "izin")
	if err != nil {
		return nil, err
	}
	statusSakit, err := r.refCode(ctx, "status_absensi", "sakit")
	if err != nil {
		return nil, err
	}
	statusAlpha, err := r.refCode(ctx, "status_absensi", "alpha")
	if err != nil {
		return nil, err
	}

	now := time.Now()
	startDate := now.AddDate(0, 0, -6)
	startDate = time.Date(startDate.Year(), startDate.Month(), startDate.Day(), 0, 0, 0, 0, startDate.Location())
	endDate := time.Date(now.Year(), now.Month(), now.Day(), 23, 59, 59, 0, now.Location())

	// ── Absensi last 7 days ──
	type absensiRow struct {
		Tanggal string `db:"tanggal"`
		Status  string `db:"status"`
		Total   int64  `db:"total"`
	}

	absensiQ := `
		SELECT TO_CHAR(a.tanggal, 'YYYY-MM-DD') AS tanggal, a.status::text AS status, COUNT(*) AS total
		FROM trx_absensi_siswa a
		WHERE a.tanggal BETWEEN $1 AND $2
		  AND a.deleted_at IS NULL`
	absensiArgs := []interface{}{startDate.Format("2006-01-02"), endDate.Format("2006-01-02")}

	if f.KelasID != nil {
		absensiArgs = append(absensiArgs, *f.KelasID)
		absensiQ += fmt.Sprintf(` AND EXISTS (SELECT 1 FROM mst_siswa s WHERE s.id = a.mst_siswa_id AND s.mst_kelas_id = $%d AND s.deleted_at IS NULL)`, len(absensiArgs))
	}
	if f.TahunAjaranID != nil {
		absensiArgs = append(absensiArgs, *f.TahunAjaranID)
		absensiQ += fmt.Sprintf(` AND EXISTS (SELECT 1 FROM mst_siswa s JOIN mst_kelas k ON k.id = s.mst_kelas_id WHERE s.id = a.mst_siswa_id AND k.tahun_ajaran_id = $%d AND k.deleted_at IS NULL AND s.deleted_at IS NULL)`, len(absensiArgs))
	}
	absensiQ += ` GROUP BY a.tanggal, a.status ORDER BY a.tanggal`

	var absensiRows []absensiRow
	if err := r.db.SelectContext(ctx, &absensiRows, absensiQ, absensiArgs...); err != nil {
		return nil, fmt.Errorf("absensi 7 days: %w", err)
	}

	dates := make([]string, 7)
	hadirData := make([]int64, 7)
	izinData := make([]int64, 7)
	sakitData := make([]int64, 7)
	alphaData := make([]int64, 7)

	for i := 6; i >= 0; i-- {
		d := now.AddDate(0, 0, -i)
		idx := 6 - i
		dates[idx] = d.Format("Mon, 02 Jan")
		dateStr := d.Format("2006-01-02")
		for _, row := range absensiRows {
			if row.Tanggal == dateStr {
				switch row.Status {
				case statusHadir:
					hadirData[idx] = row.Total
				case statusIzin:
					izinData[idx] = row.Total
				case statusSakit:
					sakitData[idx] = row.Total
				case statusAlpha:
					alphaData[idx] = row.Total
				}
			}
		}
	}

	totalHadir := int64(0)
	for _, v := range hadirData {
		totalHadir += v
	}
	totalIzin := int64(0)
	for _, v := range izinData {
		totalIzin += v
	}
	totalSakit := int64(0)
	for _, v := range sakitData {
		totalSakit += v
	}
	totalAlpha := int64(0)
	for _, v := range alphaData {
		totalAlpha += v
	}
	totalRecords := totalHadir + totalIzin + totalSakit + totalAlpha
	rataRataKehadiran := 0.0
	if totalRecords > 0 {
		rataRataKehadiran = round2(float64(totalHadir) / float64(totalRecords) * 100)
	}

	// ── Nilai distribution ──
	type nilaiStats struct {
		Total        int64   `db:"total"`
		RataRata     float64 `db:"rata_rata"`
		MaxNilai     float64 `db:"max_nilai"`
		MinNilai     float64 `db:"min_nilai"`
		SangatBaik   int64   `db:"sangat_baik"`
		Baik         int64   `db:"baik"`
		Cukup        int64   `db:"cukup"`
		Kurang       int64   `db:"kurang"`
		SangatKurang int64   `db:"sangat_kurang"`
	}

	nilaiQ := `
		SELECT COUNT(*) AS total,
		       COALESCE(AVG(n.nilai), 0) AS rata_rata,
		       COALESCE(MAX(n.nilai), 0) AS max_nilai,
		       COALESCE(MIN(n.nilai), 0) AS min_nilai,
		       SUM(CASE WHEN n.nilai >= 90 THEN 1 ELSE 0 END) AS sangat_baik,
		       SUM(CASE WHEN n.nilai >= 80 AND n.nilai < 90 THEN 1 ELSE 0 END) AS baik,
		       SUM(CASE WHEN n.nilai >= 70 AND n.nilai < 80 THEN 1 ELSE 0 END) AS cukup,
		       SUM(CASE WHEN n.nilai >= 60 AND n.nilai < 70 THEN 1 ELSE 0 END) AS kurang,
		       SUM(CASE WHEN n.nilai < 60 THEN 1 ELSE 0 END) AS sangat_kurang
		FROM trx_nilai n
		JOIN trx_ujian u ON u.id = n.trx_ujian_id`
	nilaiArgs := []interface{}{}

	conditions := []string{"n.deleted_at IS NULL", "u.deleted_at IS NULL"}
	if f.KelasID != nil {
		nilaiArgs = append(nilaiArgs, *f.KelasID)
		conditions = append(conditions, fmt.Sprintf(`u.mst_kelas_id = $%d`, len(nilaiArgs)))
	}
	if f.TahunAjaranID != nil {
		nilaiArgs = append(nilaiArgs, *f.TahunAjaranID)
		conditions = append(conditions, fmt.Sprintf(`EXISTS (SELECT 1 FROM mst_kelas k WHERE k.id = u.mst_kelas_id AND k.tahun_ajaran_id = $%d AND k.deleted_at IS NULL)`, len(nilaiArgs)))
	}
	if len(conditions) > 0 {
		nilaiQ += " WHERE " + strings.Join(conditions, " AND ")
	}

	var ns nilaiStats
	if err := r.db.QueryRowContext(ctx, nilaiQ, nilaiArgs...).Scan(
		&ns.Total, &ns.RataRata, &ns.MaxNilai, &ns.MinNilai,
		&ns.SangatBaik, &ns.Baik, &ns.Cukup, &ns.Kurang, &ns.SangatKurang,
	); err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("nilai stats: %w", err)
	}

	pctNilai := func(n int64) float64 {
		if ns.Total == 0 {
			return 0
		}
		return round2(float64(n) / float64(ns.Total) * 100)
	}

	return &model.AcademicAttendanceAnalytics{
		Attendance7Days: model.Attendance7Days{
			Labels: dates,
			Datasets: []model.AttendanceDataset{
				{Label: "Hadir", Data: hadirData, Color: "#10B981"},
				{Label: "Izin", Data: izinData, Color: "#3B82F6"},
				{Label: "Sakit", Data: sakitData, Color: "#F59E0B"},
				{Label: "Alpha", Data: alphaData, Color: "#EF4444"},
			},
		},
		AttendanceSummary: model.AttendanceSummary{
			RataRataKehadiran: rataRataKehadiran,
			TotalHadir7Hari:  totalHadir,
			TotalRecords:     totalRecords,
		},
		NilaiDistribution: model.NilaiDistribution{
			Labels: []string{
				"Sangat Baik (90-100)",
				"Baik (80-89)",
				"Cukup (70-79)",
				"Kurang (60-69)",
				"Sangat Kurang (<60)",
			},
			Data:        []int64{ns.SangatBaik, ns.Baik, ns.Cukup, ns.Kurang, ns.SangatKurang},
			Percentages: []float64{pctNilai(ns.SangatBaik), pctNilai(ns.Baik), pctNilai(ns.Cukup), pctNilai(ns.Kurang), pctNilai(ns.SangatKurang)},
			Colors:      []string{"#10B981", "#3B82F6", "#F59E0B", "#EF4444", "#6B7280"},
		},
		NilaiSummary: model.NilaiSummary{
			RataRata:       round2(ns.RataRata),
			TotalUjian:     ns.Total,
			NilaiTertinggi: ns.MaxNilai,
			NilaiTerendah:  ns.MinNilai,
		},
	}, nil
}

// ─── Counseling Insights ──────────────────────────────────────────────────

func (r *DashboardRepo) GetCounselingInsights(ctx context.Context, f model.DashboardFilter) (*model.CounselingInsights, error) {
	currentYear := time.Now().Year()

	statusSelesai, err := r.refCode(ctx, "status_bk", "selesai")
	if err != nil {
		return nil, err
	}
	statusProses, err := r.refCode(ctx, "status_bk", "proses")
	if err != nil {
		return nil, err
	}
	statusDibuka, err := r.refCode(ctx, "status_bk", "dibuka")
	if err != nil {
		return nil, err
	}
	statusDirujuk, err := r.refCode(ctx, "status_bk", "dirujuk")
	if err != nil {
		return nil, err
	}

	// ── Top 5 kategori ──
	type kategoriRow struct {
		KategoriID int64  `db:"mst_bk_kategori_id"`
		NamaKategori string `db:"nama_kategori"`
		Total      int64  `db:"total_kasus"`
	}

	topQ := `
		SELECT bk.mst_bk_kategori_id, COALESCE(k.nama, 'Tidak Diketahui') AS nama_kategori, COUNT(*) AS total_kasus
		FROM trx_bk_kasus bk
		LEFT JOIN mst_bk_kategori k ON k.id = bk.mst_bk_kategori_id AND k.deleted_at IS NULL
		WHERE bk.deleted_at IS NULL`
	topArgs := []interface{}{}
	topQ += r.buildBkSiswaFilters(&topArgs, f)
	topQ += ` GROUP BY bk.mst_bk_kategori_id, k.nama ORDER BY total_kasus DESC LIMIT 5`

	var topRows []kategoriRow
	if err := r.db.SelectContext(ctx, &topRows, topQ, topArgs...); err != nil {
		return nil, fmt.Errorf("top kategori: %w", err)
	}

	topLabels := make([]string, len(topRows))
	topData := make([]int64, len(topRows))
	for i, row := range topRows {
		topLabels[i] = row.NamaKategori
		topData[i] = row.Total
	}

	// ── Status distribution ──
	type statusRow struct {
		Status string `db:"status"`
		Total  int64  `db:"total"`
	}
	statusQ := `
		SELECT bk.status::text AS status, COUNT(*) AS total
		FROM trx_bk_kasus bk
		WHERE bk.deleted_at IS NULL`
	statusArgs := []interface{}{}
	statusQ += r.buildBkSiswaFilters(&statusArgs, f)
	statusQ += ` GROUP BY bk.status`

	var statusRows []statusRow
	if err := r.db.SelectContext(ctx, &statusRows, statusQ, statusArgs...); err != nil {
		return nil, fmt.Errorf("bk status: %w", err)
	}

	findStatus := func(code string) int64 {
		for _, sr := range statusRows {
			if sr.Status == code {
				return sr.Total
			}
		}
		return 0
	}

	selesai := findStatus(statusSelesai)
	proses := findStatus(statusProses)
	dibuka := findStatus(statusDibuka)
	dirujuk := findStatus(statusDirujuk)
	totalKasus := selesai + proses + dibuka + dirujuk

	pct := func(n int64) float64 {
		if totalKasus == 0 {
			return 0
		}
		return round2(float64(n) / float64(totalKasus) * 100)
	}

	// ── Monthly ──
	type monthlyRow struct {
		Month int64 `db:"month"`
		Total int64 `db:"total"`
	}
	monthlyQ := `
		SELECT EXTRACT(MONTH FROM bk.tanggal_mulai)::int AS month, COUNT(*) AS total
		FROM trx_bk_kasus bk
		WHERE EXTRACT(YEAR FROM bk.tanggal_mulai) = $1
		  AND bk.deleted_at IS NULL`
	monthlyArgs := []interface{}{currentYear}
	monthlyQ += r.buildBkSiswaFilters(&monthlyArgs, f)
	monthlyQ += ` GROUP BY month ORDER BY month`

	var monthlyRows []monthlyRow
	if err := r.db.SelectContext(ctx, &monthlyRows, monthlyQ, monthlyArgs...); err != nil {
		return nil, fmt.Errorf("bk monthly: %w", err)
	}

	monthlyLabels := make([]string, 12)
	monthlyCounts := make([]int64, 12)
	for i := 1; i <= 12; i++ {
		monthlyLabels[i-1] = namaBulan(i)
		for _, row := range monthlyRows {
			if row.Month == int64(i) {
				monthlyCounts[i-1] = row.Total
				break
			}
		}
	}

	return &model.CounselingInsights{
		TopKategoriKasus: model.TopKategoriKasus{
			Labels: topLabels,
			Data:   topData,
		},
		StatusPenyelesaian: model.StatusPenyelesaian{
			Labels:      []string{"Selesai", "Proses", "Dibuka", "Dirujuk"},
			Data:        []int64{selesai, proses, dibuka, dirujuk},
			Percentages: []float64{pct(selesai), pct(proses), pct(dibuka), pct(dirujuk)},
			Colors:      []string{"#10B981", "#3B82F6", "#F59E0B", "#EF4444"},
		},
		KasusPerBulan: model.KasusPerBulan{
			Labels: monthlyLabels,
			Data:   monthlyCounts,
		},
		Ringkasan: model.BkRingkasan{
			TotalKasus:             totalKasus,
			KasusSelesai:           selesai,
			KasusProses:            proses,
			KasusDibuka:            dibuka,
			KasusDirujuk:           dirujuk,
			PersentasePenyelesaian: pct(selesai),
		},
	}, nil
}

// buildBkSiswaFilters appends siswa-scoped kelas/tahun filters for trx_bk_kasus (aliased as bk).
func (r *DashboardRepo) buildBkSiswaFilters(args *[]interface{}, f model.DashboardFilter) string {
	var sb strings.Builder
	if f.KelasID != nil {
		*args = append(*args, *f.KelasID)
		sb.WriteString(fmt.Sprintf(
			` AND EXISTS (SELECT 1 FROM mst_siswa s WHERE s.id = bk.mst_siswa_id AND s.mst_kelas_id = $%d AND s.deleted_at IS NULL)`,
			len(*args),
		))
	}
	if f.TahunAjaranID != nil {
		*args = append(*args, *f.TahunAjaranID)
		sb.WriteString(fmt.Sprintf(
			` AND EXISTS (SELECT 1 FROM mst_siswa s JOIN mst_kelas k ON k.id = s.mst_kelas_id WHERE s.id = bk.mst_siswa_id AND k.tahun_ajaran_id = $%d AND k.deleted_at IS NULL AND s.deleted_at IS NULL)`,
			len(*args),
		))
	}
	return sb.String()
}

// ─── PPDB Insights ────────────────────────────────────────────────────────

func (r *DashboardRepo) GetPpdbInsights(ctx context.Context) (*model.PpdbInsights, error) {
	currentYear := time.Now().Year()

	type statusRow struct {
		Status string `db:"status_pendaftaran"`
		Total  int64  `db:"total"`
	}
	var statusRows []statusRow
	if err := r.db.SelectContext(ctx, &statusRows, `
		SELECT status_pendaftaran, COUNT(*) AS total
		FROM ppdb_pendaftaran
		WHERE deleted_at IS NULL
		GROUP BY status_pendaftaran`,
	); err != nil {
		return nil, fmt.Errorf("ppdb status: %w", err)
	}

	statusLabels := make([]string, len(statusRows))
	statusData := make([]int64, len(statusRows))
	colors := []string{"#6B7280", "#10B981", "#3B82F6", "#F59E0B", "#EF4444", "#EC4899"}
	for i, sr := range statusRows {
		statusLabels[i] = sr.Status
		statusData[i] = sr.Total
	}
	for len(colors) < len(statusRows) {
		colors = append(colors, "#6B7280")
	}

	type monthlyRow struct {
		Month int64 `db:"month"`
		Total int64 `db:"total"`
	}
	var monthlyRows []monthlyRow
	if err := r.db.SelectContext(ctx, &monthlyRows, `
		SELECT EXTRACT(MONTH FROM created_at)::int AS month, COUNT(*) AS total
		FROM ppdb_pendaftaran
		WHERE EXTRACT(YEAR FROM created_at) = $1
		  AND deleted_at IS NULL
		GROUP BY month ORDER BY month`,
		currentYear,
	); err != nil {
		return nil, fmt.Errorf("ppdb monthly: %w", err)
	}

	monthlyLabels := make([]string, 12)
	monthlyCounts := make([]int64, 12)
	for i := 1; i <= 12; i++ {
		monthlyLabels[i-1] = namaBulan(i)
		for _, row := range monthlyRows {
			if row.Month == int64(i) {
				monthlyCounts[i-1] = row.Total
				break
			}
		}
	}

	return &model.PpdbInsights{
		StatusDistribution: model.StatusDistribution{
			Labels: statusLabels,
			Data:   statusData,
			Colors: colors[:len(statusRows)],
		},
		RegistrationsPerMonth: model.RegistrationsPerMonth{
			Labels: monthlyLabels,
			Data:   monthlyCounts,
		},
	}, nil
}

// ─── Siswa Summary Cards ──────────────────────────────────────────────────

func (r *DashboardRepo) GetSiswaSummaryCards(ctx context.Context, siswaID, kelasID int64) (*model.SiswaSummaryCards, error) {
	statusBayarLunas, err := r.refCode(ctx, "status_bayar", "Lunas")
	if err != nil {
		return nil, err
	}

	now := time.Now()
	startOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	endOfMonth := startOfMonth.AddDate(0, 1, -1)

	var (
		attendanceThisMonth int64
		pendingSppCount     int64
		recentGradeCount    int64
		upcomingTaskCount   int64
	)

	g, gctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		var err error
		attendanceThisMonth, err = countQuery(gctx, r.db,
			`SELECT COUNT(*) FROM trx_absensi_siswa WHERE mst_siswa_id = $1 AND tanggal BETWEEN $2 AND $3 AND deleted_at IS NULL`,
			siswaID, startOfMonth.Format("2006-01-02"), endOfMonth.Format("2006-01-02"),
		)
		return err
	})

	g.Go(func() error {
		var err error
		pendingSppCount, err = countQuery(gctx, r.db,
			`SELECT COUNT(*) FROM trx_pembayaran_spp WHERE mst_siswa_id = $1 AND status != $2`,
			siswaID, statusBayarLunas,
		)
		return err
	})

	g.Go(func() error {
		var err error
		recentGradeCount, err = countQuery(gctx, r.db,
			`SELECT COUNT(*) FROM trx_nilai WHERE mst_siswa_id = $1 AND created_at >= NOW() - INTERVAL '30 days' AND deleted_at IS NULL`,
			siswaID,
		)
		return err
	})

	g.Go(func() error {
		var err error
		upcomingTaskCount, err = countQuery(gctx, r.db,
			`SELECT COUNT(*) FROM mst_tugas WHERE mst_kelas_id = $1 AND tenggat_waktu >= NOW() AND status = 1 AND deleted_at IS NULL`,
			kelasID,
		)
		return err
	})

	if err := g.Wait(); err != nil {
		return nil, fmt.Errorf("siswa summary cards: %w", err)
	}

	return &model.SiswaSummaryCards{
		AttendanceThisMonth: attendanceThisMonth,
		PendingSppCount:     pendingSppCount,
		RecentGradeCount:    recentGradeCount,
		UpcomingTaskCount:   upcomingTaskCount,
	}, nil
}

// ─── Guru Summary Cards ───────────────────────────────────────────────────

func (r *DashboardRepo) GetGuruSummaryCards(ctx context.Context, guruID int64) (*model.GuruSummaryCards, error) {
	// Get kelas IDs managed by this guru as wali kelas
	var kelasIDs []int64
	if err := r.db.SelectContext(ctx, &kelasIDs,
		`SELECT id FROM mst_kelas WHERE wali_guru_id = $1 AND deleted_at IS NULL`, guruID,
	); err != nil {
		return nil, fmt.Errorf("guru kelas wali: %w", err)
	}

	totalKelasWali := int64(len(kelasIDs))
	var totalSiswaWali, totalMapel, tugasBelumDinilai int64

	g, gctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		if len(kelasIDs) == 0 {
			return nil
		}
		query, args, err := sqlx.In(`SELECT COUNT(*) FROM mst_siswa WHERE mst_kelas_id IN (?) AND deleted_at IS NULL`, kelasIDs)
		if err != nil {
			return err
		}
		query = r.db.Rebind(query)
		totalSiswaWali, err = countQuery(gctx, r.db, query, args...)
		return err
	})

	g.Go(func() error {
		var err error
		totalMapel, err = countQuery(gctx, r.db,
			`SELECT COUNT(DISTINCT mst_mapel_id) FROM mst_guru_mapel WHERE mst_guru_id = $1 AND deleted_at IS NULL`,
			guruID,
		)
		return err
	})

	g.Go(func() error {
		var err error
		tugasBelumDinilai, err = countQuery(gctx, r.db, `
			SELECT COUNT(*)
			FROM trx_tugas_siswa ts
			JOIN mst_tugas t ON t.id = ts.mst_tugas_id
			JOIN mst_guru_mapel gm ON gm.id = t.mst_guru_mapel_id
			WHERE gm.mst_guru_id = $1
			  AND ts.nilai IS NULL
			  AND t.deleted_at IS NULL
			  AND ts.deleted_at IS NULL`,
			guruID,
		)
		return err
	})

	if err := g.Wait(); err != nil {
		return nil, fmt.Errorf("guru summary cards: %w", err)
	}

	return &model.GuruSummaryCards{
		TotalKelasWali:    totalKelasWali,
		TotalSiswaWali:    totalSiswaWali,
		TotalMapel:        totalMapel,
		TugasBelumDinilai: tugasBelumDinilai,
	}, nil
}

// GetStatusBayarLunas is a convenience accessor so service layer can obtain
// the cached "Lunas" kode without duplicating refCode logic.
func (r *DashboardRepo) GetStatusBayarLunas(ctx context.Context) (string, error) {
	return r.refCode(ctx, "status_bayar", "Lunas")
}

// ─── Guru profile ─────────────────────────────────────────────────────────

type GuruRow struct {
	ID   int64  `db:"id"`
	Nama string `db:"nama"`
	NIP  string `db:"nip"`
}

func (r *DashboardRepo) GetGuruByUserID(ctx context.Context, userID int64) (*GuruRow, error) {
	var row GuruRow
	err := r.db.QueryRowxContext(ctx,
		`SELECT id, nama, COALESCE(nip,'') AS nip FROM mst_guru WHERE sys_user_id = $1 AND deleted_at IS NULL LIMIT 1`,
		userID,
	).StructScan(&row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &row, err
}

// ─── Siswa profile ────────────────────────────────────────────────────────

type SiswaRow struct {
	ID        int64  `db:"id"`
	Nama      string `db:"nama"`
	NIS       string `db:"nis"`
	KelasID   int64  `db:"mst_kelas_id"`
	NamaKelas string `db:"nama_kelas"`
}

func (r *DashboardRepo) GetSiswaByUserID(ctx context.Context, userID int64) (*SiswaRow, error) {
	var row SiswaRow
	err := r.db.QueryRowxContext(ctx, `
		SELECT s.id, s.nama, COALESCE(s.nis,'') AS nis, s.mst_kelas_id,
		       COALESCE(k.nama_kelas,'') AS nama_kelas
		FROM mst_siswa s
		LEFT JOIN mst_kelas k ON k.id = s.mst_kelas_id AND k.deleted_at IS NULL
		WHERE s.sys_user_id = $1 AND s.deleted_at IS NULL
		LIMIT 1`,
		userID,
	).StructScan(&row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &row, err
}

// ─── Siswa full dashboard data ────────────────────────────────────────────

func (r *DashboardRepo) GetSiswaDashboardData(ctx context.Context, siswaID, kelasID int64) (
	attendanceSummary []model.AttendanceItem,
	unpaidSpp []model.UnpaidSppItem,
	recentGrades []model.RecentGrade,
	upcomingTasks []model.UpcomingTask,
	err error,
) {
	statusBayarLunas, err := r.refCode(ctx, "status_bayar", "Lunas")
	if err != nil {
		return
	}

	g, gctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		rows := []struct {
			Status int64 `db:"status"`
			Total  int64 `db:"total"`
		}{}
		if e := r.db.SelectContext(gctx, &rows, `
			SELECT status, COUNT(*) AS total
			FROM trx_absensi_siswa
			WHERE mst_siswa_id = $1 AND deleted_at IS NULL
			GROUP BY status`,
			siswaID,
		); e != nil {
			return e
		}
		for _, row := range rows {
			attendanceSummary = append(attendanceSummary, model.AttendanceItem{
				Status:      row.Status,
				Total:       row.Total,
				StatusLabel: statusAbsensiLabel(row.Status),
			})
		}
		return nil
	})

	g.Go(func() error {
		return r.db.SelectContext(gctx, &unpaidSpp, `
			SELECT id, bulan, tahun, COALESCE(jumlah_bayar, 0) AS jumlah_bayar, status
			FROM trx_pembayaran_spp
			WHERE mst_siswa_id = $1 AND status != $2
			ORDER BY tahun DESC, bulan DESC
			LIMIT 3`,
			siswaID, statusBayarLunas,
		)
	})

	g.Go(func() error {
		return r.db.SelectContext(gctx, &recentGrades, `
			SELECT n.id, n.nilai,
			       COALESCE(u.nama,'') AS nama_ujian,
			       COALESCE(m.nama_mapel,'') AS nama_mapel
			FROM trx_nilai n
			JOIN trx_ujian u ON u.id = n.trx_ujian_id
			LEFT JOIN mst_mapel m ON m.id = u.mst_mapel_id
			WHERE n.mst_siswa_id = $1 AND n.deleted_at IS NULL
			ORDER BY n.created_at DESC
			LIMIT 5`,
			siswaID,
		)
	})

	g.Go(func() error {
		return r.db.SelectContext(gctx, &upcomingTasks, `
			SELECT id, judul,
			       TO_CHAR(tenggat_waktu, 'DD Mon YYYY HH24:MI') AS tenggat_waktu
			FROM mst_tugas
			WHERE mst_kelas_id = $1
			  AND tenggat_waktu >= NOW()
			  AND status = 1
			  AND deleted_at IS NULL
			ORDER BY tenggat_waktu ASC
			LIMIT 5`,
			kelasID,
		)
	})

	err = g.Wait()
	return
}

// ─── Wali dashboard ───────────────────────────────────────────────────────

type WaliRow struct {
	Nama string `db:"nama"`
}

func (r *DashboardRepo) GetWaliByUserID(ctx context.Context, userID int64) (*WaliRow, error) {
	var row WaliRow
	err := r.db.QueryRowxContext(ctx,
		`SELECT nama FROM mst_wali WHERE sys_user_id = $1 AND deleted_at IS NULL LIMIT 1`,
		userID,
	).StructScan(&row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &row, err
}

type ChildRow struct {
	ID        int64  `db:"id"`
	Nama      string `db:"nama"`
	KelasID   int64  `db:"mst_kelas_id"`
	NamaKelas string `db:"nama_kelas"`
}

func (r *DashboardRepo) GetWaliChildren(ctx context.Context, waliID int64) ([]ChildRow, error) {
	var rows []ChildRow
	err := r.db.SelectContext(ctx, &rows, `
		SELECT s.id, s.nama, s.mst_kelas_id,
		       COALESCE(k.nama_kelas, '') AS nama_kelas
		FROM mst_siswa s
		JOIN mst_siswa_wali sw ON sw.mst_siswa_id = s.id
		LEFT JOIN mst_kelas k ON k.id = s.mst_kelas_id AND k.deleted_at IS NULL
		WHERE sw.mst_wali_id = $1 AND s.deleted_at IS NULL`,
		waliID,
	)
	return rows, err
}

func (r *DashboardRepo) GetChildTodayAbsensi(ctx context.Context, siswaID int64) (string, error) {
	today := time.Now().Format("2006-01-02")
	var status int64
	err := r.db.QueryRowContext(ctx,
		`SELECT status FROM trx_absensi_siswa WHERE mst_siswa_id = $1 AND tanggal = $2 AND deleted_at IS NULL LIMIT 1`,
		siswaID, today,
	).Scan(&status)
	if err == sql.ErrNoRows {
		return "Belum Absen", nil
	}
	if err != nil {
		return "", err
	}
	return statusAbsensiLabel(status), nil
}

func (r *DashboardRepo) GetChildTunggakanSppCount(ctx context.Context, siswaID int64, statusBayarLunas string) (int64, error) {
	return countQuery(ctx, r.db,
		`SELECT COUNT(*) FROM trx_pembayaran_spp WHERE mst_siswa_id = $1 AND status != $2`,
		siswaID, statusBayarLunas,
	)
}

type JadwalRow struct {
	NamaMapel  string `db:"nama_mapel"`
	NamaGuru   string `db:"nama_guru"`
	JamMulai   string `db:"jam_mulai"`
	JamSelesai string `db:"jam_selesai"`
	Ruangan    string `db:"ruangan"`
}

func (r *DashboardRepo) GetJadwalHariIni(ctx context.Context, kelasID int64) ([]JadwalRow, error) {
	// PostgreSQL day-of-week: Sunday=0..Saturday=6.  We use TRIM(TO_CHAR) for short English names
	// matching the PHP pattern (strtoupper of D-format e.g. "MON", "TUE", ...)
	var rows []JadwalRow
	err := r.db.SelectContext(ctx, &rows, `
		SELECT COALESCE(m.nama_mapel, '') AS nama_mapel,
		       COALESCE(g.nama, '') AS nama_guru,
		       TO_CHAR(jp.jam_mulai, 'HH24:MI') AS jam_mulai,
		       TO_CHAR(jp.jam_selesai, 'HH24:MI') AS jam_selesai,
		       COALESCE(jp.ruangan, '') AS ruangan
		FROM trx_jadwal_pelajaran jp
		LEFT JOIN mst_guru_mapel gm ON gm.id = jp.mst_guru_mapel_id AND gm.deleted_at IS NULL
		LEFT JOIN mst_mapel m ON m.id = gm.mst_mapel_id AND m.deleted_at IS NULL
		LEFT JOIN mst_guru g ON g.id = gm.mst_guru_id AND g.deleted_at IS NULL
		WHERE jp.mst_kelas_id = $1
		  AND UPPER(jp.hari) = UPPER(TO_CHAR(NOW(), 'DY'))
		  AND jp.deleted_at IS NULL
		ORDER BY jp.jam_mulai`,
		kelasID,
	)
	return rows, err
}
