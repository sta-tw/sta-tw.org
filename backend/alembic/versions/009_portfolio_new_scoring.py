"""Replace result-based scoring with dept rules + view scoring

Revision ID: 009
Revises: 008
Create Date: 2026-02-21

Changes:
- Add long_view_count to portfolio_documents
- Create portfolio_scoring_rules table (admin-configurable per school/dept/year)
- Create portfolio_view_logs table (per-user long-view tracking)
"""
from typing import Sequence, Union

from alembic import op
import sqlalchemy as sa

revision: str = "009"
down_revision: Union[str, None] = "008"
branch_labels: Union[str, Sequence[str], None] = None
depends_on: Union[str, Sequence[str], None] = None


def upgrade() -> None:
    # Add long_view_count to portfolio_documents
    op.add_column(
        "portfolio_documents",
        sa.Column("long_view_count", sa.Integer, nullable=False, server_default="0"),
    )

    # Create portfolio_scoring_rules
    op.create_table(
        "portfolio_scoring_rules",
        sa.Column("id", sa.String(36), primary_key=True),
        sa.Column("school_name", sa.String(120), nullable=False),
        sa.Column("dept_name", sa.String(120), nullable=False),
        sa.Column("admission_year", sa.Integer, nullable=True),
        sa.Column("score", sa.Float, nullable=False, server_default="0"),
        sa.Column("note", sa.Text, nullable=True),
        sa.Column("created_at", sa.DateTime(timezone=True), server_default=sa.func.now(), nullable=False),
        sa.Column("updated_at", sa.DateTime(timezone=True), server_default=sa.func.now(), nullable=False),
    )
    op.create_index("ix_scoring_rules_school_name", "portfolio_scoring_rules", ["school_name"])
    op.create_index("ix_scoring_rules_dept_name", "portfolio_scoring_rules", ["dept_name"])
    op.create_index("ix_scoring_rules_admission_year", "portfolio_scoring_rules", ["admission_year"])

    # Create portfolio_view_logs
    op.create_table(
        "portfolio_view_logs",
        sa.Column("id", sa.String(36), primary_key=True),
        sa.Column(
            "doc_id",
            sa.String(36),
            sa.ForeignKey("portfolio_documents.id", ondelete="CASCADE"),
            nullable=False,
        ),
        sa.Column(
            "user_id",
            sa.String(36),
            sa.ForeignKey("users.id", ondelete="CASCADE"),
            nullable=False,
        ),
        sa.Column("long_view_granted", sa.Boolean, nullable=False, server_default="false"),
        sa.Column("created_at", sa.DateTime(timezone=True), server_default=sa.func.now(), nullable=False),
        sa.Column("updated_at", sa.DateTime(timezone=True), server_default=sa.func.now(), nullable=False),
        sa.UniqueConstraint("doc_id", "user_id", name="uq_portfolio_view_doc_user"),
    )
    op.create_index("ix_portfolio_view_logs_doc_id", "portfolio_view_logs", ["doc_id"])
    op.create_index("ix_portfolio_view_logs_user_id", "portfolio_view_logs", ["user_id"])


def downgrade() -> None:
    op.drop_index("ix_portfolio_view_logs_user_id", table_name="portfolio_view_logs")
    op.drop_index("ix_portfolio_view_logs_doc_id", table_name="portfolio_view_logs")
    op.drop_table("portfolio_view_logs")

    op.drop_index("ix_scoring_rules_admission_year", table_name="portfolio_scoring_rules")
    op.drop_index("ix_scoring_rules_dept_name", table_name="portfolio_scoring_rules")
    op.drop_index("ix_scoring_rules_school_name", table_name="portfolio_scoring_rules")
    op.drop_table("portfolio_scoring_rules")

    op.drop_column("portfolio_documents", "long_view_count")
