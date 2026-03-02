"""Verification request business logic."""
from __future__ import annotations

import json
import os
import uuid
from datetime import datetime, timezone

from fastapi import HTTPException
from sqlalchemy import select
from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy.orm import selectinload

from app.config import settings
from app.models.user import User, UserRole, VerificationStatus
from app.models.verification import RequestStatus, VerificationRequest
from app.schemas.verification import VerificationRequestOut, VerificationUserInfo
from app.utils import storage


def _get_file_keys(req: VerificationRequest) -> list[str]:
    """Return all file keys for a request (new multi-file or legacy single-file)."""
    if req.file_keys:
        return json.loads(req.file_keys)
    if req.file_key:
        return [req.file_key]
    return []


def _get_file_url(file_key: str) -> str | None:
    """Return a viewable URL for a file key."""
    if file_key.startswith("local/"):
        return f"/api/v1/verification/local/{file_key[6:]}"
    return storage.generate_presigned_get_url(file_key, expiry=3600)


def _req_to_out(req: VerificationRequest) -> VerificationRequestOut:
    """Minimal conversion for user-facing endpoints."""
    keys = _get_file_keys(req)
    urls = [_get_file_url(k) for k in keys]
    return VerificationRequestOut(
        id=req.id,
        userId=req.user_id,
        status=req.status.value,
        docType=req.doc_type,
        fileKeys=keys,
        fileUrls=urls,
        fileHash=req.file_hash,
        adminNote=req.admin_note,
        submittedAt=req.submitted_at.isoformat(),
        reviewedAt=req.reviewed_at.isoformat() if req.reviewed_at else None,
    )


def _req_to_out_admin(req: VerificationRequest) -> VerificationRequestOut:
    """Richer conversion for admin endpoints — requires req.user to be loaded."""
    u = req.user
    user_info = VerificationUserInfo(
        id=str(u.id),
        username=u.username,
        displayName=u.display_name,
        email=u.email,
        role=u.role.value,
        verificationStatus=u.verification_status.value,
    ) if u else None

    keys = _get_file_keys(req)
    urls = [_get_file_url(k) for k in keys]

    return VerificationRequestOut(
        id=req.id,
        userId=req.user_id,
        user=user_info,
        status=req.status.value,
        docType=req.doc_type,
        fileKeys=keys,
        fileUrls=urls,
        fileHash=req.file_hash,
        adminNote=req.admin_note,
        submittedAt=req.submitted_at.isoformat(),
        reviewedAt=req.reviewed_at.isoformat() if req.reviewed_at else None,
    )


def get_upload_url(user_id: str) -> dict:
    """Generate a presigned PUT URL, or signal local upload if R2 is unavailable."""
    file_key = f"verification/{user_id}/{uuid.uuid4()}.pdf"
    upload_url = storage.generate_presigned_upload_url(file_key)
    local_upload = upload_url is None
    if local_upload:
        file_key = f"local/placeholder-{uuid.uuid4()}.bin"
    return {"uploadUrl": upload_url, "fileKey": file_key, "localUpload": local_upload}


async def submit_request(
    user_id: str,
    file_keys: list[str],
    doc_type: str,
    db: AsyncSession,
) -> VerificationRequestOut:
    user = await db.get(User, user_id)
    if user and user.verification_status == VerificationStatus.approved:
        raise HTTPException(status_code=400, detail="Already verified")

    # Cancel any previous pending request
    result = await db.execute(
        select(VerificationRequest).where(
            VerificationRequest.user_id == user_id,
            VerificationRequest.status == RequestStatus.pending,
        )
    )
    for old in result.scalars().all():
        old.status = RequestStatus.rejected

    req = VerificationRequest(
        user_id=user_id,
        file_keys=json.dumps(file_keys),
        doc_type=doc_type,
    )
    db.add(req)

    if user:
        user.verification_status = VerificationStatus.pending

    await db.commit()
    await db.refresh(req)
    return _req_to_out(req)


async def get_status(user_id: str, db: AsyncSession) -> VerificationRequestOut | None:
    result = await db.execute(
        select(VerificationRequest)
        .where(VerificationRequest.user_id == user_id)
        .order_by(VerificationRequest.submitted_at.desc())
        .limit(1)
    )
    req = result.scalar_one_or_none()
    return _req_to_out(req) if req else None


async def list_pending(db: AsyncSession) -> list[VerificationRequestOut]:
    result = await db.execute(
        select(VerificationRequest)
        .where(VerificationRequest.status == RequestStatus.pending)
        .options(selectinload(VerificationRequest.user))
        .order_by(VerificationRequest.submitted_at.asc())
    )
    return [_req_to_out_admin(r) for r in result.scalars().all()]


async def review_request(
    request_id: str,
    reviewer_id: str,
    approved: bool,
    admin_note: str | None,
    db: AsyncSession,
) -> VerificationRequestOut:
    req = await db.get(VerificationRequest, request_id)
    if not req:
        raise HTTPException(status_code=404, detail="Verification request not found")
    if req.status != RequestStatus.pending:
        raise HTTPException(status_code=400, detail="Request is not pending")

    req.status = RequestStatus.approved if approved else RequestStatus.rejected
    req.reviewed_at = datetime.now(timezone.utc)
    req.reviewed_by_id = reviewer_id
    req.admin_note = admin_note

    user = await db.get(User, req.user_id)
    if user:
        if approved:
            user.verification_status = VerificationStatus.approved
            user.role = UserRole.active_student
            # Delete all uploaded files
            for key in _get_file_keys(req):
                if key.startswith("local/"):
                    _delete_local_file(key)
                else:
                    storage.delete_object(key)
            req.file_keys = None
            req.file_key = None
        else:
            user.verification_status = VerificationStatus.rejected

    await db.commit()

    result = await db.execute(
        select(VerificationRequest)
        .where(VerificationRequest.id == request_id)
        .options(selectinload(VerificationRequest.user))
    )
    req = result.scalar_one()
    return _req_to_out_admin(req)


def _delete_local_file(file_key: str) -> None:
    """Delete a local dev-mode file (best-effort)."""
    filename = file_key[6:]  # strip "local/"
    filepath = os.path.join(settings.local_upload_dir, filename)
    try:
        if os.path.isfile(filepath):
            os.remove(filepath)
    except OSError:
        pass
