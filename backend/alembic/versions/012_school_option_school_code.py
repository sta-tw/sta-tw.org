"""portfolio school options add school_code

Revision ID: 012
Revises: 011
Create Date: 2026-02-24

"""
from typing import Sequence, Union

from alembic import op
import sqlalchemy as sa

revision: str = "012"
down_revision: Union[str, None] = "011"
branch_labels: Union[str, Sequence[str], None] = None
depends_on: Union[str, Sequence[str], None] = None


def upgrade() -> None:
    op.add_column(
        "portfolio_school_options",
        sa.Column("school_code", sa.String(length=30), nullable=True),
    )
    op.create_index(
        "ix_school_options_school_code",
        "portfolio_school_options",
        ["school_code"],
        unique=False,
    )


def downgrade() -> None:
    op.drop_index("ix_school_options_school_code", table_name="portfolio_school_options")
    op.drop_column("portfolio_school_options", "school_code")
