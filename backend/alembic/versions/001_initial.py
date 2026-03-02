"""initial schema

Revision ID: 001
Revises:
Create Date: 2026-02-20

"""
from typing import Sequence, Union

from alembic import op
import sqlalchemy as sa

revision: str = "001"
down_revision: Union[str, None] = None
branch_labels: Union[str, Sequence[str], None] = None
depends_on: Union[str, Sequence[str], None] = None


def upgrade() -> None:
    # ── users ───────────────────────────────────────────────────
    op.create_table(
        "users",
        sa.Column("id", sa.String(36), primary_key=True),
        sa.Column("username", sa.String(50), nullable=False),
        sa.Column("email", sa.String(255), nullable=False),
        sa.Column("hashed_password", sa.String(255), nullable=True),
        sa.Column("display_name", sa.String(100), nullable=False),
        sa.Column("avatar_url", sa.String(500), nullable=True),
        sa.Column("bio", sa.Text, nullable=True),
        sa.Column("role", sa.Enum("registered", "active_student", "alumni", "moderator", "admin", name="userrole"), nullable=False, server_default="registered"),
        sa.Column("verification_status", sa.Enum("none", "pending", "approved", "rejected", name="verificationstatus"), nullable=False, server_default="none"),
        sa.Column("reputation_score", sa.Integer, nullable=False, server_default="0"),
        sa.Column("is_email_verified", sa.Boolean, nullable=False, server_default="false"),
        sa.Column("is_active", sa.Boolean, nullable=False, server_default="true"),
        sa.Column("google_id", sa.String(255), nullable=True),
        sa.Column("discord_id", sa.String(255), nullable=True),
        sa.Column("created_at", sa.DateTime(timezone=True), server_default=sa.func.now(), nullable=False),
        sa.Column("updated_at", sa.DateTime(timezone=True), server_default=sa.func.now(), nullable=False),
    )
    op.create_index("ix_users_username", "users", ["username"], unique=True)
    op.create_index("ix_users_email", "users", ["email"], unique=True)
    op.create_index("ix_users_google_id", "users", ["google_id"], unique=True)
    op.create_index("ix_users_discord_id", "users", ["discord_id"], unique=True)

    # ── user_sessions ──────────────────────────────────────────
    op.create_table(
        "user_sessions",
        sa.Column("id", sa.String(36), primary_key=True),
        sa.Column("user_id", sa.String(36), sa.ForeignKey("users.id", ondelete="CASCADE"), nullable=False),
        sa.Column("refresh_token_hash", sa.String(255), nullable=False),
        sa.Column("device_info", sa.String(500), nullable=True),
        sa.Column("ip_address", sa.String(50), nullable=True),
        sa.Column("is_revoked", sa.Boolean, nullable=False, server_default="false"),
        sa.Column("expires_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("created_at", sa.DateTime(timezone=True), server_default=sa.func.now(), nullable=False),
    )
    op.create_index("ix_user_sessions_user_id", "user_sessions", ["user_id"])
    op.create_index("ix_user_sessions_refresh_token_hash", "user_sessions", ["refresh_token_hash"], unique=True)

    # ── channels ───────────────────────────────────────────────
    op.create_table(
        "channels",
        sa.Column("id", sa.String(36), primary_key=True),
        sa.Column("name", sa.String(100), nullable=False),
        sa.Column("description", sa.Text, nullable=True),
        sa.Column("type", sa.Enum("text", "announcement", name="channeltype"), nullable=False, server_default="text"),
        sa.Column("is_archived", sa.Boolean, nullable=False, server_default="false"),
        sa.Column("cohort_year", sa.Integer, nullable=True),
        sa.Column("order_index", sa.Integer, nullable=False, server_default="0"),
        sa.Column("created_at", sa.DateTime(timezone=True), server_default=sa.func.now(), nullable=False),
    )

    # ── messages ───────────────────────────────────────────────
    op.create_table(
        "messages",
        sa.Column("id", sa.String(36), primary_key=True),
        sa.Column("channel_id", sa.String(36), sa.ForeignKey("channels.id", ondelete="CASCADE"), nullable=False),
        sa.Column("author_id", sa.String(36), sa.ForeignKey("users.id", ondelete="SET NULL"), nullable=False),
        sa.Column("content", sa.Text, nullable=False),
        sa.Column("status", sa.Enum("active", "withdrawn", "deleted", name="messagestatus"), nullable=False, server_default="active"),
        sa.Column("is_edited", sa.Boolean, nullable=False, server_default="false"),
        sa.Column("is_pinned", sa.Boolean, nullable=False, server_default="false"),
        sa.Column("reply_to_id", sa.String(36), sa.ForeignKey("messages.id", ondelete="SET NULL"), nullable=True),
        sa.Column("forward_from_id", sa.String(36), sa.ForeignKey("messages.id", ondelete="SET NULL"), nullable=True),
        sa.Column("created_at", sa.DateTime(timezone=True), server_default=sa.func.now(), nullable=False),
        sa.Column("updated_at", sa.DateTime(timezone=True), nullable=True),
    )
    op.create_index("ix_messages_channel_id", "messages", ["channel_id"])
    op.create_index("ix_messages_author_id", "messages", ["author_id"])
    op.create_index("ix_messages_reply_to_id", "messages", ["reply_to_id"])

    # ── message_reactions ──────────────────────────────────────
    op.create_table(
        "message_reactions",
        sa.Column("id", sa.String(36), primary_key=True),
        sa.Column("message_id", sa.String(36), sa.ForeignKey("messages.id", ondelete="CASCADE"), nullable=False),
        sa.Column("user_id", sa.String(36), sa.ForeignKey("users.id", ondelete="CASCADE"), nullable=False),
        sa.Column("emoji", sa.String(10), nullable=False),
        sa.Column("created_at", sa.DateTime(timezone=True), server_default=sa.func.now(), nullable=False),
        sa.UniqueConstraint("message_id", "user_id", "emoji", name="uq_reaction"),
    )
    op.create_index("ix_message_reactions_message_id", "message_reactions", ["message_id"])

    # ── audit_logs ─────────────────────────────────────────────
    op.create_table(
        "audit_logs",
        sa.Column("id", sa.String(36), primary_key=True),
        sa.Column("correlation_id", sa.String(36), nullable=False),
        sa.Column("actor_id", sa.String(36), sa.ForeignKey("users.id", ondelete="SET NULL"), nullable=True),
        sa.Column("action", sa.String(100), nullable=False),
        sa.Column("target_type", sa.String(50), nullable=True),
        sa.Column("target_id", sa.String(36), nullable=True),
        sa.Column("ip", sa.String(50), nullable=False),
        sa.Column("user_agent", sa.String(500), nullable=True),
        sa.Column("metadata", sa.JSON, nullable=True),
        sa.Column("created_at", sa.DateTime(timezone=True), server_default=sa.func.now(), nullable=False),
    )
    op.create_index("ix_audit_logs_correlation_id", "audit_logs", ["correlation_id"])
    op.create_index("ix_audit_logs_actor_id", "audit_logs", ["actor_id"])
    op.create_index("ix_audit_logs_action", "audit_logs", ["action"])

    # ── seed: default channels ─────────────────────────────────
    op.execute("""
        INSERT INTO channels (id, name, description, type, is_archived, order_index) VALUES
        ('ch-1', '一般討論', '輕鬆閒聊，什麼都可以聊', 'text', false, 1),
        ('ch-2', '考試分享', '特選考試心得與題目討論', 'text', false, 2),
        ('ch-3', '備審資料', '備審資料製作討論與範本分享', 'text', false, 3),
        ('ch-4', '口試準備', '口試題目與技巧分享', 'text', false, 4),
        ('ch-5', '錄取心得', '錄取後的感想與建議', 'text', false, 5),
        ('ch-6', '公告', '平台重要公告', 'announcement', false, 6),
        ('ch-arch-1', '一般討論', '113 屆封存頻道', 'text', true, 7)
    """)


def downgrade() -> None:
    op.drop_table("audit_logs")
    op.drop_table("message_reactions")
    op.drop_table("messages")
    op.drop_table("channels")
    op.drop_table("user_sessions")
    op.drop_table("users")
    op.execute("DROP TYPE IF EXISTS userrole")
    op.execute("DROP TYPE IF EXISTS verificationstatus")
    op.execute("DROP TYPE IF EXISTS channeltype")
    op.execute("DROP TYPE IF EXISTS messagestatus")
