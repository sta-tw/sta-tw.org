from __future__ import annotations

import uuid
from datetime import datetime

from sqlalchemy import Boolean, DateTime, ForeignKey, Integer, String, Text, func
from sqlalchemy.orm import Mapped, mapped_column

from app.database import Base


class SchoolOption(Base):
    """管理員維護的學校/科系選單清單，供使用者上傳時選擇。"""
    __tablename__ = "portfolio_school_options"

    id: Mapped[str] = mapped_column(String(36), primary_key=True, default=lambda: str(uuid.uuid4()))
    school_name: Mapped[str] = mapped_column(String(120), nullable=False, index=True)
    school_code: Mapped[str | None] = mapped_column(String(30), nullable=True, index=True)
    dept_name: Mapped[str] = mapped_column(String(120), nullable=False, index=True)
    is_active: Mapped[bool] = mapped_column(Boolean, nullable=False, default=True)
    sort_order: Mapped[int] = mapped_column(Integer, nullable=False, default=0)
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), server_default=func.now(), nullable=False)
    updated_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True), server_default=func.now(), onupdate=func.now(), nullable=False
    )


class SchoolRequest(Base):
    """使用者申請新增學校/科系的請求，待管理員審核。"""
    __tablename__ = "portfolio_school_requests"

    id: Mapped[str] = mapped_column(String(36), primary_key=True, default=lambda: str(uuid.uuid4()))
    requester_id: Mapped[str] = mapped_column(
        String(36), ForeignKey("users.id", ondelete="CASCADE"), nullable=False, index=True
    )
    school_name: Mapped[str] = mapped_column(String(120), nullable=False)
    dept_name: Mapped[str] = mapped_column(String(120), nullable=False)
    # pending / approved / rejected
    status: Mapped[str] = mapped_column(String(20), nullable=False, default="pending", index=True)
    note: Mapped[str | None] = mapped_column(Text, nullable=True)       # 申請備註
    review_note: Mapped[str | None] = mapped_column(Text, nullable=True)  # 審核備註
    reviewed_by: Mapped[str | None] = mapped_column(
        String(36), ForeignKey("users.id", ondelete="SET NULL"), nullable=True
    )
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), server_default=func.now(), nullable=False)
    updated_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True), server_default=func.now(), onupdate=func.now(), nullable=False
    )
