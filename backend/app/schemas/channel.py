from __future__ import annotations

from pydantic import BaseModel


class ChannelOut(BaseModel):
    id: str
    name: str
    description: str | None = None
    type: str
    scopeType: str
    schoolCode: str | None = None
    deptCode: str | None = None
    parentId: str | None = None
    isArchived: bool
    cohortYear: int | None = None
    audience: str | None = None
    unreadCount: int = 0
    order: int
    pinnedCount: int | None = None
