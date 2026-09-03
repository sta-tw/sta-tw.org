# Deploying the STA backend (single host, docker compose)

`docker-compose.yml` runs the whole backend on one machine: PostgreSQL, MinIO,
optionally RabbitMQ, ClamAV, MailHog (local SMTP capture), SearXNG, the Go API,
the Go workers and the two Python workers. It is meant for local development and
small staging hosts — production should use managed datastores, a secret
manager and real TLS termination in front of the API (see `../docs/deployment.md`
and `../docs/security.md`).

## Quick start

```sh
cd deploy
cp .env.example .env          # then edit secrets/keys
docker compose up -d --build  # HTTP extraction transport (no RabbitMQ)
```

`migrate` runs automatically as an init dependency of `api`, applying
`migrations/` (including the detached Telegram adapter, `-include-telegram`).
`api` waits for Postgres to be healthy, the MinIO bucket to exist and `migrate`
to succeed before it starts.

Check it:

```sh
curl -fsS localhost:8080/healthz          # liveness
curl -fsS localhost:8080/readyz           # {"status":"ready","checks":{...}}
curl -fsS localhost:8080/api/v1/meta
curl -fsS localhost:8080/metrics | grep sta_http
```

`/readyz` reports a per-dependency `checks` object (database, and — when
configured — object_storage, clamav, broker) and returns 503 until all pass.
ClamAV's first signature download takes a few minutes; its `start_period` is
180s. If you are not using file scanning, set `STA_CLAMAV_ADDRESS=` and
`STA_REQUIRE_FILE_SCAN=false` and remove the `clamav` service.

## Profiles

| Profile | Adds | Use when |
|---|---|---|
| _(default)_ | api, chat/notification/support workers, python-worker, discovery-worker | HTTP extraction transport |
| `rabbitmq` | `rabbitmq`, `ingestion-worker`, sets `STA_RABBITMQ_URL` on the worker | you want the RabbitMQ compatibility transport; also set `STA_EXTRACTION_TRANSPORT=rabbitmq` and `STA_WORKER_OBJECT_STORAGE_*` in `.env` |
| `maintenance` | `annual-maintenance` (one-shot) | running the yearly June cleanup |

```sh
docker compose --profile rabbitmq up -d --build
```

## One-shot commands

```sh
# apply migrations manually (also runs on `up`)
docker compose run --rm migrate

# grant admin to an already-registered account (never expose this publicly)
docker compose run --rm api bootstrap-admin -username <existing-username>

# yearly cleanup for a specific 3-digit ROC academic year
docker compose --profile maintenance run --rm annual-maintenance -academic-year 114
```

## Ports

All host bindings are on `127.0.0.1` and overridable via `STA_COMPOSE_*` in
`.env` (change them if a port is already taken):

| Service | Default host port |
|---|---|
| api | 8080 |
| postgres | 5432 |
| minio API / console | 9000 / 9001 |
| rabbitmq AMQP / management | 5672 / 15672 |
| mailhog UI | 8025 |
| searxng | 8888 |

## Images

- `Dockerfile` (repo root) builds one distroless image containing every `cmd/*`
  binary plus `migrations/`. The compose `command:` selects the process
  (`["api"]`, `["chat-worker"]`, `["migrate", …]`, …); binaries are on `PATH`.
- `worker/Dockerfile` builds the Python worker image; `command:` selects
  `worker.sta_worker.main` or `worker.sta_worker.discovery`.

## Shutdown

`api` and the Go workers handle SIGTERM and drain in-flight work within
`STA_SHUTDOWN_TIMEOUT` (compose gives them a 20s `stop_grace_period`). The
Python workers stop at their next loop boundary. `docker compose down` is clean;
add `-v` to also drop the Postgres/MinIO/RabbitMQ/ClamAV volumes.
