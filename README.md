# Shop POS Backend — Go Fiber v3

Point-of-sale (POS) backend for small shops, built with **Go Fiber v3** and Clean Architecture. It handles products (physical and digital), categories, customers, staff, sales transactions (including offline queues from the cashier app), debts (kasbon), online payments through Midtrans (QRIS and Virtual Account), push notifications (FCM), a shop dashboard, and authentication with Redis-backed sessions. Sales and debt reports are generated as PDF; products can be imported from CSV/Excel.

The API serves two clients: the **CMS web app** for the owner (superadmin) and the **Android/iOS cashier app** for staff.

## Purpose

Provide a lightweight but secure cashier API for small shops: stock tracking, atomic sales transactions (stock and debt change in a single DB transaction), customer debt tracking, online payments, and monthly reports. Access is split between **cashier/staff** and **superadmin** through JWT + role.

## Features

- **Domain modules**: products, categories, customers, transactions, debts (kasbon), payments, dashboard, FCM/devices, users/sessions
- **Physical and digital products**: `ProductType` separates physical products (stock-managed) from digital ones (airtime/e-wallet/data packages — no stock, need a target number at sale time). Digital products are skipped when stock is deducted or restored
- **Atomic sales transactions**: stock and debt change in one database transaction. Products are locked with `FOR UPDATE` in ID order (deadlock-free); concurrent conflicts (unique violation, serialization, deadlock) are retried automatically with backoff (see [internal/repository/retry.go](internal/repository/retry.go))
- **Pricing rules**: the selling price must be above the purchase price and the debt price at least 105% of the selling price; checked in the usecase and enforced by the `chk_products_price_rules` constraint. Staff may edit products, but purchase price, product type, stock, and new categories are superadmin-only
- **Product barcode**: optional, one per product, unique among active products (letters, digits, `-._`, max 64 characters, no checksum validation because shops also use self-made barcodes). In bulk import, an empty barcode cell keeps the existing barcode, and a conflicting barcode only fails that row
- **Categories as a table**: names are unique case-insensitively and ignoring edge whitespace (`key = lower(trim(name))`); a category still used by products cannot be deleted
- **Offline transactions (cashier)**: `POST /api/v1/sales/transactions/sync` accepts up to 100 cash/debt transactions created without internet. Each item carries a `client_ref` (UUID) so a resent queue is never recorded twice, uses the price the buyer actually paid, and succeeds or fails independently (`retryable` tells the client whether to resend). If server stock is short, the transaction is still accepted, stock stops at 0, and the transaction is flagged `stock_conflict` for admin review
- **Transaction cancellation** requires a reason; staff may only cancel their own transactions on the same day
- **Debts**: one open debt per customer (guarded by a partial unique index), installment payments with an `idempotency_key` so a retried request is not recorded twice, manual debts recorded in `debt_adjustments` for reports
- **Staff and customer management**: deactivate (soft delete), restore, and permanently delete. Accounts still referenced by sales/debt history are anonymized instead of deleted, so old reports stay intact. Deactivating staff immediately revokes all their sessions and push tokens
- **Dashboard**: revenue, gross profit, debt (balance, new, paid), daily chart, top 5 products, payment method breakdown, devices currently logged in, and the number of stock conflicts for a date range (max 366 days)
- **Online payments (Midtrans)**: **QRIS** and **Virtual Account** charges (BCA `bank_transfer`, Mandiri `echannel`); item prices are computed server-side and the admin fee is added automatically (QRIS 0.7%, VA flat). Status is updated through a webhook verified by `signature_key` (SHA-512, constant-time comparison). Digital products are **POS-only** and rejected for online charges
- **Push notifications (FCM)**: device token registration and logout through Firebase Cloud Messaging; the superadmin can view and revoke devices
- **Authentication and authorization**: JWT access + refresh tokens (HS256), sessions in Redis, role-based access (`superadmin` vs `staff`); passwords hashed with bcrypt
- **CMS web auth**: requests with the `X-Client: web` header receive the refresh token as an `HttpOnly; SameSite=Strict` cookie instead of in the body, so JavaScript cannot read it. The header also acts as a CSRF guard. Staff login from the CMS is rejected (403)
- **Privileged accounts via CLI**: `superadmin` is created with the `create-admin` command, not a public endpoint
- **Keyset pagination** (`after_time` + `after_id`) on all list endpoints, backed by a `(created_at, id)` index
- **Reports**: CSV/Excel product import (max 10,000 rows, per-row results returned) and PDF for receipts, monthly reports, and debt summaries; PDFs are downloaded through `GET /api/v1/insight/reports/:file` (JWT required, random file name, expires after 1 hour)
- **Built-in security**: rate limiting (Redis storage) per IP for public routes and per user for `/api/{version}/*`, with separate buckets for login, refresh, and webhook; Helmet/XSS headers, CORS (allowlist of CMS origins when `server.cms_origins` is set), trusted proxy for real client IP detection
- **Versioned migrations with goose**: SQL files embedded in the binary, run through the `migrate` / `migrate-reset` commands
- **Per-environment configuration** via Viper (`.config.<env>.yaml`), env var overrides, and secrets from files (`<KEY>_FILE`)
- **Docker Compose** for Postgres, Redis, migrations, and the app; secrets are passed as files, not container env vars
- **Centralized logging (Loki + Grafana)**: the app writes one JSON line per request to stdout (`request_id`, `method`, `path` without query string, `route`, `status`, `latency_ms`, `ip`); Grafana Alloy reads it from Docker and ships it to Loki, viewed through Grafana at `127.0.0.1:3000`. Postgres and Redis logs are collected too. See [Logging (Loki + Grafana)](#logging-loki--grafana)
- **Swagger UI** at `/{version}` (e.g. `/v1`, `/` redirects there), generated from handler annotations

## Architecture

The project uses **Clean Architecture** on top of Fiber v3. Code is split into layers, and dependencies may only point inward: outer layers (HTTP, database, external services) depend on `domain`, never the other way around. `domain` defines entities and interfaces; the other layers implement or use them.

### Layers and dependency direction

```
                    ┌──────────────────────────────────────────────────┐
   REST/JSON        │               Fiber v3 app (:3030)               │
  ───────────────►  │ helmet, CORS, global rate limit, Swagger UI (/v1)│
                    └────────────────────────┬─────────────────────────┘
                                             ▼
                    ┌──────────────────────────────────────────────────┐
                    │ delivery/http                                    │
                    │ route → middleware (JWT, role, limit) → handler  │
                    │ bind + DTO validation, typed errors → HTTP status│
                    └────────────────────────┬─────────────────────────┘
                                             │ domain.XxxUsecase
                                             ▼
                    ┌──────────────────────────────────────────────────┐
                    │ usecase                                          │
                    │ business rules: DB prices, admin fee, stock      │
                    └──────┬────────────────────┬─────────────────┬────┘
            XxxRepository  │     PaymentGateway │     *fcm.Sender │
                           ▼                    ▼                 ▼
              ┌──────────────────────┐ ┌────────────────┐ ┌──────────────┐
              │      repository      │ │  api/payment   │ │     fcm      │
              │ GORM, tx + retry,    │ │ QRIS/VA charge │ │ push notif   │
              │ lock, keyset paging  │ │ signature check│ │              │
              └──────┬────────┬──────┘ └───────┬────────┘ └──────┬───────┘
                     ▼        ▼                ▼                 ▼
              ┌──────────┐ ┌───────┐   ┌──────────────┐   ┌──────────────┐
              │ Postgres │ │ Redis │   │   Midtrans   │   │ Firebase FCM │
              │    18    │ │   8   │   │     API      │   │              │
              └──────────┘ └───▲───┘   └──────┬───────┘   └──────────────┘
                               │              │ webhook
          JWT sessions + rate limit ┘         └──► POST /api/v1/sales/payments/notification

        ┌──────────────────────────────────────────────────────────────────┐
        │ domain — the core, depends on none of the layers above           │
        │ entities · Usecase/Repository/PaymentGateway interfaces · errors │
        │ every layer above implements or uses the interfaces defined here│
        └──────────────────────────────────────────────────────────────────┘
```

| Layer | Package | Responsibility | May depend on |
| --- | --- | --- | --- |
| Domain | `internal/domain` | Entities, repository/usecase/gateway interfaces, typed errors | `internal/constant`, `internal/dto` |
| Application | `internal/usecase` | Business rules: cross-entity validation, price and admin fee calculation, response assembly | `domain`, `pkg/*` |
| Delivery | `internal/delivery/http` | Parse and validate requests, call usecases, map errors → HTTP status | `domain` (usecase interfaces), `pkg/*` |
| Infrastructure | `internal/repository`, `infrastructure/*` | DB queries, transactions + retry, Redis, Midtrans, FCM | `domain` |
| Composition root | `cmd/` | Creates all dependencies and wires them together | everything |

Rules of thumb:

- Handlers never touch GORM or Redis; repositories know nothing about Fiber.
- Usecases only know interfaces (`domain.ProductRepository`, `domain.PaymentGateway`, ...), so they can be tested with fakes and no database (see `internal/usecase/*_test.go`).
- Errors are typed in the domain (`domain.NotFound(...)`, `domain.Conflict(...)`) and translated to HTTP status in a single place (`handler/common.go`).
- Database transactions (locking, retry) are the repository's concern; a usecase just calls one method such as `CreateTransaction`.

### Dependency wiring (composition root)

All objects are created manually in `cmd/fiber_command.go` (the `serve` command), without a DI framework:

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

### Folder structure

```
├── main.go                  # Entry point — calls cmd.Execute()
├── cmd/                     # Cobra commands: serve, migrate, migrate-reset, create-admin
├── config/
│   ├── env_config/          # Config loader and validation (Viper + secret files)
│   └── fiber_config/        # Fiber setup, middleware chain, Swagger, listener (prefork)
├── infrastructure/
│   ├── database/            # Postgres init (GORM) + goose migrations
│   │   └── migrations/      # Versioned SQL files (embedded in the binary)
│   ├── cache/               # Redis (one client for sessions + rate limiter storage)
│   ├── api/payment/         # Midtrans gateway (QRIS/VA charge, webhook verification)
│   ├── fcm/                 # Firebase Cloud Messaging sender
│   └── logger/              # zap logger: JSON to stdout (production and non-TTY), colored console in a dev terminal
├── internal/
│   ├── domain/              # Entities, repository/usecase interfaces, typed errors
│   ├── dto/                 # Request/response DTOs per domain
│   ├── repository/          # Data access (GORM / Redis), transactions + retry, pagination
│   ├── usecase/             # Business logic
│   ├── delivery/http/
│   │   ├── handler/         # Fiber v3 handlers + Swagger annotations
│   │   ├── middleware/      # JWT, CORS, rate limit, XSS, feature flag, logger
│   │   └── route/           # Route registration
│   └── constant/            # Enums (role, payment, product type, debt status) and cursor
├── pkg/
│   ├── jwt/                 # Token generation/verification
│   ├── response/            # Standard response format
│   ├── pdf/                 # PDF reports (receipt, debt, monthly)
│   └── sheet/               # Product import from Excel and CSV
├── .dockerfile              # Multi-stage image (Go binary + swagger.json, non-root user)
├── observability/           # Loki and Alloy config, Grafana datasource provisioning
├── docker-compose.yaml      # postgres, redis, migrate, app, loki, alloy, grafana
└── Makefile                 # Local targets and docker-*
```

### Request flow

`requestid` → `logger` → `recover`/helmet/CORS → `route` → `middleware` (rate limit, JWT + role) → `handler` (bind + DTO validation) → `usecase` → `repository` → Postgres/Redis.

- Public: `/api/v1/account/auth/login`, `/api/v1/account/auth/refresh`, and the webhook `/api/v1/sales/payments/notification` (verified by `signature_key`, not JWT).
- Every URL is `/api/{version}/{group}/{service}` with groups `account`, `catalog`, `sales`, and `insight`. The version comes from `server.api_version` (default `v1`; override with `SERVER_API_VERSION`). Requests to another version, or to old unversioned URLs, get a 404 that names the supported version. Everything else needs a JWT; sensitive actions (staff management, categories, devices, data deletion, stock, dashboard, monthly and debt reports) are `superadmin` only.

### Configuration flow

1. `APP_ENV` (`development` | `staging` | `production`) selects the `.config.<env>.yaml` file from the working directory.
2. Every key can be overridden by an env var: dots become underscores, uppercase (`database.host` → `DATABASE_HOST`).
3. Secrets can be read from a file through the env var `<KEY>_FILE` (same pattern as the Postgres image). Supported for `database.pass`, `redis.password`, `jwt.secret`, `midtrans.server_key`; for example `JWT_SECRET_FILE=/run/secrets/jwt_secret`. The value from the file wins over yaml and env.
4. Config is validated at startup (required fields, `jwt.secret` ≥ 32 characters, TTL > 0); a validation failure means the app does not start.

## Infrastructure (Docker)

```
                     .config.<env>.yaml
                             │ make docker-env
                             ▼
          ┌─────────────────────────────────────────────┐
          │ .docker/<env>/   mode 700, gitignored       │
          │ app.yaml · compose.env · 1 file per secret  │
          └──────┬────────────────────┬─────────────┬───┘
                 │ app.yaml           │ /run/secrets│ redis.conf
                 │ + /run/secrets     │             │
                 ▼                    ▼             ▼
   ┌──────────────────────┐  ┌────────────────┐  ┌─────────────┐
   │ migrate (one-shot)   │─►│  postgres:18   │  │   redis:8   │
   │ app :3030            │─►│ 127.0.0.1:5432 │  │ internal    │
   │ prefork off          │  │ volume pg-data │  │ only        │
   └──────────┬───────────┘  └────────────────┘  └──────▲──────┘
              └─────────────────────────────────────────┘
```

| Service | Role | Notes |
| --- | --- | --- |
| `postgres` | Database | Port only exposed to `127.0.0.1:5432`; data in the `postgres-data` volume |
| `redis` | Sessions + rate limiter | Not exposed to the host; password from `redis.conf` |
| `migrate` | Runs goose migrations then exits | `app` only starts after `migrate` succeeds |
| `app` | API server | Prefork disabled (`SERVER_PREFORK=false`) so `docker stop` is graceful; `stop_grace_period` 15s |
| `loki` | Log storage | Not exposed to the host; data in the `loki-data` volume, 14-day retention |
| `alloy` | Log shipper | Reads logs of containers labelled `logging=true` through `docker.sock` (read-only) |
| `grafana` | Log dashboard | Only `127.0.0.1:${GRAFANA_PORT:-3000}`; Loki datasource is provisioned |

`make docker-env` (run automatically by the `docker-*` targets) reads `.config.<env>.yaml` and generates `.docker/<env>/` (mode 700 folder, gitignored):

- `app.yaml` — the same yaml with secret values blanked; mounted into the app as `.config.<env>.yaml`
- `compose.env` — non-secret keys for `${...}` interpolation in compose (not attached to containers)
- `database_pass`, `redis_password`, `jwt_secret`, `midtrans_server_key`, `grafana_admin_password`, `redis.conf` — attached as compose secrets

As a result `docker inspect` only shows paths such as `JWT_SECRET_FILE=/run/secrets/jwt_secret`, not the secret values. Firebase credentials are also attached as a secret (`/run/secrets/firebase`).

### Logging (Loki + Grafana)

```text
app / postgres / redis ──stdout──► Docker ──► Alloy ──► Loki ──► Grafana
 (label logging=true)                      (docker.sock)  (disk, 14 days)  (127.0.0.1:3000)
```

The app does not know Loki exists; it only writes JSON to stdout, so a Loki or Grafana outage never affects requests.

- **Log format**: production is always JSON. Development uses a colored console only when stderr is a terminal (`make serve-dev`); in a container the format is JSON with the same keys (`level`, `ts`, `msg`). zap sampling is off so the number of `HTTP Request` lines equals the number of requests.
- **Request log**: one `HTTP Request` line per request, written by the outermost middleware so 429s from the rate limiter and panics recovered by `recover` still appear with their real status. `request_id` matches the `X-Request-ID` response header and the field in error logs.
- **Loki labels**: only `env`, `service`, and `level` (low cardinality). Other fields are read at query time with `| json`.
- **Retention**: 14 days (`retention_period: 336h` in [observability/loki.yaml](observability/loki.yaml)); disk usage grows linearly with it.
- **Access**: open `http://127.0.0.1:3000` and log in as `admin` with the password from `grafana.admin_password`. For a remote server use an SSH tunnel or a reverse proxy; the port is not opened to the network. The password is only applied the first time Grafana starts.

Example LogQL queries:

```text
{service="app", level="error"}                      # errors from the app
{env="development"} | detected_level="error"        # errors from all services, including Postgres
{service="app"} | json | status >= 500              # failed requests
{service="app"} | json | latency_ms > 500           # slow requests
{service="app"} | json | request_id="<id>"          # a single request
```

Notes:

- The `level` label only exists for JSON logs from the app. Postgres and Redis lines (not JSON) show up as `unknown` in Drilldown; use `detected_level` for errors across services.
- Only `HTTP Request` and app logs are structured. The route table Fiber prints at startup (development only) is also shipped as plain text.
- Logs of the `migrate` container are not shipped because it exits before Alloy discovers it; read them with `docker compose logs migrate`.
- Alloy mounts `docker.sock` (read-only). That grants read access to every container, so do not run Alloy on a shared host.

## Main endpoints

Every path below is shown with `v1`; the real prefix is `/api/{server.api_version}`. The frontend is built with the same version (`API_VERSION` in its `.env`), so raise both together.

| Group | Endpoint | Access |
| --- | --- | --- |
| Auth | `POST /api/v1/account/auth/login`, `POST /api/v1/account/auth/refresh` (CMS: send `X-Client: web`, refresh token via cookie) | Public (rate-limited) |
| Auth | `POST /api/v1/account/auth/logout` | JWT |
| Auth | `POST /api/v1/account/auth/register` (creates staff) | superadmin |
| Staff | `GET /api/v1/account/users?status=active\|inactive`, `PATCH /api/v1/account/users/:id`, `DELETE /api/v1/account/users/:id` (deactivate), `POST /api/v1/account/users/:id/restore`, `DELETE /api/v1/account/users/:id/permanent` (body `{"confirm": "<username>"}`) | superadmin |
| Dashboard | `GET /api/v1/insight/dashboard/summary?date_start=&date_end=` (defaults to today) | superadmin |
| Reports | `GET /api/v1/insight/reports/:file` | JWT |
| Payments | `POST /api/v1/sales/payments/notification` (Midtrans webhook) | Public (rate-limited, verified by `signature_key`) |
| Payments | `POST /api/v1/sales/payments/qris`, `POST /api/v1/sales/payments/va`, `GET /api/v1/sales/payments/:order_id/status` | JWT (503 if Midtrans is not configured) |
| Payments | `GET /api/v1/sales/payments?status=&method=&date_start=&date_end=` (monitoring) | superadmin |
| Products | `GET /api/v1/catalog/products` (`barcode=` for scan results, exact match), `GET /api/v1/catalog/products/:id` | JWT |
| Products | `PATCH /api/v1/catalog/products/:id` (staff: no purchase price, product type, or new categories) | JWT |
| Products | `POST /api/v1/catalog/products`, `POST /api/v1/catalog/products/bulk`, `PATCH /api/v1/catalog/products/stock`, `DELETE /api/v1/catalog/products/:id` | superadmin |
| Categories | `GET /api/v1/catalog/categories` (with `product_count`) | JWT |
| Categories | `POST /api/v1/catalog/categories`, `PATCH /api/v1/catalog/categories/:id`, `DELETE /api/v1/catalog/categories/:id` | superadmin |
| Transactions | `POST /api/v1/sales/transactions`, `POST /api/v1/sales/transactions/sync`, `GET /api/v1/sales/transactions` (`open_conflict=true` for stock conflicts), `GET /api/v1/sales/transactions/:id`, `GET /api/v1/sales/transactions/report/transaction` | JWT |
| Transactions | `DELETE /api/v1/sales/transactions/:id` (body `{"reason"}`; staff: own transactions today) | JWT |
| Transactions | `GET /api/v1/sales/transactions/report/month`, `PATCH /api/v1/sales/transactions/:id/conflict-review` | superadmin |
| Customers | `POST /api/v1/sales/customers`, `GET /api/v1/sales/customers`, `GET /api/v1/sales/customers/:id`, `GET /api/v1/sales/customers/:id/transactions` | JWT |
| Customers | `GET /api/v1/sales/customers?status=inactive`, `PUT /api/v1/sales/customers/:id`, `DELETE /api/v1/sales/customers/:id` (deactivate), `POST /api/v1/sales/customers/:id/restore`, `DELETE /api/v1/sales/customers/:id/permanent` | superadmin |
| Debts | `POST /api/v1/sales/debts/pay`, `GET /api/v1/sales/debts`, `GET /api/v1/sales/debts/:id` | JWT |
| Debts | `POST /api/v1/sales/debts` (manual debt), `GET /api/v1/sales/debts/report` | superadmin |
| FCM | `POST /api/v1/account/fcm/register`, `POST /api/v1/account/fcm/logout` | JWT |
| FCM | `GET /api/v1/account/fcm/devices`, `DELETE /api/v1/account/fcm/devices/:id` | superadmin |

Behavior notes:

- `GET /api/v1/sales/debts` does not include the transaction list (`transaction_list` is always `[]`) because a single open debt can hold an unbounded number of transactions. Fetch them through `GET /api/v1/sales/customers/:id/transactions` using the `customer_id` from the debt response; filter with `type_payment=1` for debt transactions only. `GET /api/v1/sales/debts/:id` still includes its transactions.
- `PUT /api/v1/sales/customers/:id` is a partial update: fields not sent are left unchanged, an empty string clears `phone_number`/`address`, the name must not be empty, and a body with no fields → 400.
- Permanent deletion only applies to data that has already been deactivated. The response `mode` is `deleted` (row removed) or `anonymized` (row kept because history references it). A customer with an unpaid debt can be neither deactivated nor deleted.
- `POST /api/v1/sales/transactions/sync`: a device clock more than 5 minutes ahead of, or more than 30 days behind, the server time is replaced by the server time. Only cash and debt can be recorded offline.
- "Devices online" on the dashboard is counted from push tokens still attached to a user (the device is logged in).

Full documentation is available in the Swagger UI (`/v1`, or whichever version is configured) once the server is running.

## Stack

Go 1.26 · Fiber v3 · GORM + PostgreSQL 18 · goose · Redis 8 · Midtrans · Firebase FCM · Cobra · Viper · zap · Swagger UI (swag) · excelize · fpdf · go-playground/validator · bytedance/sonic · Grafana Loki · Grafana Alloy · Grafana · Docker Compose

## Running

### 1. Configuration

Copy the example config for each environment you use:

```bash
cp .config.example.yaml .config.development.yaml
```

```yaml
server:
  port: 3030
  name: novi_shop
  env: development
  host: localhost
  trusted_proxies: []      # IP/CIDR of the reverse proxy when behind nginx
  cms_origins: []          # CMS web origin, e.g. https://cms.tokoku.id; empty = CORS * without cookies
  api_version: v1          # URL version, /api/{api_version}/...; format v1, v2; CI/CD sets SERVER_API_VERSION
  public_url: https://api.tokoku.id  # required when Midtrans is configured; builds the per-charge webhook URL (https in production)
  prefork: true            # optional, default true; Docker forces false
database:
  user: <user>
  pass: <password>
  port: 5432
  dbname: <db_name>
  host: localhost          # Docker overrides it to "postgres"
  sslmode: disable         # must be verify-full if the DB is on another host
  time_zone: Asia/Jakarta
redis:
  db: 0
  username: default
  password: <password>
  port: 6379
  host: localhost          # Docker overrides it to "redis"
jwt:
  secret: <min 32 random bytes>
  token_ttl: 3600          # seconds
  refresh_token_ttl: 604800
midtrans:
  server_key:              # empty = online payment routes reply 503
  environment: sandbox     # sandbox | production
firebase:
  google_application_credentials: <credentials-file>.json
grafana:                   # only used by the Docker stack (Loki + Grafana)
  port: 3000               # port on 127.0.0.1
  admin_password: <random> # required: make docker-up stops if empty
```

Secret rules:

- Use **different secrets per environment**; never copy development values to production.
- `jwt.secret` must be at least 256 random bits for HS256 (RFC 7518 §3.2), for example `openssl rand -base64 48`. Do not use a UUID.
- `.config.*.yaml` files (except `.config.example.yaml`), Firebase credential `*.json` files, and `.docker/` are already gitignored. Never write real values in the README or in committed files.

### 2a. With Docker (recommended)

Prerequisites: Docker + Compose v2, and [swag](https://github.com/swaggo/swag) on the host (`go install github.com/swaggo/swag/cmd/swag@v1.16.4`) because Swagger is generated before the build.

```bash
make docker-up                        # development: build the image, start all services
make docker-up APP_ENV=production     # use .config.production.yaml
make docker-create-admin              # create a superadmin (username and password prompted, no echo)
make docker-rebuild                   # rebuild Go + run migrations; postgres/redis are not restarted
make docker-logs                      # follow app logs (last 200 lines); history and filters via Grafana
make docker-config                    # print the final compose (without secret values)
make docker-down                      # stop all services (Postgres data stays in the volume)
```

`APP_ENV` must be the same across `up`, `rebuild`, and `down`. The targets reject an invalid `APP_ENV` or a missing config file before any container is created.

The Postgres password from the secret is only used the first time the volume is initialized. If you change `database.pass` afterwards, also change it in the database (`ALTER USER ... PASSWORD ...`).

### 2b. Without Docker

Prerequisites: Go 1.26+, PostgreSQL, Redis.

```bash
make migrate-dev        # APP_ENV=development go run main.go migrate
make migrate-reset-dev  # drop all tables and migrate again (refused in production without --force)
APP_ENV=development go run main.go create-admin --username <user> --password '<password>'
make serve-dev          # APP_ENV=development go run main.go serve
make serve              # APP_ENV=production
```

The server runs on `server.port`; Swagger UI at `/{version}` (default `/v1`).

### Other Makefile commands

```bash
make build / build-dev  # build the binary
make swagger            # generate swagger.json from annotations (needs swag)
make test               # APP_ENV=development go test -v ./...
make fmt / tidy / clean
```

### Integration tests

Tests that need a real Postgres/Redis are skipped unless the following env vars are set:

| Env | Value | Used by |
| --- | --- | --- |
| `TEST_DATABASE_DSN` | A migrated Postgres | `internal/repository` |
| `TEST_REDIS_ADDR` | Redis, e.g. `127.0.0.1:6390` | session revocation test |
| `TEST_MIGRATION_DSN` | A **disposable empty** database (the test runs up, down to 0, then up) | `infrastructure/database` |

Never point these env vars at a development or production database: the tests write data, and the migration test drops the entire schema.

## Database migrations

New migrations are added as the next file in [infrastructure/database/migrations/](infrastructure/database/migrations/) (currently the latest is `00008`, so the next is `00009_<name>.sql`, goose format `-- +goose Up` / `-- +goose Down`). Do not edit files that have already run; the schema does not use AutoMigrate. In Docker, `make docker-rebuild` applies new migrations right away.

Indexes on tables in active use are created with `CREATE INDEX CONCURRENTLY` in files starting with `-- +goose NO TRANSACTION` (example: `00007_list_indexes.sql`), so writes are not locked while the index is built.

## Technical notes

- **Fiber v3**: handlers receive `fiber.Ctx` as a value type, and contrib packages use the `/v3` module path — most examples online still use v2 idioms.
- **Prefork and Docker**: the Fiber prefork master does not forward SIGTERM to its children, so `docker stop` ends in SIGKILL. That is why prefork is disabled in containers; a single Go process still uses all cores.
- **TimeZone in the DSN** is deliberately unquoted: `gorm.io/driver/postgres` reads the value raw through a regex, so quotes would be sent along and rejected by Postgres.
- **Alpine image** is built with `-tags timetzdata` because Alpine ships no timezone database.
- **CMS refresh cookie** uses `SameSite=Strict`, so the CMS and API must be on the same site (e.g. `cms.tokoku.id` and `api.tokoku.id`). Set `server.cms_origins` so CORS allows credentials.
- **Stock never goes negative**: the `chk_products_stock_non_negative` constraint always applies, including to offline transactions that oversell (stock stops at 0 and the transaction is flagged as a conflict).
- **Stock writes** for one cart are done in a single upsert statement (`ON CONFLICT (id) DO UPDATE SET stock, updated_at`) on rows already locked with `FOR UPDATE`, instead of one `UPDATE` per cart line.
- **GORM queries**: the dashboard, daily debt flow, and pre-deletion history checks use GORM instead of raw SQL; the debt flow runs two queries and merges them per day in Go because GORM has no UNION.
- **Session revocation** scans the `session:*`/`refresh:*` keys in Redis. The cost is proportional to the number of active sessions, but it only happens when an admin deactivates staff or changes a password.
