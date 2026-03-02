"""015_scoring_rule_school_abbr

Revision ID: 015
Revises: 014
Create Date: 2026-02-25
"""
from alembic import op
import sqlalchemy as sa

revision = '015'
down_revision = '014'
branch_labels = None
depends_on = None


def upgrade():
    op.add_column("portfolio_scoring_rules",
        sa.Column("school_abbr", sa.String(30), nullable=True))
    op.drop_column("portfolio_scoring_rules", "admission_year")


def downgrade():
    op.add_column("portfolio_scoring_rules",
        sa.Column("admission_year", sa.Integer, nullable=True))
    op.drop_column("portfolio_scoring_rules", "school_abbr")
