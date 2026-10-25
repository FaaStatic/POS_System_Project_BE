# Shop POS Backend — Go Fiber v3

Backend point-of-sale (POS) untuk toko/warung kecil, dibangun dengan **Go Fiber v3** dan Clean Architecture. Menangani produk (fisik & digital), kategori, pelanggan, staff, transaksi penjualan (termasuk antrean offline dari aplikasi kasir), hutang (kasbon), pembayaran online lewat Midtrans (QRIS & Virtual Account), notifikasi push (FCM), dashboard toko, serta autentikasi dengan sesi berbasis Redis. Laporan penjualan dan hutang dibuat dalam PDF; produk bisa diimpor dari CSV/Excel.

API ini melayani dua klien: **CMS web** untuk owner (superadmin) dan **aplikasi kasir** Android/iOS untuk staff.

## Tujuan

Menyediakan API kasir yang ringan namun aman untuk toko kecil: pencatatan stok, transaksi penjualan yang atomik (stok dan hutang berubah dalam satu transaksi DB), pelacakan hutang pelanggan, pembayaran online, dan laporan bulanan. Akses dibedakan antara **kasir/staff** dan **superadmin** lewat JWT + role.

## Fitur

- **Modul domain**: produk, kategori, pelanggan, transaksi, hutang (kasbon), pembayaran, dashboard, FCM/device, user/sesi
- **Produk fisik & digital**: `ProductType` membedakan produk fisik (dikelola stok) dari produk digital (pulsa/e-wallet/paket data — tanpa stok, butuh nomor tujuan saat penjualan). Produk digital dilewati dari pengurangan/pengembalian stok
- **Transaksi penjualan atomik**: stok dan hutang berubah dalam satu transaksi database. Produk dikunci `FOR UPDATE` berurutan per ID (bebas deadlock); konflik serentak (unique violation, serialization, deadlock) di-retry otomatis dengan backoff (lihat [internal/repository/retry.go](internal/repository/retry.go))
- **Aturan harga**: harga jual harus di atas harga beli dan harga hutang minimal 105% harga jual; dicek di usecase dan dijaga constraint `chk_products_price_rules`. Staff boleh mengubah produk, tapi harga beli, tipe produk, stok, dan kategori baru hanya untuk superadmin
- **Kategori sebagai tabel**: nama unik tanpa membedakan huruf besar/kecil dan spasi di tepi (`key = lower(trim(name))`); kategori yang masih dipakai produk tidak bisa dihapus
- **Transaksi offline (kasir)**: `POST /api/transactions/sync` menerima sampai 100 transaksi tunai/hutang yang dibuat tanpa internet. Tiap item membawa `client_ref` (UUID) sehingga antrean yang dikirim ulang tidak tercatat dua kali, memakai harga yang dibayar pembeli saat itu, dan berhasil/gagal sendiri-sendiri (`retryable` memberi tahu klien perlu kirim ulang atau tidak). Bila stok di server kurang, transaksi tetap diterima, stok berhenti di 0, dan transaksi ditandai `stock_conflict` untuk ditinjau admin
- **Pembatalan transaksi** wajib menyertakan alasan; staff hanya boleh membatalkan transaksinya sendiri di hari yang sama
- **Hutang**: satu hutang terbuka per pelanggan (dijaga partial unique index), pembayaran cicilan dengan `idempotency_key` agar request ulang tidak tercatat dua kali, hutang manual tercatat di `debt_adjustments` untuk laporan
- **Manajemen staff & pelanggan**: nonaktifkan (soft delete), pulihkan, dan hapus permanen. Akun yang masih dirujuk riwayat penjualan/hutang dianonimkan alih-alih dihapus, supaya laporan lama tetap utuh. Menonaktifkan staff langsung mencabut semua sesi dan token push-nya
- **Dashboard**: omzet, laba kotor, hutang (saldo, baru, dibayar), grafik harian, top 5 produk, pembagian metode bayar, device yang sedang login, dan jumlah konflik stok untuk rentang tanggal (maks 366 hari)
- **Pembayaran online (Midtrans)**: charge **QRIS** dan **Virtual Account** (BCA `bank_transfer`, Mandiri `echannel`); harga item dihitung server-side dan biaya admin ditambahkan otomatis (QRIS 0.7%, VA flat). Status diperbarui lewat webhook yang diverifikasi `signature_key` (SHA-512, perbandingan constant-time). Produk digital **hanya lewat POS**, ditolak pada charge online
- **Notifikasi push (FCM)**: registrasi & logout device token via Firebase Cloud Messaging; superadmin bisa melihat dan mencabut device
- **Autentikasi & otorisasi**: access + refresh token JWT (HS256), sesi di Redis, role-based access (`superadmin` vs `staff`); password di-hash bcrypt
- **Auth CMS web**: request dengan header `X-Client: web` menerima refresh token sebagai cookie `HttpOnly; SameSite=Strict`, bukan di body, sehingga JavaScript tidak bisa membacanya. Header itu juga menjadi penjaga CSRF. Login staff dari CMS ditolak (403)
- **Akun privileged via CLI**: `superadmin` dibuat lewat command `create-admin`, bukan endpoint publik
- **Paginasi keyset** (`after_time` + `after_id`) di semua endpoint list, didukung index `(created_at, id)`
- **Laporan**: impor produk CSV/Excel (maks 10.000 baris, hasil per baris dikembalikan) dan PDF untuk struk, laporan bulanan, dan rekap hutang; PDF diunduh lewat `GET /api/reports/:file` (wajib JWT, nama file acak, kedaluwarsa 1 jam)
- **Keamanan bawaan**: rate limiting (storage Redis) per IP untuk route publik dan per user untuk `/api/*`, bucket terpisah untuk login, refresh & webhook; Helmet/XSS headers, CORS (allowlist origin CMS bila `server.cms_origins` diisi), trusted proxy untuk pembacaan IP asli
- **Migrasi versioned dengan goose**: file SQL di-embed ke binary, dijalankan lewat command `migrate` / `migrate-reset`
- **Konfigurasi per environment** via Viper (`.config.<env>.yaml`), override lewat env var, dan secret dari file (`<KEY>_FILE`)
- **Docker Compose** untuk Postgres, Redis, migrasi, dan app; secret dikirim sebagai file, tidak lewat env var container
- **Swagger UI** di root (`/`), di-generate dari anotasi handler

## Arsitektur

Proyek memakai **Clean Architecture** di atas Fiber v3. Kode dibagi per layer, dan dependensi hanya boleh mengarah ke dalam: layer luar (HTTP, database, layanan eksternal) bergantung pada `domain`, tidak pernah sebaliknya. `domain` mendefinisikan entitas dan interface; layer lain mengimplementasikan atau memakainya.

### Layer dan arah dependensi

```
                    ┌──────────────────────────────────────────────────┐
   REST/JSON        │               Fiber v3 app (:3030)               │
  ───────────────►  │ helmet, CORS, global rate limit, Swagger UI (/)  │
                    └────────────────────────┬─────────────────────────┘
                                             ▼
                    ┌──────────────────────────────────────────────────┐
                    │ delivery/http                                    │
                    │ route → middleware (JWT, role, limit) → handler  │
                    │ bind + validasi DTO, error bertipe → HTTP status │
                    └────────────────────────┬─────────────────────────┘
                                             │ domain.XxxUsecase
                                             ▼
                    ┌──────────────────────────────────────────────────┐
                    │ usecase                                          │
                    │ aturan bisnis: harga dari DB, biaya admin, stok  │
                    └──────┬────────────────────┬─────────────────┬────┘
            XxxRepository  │     PaymentGateway │     *fcm.Sender │
                           ▼                    ▼                 ▼
              ┌──────────────────────┐ ┌────────────────┐ ┌──────────────┐
              │      repository      │ │  api/payment   │ │     fcm      │
              │ GORM, tx + retry,    │ │ charge QRIS/VA │ │ push notif   │
              │ lock, keyset paging  │ │ cek signature  │ │              │
              └──────┬────────┬──────┘ └───────┬────────┘ └──────┬───────┘
                     ▼        ▼                ▼                 ▼
              ┌──────────┐ ┌───────┐   ┌──────────────┐   ┌──────────────┐
              │ Postgres │ │ Redis │   │   Midtrans   │   │ Firebase FCM │
              │    18    │ │   8   │   │     API      │   │              │
              └──────────┘ └───▲───┘   └──────┬───────┘   └──────────────┘
                               │              │ webhook
         sesi JWT + rate limit ┘              └──► POST /payments/notification

        ┌──────────────────────────────────────────────────────────────────┐
        │ domain — inti, tidak bergantung pada layer mana pun di atas      │
        │ entitas · interface Usecase/Repository/PaymentGateway · error    │
        │ semua layer di atas mengimplementasikan atau memakai interface   │
        │ di sini                                                          │
        └──────────────────────────────────────────────────────────────────┘
```

| Layer | Paket | Tanggung jawab | Boleh bergantung pada |
| --- | --- | --- | --- |
| Domain | `internal/domain` | Entitas, interface repository/usecase/gateway, error bertipe | `internal/constant`, `internal/dto` |
| Application | `internal/usecase` | Aturan bisnis: validasi lintas entitas, hitung harga & biaya admin, susun response | `domain`, `pkg/*` |
| Delivery | `internal/delivery/http` | Parse & validasi request, panggil usecase, map error → HTTP status | `domain` (interface usecase), `pkg/*` |
| Infrastructure | `internal/repository`, `infrastructure/*` | Query DB, transaksi + retry, Redis, Midtrans, FCM | `domain` |
| Composition root | `cmd/` | Membuat semua dependensi dan menyambungkannya | semua |

Aturan praktis:

- Handler tidak pernah menyentuh GORM atau Redis; repository tidak pernah tahu soal Fiber.
- Usecase hanya mengenal interface (`domain.ProductRepository`, `domain.PaymentGateway`, ...), sehingga bisa diuji dengan fake tanpa database (lihat `internal/usecase/*_test.go`).
- Error dibuat bertipe di domain (`domain.NotFound(...)`, `domain.Conflict(...)`) lalu diterjemahkan ke status HTTP di satu tempat (`handler/common.go`).
- Transaksi database (lock, retry) adalah urusan repository; usecase cukup memanggil satu method seperti `CreateTransaction`.

### Wiring dependensi (composition root)

Semua objek dibuat manual di `cmd/fiber_command.go` (command `serve`), tanpa framework DI:

```
  env_config (yaml + env + secret file)
     │
     ├──► database.InitDB ──► repository.NewXxxRepository ────────┐
     ├──► cache.InitRedis ──► repository.NewSessionRepository ────┤
     └──► jwt service · Midtrans gateway · FCM sender ────────────┤
                                                                  ▼
                                                        usecase.NewXxxUsecase
                                                                  │
                                                                  ▼
                                                        handler.NewXxxHandler
                                                                  │
                                                                  ▼
                                                              route.New
                                                                  │
                                                                  ▼
                                                       fiber_config.InitFiber
                                                                  │
                                                                  ▼
                                                             app.Listen
```

### Struktur folder

```
├── main.go                  # Entry point — memanggil cmd.Execute()
├── cmd/                     # Cobra commands: serve, migrate, migrate-reset, create-admin
├── config/
│   ├── env_config/          # Loader & validasi config (Viper + secret file)
│   └── fiber_config/        # Setup Fiber, middleware chain, Swagger, listener (prefork)
├── infrastructure/
│   ├── database/            # Init Postgres (GORM) + migrasi goose
│   │   └── migrations/      # File SQL versioned (di-embed ke binary)
│   ├── cache/               # Redis (satu client untuk sesi + storage rate limiter)
│   ├── api/payment/         # Gateway Midtrans (charge QRIS/VA, verifikasi webhook)
│   ├── fcm/                 # Sender Firebase Cloud Messaging
│   └── logger/              # Logger zap kustom
├── internal/
│   ├── domain/              # Entitas, interface repository/usecase, error bertipe
│   ├── dto/                 # Request/response DTO per domain
│   ├── repository/          # Akses data (GORM / Redis), transaksi + retry, paginasi
│   ├── usecase/             # Business logic
│   ├── delivery/http/
│   │   ├── handler/         # Fiber v3 handlers + anotasi Swagger
│   │   ├── middleware/      # JWT, CORS, rate limit, XSS, feature flag, logger
│   │   └── route/           # Pendaftaran route
│   └── constant/            # Enum (role, payment, product type, status hutang) & cursor
├── pkg/
│   ├── jwt/                 # Generate/verifikasi token
│   ├── response/            # Format response standar
│   ├── pdf/                 # Laporan PDF (struk, hutang, bulanan)
│   └── sheet/               # Impor produk dari Excel & CSV
├── .dockerfile              # Image multi-stage (binary Go + swagger.json, user non-root)
├── docker-compose.yaml      # postgres, redis, migrate, app
└── Makefile                 # Target lokal & docker-*
```

### Alur request

`route` → `middleware` (rate limit, JWT + role) → `handler` (bind + validasi DTO) → `usecase` → `repository` → Postgres/Redis.

- Publik: `/auth/login`, `/auth/refresh`, dan webhook `/payments/notification` (diverifikasi `signature_key`, bukan JWT).
- Sisanya di bawah grup `/api` yang dilindungi JWT; aksi sensitif (manajemen staff, kategori, device, hapus data, stok, dashboard, laporan bulanan & hutang) hanya untuk `superadmin`.

### Alur konfigurasi

1. `APP_ENV` (`development` | `staging` | `production`) memilih file `.config.<env>.yaml` dari working directory.
2. Setiap key bisa di-override env var: titik jadi underscore, huruf besar (`database.host` → `DATABASE_HOST`).
3. Secret bisa dibaca dari file lewat env `<KEY>_FILE` (pola yang sama dengan image Postgres). Didukung untuk `database.pass`, `redis.password`, `jwt.secret`, `midtrans.server_key`; contoh `JWT_SECRET_FILE=/run/secrets/jwt_secret`. Nilai dari file menang atas yaml dan env.
4. Config divalidasi saat startup (field wajib, `jwt.secret` ≥ 32 karakter, TTL > 0); gagal validasi = app tidak start.

## Infrastruktur (Docker)

```
                     .config.<env>.yaml
                             │ make docker-env
                             ▼
          ┌─────────────────────────────────────────────┐
          │ .docker/<env>/   folder 700, gitignored     │
          │ app.yaml · compose.env · 1 file per secret  │
          └──────┬────────────────────┬─────────────┬───┘
                 │ app.yaml           │ /run/secrets│ redis.conf
                 │ + /run/secrets     │             │
                 ▼                    ▼             ▼
   ┌──────────────────────┐  ┌────────────────┐  ┌─────────────┐
   │ migrate (sekali)     │─►│  postgres:18   │  │   redis:8   │
   │ app :3030            │─►│ 127.0.0.1:5432 │  │ internal    │
   │ prefork off          │  │ volume pg-data │  │ saja        │
   └──────────┬───────────┘  └────────────────┘  └──────▲──────┘
              └─────────────────────────────────────────┘
```

| Service | Peran | Catatan |
| --- | --- | --- |
| `postgres` | Database | Port hanya dibuka ke `127.0.0.1:5432`; data di volume `postgres-data` |
| `redis` | Sesi + rate limiter | Tidak dibuka ke host; password dari `redis.conf` |
| `migrate` | Menjalankan migrasi goose lalu keluar | `app` baru start setelah `migrate` sukses |
| `app` | API server | Prefork dimatikan (`SERVER_PREFORK=false`) agar `docker stop` graceful; `stop_grace_period` 15s |

`make docker-env` (dijalankan otomatis oleh target `docker-*`) membaca `.config.<env>.yaml` dan menghasilkan `.docker/<env>/` (folder mode 700, di-gitignore):

- `app.yaml` — yaml yang sama dengan nilai secret dikosongkan; di-mount ke app sebagai `.config.<env>.yaml`
- `compose.env` — key non-secret untuk interpolasi `${...}` di compose (tidak dipasang ke container)
- `database_pass`, `redis_password`, `jwt_secret`, `midtrans_server_key`, `redis.conf` — dipasang sebagai compose secrets

Akibatnya `docker inspect` hanya menampilkan path seperti `JWT_SECRET_FILE=/run/secrets/jwt_secret`, bukan nilai secret. Kredensial Firebase juga dipasang sebagai secret (`/run/secrets/firebase`).

## Endpoint utama

| Grup | Endpoint | Akses |
| --- | --- | --- |
| Auth | `POST /auth/login`, `POST /auth/refresh` (CMS: kirim `X-Client: web`, refresh token lewat cookie) | Publik (rate-limited) |
| Auth | `POST /api/auth/logout` | JWT |
| Auth | `POST /api/auth/register` (membuat staff) | superadmin |
| Staff | `GET /api/users?status=active\|inactive`, `PATCH /api/users/:id`, `DELETE /api/users/:id` (nonaktifkan), `POST /api/users/:id/restore`, `DELETE /api/users/:id/permanent` (body `{"confirm": "<username>"}`) | superadmin |
| Dashboard | `GET /api/dashboard/summary?date_start=&date_end=` (default hari ini) | superadmin |
| Reports | `GET /api/reports/:file` | JWT |
| Payments | `POST /payments/notification` (webhook Midtrans) | Publik (rate-limited, diverifikasi `signature_key`) |
| Payments | `POST /api/payments/qris`, `POST /api/payments/va`, `GET /api/payments/:order_id/status` | JWT (503 bila Midtrans tidak dikonfigurasi) |
| Payments | `GET /api/payments?status=&method=&date_start=&date_end=` (monitoring) | superadmin |
| Products | `GET /api/products`, `GET /api/products/:id` | JWT |
| Products | `PATCH /api/products/:id` (staff: tanpa harga beli, tipe produk, dan kategori baru) | JWT |
| Products | `POST /api/products`, `POST /api/products/bulk`, `PATCH /api/products/stock`, `DELETE /api/products/:id` | superadmin |
| Categories | `GET /api/categories` (dengan `product_count`) | JWT |
| Categories | `POST /api/categories`, `PATCH /api/categories/:id`, `DELETE /api/categories/:id` | superadmin |
| Transactions | `POST /api/transactions`, `POST /api/transactions/sync`, `GET /api/transactions` (`open_conflict=true` untuk konflik stok), `GET /api/transactions/:id`, `GET /api/transactions/report/transaction` | JWT |
| Transactions | `DELETE /api/transactions/:id` (body `{"reason"}`; staff: transaksi sendiri hari ini) | JWT |
| Transactions | `GET /api/transactions/report/month`, `PATCH /api/transactions/:id/conflict-review` | superadmin |
| Customers | `POST /api/customers`, `GET /api/customers`, `GET /api/customers/:id`, `GET /api/customers/:id/transactions` | JWT |
| Customers | `GET /api/customers?status=inactive`, `PUT /api/customers/:id`, `DELETE /api/customers/:id` (nonaktifkan), `POST /api/customers/:id/restore`, `DELETE /api/customers/:id/permanent` | superadmin |
| Debts | `POST /api/debts/pay`, `GET /api/debts`, `GET /api/debts/:id` | JWT |
| Debts | `POST /api/debts` (hutang manual), `GET /api/debts/report` | superadmin |
| FCM | `POST /api/fcm/register`, `POST /api/fcm/logout` | JWT |
| FCM | `GET /api/fcm/devices`, `DELETE /api/fcm/devices/:id` | superadmin |

Catatan perilaku:

- `GET /api/debts` tidak menyertakan daftar transaksi (`transaction_list` selalu `[]`) karena satu hutang terbuka bisa menampung transaksi tanpa batas. Ambil lewat `GET /api/customers/:id/transactions` memakai `customer_id` dari response hutang; filter `type_payment=1` untuk transaksi hutang saja. `GET /api/debts/:id` tetap menyertakan transaksinya.
- `PUT /api/customers/:id` adalah partial update: field yang tidak dikirim tidak diubah, string kosong mengosongkan `phone_number`/`address`, nama tidak boleh kosong, body tanpa field → 400.
- Hapus permanen hanya untuk data yang sudah dinonaktifkan. Response `mode` bernilai `deleted` (baris hilang) atau `anonymized` (baris tetap ada karena dirujuk riwayat). Pelanggan yang masih punya hutang belum lunas tidak bisa dinonaktifkan maupun dihapus.
- `POST /api/transactions/sync`: jam device yang lebih dari 5 menit di depan atau lebih dari 30 hari di belakang waktu server diganti waktu server. Hanya tunai dan hutang yang bisa dicatat offline.
- "Device online" di dashboard dihitung dari token push yang masih terpasang ke user (device sedang login).

Dokumentasi lengkap ada di Swagger UI (root `/`) setelah server berjalan.

## Stack

Go 1.26 · Fiber v3 · GORM + PostgreSQL 18 · goose · Redis 8 · Midtrans · Firebase FCM · Cobra · Viper · zap · Swagger UI (swag) · excelize · fpdf · go-playground/validator · bytedance/sonic · Docker Compose

## Menjalankan

### 1. Konfigurasi

Salin contoh config untuk tiap environment yang dipakai:

```bash
cp .config.example.yaml .config.development.yaml
```

```yaml
server:
  port: 3030
  name: novi_shop
  env: development
  host: localhost
  trusted_proxies: []      # IP/CIDR reverse proxy bila di belakang nginx
  cms_origins: []          # origin CMS web, mis. https://cms.tokoku.id; kosong = CORS * tanpa cookie
  prefork: true            # opsional, default true; Docker memaksa false
database:
  user: <user>
  pass: <password>
  port: 5432
  dbname: <nama_db>
  host: localhost          # Docker menimpanya jadi "postgres"
  sslmode: disable         # wajib verify-full bila DB di host lain
  time_zone: Asia/Jakarta
redis:
  db: 0
  username: default
  password: <password>
  port: 6379
  host: localhost          # Docker menimpanya jadi "redis"
jwt:
  secret: <min 32 byte acak>
  token_ttl: 3600          # detik
  refresh_token_ttl: 604800
midtrans:
  server_key:              # kosong = route pembayaran online membalas 503
  environment: sandbox     # sandbox | production
firebase:
  google_application_credentials: <file-kredensial>.json
```

Aturan secret:

- Buat secret **berbeda per environment**; jangan menyalin nilai development ke production.
- `jwt.secret` minimal 256 bit acak untuk HS256 (RFC 7518 §3.2), misalnya `openssl rand -base64 48`. Jangan memakai UUID.
- File `.config.*.yaml` (kecuali `.config.example.yaml`), kredensial Firebase `*.json`, dan `.docker/` sudah di-gitignore. Jangan pernah menulis nilai asli di README atau file yang di-commit.

### 2a. Dengan Docker (disarankan)

Prasyarat: Docker + Compose v2, dan [swag](https://github.com/swaggo/swag) di host (`go install github.com/swaggo/swag/cmd/swag@v1.16.4`) karena Swagger di-generate sebelum build.

```bash
make docker-up                        # development: build image, jalankan semua service
make docker-up APP_ENV=production     # pakai .config.production.yaml
make docker-create-admin              # buat superadmin (username & password ditanya, tanpa echo)
make docker-rebuild                   # build ulang Go + jalankan migrasi; postgres/redis tidak di-restart
make docker-config                    # tampilkan compose final (tanpa nilai secret)
make docker-down                      # hentikan semua service (data Postgres tetap di volume)
```

`APP_ENV` harus sama antara `up`, `rebuild`, dan `down`. Target menolak `APP_ENV` yang tidak valid atau file config yang tidak ada sebelum container dibuat.

Password Postgres dari secret hanya dipakai saat volume pertama kali diinisialisasi. Bila mengganti `database.pass` setelahnya, ubah juga di database (`ALTER USER ... PASSWORD ...`).

### 2b. Tanpa Docker

Prasyarat: Go 1.26+, PostgreSQL, Redis.

```bash
make migrate-dev        # APP_ENV=development go run main.go migrate
make migrate-reset-dev  # drop semua tabel & migrasi ulang (ditolak di production tanpa --force)
APP_ENV=development go run main.go create-admin --username <user> --password '<password>'
make serve-dev          # APP_ENV=development go run main.go serve
make serve              # APP_ENV=production
```

Server berjalan di port `server.port`; Swagger UI di root `/`.

### Perintah Makefile lain

```bash
make build / build-dev  # build binary
make swagger            # generate swagger.json dari anotasi (butuh swag)
make test               # APP_ENV=development go test -v ./...
make fmt / tidy / clean
```

### Integration test

Test yang butuh Postgres/Redis sungguhan dilewati kecuali env berikut diisi:

| Env | Isi | Dipakai oleh |
| --- | --- | --- |
| `TEST_DATABASE_DSN` | Postgres yang sudah dimigrasi | `internal/repository` |
| `TEST_REDIS_ADDR` | Redis, mis. `127.0.0.1:6390` | test pencabutan sesi |
| `TEST_MIGRATION_DSN` | Database **kosong sekali pakai** (test menjalankan up, down sampai 0, lalu up) | `infrastructure/database` |

Jangan arahkan env ini ke database development atau production: test menulis data, dan test migrasi menghapus seluruh skema.

## Migrasi database

Migrasi baru ditambahkan sebagai file berikutnya di [infrastructure/database/migrations/](infrastructure/database/migrations/) (saat ini terakhir `00007`, jadi berikutnya `00008_<nama>.sql`, format goose `-- +goose Up` / `-- +goose Down`). Jangan mengubah file yang sudah dijalankan; skema tidak memakai AutoMigrate. Di Docker, `make docker-rebuild` langsung menerapkan migrasi baru.

Index pada tabel yang sedang dipakai dibuat dengan `CREATE INDEX CONCURRENTLY` di file yang diawali `-- +goose NO TRANSACTION` (contoh: `00007_list_indexes.sql`), supaya penulisan tidak terkunci selama index dibangun.

## Catatan teknis

- **Fiber v3**: handler menerima `fiber.Ctx` sebagai value type, dan paket contrib memakai module path `/v3` — sebagian besar contoh online masih memakai idiom v2.
- **Prefork & Docker**: master prefork Fiber tidak meneruskan SIGTERM ke child, sehingga `docker stop` berakhir SIGKILL. Karena itu prefork dimatikan di container; satu proses Go tetap memakai semua core.
- **TimeZone di DSN** sengaja tanpa tanda kutip: `gorm.io/driver/postgres` membaca nilainya mentah lewat regex, sehingga tanda kutip ikut terkirim dan ditolak Postgres.
- **Image Alpine** di-build dengan `-tags timetzdata` karena Alpine tidak membawa database zona waktu.
- **Cookie refresh CMS** memakai `SameSite=Strict`, jadi CMS dan API harus berada di site yang sama (mis. `cms.tokoku.id` dan `api.tokoku.id`). Isi `server.cms_origins` agar CORS mengizinkan credentials.
- **Stok tidak pernah minus**: constraint `chk_products_stock_non_negative` tetap berlaku, termasuk untuk transaksi offline yang oversell (stok berhenti di 0 dan ditandai konflik).
- **Pencabutan sesi** memindai key `session:*`/`refresh:*` di Redis. Biayanya sebanding jumlah sesi aktif, tapi hanya terjadi saat admin menonaktifkan staff atau mengganti password.
