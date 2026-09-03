from __future__ import annotations

from dataclasses import dataclass
from datetime import datetime, timezone
import hashlib
import re
from typing import Any, Mapping


SCHOOL_CODE_RE = re.compile(r"^[0-9]{3}$")
SHA256_RE = re.compile(r"^[0-9a-f]{64}$")
PROGRAM_CODE_RE = re.compile(r"^[0-9]{3}$")


class InvalidJob(ValueError):
    """The message is malformed and must not be retried."""


class RetryableProcessingError(RuntimeError):
    """The document may be processed again after an operational failure."""


@dataclass(frozen=True)
class BrochureExtractJob:
    job_id: str
    academic_year: int
    school_code: str
    storage_key: str
    sha256_hex: str
    requested_at: str
    processor_hint: str = ""
    source_type: str = "brochure"
    source_url: str = ""
    program_code: str = ""

    @classmethod
    def from_mapping(cls, payload: Mapping[str, Any]) -> "BrochureExtractJob":
        required = ("job_id", "academic_year", "school_code", "storage_key", "sha256_hex", "requested_at")
        if any(key not in payload for key in required):
            raise InvalidJob("required job field is missing")
        try:
            year = int(payload["academic_year"])
        except (TypeError, ValueError) as exc:
            raise InvalidJob("academic_year is invalid") from exc
        job = cls(
            job_id=str(payload["job_id"]),
            academic_year=year,
            school_code=str(payload["school_code"]),
            storage_key=str(payload["storage_key"]),
            sha256_hex=str(payload["sha256_hex"]),
            requested_at=str(payload["requested_at"]),
            processor_hint=str(payload.get("processor_hint", "")),
            source_type=str(payload.get("source_type", "brochure")) or "brochure",
            source_url=str(payload.get("source_url", "")),
            program_code=str(payload.get("program_code", "")),
        )
        job.validate()
        return job

    def validate(self) -> None:
        if not self.job_id or len(self.job_id) > 128:
            raise InvalidJob("job_id is invalid")
        if not 100 <= self.academic_year <= 999:
            raise InvalidJob("academic_year is invalid")
        if not SCHOOL_CODE_RE.fullmatch(self.school_code):
            raise InvalidJob("school_code is invalid")
        if not self.storage_key or len(self.storage_key) > 1024 or self.storage_key.startswith("/"):
            raise InvalidJob("storage_key is invalid")
        if any(ord(character) < 32 for character in self.storage_key):
            raise InvalidJob("storage_key contains a control character")
        if ".." in self.storage_key.replace("\\", "/").split("/"):
            raise InvalidJob("storage_key contains a parent traversal")
        if not SHA256_RE.fullmatch(self.sha256_hex):
            raise InvalidJob("sha256_hex is invalid")
        if not self.requested_at:
            raise InvalidJob("requested_at is required")
        if self.source_type not in {"brochure", "candidate_list"}:
            raise InvalidJob("source_type is invalid")
        if self.program_code and not PROGRAM_CODE_RE.fullmatch(self.program_code):
            raise InvalidJob("program_code is invalid")


@dataclass(frozen=True)
class ExtractionCandidate:
    program_code: str
    data: dict[str, Any]
    source_page: int = 0
    confidence: float | None = None

    def validate(self) -> None:
        if not PROGRAM_CODE_RE.fullmatch(self.program_code):
            raise InvalidJob("program_code is invalid")
        if not self.data:
            raise InvalidJob("candidate data is empty")
        if not 0 <= self.source_page <= 999:
            raise InvalidJob("source_page is invalid")
        if self.confidence is not None and not 0 <= self.confidence <= 1:
            raise InvalidJob("confidence is invalid")


def result_payload(job: BrochureExtractJob, processor: str, candidates: list[ExtractionCandidate]) -> dict[str, Any]:
    for candidate in candidates:
        candidate.validate()
    return {
        "result_type": "brochure",
        "job_id": job.job_id,
        "academic_year": job.academic_year,
        "school_code": job.school_code,
        "sha256_hex": job.sha256_hex,
        "processor": processor,
        "candidates": [
            {
                "program_code": candidate.program_code,
                "data": candidate.data,
                "source_page": candidate.source_page,
                "confidence": candidate.confidence,
            }
            for candidate in candidates
        ],
        "generated_at": datetime.now(timezone.utc).isoformat(),
    }


def sha256_file(path, max_bytes: int) -> str:
    digest = hashlib.sha256()
    total = 0
    with path.open("rb") as source:
        while True:
            chunk = source.read(1024 * 1024)
            if not chunk:
                break
            total += len(chunk)
            if total > max_bytes:
                raise RetryableProcessingError("document exceeds worker size limit")
            digest.update(chunk)
    return digest.hexdigest()
