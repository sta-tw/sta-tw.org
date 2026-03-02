from __future__ import annotations

from pydantic import BaseModel, field_validator

from app.schemas.user import AuthorInfo


class CreateTicketRequest(BaseModel):
    category: str
    subject: str
    content: str

    @field_validator("subject")
    @classmethod
    def subject_valid(cls, v: str) -> str:
        v = v.strip()
        if not v:
            raise ValueError("主旨不能為空")
        if len(v) > 200:
            raise ValueError("主旨不能超過 200 字元")
        return v

    @field_validator("content")
    @classmethod
    def content_not_empty(cls, v: str) -> str:
        if not v.strip():
            raise ValueError("內容不能為空")
        return v.strip()


class AddMessageRequest(BaseModel):
    content: str

    @field_validator("content")
    @classmethod
    def content_not_empty(cls, v: str) -> str:
        if not v.strip():
            raise ValueError("內容不能為空")
        return v.strip()


class UpdateTicketRequest(BaseModel):
    status: str | None = None
    assigneeId: str | None = None


class TicketMessageOut(BaseModel):
    id: str
    ticketId: str
    authorId: str
    author: AuthorInfo
    content: str
    isStaff: bool
    createdAt: str


class TicketOut(BaseModel):
    id: str
    userId: str
    user: AuthorInfo
    category: str
    subject: str
    status: str
    assigneeId: str | None = None
    createdAt: str
    updatedAt: str
    messages: list[TicketMessageOut] = []
