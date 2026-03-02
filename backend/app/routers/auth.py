"""Auth router — /api/v1/auth/*"""
from fastapi import APIRouter, Cookie, Depends, Request, Response
from fastapi.responses import RedirectResponse
import httpx

from app.config import settings
from app.deps import CurrentUser, DB, get_client_ip
from app.schemas.auth import (
    ChangePasswordRequest,
    ForgotPasswordRequest,
    LoginRequest,
    RefreshResponse,
    RegisterRequest,
    ResendVerificationRequest,
    ResetPasswordRequest,
    SessionOut,
    TokenResponse,
    VerifyEmailRequest,
)
from app.schemas.common import MessageResponse
from app.services import auth_service
from app.utils.security import create_access_token, create_email_token, hash_password
from app.models.user import User, UserRole, VerificationStatus
from app.schemas.user import UserOut
import uuid

router = APIRouter()

REFRESH_COOKIE = "refresh_token"
REFRESH_COOKIE_OPTIONS = {
    "key": REFRESH_COOKIE,
    "httponly": True,
    "samesite": "lax",
    "secure": not settings.debug,
    "max_age": settings.refresh_token_expire_days * 86400,
}


@router.post("/register", response_model=MessageResponse, status_code=201)
async def register(req: RegisterRequest, db: DB):
    await auth_service.register(req, db)
    return MessageResponse(message="註冊成功，請至信箱驗證帳號")


@router.post("/login", response_model=TokenResponse)
async def login(req: LoginRequest, request: Request, response: Response, db: DB):
    ip = get_client_ip(request)
    device = request.headers.get("User-Agent", "")[:500]
    user_out, access_token, refresh_token = await auth_service.login(req, db, ip, device)
    response.set_cookie(value=refresh_token, **REFRESH_COOKIE_OPTIONS)
    return TokenResponse(access_token=access_token, user=user_out)


@router.post("/logout", response_model=MessageResponse)
async def logout(response: Response, db: DB, refresh_token: str | None = Cookie(default=None)):
    await auth_service.logout(refresh_token, db)
    response.delete_cookie(REFRESH_COOKIE)
    return MessageResponse(message="ok")


@router.post("/refresh", response_model=RefreshResponse)
async def refresh(db: DB, refresh_token: str | None = Cookie(default=None)):
    if not refresh_token:
        from fastapi import HTTPException, status
        raise HTTPException(status_code=status.HTTP_401_UNAUTHORIZED, detail="No refresh token")
    access_token = await auth_service.refresh_tokens(refresh_token, db)
    return RefreshResponse(access_token=access_token)


@router.get("/me", response_model=UserOut)
async def me(user: CurrentUser):
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


@router.post("/verify-email", response_model=MessageResponse)
async def verify_email(req: VerifyEmailRequest, db: DB):
    await auth_service.verify_email(req.token, db)
    return MessageResponse(message="信箱驗證成功")


@router.post("/resend-verification", response_model=MessageResponse)
async def resend_verification(req: ResendVerificationRequest, db: DB):
    await auth_service.resend_verification(req.email, db)
    return MessageResponse(message="驗證信已重新發送（若信箱存在）")


@router.post("/forgot-password", response_model=MessageResponse)
async def forgot_password(req: ForgotPasswordRequest, db: DB):
    await auth_service.forgot_password(req.email, db)
    return MessageResponse(message="重設密碼信已發送（若信箱存在）")


@router.post("/reset-password", response_model=MessageResponse)
async def reset_password(req: ResetPasswordRequest, db: DB):
    await auth_service.reset_password(req.token, req.password, db)
    return MessageResponse(message="密碼已重設成功")


@router.post("/change-password", response_model=MessageResponse)
async def change_password(req: ChangePasswordRequest, user: CurrentUser, db: DB):
    await auth_service.change_password(user, req.currentPassword, req.newPassword, db)
    return MessageResponse(message="密碼已更新")


@router.get("/sessions", response_model=list[SessionOut])
async def list_sessions(
    user: CurrentUser,
    db: DB,
    refresh_token: str | None = Cookie(default=None),
):
    current_session_id: str | None = None
    if refresh_token:
        try:
            payload = decode_token(refresh_token, "refresh")
            current_session_id = payload.get("jti")
        except Exception:
            pass
    sessions = await auth_service.list_sessions(user.id, current_session_id, db)
    return sessions


@router.delete("/sessions/{session_id}", response_model=MessageResponse)
async def revoke_session(session_id: str, user: CurrentUser, db: DB):
    await auth_service.revoke_session(user.id, session_id, db)
    return MessageResponse(message="裝置已登出")


@router.delete("/sessions", response_model=MessageResponse)
async def revoke_all_sessions(
    user: CurrentUser,
    db: DB,
    refresh_token: str | None = Cookie(default=None),
):
    keep: str | None = None
    if refresh_token:
        try:
            payload = decode_token(refresh_token, "refresh")
            keep = payload.get("jti")
        except Exception:
            pass
    await auth_service.revoke_all_sessions(user.id, keep, db)
    return MessageResponse(message="其他裝置已全部登出")


# ── OAuth: Google ─────────────────────────────────────────────

GOOGLE_AUTH_URL = "https://accounts.google.com/o/oauth2/v2/auth"
GOOGLE_TOKEN_URL = "https://oauth2.googleapis.com/token"
GOOGLE_USER_URL = "https://www.googleapis.com/oauth2/v3/userinfo"


@router.get("/oauth/google")
async def oauth_google():
    if not settings.google_client_id:
        from fastapi import HTTPException
        raise HTTPException(status_code=501, detail="Google OAuth not configured")
    redirect_uri = f"{settings.frontend_url}/api/v1/auth/oauth/google/callback"
    params = (
        f"?client_id={settings.google_client_id}"
        f"&redirect_uri={redirect_uri}"
        f"&response_type=code"
        f"&scope=openid email profile"
    )
    return RedirectResponse(GOOGLE_AUTH_URL + params)


@router.get("/oauth/google/callback")
async def oauth_google_callback(code: str, response: Response, db: DB):
    redirect_uri = f"{settings.frontend_url}/api/v1/auth/oauth/google/callback"
    async with httpx.AsyncClient() as client:
        token_resp = await client.post(GOOGLE_TOKEN_URL, data={
            "code": code,
            "client_id": settings.google_client_id,
            "client_secret": settings.google_client_secret,
            "redirect_uri": redirect_uri,
            "grant_type": "authorization_code",
        })
        token_resp.raise_for_status()
        id_token = token_resp.json()["access_token"]

        user_resp = await client.get(GOOGLE_USER_URL, headers={"Authorization": f"Bearer {id_token}"})
        user_resp.raise_for_status()
        profile = user_resp.json()

    user_out, access_token, refresh_token = await _oauth_upsert(
        db, google_id=profile["sub"], email=profile["email"],
        display_name=profile.get("name", profile["email"]),
        avatar_url=profile.get("picture"),
    )
    response.set_cookie(value=refresh_token, **REFRESH_COOKIE_OPTIONS)
    return RedirectResponse(f"{settings.frontend_url}/auth/oauth-callback?token={access_token}")


# ── OAuth: Discord ────────────────────────────────────────────

DISCORD_AUTH_URL = "https://discord.com/api/oauth2/authorize"
DISCORD_TOKEN_URL = "https://discord.com/api/oauth2/token"
DISCORD_USER_URL = "https://discord.com/api/users/@me"


@router.get("/oauth/discord")
async def oauth_discord():
    if not settings.discord_client_id:
        from fastapi import HTTPException
        raise HTTPException(status_code=501, detail="Discord OAuth not configured")
    redirect_uri = f"{settings.frontend_url}/api/v1/auth/oauth/discord/callback"
    params = (
        f"?client_id={settings.discord_client_id}"
        f"&redirect_uri={redirect_uri}"
        f"&response_type=code"
        f"&scope=identify email"
    )
    return RedirectResponse(DISCORD_AUTH_URL + params)


@router.get("/oauth/discord/callback")
async def oauth_discord_callback(code: str, response: Response, db: DB):
    redirect_uri = f"{settings.frontend_url}/api/v1/auth/oauth/discord/callback"
    async with httpx.AsyncClient() as client:
        token_resp = await client.post(DISCORD_TOKEN_URL, data={
            "client_id": settings.discord_client_id,
            "client_secret": settings.discord_client_secret,
            "grant_type": "authorization_code",
            "code": code,
            "redirect_uri": redirect_uri,
        })
        token_resp.raise_for_status()
        access_tok = token_resp.json()["access_token"]

        user_resp = await client.get(DISCORD_USER_URL, headers={"Authorization": f"Bearer {access_tok}"})
        user_resp.raise_for_status()
        profile = user_resp.json()

    avatar_url = None
    if profile.get("avatar"):
        avatar_url = f"https://cdn.discordapp.com/avatars/{profile['id']}/{profile['avatar']}.png"

    user_out, access_token, refresh_token = await _oauth_upsert(
        db, discord_id=profile["id"], email=profile.get("email", f"{profile['id']}@discord"),
        display_name=profile.get("global_name") or profile["username"],
        avatar_url=avatar_url,
    )
    response.set_cookie(value=refresh_token, **REFRESH_COOKIE_OPTIONS)
    return RedirectResponse(f"{settings.frontend_url}/auth/oauth-callback?token={access_token}")


async def _oauth_upsert(
    db,
    email: str,
    display_name: str,
    avatar_url: str | None = None,
    google_id: str | None = None,
    discord_id: str | None = None,
) -> tuple[UserOut, str, str]:
    """Find or create a user for OAuth login, return (user_out, access_token, refresh_token)."""
    from sqlalchemy import or_, select
    from datetime import datetime, timedelta, timezone
    from app.utils.security import create_refresh_token, hash_password as hp
    from app.models.user import UserSession

    filter_clause = []
    if google_id:
        filter_clause.append(User.google_id == google_id)
    if discord_id:
        filter_clause.append(User.discord_id == discord_id)
    filter_clause.append(User.email == email)

    result = await db.execute(select(User).where(or_(*filter_clause)))
    user = result.scalar_one_or_none()

    if not user:
        username_base = email.split("@")[0][:45]
        # Ensure unique username
        uname = username_base
        i = 1
        while True:
            r = await db.execute(select(User).where(User.username == uname))
            if not r.scalar_one_or_none():
                break
            uname = f"{username_base}{i}"
            i += 1

        user = User(
            username=uname,
            email=email,
            display_name=display_name,
            avatar_url=avatar_url,
            is_email_verified=True,
            google_id=google_id,
            discord_id=discord_id,
        )
        db.add(user)
        await db.flush()
    else:
        if google_id and not user.google_id:
            user.google_id = google_id
        if discord_id and not user.discord_id:
            user.discord_id = discord_id
        if avatar_url and not user.avatar_url:
            user.avatar_url = avatar_url

    session = UserSession(
        user_id=user.id,
        refresh_token_hash="pending",
        expires_at=datetime.now(timezone.utc) + timedelta(days=settings.refresh_token_expire_days),
    )
    db.add(session)
    await db.flush()

    refresh_tok = create_refresh_token(user.id, session.id)
    session.refresh_token_hash = hp(refresh_tok)
    await db.commit()

    user_out = UserOut(
        id=user.id, username=user.username, email=user.email,
        displayName=user.display_name, avatarUrl=user.avatar_url,
        role=user.role.value, verificationStatus=user.verification_status.value,
        reputationScore=user.reputation_score, bio=user.bio,
        createdAt=user.created_at.isoformat(),
    )
    return user_out, create_access_token(user.id), refresh_tok
