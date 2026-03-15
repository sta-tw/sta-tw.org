from __future__ import annotations

from pydantic import BaseModel


class PublicStatsOut(BaseModel):
    publicPortfolioCount: int
    admissionCount: int
    specialStudentCount: int
