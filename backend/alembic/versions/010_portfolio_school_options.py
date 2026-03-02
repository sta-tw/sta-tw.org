"""Add school options + school requests; add applicant_name; make category nullable

Revision ID: 010
Revises: 009
Create Date: 2026-02-22

Changes:
- Create portfolio_school_options table
- Create portfolio_school_requests table
- Add applicant_name column to portfolio_documents
- Make portfolio_documents.category nullable
"""
from typing import Sequence, Union

from alembic import op
import sqlalchemy as sa

revision: str = "010"
down_revision: Union[str, None] = "009"
branch_labels: Union[str, Sequence[str], None] = None
depends_on: Union[str, Sequence[str], None] = None


def upgrade() -> None:
    # Create portfolio_school_options
    op.create_table(
        "portfolio_school_options",
        sa.Column("id", sa.String(36), primary_key=True),
        sa.Column("school_name", sa.String(120), nullable=False),
        sa.Column("dept_name", sa.String(120), nullable=False),
        sa.Column("is_active", sa.Boolean, nullable=False, server_default="true"),
        sa.Column("sort_order", sa.Integer, nullable=False, server_default="0"),
        sa.Column("created_at", sa.DateTime(timezone=True), server_default=sa.func.now(), nullable=False),
        sa.Column("updated_at", sa.DateTime(timezone=True), server_default=sa.func.now(), nullable=False),
    )
    op.create_index("ix_school_options_school_name", "portfolio_school_options", ["school_name"])
    op.create_index("ix_school_options_dept_name", "portfolio_school_options", ["dept_name"])

    # Create portfolio_school_requests
    op.create_table(
        "portfolio_school_requests",
        sa.Column("id", sa.String(36), primary_key=True),
        sa.Column(
            "requester_id",
            sa.String(36),
            sa.ForeignKey("users.id", ondelete="CASCADE"),
            nullable=False,
        ),
        sa.Column("school_name", sa.String(120), nullable=False),
        sa.Column("dept_name", sa.String(120), nullable=False),
        sa.Column("status", sa.String(20), nullable=False, server_default="pending"),
        sa.Column("note", sa.Text, nullable=True),
        sa.Column("review_note", sa.Text, nullable=True),
        sa.Column(
            "reviewed_by",
            sa.String(36),
            sa.ForeignKey("users.id", ondelete="SET NULL"),
            nullable=True,
        ),
        sa.Column("created_at", sa.DateTime(timezone=True), server_default=sa.func.now(), nullable=False),
        sa.Column("updated_at", sa.DateTime(timezone=True), server_default=sa.func.now(), nullable=False),
    )
    op.create_index("ix_school_requests_requester_id", "portfolio_school_requests", ["requester_id"])
    op.create_index("ix_school_requests_status", "portfolio_school_requests", ["status"])

    # Add applicant_name to portfolio_documents
    op.add_column(
        "portfolio_documents",
        sa.Column("applicant_name", sa.String(100), nullable=True),
    )

    # Make category nullable
    op.alter_column("portfolio_documents", "category", nullable=True)


def downgrade() -> None:
    op.alter_column("portfolio_documents", "category", nullable=False)
    op.drop_column("portfolio_documents", "applicant_name")

    op.drop_index("ix_school_requests_status", table_name="portfolio_school_requests")
    op.drop_index("ix_school_requests_requester_id", table_name="portfolio_school_requests")
    op.drop_table("portfolio_school_requests")

    op.drop_index("ix_school_options_dept_name", table_name="portfolio_school_options")
    op.drop_index("ix_school_options_school_name", table_name="portfolio_school_options")
    op.drop_table("portfolio_school_options")
