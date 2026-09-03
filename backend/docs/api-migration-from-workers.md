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

Cross-cutting operator routes (admin role + admin MFA, read-only, no CSRF):

- `GET /api/v1/admin/stats` — one JSON snapshot: entity counts (accounts by
  status/identity, applications, experiences, forum, chat, support tickets,
  verification requests, result batches, audit-log total) plus the backlog of
  every retry outbox (`pending` / `failed` / `abandoned` for email, chat sync,
  support Discord, willingness notifications). `abandoned > 0` is the alert.
- `GET /api/v1/admin/audit-log` — global query over the shared `audit_log`
  (the per-domain `…/history` routes only ever show one entity). Filters:
  `entity_type`, `entity_key`, `action`, `actor` (UUID), `since` / `until`
  (RFC3339). Keyset pagination: `?limit=` (≤ 100) + `next_cursor` → `?cursor=`.

**Not present**: generic user management (list/suspend/force-logout), a
reputation system, portfolio-rule CRUD, public `/stats`. These are roadmap
items.

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
- Pagination:
  - Keyset (opaque cursor) on the high-volume lists — chat lounge messages,
    notifications, published experiences, forum threads, forum posts. Send
    `?limit=` (≤ 100, default 50); the response carries `next_cursor` (a string).
    When `next_cursor` is `""` there are no more rows; otherwise pass it back as
    `?cursor=`. Cursors are opaque — do not parse them. `?offset=` is ignored on
    these routes.
  - Offset (`?limit=` ≤ 100 `&offset=` ≤ 10000) still applies to the remaining
    admin/list routes (support tickets, admissions programs, sources, portfolio,
    ingestion candidates).
- `X-Request-ID`: sent back on every response; send your own to correlate.
- `traceparent` (W3C, version 00): honoured if sent, otherwise the API starts a
  trace. The `trace_id` is echoed as `X-Trace-Id`, written on every access-log
  line, carried into the extraction job and sent back on the worker's result
  callback — one id spans API, worker and callback logs. Not an OpenTelemetry
  exporter yet; it is log correlation only.
- Rate limiting: `429` with `code: "rate_limited"`. The rate-limited routes
  (search, chat and support messages, portfolio / verification / brochure
  uploads, the `/internal/extraction/*` callbacks) send `X-RateLimit-Limit`,
  `X-RateLimit-Remaining`, `X-RateLimit-Reset` (unix seconds) on every response
  and `Retry-After` (seconds) on a `429`. Other routes are not yet limited.
- Versioning: the team keeps changing `/api/v1` in coordination with the
  frontend; there is no `/api/v2` and no deprecation window.

## Roadmap (tracked separately)

generic user management (list / suspend / force-logout) · broader rate-limit
coverage (auth, admin mutations) · a real OpenTelemetry exporter (the
`traceparent` propagation is already in place) · HMAC lookup-hash key rotation ·
message reactions / pins / threads · multi-channel chat · user profiles +
avatars.
