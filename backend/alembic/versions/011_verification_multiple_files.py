"""verification: multiple files and doc_type

Revision ID: 011
Revises: 010
Create Date: 2026-02-22
"""
from __future__ import annotations

from alembic import op
import sqlalchemy as sa

revision = "011"
down_revision = "010"
branch_labels = None
depends_on = None


def upgrade() -> None:
    op.add_column(
        "verification_requests",
        sa.Column("file_keys", sa.Text(), nullable=True),
    )
    op.add_column(
        "verification_requests",
        sa.Column("doc_type", sa.String(30), nullable=True),
    )


def downgrade() -> None:
    op.drop_column("verification_requests", "doc_type")
    op.drop_column("verification_requests", "file_keys")
