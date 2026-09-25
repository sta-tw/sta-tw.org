# Build context is the monorepo root (see ../docker-compose.yml).
FROM node:20-alpine AS frontend-build
WORKDIR /app
RUN corepack enable
COPY frontend/package.json frontend/pnpm-lock.yaml frontend/pnpm-workspace.yaml ./
RUN pnpm install --frozen-lockfile
COPY frontend/ .
ARG NEXT_PUBLIC_API_BASE_URL=
ENV NEXT_PUBLIC_API_BASE_URL=$NEXT_PUBLIC_API_BASE_URL
RUN pnpm build

FROM caddy:2-alpine
COPY backend/deploy/caddy/Caddyfile /etc/caddy/Caddyfile
COPY --from=frontend-build /app/out /srv/frontend
