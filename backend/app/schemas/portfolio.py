from __future__ import annotations

from pydantic import BaseModel, Field


class PortfolioUploadUrlRequest(BaseModel):
    fileName: str
    fileSize: int
    contentType: str = "application/pdf"


class PortfolioUploadUrlResponse(BaseModel):
    uploadUrl: str | None
    fileKey: str


class PortfolioCreateRequest(BaseModel):
    title: str | None = Field(None, max_length=200)
    description: str | None = None
    schoolName: str = Field(..., min_length=1, max_length=120)
    deptName: str = Field(..., min_length=1, max_length=120)
    admissionYear: int = Field(..., ge=100, le=200)   # 民國年，例如 116
    applicantName: str = Field(..., min_length=1, max_length=100)
    fileKey: str
    fileName: str
    fileSize: int


class PortfolioDocumentOut(BaseModel):
    id: str
    uploaderId: str
    uploaderName: str
    title: str
    description: str | None
    schoolName: str
    deptName: str
    admissionYear: int
    applicantName: str | None
    fileName: str
    fileSize: int
    isApproved: bool
    viewCount: int
    longViewCount: int
    createdAt: str
    updatedAt: str
    downloadUrl: str | None = None
    recommendationScore: float = 0.0


class PortfolioApproveRequest(BaseModel):
    approved: bool


# ── 校系選單 ──────────────────────────────────────────────────

class SchoolOptionOut(BaseModel):
    id: str
    schoolName: str
    schoolCode: str | None = None
    deptName: str
    sortOrder: int


class SchoolOptionCreate(BaseModel):
    schoolName: str = Field(..., min_length=1, max_length=120)
    schoolCode: str | None = Field(None, max_length=30)
    deptName: str = Field(..., min_length=1, max_length=120)
    sortOrder: int = 0


# ── 學校/科系申請 ──────────────────────────────────────────────

class SchoolRequestCreate(BaseModel):
    schoolName: str = Field(..., min_length=1, max_length=120)
    deptName: str = Field(..., min_length=1, max_length=120)
    note: str | None = None


class SchoolRequestOut(BaseModel):
    id: str
    requesterId: str
    requesterName: str
    schoolName: str
    deptName: str
    status: str   # pending / approved / rejected
    note: str | None
    reviewNote: str | None
    createdAt: str


class SchoolRequestReview(BaseModel):
    status: str   # approved / rejected
    reviewNote: str | None = None
    addToOptions: bool = True  # 若 approved，是否自動加入選單


# ── 校系評分規則 ──────────────────────────────────────────────

class PortfolioScoringRuleOut(BaseModel):
    id: str
    schoolName: str
    schoolAbbr: str | None
    deptName: str
    score: float
    note: str | None
    createdAt: str
    updatedAt: str


class PortfolioScoringRuleCreate(BaseModel):
    schoolName: str = Field(..., min_length=1, max_length=120)
    schoolAbbr: str | None = Field(None, max_length=30)
    deptName: str = Field(..., min_length=1, max_length=120)
    score: float = Field(..., ge=0.0)
    note: str | None = None


class PortfolioScoringRuleUpdate(BaseModel):
    schoolName: str | None = Field(None, min_length=1, max_length=120)
    schoolAbbr: str | None = Field(None, max_length=30)
    deptName: str | None = Field(None, min_length=1, max_length=120)
    score: float | None = Field(None, ge=0.0)
    note: str | None = None


class PortfolioRuleImportRow(BaseModel):
    schoolName: str = Field(..., min_length=1, max_length=120)
    schoolAbbr: str | None = Field(None, max_length=30)
    deptName: str = Field(..., min_length=1, max_length=120)
    score: float = Field(..., ge=0)
    note: str | None = None


class PortfolioRuleImportRequest(BaseModel):
    rows: list[PortfolioRuleImportRow]


class PortfolioRuleImportResult(BaseModel):
    created: int
    updated: int
    errors: list[str]
