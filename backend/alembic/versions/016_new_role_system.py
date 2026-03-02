"""New 9-level role system + admission pdf_content + user managed scope

Revision ID: 016
Revises: 015
Create Date: 2026-02-25
"""
from typing import Sequence, Union

from alembic import op
import sqlalchemy as sa

revision: str = "016"
down_revision: Union[str, None] = "015"
branch_labels: Union[str, Sequence[str], None] = None
depends_on: Union[str, Sequence[str], None] = None


def upgrade() -> None:
    # ── 1. Rename existing enum values ──────────────────────────
    op.execute("ALTER TYPE userrole RENAME VALUE 'registered' TO 'visitor'")
    op.execute("ALTER TYPE userrole RENAME VALUE 'active_student' TO 'special_student'")
    op.execute("ALTER TYPE userrole RENAME VALUE 'alumni' TO 'senior'")
    op.execute("ALTER TYPE userrole RENAME VALUE 'moderator' TO 'school_moderator'")
    # 'admin' stays as-is

    # ── 2. Add new enum values ───────────────────────────────────
    op.execute("ALTER TYPE userrole ADD VALUE IF NOT EXISTS 'prospective_student'")
    op.execute("ALTER TYPE userrole ADD VALUE IF NOT EXISTS 'student'")
    op.execute("ALTER TYPE userrole ADD VALUE IF NOT EXISTS 'dept_moderator'")
    op.execute("ALTER TYPE userrole ADD VALUE IF NOT EXISTS 'developer'")

    # ── 3. Update column default ────────────────────────────────
    op.execute("ALTER TABLE users ALTER COLUMN role SET DEFAULT 'visitor'")

    # ── 4. Add moderator scope fields to users ──────────────────
    op.add_column("users", sa.Column("managed_school_code", sa.String(30), nullable=True))
    op.add_column("users", sa.Column("managed_dept_name", sa.String(120), nullable=True))

    # ── 5. Add pdf_content + school_code to admission_documents ──
    op.add_column("admission_documents", sa.Column("pdf_content", sa.LargeBinary(), nullable=True))
    op.add_column("admission_documents", sa.Column("school_code", sa.String(30), nullable=True))
    op.create_index("ix_admission_documents_school_code", "admission_documents", ["school_code"])


def downgrade() -> None:
    op.drop_index("ix_admission_documents_school_code", "admission_documents")
    op.drop_column("admission_documents", "school_code")
    op.drop_column("admission_documents", "pdf_content")
    op.drop_column("users", "managed_dept_name")
    op.drop_column("users", "managed_school_code")
    op.execute("ALTER TABLE users ALTER COLUMN role SET DEFAULT 'visitor'")
    # NOTE: PostgreSQL does not support removing enum values.
    # Rename back what we can:
    op.execute("ALTER TYPE userrole RENAME VALUE 'visitor' TO 'registered'")
    op.execute("ALTER TYPE userrole RENAME VALUE 'special_student' TO 'active_student'")
    op.execute("ALTER TYPE userrole RENAME VALUE 'senior' TO 'alumni'")
    op.execute("ALTER TYPE userrole RENAME VALUE 'school_moderator' TO 'moderator'")
    op.execute("ALTER TABLE users ALTER COLUMN role SET DEFAULT 'registered'")
