"""Search router — /api/v1/search"""
import asyncio

from fastapi import APIRouter, Query

from app.deps import CurrentUser, DB
from app.services import search_service

router = APIRouter()


@router.get("")
async def search(
    q: str = Query(..., min_length=1, max_length=200),
    channelId: str | None = None,
    limit: int = Query(default=20, ge=1, le=50),
    _user: CurrentUser = None,
):
    """Full-text search across messages, channels, and users."""
    loop = asyncio.get_event_loop()
    results = await loop.run_in_executor(
        None,
        lambda: search_service.search(q, channelId, limit),
    )
    return results


@router.post("/reindex", tags=["admin"])
async def reindex(db: DB, _user: CurrentUser):
    """Re-index all active messages. Admin/moderator use."""
    from sqlalchemy import select
    from sqlalchemy.orm import selectinload
    from app.models.message import Message, MessageStatus
    from app.services import message_service

    result = await db.execute(
        select(Message)
        .where(Message.status == MessageStatus.active)
        .options(
            selectinload(Message.author),
            selectinload(Message.reply_to).selectinload(Message.author),
        )
    )
    raw = list(result.scalars().all())
    msgs = await message_service._fetch_messages_with_meta(raw, db)

    loop = asyncio.get_event_loop()
    await loop.run_in_executor(None, search_service.reindex_all_messages, msgs)

    return {"indexed": len(msgs)}
