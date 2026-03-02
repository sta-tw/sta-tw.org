"""cross_rank_entries table

Revision ID: 006
Revises: 005
Create Date: 2026-02-21

"""
from typing import Sequence, Union

from alembic import op
import sqlalchemy as sa

revision: str = "006"
down_revision: Union[str, None] = "005"
branch_labels: Union[str, Sequence[str], None] = None
depends_on: Union[str, Sequence[str], None] = None


def upgrade() -> None:
    # Clean up previous failed attempts that may have created enum types.
    op.execute("DROP TYPE IF EXISTS crossrankresultstatus")
    op.execute("DROP TYPE IF EXISTS enrollmentintent")

    op.create_table(
        "cross_rank_entries",
        sa.Column("id", sa.String(36), primary_key=True),
        sa.Column("user_id", sa.String(36), sa.ForeignKey("users.id", ondelete="CASCADE"), nullable=False),
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

    op.create_index("ix_cross_rank_entries_user_id", "cross_rank_entries", ["user_id"], unique=False)
    op.create_index("ix_cross_rank_school_dept", "cross_rank_entries", ["school_name", "dept_name"], unique=False)


def downgrade() -> None:
    op.drop_index("ix_cross_rank_school_dept", table_name="cross_rank_entries")
    op.drop_index("ix_cross_rank_entries_user_id", table_name="cross_rank_entries")
    op.drop_table("cross_rank_entries")
