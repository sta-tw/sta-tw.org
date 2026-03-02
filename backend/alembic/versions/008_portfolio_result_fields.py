"""Add result fields and recommendation score to portfolio_documents

Revision ID: 008
Revises: 007
Create Date: 2026-02-21

"""
from typing import Sequence, Union

from alembic import op
import sqlalchemy as sa

revision: str = "008"
down_revision: Union[str, None] = "007"
branch_labels: Union[str, Sequence[str], None] = None
depends_on: Union[str, Sequence[str], None] = None


def upgrade() -> None:
    op.add_column("portfolio_documents", sa.Column("result_type", sa.String(20), nullable=True))
    op.add_column("portfolio_documents", sa.Column("admitted_rank", sa.Integer, nullable=True))
    op.add_column("portfolio_documents", sa.Column("total_admitted", sa.Integer, nullable=True))
    op.add_column("portfolio_documents", sa.Column("waitlist_rank", sa.Integer, nullable=True))
    op.add_column("portfolio_documents", sa.Column("portfolio_score", sa.Float, nullable=True))


def downgrade() -> None:
    op.drop_column("portfolio_documents", "portfolio_score")
    op.drop_column("portfolio_documents", "waitlist_rank")
    op.drop_column("portfolio_documents", "total_admitted")
    op.drop_column("portfolio_documents", "admitted_rank")
    op.drop_column("portfolio_documents", "result_type")
