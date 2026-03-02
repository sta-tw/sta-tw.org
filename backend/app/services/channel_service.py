"""Channel business logic."""
from sqlalchemy import select
from sqlalchemy.ext.asyncio import AsyncSession

from app.models.channel import Channel
from app.schemas.channel import ChannelOut


def _channel_to_out(ch: Channel, pinned_count: int = 0, unread_count: int = 0) -> ChannelOut:
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
        unreadCount=unread_count,
        order=ch.order_index,
        pinnedCount=pinned_count if pinned_count else None,
    )


async def list_channels(db: AsyncSession) -> list[ChannelOut]:
    result = await db.execute(select(Channel).order_by(Channel.order_index))
    channels = result.scalars().all()
    return [_channel_to_out(ch) for ch in channels]


async def get_channel(channel_id: str, db: AsyncSession) -> ChannelOut | None:
    result = await db.execute(select(Channel).where(Channel.id == channel_id))
    ch = result.scalar_one_or_none()
    if not ch:
        return None
    return _channel_to_out(ch)
