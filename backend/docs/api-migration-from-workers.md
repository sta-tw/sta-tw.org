# Frontend migration: Workers/TS backend → Go backend

The Go backend replaces the previous Cloudflare Workers + Hono + Drizzle backend
under `backend/`. Its `/api/v1` surface is **not** a drop-in replacement — the
route set, auth model, and chat data model differ. This document lists what a
frontend built against the old backend must change.

The authoritative route list is `GET /api/v1/openapi.json` (also
`docs/openapi.json`). 159 operations across ~145 paths.

## 1. Auth & sessions

| Old (Workers) | Go backend |
|---|---|
| `POST /auth/register` | `POST /api/v1/auth/register` — body `{username, email, password}` (password ≥ 12 chars). Returns `{account, email_verification_queued}` |
| `POST /auth/login` | `POST /api/v1/auth/login` — body `{username, password}`. **Login is by username only**, not email. Sets an HttpOnly session cookie + a CSRF cookie |
| `POST /auth/logout` | `POST /api/v1/auth/logout` |
| `GET /auth/me` | `GET /api/v1/auth/me` |
| `POST /auth/refresh` | **removed** — sessions are long-lived (`STA_SESSION_TTL`, default 30d) and renewed server-side; there is no refresh token |
| `POST /auth/forgot-password` / `reset-password` | `POST /api/v1/auth/password-reset/request` (always `202`, no account enumeration) → email token → `POST /api/v1/auth/password-reset/confirm` `{token, new_password}`. Confirming revokes every session |
| `POST /auth/change-password` | `POST /api/v1/auth/password/change` `{current_password, new_password}` (logged in; revokes other sessions) |
| `POST /auth/verify-email` / `resend-verification` | `POST /api/v1/auth/email-verification/confirm` `{token}` (works with just the emailed token, or with a session) / `POST /api/v1/auth/email-verification/resend` (session required) |
| `GET /auth/sessions`, `DELETE /auth/sessions/:id`, `DELETE /auth/sessions` | `GET /api/v1/auth/sessions` (redacted list; IP/UA are hash-only), `DELETE /api/v1/auth/sessions/{id}` (one other session), `DELETE /api/v1/auth/sessions` (all others). Current session is kept — use logout for it. |
| `GET /auth/oauth/:provider` + callback | `GET /api/v1/auth/oauth/{provider}/start` + `GET /api/v1/auth/oauth/{provider}/callback`; bind an extra provider with `POST /api/v1/auth/oauth/{provider}/bind/start`. Providers: `google`, `discord` |

**CSRF**: any state-changing request made with the session **cookie** must send
both the `sta_csrf` cookie value and an `X-CSRF-Token` header with the same
value. Requests authenticated with `Authorization: Bearer <token>` skip CSRF.

**Admin MFA**: once an admin has enrolled TOTP, every `/api/v1/admin/*` request
must include `X-MFA-Code`. Missing/!valid → `428 Precondition Required`,
`code: "admin_mfa_required"`.

## 2. Chat / messages / channels

The old backend had **multiple channels** with reactions, pins, threads,
forwarding and edits, plus a Durable Object WebSocket for realtime.

The Go backend has:

- **One global "lounge"**: `GET/POST /api/v1/chat/lounge/messages`. No channels,
  no reactions, no pins, no threads, no edit/withdraw, no forward.
- **Forum spaces** (a separate concept): a global space plus per-academic-year
  and per-school-program spaces, created automatically when an application is
  confirmed. `GET /api/v1/forum/spaces`, `.../spaces/{id}/threads`,
  `.../threads/{id}/posts`, join/leave. Threads + posts only.
- **Experiences** (心得文章): `GET /api/v1/experiences`, revision workflow with
  admin review. Articles have no comments; forum posts may quote an experience.
- **Realtime**: `GET /api/v1/events` is a Server-Sent Events stream (auth
  required) carrying `chat.message` for the lounge and `notification.created`
  for the caller. Backed by Postgres LISTEN/NOTIFY so it works across API
  replicas. Reconnect with the browser `EventSource` default.

Cross-platform sync (Discord / Telegram ↔ lounge) is handled by the
`chat-worker` process and inbound webhooks, transparent to the frontend.

## 3. Users / profiles

| Old | Go backend |
|---|---|
| `GET /users/me`, `PATCH /users/me` | only `GET /api/v1/auth/me`. **No profile editing endpoint** |
| `GET /users/me/avatar-upload-url` | **no avatars** |
| `GET /users/:username` (public profile) | **no public profiles** |

## 4. Admin

The old `admin.ts` was a single console API (`/stats`, `/users`, `/audit-log` +
export, `/reputation`, `/portfolio-rules` CRUD, `/school-options`,
`/school-requests`). The Go backend instead exposes **per-domain** admin routes
under `/api/v1/admin/…`:

- `admin/schools` (+ `sync`, `{code}/history`)
- `admin/admissions/programs` (+ `sync`, `{id}/review`, `{id}/history`, `PUT`)
- `admin/admissions/brochures`, `admin/admissions/brochure-discovery/*`
- `admin/admission-sources`
- `admin/results/*` (import, batches, publish, correct, inquiries)
- `admin/portfolio/files` (+ `{id}/review`, `{id}/events`)
- `admin/verification/*` (domains, pending requests, review, document download)
- `admin/support/tickets` (+ messages, close, attachments)
- `admin/applications/service-tickets` (+ `{id}/review`)
- `admin/ingestion/*` (brochure runs, jobs, candidate review)
- `admin/telegram-cross-check/*`
- `admin/experience-revisions/{id}/review`

**Not present**: unified `/admin/stats`, generic user management
(list/suspend/force-logout), a global audit-log query API (only per-entity
`…/history`), a reputation system, portfolio-rule CRUD, public `/stats`. These
are roadmap items.

## 5. Portfolio

| Old | Go backend |
|---|---|
| `GET /portfolio/upload-url` → direct-to-storage | `POST /api/v1/portfolio/projects/{id}/files` (multipart through the API; the API stores privately and scans with ClamAV) |
| `GET /portfolio/:doc/download-url` | `GET /api/v1/portfolio/files/{id}/download` (API issues a short-lived signed URL) |
| `long-view` / `share-view` / `heartbeat` view tracking | **not implemented** |
| `PATCH /portfolio/:doc/approve` | `POST /api/v1/admin/portfolio/files/{id}/review` `{approved, reason}` |
| `school-request` | `POST /api/v1/applications/service-tickets` / `admin/…/review` |

Projects are tied to a confirmed application; files have a versioned state
machine `hidden → pending_review → published` (+ `unpublished`, `rejected`).

## 6. Search

Old: `GET /search` (Meilisearch multi-search over messages/channels/users) +
`POST /search/reindex`.

Go backend: `GET /api/v1/search?q=<term>&types=schools,programs,experiences`
(public, rate-limited) runs a Meilisearch multi-search and returns hits grouped
by index. `POST /api/v1/admin/search/reindex` (admin) or `cmd/reindex` rebuilds
the indexes from PostgreSQL. Requires `STA_MEILISEARCH_URL`; when unset the
route is not mounted. School master also still has `GET /api/v1/schools?q=`.

## 7. Conventions

- Errors: `{"error": {"code": "<snake_case>", "message": "<human text>"}}`.
  Branch on `code`; `message` is not stable and mixes English/Chinese.
- Pagination: `?limit=` (≤ 100) `&offset=` (≤ 10000). Offset only for now;
  cursor pagination for high-volume lists (forum, chat, notifications) is a
  roadmap item.
- `X-Request-ID`: sent back on every response; send your own to correlate.
- No `X-RateLimit-*` headers yet (roadmap). `429` with `code: "rate_limited"`.
- Versioning: the team keeps changing `/api/v1` in coordination with the
  frontend; there is no `/api/v2` and no deprecation window.

## Roadmap (tracked separately)

cursor pagination (forum / chat / notifications) · unified `/admin/stats` +
user management + audit-log query API · account deletion / data export ·
`X-RateLimit-*` response headers + broader rate-limit coverage · OpenTelemetry
tracing across the job boundary · encryption key-version columns for AES/HMAC
key rotation · message reactions / pins / threads · multi-channel chat · user
profiles + avatars.
