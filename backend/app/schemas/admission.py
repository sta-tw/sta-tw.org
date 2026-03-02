from __future__ import annotations

from pydantic import BaseModel, HttpUrl


class ImportAdmissionByUrlRequest(BaseModel):
    url: HttpUrl
    school_code: str | None = None


class AdmissionKeyDateOut(BaseModel):
    label: str
    dateText: str


class AdmissionDocumentOut(BaseModel):
    id: str
    sourceUrl: str
    title: str | None = None
    schoolName: str | None = None
    academicYear: int | None = None
    pageCount: int
    textPreview: str | None = None
    keyDates: list[AdmissionKeyDateOut]
    createdAt: str
    updatedAt: str
