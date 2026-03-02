from __future__ import annotations

from pydantic import BaseModel


class StatsOut(BaseModel):
    totalUsers: int
    usersByRole: dict[str, int]
    activeTickets: int
    pendingVerifications: int
    totalChannels: int
    messagesLast24h: int


class AdminUserOut(BaseModel):
    id: str
    username: str
    email: str
    displayName: str
    avatarUrl: str | None = None
    role: str
    verificationStatus: str
    reputationScore: int
    isActive: bool
    isEmailVerified: bool
    managedSchoolCode: str | None = None
    managedDeptName: str | None = None
    createdAt: str


class AdminUserListOut(BaseModel):
    items: list[AdminUserOut]
    total: int
    page: int
    pageSize: int


class UpdateUserRequest(BaseModel):
    role: str | None = None
    isActive: bool | None = None
    managedSchoolCode: str | None = None
    managedDeptName: str | None = None


class CreateChannelRequest(BaseModel):
    name: str
    description: str | None = None
    type: str = "text"
    scopeType: str = "global"
    schoolCode: str | None = None
    deptCode: str | None = None
    parentId: str | None = None
    cohortYear: int | None = None
    audience: str | None = None
    order: int = 0


class UpdateChannelRequest(BaseModel):
    name: str | None = None
    description: str | None = None
    type: str | None = None
    scopeType: str | None = None
    schoolCode: str | None = None
    deptCode: str | None = None
    parentId: str | None = None
    isArchived: bool | None = None
    cohortYear: int | None = None
    audience: str | None = None
    order: int | None = None


class AuditLogOut(BaseModel):
    id: str
    correlationId: str
    actorId: str | None = None
    actorDisplayName: str | None = None
    action: str
    targetType: str | None = None
    targetId: str | None = None
    ip: str
    userAgent: str | None = None
    createdAt: str


class AuditLogListOut(BaseModel):
    items: list[AuditLogOut]
    total: int
    page: int
    pageSize: int


class ReputationEventOut(BaseModel):
    id: str
    userId: str
    delta: int
    reason: str
    createdAt: str
    actorId: str | None = None
    actorDisplayName: str | None = None


class ReputationDetailOut(BaseModel):
    userId: str
    username: str
    displayName: str
    currentScore: int
    events: list[ReputationEventOut]


class AdjustReputationRequest(BaseModel):
    delta: int
    reason: str
