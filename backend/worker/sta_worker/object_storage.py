from __future__ import annotations

from contextlib import contextmanager
import os
from pathlib import Path
import tempfile
from typing import Iterator

from .contracts import BrochureExtractJob, InvalidJob, RetryableProcessingError
from .processor import safe_document_path


class ObjectStorageClient:
    def __init__(self, endpoint: str, access_key: str, secret_key: str, bucket: str, secure: bool) -> None:
        try:
            from minio import Minio
        except ImportError as exc:
            raise RuntimeError("minio is required when STA_WORKER_DOCUMENT_ROOT is not configured") from exc
        if not endpoint or not access_key or not secret_key or not bucket:
            raise RuntimeError("worker object storage credentials are incomplete")
        self.bucket = bucket
        self.client = Minio(endpoint, access_key=access_key, secret_key=secret_key, secure=secure)

    def download(self, job: BrochureExtractJob, destination: Path, max_bytes: int) -> None:
        path = safe_document_path(destination, job.storage_key)
        path.parent.mkdir(parents=True, exist_ok=True)
        response = None
        try:
            response = self.client.get_object(self.bucket, job.storage_key)
            total = 0
            with path.open("wb") as target:
                while True:
                    chunk = response.read(1024 * 1024)
                    if not chunk:
                        break
                    total += len(chunk)
                    if total > max_bytes:
                        raise RetryableProcessingError("document exceeds worker size limit")
                    target.write(chunk)
        except (InvalidJob, RetryableProcessingError):
            raise
        except Exception as exc:
            raise RetryableProcessingError("document could not be downloaded") from exc
        finally:
            if response is not None:
                response.close()
                response.release_conn()


def object_storage_from_env() -> ObjectStorageClient:
    endpoint = os.environ.get("STA_WORKER_OBJECT_STORAGE_ENDPOINT", "").strip()
    access_key = os.environ.get("STA_WORKER_OBJECT_STORAGE_ACCESS_KEY", "").strip()
    secret_key = os.environ.get("STA_WORKER_OBJECT_STORAGE_SECRET_KEY", "").strip()
    bucket = os.environ.get("STA_WORKER_OBJECT_STORAGE_BUCKET", "sta-private").strip()
    secure = os.environ.get("STA_WORKER_OBJECT_STORAGE_USE_SSL", "false").strip().lower() == "true"
    return ObjectStorageClient(endpoint, access_key, secret_key, bucket, secure)


@contextmanager
def download_document(
    job: BrochureExtractJob,
    root: Path | None,
    object_storage: ObjectStorageClient | None,
    max_bytes: int,
) -> Iterator[Path]:
    if root is not None:
        yield root
        return
    if object_storage is None:
        raise RetryableProcessingError("worker document storage is not configured")
    with tempfile.TemporaryDirectory(prefix="sta-brochure-") as directory:
        processing_root = Path(directory)
        object_storage.download(job, processing_root, max_bytes)
        yield processing_root
