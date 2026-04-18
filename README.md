# Dashboard Engine

Microservice untuk agregasi dan penyajian data dashboard aplikasi **Sekolah Pintar**. Layanan ini menyediakan berbagai endpoint analitik yang disesuaikan berdasarkan peran pengguna (Admin, Guru, Siswa).

## Tech Stack

- **Go** 1.25
- **chi** v5 — HTTP router
- **sqlx** + **pgx/v5** — PostgreSQL driver
- **golang-jwt/jwt** v5 — JWT authentication
- **godotenv** — environment variable loader

## Struktur Proyek

```
dashboard-engine/
├── cmd/
│   └── main.go               # Entry point, router, graceful shutdown
├── internal/
│   ├── config/               # Konfigurasi dari environment variables
│   ├── db/                   # Koneksi database (sqlx + pgx)
│   ├── handler/              # HTTP handler layer
│   ├── middleware/           # JWT auth middleware
│   ├── model/                # Struct request/response
│   ├── repository/           # Query database
│   └── service/              # Business logic
├── Dockerfile
└── go.mod
```

## API Endpoints

Semua endpoint (kecuali health check) memerlukan header `Authorization: Bearer <token>`.

| Method | Path | Deskripsi |
|--------|------|-----------|
| `GET` | `/health` | Health check |
| `GET` | `/api/v1/dashboard` | Data dashboard lengkap (role-based) |
| `GET` | `/api/v1/dashboard/summary-cards` | Kartu ringkasan statistik |
| `GET` | `/api/v1/dashboard/financial-analytics` | Analitik keuangan & SPP |
| `GET` | `/api/v1/dashboard/academic-attendance` | Analitik akademik & kehadiran |
| `GET` | `/api/v1/dashboard/counseling-insights` | Insight kasus BK |
| `GET` | `/api/v1/dashboard/ppdb-insights` | Insight pendaftaran peserta didik baru |

### Query Parameters (opsional)

| Parameter | Tipe | Deskripsi |
|-----------|------|-----------|
| `tahun_ajaran_id` | `int64` | Filter berdasarkan ID tahun ajaran |
| `mst_kelas_id` | `int64` | Filter berdasarkan ID kelas |

### Role-based Response

- **Admin** — mendapatkan seluruh data: summary cards, keuangan, akademik, BK, dan PPDB
- **Guru** — mendapatkan profil guru, kelas wali, mata pelajaran, dan tugas belum dinilai
- **Siswa** — mendapatkan profil siswa, kehadiran, SPP belum lunas, dan nilai terbaru

## Konfigurasi

Salin `.env.example` menjadi `.env` dan sesuaikan nilainya:

```env
APP_PORT=8084

JWT_SECRET=your_jwt_secret_here

DB_HOST=127.0.0.1
DB_PORT=5432
DB_DATABASE=db_sekolah
DB_USERNAME=root
DB_PASSWORD=
```

> `JWT_SECRET` bersifat **wajib** — aplikasi akan panic saat startup jika tidak diset.

## Menjalankan Lokal

```bash
# Clone dan masuk ke direktori
cd dashboard-engine

# Salin environment
cp .env.example .env

# Jalankan
go run ./cmd/main.go
```

Server akan berjalan di `http://localhost:8084`.

## Docker

```bash
# Build image
docker build -t dashboard-engine .

# Jalankan container
docker run -p 8084:8084 \
  -e JWT_SECRET=your_secret \
  -e DB_HOST=host.docker.internal \
  -e DB_DATABASE=db_sekolah \
  -e DB_USERNAME=root \
  -e DB_PASSWORD=secret \
  dashboard-engine
```

## Contoh Response

### `GET /health`

```json
{
  "status": "ok",
  "service": "dashboard-engine"
}
```

### `GET /api/v1/dashboard/summary-cards` (Admin)

```json
{
  "success": true,
  "message": "Summary cards retrieved successfully",
  "data": {
    "total_siswa_aktif": 850,
    "total_guru": 42,
    "total_kelas": 24,
    "total_tunggakan_spp": {
      "amount": 15000000,
      "formatted": "Rp 15.000.000",
      "month": "April",
      "year": 2026,
      "jumlah_siswa": 30
    },
    "kasus_bk_proses": 5,
    "ppdb_summary": {
      "total_pendaftar": 120,
      "pendaftar_diterima": 95
    }
  }
}
```

## Lisensi

Internal project — Sekolah Pintar.
