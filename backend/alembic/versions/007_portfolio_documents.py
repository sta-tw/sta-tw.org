"""portfolio_documents table; drop cross_rank_entries

Revision ID: 007
Revises: 006
Create Date: 2026-02-21

"""
from typing import Sequence, Union

from alembic import op
import sqlalchemy as sa

revision: str = "007"
down_revision: Union[str, None] = "006"
branch_labels: Union[str, Sequence[str], None] = None
depends_on: Union[str, Sequence[str], None] = None


def upgrade() -> None:
    # Drop cross_rank_entries
    op.drop_index("ix_cross_rank_school_dept", table_name="cross_rank_entries")
    op.drop_index("ix_cross_rank_entries_user_id", table_name="cross_rank_entries")
    op.drop_table("cross_rank_entries")

    # Create portfolio_documents
    op.create_table(
        "portfolio_documents",
        sa.Column("id", sa.String(36), primary_key=True),
        sa.Column(
            "uploader_id",
            sa.String(36),
            sa.ForeignKey("users.id", ondelete="CASCADE"),
            nullable=False,
        ),
        sa.Column("title", sa.String(200), nullable=False),
        sa.Column("description", sa.Text, nullable=True),
        sa.Column("school_name", sa.String(120), nullable=False),
        sa.Column("dept_name", sa.String(120), nullable=False),
        sa.Column("admission_year", sa.Integer, nullable=False),
        sa.Column("category", sa.String(50), nullable=False),
        sa.Column("file_key", sa.String(500), nullable=False),
        sa.Column("file_name", sa.String(300), nullable=False),
        sa.Column("file_size", sa.Integer, nullable=False, server_default="0"),
        sa.Column("is_approved", sa.Boolean, nullable=False, server_default="false"),
        sa.Column("view_count", sa.Integer, nullable=False, server_default="0"),
        sa.Column(
            "created_at",
            sa.DateTime(timezone=True),
            server_default=sa.func.now(),
            nullable=False,
        ),
        sa.Column(
            "updated_at",
            sa.DateTime(timezone=True),
            server_default=sa.func.now(),
            nullable=False,
        ),
    )
    op.create_index("ix_portfolio_uploader_id", "portfolio_documents", ["uploader_id"])
    op.create_index("ix_portfolio_school_name", "portfolio_documents", ["school_name"])
    op.create_index("ix_portfolio_dept_name", "portfolio_documents", ["dept_name"])
    op.create_index("ix_portfolio_admission_year", "portfolio_documents", ["admission_year"])


def downgrade() -> None:
    op.drop_index("ix_portfolio_admission_year", table_name="portfolio_documents")
    op.drop_index("ix_portfolio_dept_name", table_name="portfolio_documents")
    op.drop_index("ix_portfolio_school_name", table_name="portfolio_documents")
    op.drop_index("ix_portfolio_uploader_id", table_name="portfolio_documents")
    op.drop_table("portfolio_documents")

    # Restore cross_rank_entries
    op.create_table(
        "cross_rank_entries",
        sa.Column("id", sa.String(36), primary_key=True),
        sa.Column(
            "user_id",
            sa.String(36),
            sa.ForeignKey("users.id", ondelete="CASCADE"),
            nullable=False,
        ),
        sa.Column("school_name", sa.String(120), nullable=False),
        sa.Column("dept_name", sa.String(120), nullable=False),
        sa.Column("result_status", sa.String(20), nullable=False),
        sa.Column("waitlist_rank", sa.Integer, nullable=True),
        sa.Column("enrollment_intent", sa.String(20), nullable=False, server_default="unknown"),
        sa.Column("notes", sa.Text, nullable=True),
        sa.Column("created_at", sa.DateTime(timezone=True), server_default=sa.func.now(), nullable=False),
        sa.Column("updated_at", sa.DateTime(timezone=True), server_default=sa.func.now(), nullable=False),
        sa.UniqueConstraint("user_id", "school_name", "dept_name", name="uq_cross_rank_user_school_dept"),
    )
    op.create_index("ix_cross_rank_entries_user_id", "cross_rank_entries", ["user_id"])
    op.create_index("ix_cross_rank_school_dept", "cross_rank_entries", ["school_name", "dept_name"])
