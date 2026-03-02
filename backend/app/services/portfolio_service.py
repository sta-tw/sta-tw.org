from __future__ import annotations

import uuid
from datetime import datetime, timezone

from sqlalchemy import select, update
from sqlalchemy.ext.asyncio import AsyncSession

from app.models.audit import AuditLog
from app.models.portfolio import PortfolioDocument
from app.models.portfolio_scoring import PortfolioScoringRule, PortfolioViewLog
from app.models.portfolio_school import SchoolOption, SchoolRequest
from app.models.user import User
from app.schemas.portfolio import (
    PortfolioCreateRequest,
    PortfolioDocumentOut,
    PortfolioRuleImportResult,
    PortfolioRuleImportRow,
    PortfolioScoringRuleCreate,
    PortfolioScoringRuleOut,
    PortfolioScoringRuleUpdate,
    SchoolOptionCreate,
    SchoolOptionOut,
    SchoolRequestCreate,
    SchoolRequestOut,
    SchoolRequestReview,
)
from app.utils.storage import generate_presigned_upload_url, generate_presigned_get_url


# ── 推薦評分計算 ───────────────────────────────────────────────


def _current_cohort() -> int:
    """當前民國年（屆數），每年動態計算。"""
    return datetime.now().year - 1911


def _cohort_score(admission_year: int, current: int) -> float:
    """
    依屆數距離計算基礎分數。

    years_back │ 分數
    ───────────┼─────
        0      │  3  （應屆）
       1–3     │  5
       4–5     │  4
       6–8     │  3
      9–11     │  2
     12–16     │  1
      17+      │  0
    """
    years_back = current - admission_year
    if years_back == 0:
        return 3.0
    elif 1 <= years_back <= 3:
        return 5.0
    elif 4 <= years_back <= 5:
        return 4.0
    elif 6 <= years_back <= 8:
        return 3.0
    elif 9 <= years_back <= 11:
        return 2.0
    elif 12 <= years_back <= 16:
        return 1.0
    else:
        return 0.0


def _view_score(view_count: int, long_view_count: int) -> float:
    """點閱率加分：每次瀏覽 +0.1，長觀看（>30分鐘）每人 +0.5（僅一次）。"""
    return round(view_count * 0.1 + long_view_count * 0.5, 2)


async def _get_dept_rule_score(
    school_name: str, dept_name: str, db: AsyncSession
) -> float:
    """查詢校系評分規則，以 school_name + dept_name 為鍵。若無規則則回傳 0.0。"""
    result = await db.execute(
        select(PortfolioScoringRule).where(
            PortfolioScoringRule.school_name == school_name,
            PortfolioScoringRule.dept_name == dept_name,
        )
    )
    rule = result.scalar_one_or_none()
    return rule.score if rule else 0.0


# ── 輔助轉換 ──────────────────────────────────────────────────


def _to_out(
    doc: PortfolioDocument,
    uploader_name: str,
    recommendation_score: float,
    download_url: str | None = None,
) -> PortfolioDocumentOut:
    return PortfolioDocumentOut(
        id=doc.id,
        uploaderId=doc.uploader_id,
        uploaderName=uploader_name,
        title=doc.title or f"{doc.school_name} {doc.dept_name}",
        description=doc.description,
        schoolName=doc.school_name,
        deptName=doc.dept_name,
        admissionYear=doc.admission_year,
        applicantName=doc.applicant_name,
        fileName=doc.file_name,
        fileSize=doc.file_size,
        isApproved=doc.is_approved,
        viewCount=doc.view_count,
        longViewCount=doc.long_view_count,
        createdAt=doc.created_at.isoformat(),
        updatedAt=doc.updated_at.isoformat(),
        downloadUrl=download_url,
        recommendationScore=recommendation_score,
    )


# ── Service functions ─────────────────────────────────────────


async def request_upload_url(
    file_name: str, file_size: int, content_type: str, uploader_id: str
) -> tuple[str | None, str]:
    file_key = f"portfolio/{uploader_id}/{uuid.uuid4()}_{file_name}"
    upload_url = generate_presigned_upload_url(file_key, content_type)
    return upload_url, file_key


async def create_document(
    req: PortfolioCreateRequest, uploader_id: str, db: AsyncSession
) -> PortfolioDocumentOut:
    # Auto-generate title if not provided
    auto_title = req.title or f"{req.schoolName} {req.deptName} {req.applicantName}"
    doc = PortfolioDocument(
        id=str(uuid.uuid4()),
        uploader_id=uploader_id,
        title=auto_title,
        description=req.description,
        school_name=req.schoolName,
        dept_name=req.deptName,
        admission_year=req.admissionYear,
        applicant_name=req.applicantName,
        category=None,
        file_key=req.fileKey,
        file_name=req.fileName,
        file_size=req.fileSize,
        is_approved=False,
    )
    db.add(doc)
    await db.commit()
    await db.refresh(doc)

    user_result = await db.execute(select(User).where(User.id == uploader_id))
    uploader = user_result.scalar_one_or_none()
    uploader_name = uploader.display_name if uploader else "Unknown"

    current = _current_cohort()
    dept_score = await _get_dept_rule_score(doc.school_name, doc.dept_name, db)
    score = _cohort_score(doc.admission_year, current) + dept_score + _view_score(doc.view_count, doc.long_view_count)
    return _to_out(doc, uploader_name, score)


async def list_documents(
    db: AsyncSession,
    school_name: str | None = None,
    dept_name: str | None = None,
    admission_year: int | None = None,
    category: str | None = None,
    approved_only: bool = True,
) -> list[PortfolioDocumentOut]:
    q = select(PortfolioDocument)
    if approved_only:
        q = q.where(PortfolioDocument.is_approved == True)  # noqa: E712
    if school_name:
        q = q.where(PortfolioDocument.school_name.ilike(f"%{school_name}%"))
    if dept_name:
        q = q.where(PortfolioDocument.dept_name.ilike(f"%{dept_name}%"))
    if admission_year:
        q = q.where(PortfolioDocument.admission_year == admission_year)
    if category:
        q = q.where(PortfolioDocument.category == category)

    result = await db.execute(q)
    docs = list(result.scalars().all())

    uploader_ids = list({d.uploader_id for d in docs})
    users_result = await db.execute(select(User).where(User.id.in_(uploader_ids)))
    users_map = {u.id: u.display_name for u in users_result.scalars().all()}

    current = _current_cohort()
    out = []
    for doc in docs:
        dept_score = await _get_dept_rule_score(doc.school_name, doc.dept_name, db)
        score = _cohort_score(doc.admission_year, current) + dept_score + _view_score(doc.view_count, doc.long_view_count)
        out.append(_to_out(doc, users_map.get(doc.uploader_id, "Unknown"), score))

    # 依推薦分數由高到低排序
    out.sort(key=lambda x: x.recommendationScore, reverse=True)
    return out


async def get_document(
    doc_id: str, db: AsyncSession, with_download_url: bool = False, viewer_id: str | None = None
) -> PortfolioDocumentOut | None:
    result = await db.execute(select(PortfolioDocument).where(PortfolioDocument.id == doc_id))
    doc = result.scalar_one_or_none()
    if doc is None:
        return None

    if with_download_url:
        await db.execute(
            update(PortfolioDocument)
            .where(PortfolioDocument.id == doc_id)
            .values(view_count=PortfolioDocument.view_count + 1)
        )
        await db.commit()
        await db.refresh(doc)
        download_url = generate_presigned_get_url(doc.file_key)
    else:
        download_url = None

    user_result = await db.execute(select(User).where(User.id == doc.uploader_id))
    uploader = user_result.scalar_one_or_none()
    uploader_name = uploader.display_name if uploader else "Unknown"

    current = _current_cohort()
    dept_score = await _get_dept_rule_score(doc.school_name, doc.dept_name, db)
    score = _cohort_score(doc.admission_year, current) + dept_score + _view_score(doc.view_count, doc.long_view_count)
    return _to_out(doc, uploader_name, score, download_url)


async def record_long_view(doc_id: str, user_id: str, db: AsyncSession) -> bool:
    """
    記錄使用者長觀看（>30分鐘）。每位使用者每份文件只計一次 +0.5。
    回傳 True 表示成功新增，False 表示已經計過了。
    """
    # 查詢是否已有記錄
    result = await db.execute(
        select(PortfolioViewLog).where(
            PortfolioViewLog.doc_id == doc_id,
            PortfolioViewLog.user_id == user_id,
        )
    )
    log = result.scalar_one_or_none()

    if log and log.long_view_granted:
        return False  # 已計過，不重複加分

    if log:
        log.long_view_granted = True
    else:
        db.add(PortfolioViewLog(
            id=str(uuid.uuid4()),
            doc_id=doc_id,
            user_id=user_id,
            long_view_granted=True,
        ))

    # 增加 long_view_count
    await db.execute(
        update(PortfolioDocument)
        .where(PortfolioDocument.id == doc_id)
        .values(long_view_count=PortfolioDocument.long_view_count + 1)
    )
    await db.commit()
    return True


async def _award_reputation(
    user_id: str, delta: int, reason: str, doc_id: str, db: AsyncSession
) -> None:
    """給指定使用者增加信譽分，並建立 AuditLog。不自行 commit。"""
    user_result = await db.execute(select(User).where(User.id == user_id))
    user = user_result.scalar_one_or_none()
    if user is None:
        return
    user.reputation_score = (user.reputation_score or 0) + delta
    db.add(AuditLog(
        id=str(uuid.uuid4()),
        correlation_id=str(uuid.uuid4()),
        actor_id=None,
        action="reputation.auto_awarded",
        target_type="user",
        target_id=user_id,
        ip="system",
        metadata_={"delta": delta, "reason": reason, "doc_id": doc_id},
    ))


async def record_share_view(doc_id: str, viewer_id: str, db: AsyncSession) -> bool:
    """
    記錄分享連結被開啟。每位觀看者每份文件限一次，觸發上傳者 +20 信譽分。
    回傳 True 表示成功發放，False 表示已發放過或無效。
    """
    # 取 doc，驗證已審核
    doc_result = await db.execute(select(PortfolioDocument).where(PortfolioDocument.id == doc_id))
    doc = doc_result.scalar_one_or_none()
    if doc is None or not doc.is_approved:
        return False

    # 防自刷
    if doc.uploader_id == viewer_id:
        return False

    # upsert PortfolioViewLog
    log_result = await db.execute(
        select(PortfolioViewLog).where(
            PortfolioViewLog.doc_id == doc_id,
            PortfolioViewLog.user_id == viewer_id,
        )
    )
    log = log_result.scalar_one_or_none()

    if log is None:
        log = PortfolioViewLog(
            id=str(uuid.uuid4()),
            doc_id=doc_id,
            user_id=viewer_id,
        )
        db.add(log)

    if log.share_reward_granted:
        return False

    log.share_reward_granted = True
    await _award_reputation(doc.uploader_id, 20, "share_view", doc_id, db)
    await db.commit()
    return True


async def record_heartbeat(doc_id: str, viewer_id: str, db: AsyncSession) -> None:
    """
    連續閱讀心跳。10 分鐘寬限期後，每累積 30 分鐘有效閱讀 → 上傳者 +10。
    """
    SESSION_BREAK = 120    # 秒：超過此間隔視為新 session
    GRACE = 600            # 秒：寬限期 10 分鐘
    REWARD_INTERVAL = 1800 # 秒：每 30 分鐘發放一次
    CAP = 45               # 秒：單次心跳最多計算 45 秒

    now = datetime.now(timezone.utc)

    # 取 doc，驗證已審核
    doc_result = await db.execute(select(PortfolioDocument).where(PortfolioDocument.id == doc_id))
    doc = doc_result.scalar_one_or_none()
    if doc is None or not doc.is_approved:
        return

    # upsert PortfolioViewLog with FOR UPDATE
    log_result = await db.execute(
        select(PortfolioViewLog)
        .where(
            PortfolioViewLog.doc_id == doc_id,
            PortfolioViewLog.user_id == viewer_id,
        )
        .with_for_update()
    )
    log = log_result.scalar_one_or_none()

    if log is None:
        log = PortfolioViewLog(
            id=str(uuid.uuid4()),
            doc_id=doc_id,
            user_id=viewer_id,
            last_heartbeat_at=now,
            session_grace_remaining_s=GRACE,
        )
        db.add(log)
        await db.commit()
        return

    # 計算時間差
    if log.last_heartbeat_at is None:
        log.last_heartbeat_at = now
        await db.commit()
        return

    gap = (now - log.last_heartbeat_at).total_seconds()

    # 超過 SESSION_BREAK → 重置 session grace
    if gap > SESSION_BREAK:
        log.session_grace_remaining_s = GRACE

    elapsed = min(gap, CAP)
    remaining_before = log.session_grace_remaining_s
    grace_consumed = min(elapsed, remaining_before)
    log.session_grace_remaining_s = remaining_before - grace_consumed

    if remaining_before == grace_consumed:
        # grace 剛好耗盡或原本已是 0
        effective_this_tick = elapsed - grace_consumed
    else:
        effective_this_tick = 0

    log.total_effective_seconds += int(effective_this_tick)
    log.last_heartbeat_at = now

    # 計算應發放的獎勵 interval 數
    if doc.uploader_id != viewer_id:
        new_intervals = log.total_effective_seconds // REWARD_INTERVAL
        if new_intervals > log.reputation_intervals_granted:
            delta = (new_intervals - log.reputation_intervals_granted) * 10
            log.reputation_intervals_granted = new_intervals
            await _award_reputation(doc.uploader_id, delta, "reading_heartbeat", doc_id, db)

    await db.commit()


async def delete_document(doc_id: str, user_id: str, db: AsyncSession, is_staff: bool = False) -> bool:
    result = await db.execute(select(PortfolioDocument).where(PortfolioDocument.id == doc_id))
    doc = result.scalar_one_or_none()
    if doc is None:
        return False
    if doc.uploader_id != user_id and not is_staff:
        return False
    await db.delete(doc)
    await db.commit()
    return True


async def set_approval(doc_id: str, approved: bool, db: AsyncSession) -> PortfolioDocumentOut | None:
    result = await db.execute(select(PortfolioDocument).where(PortfolioDocument.id == doc_id))
    doc = result.scalar_one_or_none()
    if doc is None:
        return None
    doc.is_approved = approved
    await db.commit()
    await db.refresh(doc)

    user_result = await db.execute(select(User).where(User.id == doc.uploader_id))
    uploader = user_result.scalar_one_or_none()
    uploader_name = uploader.display_name if uploader else "Unknown"

    current = _current_cohort()
    dept_score = await _get_dept_rule_score(doc.school_name, doc.dept_name, db)
    score = _cohort_score(doc.admission_year, current) + dept_score + _view_score(doc.view_count, doc.long_view_count)
    return _to_out(doc, uploader_name, score)


# ── 校系評分規則 CRUD ─────────────────────────────────────────

def _rule_to_out(rule: PortfolioScoringRule) -> PortfolioScoringRuleOut:
    return PortfolioScoringRuleOut(
        id=rule.id,
        schoolName=rule.school_name,
        schoolAbbr=rule.school_abbr,
        deptName=rule.dept_name,
        score=rule.score,
        note=rule.note,
        createdAt=rule.created_at.isoformat(),
        updatedAt=rule.updated_at.isoformat(),
    )


async def list_scoring_rules(db: AsyncSession) -> list[PortfolioScoringRuleOut]:
    result = await db.execute(
        select(PortfolioScoringRule).order_by(
            PortfolioScoringRule.school_name,
            PortfolioScoringRule.dept_name,
        )
    )
    return [_rule_to_out(r) for r in result.scalars().all()]


async def create_scoring_rule(req: PortfolioScoringRuleCreate, db: AsyncSession) -> PortfolioScoringRuleOut:
    rule = PortfolioScoringRule(
        id=str(uuid.uuid4()),
        school_name=req.schoolName,
        school_abbr=req.schoolAbbr or None,
        dept_name=req.deptName,
        score=req.score,
        note=req.note,
    )
    db.add(rule)
    await db.commit()
    await db.refresh(rule)
    return _rule_to_out(rule)


async def update_scoring_rule(
    rule_id: str, req: PortfolioScoringRuleUpdate, db: AsyncSession
) -> PortfolioScoringRuleOut | None:
    result = await db.execute(select(PortfolioScoringRule).where(PortfolioScoringRule.id == rule_id))
    rule = result.scalar_one_or_none()
    if rule is None:
        return None
    if req.schoolName is not None:
        rule.school_name = req.schoolName
    if req.schoolAbbr is not None:
        rule.school_abbr = req.schoolAbbr or None
    if req.deptName is not None:
        rule.dept_name = req.deptName
    if req.score is not None:
        rule.score = req.score
    if req.note is not None:
        rule.note = req.note
    await db.commit()
    await db.refresh(rule)
    return _rule_to_out(rule)


async def bulk_upsert_scoring_rules(
    rows: list[PortfolioRuleImportRow], db: AsyncSession
) -> PortfolioRuleImportResult:
    created = 0
    updated = 0
    errors: list[str] = []

    for i, row in enumerate(rows):
        try:
            stmt = select(PortfolioScoringRule).where(
                PortfolioScoringRule.school_name == row.schoolName,
                PortfolioScoringRule.dept_name == row.deptName,
            )
            result = await db.execute(stmt)
            existing = result.scalar_one_or_none()
            if existing:
                existing.school_abbr = row.schoolAbbr or None
                existing.score = row.score
                existing.note = row.note
                updated += 1
            else:
                db.add(PortfolioScoringRule(
                    id=str(uuid.uuid4()),
                    school_name=row.schoolName,
                    school_abbr=row.schoolAbbr or None,
                    dept_name=row.deptName,
                    score=row.score,
                    note=row.note,
                ))
                created += 1
        except Exception as e:
            errors.append(f"第 {i + 1} 筆：{e}")

    if created > 0 or updated > 0:
        await db.commit()

    return PortfolioRuleImportResult(created=created, updated=updated, errors=errors)


async def delete_scoring_rule(rule_id: str, db: AsyncSession) -> bool:
    result = await db.execute(select(PortfolioScoringRule).where(PortfolioScoringRule.id == rule_id))
    rule = result.scalar_one_or_none()
    if rule is None:
        return False
    await db.delete(rule)
    await db.commit()
    return True


# ── 校系選單 ──────────────────────────────────────────────────


async def list_schools(db: AsyncSession) -> list[str]:
    """回傳所有啟用中的學校名稱（去重、排序）。"""
    from sqlalchemy import distinct
    result = await db.execute(
        select(distinct(SchoolOption.school_name))
        .where(SchoolOption.is_active == True)  # noqa: E712
        .order_by(SchoolOption.school_name)
    )
    return list(result.scalars().all())


async def list_depts(school_name: str, db: AsyncSession) -> list[str]:
    """回傳指定學校下所有啟用中的科系名稱。"""
    result = await db.execute(
        select(SchoolOption.dept_name)
        .where(SchoolOption.school_name == school_name, SchoolOption.is_active == True)  # noqa: E712
        .order_by(SchoolOption.sort_order, SchoolOption.dept_name)
    )
    return list(result.scalars().all())


async def list_school_options(db: AsyncSession) -> list[SchoolOptionOut]:
    result = await db.execute(
        select(SchoolOption)
        .where(SchoolOption.is_active == True)  # noqa: E712
        .order_by(SchoolOption.sort_order, SchoolOption.school_name, SchoolOption.dept_name)
    )
    return [
        SchoolOptionOut(
            id=opt.id,
            schoolName=opt.school_name,
            schoolCode=opt.school_code,
            deptName=opt.dept_name,
            sortOrder=opt.sort_order,
        )
        for opt in result.scalars().all()
    ]


async def create_school_option(req: SchoolOptionCreate, db: AsyncSession) -> SchoolOptionOut:
    opt = SchoolOption(
        id=str(uuid.uuid4()),
        school_name=req.schoolName,
        school_code=req.schoolCode,
        dept_name=req.deptName,
        sort_order=req.sortOrder,
    )
    db.add(opt)
    await db.commit()
    await db.refresh(opt)
    return SchoolOptionOut(
        id=opt.id,
        schoolName=opt.school_name,
        schoolCode=opt.school_code,
        deptName=opt.dept_name,
        sortOrder=opt.sort_order,
    )


async def delete_school_option(option_id: str, db: AsyncSession) -> bool:
    opt = await db.get(SchoolOption, option_id)
    if opt is None:
        return False
    await db.delete(opt)
    await db.commit()
    return True


# ── 學校/科系申請 ──────────────────────────────────────────────


def _request_to_out(req: SchoolRequest, requester_name: str) -> SchoolRequestOut:
    return SchoolRequestOut(
        id=req.id,
        requesterId=req.requester_id,
        requesterName=requester_name,
        schoolName=req.school_name,
        deptName=req.dept_name,
        status=req.status,
        note=req.note,
        reviewNote=req.review_note,
        createdAt=req.created_at.isoformat(),
    )


async def create_school_request(
    req: SchoolRequestCreate, requester_id: str, db: AsyncSession
) -> SchoolRequestOut:
    sr = SchoolRequest(
        id=str(uuid.uuid4()),
        requester_id=requester_id,
        school_name=req.schoolName,
        dept_name=req.deptName,
        note=req.note,
        status="pending",
    )
    db.add(sr)
    await db.commit()
    await db.refresh(sr)

    user_result = await db.execute(select(User).where(User.id == requester_id))
    user = user_result.scalar_one_or_none()
    return _request_to_out(sr, user.display_name if user else "Unknown")


async def list_school_requests(
    db: AsyncSession, status: str | None = None
) -> list[SchoolRequestOut]:
    stmt = select(SchoolRequest).order_by(SchoolRequest.created_at.desc())
    if status:
        stmt = stmt.where(SchoolRequest.status == status)
    result = await db.execute(stmt)
    requests = list(result.scalars().all())

    requester_ids = list({r.requester_id for r in requests})
    users_result = await db.execute(select(User).where(User.id.in_(requester_ids)))
    users_map = {u.id: u.display_name for u in users_result.scalars().all()}

    return [_request_to_out(r, users_map.get(r.requester_id, "Unknown")) for r in requests]


async def review_school_request(
    request_id: str, review: SchoolRequestReview, reviewer_id: str, db: AsyncSession
) -> SchoolRequestOut | None:
    result = await db.execute(select(SchoolRequest).where(SchoolRequest.id == request_id))
    sr = result.scalar_one_or_none()
    if sr is None:
        return None

    sr.status = review.status
    sr.review_note = review.reviewNote
    sr.reviewed_by = reviewer_id

    # 若核准且勾選「加入選單」
    if review.status == "approved" and review.addToOptions:
        # 確認不重複新增
        exists_result = await db.execute(
            select(SchoolOption).where(
                SchoolOption.school_name == sr.school_name,
                SchoolOption.dept_name == sr.dept_name,
            )
        )
        if not exists_result.scalar_one_or_none():
            db.add(SchoolOption(
                id=str(uuid.uuid4()),
                school_name=sr.school_name,
                dept_name=sr.dept_name,
            ))

    await db.commit()
    await db.refresh(sr)

    user_result = await db.execute(select(User).where(User.id == sr.requester_id))
    user = user_result.scalar_one_or_none()
    return _request_to_out(sr, user.display_name if user else "Unknown")
