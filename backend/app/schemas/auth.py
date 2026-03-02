from pydantic import BaseModel, EmailStr, field_validator


class RegisterRequest(BaseModel):
    username: str
    email: EmailStr
    displayName: str
    password: str

    @field_validator("username")
    @classmethod
    def username_alphanumeric(cls, v: str) -> str:
        v = v.strip()
        if not 3 <= len(v) <= 50:
            raise ValueError("用戶名長度須在 3-50 字元之間")
        return v

    @field_validator("password")
    @classmethod
    def password_length(cls, v: str) -> str:
        if len(v) < 8:
            raise ValueError("密碼至少 8 個字元")
        return v


class LoginRequest(BaseModel):
    email: str  # accepts email or username
    password: str


class TokenResponse(BaseModel):
    access_token: str
    user: "UserOut"


class RefreshResponse(BaseModel):
    access_token: str


class VerifyEmailRequest(BaseModel):
    token: str


class ForgotPasswordRequest(BaseModel):
    email: EmailStr


class ResetPasswordRequest(BaseModel):
    token: str
    password: str


class ResendVerificationRequest(BaseModel):
    email: EmailStr


class ChangePasswordRequest(BaseModel):
    currentPassword: str
    newPassword: str

    @field_validator("newPassword")
    @classmethod
    def new_password_length(cls, v: str) -> str:
        if len(v) < 8:
            raise ValueError("新密碼至少 8 個字元")
        return v


class SessionOut(BaseModel):
    id: str
    deviceInfo: str | None
    ipAddress: str | None
    createdAt: str
    expiresAt: str
    isCurrent: bool = False


# Forward ref
from app.schemas.user import UserOut  # noqa: E402, F401

TokenResponse.model_rebuild()
