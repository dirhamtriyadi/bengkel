# BengkelOS

Monorepo operasional bengkel multi-cabang (single tenant) dengan backend Go di root dan frontend Next.js di `frontend/`. Sistem mencakup penerimaan motor, inspeksi dan pengerjaan montir, pemakaian stok, penjualan retail/jasa, pembayaran tunai atau Midtrans, bukti transaksi, CMS landing page, RBAC + branch-scoped ABAC, audit log, dan akuntansi double-entry.

## Stack

- Backend: Go, Gin, GORM, PostgreSQL, Goose, go-playground/validator, JWT
- API docs: Swag + gin-swagger di `/swagger/index.html`
- Frontend: Next.js App Router, TypeScript, Tailwind CSS, komponen bergaya shadcn/ui
- Form/data: Zod, React Hook Form, TanStack Query, TanStack Table
- Infrastructure: Docker Compose, Makefile, Jenkins declarative pipeline

## Menjalankan dengan Docker

```bash
cp .env.example .env
cp frontend/.env.example frontend/.env.local
docker compose up --build
```

URL lokal:

- Landing page dan dashboard: `http://localhost:3000`
- API: `http://localhost:8080/api/v1`
- Swagger UI: `http://localhost:8080/swagger/index.html`
- Health check: `http://localhost:8080/health`

Seeder dijalankan oleh service `seed` dan aman diulang. Akun demo memakai kata sandi `Bengkel123!`:

| Peran | Email |
|---|---|
| Owner | `owner@bengkel.local` |
| Manager | `manager@bengkel.local` |
| Kasir | `kasir@bengkel.local` |
| Montir | `montir@bengkel.local` |

## Menjalankan tanpa Docker

PostgreSQL harus aktif, kemudian:

```bash
make setup
make dev
make frontend-dev
```

Perintah penting:

```bash
make migrate
make migrate-down
make seed
make docs
make test
make lint
make build
```

`database/seeders.DatabaseSeeder` menjadi orkestrator seperti Laravel. Child seeder dijalankan berurutan: cabang, role/permission, user, chart of accounts, produk, saldo awal, pelanggan/kendaraan, CMS, lalu settings. UUID deterministik dan upsert membuat `make seed` idempotent.

## Arsitektur dan aturan bisnis

```text
Kasir menerima motor
  → Customer + Vehicle
  → Work Order (inspection → approval → in_progress)
  → Part + Service dicatat sebagai WorkOrderItem
  → Checkout menghasilkan Sale + Payment
  → stok part berkurang dengan row lock
  → pembayaran sukses mem-posting jurnal seimbang
  → receipt thermal/A4 dapat dicetak
```

Penjualan barang langsung masuk melalui POS tanpa work order. Semua harga disimpan sebagai integer rupiah untuk menghindari masalah floating point. Kuantitas memakai `numeric(15,3)`.

Transaksi checkout mengunci `inventory_balances` dengan `SELECT ... FOR UPDATE`, menolak stok negatif, membuat inventory movement, sale, payment, dan jurnal dalam transaksi database. Jurnal yang sudah `posted` serta audit log dibuat immutable oleh trigger database. Constraint deferred memastikan total debit dan kredit seimbang saat commit.

Chart of accounts awal:

- `1101` Kas dan Bank
- `1201` Persediaan Suku Cadang
- `1301` Piutang Usaha
- `2101` Utang Usaha
- `3101` Modal Pemilik
- `4101` Pendapatan Penjualan dan Jasa
- `5101` Harga Pokok Penjualan
- `5201` Beban Operasional
- `5202` Beban Payment Gateway

## Kontrak API konsisten

Sukses:

```json
{
  "data": { "id": "..." },
  "meta": { "request_id": "..." }
}
```

Daftar:

```json
{
  "data": [],
  "meta": {
    "request_id": "...",
    "page": 1,
    "per_page": 10,
    "total": 120,
    "last_page": 12
  }
}
```

Error validasi:

```json
{
  "meta": { "request_id": "..." },
  "error": {
    "code": "VALIDATION_FAILED",
    "message": "Validasi gagal",
    "details": [
      { "field": "email", "rule": "email", "message": "Format email tidak valid" }
    ]
  }
}
```

Frontend hanya menangani satu `ApiEnvelope<T>` dan `ApiException`. Access/refresh token disimpan pada cookie HttpOnly oleh route BFF Next.js, bukan di local storage. Proxy otomatis merotasi refresh token saat access token kedaluwarsa.

Query datatable konsisten:

```text
?page=1&per_page=25&search=oli&sort=created_at&direction=desc
```

Semua tabel master/transaksi memakai komponen yang sama dengan pencarian server-side, sorting, pagination, pilihan jumlah baris, visibilitas kolom, refresh, loading/empty/error state, dan ekspor CSV.

## Otorisasi

- RBAC: `owner`, `manager`, `cashier`, `mechanic`; permission granular tersimpan di database.
- ABAC: JWT membawa cabang aktif, setiap query bisnis wajib branch-scoped. Header `X-Branch-ID` hanya dapat mengganti scope bagi owner.
- Audit: create/update/delete/checkout/stock adjustment/login disimpan beserta user, cabang, IP, user-agent, request ID, dan snapshot.

Struktur ini tetap single tenant. Jika nanti menjadi SaaS, tambahkan `tenant_id` sebagai policy boundary dan PostgreSQL Row-Level Security tanpa mengubah konsep `branch_id`.

## Midtrans

Isi `MIDTRANS_SERVER_KEY`, `MIDTRANS_CLIENT_KEY`, dan `MIDTRANS_IS_PRODUCTION`. Notification URL:

```text
POST https://api.example.com/api/v1/payments/midtrans/notification
```

Webhook memverifikasi SHA-512 signature, idempotent terhadap status yang sama, lalu mem-posting sale dan jurnal hanya setelah `settlement` atau `capture` yang diterima. Beban fee disimpan per pembayaran dengan `fee_bearer`: `merchant`, `customer`, atau `split`. Konfigurasi default per cabang berada di `payment.midtrans.fee_bearer`.

## Deploy Jenkins

`Jenkinsfile` menjalankan vet/test backend, memastikan Swagger hasil generate sudah committed, lint/typecheck/build frontend, build dan push dua image, lalu meminta approval sebelum production.

Siapkan Jenkins credentials:

- `bengkel-container-registry`: username/password registry
- `bengkel-production-ssh`: SSH private key
- environment `DEPLOY_USER` dan `DEPLOY_HOST`

Di server, salin isi `deployment/` ke `/opt/bengkel`, salin `.env.production.example` menjadi `.env.production`, buat external Docker network bernama `proxy`, dan pastikan reverse proxy/TLS meneruskan domain web ke `frontend:3000` serta domain API ke `api:8080`.

`deployment/deploy.sh` menarik image versi build, menjalankan migrasi sebagai one-shot service, melakukan rolling recreate container, dan menyimpan volume PostgreSQL.
