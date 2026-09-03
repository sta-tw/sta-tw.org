from __future__ import annotations

from pathlib import Path
from typing import Any

from .brochure_local import PROGRAM_CODE_PATTERN, extract_local_candidates
from .contracts import (
    BrochureExtractJob,
    ExtractionCandidate,
    InvalidJob,
    RetryableProcessingError,
    result_payload,
    sha256_file,
)


PROGRAM_CODE_LABEL = PROGRAM_CODE_PATTERN


def safe_document_path(root: Path, storage_key: str) -> Path:
    root = root.resolve()
    candidate = (root / storage_key).resolve()
    try:
        candidate.relative_to(root)
    except ValueError as exc:
        raise InvalidJob("document path escapes worker root") from exc
    return candidate


def extract_document(
    job: BrochureExtractJob,
    root: Path,
    processor_version: str,
    max_bytes: int,
    logger=None,
) -> dict[str, Any]:
    path = safe_document_path(root, job.storage_key)
    if not path.is_file():
        raise RetryableProcessingError("document is not available")
    if sha256_file(path, max_bytes) != job.sha256_hex:
        raise InvalidJob("document checksum does not match job")

    pages = _read_pdf_pages(path)
    return result_payload(job, processor_version, _fallback_candidates(job, pages))


def _fallback_candidates(job: BrochureExtractJob, pages: list[str]) -> list[ExtractionCandidate]:
    local_candidates = extract_local_candidates(job, pages)
    if local_candidates:
        # Rule-based extraction is useful but intentionally remains low
        # confidence until an administrator verifies the source evidence.
        return [
            ExtractionCandidate(
                program_code=candidate.program_code,
                data=candidate.data,
                source_page=candidate.source_page,
                confidence=0.2,
            )
            for candidate in local_candidates
        ]
    candidates: list[ExtractionCandidate] = []
    for page_number, text in enumerate(pages, start=1):
        match = PROGRAM_CODE_LABEL.search(text)
        if not match:
            continue
        candidates.append(
            ExtractionCandidate(
                program_code=match.group(1),
                data={"raw_text_excerpt": _safe_excerpt(text)},
                source_page=min(page_number, 999),
                confidence=0.2,
            )
        )
    return candidates


def _read_pdf_pages(path: Path) -> list[str]:
    try:
        from pypdf import PdfReader
    except ImportError as exc:
        raise RetryableProcessingError("pypdf is not installed") from exc
    try:
        reader = PdfReader(str(path), strict=False)
        if len(reader.pages) > 2000:
            raise InvalidJob("document contains too many pages")
        return [page.extract_text() or "" for page in reader.pages]
    except InvalidJob:
        raise
    except Exception as exc:  # pypdf exposes parser-specific exception types.
        raise RetryableProcessingError("PDF parsing failed") from exc


def _safe_excerpt(text: str, limit: int = 2000) -> str:
    return " ".join(text.split())[:limit]
