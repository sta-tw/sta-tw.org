"""admission_documents table

Revision ID: 005
Revises: 004
Create Date: 2026-02-20

"""
from typing import Sequence, Union

from alembic import op
import sqlalchemy as sa

revision: str = "005"
down_revision: Union[str, None] = "004"
branch_labels: Union[str, Sequence[str], None] = None
depends_on: Union[str, Sequence[str], None] = None


def upgrade() -> None:
    op.create_table(
        "admission_documents",
        sa.Column("id", sa.String(36), primary_key=True),
        sa.Column("source_url", sa.String(1000), nullable=False),
        sa.Column("title", sa.String(300), nullable=True),
        sa.Column("school_name", sa.String(120), nullable=True),
        sa.Column("academic_year", sa.Integer, nullable=True),
        sa.Column("page_count", sa.Integer, nullable=False, server_default="0"),
        sa.Column("text_preview", sa.Text, nullable=True),
        sa.Column("key_dates", sa.JSON, nullable=False, server_default=sa.text("'[]'::json")),
        sa.Column("created_by_id", sa.String(36), sa.ForeignKey("users.id", ondelete="SET NULL"), nullable=True),
        sa.Column("created_at", sa.DateTime(timezone=True), server_default=sa.func.now(), nullable=False),
        sa.Column("updated_at", sa.DateTime(timezone=True), server_default=sa.func.now(), nullable=False),
    )
    op.create_index("ix_admission_documents_source_url", "admission_documents", ["source_url"], unique=True)
    op.create_index("ix_admission_documents_school_name", "admission_documents", ["school_name"])
    op.create_index("ix_admission_documents_academic_year", "admission_documents", ["academic_year"])


def downgrade() -> None:
    op.drop_table("admission_documents")
