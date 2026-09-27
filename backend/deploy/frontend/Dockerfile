# Build context is the monorepo root (see ../docker-compose.yml). Publishes
# into a mounted volume instead of baking into an image, so a frontend-only
# deploy never has to restart the caddy container that serves it.
FROM node:20-alpine
WORKDIR /app
RUN corepack enable
COPY frontend/package.json frontend/pnpm-lock.yaml frontend/pnpm-workspace.yaml ./
RUN pnpm install --frozen-lockfile
COPY frontend/ .
ARG NEXT_PUBLIC_API_BASE_URL=
ENV NEXT_PUBLIC_API_BASE_URL=$NEXT_PUBLIC_API_BASE_URL
RUN pnpm build

# Alternates between two release dirs and atomically swaps the `current`
# symlink (mv -T is an atomic rename on the same filesystem), so whatever is
# reading through .../current always sees a fully-old or fully-new tree,
# never a partial copy. The symlink target is relative (just "a"/"b", not
# "/out/a") since this volume is mounted at a different path in the caddy
# container (/srv/frontend) — an absolute target here would resolve to a
# path that doesn't exist there.
#
# After the swap, purge Cloudflare's edge cache so the "Cache Html Page"
# rule's 2-hour TTL doesn't leave stale content live after a deploy. Skipped
# entirely if the token/zone id aren't set (e.g. local/dev use).
CMD ["sh", "-c", "\
  target=a; [ \"$(readlink /out/current)\" = a ] && target=b; \
  rm -rf \"/out/$target\" && mkdir -p \"/out/$target\" && cp -r /app/out/. \"/out/$target/\" && \
  ln -sfn \"$target\" /out/current.tmp && mv -Tf /out/current.tmp /out/current && \
  if [ -n \"$STA_CLOUDFLARE_API_TOKEN\" ] && [ -n \"$STA_CLOUDFLARE_ZONE_ID\" ]; then \
    wget -q -O - --header=\"Authorization: Bearer $STA_CLOUDFLARE_API_TOKEN\" --header=\"Content-Type: application/json\" \
      --post-data='{\"purge_everything\":true}' \
      \"https://api.cloudflare.com/client/v4/zones/$STA_CLOUDFLARE_ZONE_ID/purge_cache\"; \
    echo; \
  else \
    echo 'STA_CLOUDFLARE_API_TOKEN/STA_CLOUDFLARE_ZONE_ID not set, skipping Cloudflare cache purge'; \
  fi \
"]
