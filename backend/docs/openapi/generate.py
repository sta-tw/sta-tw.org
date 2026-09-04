#!/usr/bin/env python3
"""Generate docs/openapi.json from the route literals in internal/**/*.go.

Every route contributes its method, a derived summary, a tag from its first
meaningful path segment, path parameters, and a method-aware security block.
On top of that:

  * keyset list routes (KEYSET) get `limit` + `cursor` query params and a
    `{data: [<item>], next_cursor}` response;
  * QUERY adds documented query params to the filter-heavy routes;
  * REQUEST / RESPONSE attach real schemas (often $refs into `components`) to
    the endpoints a client touches most; everything else stays a generic object.

Usage:  python3 docs/openapi/generate.py   # writes docs/openapi.json
"""

from __future__ import annotations

import json
import pathlib
import re
import sys

ROOT = pathlib.Path(__file__).resolve().parents[2]
ROUTE_RE = re.compile(r'"(GET|POST|PUT|PATCH|DELETE)\s+(/[^"]*)"')

R = lambda name: {"$ref": f"#/components/schemas/{name}"}  # noqa: E731


# --- reusable component schemas -------------------------------------------------

SCHEMAS: dict[str, dict] = {
    "Error": {
        "type": "object",
        "required": ["error"],
        "properties": {
            "error": {
                "type": "object",
                "required": ["code", "message"],
                "properties": {
                    "code": {"type": "string", "description": "stable snake_case discriminator"},
                    "message": {"type": "string", "description": "human text, not stable, mixed en/zh"},
                },
            }
        },
    },
    "Account": {
        "type": "object",
        "required": ["id", "username", "identity_status", "account_status", "email_verified"],
        "properties": {
            "id": {"type": "string", "format": "uuid"},
            "username": {"type": "string"},
            "identity_status": {"type": "string", "enum": ["temporary", "student", "senior"]},
            "account_status": {"type": "string", "enum": ["active", "suspended", "deleted"]},
            "email_verified": {"type": "boolean"},
        },
    },
    "SessionSummary": {
        "type": "object",
        "required": ["id", "created_at", "expires_at"],
        "properties": {
            "id": {"type": "string", "format": "uuid"},
            "created_at": {"type": "string", "format": "date-time"},
            "last_seen_at": {"type": "string", "format": "date-time"},
            "expires_at": {"type": "string", "format": "date-time"},
            "current": {"type": "boolean", "description": "the session making the request"},
        },
    },
    "ReactionTally": {
        "type": "object",
        "required": ["emoji", "count", "mine"],
        "properties": {
            "emoji": {"type": "string"},
            "count": {"type": "integer"},
            "mine": {"type": "boolean", "description": "the caller reacted with this emoji"},
        },
    },
    "ChatChannel": {
        "type": "object",
        "required": ["key", "display_name", "kind", "is_default"],
        "properties": {
            "key": {"type": "string"},
            "display_name": {"type": "string"},
            "kind": {"type": "string", "enum": ["standard", "announcement"]},
            "topic": {"type": "string"},
            "is_default": {"type": "boolean", "description": "the one channel bridged to Discord/Telegram"},
        },
    },
    "ChatMessage": {
        "type": "object",
        "required": ["id", "body", "source_platform", "status", "created_at"],
        "properties": {
            "id": {"type": "string", "format": "uuid"},
            "body": {"type": "string"},
            "source_platform": {"type": "string", "enum": ["website", "discord", "telegram"]},
            "status": {"type": "string", "enum": ["active", "edited", "deleted"]},
            "created_at": {"type": "string", "format": "date-time"},
            "edited_at": {"type": "string", "format": "date-time"},
            "channel_key": {"type": "string"},
            "parent_id": {"type": "string", "format": "uuid", "description": "set on a thread reply"},
            "forwarded_from_id": {"type": "string", "format": "uuid"},
            "pinned_at": {"type": "string", "format": "date-time"},
            "reply_count": {"type": "integer"},
            "reactions": {"type": "array", "items": R("ReactionTally")},
        },
    },
    "Notification": {
        "type": "object",
        "required": ["id", "kind", "title", "body", "created_at"],
        "properties": {
            "id": {"type": "string", "format": "uuid"},
            "kind": {"type": "string"},
            "title": {"type": "string"},
            "body": {"type": "string"},
            "read_at": {"type": "string", "format": "date-time"},
            "created_at": {"type": "string", "format": "date-time"},
        },
    },
    "ProfileLink": {
        "type": "object",
        "required": ["label", "url"],
        "properties": {"label": {"type": "string"}, "url": {"type": "string", "format": "uri"}},
    },
    "Profile": {
        "type": "object",
        "required": ["account_id", "username", "identity_status", "links", "has_avatar"],
        "properties": {
            "account_id": {"type": "string", "format": "uuid"},
            "username": {"type": "string"},
            "identity_status": {"type": "string"},
            "display_name": {"type": "string"},
            "bio": {"type": "string"},
            "links": {"type": "array", "items": R("ProfileLink")},
            "has_avatar": {"type": "boolean"},
            "avatar_updated_at": {"type": "string", "format": "date-time"},
            "updated_at": {"type": "string", "format": "date-time"},
        },
    },
    "AdminUser": {
        "type": "object",
        "required": ["id", "username", "identity_status", "account_status", "email_verified", "is_admin", "created_at"],
        "properties": {
            "id": {"type": "string", "format": "uuid"},
            "username": {"type": "string"},
            "identity_status": {"type": "string"},
            "account_status": {"type": "string"},
            "email_verified": {"type": "boolean"},
            "is_admin": {"type": "boolean"},
            "last_login_at": {"type": "string", "format": "date-time"},
            "suspended_at": {"type": "string", "format": "date-time"},
            "suspension_reason": {"type": "string"},
            "created_at": {"type": "string", "format": "date-time"},
        },
    },
    "AuditRow": {
        "type": "object",
        "properties": {
            "id": {"type": "integer", "format": "int64"},
            "actor_account_id": {"type": "string", "format": "uuid"},
            "action": {"type": "string"},
            "entity_type": {"type": "string"},
            "entity_key": {"type": "string"},
            "before_data": {"type": "object"},
            "after_data": {"type": "object"},
            "reason": {"type": "string"},
            "request_id": {"type": "string"},
            "created_at": {"type": "string", "format": "date-time"},
        },
    },
}


def data_obj(schema: dict) -> dict:
    return {"type": "object", "required": ["data"], "properties": {"data": schema}}


def page_obj(item: dict) -> dict:
    return {
        "type": "object",
        "required": ["data", "next_cursor"],
        "properties": {
            "data": {"type": "array", "items": item},
            "next_cursor": {"type": "string", "description": '"" when there are no more rows'},
        },
    }


# --- per-route hand augmentation ----------------------------------------------

# Keyset list routes: get limit+cursor query params and a paginated response
# whose items use the mapped schema ($ref name, or None for a generic object).
KEYSET: dict[tuple[str, str], str | None] = {
    ("get", "/api/v1/chat/lounge/messages"): "ChatMessage",
    ("get", "/api/v1/chat/channels/{channelKey}/messages"): "ChatMessage",
    ("get", "/api/v1/chat/messages/{messageID}/replies"): "ChatMessage",
    ("get", "/api/v1/notifications"): "Notification",
    ("get", "/api/v1/experiences"): None,
    ("get", "/api/v1/forum/spaces/{spaceID}/threads"): None,
    ("get", "/api/v1/forum/threads/{threadID}/posts"): None,
    ("get", "/api/v1/admin/users"): "AdminUser",
    ("get", "/api/v1/admin/audit-log"): "AuditRow",
}

_LIMIT = {"name": "limit", "in": "query", "schema": {"type": "integer", "minimum": 1, "maximum": 100, "default": 50}}
_CURSOR = {"name": "cursor", "in": "query", "schema": {"type": "string"}, "description": "opaque; from a prior next_cursor"}


def _q(name: str, typ: str = "string", **extra) -> dict:
    p = {"name": name, "in": "query", "schema": {"type": typ}}
    p.update(extra)
    return p


QUERY: dict[tuple[str, str], list[dict]] = {
    ("get", "/api/v1/search"): [
        _q("q", description="search term"),
        _q("types", description="comma list: schools,programs,experiences"),
    ],
    ("get", "/api/v1/schools"): [_q("q", description="name/code prefix")],
    ("get", "/api/v1/events"): [
        _q("channel", description="comma list of chat channel keys (default lounge, max 10)"),
    ],
    ("get", "/api/v1/admin/users"): [
        _q("status", description="active|suspended|deleted"),
        _q("identity", description="temporary|student|senior"),
        _q("role", description="admin"),
        _q("q", description="username prefix"),
    ],
    ("get", "/api/v1/admin/audit-log"): [
        _q("entity_type"), _q("entity_key"), _q("action"),
        _q("actor", description="actor account UUID"),
        _q("since", description="RFC3339"), _q("until", description="RFC3339"),
    ],
}

# Request bodies + specific response schemas / extra status codes.
OVERRIDES: dict[tuple[str, str], dict] = {
    ("post", "/api/v1/auth/register"): {
        "summary": "Register a native account",
        "request": {"username": {"type": "string"}, "email": {"type": "string", "format": "email"},
                    "password": {"type": "string", "minLength": 12}},
        "response": {"type": "object", "properties": {
            "account": R("Account"), "email_verification_queued": {"type": "boolean"}}},
        "status": "201",
    },
    ("post", "/api/v1/auth/login"): {
        "summary": "Log in with username + password (username only, not email)",
        "request": {"username": {"type": "string"}, "password": {"type": "string"}},
        "response": {"type": "object", "properties": {
            "account": R("Account"), "expires_at": {"type": "string", "format": "date-time"}}},
    },
    ("post", "/api/v1/auth/password-reset/request"): {
        "summary": "Request a password-reset email (always 202, no account enumeration)",
        "request": {"email": {"type": "string", "format": "email"}},
        "status": "202",
    },
    ("post", "/api/v1/auth/password-reset/confirm"): {
        "summary": "Set a new password using the emailed token",
        "request": {"token": {"type": "string"}, "new_password": {"type": "string", "minLength": 12}},
        "status": "204",
    },
    ("post", "/api/v1/auth/password/change"): {
        "summary": "Change password for the logged-in account (revokes other sessions)",
        "request": {"current_password": {"type": "string"}, "new_password": {"type": "string", "minLength": 12}},
        "status": "204",
    },
    ("post", "/api/v1/auth/email-verification/confirm"): {
        "summary": "Confirm an email with the emailed token", "request": {"token": {"type": "string"}}, "status": "204",
    },
    ("get", "/api/v1/auth/me"): {"response": data_obj(R("Account"))},
    ("get", "/api/v1/auth/sessions"): {"response": data_obj({"type": "array", "items": R("SessionSummary")})},
    ("post", "/api/v1/auth/admin-mfa/verify"): {
        "summary": "Verify a TOTP code and open the admin MFA grant window",
        "request": {"code": {"type": "string"}},
        "response": {"type": "object", "properties": {
            "verified": {"type": "boolean"}, "expires_at": {"type": "string", "format": "date-time"},
            "grant_seconds": {"type": "integer"}}},
    },
    ("post", "/api/v1/auth/admin-mfa/enable"): {"request": {"code": {"type": "string"}}},
    ("post", "/api/v1/auth/admin-mfa/disable"): {"request": {"code": {"type": "string"}}},
    ("get", "/api/v1/chat/channels"): {"response": data_obj({"type": "array", "items": R("ChatChannel")})},
    ("post", "/api/v1/chat/lounge/messages"): {
        "request": {"body": {"type": "string", "maxLength": 2000}},
        "response": data_obj(R("ChatMessage")), "status": "201",
    },
    ("post", "/api/v1/chat/channels/{channelKey}/messages"): {
        "request": {"body": {"type": "string", "maxLength": 2000},
                    "parent_id": {"type": "string", "format": "uuid", "description": "reply target (one level)"}},
        "response": data_obj(R("ChatMessage")), "status": "201",
    },
    ("patch", "/api/v1/chat/messages/{messageID}"): {
        "summary": "Edit your own website message",
        "request": {"body": {"type": "string", "maxLength": 2000}}, "response": data_obj(R("ChatMessage")),
    },
    ("delete", "/api/v1/chat/messages/{messageID}"): {"summary": "Withdraw your own website message", "status": "204"},
    ("post", "/api/v1/chat/messages/{messageID}/forward"): {
        "summary": "Forward a message into another channel",
        "request": {"channel_key": {"type": "string"}}, "response": data_obj(R("ChatMessage")), "status": "201",
    },
    ("put", "/api/v1/chat/messages/{messageID}/reactions/{emoji}"): {"summary": "Add a reaction (URL-encode the emoji)", "status": "204"},
    ("delete", "/api/v1/chat/messages/{messageID}/reactions/{emoji}"): {"summary": "Remove a reaction", "status": "204"},
    ("get", "/api/v1/chat/channels/{channelKey}/pins"): {"response": data_obj({"type": "array", "items": R("ChatMessage")})},
    ("post", "/api/v1/chat/messages/{messageID}/pin"): {"summary": "Pin a message (admin)", "status": "204"},
    ("delete", "/api/v1/chat/messages/{messageID}/pin"): {"summary": "Unpin a message (admin)", "status": "204"},
    ("put", "/api/v1/forum/posts/{postID}/reactions/{emoji}"): {"summary": "Add a reaction to a forum post", "status": "204"},
    ("delete", "/api/v1/forum/posts/{postID}/reactions/{emoji}"): {"summary": "Remove a forum-post reaction", "status": "204"},
    ("put", "/api/v1/experiences/{experienceID}/reactions/{emoji}"): {"summary": "Add a reaction to an experience", "status": "204"},
    ("delete", "/api/v1/experiences/{experienceID}/reactions/{emoji}"): {"summary": "Remove an experience reaction", "status": "204"},
    ("get", "/api/v1/notifications/unread-count"): {"response": {"type": "object", "properties": {"unread": {"type": "integer"}}}},
    ("post", "/api/v1/notifications/read-all"): {"response": {"type": "object", "properties": {"marked_read": {"type": "integer"}}}},
    ("post", "/api/v1/notifications/{notificationID}/read"): {"status": "204"},
    ("get", "/api/v1/profile"): {"response": data_obj(R("Profile"))},
    ("put", "/api/v1/profile"): {
        "summary": "Create or update your profile",
        "request": {"display_name": {"type": "string", "maxLength": 80}, "bio": {"type": "string", "maxLength": 500},
                    "links": {"type": "array", "items": R("ProfileLink"), "maxItems": 10}},
        "response": data_obj(R("Profile")),
    },
    ("post", "/api/v1/profile/avatar"): {
        "summary": "Upload an avatar (multipart form field `file`, PNG/JPEG <= 2 MB)",
        "requestContent": {"multipart/form-data": {"schema": {"type": "object", "properties": {
            "file": {"type": "string", "format": "binary"}}}}},
        "response": data_obj(R("Profile")),
    },
    ("delete", "/api/v1/profile/avatar"): {"summary": "Remove your avatar", "status": "204"},
    ("get", "/api/v1/profile/avatar"): {"summary": "302 redirect to a 5-minute presigned avatar URL", "status": "302"},
    ("get", "/api/v1/users/{username}"): {"response": data_obj(R("Profile"))},
    ("get", "/api/v1/users/{username}/avatar"): {"summary": "302 redirect to a presigned avatar URL", "status": "302"},
    ("get", "/api/v1/admin/stats"): {"summary": "Platform statistics snapshot"},
    ("post", "/api/v1/admin/users/{accountID}/suspend"): {
        "request": {"reason": {"type": "string", "minLength": 1, "maxLength": 500}},
        "response": {"type": "object", "properties": {"status": {"type": "string"}, "sessions_revoked": {"type": "integer"}}},
    },
    ("post", "/api/v1/admin/users/{accountID}/reinstate"): {"request": {"reason": {"type": "string"}}},
    ("post", "/api/v1/admin/users/{accountID}/force-logout"): {
        "request": {"reason": {"type": "string"}},
        "response": {"type": "object", "properties": {"sessions_revoked": {"type": "integer"}}},
    },
    ("get", "/api/v1/admin/users/{accountID}"): {"response": data_obj(R("AdminUser"))},
    ("get", "/api/v1/meta"): {"response": {"type": "object", "properties": {
        "api_version": {"type": "string"}, "service": {"type": "string"}}}},
}


# --- route -> operation ------------------------------------------------------

_PUBLIC_AUTH = {
    "/api/v1/auth/register",
    "/api/v1/auth/login",
    "/api/v1/auth/password-reset/request",
    "/api/v1/auth/password-reset/confirm",
    "/api/v1/auth/email-verification/confirm",
    "/api/v1/auth/oauth/{provider}/start",
    "/api/v1/auth/oauth/{provider}/callback",
}


def tag_for(path: str) -> str:
    parts = [p for p in path.split("/") if p and not p.startswith("{")]
    if parts[:2] == ["api", "v1"]:
        if len(parts) >= 3 and parts[2] == "admin":
            return "admin-" + (parts[3] if len(parts) > 3 else "misc")
        if len(parts) >= 3 and parts[2] == "internal":
            return "internal-" + (parts[3] if len(parts) > 3 else "misc")
        return parts[2] if len(parts) > 2 else "meta"
    return "meta"


def security_for(method: str, path: str) -> list[dict]:
    mutating = method in ("post", "put", "patch", "delete")
    if "/api/v1/internal/" in path:
        return [{"serviceToken": []}]
    if path in ("/healthz", "/readyz", "/metrics") or path.endswith("/openapi.json") or path == "/api/v1/meta":
        return []
    if "/webhooks/" in path:
        return [{"webhookSignature": []}]
    if path in _PUBLIC_AUTH:
        return []
    if "/api/v1/admin/" in path:
        cookie = {"sessionCookie": [], "adminMFA": []}
        token = {"bearer": [], "adminMFA": []}
        if mutating:
            cookie["csrf"] = []
        return [cookie, token]
    cookie = {"sessionCookie": []}
    if mutating:
        cookie["csrf"] = []
    return [cookie, {"bearer": []}]


def summarize(method: str, path: str) -> str:
    tail = [p for p in path.split("/") if p]
    verb = {"get": "Get", "post": "Create", "put": "Replace", "patch": "Update", "delete": "Delete"}[method]
    noun = " ".join(s.strip("{}").replace("-", " ") for s in tail[2:] or tail)
    return f"{verb} {noun}".strip()


def json_response(desc: str, schema: dict) -> dict:
    return {"description": desc, "content": {"application/json": {"schema": schema}}}


def build_op(method: str, path: str) -> dict:
    ov = OVERRIDES.get((method, path), {})
    keyset = (method, path) in KEYSET

    op: dict = {
        "operationId": f"{method}_" + re.sub(r"[^a-zA-Z0-9]+", "_", path).strip("_"),
        "summary": ov.get("summary") or summarize(method, path),
        "tags": [tag_for(path)],
    }

    params: list[dict] = [
        {"name": name, "in": "path", "required": True, "schema": {"type": "string"}}
        for name in re.findall(r"\{([^}]+)\}", path)
    ]
    if keyset:
        params += [_LIMIT, _CURSOR]
    params += QUERY.get((method, path), [])
    if params:
        op["parameters"] = params

    sec = security_for(method, path)
    if sec:
        op["security"] = sec

    if "requestContent" in ov:
        op["requestBody"] = {"required": True, "content": ov["requestContent"]}
    elif method in ("post", "put", "patch"):
        props = ov.get("request")
        schema = {"type": "object", "properties": props} if props else {"type": "object"}
        op["requestBody"] = {"content": {"application/json": {"schema": schema}}}

    ok_status = ov.get("status", "200")
    responses: dict = {}
    if ok_status == "204":
        responses["204"] = {"description": "No Content"}
    elif ok_status == "302":
        responses["302"] = {"description": "Redirect to a presigned URL (Location header)"}
    else:
        if keyset:
            item = KEYSET[(method, path)]
            body = page_obj(R(item) if item else {"type": "object"})
        else:
            body = ov.get("response", {"type": "object"})
        responses[ok_status] = json_response("OK", body)
    responses["4XX"] = json_response("Client error", R("Error"))
    op["responses"] = responses
    return op


def main() -> int:
    routes: set[tuple[str, str]] = set()
    for go in (ROOT / "internal").rglob("*.go"):
        if go.name.endswith("_test.go"):
            continue
        for m in ROUTE_RE.finditer(go.read_text(encoding="utf-8")):
            routes.add((m.group(1).lower(), m.group(2)))

    paths: dict[str, dict] = {}
    for method, path in sorted(routes, key=lambda r: (r[1], r[0])):
        paths.setdefault(path, {})[method] = build_op(method, path)

    spec = {
        "openapi": "3.1.0",
        "info": {
            "title": "STA Platform API",
            "version": "v1",
            "description": (
                "Generated from the route literals in internal/**/*.go. Method, path, "
                "tags and the method-aware security block are accurate for every route; "
                "keyset list routes and the endpoints a client touches most also carry "
                "real query params and response schemas. Regenerate with "
                "`python3 docs/openapi/generate.py`."
            ),
        },
        "servers": [{"url": "http://localhost:8080", "description": "local"}],
        "components": {
            "securitySchemes": {
                "sessionCookie": {"type": "apiKey", "in": "cookie", "name": "sta_session"},
                "csrf": {"type": "apiKey", "in": "header", "name": "X-CSRF-Token"},
                "bearer": {"type": "http", "scheme": "bearer"},
                "adminMFA": {"type": "apiKey", "in": "header", "name": "X-MFA-Code"},
                "serviceToken": {"type": "http", "scheme": "bearer", "description": "extraction / agent service token"},
                "webhookSignature": {"type": "apiKey", "in": "header", "name": "X-STA-Signature",
                                     "description": "HMAC-SHA256 of the raw body"},
            },
            "schemas": SCHEMAS,
        },
        "paths": paths,
    }

    payload = json.dumps(spec, indent=2, ensure_ascii=False) + "\n"
    for rel in ("docs/openapi.json", "internal/httpapi/openapi.json"):
        (ROOT / rel).write_text(payload, encoding="utf-8")
    n_ops = sum(len(v) for v in paths.values())
    print(f"wrote docs/openapi.json + internal/httpapi/openapi.json  ({len(paths)} paths, {n_ops} operations)")
    return 0


if __name__ == "__main__":
    sys.exit(main())
