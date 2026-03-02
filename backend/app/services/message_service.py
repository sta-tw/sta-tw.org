"""Message business logic."""
from datetime import datetime, timezone

from fastapi import HTTPException, status
from sqlalchemy import func, select
from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy.orm import selectinload

from app.models.message import Message, MessageReaction, MessageStatus
from app.models.user import User
from app.schemas.message import AuthorInfo, MessageOut, MessageReplyOut, MessagesListOut, ReactionOut

ALLOWED_EMOJIS = {"👍", "❤️", "😂", "😮", "😢", "🎉", "🔥", "✅", "👏", "🙏", "💯", "👋"}


def _author_info(user: User) -> AuthorInfo:
    return AuthorInfo(
        id=user.id,
        username=user.username,
        displayName=user.display_name,
        role=user.role.value,
        avatarUrl=user.avatar_url,
    )


async def _build_message_out(
    msg: Message,
    reactions_map: dict[str, list[MessageReaction]],
    thread_count_map: dict[str, int],
) -> MessageOut:
    reactions = [
        ReactionOut(
            emoji=emoji,
            count=len(rxns),
            userIds=[r.user_id for r in rxns],
        )
        for emoji, rxns in _group_reactions(reactions_map.get(msg.id, [])).items()
    ]

    reply_to = None
    if msg.reply_to_id and msg.reply_to:
        reply_to = MessageReplyOut(
            id=msg.reply_to_id,
            authorDisplayName=msg.reply_to.author.display_name,
            content=msg.reply_to.content,
        )

    return MessageOut(
        id=msg.id,
        channelId=msg.channel_id,
        authorId=msg.author_id,
        author=_author_info(msg.author),
        content=msg.content,
        status=msg.status.value,
        createdAt=msg.created_at.isoformat(),
        updatedAt=msg.updated_at.isoformat() if msg.updated_at else None,
        isEdited=msg.is_edited,
        isPinned=msg.is_pinned,
        replyTo=reply_to,
        reactions=reactions,
        threadCount=thread_count_map.get(msg.id, 0),
        forwardFromId=msg.forward_from_id,
    )


def _group_reactions(reactions: list[MessageReaction]) -> dict[str, list[MessageReaction]]:
    groups: dict[str, list[MessageReaction]] = {}
    for r in reactions:
        groups.setdefault(r.emoji, []).append(r)
    return groups


async def _fetch_messages_with_meta(
    messages: list[Message], db: AsyncSession
) -> list[MessageOut]:
    if not messages:
        return []

    msg_ids = [m.id for m in messages]

    # Batch-load reactions
    rxn_result = await db.execute(
        select(MessageReaction).where(MessageReaction.message_id.in_(msg_ids))
    )
    all_reactions = rxn_result.scalars().all()
    reactions_map: dict[str, list[MessageReaction]] = {}
    for r in all_reactions:
        reactions_map.setdefault(r.message_id, []).append(r)

    # Batch-load thread counts
    thread_result = await db.execute(
        select(Message.reply_to_id, func.count().label("cnt"))
        .where(
            Message.reply_to_id.in_(msg_ids),
            Message.status != MessageStatus.deleted,
        )
        .group_by(Message.reply_to_id)
    )
    thread_count_map: dict[str, int] = {row[0]: row[1] for row in thread_result}

    return [await _build_message_out(m, reactions_map, thread_count_map) for m in messages]


async def list_messages(
    channel_id: str, db: AsyncSession, cursor: str | None = None, limit: int = 50
) -> MessagesListOut:
    q = (
        select(Message)
        .where(Message.channel_id == channel_id, Message.status != MessageStatus.deleted)
        .options(
            selectinload(Message.author),
            selectinload(Message.reply_to).selectinload(Message.author),
        )
        .order_by(Message.created_at.asc())
        .limit(limit + 1)
    )
    if cursor:
        q = q.where(Message.created_at > cursor)

    result = await db.execute(q)
    messages = list(result.scalars().all())

    has_more = len(messages) > limit
    if has_more:
        messages = messages[:limit]

    next_cursor = messages[-1].created_at.isoformat() if has_more and messages else None
    out = await _fetch_messages_with_meta(messages, db)
    return MessagesListOut(messages=out, hasMore=has_more, nextCursor=next_cursor)


async def create_message(
    channel_id: str, author_id: str, content: str, reply_to_id: str | None, db: AsyncSession
) -> MessageOut:
    msg = Message(
        channel_id=channel_id,
        author_id=author_id,
        content=content,
        reply_to_id=reply_to_id,
    )
    db.add(msg)
    await db.flush()

    # Reload with relationships
    await db.refresh(msg, ["author", "reply_to"])
    if msg.reply_to:
        await db.refresh(msg.reply_to, ["author"])

    await db.commit()
    return await _build_message_out(msg, {}, {})


async def edit_message(message_id: str, user_id: str, content: str, db: AsyncSession) -> MessageOut:
    result = await db.execute(
        select(Message)
        .where(Message.id == message_id)
        .options(selectinload(Message.author), selectinload(Message.reply_to).selectinload(Message.author))
    )
    msg = result.scalar_one_or_none()
    if not msg:
        raise HTTPException(status_code=404, detail="Message not found")
    if msg.author_id != user_id:
        raise HTTPException(status_code=403, detail="Cannot edit another user's message")
    if msg.status != MessageStatus.active:
        raise HTTPException(status_code=400, detail="Cannot edit this message")

    msg.content = content
    msg.is_edited = True
    msg.updated_at = datetime.now(timezone.utc)
    await db.commit()
    return await _build_message_out(msg, {}, {})


async def withdraw_message(message_id: str, user_id: str, user_role: str, db: AsyncSession) -> None:
    result = await db.execute(select(Message).where(Message.id == message_id))
    msg = result.scalar_one_or_none()
    if not msg:
        raise HTTPException(status_code=404, detail="Message not found")
    if msg.author_id != user_id and user_role not in ("admin", "moderator"):
        raise HTTPException(status_code=403, detail="Permission denied")

    msg.status = MessageStatus.withdrawn
    await db.commit()


async def toggle_reaction(message_id: str, user_id: str, emoji: str, db: AsyncSession) -> None:
    if emoji not in ALLOWED_EMOJIS:
        raise HTTPException(status_code=400, detail="Emoji not allowed")

    result = await db.execute(
        select(MessageReaction).where(
            MessageReaction.message_id == message_id,
            MessageReaction.user_id == user_id,
            MessageReaction.emoji == emoji,
        )
    )
    existing = result.scalar_one_or_none()
    if existing:
        await db.delete(existing)
    else:
        db.add(MessageReaction(message_id=message_id, user_id=user_id, emoji=emoji))
    await db.commit()


async def unreact(message_id: str, user_id: str, emoji: str, db: AsyncSession) -> None:
    result = await db.execute(
        select(MessageReaction).where(
            MessageReaction.message_id == message_id,
            MessageReaction.user_id == user_id,
            MessageReaction.emoji == emoji,
        )
    )
    existing = result.scalar_one_or_none()
    if existing:
        await db.delete(existing)
        await db.commit()


async def pin_message(message_id: str, db: AsyncSession) -> None:
    result = await db.execute(select(Message).where(Message.id == message_id))
    msg = result.scalar_one_or_none()
    if not msg:
        raise HTTPException(status_code=404, detail="Message not found")
    msg.is_pinned = True
    await db.commit()


async def unpin_message(message_id: str, db: AsyncSession) -> None:
    result = await db.execute(select(Message).where(Message.id == message_id))
    msg = result.scalar_one_or_none()
    if not msg:
        raise HTTPException(status_code=404, detail="Message not found")
    msg.is_pinned = False
    await db.commit()


async def get_thread(message_id: str, db: AsyncSession) -> MessagesListOut:
    result = await db.execute(
        select(Message)
        .where(Message.reply_to_id == message_id, Message.status != MessageStatus.deleted)
        .options(selectinload(Message.author), selectinload(Message.reply_to).selectinload(Message.author))
        .order_by(Message.created_at.asc())
    )
    messages = list(result.scalars().all())
    out = await _fetch_messages_with_meta(messages, db)
    return MessagesListOut(messages=out, hasMore=False)


async def forward_message(
    message_id: str, user_id: str, target_channel_id: str, db: AsyncSession
) -> MessageOut:
    result = await db.execute(
        select(Message).where(Message.id == message_id).options(selectinload(Message.author))
    )
    src = result.scalar_one_or_none()
    if not src:
        raise HTTPException(status_code=404, detail="Message not found")

    fwd = Message(
        channel_id=target_channel_id,
        author_id=user_id,
        content=src.content,
        forward_from_id=src.id,
    )
    db.add(fwd)
    await db.flush()
    await db.refresh(fwd, ["author"])
    await db.commit()
    return await _build_message_out(fwd, {}, {})
