# STA

Monorepo for the STA backend and frontend.

## Structure
- `backend/`: API server and services
- `frontend/`: Next.js web app
- `api-docs.md`: API reference
- `HOPPSCOTCH.md`: Hoppscotch integration guide

## API Testing

### Hoppscotch (Recommended)
Import OpenAPI spec directly into [Hoppscotch](https://hoppscotch.io/):
```
https://sta-backend.sta-tw.workers.dev/openapi.json
```

Or use this quick link: [Open in Hoppscotch](https://hoppscotch.io/?import=https://sta-backend.sta-tw.workers.dev/openapi.json)

See [HOPPSCOTCH.md](./HOPPSCOTCH.md) for detailed setup guide.

### OpenAPI Spec
Access the OpenAPI 3.0 specification at:
- Production: `https://sta-backend.sta-tw.workers.dev/openapi.json`
- Local: `http://localhost:12004/openapi.json`
- Compatible with Swagger UI, Postman, Insomnia, and other API clients

## Deployment (Docker Compose)

The frontend (static Next.js export) and backend are deployed together on one
host via Caddy, which serves the static site and reverse-proxies `/api/*` to
the Go API. See [backend/deploy/README.md](backend/deploy/README.md) for the
local-dev and production quick starts.

## Frontend
See `frontend/README.md` for app-specific details.

## Backend
Use `backend/.env.example` as a reference for environment variables.
