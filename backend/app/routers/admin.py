"""Admin router — /api/v1/admin/*  (admin / moderator only)"""
from __future__ import annotations

import csv
import io
import uuid
from datetime import datetime, timedelta, timezone

from fastapi import APIRouter, HTTPException, Query, Request
from fastapi.responses import StreamingResponse
from sqlalchemy import func, select
from app.deps import CurrentUser, DB, get_client_ip
from app.models.audit import AuditLog
from app.models.channel import Channel, ChannelScopeType, ChannelType
from app.models.message import Message
from app.models.ticket import Ticket, TicketStatus
from app.models.user import User, UserRole, UserSession
from app.models.verification import RequestStatus, VerificationRequest
from app.schemas.admin import (
    AdminUserListOut,
    AdminUserOut,
    AdjustReputationRequest,
    AuditLogListOut,
    AuditLogOut,
    CreateChannelRequest,
    ReputationDetailOut,
    ReputationEventOut,
    StatsOut,
    UpdateChannelRequest,
    UpdateUserRequest,
)
from app.schemas.channel import ChannelOut
from app.schemas.portfolio import (
    PortfolioRuleImportRequest,
    PortfolioRuleImportResult,
    PortfolioScoringRuleCreate,
    PortfolioScoringRuleOut,
    PortfolioScoringRuleUpdate,
    SchoolOptionCreate,
    SchoolOptionOut,
    SchoolRequestOut,
    SchoolRequestReview,
)
from app.services import portfolio_service

router = APIRouter()

_ADMIN_ROLES = (UserRole.admin, UserRole.developer)


def _require_staff(user: User) -> None:
    if user.role not in _ADMIN_ROLES:
        raise HTTPException(status_code=403, detail="Insufficient role")


def _user_out(u: User) -> AdminUserOut:
    return AdminUserOut(
        id=u.id,
        username=u.username,
        email=u.email,
        displayName=u.display_name,
        avatarUrl=u.avatar_url,
        role=u.role.value,
        verificationStatus=u.verification_status.value,
        reputationScore=u.reputation_score,
        isActive=u.is_active,
        isEmailVerified=u.is_email_verified,
        managedSchoolCode=u.managed_school_code,
        managedDeptName=u.managed_dept_name,
        createdAt=u.created_at.isoformat(),
    )


def _channel_out(ch: Channel) -> ChannelOut:
    return ChannelOut(
        id=ch.id,
        name=ch.name,
        description=ch.description,
        type=ch.type.value,
        scopeType=ch.scope_type.value,
        schoolCode=ch.school_code,
        deptCode=ch.dept_code,
        parentId=ch.parent_id,
        isArchived=ch.is_archived,
        cohortYear=ch.cohort_year,
        audience=ch.audience,
        unreadCount=0,
        order=ch.order_index,
    )


# ── Stats ─────────────────────────────────────────────────────

@router.get("/stats", response_model=StatsOut)
async def get_stats(user: CurrentUser, db: DB):
    _require_staff(user)

    # User counts
    user_rows = await db.execute(
        select(User.role, func.count()).group_by(User.role)
    )
    by_role: dict[str, int] = {r.value: 0 for r in UserRole}
    total = 0
    for role, cnt in user_rows.all():
        by_role[role.value] = cnt
        total += cnt

    # Active tickets
    active_tickets_row = await db.execute(
        select(func.count()).where(Ticket.status != TicketStatus.closed)
    )
    active_tickets = active_tickets_row.scalar_one()

    # Pending verifications
    pending_ver_row = await db.execute(
        select(func.count()).where(VerificationRequest.status == RequestStatus.pending)
    )
    pending_ver = pending_ver_row.scalar_one()

    # Channels
    ch_row = await db.execute(select(func.count()).select_from(Channel))
    total_channels = ch_row.scalar_one()

    # Messages last 24h
    since = datetime.now(timezone.utc) - timedelta(hours=24)
    msg_row = await db.execute(
        select(func.count()).where(Message.created_at >= since)
    )
    msg_count = msg_row.scalar_one()

    return StatsOut(
        totalUsers=total,
        usersByRole=by_role,
        activeTickets=active_tickets,
        pendingVerifications=pending_ver,
        totalChannels=total_channels,
        messagesLast24h=msg_count,
    )


# ── Users ─────────────────────────────────────────────────────

@router.get("/users", response_model=AdminUserListOut)
async def list_users(
    user: CurrentUser,
    db: DB,
    q: str | None = Query(None),
    role: str | None = Query(None),
    page: int = Query(1, ge=1),
    page_size: int = Query(20, ge=1, le=100),
):
    _require_staff(user)

    stmt = select(User)
    if q:
        like = f"%{q}%"
        from sqlalchemy import or_
        stmt = stmt.where(
            or_(User.username.ilike(like), User.email.ilike(like), User.display_name.ilike(like))
        )
    if role:
        try:
            stmt = stmt.where(User.role == UserRole(role))
        except ValueError:
            pass

    count_row = await db.execute(select(func.count()).select_from(stmt.subquery()))
    total = count_row.scalar_one()

    stmt = stmt.order_by(User.created_at.desc()).offset((page - 1) * page_size).limit(page_size)
    rows = await db.execute(stmt)
    users = rows.scalars().all()

    return AdminUserListOut(
        items=[_user_out(u) for u in users],
        total=total,
        page=page,
        pageSize=page_size,
    )


@router.get("/users/{user_id}", response_model=AdminUserOut)
async def get_user(user_id: str, user: CurrentUser, db: DB):
    _require_staff(user)
    u = await db.get(User, user_id)
    if not u:
        raise HTTPException(status_code=404, detail="User not found")
    return _user_out(u)


@router.patch("/users/{user_id}", response_model=AdminUserOut)
async def update_user(user_id: str, req: UpdateUserRequest, user: CurrentUser, db: DB):
    _require_staff(user)
    u = await db.get(User, user_id)
    if not u:
        raise HTTPException(status_code=404, detail="User not found")

    if req.role is not None:
        try:
            u.role = UserRole(req.role)
        except ValueError:
            raise HTTPException(status_code=422, detail=f"Invalid role: {req.role}")
    if req.isActive is not None:
        u.is_active = req.isActive
    if req.managedSchoolCode is not None:
        u.managed_school_code = req.managedSchoolCode or None
    if req.managedDeptName is not None:
        u.managed_dept_name = req.managedDeptName or None

    await db.commit()
    await db.refresh(u)
    return _user_out(u)


@router.post("/users/{user_id}/force-logout", status_code=204)
async def force_logout(user_id: str, user: CurrentUser, db: DB):
    _require_staff(user)
    u = await db.get(User, user_id)
    if not u:
        raise HTTPException(status_code=404, detail="User not found")

    result = await db.execute(
        select(UserSession).where(UserSession.user_id == user_id, UserSession.is_revoked == False)  # noqa: E712
    )
    for session in result.scalars().all():
        session.is_revoked = True
    await db.commit()


# ── Channels ──────────────────────────────────────────────────

@router.get("/channels", response_model=list[ChannelOut])
async def list_all_channels(user: CurrentUser, db: DB):
    _require_staff(user)
    result = await db.execute(select(Channel).order_by(Channel.order_index))
    return [_channel_out(ch) for ch in result.scalars().all()]


@router.post("/channels", response_model=ChannelOut, status_code=201)
async def create_channel(req: CreateChannelRequest, user: CurrentUser, db: DB):
    _require_staff(user)
    try:
        ch_type = ChannelType(req.type)
    except ValueError:
        raise HTTPException(status_code=422, detail=f"Invalid type: {req.type}")
    try:
        scope_type = ChannelScopeType(req.scopeType)
    except ValueError:
        raise HTTPException(status_code=422, detail=f"Invalid scopeType: {req.scopeType}")

    ch = Channel(
        id=str(uuid.uuid4()),
        name=req.name,
        description=req.description,
        type=ch_type,
        scope_type=scope_type,
        school_code=req.schoolCode,
        dept_code=req.deptCode,
        parent_id=req.parentId,
        cohort_year=req.cohortYear,
        audience=req.audience,
        order_index=req.order,
    )
    db.add(ch)
    await db.commit()
    await db.refresh(ch)
    return _channel_out(ch)


@router.patch("/channels/{channel_id}", response_model=ChannelOut)
async def update_channel(channel_id: str, req: UpdateChannelRequest, user: CurrentUser, db: DB):
    _require_staff(user)
    ch = await db.get(Channel, channel_id)
    if not ch:
        raise HTTPException(status_code=404, detail="Channel not found")

    if req.name is not None:
        ch.name = req.name
    if req.description is not None:
        ch.description = req.description
    if req.type is not None:
        try:
            ch.type = ChannelType(req.type)
        except ValueError:
            raise HTTPException(status_code=422, detail=f"Invalid type: {req.type}")
    if req.scopeType is not None:
        try:
            ch.scope_type = ChannelScopeType(req.scopeType)
        except ValueError:
            raise HTTPException(status_code=422, detail=f"Invalid scopeType: {req.scopeType}")
    if req.schoolCode is not None:
        ch.school_code = req.schoolCode
    if req.deptCode is not None:
        ch.dept_code = req.deptCode
    if req.parentId is not None:
        ch.parent_id = req.parentId
    if req.isArchived is not None:
        ch.is_archived = req.isArchived
    if req.cohortYear is not None:
        ch.cohort_year = req.cohortYear
    if req.audience is not None:
        ch.audience = req.audience
    if req.order is not None:
        ch.order_index = req.order

    await db.commit()
    await db.refresh(ch)
    return _channel_out(ch)


@router.delete("/channels/{channel_id}", status_code=204)
async def delete_channel(channel_id: str, user: CurrentUser, db: DB):
    _require_staff(user)
    ch = await db.get(Channel, channel_id)
    if not ch:
        raise HTTPException(status_code=404, detail="Channel not found")
    await db.delete(ch)
    await db.commit()


# ── Audit log ─────────────────────────────────────────────────

def _log_out(log: AuditLog, actor_name: str | None) -> AuditLogOut:
    return AuditLogOut(
        id=log.id,
        correlationId=log.correlation_id,
        actorId=log.actor_id,
        actorDisplayName=actor_name,
        action=log.action,
        targetType=log.target_type,
        targetId=log.target_id,
        ip=log.ip,
        userAgent=log.user_agent,
        createdAt=log.created_at.isoformat(),
    )


@router.get("/audit-log", response_model=AuditLogListOut)
async def list_audit_log(
    user: CurrentUser,
    db: DB,
    action: str | None = Query(None),
    actor_id: str | None = Query(None),
    correlation_id: str | None = Query(None),
    page: int = Query(1, ge=1),
    page_size: int = Query(50, ge=1, le=200),
):
    _require_staff(user)

    stmt = select(AuditLog, User.display_name.label("actor_name")).outerjoin(
        User, AuditLog.actor_id == User.id
    )

    if action:
        stmt = stmt.where(AuditLog.action.ilike(f"%{action}%"))
    if actor_id:
        stmt = stmt.where(AuditLog.actor_id == actor_id)
    if correlation_id:
        stmt = stmt.where(AuditLog.correlation_id == correlation_id)

    count_stmt = select(func.count()).select_from(
        select(AuditLog).where(
            *([AuditLog.action.ilike(f"%{action}%")] if action else []),
            *([AuditLog.actor_id == actor_id] if actor_id else []),
            *([AuditLog.correlation_id == correlation_id] if correlation_id else []),
        ).subquery()
    )
    total = (await db.execute(count_stmt)).scalar_one()

    stmt = stmt.order_by(AuditLog.created_at.desc()).offset((page - 1) * page_size).limit(page_size)
    rows = await db.execute(stmt)

    items = [_log_out(log, name) for log, name in rows.all()]
    return AuditLogListOut(items=items, total=total, page=page, pageSize=page_size)


@router.get("/audit-log/export")
async def export_audit_log(
    user: CurrentUser,
    db: DB,
    action: str | None = Query(None),
):
    _require_staff(user)

    stmt = select(AuditLog, User.display_name.label("actor_name")).outerjoin(
        User, AuditLog.actor_id == User.id
    )
    if action:
        stmt = stmt.where(AuditLog.action.ilike(f"%{action}%"))
    stmt = stmt.order_by(AuditLog.created_at.desc()).limit(10000)
    rows = await db.execute(stmt)

    output = io.StringIO()
    writer = csv.writer(output)
    writer.writerow(["id", "correlationId", "actorId", "actor", "action", "targetType", "targetId", "ip", "createdAt"])
    for log, actor_name in rows.all():
        writer.writerow([
            log.id, log.correlation_id, log.actor_id or "",
            actor_name or "", log.action,
            log.target_type or "", log.target_id or "",
            log.ip, log.created_at.isoformat(),
        ])

    output.seek(0)
    return StreamingResponse(
        iter([output.getvalue()]),
        media_type="text/csv",
        headers={"Content-Disposition": "attachment; filename=audit-log.csv"},
    )


# ── Reputation ────────────────────────────────────────────────

@router.get("/reputation/{user_id}", response_model=ReputationDetailOut)
async def get_reputation(user_id: str, user: CurrentUser, db: DB):
    _require_staff(user)

    target = await db.get(User, user_id)
    if not target:
        raise HTTPException(status_code=404, detail="User not found")

    logs = await db.execute(
        select(AuditLog, User.display_name.label("actor_name"))
        .outerjoin(User, AuditLog.actor_id == User.id)
        .where(
            AuditLog.action == "reputation.adjusted",
            AuditLog.target_type == "user",
            AuditLog.target_id == user_id,
        )
        .order_by(AuditLog.created_at.desc())
        .limit(200)
    )

    events: list[ReputationEventOut] = []
    for log, actor_name in logs.all():
        metadata = log.metadata_ or {}
        events.append(
            ReputationEventOut(
                id=log.id,
                userId=user_id,
                delta=int(metadata.get("delta", 0)),
                reason=str(metadata.get("reason", "")),
                createdAt=log.created_at.isoformat(),
                actorId=log.actor_id,
                actorDisplayName=actor_name,
            )
        )

    return ReputationDetailOut(
        userId=target.id,
        username=target.username,
        displayName=target.display_name,
        currentScore=target.reputation_score,
        events=events,
    )


@router.post("/reputation/{user_id}/adjust", response_model=ReputationDetailOut)
async def adjust_reputation(
    user_id: str,
    req: AdjustReputationRequest,
    request: Request,
    user: CurrentUser,
    db: DB,
):
    _require_staff(user)

    if req.delta == 0:
        raise HTTPException(status_code=422, detail="delta must not be 0")
    reason = req.reason.strip()
    if not reason:
        raise HTTPException(status_code=422, detail="reason is required")

    target = await db.get(User, user_id)
    if not target:
        raise HTTPException(status_code=404, detail="User not found")

    before = target.reputation_score
    target.reputation_score = before + req.delta

    db.add(
        AuditLog(
            correlation_id=request.state.correlation_id,
            actor_id=user.id,
            action="reputation.adjusted",
            target_type="user",
            target_id=target.id,
            ip=get_client_ip(request),
            user_agent=request.headers.get("user-agent"),
            metadata_={
                "delta": req.delta,
                "reason": reason,
                "before": before,
                "after": target.reputation_score,
            },
        )
    )

    await db.commit()
    await db.refresh(target)
    return await get_reputation(user_id, user, db)


# ── 校系評分規則 ───────────────────────────────────────────────

@router.get("/portfolio-rules", response_model=list[PortfolioScoringRuleOut])
async def list_portfolio_scoring_rules(user: CurrentUser, db: DB):
    _require_staff(user)
    return await portfolio_service.list_scoring_rules(db)


@router.post("/portfolio-rules", response_model=PortfolioScoringRuleOut, status_code=201)
async def create_portfolio_scoring_rule(req: PortfolioScoringRuleCreate, user: CurrentUser, db: DB):
    _require_staff(user)
    return await portfolio_service.create_scoring_rule(req, db)


@router.get("/portfolio-rules/export")
async def export_portfolio_scoring_rules(user: CurrentUser, db: DB):
    _require_staff(user)
    rules = await portfolio_service.list_scoring_rules(db)
    output = io.StringIO()
    writer = csv.writer(output)
    writer.writerow(["schoolName", "schoolAbbr", "deptName", "score", "note"])
    for rule in rules:
        writer.writerow([
            rule.schoolName,
            rule.schoolAbbr or '',
            rule.deptName,
            rule.score,
            rule.note or '',
        ])
    return StreamingResponse(
        iter([output.getvalue().encode("utf-8-sig")]),
        media_type="text/csv; charset=utf-8",
        headers={"Content-Disposition": "attachment; filename=portfolio-rules.csv"},
    )


@router.post("/portfolio-rules/import", response_model=PortfolioRuleImportResult)
async def import_portfolio_scoring_rules(req: PortfolioRuleImportRequest, user: CurrentUser, db: DB):
    _require_staff(user)
    if not req.rows:
        raise HTTPException(status_code=422, detail="rows is empty")
    return await portfolio_service.bulk_upsert_scoring_rules(req.rows, db)


@router.patch("/portfolio-rules/{rule_id}", response_model=PortfolioScoringRuleOut)
async def update_portfolio_scoring_rule(
    rule_id: str, req: PortfolioScoringRuleUpdate, user: CurrentUser, db: DB
):
    _require_staff(user)
    rule = await portfolio_service.update_scoring_rule(rule_id, req, db)
    if rule is None:
        raise HTTPException(status_code=404, detail="Rule not found")
    return rule


@router.delete("/portfolio-rules/{rule_id}", status_code=204)
async def delete_portfolio_scoring_rule(rule_id: str, user: CurrentUser, db: DB):
    _require_staff(user)
    deleted = await portfolio_service.delete_scoring_rule(rule_id, db)
    if not deleted:
        raise HTTPException(status_code=404, detail="Rule not found")


# ── 校系選單管理 ───────────────────────────────────────────────

@router.get("/school-options", response_model=list[SchoolOptionOut])
async def list_school_options(user: CurrentUser, db: DB):
    _require_staff(user)
    return await portfolio_service.list_school_options(db)


@router.post("/school-options", response_model=SchoolOptionOut, status_code=201)
async def create_school_option(req: SchoolOptionCreate, user: CurrentUser, db: DB):
    _require_staff(user)
    return await portfolio_service.create_school_option(req, db)


@router.delete("/school-options/{option_id}", status_code=204)
async def delete_school_option(option_id: str, user: CurrentUser, db: DB):
    _require_staff(user)
    deleted = await portfolio_service.delete_school_option(option_id, db)
    if not deleted:
        raise HTTPException(status_code=404, detail="Option not found")


# ── 學校/科系申請審核 ──────────────────────────────────────────

@router.get("/school-requests", response_model=list[SchoolRequestOut])
async def list_school_requests(
    user: CurrentUser,
    db: DB,
    status: str | None = Query(None),
):
    _require_staff(user)
    return await portfolio_service.list_school_requests(db, status=status)


@router.patch("/school-requests/{request_id}", response_model=SchoolRequestOut)
async def review_school_request(
    request_id: str, req: SchoolRequestReview, user: CurrentUser, db: DB
):
    _require_staff(user)
    result = await portfolio_service.review_school_request(request_id, req, str(user.id), db)
    if result is None:
        raise HTTPException(status_code=404, detail="Request not found")
    return result
