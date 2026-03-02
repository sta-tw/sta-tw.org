"""Ticket business logic."""
from __future__ import annotations

from fastapi import HTTPException
from sqlalchemy import select
from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy.orm import selectinload

from app.models.ticket import Ticket, TicketCategory, TicketMessage, TicketStatus
from app.models.user import User, UserRole
from app.schemas.ticket import TicketMessageOut, TicketOut
from app.schemas.user import AuthorInfo


def _author(user: User) -> AuthorInfo:
    return AuthorInfo(
        id=str(user.id),
        username=user.username,
        displayName=user.display_name,
        role=user.role.value,
        avatarUrl=user.avatar_url,
    )


def _msg_out(msg: TicketMessage) -> TicketMessageOut:
    return TicketMessageOut(
        id=msg.id,
        ticketId=msg.ticket_id,
        authorId=msg.author_id,
        author=_author(msg.author),
        content=msg.content,
        isStaff=msg.is_staff,
        createdAt=msg.created_at.isoformat(),
    )


def _ticket_out(ticket: Ticket, include_messages: bool = False) -> TicketOut:
    return TicketOut(
        id=ticket.id,
        userId=ticket.user_id,
        user=_author(ticket.user),
        category=ticket.category.value,
        subject=ticket.subject,
        status=ticket.status.value,
        assigneeId=ticket.assignee_id,
        createdAt=ticket.created_at.isoformat(),
        updatedAt=ticket.updated_at.isoformat(),
        messages=[_msg_out(m) for m in ticket.messages] if include_messages else [],
    )


async def create_ticket(
    user_id: str,
    category: str,
    subject: str,
    content: str,
    db: AsyncSession,
) -> TicketOut:
    try:
        cat = TicketCategory(category)
    except ValueError:
        raise HTTPException(status_code=422, detail=f"Invalid category: {category}")

    ticket = Ticket(user_id=user_id, category=cat, subject=subject)
    db.add(ticket)
    await db.flush()  # get ticket.id

    msg = TicketMessage(
        ticket_id=ticket.id,
        author_id=user_id,
        content=content,
        is_staff=False,
    )
    db.add(msg)
    await db.commit()

    # Reload with relationships
    result = await db.execute(
        select(Ticket)
        .where(Ticket.id == ticket.id)
        .options(selectinload(Ticket.user), selectinload(Ticket.messages).selectinload(TicketMessage.author))
    )
    ticket = result.scalar_one()
    return _ticket_out(ticket, include_messages=True)


async def list_tickets(user_id: str, db: AsyncSession) -> list[TicketOut]:
    result = await db.execute(
        select(Ticket)
        .where(Ticket.user_id == user_id)
        .options(selectinload(Ticket.user))
        .order_by(Ticket.updated_at.desc())
    )
    return [_ticket_out(t) for t in result.scalars().all()]


async def get_ticket(ticket_id: str, user_id: str, is_staff: bool, db: AsyncSession) -> TicketOut:
    result = await db.execute(
        select(Ticket)
        .where(Ticket.id == ticket_id)
        .options(
            selectinload(Ticket.user),
            selectinload(Ticket.messages).selectinload(TicketMessage.author),
        )
    )
    ticket = result.scalar_one_or_none()
    if not ticket:
        raise HTTPException(status_code=404, detail="Ticket not found")
    if not is_staff and ticket.user_id != user_id:
        raise HTTPException(status_code=403, detail="Permission denied")
    return _ticket_out(ticket, include_messages=True)


async def add_message(
    ticket_id: str,
    author_id: str,
    content: str,
    is_staff: bool,
    db: AsyncSession,
) -> TicketMessageOut:
    ticket = await db.get(Ticket, ticket_id)
    if not ticket:
        raise HTTPException(status_code=404, detail="Ticket not found")
    if ticket.status == TicketStatus.closed:
        raise HTTPException(status_code=400, detail="Ticket is closed")
    if not is_staff and ticket.user_id != author_id:
        raise HTTPException(status_code=403, detail="Permission denied")

    msg = TicketMessage(
        ticket_id=ticket_id,
        author_id=author_id,
        content=content,
        is_staff=is_staff,
    )
    # Auto-update status
    if is_staff and ticket.status == TicketStatus.open:
        ticket.status = TicketStatus.processing
    elif is_staff:
        ticket.status = TicketStatus.pending  # waiting for user
    elif not is_staff and ticket.status == TicketStatus.pending:
        ticket.status = TicketStatus.processing  # user replied

    db.add(msg)
    await db.commit()

    # Reload author
    result = await db.execute(
        select(TicketMessage)
        .where(TicketMessage.id == msg.id)
        .options(selectinload(TicketMessage.author))
    )
    msg = result.scalar_one()
    return _msg_out(msg)


async def update_ticket(
    ticket_id: str,
    status: str | None,
    assignee_id: str | None,
    db: AsyncSession,
) -> TicketOut:
    result = await db.execute(
        select(Ticket)
        .where(Ticket.id == ticket_id)
        .options(selectinload(Ticket.user), selectinload(Ticket.messages).selectinload(TicketMessage.author))
    )
    ticket = result.scalar_one_or_none()
    if not ticket:
        raise HTTPException(status_code=404, detail="Ticket not found")

    if status is not None:
        try:
            ticket.status = TicketStatus(status)
        except ValueError:
            raise HTTPException(status_code=422, detail=f"Invalid status: {status}")
    if assignee_id is not None:
        ticket.assignee_id = assignee_id

    await db.commit()
    await db.refresh(ticket)
    return _ticket_out(ticket, include_messages=True)


async def list_all_tickets(
    status: str | None,
    db: AsyncSession,
) -> list[TicketOut]:
    q = select(Ticket).options(selectinload(Ticket.user)).order_by(Ticket.updated_at.desc())
    if status:
        try:
            q = q.where(Ticket.status == TicketStatus(status))
        except ValueError:
            pass
    result = await db.execute(q)
    return [_ticket_out(t) for t in result.scalars().all()]
