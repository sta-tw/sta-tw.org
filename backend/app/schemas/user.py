from __future__ import annotations

from pydantic import BaseModel, field_validator


class AuthorInfo(BaseModel):
    id: str
    username: str
    displayName: str
    role: str
    avatarUrl: str | None = None


class UserOut(BaseModel):
    id: str
    username: str
    email: str
    displayName: str
    avatarUrl: str | None = None
    role: str
    verificationStatus: str
    reputationScore: int
    bio: str | None = None
    createdAt: str


class UpdateProfileRequest(BaseModel):
    displayName: str | None = None
    bio: str | None = None
    avatarUrl: str | None = None

    @field_validator("displayName")
    @classmethod
    def display_name_length(cls, v: str | None) -> str | None:
        if v is not None and not 1 <= len(v.strip()) <= 100:
            raise ValueError("顯示名稱長度須在 1–100 字元之間")
        return v.strip() if v else v


class AvatarUploadUrlResponse(BaseModel):
    uploadUrl: str | None
    fileKey: str
