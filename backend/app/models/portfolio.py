from __future__ import annotations

import uuid
from datetime import datetime

from sqlalchemy import Boolean, DateTime, Float, ForeignKey, Integer, String, Text, func
from sqlalchemy.orm import Mapped, mapped_column

from app.database import Base


class PortfolioDocument(Base):
    __tablename__ = "portfolio_documents"

    id: Mapped[str] = mapped_column(String(36), primary_key=True, default=lambda: str(uuid.uuid4()))
    uploader_id: Mapped[str] = mapped_column(
        String(36), ForeignKey("users.id", ondelete="CASCADE"), nullable=False, index=True
    )
    title: Mapped[str] = mapped_column(String(200), nullable=False)
    description: Mapped[str | None] = mapped_column(Text, nullable=True)
    school_name: Mapped[str] = mapped_column(String(120), nullable=False, index=True)
    dept_name: Mapped[str] = mapped_column(String(120), nullable=False, index=True)
    # 民國年 / 屆數，例如 116
    admission_year: Mapped[int] = mapped_column(Integer, nullable=False, index=True)
    category: Mapped[str | None] = mapped_column(String(50), nullable=True)
    applicant_name: Mapped[str | None] = mapped_column(String(100), nullable=True)
    # 結果資訊（用於推薦評分）
    result_type: Mapped[str | None] = mapped_column(String(20), nullable=True)      # admitted / waitlisted / not_admitted
    admitted_rank: Mapped[int | None] = mapped_column(Integer, nullable=True)        # 正取名次
    total_admitted: Mapped[int | None] = mapped_column(Integer, nullable=True)       # 正取總人數
    waitlist_rank: Mapped[int | None] = mapped_column(Integer, nullable=True)        # 備取名次
    portfolio_score: Mapped[float | None] = mapped_column(Float, nullable=True)      # 書審成績 0-100
    # 檔案
    file_key: Mapped[str] = mapped_column(String(500), nullable=False)
    file_name: Mapped[str] = mapped_column(String(300), nullable=False)
    file_size: Mapped[int] = mapped_column(Integer, nullable=False, default=0)
    is_approved: Mapped[bool] = mapped_column(Boolean, nullable=False, default=False)
    view_count: Mapped[int] = mapped_column(Integer, nullable=False, default=0)
    # 長觀看（>30分鐘）人數累計，用於計算 +0.5 加分
    long_view_count: Mapped[int] = mapped_column(Integer, nullable=False, default=0)
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), server_default=func.now(), nullable=False)
    updated_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True), server_default=func.now(), onupdate=func.now(), nullable=False
    )
