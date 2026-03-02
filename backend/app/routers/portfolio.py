"""Portfolio documents router — /api/v1/portfolio/*"""
from __future__ import annotations

from fastapi import APIRouter, HTTPException, Query

from app.deps import CurrentUser, DB
from app.models.user import UserRole
from app.schemas.portfolio import (
    PortfolioApproveRequest,
    PortfolioCreateRequest,
    PortfolioDocumentOut,
    PortfolioUploadUrlRequest,
    PortfolioUploadUrlResponse,
    SchoolRequestCreate,
    SchoolRequestOut,
)
from app.services import portfolio_service

router = APIRouter()

_UPLOAD_ROLES = (UserRole.senior, UserRole.admin, UserRole.developer)
_STAFF_ROLES = (UserRole.admin, UserRole.developer)


@router.get("/schools", response_model=list[str])
async def list_schools(_user: CurrentUser, db: DB):
    """回傳所有可用的學校名稱清單（用於上傳表單下拉選單）。"""
    return await portfolio_service.list_schools(db)


@router.get("/schools/{school_name}/depts", response_model=list[str])
async def list_depts(school_name: str, _user: CurrentUser, db: DB):
    """回傳指定學校下所有科系名稱。"""
    return await portfolio_service.list_depts(school_name, db)


@router.post("/school-request", response_model=SchoolRequestOut, status_code=201)
async def submit_school_request(req: SchoolRequestCreate, user: CurrentUser, db: DB):
    """使用者申請新增學校/科系，送交管理員審核。"""
    return await portfolio_service.create_school_request(req, str(user.id), db)


@router.get("", response_model=list[PortfolioDocumentOut])
async def list_portfolio(
    _user: CurrentUser,
    db: DB,
    school_name: str | None = Query(None),
    dept_name: str | None = Query(None),
    admission_year: int | None = Query(None),
    category: str | None = Query(None),
):
    return await portfolio_service.list_documents(
        db,
        school_name=school_name,
        dept_name=dept_name,
        admission_year=admission_year,
        category=category,
    )


@router.post("/upload-url", response_model=PortfolioUploadUrlResponse)
async def request_upload_url(req: PortfolioUploadUrlRequest, user: CurrentUser, db: DB):
    if user.role not in _UPLOAD_ROLES:
        raise HTTPException(status_code=403, detail="只有歷屆學姊（alumni）可以上傳備審資料")
    upload_url, file_key = await portfolio_service.request_upload_url(
        req.fileName, req.fileSize, req.contentType, str(user.id)
    )
    return PortfolioUploadUrlResponse(uploadUrl=upload_url, fileKey=file_key)


@router.post("", response_model=PortfolioDocumentOut, status_code=201)
async def create_portfolio_document(req: PortfolioCreateRequest, user: CurrentUser, db: DB):
    if user.role not in _UPLOAD_ROLES:
        raise HTTPException(status_code=403, detail="只有歷屆學姊（alumni）可以上傳備審資料")
    return await portfolio_service.create_document(req, str(user.id), db)


@router.get("/{doc_id}", response_model=PortfolioDocumentOut)
async def get_portfolio_document(doc_id: str, _user: CurrentUser, db: DB):
    doc = await portfolio_service.get_document(doc_id, db)
    if doc is None or not doc.isApproved:
        raise HTTPException(status_code=404, detail="Document not found")
    return doc


@router.get("/{doc_id}/download-url", response_model=PortfolioDocumentOut)
async def get_portfolio_download_url(doc_id: str, user: CurrentUser, db: DB):
    doc = await portfolio_service.get_document(doc_id, db, with_download_url=True, viewer_id=str(user.id))
    if doc is None or not doc.isApproved:
        raise HTTPException(status_code=404, detail="Document not found")
    return doc


@router.post("/{doc_id}/long-view", status_code=204)
async def record_long_view(doc_id: str, user: CurrentUser, db: DB):
    """
    前端在使用者開啟文件超過 30 分鐘後呼叫，給予 +0.5 點閱加分（每位使用者僅一次）。
    """
    doc = await portfolio_service.get_document(doc_id, db)
    if doc is None or not doc.isApproved:
        raise HTTPException(status_code=404, detail="Document not found")
    await portfolio_service.record_long_view(doc_id, str(user.id), db)


@router.post("/{doc_id}/share-view", status_code=204)
async def record_share_view(doc_id: str, user: CurrentUser, db: DB):
    """分享連結被觀看者開啟時呼叫，觸發上傳者 +20 信譽分（每位觀看者每份文件限一次）。"""
    await portfolio_service.record_share_view(doc_id, str(user.id), db)


@router.post("/{doc_id}/heartbeat", status_code=204)
async def record_heartbeat(doc_id: str, user: CurrentUser, db: DB):
    """閱讀心跳，每 30 秒由前端呼叫。超過 10 分鐘寬限後，每累積 30 分鐘有效閱讀 → 上傳者 +10。"""
    await portfolio_service.record_heartbeat(doc_id, str(user.id), db)


@router.delete("/{doc_id}", status_code=204)
async def delete_portfolio_document(doc_id: str, user: CurrentUser, db: DB):
    is_staff = user.role in _STAFF_ROLES
    deleted = await portfolio_service.delete_document(doc_id, str(user.id), db, is_staff=is_staff)
    if not deleted:
        raise HTTPException(status_code=404, detail="Document not found or insufficient permissions")


@router.patch("/{doc_id}/approve", response_model=PortfolioDocumentOut)
async def approve_portfolio_document(doc_id: str, req: PortfolioApproveRequest, user: CurrentUser, db: DB):
    if user.role not in _STAFF_ROLES:
        raise HTTPException(status_code=403, detail="Insufficient role")
    doc = await portfolio_service.set_approval(doc_id, req.approved, db)
    if doc is None:
        raise HTTPException(status_code=404, detail="Document not found")
    return doc
