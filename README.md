# STA

Monorepo for the STA backend and frontend.

## Structure
- `backend/`: API server and services
- `frontend/`: Next.js web app
- `api-docs.md`: API reference

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
