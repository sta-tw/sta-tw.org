# Build context is the monorepo root (see ../docker-compose.yml).
#
# Frontend static output no longer lives in this image — it's built by the
# frontend-build service into a shared volume mounted at /srv/frontend, so a
# frontend-only deploy never needs this container to restart. See
# ../frontend/Dockerfile and the caddy service's volumes in docker-compose.yml.
FROM caddy:2-alpine
COPY backend/deploy/caddy/Caddyfile /etc/caddy/Caddyfile
