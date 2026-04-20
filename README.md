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
http://localhost:12004/openapi.json
```

See [HOPPSCOTCH.md](./HOPPSCOTCH.md) for detailed setup guide.

### OpenAPI Spec
Access the OpenAPI 3.0 specification at:
- Local: `http://localhost:12004/openapi.json`
- Compatible with Swagger UI, Postman, Insomnia, and other API clients

## Local Development (Docker)
1. Ensure Docker is running.
2. Start services:

```bash
docker compose up --build
```

Default ports:
- Frontend: `http://localhost:12003`
- Backend: `http://localhost:12004`

## Frontend
See `frontend/README.md` for app-specific details.

## Backend
Use `backend/.env.example` as a reference for environment variables.
