from __future__ import annotations

import enum
import uuid
from datetime import datetime

from sqlalchemy import Boolean, DateTime, Enum as SAEnum, Integer, String, Text, func
from sqlalchemy.orm import Mapped, mapped_column, relationship

from app.database import Base


class ChannelType(str, enum.Enum):
    text = "text"
    announcement = "announcement"


class ChannelScopeType(str, enum.Enum):
    global_ = "global"
    school = "school"
    dept = "dept"


class Channel(Base):
    __tablename__ = "channels"

    id: Mapped[str] = mapped_column(String(36), primary_key=True, default=lambda: str(uuid.uuid4()))
    name: Mapped[str] = mapped_column(String(100), nullable=False)
    description: Mapped[str | None] = mapped_column(Text, nullable=True)
    type: Mapped[ChannelType] = mapped_column(SAEnum(ChannelType), default=ChannelType.text, nullable=False)
    scope_type: Mapped[ChannelScopeType] = mapped_column(
        SAEnum(
            ChannelScopeType,
            name="channelscopetype",
            values_callable=lambda enum_cls: [e.value for e in enum_cls],
        ),
        default=ChannelScopeType.global_,
        nullable=False,
    )
    school_code: Mapped[str | None] = mapped_column(String(30), nullable=True)
    dept_code: Mapped[str | None] = mapped_column(String(30), nullable=True)
    parent_id: Mapped[str | None] = mapped_column(String(36), nullable=True, index=True)
    is_archived: Mapped[bool] = mapped_column(Boolean, default=False, nullable=False)
    cohort_year: Mapped[int | None] = mapped_column(Integer, nullable=True)
    audience: Mapped[str | None] = mapped_column(String(20), nullable=True)
    order_index: Mapped[int] = mapped_column(Integer, default=0, nullable=False)
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), server_default=func.now(), nullable=False)

    messages: Mapped[list["Message"]] = relationship(back_populates="channel")
