"""014_portfolio_reputation_tracking

Revision ID: 014
Revises: 013
Create Date: 2026-02-25
"""
from alembic import op
import sqlalchemy as sa

revision = '014'
down_revision = '013'
branch_labels = None
depends_on = None


def upgrade():
    op.add_column("portfolio_view_logs",
        sa.Column("share_reward_granted", sa.Boolean, nullable=False, server_default="false"))
    op.add_column("portfolio_view_logs",
        sa.Column("session_grace_remaining_s", sa.Integer, nullable=False, server_default="600"))
    op.add_column("portfolio_view_logs",
        sa.Column("last_heartbeat_at", sa.DateTime(timezone=True), nullable=True))
    op.add_column("portfolio_view_logs",
        sa.Column("total_effective_seconds", sa.Integer, nullable=False, server_default="0"))
    op.add_column("portfolio_view_logs",
        sa.Column("reputation_intervals_granted", sa.Integer, nullable=False, server_default="0"))


def downgrade():
    op.drop_column("portfolio_view_logs", "reputation_intervals_granted")
    op.drop_column("portfolio_view_logs", "total_effective_seconds")
    op.drop_column("portfolio_view_logs", "last_heartbeat_at")
    op.drop_column("portfolio_view_logs", "session_grace_remaining_s")
    op.drop_column("portfolio_view_logs", "share_reward_granted")
