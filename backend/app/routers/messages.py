"""Message router — /api/v1/messages/*"""
from fastapi import APIRouter, BackgroundTasks

from app.deps import CurrentUser, DB
from app.schemas.common import MessageResponse
from app.schemas.message import EditMessageRequest, ForwardRequest, MessageOut, MessagesListOut, ReactRequest
from app.services import message_service, search_service
from app.ws.manager import manager

router = APIRouter()


@router.patch("/{message_id}", response_model=MessageOut)
async def edit_message(message_id: str, req: EditMessageRequest, db: DB, user: CurrentUser, background_tasks: BackgroundTasks):
    msg = await message_service.edit_message(message_id, user.id, req.content, db)
    await manager.broadcast(msg.channelId, {"type": "message.update", "data": msg.model_dump()})
    background_tasks.add_task(search_service.update_message_index, msg)
    return msg


@router.post("/{message_id}/withdraw", response_model=MessageResponse)
async def withdraw_message(message_id: str, db: DB, user: CurrentUser, background_tasks: BackgroundTasks):
    await message_service.withdraw_message(message_id, user.id, user.role.value, db)
    background_tasks.add_task(search_service.delete_message_index, message_id)
    return MessageResponse(message="ok")


@router.delete("/{message_id}", response_model=MessageResponse)
async def delete_message(message_id: str, db: DB, user: CurrentUser, background_tasks: BackgroundTasks):
    from app.models.message import Message, MessageStatus
    from sqlalchemy import select
    result = await db.execute(select(Message).where(Message.id == message_id))
    msg = result.scalar_one_or_none()
    _MOD_VALUES = {"admin", "developer", "school_moderator", "dept_moderator"}
    if msg and (msg.author_id == user.id or user.role.value in _MOD_VALUES):
        msg.status = MessageStatus.deleted
        await db.commit()
    background_tasks.add_task(search_service.delete_message_index, message_id)
    return MessageResponse(message="ok")


@router.post("/{message_id}/reactions", response_model=MessageResponse)
async def react(message_id: str, req: ReactRequest, db: DB, user: CurrentUser):
    await message_service.toggle_reaction(message_id, user.id, req.emoji, db)
    return MessageResponse(message="ok")


@router.delete("/{message_id}/reactions/{emoji}", response_model=MessageResponse)
async def unreact(message_id: str, emoji: str, db: DB, user: CurrentUser):
    await message_service.unreact(message_id, user.id, emoji, db)
    return MessageResponse(message="ok")


@router.post("/{message_id}/pin", response_model=MessageResponse)
async def pin(message_id: str, db: DB, user: CurrentUser):
    await message_service.pin_message(message_id, db)
    return MessageResponse(message="ok")


@router.delete("/{message_id}/pin", response_model=MessageResponse)
async def unpin(message_id: str, db: DB, user: CurrentUser):
    await message_service.unpin_message(message_id, db)
    return MessageResponse(message="ok")


@router.get("/{message_id}/thread", response_model=MessagesListOut)
async def get_thread(message_id: str, db: DB, _user: CurrentUser):
    return await message_service.get_thread(message_id, db)


@router.post("/{message_id}/forward", response_model=MessageOut, status_code=201)
async def forward(message_id: str, req: ForwardRequest, db: DB, user: CurrentUser, background_tasks: BackgroundTasks):
    msg = await message_service.forward_message(message_id, user.id, req.targetChannelId, db)
    await manager.broadcast(req.targetChannelId, {"type": "message.new", "data": msg.model_dump()})
    background_tasks.add_task(search_service.index_message, msg)
    return msg
