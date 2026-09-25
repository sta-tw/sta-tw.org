"""Cloudflare Workers AI extraction for admission brochures.

This is an alternative to the rule-based parser in ``brochure_local``. When
``STA_WORKER_AI_ENABLED`` is set and credentials are present, the worker sends
the PDF text (still produced locally by pypdf + Tesseract OCR) to a Workers AI
chat model and asks for the structured programme list. The output is still an
extraction *candidate* — an administrator confirms every field before it is
published — so the model is asked to be literal, not creative.

If the API is unreachable it raises ``RetryableProcessingError`` (the job is
retried). If it answers but the answer cannot be used, it returns ``None`` and
the caller falls back to the rule-based parser.
"""

from __future__ import annotations

from dataclasses import dataclass
import json
import os
import re
import urllib.error
import urllib.request
from typing import Any

from .brochure_local import BrochureIdentity
from .contracts import BrochureExtractJob, ExtractionCandidate, RetryableProcessingError

PROCESSOR_SUFFIX = "cf-workers-ai"


class _PermanentAIError(Exception):
    """A request the API rejected for good (bad model, prompt too long, auth).

    Retrying is pointless; the caller drops back to the rule-based parser.
    """

_DEFAULT_MODEL = "@cf/mistralai/mistral-small-3.1-24b-instruct"
_DEFAULT_BASE_URL = "https://api.cloudflare.com/client/v4"

# Deliberately narrow while the AI extraction pipeline is still new: only
# 校系基本資料 (name/quota) and 聯絡與來源 (contact info + the brochure's own
# URL) are asked for directly. Everything else the extraction *candidate*
# schema supports (exam item weights/descriptions, special_talent_target,
# registration_fee, deadlines, ...) is left for an admin to fill in by hand
# for now — expand this list once the extraction quality on these fields is
# proven out.
_PROGRAM_FIELDS: tuple[str, ...] = (
    "program_code",
    "admission_program_name",
    "admission_quota",
    "consultation_phone",
    "consultation_email",
    "consultation_contact",
    "brochure_url",
)

_SYSTEM_PROMPT = (
    "You extract a narrow slice of structured admissions data from Taiwanese "
    "university special-selection (特殊選才) brochures. Reply with a single "
    "JSON object and nothing else. Only extract: each programme's basic info "
    "(name and admission quota), its admissions timeline, and its contact "
    "info / source. Do NOT extract exam item weights or descriptions, "
    "special_talent_target, different-education-background eligibility, "
    "notes, registration fee, exam location, recommendation-letter/portfolio "
    "deadlines, check-in/waitlist process, or fee-reduction eligibility — "
    "those are filled in by hand for now. Rules: academic_year is the "
    "3-digit ROC year (e.g. 116). All dates are Gregorian YYYY-MM-DD. Convert "
    "by adding 1911 to the ROC year printed next to that specific date -- "
    "e.g. 民國115年9月30日 is 2026-09-30 -- never just prepend \"20\" to the ROC "
    "year (that gives the wrong decade). A date's ROC year is whatever the "
    "brochure prints beside it and is NOT the same number as academic_year; "
    "do not assume they match. Keep every programme/department that has an "
    "admission quota. Split contact info: consultation_phone is only the "
    "number(s) and extension, consultation_email is only the address, "
    "consultation_contact is the person or unit. timeline_events is a flat, "
    "ordered list of every schedule milestone the brochure prints for that "
    "programme, in the order the brochure lists them -- this includes the "
    "brochure's own announcement date, the registration window, the exam/"
    "interview date(s), and the result/放榜 date, as well as upload "
    "deadlines, recommendation-letter deadline, second-round/interview list "
    "announcement, check-in, waitlist promotion, and waitlist deadline. Do "
    "NOT include a generic \"查詢報名狀態\"/\"報名結果公告\" entry for checking "
    "whether the application itself went through (the second-round/複試 list "
    "announcement is fine and expected, this is only about the earlier \"did "
    "my submission succeed\" status check), or a registration-fee refund "
    "deadline — neither is a useful schedule milestone for an applicant. "
    "Keep the milestone's own wording as `name`. Each milestone has "
    "start_date/start_time and, only when the brochure gives a real end "
    "point, end_date/end_time. start_date is Gregorian YYYY-MM-DD when the "
    "brochure gives one specific day to start from, else \"-\". start_time is "
    "HH:MM (24-hour) when the brochure gives a specific time of day for the "
    "start, else \"-\". Leave end_date/end_time as \"-\" for a one-day/point-"
    "in-time milestone (e.g. a single announcement date) -- only fill them in "
    "when the brochure genuinely spans a range (e.g. a registration window "
    "9/29~10/7, or a same-day 9:00~17:00 window: for a same-day time window, "
    "end_date should repeat start_date). Use the string \"-\" for anything the "
    "brochure does not state. Do not invent data. Copy every Chinese name and "
    "phrase EXACTLY as printed — keep Traditional Chinese, never convert to "
    "Simplified, never substitute a character."
)

_SCHEMA_HINT = (
    '{"academic_year": 116, "school_code": "-", "school_name": "…", '
    '"programs": [{"program_code": "-", "admission_program_name": "…", '
    '"admission_quota": 0, "source_page": 0, '
    '"consultation_phone": "-", '
    '"consultation_email": "-", "consultation_contact": "-", "brochure_url": "-", '
    '"timeline_events": [{"name": "招生簡章公告", "start_date": "2026-08-15", '
    '"start_time": "-", "end_date": "-", "end_time": "-", "notes": "-"}, '
    '{"name": "網路報名", "start_date": "2026-09-29", "start_time": "09:00", '
    '"end_date": "2026-10-07", "end_time": "17:00", "notes": "-"}]'
    '}]}'
)


@dataclass(frozen=True)
class AISettings:
    enabled: bool
    account_id: str
    api_token: str
    model: str
    base_url: str
    timeout_seconds: int
    max_input_chars: int
    max_output_tokens: int
    max_chunks: int
    confidence: float

    @property
    def usable(self) -> bool:
        return bool(self.enabled and self.account_id and self.api_token)


def _env(name: str, *aliases: str) -> str:
    for key in (name, *aliases):
        value = os.environ.get(key, "").strip()
        if value:
            return value
    return ""


def _env_int(name: str, default: int) -> int:
    try:
        return int(os.environ.get(name, "").strip() or default)
    except ValueError:
        return default


def _env_float(name: str, default: float) -> float:
    try:
        return float(os.environ.get(name, "").strip() or default)
    except ValueError:
        return default


def load_ai_settings() -> AISettings:
    flag = os.environ.get("STA_WORKER_AI_ENABLED", "").strip().lower()
    return AISettings(
        enabled=flag in {"1", "true", "yes", "on"},
        account_id=_env("STA_WORKER_AI_ACCOUNT_ID"),
        api_token=_env("STA_WORKER_AI_API_TOKEN", "STA_WORKER_AI_TOKEN"),
        model=_env("STA_WORKER_AI_MODEL") or _DEFAULT_MODEL,
        base_url=(_env("STA_WORKER_AI_BASE_URL") or _DEFAULT_BASE_URL).rstrip("/"),
        timeout_seconds=_env_int("STA_WORKER_AI_TIMEOUT", 120),
        # Context windows vary a lot by model: llama-3.3-70b-fp8-fast is only
        # 24k *total* tokens, llama-4-scout takes far more. CJK text is roughly
        # one token per character; keep the per-call text under the model's
        # window once the prompt and requested output are subtracted. A prompt
        # that is still too long fails with a 4xx and the caller falls back.
        max_input_chars=max(2000, _env_int("STA_WORKER_AI_MAX_INPUT_CHARS", 40000)),
        max_output_tokens=max(512, _env_int("STA_WORKER_AI_MAX_OUTPUT_TOKENS", 3072)),
        max_chunks=max(1, _env_int("STA_WORKER_AI_MAX_CHUNKS", 16)),
        confidence=min(0.9, max(0.05, _env_float("STA_WORKER_AI_CONFIDENCE", 0.3))),
    )


def ai_extract_brochure(
    job: BrochureExtractJob,
    pages: list[str],
    logger: Any = None,
    *,
    settings: AISettings | None = None,
    caller: Any = None,
) -> tuple[BrochureIdentity, list[ExtractionCandidate]] | None:
    """Return ``(identity, candidates)`` from Workers AI, or ``None`` to fall back."""

    settings = settings or load_ai_settings()
    if not settings.usable:
        return None

    run = caller or _CloudflareRunner(settings)
    chunks = _chunk_pages(pages, settings.max_input_chars, settings.max_chunks)
    if not chunks:
        return None

    academic_year = job.academic_year
    school_code = job.school_code
    school_name = ""
    merged: dict[str, dict[str, Any]] = {}

    for index, chunk in enumerate(chunks):
        try:
            payload = _ask(run, chunk[: settings.max_input_chars], first=index == 0)
        except _PermanentAIError as exc:
            if logger is not None:
                logger.warning(
                    "Workers AI rejected the request (%s); using the rule-based parser",
                    exc,
                )
            return None
        if payload is None:
            continue
        if index == 0:
            academic_year = academic_year or _as_year(payload.get("academic_year"))
            school_code = school_code or _as_school_code(payload.get("school_code"))
            school_name = school_name or _as_text(payload.get("school_name"))
        for raw in payload.get("programs") or []:
            if not isinstance(raw, dict):
                continue
            name = _as_text(raw.get("admission_program_name"))
            if len(name) < 2:
                continue
            key = re.sub(r"\s+", "", name)
            merged.setdefault(key, {})
            _merge_program(merged[key], raw)

    if not merged:
        if logger is not None:
            logger.warning("Workers AI returned no usable programmes; using rules")
        return None

    identity = BrochureIdentity(
        academic_year=academic_year or 0,
        school_code=school_code or "",
        school_name=school_name,
    )
    candidates = _to_candidates(merged, identity, settings, pages)
    if logger is not None:
        logger.info(
            "Workers AI extracted %d programme candidate(s)",
            len(candidates),
            extra={"ai_model": settings.model, "ai_chunks": len(chunks)},
        )
    return identity, candidates


# --- Cloudflare call -------------------------------------------------------


class _CloudflareRunner:
    def __init__(self, settings: AISettings) -> None:
        self._settings = settings

    def __call__(self, messages: list[dict[str, str]]) -> str:
        settings = self._settings
        url = f"{settings.base_url}/accounts/{settings.account_id}/ai/run/{settings.model}"
        body = json.dumps(
            {
                "messages": messages,
                "max_tokens": settings.max_output_tokens,
                "temperature": 0.1,
                "response_format": {"type": "json_object"},
            }
        ).encode("utf-8")
        request = urllib.request.Request(
            url,
            data=body,
            headers={
                "Authorization": f"Bearer {settings.api_token}",
                "Content-Type": "application/json",
            },
            method="POST",
        )
        try:
            with urllib.request.urlopen(request, timeout=settings.timeout_seconds) as response:
                raw = response.read().decode("utf-8")
        except urllib.error.HTTPError as exc:
            detail = exc.read().decode("utf-8", "replace")[:2000]
            summary = _summarize_cloudflare_error(f"Workers AI HTTP {exc.code}", detail)
            # 4xx (bad prompt/model/token) will fail identically on retry — give
            # up so the caller falls back. A 429 for the daily free-Neuron
            # allocation (Cloudflare error code 4006) will not clear within this
            # job's retry window either, so it gets the same treatment; other
            # 429s (a burst of concurrent requests) and 5xx are worth retrying.
            if 400 <= exc.code < 500 and (exc.code != 429 or "4006" in detail):
                raise _PermanentAIError(summary) from exc
            raise RetryableProcessingError(summary) from exc
        except (urllib.error.URLError, TimeoutError, OSError) as exc:
            raise RetryableProcessingError(f"Workers AI unreachable: {exc}") from exc

        try:
            envelope = json.loads(raw)
        except json.JSONDecodeError as exc:
            raise RetryableProcessingError("Workers AI response was not JSON") from exc
        if not envelope.get("success", True):
            raise RetryableProcessingError(
                _summarize_cloudflare_error(
                    "Workers AI error", json.dumps(envelope.get("errors"))
                )
            )
        result = envelope.get("result", envelope)
        response = result.get("response", result) if isinstance(result, dict) else result
        if isinstance(response, (dict, list)):
            return json.dumps(response, ensure_ascii=False)
        return str(response)


def _ask(run: Any, chunk: str, *, first: bool) -> dict[str, Any] | None:
    scope = (
        "This is the start of the brochure; also fill academic_year, "
        "school_code and school_name."
        if first
        else "Continuation of the same brochure; academic_year/school may be "
        "absent here, focus on the programmes."
    )
    user = (
        f"{scope}\nReturn JSON shaped like:\n{_SCHEMA_HINT}\n\n"
        f"Brochure text:\n{chunk}"
    )
    messages = [
        {"role": "system", "content": _SYSTEM_PROMPT},
        {"role": "user", "content": user},
    ]
    text = run(messages)
    return _parse_json_object(text)


def _parse_json_object(text: str) -> dict[str, Any] | None:
    text = text.strip()
    fence = re.search(r"```(?:json)?\s*(.+?)\s*```", text, re.DOTALL)
    if fence:
        text = fence.group(1).strip()
    start = text.find("{")
    end = text.rfind("}")
    if start == -1 or end <= start:
        return None
    try:
        value = json.loads(text[start : end + 1])
    except json.JSONDecodeError:
        return None
    return value if isinstance(value, dict) else None


# --- shaping -------------------------------------------------------------


def _chunk_pages(pages: list[str], max_chars: int, max_chunks: int) -> list[str]:
    chunks: list[str] = []
    current: list[str] = []
    size = 0
    for page in pages:
        page = page or ""
        if size and size + len(page) > max_chars:
            chunks.append("\n\n".join(current))
            current, size = [], 0
            if len(chunks) >= max_chunks:
                return chunks
        current.append(page)
        size += len(page) + 2
    if current and len(chunks) < max_chunks:
        chunks.append("\n\n".join(current))
    return chunks


def _merge_program(into: dict[str, Any], raw: dict[str, Any]) -> None:
    for field in _PROGRAM_FIELDS:
        value = raw.get(field)
        if field == "admission_quota":
            if into.get(field) in (None, 0) and isinstance(value, (int, float)):
                into[field] = int(value)
            continue
        text = _as_text(value)
        if text and text != "-" and not _as_text(into.get(field)):
            into[field] = text
    if not into.get("source_page"):
        page = raw.get("source_page")
        if isinstance(page, (int, float)) and 1 <= int(page) <= 999:
            into["source_page"] = int(page)
    if not into.get("timeline_events"):
        events = [
            event for event in (raw.get("timeline_events") or []) if isinstance(event, dict)
        ]
        if events:
            into["timeline_events"] = events


def _to_candidates(
    merged: dict[str, dict[str, Any]],
    identity: BrochureIdentity,
    settings: AISettings,
    pages: list[str],
) -> list[ExtractionCandidate]:
    candidates: list[ExtractionCandidate] = []
    for order, raw in enumerate(merged.values(), start=1):
        code = f"{order:03d}"
        printed = _as_text(raw.get("program_code"))
        source_page = int(raw.get("source_page") or 0)
        timeline_events = _clean_timeline_events(raw.get("timeline_events") or [])
        data: dict[str, Any] = {
            "document_type": "brochure",
            "academic_year": str(identity.academic_year) if identity.academic_year else "-",
            "school_code": identity.school_code or "-",
            "school_name": identity.school_name or "-",
            "program_code": code,
            "source_program_code": printed if re.fullmatch(r"[A-Za-z0-9]{2,12}", printed) else "-",
            "admission_program_name": _as_text(raw.get("admission_program_name")) or "-",
            "admission_quota": int(raw["admission_quota"]) if isinstance(raw.get("admission_quota"), (int, float)) else None,
            "consultation_phone": _as_text(raw.get("consultation_phone")) or "-",
            "consultation_email": _as_text(raw.get("consultation_email")) or "-",
            "consultation_contact": _as_text(raw.get("consultation_contact")) or "-",
            "brochure_url": _as_text(raw.get("brochure_url")) or "-",
            "timeline_events": timeline_events,
            "evidence": [],
            "extractor": PROCESSOR_SUFFIX,
        }
        if 1 <= source_page <= len(pages):
            data["raw_text_excerpt"] = " ".join(pages[source_page - 1].split())[:3000]
        candidates.append(
            ExtractionCandidate(
                program_code=code,
                data=data,
                source_page=min(max(source_page, 0), 999),
                confidence=settings.confidence,
            )
        )
    return candidates[:2000]


# Milestones excluded from every timeline: a generic application-status
# check and the registration-fee refund deadline. Nobody finds these useful.
_EXCLUDED_TIMELINE_PATTERNS = (
    re.compile(r"查詢報名狀態"),
    re.compile(r"報名結果(公告)?"),
    re.compile(r"退費"),
)


def _is_excluded_timeline_event(name: str) -> bool:
    return any(pattern.search(name) for pattern in _EXCLUDED_TIMELINE_PATTERNS)


def _clean_timeline_events(events: list[Any]) -> list[dict[str, Any]]:
    cleaned: list[dict[str, Any]] = []
    for event in events:
        if not isinstance(event, dict):
            continue
        name = _as_text(event.get("name"))
        if not name or name == "-":
            continue
        if _is_excluded_timeline_event(name):
            continue
        cleaned.append(
            {
                "name": name[:500],
                "start_date": _as_date(event.get("start_date")),
                "start_time": _as_time(event.get("start_time")),
                "end_date": _as_date(event.get("end_date")),
                "end_time": _as_time(event.get("end_time")),
                "sort_order": len(cleaned) + 1,
                "notes": _as_text(event.get("notes"))[:2000] or "-",
            }
        )
    return cleaned[:100]


_MESSAGE_FIELD_PATTERN = re.compile(r'\\?"message\\?"\s*:\s*\\?"(.*?)(?<!\\)\\?"')
_TRAILING_REQUEST_ID_PATTERN = re.compile(r"\s*\([0-9a-f]{8}-[0-9a-f-]{27,}\)\s*$")
_TRAILING_PARAMETER_NOTE_PATTERN = re.compile(r"\s*\(parameter=[^)]*\)\s*$")


def _summarize_cloudflare_error(label: str, raw: str) -> str:
    """Collapse Cloudflare's doubly-nested AiError JSON into one short line.

    Workers AI wraps the model's own error inside an ``errors[].message``
    string that is itself escaped JSON (and sometimes wraps that again), plus
    a trailing request id. Keep only the innermost, human-readable sentence so
    a failed job shows something a reviewer can actually read.
    """

    raw = raw.strip()
    matches = _MESSAGE_FIELD_PATTERN.findall(raw)
    text = matches[-1] if matches else raw
    text = text.replace('\\"', '"').replace("AiError: ", "").strip()
    text = _TRAILING_REQUEST_ID_PATTERN.sub("", text)
    text = _TRAILING_PARAMETER_NOTE_PATTERN.sub("", text)
    if len(text) > 140:
        text = text[:137] + "…"
    return f"{label}: {text}" if text else label


def _as_text(value: Any) -> str:
    if value is None:
        return ""
    return str(value).strip()


def _as_year(value: Any) -> int:
    text = _as_text(value)
    match = re.search(r"1[0-9]{2}", text)
    return int(match.group(0)) if match else 0


def _as_school_code(value: Any) -> str:
    text = _as_text(value)
    return text if re.fullmatch(r"[0-9]{3}", text) else ""


# The model is asked to convert ROC (民國) years to Gregorian itself, but its
# arithmetic on 3-digit years is unreliable -- it will confidently emit an
# already-4-digit-looking year like "2016" for 民國116年 (dropping the leading
# "1" and prepending "20" instead of adding 1911). A 4-digit year is therefore
# not proof the model did the conversion correctly, so any parsed date outside
# this admissions-timeline-plausible window is treated as malformed rather
# than trusted -- consistent with every other AI field, it stays "-" for an
# administrator to fill in from the source PDF rather than silently publishing
# a wrong decade.
_PLAUSIBLE_YEAR_RANGE = range(2020, 2036)


def _as_date(value: Any) -> str:
    text = _as_text(value)
    match = re.match(r"(\d{4})-(\d{2})-(\d{2})", text)
    if match:
        year = int(match.group(1))
        if year not in _PLAUSIBLE_YEAR_RANGE:
            return "-"
        return match.group(0)
    roc = re.match(r"(\d{2,3})\D+(\d{1,2})\D+(\d{1,2})", text)
    if roc:
        year = int(roc.group(1))
        if year < 1000:
            year += 1911
        if year not in _PLAUSIBLE_YEAR_RANGE:
            return "-"
        return f"{year:04d}-{int(roc.group(2)):02d}-{int(roc.group(3)):02d}"
    return "-"


_TIME_PATTERN = re.compile(r"([01]?\d|2[0-3]):([0-5]\d)")


def _as_time(value: Any) -> str:
    match = _TIME_PATTERN.search(_as_text(value))
    return f"{int(match.group(1)):02d}:{match.group(2)}" if match else "-"
