"""Ticket router — /api/v1/tickets/*"""
from fastapi import APIRouter, Query

from app.deps import CurrentUser, DB
from app.models.user import UserRole
from app.schemas.ticket import (
    AddMessageRequest,
    CreateTicketRequest,
    TicketMessageOut,
    TicketOut,
    UpdateTicketRequest,
)
from app.services import ticket_service
from fastapi import HTTPException

router = APIRouter()

_STAFF_ROLES = (UserRole.dept_moderator, UserRole.school_moderator, UserRole.admin, UserRole.developer)


# ── User endpoints ────────────────────────────────────────────

@router.get("", response_model=list[TicketOut])
async def list_my_tickets(user: CurrentUser, db: DB):
    return await ticket_service.list_tickets(str(user.id), db)


@router.post("", response_model=TicketOut, status_code=201)
async def create_ticket(req: CreateTicketRequest, user: CurrentUser, db: DB):
    return await ticket_service.create_ticket(
        str(user.id), req.category, req.subject, req.content, db
    )


@router.get("/{ticket_id}", response_model=TicketOut)
async def get_ticket(ticket_id: str, user: CurrentUser, db: DB):
    is_staff = user.role in _STAFF_ROLES
    return await ticket_service.get_ticket(ticket_id, str(user.id), is_staff, db)


@router.post("/{ticket_id}/messages", response_model=TicketMessageOut, status_code=201)
async def add_message(ticket_id: str, req: AddMessageRequest, user: CurrentUser, db: DB):
    is_staff = user.role in _STAFF_ROLES
    return await ticket_service.add_message(
        ticket_id, str(user.id), req.content, is_staff, db
    )


# ── Admin endpoints ───────────────────────────────────────────

@router.get("/admin/all", response_model=list[TicketOut])
async def list_all_tickets(
    user: CurrentUser,
    db: DB,
    status: str | None = Query(None),
):
    if user.role not in _STAFF_ROLES:
        raise HTTPException(status_code=403, detail="Insufficient role")
    return await ticket_service.list_all_tickets(status, db)


@router.patch("/admin/{ticket_id}", response_model=TicketOut)
async def update_ticket(ticket_id: str, req: UpdateTicketRequest, user: CurrentUser, db: DB):
    if user.role not in _STAFF_ROLES:
        raise HTTPException(status_code=403, detail="Insufficient role")
    return await ticket_service.update_ticket(ticket_id, req.status, req.assigneeId, db)
