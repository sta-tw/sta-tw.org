from __future__ import annotations

import uuid
from datetime import datetime

from sqlalchemy import Boolean, DateTime, Float, ForeignKey, Integer, String, Text, UniqueConstraint, func  # noqa: F401
from sqlalchemy.orm import Mapped, mapped_column

from app.database import Base


class PortfolioScoringRule(Base):
    """校系評分規則，由管理員手動設定。"""
    __tablename__ = "portfolio_scoring_rules"

    id: Mapped[str] = mapped_column(String(36), primary_key=True, default=lambda: str(uuid.uuid4()))
    school_name: Mapped[str] = mapped_column(String(120), nullable=False, index=True)
    school_abbr: Mapped[str | None] = mapped_column(String(30), nullable=True)
    dept_name: Mapped[str] = mapped_column(String(120), nullable=False, index=True)
    score: Mapped[float] = mapped_column(Float, nullable=False, default=0.0)
    note: Mapped[str | None] = mapped_column(Text, nullable=True)
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), server_default=func.now(), nullable=False)
    updated_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True), server_default=func.now(), onupdate=func.now(), nullable=False
    )


class PortfolioViewLog(Base):
    """記錄每位使用者對每份備審資料的瀏覽狀態，用於控制長觀看加分僅一次。"""
    __tablename__ = "portfolio_view_logs"
    __table_args__ = (
        UniqueConstraint("doc_id", "user_id", name="uq_portfolio_view_doc_user"),
    )

    id: Mapped[str] = mapped_column(String(36), primary_key=True, default=lambda: str(uuid.uuid4()))
    doc_id: Mapped[str] = mapped_column(
        String(36), ForeignKey("portfolio_documents.id", ondelete="CASCADE"), nullable=False, index=True
    )
    user_id: Mapped[str] = mapped_column(
        String(36), ForeignKey("users.id", ondelete="CASCADE"), nullable=False, index=True
    )
    # 是否已發放「點開超過 30 分鐘」+0.5 加分
    long_view_granted: Mapped[bool] = mapped_column(Boolean, nullable=False, default=False)
    # 信譽自動加分欄位
    share_reward_granted: Mapped[bool] = mapped_column(Boolean, nullable=False, default=False)
    session_grace_remaining_s: Mapped[int] = mapped_column(Integer, nullable=False, default=600)
    last_heartbeat_at: Mapped[datetime | None] = mapped_column(DateTime(timezone=True), nullable=True)
    total_effective_seconds: Mapped[int] = mapped_column(Integer, nullable=False, default=0)
    reputation_intervals_granted: Mapped[int] = mapped_column(Integer, nullable=False, default=0)
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), server_default=func.now(), nullable=False)
    updated_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True), server_default=func.now(), onupdate=func.now(), nullable=False
    )
