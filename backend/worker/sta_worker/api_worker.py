"""HTTP transport for the local extraction worker.

The Go API owns uploads, private storage, job leases, and result persistence.
This process only claims a job, downloads the source through the short-lived
URL returned by the API, performs deterministic extraction, and posts the
validated result back. No AI SDK, model endpoint, database, or object-storage
credential is needed in this mode.
"""

from __future__ import annotations

import hashlib
import json
import logging
import os
from pathlib import Path
import tempfile
import urllib.error
import urllib.parse
import urllib.request
from typing import Any

from .candidate_list import extract_candidate_list
from .contracts import BrochureExtractJob, InvalidJob, RetryableProcessingError
from .processor import extract_document, safe_document_path
from .runtime import configure_logging, install_shutdown


LOGGER = logging.getLogger("sta-api-worker")
BROCHURE_SOURCE = "brochure"
CANDIDATE_LIST_SOURCE = "candidate_list"
DEFAULT_PROCESSOR_VERSION = "local-extraction-v1"
DEFAULT_MAX_FILE_BYTES = 50 * 1024 * 1024


class ExtractionAPIError(RuntimeError):
    def __init__(self, message: str, status: int | None = None) -> None:
        super().__init__(message)
        self.status = status

    @property
    def retryable(self) -> bool:
        return self.status is None or self.status == 429 or self.status >= 500


def _required_env(name: str, *aliases: str) -> str:
    for key in (name, *aliases):
        value = os.environ.get(key, "").strip()
        if value:
            return value
    raise RuntimeError(f"{name} is required")


def _positive_int(name: str, default: int) -> int:
    raw = os.environ.get(name, str(default)).strip()
    try:
        value = int(raw)
    except ValueError as exc:
        raise RuntimeError(f"{name} must be a positive integer") from exc
    if value <= 0:
        raise RuntimeError(f"{name} must be a positive integer")
    return value


def _positive_duration(name: str, default: float) -> float:
    raw = os.environ.get(name, "").strip().lower()
    if not raw:
        return default
    multiplier = 1.0
    if raw[-1:] in {"s", "m", "h"}:
        multiplier = {"s": 1.0, "m": 60.0, "h": 3600.0}[raw[-1]]
        raw = raw[:-1]
    try:
        value = float(raw) * multiplier
    except ValueError as exc:
        raise RuntimeError(f"{name} is not a valid duration") from exc
    if value <= 0:
        raise RuntimeError(f"{name} must be positive")
    return value


class ExtractionAPIClient:
    def __init__(self, base_url: str, token: str, timeout: float = 60.0) -> None:
        value = base_url.strip().rstrip("/")
        parsed = urllib.parse.urlsplit(value)
        if parsed.scheme not in {"http", "https"} or not parsed.hostname or parsed.username or parsed.password or parsed.query or parsed.fragment:
            raise RuntimeError("STA_EXTRACTION_API_BASE_URL must be an absolute HTTP(S) URL without credentials, query, or fragment")
        if len(token.strip()) < 32:
            raise RuntimeError("STA_EXTRACTION_SERVICE_TOKEN must contain at least 32 characters")
        if timeout <= 0:
            raise RuntimeError("STA_EXTRACTION_API_TIMEOUT must be positive")
        self.base_url = value
        self.token = token.strip()
        self.timeout = timeout

    def _request(
        self,
        path: str,
        method: str = "POST",
        payload: dict[str, Any] | None = None,
        traceparent: str | None = None,
    ) -> tuple[int, bytes]:
        body = None if payload is None else json.dumps(payload, ensure_ascii=False, separators=(",", ":")).encode("utf-8")
        headers = {"Authorization": f"Bearer {self.token}", "Accept": "application/json"}
        if body is not None:
            headers["Content-Type"] = "application/json"
        if traceparent:
            # Forward the trace context so the API logs the callback under the
            # same trace_id as the request that created the job.
            headers["traceparent"] = traceparent
        request = urllib.request.Request(self.base_url + path, data=body, method=method, headers=headers)
        try:
            with urllib.request.urlopen(request, timeout=self.timeout) as response:
                return response.status, response.read(16 * 1024 * 1024)
        except urllib.error.HTTPError as exc:
            try:
                exc.read(16 * 1024)
            except OSError:
                pass
            raise ExtractionAPIError(f"extraction API returned HTTP {exc.code}", exc.code) from exc
        except (urllib.error.URLError, TimeoutError, OSError) as exc:
            raise ExtractionAPIError("extraction API request failed") from exc

    def claim(self, source_type: str) -> dict[str, Any] | None:
        status, body = self._request(
            "/api/v1/internal/extraction/jobs/claim",
            payload={"document_type": source_type},
        )
        if status == 204 or not body:
            return None
        try:
            data = json.loads(body.decode("utf-8"))["data"]
        except (UnicodeDecodeError, json.JSONDecodeError, KeyError, TypeError) as exc:
            raise ExtractionAPIError("extraction API claim response is invalid") from exc
        if not isinstance(data, dict) or not isinstance(data.get("job"), dict) or not isinstance(data.get("download_url"), str):
            raise ExtractionAPIError("extraction API claim response is invalid")
        return data

    def download(self, download_url: str, destination: Path, expected_sha256: str, max_bytes: int) -> None:
        parsed = urllib.parse.urlsplit(download_url)
        if parsed.scheme not in {"http", "https"} or not parsed.hostname or parsed.username or parsed.password:
            raise InvalidJob("download URL is invalid")
        request = urllib.request.Request(download_url, headers={"Accept": "application/pdf,text/plain,application/json,text/csv,*/*"})
        digest = hashlib.sha256()
        total = 0
        destination.parent.mkdir(parents=True, exist_ok=True)
        try:
            with urllib.request.urlopen(request, timeout=self.timeout) as response, destination.open("wb") as target:
                while True:
                    chunk = response.read(1024 * 1024)
                    if not chunk:
                        break
                    total += len(chunk)
                    if total > max_bytes:
                        raise RetryableProcessingError("source exceeds worker size limit")
                    digest.update(chunk)
                    target.write(chunk)
        except (InvalidJob, RetryableProcessingError):
            raise
        except urllib.error.HTTPError as exc:
            raise RetryableProcessingError(f"source download returned HTTP {exc.code}") from exc
        except (urllib.error.URLError, TimeoutError, OSError) as exc:
            raise RetryableProcessingError("source download failed") from exc
        if total <= 0 or digest.hexdigest() != expected_sha256:
            raise InvalidJob("source checksum does not match job")

    def submit_result(self, job_id: str, result: dict[str, Any], traceparent: str | None = None) -> None:
        self._request(
            f"/api/v1/internal/extraction/jobs/{urllib.parse.quote(job_id, safe='')}/result",
            payload=result,
            traceparent=traceparent,
        )

    def report_failure(self, job_id: str, error: Exception, retryable: bool, traceparent: str | None = None) -> None:
        code = "processing_failed" if retryable else "invalid_source"
        self._request(
            f"/api/v1/internal/extraction/jobs/{urllib.parse.quote(job_id, safe='')}/failure",
            payload={"code": code, "message": str(error)[:500], "retryable": retryable},
            traceparent=traceparent,
        )


def _trace_id(traceparent: str | None) -> str:
    """W3C traceparent is "00-<trace>-<span>-<flags>"; return <trace> for logs."""
    if not traceparent:
        return ""
    parts = traceparent.split("-")
    return parts[1] if len(parts) == 4 else ""


def _process_claim(
    client: ExtractionAPIClient,
    claim: dict[str, Any],
    processor_version: str,
    max_bytes: int,
    traceparent: str | None = None,
) -> None:
    job = BrochureExtractJob.from_mapping(claim["job"])
    download_url = str(claim["download_url"])
    with tempfile.TemporaryDirectory(prefix="sta-api-extraction-") as directory:
        root = Path(directory)
        path = safe_document_path(root, job.storage_key)
        client.download(download_url, path, job.sha256_hex, max_bytes)
        if job.source_type == CANDIDATE_LIST_SOURCE:
            result = extract_candidate_list(job, path, processor_version, max_bytes)
        else:
            result = extract_document(job, root, processor_version, max_bytes, logger=LOGGER)
    client.submit_result(job.job_id, result, traceparent=traceparent)


def run() -> None:
    client = ExtractionAPIClient(
        os.environ.get("STA_EXTRACTION_API_BASE_URL", "http://localhost:8080"),
        _required_env("STA_EXTRACTION_SERVICE_TOKEN", "STA_EXTERNAL_INGESTION_TOKEN"),
        _positive_duration("STA_EXTRACTION_API_TIMEOUT", 60.0),
    )
    processor_version = os.environ.get("STA_WORKER_PROCESSOR_VERSION", DEFAULT_PROCESSOR_VERSION).strip() or DEFAULT_PROCESSOR_VERSION
    max_bytes = _positive_int("STA_WORKER_MAX_FILE_BYTES", DEFAULT_MAX_FILE_BYTES)
    poll_interval = _positive_duration("STA_EXTRACTION_POLL_INTERVAL", 5.0)
    stop = install_shutdown()
    LOGGER.info("STA HTTP local extraction worker started")
    while not stop.is_set():
        claimed = False
        for source_type in (BROCHURE_SOURCE, CANDIDATE_LIST_SOURCE):
            if stop.is_set():
                break
            try:
                claim = client.claim(source_type)
            except ExtractionAPIError as exc:
                LOGGER.warning("extraction API claim failed: %s", exc)
                stop.wait(poll_interval)
                break
            if claim is None:
                continue
            claimed = True
            job_id = str(claim.get("job", {}).get("job_id", ""))
            traceparent = str(claim.get("job", {}).get("traceparent", "")) or None
            log_extra = {"job_id": job_id, "source_type": source_type, "trace_id": _trace_id(traceparent)}
            try:
                _process_claim(client, claim, processor_version, max_bytes, traceparent=traceparent)
                LOGGER.info("extraction job completed", extra=log_extra)
            except InvalidJob as exc:
                LOGGER.warning("extraction job rejected", extra={**log_extra, "error": str(exc)})
                try:
                    client.report_failure(job_id, exc, False, traceparent=traceparent)
                except ExtractionAPIError:
                    LOGGER.exception("could not report invalid extraction job", extra=log_extra)
            except RetryableProcessingError as exc:
                LOGGER.warning("extraction job will retry", extra={**log_extra, "error": str(exc)})
                try:
                    client.report_failure(job_id, exc, True, traceparent=traceparent)
                except ExtractionAPIError:
                    LOGGER.exception("could not report retryable extraction failure", extra=log_extra)
            except ExtractionAPIError as exc:
                LOGGER.warning("extraction API result call failed", extra={**log_extra, "error": str(exc)})
                try:
                    client.report_failure(job_id, exc, exc.retryable, traceparent=traceparent)
                except ExtractionAPIError:
                    LOGGER.exception("could not report extraction API failure", extra=log_extra)
            except Exception as exc:
                LOGGER.exception("unexpected extraction job error", extra=log_extra)
                try:
                    client.report_failure(job_id, exc, True, traceparent=traceparent)
                except ExtractionAPIError:
                    LOGGER.exception("could not report unexpected extraction failure", extra=log_extra)
        if not claimed:
            stop.wait(poll_interval)
    LOGGER.info("STA HTTP local extraction worker stopped")


if __name__ == "__main__":
    configure_logging()
    run()
