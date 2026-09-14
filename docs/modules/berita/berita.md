---
name: Berita Domain Documentation
relation: RULES.md → modules/berita/
description: Documentation for the berita (news) domain — markdown article CRUD, cover image, and inline content images
type: editable
---

# Berita Domain

## Overview

The `berita` domain handles news article CRUD and image management. Articles are authored by users with the `jurnal` role. The article `content` is plain markdown with support for multiple paragraphs, headings, lists, bold/italic, blockquotes, links, and inline images embedded in the text ("mini Google Docs"). Images are stored as plain files on **local disk** (`STORAGE_DIR`) and served through a **read-through proxy** endpoint that never expires. Future email rendering will use the same markdown (via goldmark) — no format migration needed.

## Content format

- `content` is **plain markdown**, stored in the existing TEXT column. No frontmatter, no MDX, no HTML, no structured JSON.
- Content must be non-empty and at most **100 KB** (enforced by the service).
- Inline images use standard markdown: `![caption](berita/{id}/content/{uuid}.{ext})`. The destination is an object **key**, never a signed URL.
- External image URLs (`https://...`) in content are left untouched and never signed.

## Entity

```go
type Berita struct {
    ID        id.ID     `json:"id"`
    AuthorID  id.ID     `json:"author_id"`
    Title     string    `json:"title"`
    Content   string    `json:"content"`
    ImageURL  string    `json:"image_url,omitempty"`
    CreatedAt time.Time `json:"created_at"`
    UpdatedAt time.Time `json:"updated_at"`
}
```

## Endpoints

Reads are **public** — listing and reading articles requires no authentication. All write endpoints require authentication (`Bearer` token) and the `jurnal` role.

| Method | Path | Handler | Description | Access |
|---|---|---|---|---|
| GET | /api/v1/berita | List | List all news articles | public |
| GET | /api/v1/berita/{id} | Get | Get article by ID | public |
| POST | /api/v1/berita/{id}/engagement | Engagement | Record a read/share/link event | public |
| GET | /api/v1/berita/images/{key} | GetImage | Stream an image object by key (never expires) | public |
| POST | /api/v1/berita | Create | Create a new article | jurnal |
| PUT | /api/v1/berita/{id} | Update | Update article (author only) | jurnal |
| DELETE | /api/v1/berita/{id} | Delete | Delete article + its images (author only) | jurnal |
| POST | /api/v1/berita/{id}/image | UploadImage | Upload/replace article cover image | jurnal |
| POST | /api/v1/berita/{id}/images | UploadContentImage | Upload an inline content image (author only) | jurnal |
| DELETE | /api/v1/berita/{id}/images?key={key} | DeleteContentImage | Delete an inline content image (author only) | jurnal |

### Inline image flow

1. **Upload** — `POST /api/v1/berita/{id}/images` (multipart `image` field). Author-only; the article must exist. Validates MIME (jpeg/png/gif/webp) and 5 MB max. Stores the object at `berita/{id}/content/{uuid}.{ext}` and returns `{"image_url":"<object key>"}`.
2. **Embed** — the frontend inserts that key into the markdown (`![caption](<key>)`) and saves via POST/PUT. The backend rewrites legacy bucket-prefixed signed URLs back to bare keys on write (`normalizeImageRefs`).
3. **Read** — `Get`/`List` resolve internal `berita/...` keys to stable proxy URLs inside the returned `content` and `image_url` (`resolveImageRefs`). Each URL points at `GET /api/v1/berita/images/{key}`, which streams the object from storage on every request — so URLs never expire and long-open pages cannot go stale. External URLs pass through untouched.
4. **Delete** — when an image is removed from the editor, the frontend calls `DELETE /api/v1/berita/{id}/images?key={key}` (and on editor teardown for uploads never embedded). The key is validated to be a bare key under `berita/{id}/content/` — it cannot target the cover image, another article's images, or arbitrary objects.

### Engagement analytics

`GET /api/v1/berita/{id}` records a `berita.view` event. The frontend records meaningful actions through `POST /api/v1/berita/{id}/engagement`:

```json
{"event":"read_50","sessionId":"anonymous-session-id"}
```

Allowed events: `read_50`, `read_90`, `share`, `link_click`. Events contain only article ID and SHA-256 session hash. They do not store article content, titles, IP addresses, or link URLs. `GET /api/v1/analytics/berita/summary?days=30` returns aggregate event counts and requires `jurnal` or `admin` authentication.

## Data flow

```
GET /api/v1/berita
  → handler.List → service.List → pg.List (SELECT ... ORDER BY created_at DESC)
  → handler rewrites each image_url + inline content image key to a proxy URL under /api/v1/berita/images/{key}

GET /api/v1/berita/images/{key}
  → handler.GetImage: validate key prefix → store.Get(key) → stream bytes (long-lived Cache-Control)

POST /api/v1/berita
  → handler.Create: decode JSON → validate title → normalizeImageRefs(content)
    → service.Create: validate content (non-empty, ≤100 KB) → generate ID → create record
      → pg.Create: INSERT INTO berita (id, author_id, title, content, ...)
  → handler signs response

POST /api/v1/berita/{id}/image
  → handler.UploadImage: parse multipart form → validate MIME type (jpeg/png/gif/webp)
    → enforce 5 MB max via MaxBytesReader → upload via store.Upload
    → call service.SetImage → pg.Update (set image_url)
  → handler signs image_url before response

POST /api/v1/berita/{id}/images
  → handler.UploadContentImage: verify article exists + caller is author
    → parse/validate/upload (shared with cover) → store at berita/{id}/content/{uuid}.{ext}
  → returns {"image_url":"<key>"}; nothing is persisted until the content is saved

DELETE /api/v1/berita/{id}/images?key={key}
  → handler.DeleteContentImage: validate key prefix → verify caller is author
    → store.Delete(key)

DELETE /api/v1/berita/{id}
  → handler.Delete: fetch article → delete cover + every inline key extracted from content
    → service.Delete: verify caller is author → pg.Delete
```

## Rules

- The `Get`/`List` read routes are public (no auth middleware). `List` and `Get` do not read the request context for user identity, so anonymous access is safe.
- All write endpoints require the `jurnal` role (auth + role middleware applied in `Register`).
- Only the article author can update, delete, or upload/delete images on their own articles (`forbidden: not the author`).
- Content is plain markdown, required, and limited to 100 KB.
- Image uploads are limited to 5 MB, accepted types: `image/jpeg`, `image/png`, `image/gif`, `image/webp`.
- Cover images are stored at `berita/{beritaID}/{uuid}.{ext}`; inline images at `berita/{beritaID}/content/{uuid}.{ext}`.
- Stored `content` and `image_url` contain object keys (never signed URLs). On read they are resolved to proxy URLs under `GET /api/v1/berita/images/{key}`; the backend streams the object so URLs do not expire.
- Deleting an article also deletes its cover and all inline images referenced in its content.
- Orphaned inline images (uploaded, then never embedded or explicitly deleted) are not tracked in the database; normal editor flows clean them up via the delete endpoint. A storage garbage-collection job is a potential future follow-up.

## cURL examples

```bash
# List articles (public — no auth required)
curl http://localhost:8080/api/v1/berita

# Get one article (public — no auth required)
curl http://localhost:8080/api/v1/berita/{id}

# Create article
curl -X POST http://localhost:8080/api/v1/berita \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer <token>' \
  -d '{"title":"Breaking News","content":"## Lead\n\nSomething happened today.\n\n## Details\n\n- first\n- second"}'

# Upload inline image
curl -X POST http://localhost:8080/api/v1/berita/{id}/images \
  -H 'Authorization: Bearer <token>' \
  -F 'image=@photo.png'
# → {"image_url":"berita/{id}/content/{uuid}.png"}

# Save article embedding the inline image
curl -X PUT http://localhost:8080/api/v1/berita/{id} \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer <token>' \
  -d '{"title":"Breaking News","content":"![caption](berita/{id}/content/{uuid}.png)\n\nDetails."}'

# Delete inline image
curl -X DELETE 'http://localhost:8080/api/v1/berita/{id}/images?key=berita/{id}/content/{uuid}.png' \
  -H 'Authorization: Bearer <token>'

# Upload cover image
curl -X POST http://localhost:8080/api/v1/berita/{id}/image \
  -H 'Authorization: Bearer <token>' \
  -F 'image=@photo.jpg'

# Delete article (also removes cover + inline images)
curl -X DELETE http://localhost:8080/api/v1/berita/{id} \
  -H 'Authorization: Bearer <token>'
```
