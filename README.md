# Shop POS Backend — Go Fiber v3

Backend point-of-sale (POS) untuk toko/warung kecil, dibangun dengan **Go Fiber v3** dan Clean Architecture. Menangani produk (fisik & digital), pelanggan, transaksi penjualan, hutang (kasbon), pembayaran online lewat Midtrans (QRIS & Virtual Account), notifikasi push (FCM), serta autentikasi kasir dengan sesi berbasis Redis. Laporan penjualan dan hutang bisa diekspor ke Excel dan PDF.

## Tujuan

Menyediakan API kasir yang ringan namun aman untuk toko kecil: pencatatan stok, transaksi penjualan yang atomik (stok dan hutang berubah dalam satu transaksi DB), pelacakan hutang pelanggan, pembayaran online, dan laporan bulanan. Akses dibedakan antara **kasir/staff** dan **superadmin** lewat JWT + role.

## Fitur

- **Modul domain**: produk, pelanggan, transaksi, hutang (kasbon), pembayaran, FCM, user/sesi
- **Produk fisik & digital**: `ProductType` membedakan produk fisik (dikelola stok) dari produk digital (pulsa/e-wallet/paket data — tanpa stok, butuh nomor tujuan saat penjualan). Produk digital dilewati dari pengurangan/pengembalian stok
- **Transaksi penjualan atomik**: penyesuaian stok dan pencatatan hutang dilakukan dalam satu transaksi database (lihat [internal/repository/](internal/repository/))
- **Pembayaran online (Midtrans)**: charge **QRIS** dan **Virtual Account** (BCA `bank_transfer`, Mandiri `echannel`); harga item dihitung server-side (tidak dikirim client) dan biaya admin ditambahkan otomatis (QRIS 0.7%, VA flat). Status pembayaran diperbarui lewat webhook yang diverifikasi `signature_key`. Produk digital **hanya lewat POS**, ditolak pada charge online
- **Notifikasi push (FCM)**: registrasi & logout device token via Firebase Cloud Messaging
- **Autentikasi & otorisasi**: login/register staff, access + refresh token JWT, sesi disimpan di Redis, role-based access (`superadmin` vs `staff`)
- **Pembuatan akun privileged via CLI**: akun `superadmin` dibuat lewat command `create-admin`, bukan endpoint publik
- **Laporan**: impor produk CSV/Excel (excelize, maks 10.000 baris, hasil per baris dikembalikan) dan PDF (fpdf) untuk struk, laporan bulanan, dan rekap hutang; PDF diunduh lewat `GET /api/reports/:file` (wajib JWT, nama file acak, kedaluwarsa 1 jam; laporan bulanan & hutang hanya superadmin)
- **Keamanan bawaan**: rate limiting (storage Redis) per IP untuk route publik dan per user untuk `/api/*`, bucket terpisah untuk login, refresh & webhook; Helmet/XSS headers, CORS, trusted proxy dengan validasi IP untuk pembacaan IP asli
- **Migrasi versioned dengan goose**: file SQL di-embed ke binary, dijalankan lewat command `migrate` / `migrate-reset`
- **Konfigurasi via Viper** dari file YAML per-environment (`.config.development.yaml` / `.config.production.yaml`)
- **CLI dengan Cobra**: `serve`, `migrate`, `migrate-reset`, `create-admin`
- **Swagger UI** disajikan lewat `gofiber/contrib/v3/swaggerui` di root (`/`), selain di production
- Validasi request dengan go-playground/validator, JSON via bytedance/sonic

## Arsitektur

```
├── main.go              # Entry point — memanggil cmd.Execute()
├── cmd/                 # Cobra commands (serve, migrate, migrate-reset, create-admin)
├── config/
│   ├── env_config/      # Loader & validasi config (Viper)
│   └── fiber_config/    # Setup Fiber, middleware chain, Swagger
├── infrastructure/
│   ├── database/        # Init Postgres (GORM) + migrasi goose (SQL embed)
│   ├── cache/           # Redis (sesi + storage rate limiter)
│   ├── api/payment/     # Gateway Midtrans (charge QRIS/VA, verifikasi webhook)
│   ├── fcm/             # Sender Firebase Cloud Messaging
│   └── logger/          # Logger zap kustom
├── internal/
│   ├── domain/          # Entitas & interface (model, errors, tx)
│   ├── dto/             # Request/response DTO per domain
│   ├── repository/      # Akses data (GORM / Redis)
│   ├── usecase/         # Business logic
│   ├── delivery/http/
│   │   ├── handler/     # Fiber v3 handlers (Ctx sebagai value type)
│   │   ├── middleware/  # JWT, CORS, rate limit, XSS, compress, dll.
│   │   └── route/       # Pendaftaran route
│   └── constant/        # Enum (role, payment, product type, status hutang) & paginasi
└── pkg/
    ├── jwt/             # Generate/verifikasi token
    ├── validator/       # StructValidator Fiber
    ├── response/        # Format response standar
    ├── pdf/             # Laporan PDF (struk, hutang, bulanan)
    └── sheet/           # Ekspor Excel
```

### Alur request

`route` → `middleware` (JWT + role) → `handler` → `usecase` → `repository` → Postgres/Redis. Endpoint `/auth/*` dan webhook `/payments/notification` bersifat publik (webhook diverifikasi via `signature_key`, bukan JWT); sisanya berada di bawah grup `/api` yang dilindungi JWT, dengan beberapa aksi sensitif (delete, update produk, laporan bulanan & hutang) dibatasi hanya untuk `superadmin`.

## Endpoint utama

| Grup | Endpoint | Akses |
|------|----------|-------|
| Auth | `POST /auth/login`, `POST /auth/refresh` | Publik (rate-limited) |
| Auth | `POST /api/auth/register` (membuat staff) | superadmin |
| Reports | `GET /api/reports/:file` | JWT (struk); superadmin (laporan bulanan & hutang) |
| Payments | `POST /payments/notification` (webhook Midtrans) | Publik (rate-limited; diverifikasi `signature_key`) |
| Payments | `POST /api/payments/qris`, `POST /api/payments/va`, `GET /api/payments/:order_id/status` | JWT |
| Products | `POST /api/products`, `POST /api/products/bulk`, `GET /api/products`, `GET /api/products/:id`, `PATCH /api/products/stock` | JWT |
| Products | `PUT /api/products`, `DELETE /api/products` | superadmin |
| Transactions | `POST /api/transactions`, `GET /api/transactions`, `GET /api/transactions/:id`, `GET /api/transactions/report/transaction` | JWT |
| Transactions | `GET /api/transactions/report/month`, `DELETE /api/transactions` | superadmin |
| Customers | `POST /api/customers`, `GET /api/customers`, `GET /api/customers/:id`, `PUT /api/customers` | JWT |
| Customers | `DELETE /api/customers` | superadmin |
| Debts | `POST /api/debts`, `GET /api/debts`, `GET /api/debts/:id` | JWT |
| Debts | `GET /api/debts/report`, `DELETE /api/debts` | superadmin |
| FCM | `POST /api/fcm/register`, `POST /api/fcm/logout` | JWT |

Dokumentasi lengkap tersedia di Swagger UI (root `/`) setelah server berjalan, kecuali di production.

## Stack

Go 1.26 · Fiber v3 · GORM + PostgreSQL · goose (migrasi) · Redis · Midtrans · Firebase FCM · Cobra · Viper · zap · Swagger UI · excelize · fpdf · go-playground/validator · bytedance/sonic

## Menjalankan

### 1. Prasyarat

- Go 1.26+
- PostgreSQL
- Redis

### 2. Konfigurasi

Salin `.config.example.yaml` menjadi `.config.development.yaml` (atau `.config.production.yaml`) dan isi nilainya:

```bash
cp .config.example.yaml .config.development.yaml
```

```yaml
server:
  port: 3030
  name: novi_shop
  env: development
  host: localhost
  trusted_proxies: []      # isi IP/CIDR reverse proxy bila di belakang nginx
database:
  user: postgres
  pass: secret
  port: 5432
  dbname: shop
  host: localhost
  sslmode: disable
  time_zone: Asia/Jakarta
redis:
  url:
  db: 0
  username:
  password:
  port: 6379
  host: localhost
jwt:
  secret: ganti-dengan-secret-kuat
  token_ttl: 3600          # detik
  refresh_token_ttl: 86400 # detik
midtrans:
  server_key: SB-Mid-server-xxxx   # kosong = route pembayaran online membalas 503
  environment: sandbox     # sandbox | production
firebase:
  google_application_credentials: ./serviceAccountKey.json  # path kredensial FCM
```

File config dipilih berdasarkan variabel lingkungan `APP_ENV` (`development`, `staging`, atau `production`). Nilai bisa dioverride lewat env var: titik jadi underscore, huruf besar (`database.pass` → `DATABASE_PASS`). `firebase.google_application_credentials` wajib diisi (divalidasi saat startup); `midtrans.server_key` opsional.

### 3. Migrasi database

```bash
make migrate-dev        # APP_ENV=development go run main.go migrate
# atau reset (drop & migrate ulang):
make migrate-reset-dev
```

### 4. Buat akun superadmin

Endpoint register publik hanya membuat akun `staff`. Akun `superadmin` dibuat lewat CLI:

```bash
APP_ENV=development go run main.go create-admin \
  --username admin --password 'passwordKuat' --role superadmin
```

### 5. Jalankan server

```bash
make serve-dev          # APP_ENV=development go run main.go serve
# produksi:
make serve              # APP_ENV=production go run main.go serve
```

Server berjalan di port sesuai `server.port` (default contoh: `3030`). Swagger UI tersedia di root `/`.

### Perintah Makefile lain

```bash
make build / build-dev  # build binary
make swagger            # generate swagger.json dari anotasi (butuh swag)
make test               # APP_ENV=development go test -v ./...
make fmt / tidy / clean
```

## Catatan Fiber v3

Project ini menargetkan Fiber **v3** (bukan v2): handler menerima `fiber.Ctx` sebagai value type, dan paket contrib memakai module path `/v3` — perlu diperhatikan karena sebagian besar contoh online masih memakai idiom v2.
