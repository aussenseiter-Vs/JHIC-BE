---
name: API Reference
relation: README.md
description: Complete reference of all REST API endpoints, request/response shapes, and roles
type: editable
---

# API Reference

Base URL: `/api/v1`

Semua ID adalah **snowflake ID** (BIGINT, bukan UUID). Format response: success = object/array langsung, error = `{"error":"message"}`.

## Auth

| Method | Path | Auth | Deskripsi |
|---|---|---|---|
| POST | `/auth/register` | Public | Register user baru |
| POST | `/auth/login` | Public | Login dan dapatkan token |
| POST | `/auth/logout` | Bearer | Logout (invalidasi session) |

### POST /auth/register

Body:
```json
{ "email": "a@example.com", "password": "secret", "name": "A" }
```

Response `201`:
```json
{
  "user": {
    "id": 1,
    "email": "a@example.com",
    "name": "A",
    "role": "user",
    "avatar_url": "",
    "class": "",
    "jurusan": "",
    "position": "",
    "created_at": "...",
    "updated_at": "..."
  },
  "token": "..."
}
```

Error: `400` invalid body / email & password required, `422` gagal register.

### POST /auth/login

Body:
```json
{ "email": "a@example.com", "password": "secret" }
```

Response `200`:
```json
{ "user": { "...": "..." }, "token": "..." }
```

Error: `400` invalid body, `401` email/password salah.

### POST /auth/logout

Header: `Authorization: Bearer <token>`

Response `204` No Content. Error: `401` invalid/expired token.

## User

| Method | Path | Auth | Deskripsi |
|---|---|---|---|
| GET | `/users` | Public | List semua user |
| GET | `/users/{id}` | Public | Detail user |
| POST | `/users` | admin | Buat user baru |
| PUT | `/users/{id}` | admin | Update user |
| PUT | `/users/{id}/role` | admin | Ubah role user |
| DELETE | `/users/{id}` | admin | Hapus user (204) |

Role valid: `jurnal`, `guru`, `admin`, `user`. Position valid (hanya untuk role `guru`): `wali_kelas`, `bk`, `kesiswaan`, `kaprog`.

### POST /users

Body:
```json
{
  "email": "guru@example.com",
  "password": "secret",
  "name": "Bu Guru",
  "role": "guru",
  "class": "PPLG 1",
  "jurusan": "PPLG",
  "position": "wali_kelas"
}
```

Response `201`:
```json
{
  "id": 2,
  "email": "guru@example.com",
  "name": "Bu Guru",
  "role": "guru",
  "avatar_url": "",
  "class": "PPLG 1",
  "jurusan": "PPLG",
  "position": "wali_kelas",
  "created_at": "...",
  "updated_at": "..."
}
```

Error: `400` invalid body, `409` email sudah ada, `422` role/position tidak valid.

### PUT /users/{id}

Body:
```json
{
  "name": "Nama Baru",
  "avatar_url": "https://...",
  "class": "PPLG 2",
  "jurusan": "PPLG",
  "position": "kesiswaan"
}
```

Response `200` dengan user ter-update.

### PUT /users/{id}/role

Body:
```json
{ "role": "admin" }
```

Response `200`:
```json
{ "message": "role updated" }
```

Error: `422` role tidak valid.

## Berita

| Method | Path | Auth | Deskripsi |
|---|---|---|---|
| POST | `/berita` | auth | Buat berita baru |
| GET | `/berita` | Public | List semua berita |
| GET | `/berita/{id}` | Public | Detail berita |
| PUT | `/berita/{id}` | auth | Update berita (hanya author) |
| DELETE | `/berita/{id}` | auth | Hapus berita (hanya author, 204) |
| POST | `/berita/{id}/image` | auth | Upload cover image (multipart) |
| POST | `/berita/{id}/images` | auth | Upload konten image (multipart) |
| DELETE | `/berita/{id}/images?key=<key>` | auth | Hapus konten image |

`content` mendukung markdown dengan inline image refs. Image URL di-response sebagai **pre-signed URL** (berlaku 24 jam).

### POST /berita

Body:
```json
{ "title": "Judul Berita", "content": "Konten...", "is_achievement": false }
```

Response `201`:
```json
{
  "id": 3,
  "author_id": 2,
  "title": "Judul Berita",
  "content": "Konten...",
  "image_url": "",
  "is_achievement": false,
  "created_at": "...",
  "updated_at": "..."
}
```

Error: `400` title wajib / content invalid, `401` belum login.

### GET /berita

Response `200`: array berita.

### PUT /berita/{id}

Body:
```json
{ "title": "Judul Baru", "content": "Konten baru...", "is_achievement": true }
```

Response `200` dengan berita ter-update. Error: `400` content invalid, `403` bukan author, `404` tidak ditemukan.

### POST /berita/{id}/image (cover)

Multipart form, field `image`. Format: jpeg, png, gif, webp. Maks 5MB.

Response `200`:
```json
{ "image_url": "https://...pre-signed..." }
```

### POST /berita/{id}/images (konten)

Multipart form, field `image`. Sama seperti cover, tapi hanya author. Object path: `berita/{id}/content/<hex>.<ext>`.

Response `200`:
```json
{ "image_url": "berita/3/content/abc123.png" }
```

### DELETE /berita/{id}/images

Query param `key` (object path konten image, harus di dalam `berita/{id}/content/`). Hanya author.

Response `204` No Content.

## PKL Approval

| Method | Path | Role | Deskripsi |
|---|---|---|---|
| POST | `/approval/pkl` | user | Buat request PKL |
| GET | `/approval/pkl` | user/guru/admin | List (guru/admin lihat semua, user lihat miliknya) |
| GET | `/approval/pkl/{id}` | user/guru/admin | Detail |
| POST | `/approval/pkl/{id}/decide` | guru | Putuskan approval |
| DELETE | `/approval/pkl/{id}` | user | Batalkan request |

Status: `pending`, `accepted`, `rejected`, `needs_further_action`, `cancelled`.
Alur approval bertahap: `wali_kelas → bk → kesiswaan → kaprog`.

### POST /approval/pkl

Body:
```json
{
  "company": "PT Contoh",
  "location": "Jakarta",
  "start_date": "2026-08-01",
  "end_date": "2026-09-01",
  "description": "Deskripsi PKL"
}
```

Date format `YYYY-MM-DD`. Response `201` dengan `PklRequest` (termasuk `steps`).

Error: `400` field wajib / format date salah / end < start, `422` tidak ada approver yang cocok / requester tidak ditemukan.

### POST /approval/pkl/{id}/decide

Body:
```json
{ "decision": "approve", "note": "Disetujui" }
```

Decision valid: `approve`, `reject`, `needs_further_action`. Response `200` dengan request ter-update.

Error: `400` decision tidak valid, `403` bukan giliran anda, `404` tidak ditemukan, `409` step sudah diputuskan / request tidak menunggu keputusan.

### DELETE /approval/pkl/{id}

Body:
```json
{ "reason": "Batal karena alasan" }
```

Hanya status `pending` / `needs_further_action` yang bisa dibatalkan. Response `200` dengan request ter-update.

Error: `400` reason wajib, `403` bukan requester, `404` tidak ditemukan, `409` status tidak bisa dibatalkan.

## Umum

`GET /health` → `{"status":"ok"}`
