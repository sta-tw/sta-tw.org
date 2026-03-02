"""Admission PDF router — /api/v1/admissions/*"""
from __future__ import annotations

from fastapi import APIRouter, File, Form, HTTPException, Query, UploadFile
from fastapi.responses import Response
from jose import JWTError
from sqlalchemy import select

from app.deps import CurrentUser, DB
from app.models.admission import AdmissionDocument
from app.models.user import User, UserRole
from app.schemas.admission import AdmissionDocumentOut, ImportAdmissionByUrlRequest
from app.services import admission_service
from app.utils.security import decode_token

router = APIRouter()

_MANAGE_ROLES = (UserRole.dept_moderator, UserRole.school_moderator, UserRole.admin, UserRole.developer)


def _check_scope(user: User, school_code: str | None) -> None:
    """校系板主/校板主只能管理自己 managed_school_code 範圍內的簡章。"""
    if user.role in (UserRole.admin, UserRole.developer):
        return
    if user.role == UserRole.school_moderator:
        if not user.managed_school_code:
            raise HTTPException(status_code=403, detail="尚未設定管理學校")
        if school_code and user.managed_school_code != school_code:
            raise HTTPException(status_code=403, detail="不在您的管理範圍")
        return
    if user.role == UserRole.dept_moderator:
        if not user.managed_school_code:
            raise HTTPException(status_code=403, detail="尚未設定管理學校")
        if school_code and user.managed_school_code != school_code:
            raise HTTPException(status_code=403, detail="不在您的管理範圍")
        return
    raise HTTPException(status_code=403, detail="Insufficient role")


@router.get("", response_model=list[AdmissionDocumentOut])
async def list_admissions(_user: CurrentUser, db: DB):
    return await admission_service.list_documents(db)


@router.get("/{document_id}", response_model=AdmissionDocumentOut)
async def get_admission(document_id: str, _user: CurrentUser, db: DB):
    doc = await admission_service.get_document(document_id, db)
    if doc is None:
        raise HTTPException(status_code=404, detail="Admission document not found")
    return doc


@router.post("/import-url", response_model=AdmissionDocumentOut, status_code=201)
async def import_by_url(req: ImportAdmissionByUrlRequest, user: CurrentUser, db: DB):
    if user.role not in _MANAGE_ROLES:
        raise HTTPException(status_code=403, detail="Insufficient role")
    _check_scope(user, req.school_code)
    return await admission_service.import_from_url(str(req.url), str(user.id), req.school_code, db)


@router.post("/import-file", response_model=AdmissionDocumentOut, status_code=201)
async def import_by_file(
    user: CurrentUser,
    db: DB,
    file: UploadFile = File(...),
    school_code: str | None = Form(default=None),
):
    if user.role not in _MANAGE_ROLES:
        raise HTTPException(status_code=403, detail="Insufficient role")
    if file.content_type not in ("application/pdf",) and not (file.filename or "").lower().endswith(".pdf"):
        raise HTTPException(status_code=422, detail="只允許上傳 PDF 檔案")
    _check_scope(user, school_code)
    content = await file.read()
    return await admission_service.import_from_bytes(content, str(user.id), school_code, db)


@router.delete("/{document_id}", status_code=204)
async def delete_admission(document_id: str, user: CurrentUser, db: DB):
    if user.role not in _MANAGE_ROLES:
        raise HTTPException(status_code=403, detail="Insufficient role")
    result = await db.execute(select(AdmissionDocument).where(AdmissionDocument.id == document_id))
    doc = result.scalar_one_or_none()
    if doc is None:
        raise HTTPException(status_code=404, detail="Admission document not found")
    _check_scope(user, doc.school_code)
    await db.delete(doc)
    await db.commit()


@router.get("/{document_id}/pdf")
async def view_admission_pdf(document_id: str, db: DB, token: str = Query(...)):
    try:
        payload = decode_token(token, "access")
        user_id = payload.get("sub")
    except JWTError as exc:
        raise HTTPException(status_code=401, detail="Could not validate credentials") from exc

    if not user_id:
        raise HTTPException(status_code=401, detail="Could not validate credentials")

    user_result = await db.execute(select(User).where(User.id == user_id, User.is_active == True))  # noqa: E712
    user = user_result.scalar_one_or_none()
    if user is None:
        raise HTTPException(status_code=401, detail="Could not validate credentials")

    try:
        pdf_bytes = await admission_service.fetch_document_pdf(document_id, db)
    except Exception as exc:
        raise HTTPException(status_code=502, detail=f"Failed to fetch source PDF: {exc}") from exc

    if pdf_bytes is None:
        raise HTTPException(status_code=404, detail="Admission document not found")

    return Response(
        content=pdf_bytes,
        media_type="application/pdf",
        headers={"Content-Disposition": "inline; filename=admission.pdf"},
    )
