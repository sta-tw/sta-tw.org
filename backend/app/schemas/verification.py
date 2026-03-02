from __future__ import annotations

from pydantic import BaseModel


class UploadUrlResponse(BaseModel):
    uploadUrl: str | None  # None = use direct upload endpoint
    fileKey: str
    localUpload: bool = False  # True when R2 is not configured


class SubmitVerificationRequest(BaseModel):
    fileKeys: list[str]       # all uploaded file keys
    docType: str = "enrollment_proof"  # "student_id" | "enrollment_proof"


class VerificationUserInfo(BaseModel):
    id: str
    username: str
    displayName: str
    email: str
    role: str
    verificationStatus: str


class VerificationRequestOut(BaseModel):
    id: str
    userId: str
    user: VerificationUserInfo | None = None
    status: str
    docType: str | None = None
    fileKeys: list[str] = []
    fileUrls: list[str | None] = []
    fileHash: str | None = None  # legacy single-file field
    adminNote: str | None = None
    submittedAt: str
    reviewedAt: str | None = None


class ReviewVerificationRequest(BaseModel):
    approved: bool
    adminNote: str | None = None
