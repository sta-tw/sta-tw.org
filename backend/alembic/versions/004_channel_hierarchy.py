"""channel hierarchy fields

Revision ID: 004
Revises: 003
Create Date: 2026-02-20

"""
from typing import Sequence, Union

from alembic import op
import sqlalchemy as sa

revision: str = "004"
down_revision: Union[str, None] = "003"
branch_labels: Union[str, Sequence[str], None] = None
depends_on: Union[str, Sequence[str], None] = None


def upgrade() -> None:
    op.execute("CREATE TYPE channelscopetype AS ENUM ('global', 'school', 'dept')")

    op.add_column(
        "channels",
        sa.Column(
            "scope_type",
            sa.Enum("global", "school", "dept", name="channelscopetype"),
            nullable=False,
            server_default="global",
        ),
    )
    op.add_column("channels", sa.Column("school_code", sa.String(length=30), nullable=True))
    op.add_column("channels", sa.Column("dept_code", sa.String(length=30), nullable=True))
    op.add_column("channels", sa.Column("parent_id", sa.String(length=36), nullable=True))

    op.create_index("ix_channels_parent_id", "channels", ["parent_id"], unique=False)
    op.create_foreign_key(
        "fk_channels_parent_id",
        "channels",
        "channels",
        ["parent_id"],
        ["id"],
        ondelete="SET NULL",
    )

    op.alter_column("channels", "scope_type", server_default=None)


def downgrade() -> None:
    op.drop_constraint("fk_channels_parent_id", "channels", type_="foreignkey")
    op.drop_index("ix_channels_parent_id", table_name="channels")
    op.drop_column("channels", "parent_id")
    op.drop_column("channels", "dept_code")
    op.drop_column("channels", "school_code")
    op.drop_column("channels", "scope_type")
    op.execute("DROP TYPE IF EXISTS channelscopetype")
