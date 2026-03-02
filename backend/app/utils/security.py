from datetime import datetime, timedelta, timezone

from jose import JWTError, jwt
from passlib.context import CryptContext

from app.config import settings

pwd_context = CryptContext(schemes=["bcrypt"], deprecated="auto")
ALGORITHM = "HS256"


def hash_password(password: str) -> str:
    return pwd_context.hash(password)


def verify_password(plain: str, hashed: str) -> bool:
    return pwd_context.verify(plain, hashed)


def create_access_token(user_id: str) -> str:
    expire = datetime.now(timezone.utc) + timedelta(minutes=settings.access_token_expire_minutes)
    return jwt.encode({"sub": user_id, "type": "access", "exp": expire}, settings.secret_key, algorithm=ALGORITHM)


def create_refresh_token(user_id: str, session_id: str) -> str:
    expire = datetime.now(timezone.utc) + timedelta(days=settings.refresh_token_expire_days)
    return jwt.encode(
        {"sub": user_id, "jti": session_id, "type": "refresh", "exp": expire},
        settings.secret_key,
        algorithm=ALGORITHM,
    )


def create_email_token(user_id: str, token_type: str, hours: int = 24) -> str:
    expire = datetime.now(timezone.utc) + timedelta(hours=hours)
    return jwt.encode({"sub": user_id, "type": token_type, "exp": expire}, settings.secret_key, algorithm=ALGORITHM)


def decode_token(token: str, expected_type: str) -> dict:
    """Decode and validate a JWT. Raises JWTError on failure."""
    try:
        payload = jwt.decode(token, settings.secret_key, algorithms=[ALGORITHM])
    except JWTError:
        raise
    if payload.get("type") != expected_type:
        raise JWTError(f"Expected token type '{expected_type}', got '{payload.get('type')}'")
    return payload
