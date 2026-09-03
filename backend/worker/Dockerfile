# syntax=docker/dockerfile:1
#
# Build context is the worker/ directory:
#   docker build -f worker/Dockerfile -t sta-worker ./worker
FROM python:3.13-slim

ENV PYTHONUNBUFFERED=1 \
    PYTHONDONTWRITEBYTECODE=1

WORKDIR /app

COPY requirements.txt ./
RUN pip install --no-cache-dir -r requirements.txt

# The package is imported as `worker.sta_worker.*`, so it must sit under /app/worker.
COPY __init__.py /app/worker/__init__.py
COPY sta_worker /app/worker/sta_worker

RUN useradd --system --uid 10001 sta && chown -R sta /app
USER sta

# Override with `command: ["worker.sta_worker.discovery"]` for the discovery agent.
ENTRYPOINT ["python", "-m"]
CMD ["worker.sta_worker.main"]
