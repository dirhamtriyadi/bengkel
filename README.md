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

Setelah menarik perubahan yang menambah migration atau permission, jalankan:

```bash
docker compose up -d --build
docker compose run --rm seed
```

### Port Docker

Port publik pada host dipisahkan dari port internal container supaya deployment tidak salah arah:

| Service | Variabel port host | Port internal tetap |
|---|---|---|
| PostgreSQL | `POSTGRES_HOST_PORT` | `5432` |
| API | `API_HOST_PORT` | `8080` |
| Frontend | `FRONTEND_HOST_PORT` | `3000` |

Contoh server yang memakai port publik `5433`, `8083`, dan `3005`:

```env
POSTGRES_HOST_PORT=5433
API_HOST_PORT=8083
FRONTEND_HOST_PORT=3005
HTTP_PORT=8080
```

Komunikasi antarkontainer tidak pernah memakai port host:

```text
postgres://bengkel:PASSWORD@postgres:5432/bengkel
http://api:8080/api/v1
```

Jangan mengubah `HTTP_PORT` mengikuti port publik API. Compose mengunci proses Go di port `8080`; deployment production juga berhenti dengan pesan yang jelas jika `HTTP_PORT` atau port PostgreSQL internal keliru.

Kemudian logout dan login kembali agar JWT memuat permission terbaru.

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

Alur servis dapat dijalankan dari dashboard:

1. Buka **Operasional → Penerimaan & servis**, lalu pilih **Terima motor**.
2. Gunakan motor terdaftar atau isi pelanggan dan identitas motor baru.
3. Buka work order, pilih montir, isi diagnosis, lalu selesaikan tahap pemeriksaan.
4. Tambahkan barang yang diambil montir dan jasa yang dikerjakan. Stok barang langsung berkurang; menghapus item mengembalikan stok.
5. Jalankan status `inspection → approval → in_progress → completed`.
6. Pilih **Bayar work order**, lalu selesaikan pembayaran cash atau Midtrans.
7. Setelah pembayaran berhasil, sale, payment, jurnal, HPP, dan bukti transaksi tersedia. Receipt dapat dicetak sebagai thermal 80 mm atau A4.

Penjualan retail tetap dapat dilakukan langsung melalui **Kasir / POS** tanpa work order.

Transaksi checkout mengunci `inventory_balances` dengan `SELECT ... FOR UPDATE`, menolak stok negatif, membuat inventory movement, sale, payment, dan jurnal dalam transaksi database. Untuk work order, barang dikurangi saat montir mengambil item dan tidak dikurangi ulang ketika checkout. Jurnal yang sudah `posted` beserta barisnya dan audit log dibuat immutable oleh trigger database. Constraint deferred memastikan total debit dan kredit seimbang saat commit.

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
- Pengelolaan: owner dapat CRUD pengguna, cabang, role, dan mapping permission dari dashboard.
- Enforcement: permission dibawa dalam access token dan diperiksa per endpoint; navigasi frontend memakai permission yang sama dari `/auth/me`.
- ABAC: JWT membawa cabang aktif, setiap query bisnis wajib branch-scoped. Header `X-Branch-ID` hanya dapat mengganti scope bagi owner.
- Audit: create/update/delete/checkout/stock adjustment/login disimpan beserta user, cabang, IP, user-agent, request ID, dan snapshot.

Struktur ini tetap single tenant. Jika nanti menjadi SaaS, tambahkan `tenant_id` sebagai policy boundary dan PostgreSQL Row-Level Security tanpa mengubah konsep `branch_id`.

## Modul akuntansi

- COA per cabang dengan klasifikasi aset, liabilitas, ekuitas, pendapatan, dan beban.
- Jurnal manual melalui lifecycle `draft → posted`; jurnal posted dan barisnya immutable pada level database.
- Koreksi jurnal posted memakai reversal entry, bukan edit atau delete histori.
- Seeder membuat `OPENING-001` yang seimbang agar jurnal, buku besar, neraca saldo, dan neraca langsung memiliki saldo awal.
- Payment register untuk cash/Midtrans, status provider, fee, penanggung fee, dan referensi settlement.
- Laporan laba rugi, neraca saldo, posisi keuangan, arus kas, buku besar, penjualan, dan nilai persediaan.

## Midtrans

Backend memakai SDK resmi [`github.com/midtrans/midtrans-go`](https://github.com/Midtrans/midtrans-go) untuk membuat transaksi Snap dan memverifikasi status transaksi. Isi `MIDTRANS_SERVER_KEY`, `MIDTRANS_CLIENT_KEY`, dan `MIDTRANS_IS_PRODUCTION`. Untuk sandbox gunakan `MIDTRANS_IS_PRODUCTION=false` dan pastikan kedua key sama-sama berawalan `SB-Mid-`.

Notification URL:

```text
POST https://api.example.com/api/v1/payments/midtrans/notification
```

Webhook memverifikasi SHA-512 signature dan mengecek ulang status serta nominal transaksi ke API Midtrans. Prosesnya idempotent terhadap status yang sama, lalu mem-posting sale dan jurnal hanya setelah `settlement` atau `capture` yang diterima.

Biaya pelanggan memakai fitur **Automatic Fee Imposition** Midtrans, sehingga backend mengirim tagihan asli dan Midtrans menghitung gross-up berdasarkan channel yang benar-benar dipilih. Nilai aktual `original_amount`, `gross_amount`, dan `customer_imposed_payment_fee` direkonsiliasi saat callback/sync. Konfigurasi cabang berada di `payment.midtrans.channels` dan dapat mengatur:

- channel yang ditampilkan di Snap;
- porsi fee pelanggan per channel dari 0–100%;
- acquirer yang diwajibkan provider, misalnya `gopay` untuk QRIS;
- referensi MDR, biaya tetap, dan pajak untuk rekonsiliasi beban merchant.

Perubahan setting berlaku pada token Snap baru. Snapshot konfigurasi disimpan pada pembayaran agar transaksi yang sudah dimulai tetap dapat diaudit walaupun setting kemudian berubah. Seeder tidak menimpa setting yang telah disesuaikan operator.

## Deploy Jenkins

`Jenkinsfile` menjalankan vet/test backend, memastikan Swagger hasil generate sudah committed, lint/typecheck/build frontend, build dan push dua image, lalu meminta approval sebelum production.

Siapkan Jenkins credentials:

- `bengkel-container-registry`: username/password registry
- `bengkel-production-ssh`: SSH private key
- environment `DEPLOY_USER` dan `DEPLOY_HOST`

Di server, salin isi `deployment/` ke `/opt/bengkel`, salin `.env.production.example` menjadi `.env.production`, buat external Docker network bernama `proxy`, dan pastikan reverse proxy/TLS meneruskan domain web ke `frontend:3000` serta domain API ke `api:8080`.

`deployment/deploy.sh` menarik image versi build, menjalankan migrasi sebagai one-shot service, melakukan rolling recreate container, dan menyimpan volume PostgreSQL.
