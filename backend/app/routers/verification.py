"""Verification router — /api/v1/verification/*"""
import os
import uuid

from fastapi import APIRouter, File, HTTPException, UploadFile
from fastapi.responses import FileResponse

from app.config import settings
from app.deps import CurrentUser, DB
from app.models.user import UserRole
from app.schemas.verification import (
    ReviewVerificationRequest,
    SubmitVerificationRequest,
    UploadUrlResponse,
    VerificationRequestOut,
)
from app.services import verification_service

router = APIRouter()


@router.get("/upload-url", response_model=UploadUrlResponse)
async def get_upload_url(user: CurrentUser):
    """Generate a presigned R2 upload URL. Returns uploadUrl=null + localUpload=true in dev if R2 is not configured."""
    return verification_service.get_upload_url(str(user.id))


@router.post("/upload-local")
async def upload_local(user: CurrentUser, file: UploadFile = File(...)):
    """Dev-mode direct upload endpoint (used when R2 is not configured)."""
    os.makedirs(settings.local_upload_dir, exist_ok=True)
    ext = os.path.splitext(file.filename or "")[1] or ".bin"
    filename = f"{uuid.uuid4()}{ext}"
    filepath = os.path.join(settings.local_upload_dir, filename)
    content = await file.read()
    with open(filepath, "wb") as f:
        f.write(content)
    file_key = f"local/{filename}"
    return {"fileKey": file_key}


@router.get("/local/{filename}")
async def serve_local_file(filename: str):
    """Serve a locally-uploaded verification file (dev mode; UUID names provide obscurity)."""
    # Prevent path traversal
    if "/" in filename or "\\" in filename or ".." in filename:
        raise HTTPException(status_code=400, detail="Invalid filename")
    filepath = os.path.join(settings.local_upload_dir, filename)
    if not os.path.isfile(filepath):
        raise HTTPException(status_code=404, detail="File not found")
    return FileResponse(filepath)


@router.post("", response_model=VerificationRequestOut, status_code=201)
async def submit(req: SubmitVerificationRequest, user: CurrentUser, db: DB):
    """Submit a verification request after uploading the document(s)."""
    return await verification_service.submit_request(
        str(user.id), req.fileKeys, req.docType, db
    )


@router.get("/status", response_model=VerificationRequestOut | None)
async def get_status(user: CurrentUser, db: DB):
    """Get the most recent verification request for the current user."""
    return await verification_service.get_status(str(user.id), db)


# ── Admin endpoints ───────────────────────────────────────────

@router.get("/admin/queue", response_model=list[VerificationRequestOut])
async def list_pending(user: CurrentUser, db: DB):
    """List all pending verification requests (admin/moderator only)."""
    if user.role not in (UserRole.admin, UserRole.developer):
        raise HTTPException(status_code=403, detail="Insufficient role")
    return await verification_service.list_pending(db)


@router.patch("/admin/{request_id}", response_model=VerificationRequestOut)
async def review(
    request_id: str,
    req: ReviewVerificationRequest,
    user: CurrentUser,
    db: DB,
):
    """Approve or reject a verification request (admin/developer only — involves personal info)."""
    if user.role not in (UserRole.admin, UserRole.developer):
        raise HTTPException(status_code=403, detail="Insufficient role")
    return await verification_service.review_request(
        request_id, str(user.id), req.approved, req.adminNote, db
    )
