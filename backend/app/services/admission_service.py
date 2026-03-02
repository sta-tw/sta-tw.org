from __future__ import annotations

import io
import re
from collections.abc import Iterable

import httpx
from pypdf import PdfReader
from sqlalchemy import select
from sqlalchemy.ext.asyncio import AsyncSession

from app.models.admission import AdmissionDocument
from app.schemas.admission import AdmissionDocumentOut, AdmissionKeyDateOut


_KEYWORDS: list[tuple[str, tuple[str, ...]]] = [
    ("網路報名", ("網路報名", "上傳報名資料")),
    ("甄試資格查詢", ("查詢甄試資格", "甄試資格審查結果公告")),
    ("考生應試公告", ("考生應試公告",)),
    ("錄取公告", ("錄取公告",)),
    ("正取生報到", ("正取生報到", "正取生驗證報到")),
    ("備取生遞補", ("備取生遞補", "備取生驗證報到")),
]

_DATE_PATTERN = re.compile(r"\d{3}/\d{1,2}/\d{1,2}(?:\s*\d{1,2}:\d{2})?(?:\s*[~～\-至]\s*\d{3}/\d{1,2}/\d{1,2}(?:\s*\d{1,2}:\d{2})?)?")


def _norm_space(text: str) -> str:
    return re.sub(r"[ \t]+", " ", text).strip()


def _extract_title(text: str) -> str | None:
    for line in text.splitlines():
        line = _norm_space(line)
        if not line:
            continue
        if "招生簡章" in line:
            return line[:300]
    return None


def _extract_school_name(text: str) -> str | None:
    m = re.search(r"((?:國立|私立)?[\u4e00-\u9fff]{2,16}大學)", text)
    return m.group(1) if m else None


def _extract_year(text: str) -> int | None:
    m = re.search(r"(\d{3})\s*學年度", text)
    return int(m.group(1)) if m else None


def _extract_key_dates(lines: Iterable[str]) -> list[dict]:
    found: list[dict] = []
    used_labels: set[str] = set()
    for raw in lines:
        line = _norm_space(raw)
        if not line:
            continue
        date_match = _DATE_PATTERN.search(line)
        if not date_match:
            continue
        for label, words in _KEYWORDS:
            if label in used_labels:
                continue
            if any(word in line for word in words):
                found.append({"label": label, "dateText": date_match.group(0)})
                used_labels.add(label)
                break
        if len(found) >= 10:
            break
    return found


def _to_out(doc: AdmissionDocument) -> AdmissionDocumentOut:
    return AdmissionDocumentOut(
        id=doc.id,
        sourceUrl=doc.source_url,
        title=doc.title,
        schoolName=doc.school_name,
        academicYear=doc.academic_year,
        pageCount=doc.page_count,
        textPreview=doc.text_preview,
        keyDates=[AdmissionKeyDateOut(label=item.get("label", ""), dateText=item.get("dateText", "")) for item in (doc.key_dates or [])],
        createdAt=doc.created_at.isoformat(),
        updatedAt=doc.updated_at.isoformat(),
    )


async def list_documents(db: AsyncSession) -> list[AdmissionDocumentOut]:
    result = await db.execute(select(AdmissionDocument).order_by(AdmissionDocument.created_at.desc()).limit(50))
    return [_to_out(d) for d in result.scalars().all()]


async def get_document(document_id: str, db: AsyncSession) -> AdmissionDocumentOut | None:
    result = await db.execute(select(AdmissionDocument).where(AdmissionDocument.id == document_id))
    doc = result.scalar_one_or_none()
    return _to_out(doc) if doc else None


async def fetch_document_pdf(document_id: str, db: AsyncSession) -> bytes | None:
    result = await db.execute(select(AdmissionDocument).where(AdmissionDocument.id == document_id))
    doc = result.scalar_one_or_none()
    if doc is None:
        return None

    # Uploaded file — bytes stored in DB
    if doc.pdf_content is not None:
        return doc.pdf_content

    # Fetched from external URL
    async with httpx.AsyncClient(timeout=60.0, follow_redirects=True) as client:
        resp = await client.get(doc.source_url)
        resp.raise_for_status()
        return resp.content


def _parse_pdf_bytes(content: bytes) -> tuple[str | None, str | None, int | None, str | None, list[dict], int]:
    """Parse PDF bytes → (title, school_name, year, preview, key_dates, page_count)."""
    reader = PdfReader(io.BytesIO(content))
    texts: list[str] = []
    for page in reader.pages:
        try:
            texts.append(page.extract_text() or "")
        except Exception:
            texts.append("")
    full_text = "\n".join(texts)
    return (
        _extract_title(full_text),
        _extract_school_name(full_text),
        _extract_year(full_text),
        _norm_space(full_text[:1200]) if full_text else None,
        _extract_key_dates(full_text.splitlines()),
        len(reader.pages),
    )


async def import_from_url(url: str, created_by_id: str, school_code: str | None, db: AsyncSession) -> AdmissionDocumentOut:
    async with httpx.AsyncClient(timeout=45.0, follow_redirects=True) as client:
        resp = await client.get(url)
        resp.raise_for_status()
        content = resp.content

    title, school_name, year, preview, key_dates, page_count = _parse_pdf_bytes(content)

    existing = await db.execute(select(AdmissionDocument).where(AdmissionDocument.source_url == url))
    doc = existing.scalar_one_or_none()

    if doc is None:
        doc = AdmissionDocument(source_url=url, created_by_id=created_by_id)
        db.add(doc)

    doc.title = title
    doc.school_name = school_name
    doc.school_code = school_code
    doc.academic_year = year
    doc.page_count = page_count
    doc.text_preview = preview
    doc.key_dates = key_dates

    await db.commit()
    await db.refresh(doc)
    return _to_out(doc)


async def import_from_bytes(content: bytes, created_by_id: str, school_code: str | None, db: AsyncSession) -> AdmissionDocumentOut:
    """Import a directly-uploaded PDF (no external URL)."""
    import uuid as _uuid
    title, school_name, year, preview, key_dates, page_count = _parse_pdf_bytes(content)

    synthetic_url = f"file:{_uuid.uuid4()}"
    doc = AdmissionDocument(
        source_url=synthetic_url,
        created_by_id=created_by_id,
        pdf_content=content,
        school_code=school_code,
        title=title,
        school_name=school_name,
        academic_year=year,
        page_count=page_count,
        text_preview=preview,
        key_dates=key_dates,
    )
    db.add(doc)
    await db.commit()
    await db.refresh(doc)
    return _to_out(doc)
