"""Cloudflare R2 storage via S3-compatible API."""
from __future__ import annotations

import logging

from app.config import settings

logger = logging.getLogger(__name__)


def _get_client():
    """Return a boto3 S3 client pointed at R2, or None if not configured."""
    if not settings.r2_access_key_id:
        return None
    try:
        import boto3  # lazy import so startup doesn't fail without boto3

        return boto3.client(
            "s3",
            endpoint_url=f"https://{settings.r2_account_id}.r2.cloudflarestorage.com",
            aws_access_key_id=settings.r2_access_key_id,
            aws_secret_access_key=settings.r2_secret_access_key,
            region_name="auto",
        )
    except Exception as exc:
        logger.warning("Failed to create R2 client: %s", exc)
        return None


def generate_presigned_upload_url(file_key: str, content_type: str = "application/pdf") -> str | None:
    """Return a presigned PUT URL valid for 15 minutes, or None if R2 is unavailable."""
    client = _get_client()
    if not client:
        return None
    try:
        return client.generate_presigned_url(
            "put_object",
            Params={
                "Bucket": settings.r2_bucket,
                "Key": file_key,
                "ContentType": content_type,
            },
            ExpiresIn=900,
        )
    except Exception as exc:
        logger.warning("generate_presigned_upload_url failed: %s", exc)
        return None


def generate_presigned_get_url(file_key: str, expiry: int = 3600) -> str | None:
    """Return a presigned GET URL valid for the given seconds, or None if R2 is unavailable."""
    client = _get_client()
    if not client:
        return None
    try:
        return client.generate_presigned_url(
            "get_object",
            Params={"Bucket": settings.r2_bucket, "Key": file_key},
            ExpiresIn=expiry,
        )
    except Exception as exc:
        logger.warning("generate_presigned_get_url failed: %s", exc)
        return None


def delete_object(file_key: str) -> None:
    """Delete an object from R2 (best-effort; silently skips if unconfigured)."""
    client = _get_client()
    if not client:
        return
    try:
        client.delete_object(Bucket=settings.r2_bucket, Key=file_key)
    except Exception as exc:
        logger.warning("R2 delete_object failed: %s", exc)
