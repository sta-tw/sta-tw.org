"""Authentication business logic."""
from datetime import datetime, timedelta, timezone

from fastapi import HTTPException, status
from jose import JWTError
from sqlalchemy import or_, select
from sqlalchemy.ext.asyncio import AsyncSession

from app.models.user import User, UserSession
from app.schemas.auth import RegisterRequest, LoginRequest
from app.schemas.user import UserOut
from app.utils.security import (
    create_access_token,
    create_email_token,
    create_refresh_token,
    decode_token,
    hash_password,
    verify_password,
)
from app.utils.email import send_verification_email, send_password_reset_email
from app.config import settings


def _user_to_out(user: User) -> UserOut:
    return UserOut(
        id=user.id,
        username=user.username,
        email=user.email,
        displayName=user.display_name,
        avatarUrl=user.avatar_url,
        role=user.role.value,
        verificationStatus=user.verification_status.value,
        reputationScore=user.reputation_score,
        bio=user.bio,
        createdAt=user.created_at.isoformat(),
    )


async def register(req: RegisterRequest, db: AsyncSession) -> None:
    # Check uniqueness
    existing = await db.execute(
        select(User).where(or_(User.email == req.email, User.username == req.username))
    )
    if existing.scalar_one_or_none():
        raise HTTPException(status_code=status.HTTP_409_CONFLICT, detail="Email 或用戶名已被使用")

    user = User(
        username=req.username,
        email=req.email,
        display_name=req.displayName,
        hashed_password=hash_password(req.password),
    )
    db.add(user)
    await db.flush()  # get user.id

    token = create_email_token(user.id, "email_verification")
    await db.commit()
    send_verification_email(user.email, token)


async def login(
    req: LoginRequest,
    db: AsyncSession,
    ip: str = "",
    device: str = "",
) -> tuple[UserOut, str, str]:
    """Returns (user_out, access_token, refresh_token)."""
    result = await db.execute(
        select(User).where(or_(User.email == req.email, User.username == req.email))
    )
    user = result.scalar_one_or_none()

    if not user or not user.hashed_password or not verify_password(req.password, user.hashed_password):
        raise HTTPException(status_code=status.HTTP_401_UNAUTHORIZED, detail="帳號或密碼錯誤")
    if not user.is_active:
        raise HTTPException(status_code=status.HTTP_403_FORBIDDEN, detail="帳號已停用")

    session = UserSession(
        user_id=user.id,
        refresh_token_hash="pending",  # will be updated below
        ip_address=ip,
        device_info=device,
        expires_at=datetime.now(timezone.utc) + timedelta(days=settings.refresh_token_expire_days),
    )
    db.add(session)
    await db.flush()

    refresh_token = create_refresh_token(user.id, session.id)
    # Store hash of the refresh token for revocation
    session.refresh_token_hash = hash_password(refresh_token)
    await db.commit()

    return _user_to_out(user), create_access_token(user.id), refresh_token


async def refresh_tokens(refresh_token: str, db: AsyncSession) -> str:
    """Validate refresh token and issue new access token."""
    try:
        payload = decode_token(refresh_token, "refresh")
    except JWTError:
        raise HTTPException(status_code=status.HTTP_401_UNAUTHORIZED, detail="Invalid refresh token")

    session_id: str = payload["jti"]
    user_id: str = payload["sub"]

    result = await db.execute(
        select(UserSession).where(
            UserSession.id == session_id,
            UserSession.user_id == user_id,
            UserSession.is_revoked == False,  # noqa: E712
        )
    )
    session = result.scalar_one_or_none()
    if not session:
        raise HTTPException(status_code=status.HTTP_401_UNAUTHORIZED, detail="Session expired or revoked")

    if not verify_password(refresh_token, session.refresh_token_hash):
        raise HTTPException(status_code=status.HTTP_401_UNAUTHORIZED, detail="Invalid refresh token")

    return create_access_token(user_id)


async def logout(refresh_token: str | None, db: AsyncSession) -> None:
    if not refresh_token:
        return
    try:
        payload = decode_token(refresh_token, "refresh")
    except JWTError:
        return

    session_id = payload.get("jti")
    if session_id:
        result = await db.execute(select(UserSession).where(UserSession.id == session_id))
        session = result.scalar_one_or_none()
        if session:
            session.is_revoked = True
            await db.commit()


async def verify_email(token: str, db: AsyncSession) -> None:
    try:
        payload = decode_token(token, "email_verification")
        user_id: str = payload["sub"]
    except JWTError:
        raise HTTPException(status_code=status.HTTP_400_BAD_REQUEST, detail="無效或過期的驗證連結")

    result = await db.execute(select(User).where(User.id == user_id))
    user = result.scalar_one_or_none()
    if not user:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="User not found")
    user.is_email_verified = True
    await db.commit()


async def resend_verification(email: str, db: AsyncSession) -> None:
    result = await db.execute(select(User).where(User.email == email))
    user = result.scalar_one_or_none()
    if user and not user.is_email_verified:
        token = create_email_token(user.id, "email_verification")
        send_verification_email(user.email, token)


async def forgot_password(email: str, db: AsyncSession) -> None:
    result = await db.execute(select(User).where(User.email == email))
    user = result.scalar_one_or_none()
    if user:
        token = create_email_token(user.id, "password_reset", hours=1)
        send_password_reset_email(user.email, token)


async def reset_password(token: str, new_password: str, db: AsyncSession) -> None:
    try:
        payload = decode_token(token, "password_reset")
        user_id: str = payload["sub"]
    except JWTError:
        raise HTTPException(status_code=status.HTTP_400_BAD_REQUEST, detail="無效或過期的重設連結")

    result = await db.execute(select(User).where(User.id == user_id))
    user = result.scalar_one_or_none()
    if not user:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="User not found")

    if len(new_password) < 8:
        raise HTTPException(status_code=status.HTTP_422_UNPROCESSABLE_ENTITY, detail="密碼至少 8 個字元")

    user.hashed_password = hash_password(new_password)
    await db.commit()


async def change_password(
    user: User,
    current_password: str,
    new_password: str,
    db: AsyncSession,
) -> None:
    if not user.hashed_password or not verify_password(current_password, user.hashed_password):
        raise HTTPException(status_code=status.HTTP_400_BAD_REQUEST, detail="目前密碼不正確")
    user.hashed_password = hash_password(new_password)
    await db.commit()


async def list_sessions(
    user_id: str,
    current_session_id: str | None,
    db: AsyncSession,
) -> list[dict]:
    result = await db.execute(
        select(UserSession)
        .where(UserSession.user_id == user_id, UserSession.is_revoked == False)  # noqa: E712
        .order_by(UserSession.created_at.desc())
    )
    sessions = result.scalars().all()
    return [
        {
            "id": s.id,
            "deviceInfo": s.device_info,
            "ipAddress": s.ip_address,
            "createdAt": s.created_at.isoformat(),
            "expiresAt": s.expires_at.isoformat(),
            "isCurrent": s.id == current_session_id,
        }
        for s in sessions
    ]


async def revoke_session(user_id: str, session_id: str, db: AsyncSession) -> None:
    result = await db.execute(
        select(UserSession).where(
            UserSession.id == session_id,
            UserSession.user_id == user_id,
        )
    )
    session = result.scalar_one_or_none()
    if not session:
        raise HTTPException(status_code=404, detail="Session not found")
    session.is_revoked = True
    await db.commit()


async def revoke_all_sessions(user_id: str, keep_session_id: str | None, db: AsyncSession) -> None:
    result = await db.execute(
        select(UserSession).where(
            UserSession.user_id == user_id,
            UserSession.is_revoked == False,  # noqa: E712
        )
    )
    for s in result.scalars().all():
        if s.id != keep_session_id:
            s.is_revoked = True
    await db.commit()
