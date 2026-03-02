from __future__ import annotations

from pydantic import BaseModel

from app.schemas.user import AuthorInfo


class MessageReplyOut(BaseModel):
    id: str
    authorDisplayName: str
    content: str


class ReactionOut(BaseModel):
    emoji: str
    count: int
    userIds: list[str]


class MessageOut(BaseModel):
    id: str
    channelId: str
    authorId: str
    author: AuthorInfo
    content: str
    status: str
    createdAt: str
    updatedAt: str | None = None
    isEdited: bool
    isPinned: bool
    replyTo: MessageReplyOut | None = None
    reactions: list[ReactionOut]
    threadCount: int
    forwardFromId: str | None = None


class MessagesListOut(BaseModel):
    messages: list[MessageOut]
    hasMore: bool
    nextCursor: str | None = None


class CreateMessageRequest(BaseModel):
    content: str
    replyToId: str | None = None


class EditMessageRequest(BaseModel):
    content: str


class ReactRequest(BaseModel):
    emoji: str


class ForwardRequest(BaseModel):
    targetChannelId: str
