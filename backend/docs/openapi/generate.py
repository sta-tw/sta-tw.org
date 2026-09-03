#!/usr/bin/env python3
"""Generate docs/openapi.json from the route literals in internal/**/*.go.

This is a *shallow* spec: every route gets its method, a derived summary, a tag
from its first meaningful path segment, path parameters, and a security
requirement inferred from the path prefix. Request/response bodies are marked as
generic JSON objects except where hand-augmented in OVERRIDES below. It is meant
for client route scaffolding and Hoppscotch/Insomnia import, not as a strict
contract.

Usage:  python3 docs/openapi/generate.py   # writes docs/openapi.json
"""

from __future__ import annotations

import json
import pathlib
import re
import sys

ROOT = pathlib.Path(__file__).resolve().parents[2]
ROUTE_RE = re.compile(r'"(GET|POST|PUT|PATCH|DELETE)\s+(/[^"]*)"')

# Hand-authored request/response detail for the endpoints most worth documenting
# precisely. Everything else falls back to a generic object.
OVERRIDES: dict[tuple[str, str], dict] = {
    ("post", "/api/v1/auth/register"): {
        "summary": "Register a native account",
        "requestBody": {
            "username": {"type": "string"},
            "email": {"type": "string", "format": "email"},
            "password": {"type": "string", "minLength": 12},
        },
    },
    ("post", "/api/v1/auth/login"): {
        "summary": "Log in with username/email + password",
        "requestBody": {
            "identifier": {"type": "string", "description": "username or email"},
            "password": {"type": "string"},
        },
    },
    ("post", "/api/v1/auth/password-reset/request"): {
        "summary": "Request a password-reset email (always 202, no account enumeration)",
        "requestBody": {"email": {"type": "string", "format": "email"}},
    },
    ("post", "/api/v1/auth/password-reset/confirm"): {
        "summary": "Set a new password using the emailed token",
        "requestBody": {
            "token": {"type": "string"},
            "new_password": {"type": "string", "minLength": 12},
        },
    },
    ("post", "/api/v1/auth/password/change"): {
        "summary": "Change password for the logged-in account",
        "requestBody": {
            "current_password": {"type": "string"},
            "new_password": {"type": "string", "minLength": 12},
        },
    },
}


def tag_for(path: str) -> str:
    parts = [p for p in path.split("/") if p and not p.startswith("{")]
    # /api/v1/<area>/...  ->  <area>;  /healthz -> meta
    if parts[:2] == ["api", "v1"]:
        if len(parts) >= 3 and parts[2] == "admin":
            return "admin-" + (parts[3] if len(parts) > 3 else "misc")
        if len(parts) >= 3 and parts[2] == "internal":
            return "internal-" + (parts[3] if len(parts) > 3 else "misc")
        return parts[2] if len(parts) > 2 else "meta"
    return "meta"


def security_for(path: str) -> list[dict]:
    if "/api/v1/internal/" in path:
        return [{"serviceToken": []}]
    if "/api/v1/admin/" in path:
        return [{"sessionCookie": [], "csrf": [], "adminMFA": []}, {"bearer": [], "adminMFA": []}]
    if path in ("/healthz", "/readyz", "/metrics") or path.endswith("/openapi.json"):
        return []
    if "/api/v1/auth/" in path or path == "/api/v1/meta":
        return []  # mostly public / self-authenticating
    return [{"sessionCookie": []}, {"bearer": []}]


def summarize(method: str, path: str) -> str:
    tail = [p for p in path.split("/") if p]
    verb = {"get": "Get", "post": "Create", "put": "Replace", "patch": "Update", "delete": "Delete"}[method]
    noun = " ".join(s.strip("{}").replace("-", " ") for s in tail[2:] or tail)
    return f"{verb} {noun}".strip()


def main() -> int:
    routes: set[tuple[str, str]] = set()
    for go in (ROOT / "internal").rglob("*.go"):
        if go.name.endswith("_test.go"):
            continue
        for m in ROUTE_RE.finditer(go.read_text(encoding="utf-8")):
            routes.add((m.group(1).lower(), m.group(2)))

    paths: dict[str, dict] = {}
    for method, path in sorted(routes, key=lambda r: (r[1], r[0])):
        params = [
            {"name": name, "in": "path", "required": True, "schema": {"type": "string"}}
            for name in re.findall(r"\{([^}]+)\}", path)
        ]
        op: dict = {
            "operationId": f"{method}_" + re.sub(r"[^a-zA-Z0-9]+", "_", path).strip("_"),
            "summary": summarize(method, path),
            "tags": [tag_for(path)],
        }
        if params:
            op["parameters"] = params
        sec = security_for(path)
        if sec:
            op["security"] = sec
        override = OVERRIDES.get((method, path))
        if override:
            op["summary"] = override.get("summary", op["summary"])
        if method in ("post", "put", "patch"):
            body_props = override.get("requestBody") if override else None
            schema = {"type": "object", "properties": body_props} if body_props else {"type": "object"}
            op["requestBody"] = {"content": {"application/json": {"schema": schema}}}
        op["responses"] = {
            "200": {"description": "OK", "content": {"application/json": {"schema": {"type": "object"}}}},
            "4XX": {"description": "Client error", "content": {"application/json": {"schema": {"$ref": "#/components/schemas/Error"}}}},
        }
        paths.setdefault(path, {})[method] = op

    spec = {
        "openapi": "3.1.0",
        "info": {
            "title": "STA Platform API",
            "version": "v1",
            "description": (
                "Auto-generated shallow spec for the STA Go backend. Method, path, "
                "tags and security are accurate; request/response schemas are generic "
                "except for a few hand-augmented endpoints. Regenerate with "
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
            },
            "schemas": {
                "Error": {
                    "type": "object",
                    "properties": {
                        "error": {
                            "type": "object",
                            "properties": {"code": {"type": "string"}, "message": {"type": "string"}},
                            "required": ["code", "message"],
                        }
                    },
                    "required": ["error"],
                }
            },
        },
        "paths": paths,
    }

    payload = json.dumps(spec, indent=2, ensure_ascii=False) + "\n"
    # Canonical, human-reviewed copy + an embed copy the API serves at
    # GET /api/v1/openapi.json (go:embed cannot reach outside internal/).
    for rel in ("docs/openapi.json", "internal/httpapi/openapi.json"):
        (ROOT / rel).write_text(payload, encoding="utf-8")
    print(f"wrote docs/openapi.json + internal/httpapi/openapi.json  ({len(paths)} paths, {len(routes)} operations)")
    return 0


if __name__ == "__main__":
    sys.exit(main())
