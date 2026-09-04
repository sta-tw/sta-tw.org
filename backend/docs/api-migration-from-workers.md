# Frontend migration: Workers/TS backend → Go backend

The Go backend replaces the previous Cloudflare Workers + Hono + Drizzle backend
under `backend/`. Its `/api/v1` surface is **not** a drop-in replacement — the
route set, auth model, and chat data model differ. This document lists what a
frontend built against the old backend must change.

The authoritative route list is `GET /api/v1/openapi.json` (also
`docs/openapi.json`). 198 operations across ~174 paths. The `security` block per
operation is method-aware (cookie mutations list `csrf`, admin routes list
`adminMFA`, inbound webhooks list `webhookSignature`). Keyset list routes carry
`limit`/`cursor` query params; `admin/users`, `admin/audit-log`, `search` and
`events` carry their filter params; and the endpoints a client hits most (auth,
chat, notifications, profile, admin users) have concrete request/response
schemas via `components.schemas`. The remaining domain-admin CRUD routes still
respond as a generic object.

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

**Admin MFA**: once an admin has enrolled TOTP, `/api/v1/admin/*` requests need
`X-MFA-Code`. `POST /api/v1/auth/admin-mfa/verify` `{code}` opens a grant window
(`STA_ADMIN_MFA_GRANT_TTL`, default 15 min) during which admin requests may omit
the header; any request that does carry a valid code refreshes it. Missing code
with no live grant, or an invalid code → `428 Precondition Required`,
`code: "admin_mfa_required"`. Wrong codes are rate-limited per account (8
failures / 15 min); further attempts — including a correct code — then get
`429 rate_limited` until the window clears. A correct code never counts toward
the limit. Set the TTL to `0` to require a code on every request.

## 2. Chat / messages / channels

The old backend had **multiple channels** with reactions, pins, threads,
forwarding and edits, plus a Durable Object WebSocket for realtime.

The Go backend has:

- **Channels**: `GET /api/v1/chat/channels` lists them. The seeded `lounge` is
  the default and the only one bridged to Discord/Telegram; extra channels are
  website-only. `GET/POST /api/v1/chat/channels/{key}/messages` is channel
  scoped (`POST` body `{body, parent_id?}`). `GET/POST /api/v1/chat/lounge/messages`
  still works as an alias for the default channel. Top-level listings exclude
  replies and carry `reply_count`; each message carries a `reactions` summary
  (`[{emoji, count, mine}]`) and `pinned_at`.
- **Reactions**: `PUT`/`DELETE /api/v1/chat/messages/{id}/reactions/{emoji}`
  (URL-encode the emoji). One emoji per account per message; idempotent.
- **Threads**: one level deep. Reply by posting with `parent_id`;
  `GET /api/v1/chat/messages/{id}/replies` reads a thread oldest-first.
- **Pins** (admin only): `POST`/`DELETE /api/v1/chat/messages/{id}/pin`;
  `GET /api/v1/chat/channels/{key}/pins` lists them.
- **Edit / withdraw** (author, website-origin messages only):
  `PATCH /api/v1/chat/messages/{id}` `{body}` and
  `DELETE /api/v1/chat/messages/{id}`. On the default channel the change also
  syncs to Discord/Telegram. `403` for someone else's or a bridged message.
- **Forward**: `POST /api/v1/chat/messages/{id}/forward` `{channel_key}` copies
  the source body into another channel as a new message with
  `forwarded_from_id` set.
- **SSE**: `GET /api/v1/events?channel=lounge,study` follows those chat channels
  (default `lounge`, max 10); each `chat.message` event carries `channel_key`.
  No `?channel` = lounge only, as before.
- **Forum spaces** (a separate concept): a global space plus per-academic-year
  and per-school-program spaces, created automatically when an application is
  confirmed. `GET /api/v1/forum/spaces`, `.../spaces/{id}/threads`,
  `.../threads/{id}/posts`, join/leave. Threads + posts only.
- **Experiences** (心得文章): `GET /api/v1/experiences`, revision workflow with
  admin review. Articles have no comments; forum posts may quote an experience.
- **Reactions** on forum posts and experiences:
  `PUT`/`DELETE /api/v1/forum/posts/{id}/reactions/{emoji}` and
  `PUT`/`DELETE /api/v1/experiences/{id}/reactions/{emoji}` (URL-encode the
  emoji). Post and experience payloads carry a `reactions` summary
  (`[{emoji, count, mine}]`), same shape as chat.
- **Realtime**: `GET /api/v1/events` is a Server-Sent Events stream (auth
  required) carrying `chat.message` for the lounge and `notification.created`
  for the caller. Backed by Postgres LISTEN/NOTIFY so it works across API
  replicas. Reconnect with the browser `EventSource` default.
- **Notifications**: `GET /api/v1/notifications` (keyset paged, see below),
  `GET /api/v1/notifications/unread-count` → `{"unread": n}`,
  `POST /api/v1/notifications/read-all` → `{"marked_read": n}`,
  `POST /api/v1/notifications/{id}/read`.

Cross-platform sync (Discord / Telegram ↔ lounge) is handled by the
`chat-worker` process and inbound webhooks, transparent to the frontend.

## 3. Users / profiles

| Old | Go backend |
|---|---|
| `GET /users/me`, `PATCH /users/me` | `GET /api/v1/auth/me` for the account; `GET`/`PUT /api/v1/profile` for the opt-in profile (`{display_name, bio, links}`, all bounded; created on first `PUT`) |
| `GET /users/me/avatar-upload-url` | `POST /api/v1/profile/avatar` (multipart `file`, PNG/JPEG ≤ 2 MB, ClamAV-scanned, stored privately). `DELETE` to remove. `GET /api/v1/profile/avatar` 302-redirects to a 5-minute presigned URL |
| `GET /users/:username` (public profile) | `GET /api/v1/users/{username}` (auth required) returns display name, bio, links, identity status and `has_avatar`; `GET /api/v1/users/{username}/avatar` 302-redirects to the presigned image |

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
- `GET /api/v1/admin/users` — keyset page over accounts, newest first. Filters:
  `status` (`active`/`suspended`/`deleted`), `identity` (`temporary`/`student`/
  `senior`), `role=admin`, `q` (username prefix). `?limit=` (≤ 100) +
  `next_cursor` → `?cursor=`.
- `GET /api/v1/admin/users/{id}` — one account with role, suspension state,
  active-session / application / experience counts.
- `POST /api/v1/admin/users/{id}/suspend` `{reason}` (1-500 chars) — flips
  `account_status` to `suspended`, revokes every live session (login and the
  session check already gate on `active`), writes an `account.suspended` audit
  row. `409` for self or another admin. CSRF applies.
- `POST /api/v1/admin/users/{id}/reinstate` `{reason?}` — back to `active`,
  clears the suspension columns, `account.reinstated` audit.
- `POST /api/v1/admin/users/{id}/force-logout` `{reason?}` — revokes every live
  session without changing status, `account.force_logout` audit.

**Not present**: a reputation system, portfolio-rule CRUD, public `/stats`.
These are roadmap items.

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
- Responses: a single resource comes back as `{"data": {…}}`; a list as
  `{"data": […], "next_cursor": "…"}`. `204 No Content` (no body) is used for
  successful reaction / pin / mark-read / withdraw calls.
- Pagination:
  - Keyset (opaque cursor) on the high-volume lists — chat channel messages and
    thread replies, channel pins, notifications, published experiences, forum
    threads, forum posts, `admin/users`, `admin/audit-log`. Send `?limit=`
    (≤ 100, default 50); the response carries `next_cursor` (a string). When
    `next_cursor` is `""` there are no more rows; otherwise pass it back as
    `?cursor=`. Cursors are opaque — do not parse them. `?offset=` is ignored on
    these routes.
  - Offset (`?limit=` ≤ 100 `&offset=` ≤ 10000) still applies to the remaining
    admin/list routes (support tickets, admissions programs, sources, portfolio,
    ingestion candidates).
- CORS / credentials: allowed origins come from `STA_ALLOWED_ORIGINS`; responses
  set `Access-Control-Allow-Credentials: true` and echo the caller's `Origin`
  (never `*`). Send `fetch(..., { credentials: "include" })` / `EventSource(...,
  { withCredentials: true })`. A disallowed `Origin` gets `403 origin_not_allowed`.
- CSRF: cookie-authenticated mutations need both the `sta_csrf` cookie value and
  an `X-CSRF-Token` header carrying it. `Authorization: Bearer` callers skip
  this. Missing/wrong → `403 csrf_required`.
- Avatars: `GET .../avatar` answers `302` to a 5-minute presigned URL (works as
  an `<img src>`; a `fetch` must follow redirects). `404 no avatar` when unset.
- `X-Request-ID`: sent back on every response; send your own to correlate.
- `traceparent` (W3C, version 00): honoured if sent, otherwise the API starts a
  trace. The `trace_id` is echoed as `X-Trace-Id`, written on every access-log
  line, carried into the extraction job and sent back on the worker's result
  callback — one id spans API, worker and callback logs. When
  `STA_OTEL_EXPORTER_OTLP_ENDPOINT` is set the API also exports real spans over
  OTLP/HTTP (one server span per request, parented on the inbound `traceparent`)
  and the echoed `X-Trace-Id` is that span's trace id; unset, the propagation is
  log-correlation only with no external dependency.
- Rate limiting: `429` with `code: "rate_limited"`. The rate-limited routes
  (search, chat and support messages, portfolio / verification / brochure /
  avatar uploads, the `/internal/extraction/*` callbacks) send `X-RateLimit-Limit`,
  `X-RateLimit-Remaining`, `X-RateLimit-Reset` (unix seconds) on every response
  and `Retry-After` (seconds) on a `429`. Auth also limits login, register,
  email-verification resend and password-reset requests, and — failures only —
  admin TOTP verification. Other routes are not yet limited.
- SSE (`GET /api/v1/events`): auth required (cookie or bearer). Carries
  `chat.message` (with `channel_key`) for the channels named in `?channel=`
  (default `lounge`, max 10) plus `notification.created` for the caller. The
  server sends `retry: 3000`; reconnect with the browser `EventSource` default.
  Backed by Postgres LISTEN/NOTIFY, so it works across API replicas.
- Versioning: the team keeps changing `/api/v1` in coordination with the
  frontend; there is no `/api/v2` and no deprecation window.

## Roadmap (tracked separately)

per-channel SSE fan-out for non-default channels is in; forum thread
subscriptions and a websocket transport are not.
