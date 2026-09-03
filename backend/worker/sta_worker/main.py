from __future__ import annotations

import json
import logging
import os
import signal
from pathlib import Path
from typing import Any

from .contracts import BrochureExtractJob, InvalidJob, RetryableProcessingError
from .candidate_list import extract_candidate_list
from .object_storage import download_document, object_storage_from_env
from .processor import extract_document, safe_document_path
from .runtime import configure_logging


LOGGER = logging.getLogger("sta-worker")
EXCHANGE = "sta.events"
EXTRACT_QUEUE = "sta.admissions.extract"
RESULT_ROUTING_KEY = "admissions.brochure.extracted"
CANDIDATE_LIST_RESULT_ROUTING_KEY = "admissions.candidate-list.extracted"
RETRY_QUEUE = "sta.admissions.extract.retry"
CANDIDATE_LIST_RETRY_QUEUE = "sta.admissions.candidate-list.extract.retry"
MAX_RETRIES = 5
DEFAULT_PROCESSOR_VERSION = "local-extraction-v1"


def _required_env(name: str) -> str:
    value = os.environ.get(name, "").strip()
    if not value:
        raise RuntimeError(f"{name} is required")
    return value


def _max_file_bytes() -> int:
    raw = os.environ.get("STA_WORKER_MAX_FILE_BYTES", str(50 * 1024 * 1024))
    value = int(raw)
    if value <= 0:
        raise RuntimeError("STA_WORKER_MAX_FILE_BYTES must be positive")
    return value


def run() -> None:
    if os.environ.get("STA_EXTRACTION_TRANSPORT", "api").strip().lower() == "api":
        from .api_worker import run as run_api_worker

        run_api_worker()
        return
    try:
        import pika
    except ImportError as exc:
        raise RuntimeError("pika is required to run the worker") from exc

    rabbit_url = _required_env("STA_RABBITMQ_URL")
    document_root = os.environ.get("STA_WORKER_DOCUMENT_ROOT", "").strip()
    root = Path(document_root).resolve() if document_root else None
    object_storage = object_storage_from_env() if root is None else None
    processor_version = os.environ.get("STA_WORKER_PROCESSOR_VERSION", DEFAULT_PROCESSOR_VERSION).strip()
    if not processor_version:
        processor_version = DEFAULT_PROCESSOR_VERSION
    max_bytes = _max_file_bytes()
    LOGGER.info("STA local extraction rules enabled")
    parameters = pika.URLParameters(rabbit_url)
    connection = pika.BlockingConnection(parameters)
    channel = connection.channel()
    channel.exchange_declare(exchange=EXCHANGE, exchange_type="topic", durable=True)
    channel.exchange_declare(exchange=f"{EXCHANGE}.dlx", exchange_type="direct", durable=True)
    channel.queue_declare(queue=EXTRACT_QUEUE, durable=True, arguments={
        "x-dead-letter-exchange": f"{EXCHANGE}.dlx",
        "x-dead-letter-routing-key": EXTRACT_QUEUE,
    })
    channel.queue_bind(exchange=EXCHANGE, queue=EXTRACT_QUEUE, routing_key="admissions.brochure.extract")
    channel.queue_bind(exchange=EXCHANGE, queue=EXTRACT_QUEUE, routing_key="admissions.candidate-list.extract")
    channel.queue_declare(queue=f"{EXTRACT_QUEUE}.dead", durable=True)
    channel.queue_bind(exchange=f"{EXCHANGE}.dlx", queue=f"{EXTRACT_QUEUE}.dead", routing_key=EXTRACT_QUEUE)
    channel.queue_declare(queue=RETRY_QUEUE, durable=True, arguments={
        "x-message-ttl": 5000,
        "x-dead-letter-exchange": EXCHANGE,
        "x-dead-letter-routing-key": "admissions.brochure.extract",
    })
    channel.queue_declare(queue=CANDIDATE_LIST_RETRY_QUEUE, durable=True, arguments={
        "x-message-ttl": 5000,
        "x-dead-letter-exchange": EXCHANGE,
        "x-dead-letter-routing-key": "admissions.candidate-list.extract",
    })
    channel.basic_qos(prefetch_count=1)

    def callback(channel, method, properties, body) -> None:
        job = None
        try:
            payload: Any = json.loads(body.decode("utf-8"))
            job = BrochureExtractJob.from_mapping(payload)
            with download_document(job, root, object_storage, max_bytes) as processing_root:
                if job.source_type == "candidate_list":
                    result = extract_candidate_list(
                        job,
                        safe_document_path(processing_root, job.storage_key),
                        processor_version,
                        max_bytes,
                    )
                    result_routing_key = CANDIDATE_LIST_RESULT_ROUTING_KEY
                else:
                    result = extract_document(job, processing_root, processor_version, max_bytes, logger=LOGGER)
                    result_routing_key = RESULT_ROUTING_KEY
            result_body = json.dumps(result, separators=(",", ":")).encode("utf-8")
            channel.basic_publish(
                exchange=EXCHANGE,
                routing_key=result_routing_key,
                body=result_body,
                properties=pika.BasicProperties(
                    content_type="application/json",
                    delivery_mode=2,
                    message_id=job.job_id,
                ),
            )
            channel.basic_ack(delivery_tag=method.delivery_tag)
        except (InvalidJob, json.JSONDecodeError) as exc:
            LOGGER.warning("rejecting invalid extraction job: %s", exc)
            channel.basic_nack(delivery_tag=method.delivery_tag, requeue=False)
        except RetryableProcessingError as exc:
            headers = dict(getattr(properties, "headers", None) or {})
            try:
                retry_count = int(headers.get("x-sta-retry-count", 0))
            except (TypeError, ValueError):
                retry_count = MAX_RETRIES
            if retry_count >= MAX_RETRIES:
                LOGGER.error("dead-lettering extraction job after retries: %s", exc)
                channel.basic_nack(delivery_tag=method.delivery_tag, requeue=False)
            else:
                LOGGER.warning("retrying extraction job (%d/%d): %s", retry_count + 1, MAX_RETRIES, exc)
                headers["x-sta-retry-count"] = retry_count + 1
                retry_queue = CANDIDATE_LIST_RETRY_QUEUE if job is not None and job.source_type == "candidate_list" else RETRY_QUEUE
                channel.basic_publish(
                    exchange="",
                    routing_key=retry_queue,
                    body=body,
                    properties=pika.BasicProperties(content_type="application/json", delivery_mode=2, headers=headers),
                )
                channel.basic_ack(delivery_tag=method.delivery_tag)
        except Exception:
            LOGGER.exception("unexpected extraction worker error")
            channel.basic_nack(delivery_tag=method.delivery_tag, requeue=False)

    channel.basic_consume(queue=EXTRACT_QUEUE, on_message_callback=callback, auto_ack=False)

    def _stop(signum, _frame) -> None:
        LOGGER.info("shutdown signal received", extra={"signal": signal.Signals(signum).name})
        # start_consuming() runs on this thread; hop back onto the connection's
        # I/O loop to stop it after the in-flight message is acked.
        connection.add_callback_threadsafe(channel.stop_consuming)

    for sig in (signal.SIGTERM, signal.SIGINT):
        signal.signal(sig, _stop)

    LOGGER.info("STA local extraction worker started")
    try:
        channel.start_consuming()
    finally:
        if connection.is_open:
            connection.close()
    LOGGER.info("STA local extraction worker stopped")


if __name__ == "__main__":
    configure_logging()
    run()
