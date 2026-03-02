"""
Email utility — logs to console in dev (SMTP_USER empty).
In production, configure SMTP_* env vars.
"""
import logging
import smtplib
from email.mime.text import MIMEText

from app.config import settings

logger = logging.getLogger(__name__)


def _send_smtp(to: str, subject: str, body: str) -> None:
    msg = MIMEText(body, "html", "utf-8")
    msg["Subject"] = subject
    msg["From"] = settings.email_from
    msg["To"] = to
    with smtplib.SMTP(settings.smtp_host, settings.smtp_port) as smtp:
        smtp.starttls()
        smtp.login(settings.smtp_user, settings.smtp_pass)
        smtp.sendmail(settings.email_from, [to], msg.as_string())


def send_email(to: str, subject: str, body: str) -> None:
    if not settings.smtp_user:
        logger.info("📧 [DEV EMAIL] to=%s | subject=%s\n%s", to, subject, body)
        return
    try:
        _send_smtp(to, subject, body)
    except Exception as exc:
        logger.error("Failed to send email to %s: %s", to, exc)


def send_verification_email(to: str, token: str) -> None:
    url = f"{settings.frontend_url}/auth/verify-email?token={token}"
    send_email(
        to,
        "【STA 論壇】請驗證您的電子郵件",
        f'<p>請點擊以下連結完成驗證（24 小時內有效）：</p><p><a href="{url}">{url}</a></p>',
    )


def send_password_reset_email(to: str, token: str) -> None:
    url = f"{settings.frontend_url}/auth/reset-password?token={token}"
    send_email(
        to,
        "【STA 論壇】重設您的密碼",
        f'<p>請點擊以下連結重設密碼（1 小時內有效）：</p><p><a href="{url}">{url}</a></p>',
    )
