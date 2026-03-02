from __future__ import annotations

import uuid
from datetime import datetime

from sqlalchemy import DateTime, ForeignKey, Integer, JSON, LargeBinary, String, Text, func
from sqlalchemy.orm import Mapped, mapped_column

from app.database import Base


class AdmissionDocument(Base):
    __tablename__ = "admission_documents"

    id: Mapped[str] = mapped_column(String(36), primary_key=True, default=lambda: str(uuid.uuid4()))
    source_url: Mapped[str] = mapped_column(String(1000), unique=True, nullable=False, index=True)
    title: Mapped[str | None] = mapped_column(String(300), nullable=True)
    school_name: Mapped[str | None] = mapped_column(String(120), nullable=True, index=True)
    academic_year: Mapped[int | None] = mapped_column(Integer, nullable=True, index=True)
    page_count: Mapped[int] = mapped_column(Integer, nullable=False, default=0)
    text_preview: Mapped[str | None] = mapped_column(Text, nullable=True)
    key_dates: Mapped[list[dict]] = mapped_column(JSON, nullable=False, default=list)
    school_code: Mapped[str | None] = mapped_column(String(30), nullable=True, index=True)
    pdf_content: Mapped[bytes | None] = mapped_column(LargeBinary, nullable=True)
    created_by_id: Mapped[str | None] = mapped_column(
        String(36), ForeignKey("users.id", ondelete="SET NULL"), nullable=True
    )
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), server_default=func.now(), nullable=False)
    updated_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True), server_default=func.now(), onupdate=func.now(), nullable=False
    )
