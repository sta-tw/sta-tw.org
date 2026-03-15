from __future__ import annotations

from fastapi import APIRouter
from sqlalchemy import func, select

from app.deps import DB
from app.models.admission import AdmissionDocument
from app.models.portfolio import PortfolioDocument
from app.models.user import User, UserRole
from app.schemas.public import PublicStatsOut

router = APIRouter()


@router.get("/stats", response_model=PublicStatsOut)
async def get_public_stats(db: DB):
    portfolio_row = await db.execute(
        select(func.count())
        .select_from(PortfolioDocument)
        .where(PortfolioDocument.is_approved == True)  # noqa: E712
    )
    public_portfolio_count = portfolio_row.scalar_one()

    admission_row = await db.execute(
        select(func.count()).select_from(AdmissionDocument)
    )
    admission_count = admission_row.scalar_one()

    student_roles = (
        UserRole.special_student,
        UserRole.prospective_student,
        UserRole.student,
        UserRole.senior,
    )
    student_row = await db.execute(
        select(func.count())
        .select_from(User)
        .where(User.is_active == True, User.role.in_(student_roles))  # noqa: E712
    )
    special_student_count = student_row.scalar_one()

    return PublicStatsOut(
        publicPortfolioCount=public_portfolio_count,
        admissionCount=admission_count,
        specialStudentCount=special_student_count,
    )
