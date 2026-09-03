# syntax=docker/dockerfile:1

# ---- build stage -------------------------------------------------------------
FROM golang:1.26 AS build
WORKDIR /src

# Dependency layer (cached until go.mod/go.sum change).
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

# Source.
COPY cmd ./cmd
COPY internal ./internal

# Build every command in ./cmd into /out. CGO is not used anywhere in this module.
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    set -eux; \
    mkdir /out; \
    for d in ./cmd/*/; do \
      name=$(basename "$d"); \
      CGO_ENABLED=0 GOOS=linux go build -buildvcs=false -trimpath \
        -ldflags="-s -w" -o "/out/$name" "$d"; \
    done

# ---- runtime stage ---------------------------------------------------------
FROM gcr.io/distroless/base-debian12:nonroot

# All command binaries live in /app and are on PATH, so a compose service just
# sets `command: ["api"]` / `["chat-worker"]` / `["migrate"]` etc. Docker's
# exec-form command resolution finds them via PATH; distroless has no shell.
COPY --from=build /out/ /app/
# Migrations travel with the image so `migrate` can run as an init job.
COPY migrations /app/migrations

WORKDIR /app
ENV PATH=/app:/usr/local/bin:/usr/bin:/bin
USER nonroot:nonroot

CMD ["api"]
