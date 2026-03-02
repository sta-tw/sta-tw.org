"""Users router — /api/v1/users/*"""
import uuid

from fastapi import APIRouter, HTTPException
from sqlalchemy import select

from app.deps import CurrentUser, DB
from app.models.user import User
from app.schemas.user import AvatarUploadUrlResponse, UpdateProfileRequest, UserOut
from app.utils import storage

router = APIRouter()


def _user_out(user: User) -> UserOut:
    return UserOut(
        id=user.id, username=user.username, email=user.email,
        displayName=user.display_name, avatarUrl=user.avatar_url,
        role=user.role.value, verificationStatus=user.verification_status.value,
        reputationScore=user.reputation_score, bio=user.bio,
        createdAt=user.created_at.isoformat(),
    )


@router.get("/me", response_model=UserOut)
async def get_me(user: CurrentUser):
    return _user_out(user)


@router.patch("/me", response_model=UserOut)
async def update_me(req: UpdateProfileRequest, user: CurrentUser, db: DB):
    if req.displayName is not None:
        user.display_name = req.displayName
    if req.bio is not None:
        user.bio = req.bio
    if req.avatarUrl is not None:
        user.avatar_url = req.avatarUrl
    await db.commit()
    await db.refresh(user)
    return _user_out(user)


@router.get("/me/avatar-upload-url", response_model=AvatarUploadUrlResponse)
async def avatar_upload_url(user: CurrentUser):
    file_key = f"avatars/{user.id}/{uuid.uuid4()}.jpg"
    upload_url = storage.generate_presigned_upload_url(file_key, content_type="image/jpeg")
    return AvatarUploadUrlResponse(uploadUrl=upload_url, fileKey=file_key)


@router.get("/{username}", response_model=UserOut)
async def get_user(username: str, db: DB, _user: CurrentUser):
    result = await db.execute(select(User).where(User.username == username, User.is_active == True))  # noqa: E712
    user = result.scalar_one_or_none()
    if not user:
        raise HTTPException(status_code=404, detail="User not found")
    return _user_out(user)
