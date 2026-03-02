"""Channel router — /api/v1/channels/*"""
from fastapi import APIRouter, BackgroundTasks, HTTPException

from app.deps import CurrentUser, DB
from app.schemas.channel import ChannelOut
from app.schemas.message import MessageOut, MessagesListOut, CreateMessageRequest
from app.services import channel_service, message_service, search_service
from app.ws.manager import manager

router = APIRouter()


@router.get("", response_model=list[ChannelOut])
async def list_channels(db: DB, _user: CurrentUser):
    return await channel_service.list_channels(db)


@router.get("/{channel_id}", response_model=ChannelOut)
async def get_channel(channel_id: str, db: DB, _user: CurrentUser):
    ch = await channel_service.get_channel(channel_id, db)
    if not ch:
        raise HTTPException(status_code=404, detail="Channel not found")
    return ch


@router.get("/{channel_id}/messages", response_model=MessagesListOut)
async def list_messages(
    channel_id: str,
    db: DB,
    _user: CurrentUser,
    cursor: str | None = None,
    limit: int = 50,
):
    return await message_service.list_messages(channel_id, db, cursor, min(limit, 100))


@router.post("/{channel_id}/messages", response_model=MessageOut, status_code=201)
async def create_message(
    channel_id: str,
    req: CreateMessageRequest,
    db: DB,
    user: CurrentUser,
    background_tasks: BackgroundTasks,
):
    msg = await message_service.create_message(channel_id, user.id, req.content, req.replyToId, db)
    await manager.broadcast(channel_id, {"type": "message.new", "data": msg.model_dump()})
    background_tasks.add_task(search_service.index_message, msg)
    return msg


@router.get("/{channel_id}/pinned", response_model=list[MessageOut])
async def get_pinned(channel_id: str, db: DB, _user: CurrentUser):
    from sqlalchemy import select
    from sqlalchemy.orm import selectinload
    from app.models.message import Message, MessageStatus
    result = await db.execute(
        select(Message)
        .where(Message.channel_id == channel_id, Message.is_pinned == True, Message.status == MessageStatus.active)  # noqa: E712
        .options(selectinload(Message.author), selectinload(Message.reply_to).selectinload(Message.author))
        .order_by(Message.created_at.asc())
    )
    messages = list(result.scalars().all())
    out = await message_service._fetch_messages_with_meta(messages, db)
    return out
