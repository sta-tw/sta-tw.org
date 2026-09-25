"""OCR fallback for PDF pages that do not expose a usable text layer.

Text extraction remains the fast path.  Only sparse pages are rendered and
sent to the local Tesseract binary, so ordinary text-based PDFs do not pay the
OCR cost.  OCR output is still treated as an extraction candidate and must be
reviewed by an administrator before publication.
"""

from __future__ import annotations

from dataclasses import dataclass
import os
from pathlib import Path
from typing import Any, Callable

from .contracts import InvalidJob, RetryableProcessingError


DEFAULT_LANGUAGE = "chi_tra+eng"
DEFAULT_DPI = 250
DEFAULT_MIN_TEXT_CHARS = 40
DEFAULT_MAX_PAGES = 300
DEFAULT_TIMEOUT_SECONDS = 120
OCR_CONFIG = "--oem 1 --psm 3"


@dataclass(frozen=True)
class OCRSettings:
    enabled: bool
    language: str
    dpi: int
    min_text_chars: int
    max_pages: int
    timeout_seconds: int


def ocr_sparse_pages(path: Path, pages: list[str], logger: Any = None) -> list[str]:
    settings = _settings()
    if not settings.enabled:
        return pages

    page_numbers = [
        page_number
        for page_number, text in enumerate(pages, start=1)
        if page_needs_ocr(text, settings.min_text_chars)
    ]
    if not page_numbers:
        return pages
    if len(page_numbers) > settings.max_pages:
        raise InvalidJob(
            f"document requires OCR on {len(page_numbers)} pages; "
            f"the limit is {settings.max_pages}"
        )

    convert_from_path, pytesseract = _load_dependencies()
    if logger is not None:
        logger.info(
            "OCR fallback processing %d sparse PDF page(s)",
            len(page_numbers),
            extra={"ocr_pages": len(page_numbers), "ocr_language": settings.language},
        )

    extracted = list(pages)
    for page_number in page_numbers:
        try:
            text = _ocr_page(
                convert_from_path,
                pytesseract,
                path,
                page_number,
                settings,
            )
        except Exception as exc:
            raise RetryableProcessingError(
                f"OCR failed on PDF page {page_number}"
            ) from exc
        if text.strip():
            extracted[page_number - 1] = text
        elif logger is not None:
            logger.warning("OCR returned no text for PDF page %d", page_number)
    return extracted


def page_needs_ocr(text: str, min_text_chars: int = DEFAULT_MIN_TEXT_CHARS) -> bool:
    return len("".join(text.split())) < min_text_chars


def _load_dependencies() -> tuple[Callable[..., Any], Any]:
    try:
        from pdf2image import convert_from_path
        import pytesseract
    except ImportError as exc:
        raise RetryableProcessingError("OCR dependencies are not installed") from exc
    return convert_from_path, pytesseract


def _ocr_page(
    convert_from_path: Callable[..., Any],
    pytesseract: Any,
    path: Path,
    page_number: int,
    settings: OCRSettings,
) -> str:
    images = convert_from_path(
        str(path),
        dpi=settings.dpi,
        first_page=page_number,
        last_page=page_number,
        fmt="png",
        grayscale=True,
        thread_count=1,
        use_pdftocairo=True,
        timeout=settings.timeout_seconds,
    )
    try:
        if not images:
            return ""
        return str(
            pytesseract.image_to_string(
                images[0],
                lang=settings.language,
                config=OCR_CONFIG,
                timeout=settings.timeout_seconds,
            )
            or ""
        )
    finally:
        for image in images:
            close = getattr(image, "close", None)
            if close is not None:
                close()


def _settings() -> OCRSettings:
    return OCRSettings(
        enabled=_env_bool("STA_WORKER_OCR_ENABLED", True),
        language=_env_string("STA_WORKER_OCR_LANG", DEFAULT_LANGUAGE),
        dpi=_env_int("STA_WORKER_OCR_DPI", DEFAULT_DPI, minimum=72, maximum=400),
        min_text_chars=_env_int(
            "STA_WORKER_OCR_MIN_TEXT_CHARS",
            DEFAULT_MIN_TEXT_CHARS,
            minimum=1,
            maximum=10_000,
        ),
        max_pages=_env_int(
            "STA_WORKER_OCR_MAX_PAGES",
            DEFAULT_MAX_PAGES,
            minimum=1,
            maximum=2_000,
        ),
        timeout_seconds=_env_int(
            "STA_WORKER_OCR_TIMEOUT",
            DEFAULT_TIMEOUT_SECONDS,
            minimum=1,
            maximum=600,
        ),
    )


def _env_string(name: str, default: str) -> str:
    value = os.environ.get(name, "").strip()
    return value or default


def _env_bool(name: str, default: bool) -> bool:
    value = os.environ.get(name, "").strip().lower()
    if not value:
        return default
    if value in {"1", "true", "yes", "on"}:
        return True
    if value in {"0", "false", "no", "off"}:
        return False
    raise RetryableProcessingError(f"{name} must be a boolean")


def _env_int(name: str, default: int, minimum: int, maximum: int) -> int:
    raw = os.environ.get(name, str(default)).strip()
    try:
        value = int(raw)
    except ValueError as exc:
        raise RetryableProcessingError(f"{name} must be an integer") from exc
    if not minimum <= value <= maximum:
        raise RetryableProcessingError(f"{name} must be between {minimum} and {maximum}")
    return value
