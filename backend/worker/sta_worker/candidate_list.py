"""Deterministic extraction for candidate-number/name lists.

This module deliberately has no model or network dependency. It accepts the
common PDF, CSV/TSV, text, and JSON formats used for official lists and emits
the internal result contract. The Go API performs the final validation and
stores only a lookup hash, last four digits, and a masked name.
"""

from __future__ import annotations

import csv
from io import StringIO
import json
from pathlib import Path
import re
from typing import Any

from .contracts import BrochureExtractJob, InvalidJob, RetryableProcessingError


CANDIDATE_NUMBER_RE = re.compile(
    r"(?<![A-Za-z0-9])([A-Za-z0-9]{2,8}[-_][A-Za-z0-9]{2,32}|[A-Za-z0-9]{4,64})(?![A-Za-z0-9])"
)
NAME_RE = re.compile(r"[\u4e00-\u9fff]{2,6}(?:[·・][\u4e00-\u9fff]{1,6})?")
PROGRAM_CODE_RE = re.compile(r"(?:科系編號|系所代碼|組別|program[_ ]?code)\s*[:：#]?\s*([0-9]{3})", re.IGNORECASE)
RANK_RE = re.compile(r"(?:正取|備取)\s*(?:第\s*)?([0-9]{1,5})")

FIELD_ALIASES = {
    "candidate_number": ("candidate_number", "candidate_no", "准考證", "准考證號", "准考證號碼", "准考証號", "考號"),
    "name": ("candidate_name", "name", "姓名", "考生姓名", "考生"),
    "program_code": ("program_code", "program", "科系編號", "系所代碼", "組別"),
    "result_status": ("result_status", "status", "結果", "錄取結果", "錄取狀態"),
    "official_rank": ("official_rank", "rank", "名次", "序位", "錄取序"),
    "quota": ("quota", "名額", "招生名額"),
    "source_page": ("source_page", "page", "頁碼"),
}


def extract_candidate_list(job: BrochureExtractJob, path: Path, processor_version: str, max_bytes: int) -> dict[str, Any]:
    if not path.is_file():
        raise RetryableProcessingError("candidate list is not available")
    if path.stat().st_size > max_bytes:
        raise RetryableProcessingError("candidate list exceeds worker size limit")

    suffix = path.suffix.lower()
    if suffix == ".pdf":
        pages = _read_pdf_pages(path)
        rows = _extract_pdf_rows(pages, job)
    elif suffix == ".json":
        rows = _extract_json_rows(_read_text(path), job)
    else:
        rows = _extract_delimited_rows(_read_text(path), job)
    if not rows:
        raise InvalidJob("candidate list contains no usable candidate rows")
    return {
        "result_type": "candidate_list",
        "job_id": job.job_id,
        "academic_year": job.academic_year,
        "school_code": job.school_code,
        "sha256_hex": job.sha256_hex,
        "source_url": job.source_url,
        "processor": processor_version,
        "rows": rows,
        "generated_at": _now_iso(),
    }


def _read_pdf_pages(path: Path) -> list[str]:
    try:
        from pypdf import PdfReader
    except ImportError as exc:
        raise RetryableProcessingError("pypdf is not installed") from exc
    try:
        reader = PdfReader(str(path), strict=False)
        if len(reader.pages) > 2000:
            raise InvalidJob("candidate list contains too many pages")
        return [page.extract_text() or "" for page in reader.pages]
    except InvalidJob:
        raise
    except Exception as exc:
        raise RetryableProcessingError("candidate list PDF parsing failed") from exc


def _read_text(path: Path) -> str:
    data = path.read_bytes()
    for encoding in ("utf-8-sig", "utf-8", "cp950"):
        try:
            return data.decode(encoding)
        except UnicodeDecodeError:
            continue
    raise InvalidJob("candidate list text encoding is unsupported")


def _extract_pdf_rows(pages: list[str], job: BrochureExtractJob) -> list[dict[str, Any]]:
    rows: list[dict[str, Any]] = []
    current_program = job.program_code or ""
    for page_number, text in enumerate(pages, start=1):
        for code in PROGRAM_CODE_RE.findall(text):
            current_program = code
        for line in text.splitlines():
            candidate_match = CANDIDATE_NUMBER_RE.search(line)
            if not candidate_match:
                continue
            candidate_number = candidate_match.group(1)
            name = _name_near_candidate(line, candidate_match.span())
            if not name:
                continue
            status = _status_from_text(line)
            rank = _rank_from_text(line)
            rows.append(_row(job, candidate_number, name, current_program, status, rank, page_number))
    return _deduplicate(rows)


def _extract_delimited_rows(text: str, job: BrochureExtractJob) -> list[dict[str, Any]]:
    sample = text[:8192]
    try:
        dialect = csv.Sniffer().sniff(sample, delimiters=",\t;|")
    except csv.Error:
        dialect = csv.excel_tab if "\t" in sample else csv.excel
    reader = csv.reader(StringIO(text), dialect)
    values = [list(row) for row in reader if any(cell.strip() for cell in row)]
    if not values:
        return []
    headers = [_normal_header(value) for value in values[0]]
    indexes = {field: _find_header_index(headers, aliases) for field, aliases in FIELD_ALIASES.items()}
    has_header = indexes["candidate_number"] >= 0 or indexes["name"] >= 0
    data_rows = values[1:] if has_header else values
    rows: list[dict[str, Any]] = []
    current_program = job.program_code or ""
    for values_row in data_rows:
        candidate = _cell(values_row, indexes["candidate_number"] if has_header else 0)
        name = _cell(values_row, indexes["name"] if has_header else 1)
        if not candidate or not name:
            continue
        program = _cell(values_row, indexes["program_code"]) if has_header else ""
        if not program:
            program = _program_from_text(" ".join(values_row)) or current_program
        else:
            program = _normal_program(program)
        status = _status_from_text(_cell(values_row, indexes["result_status"]))
        rank = _int_or_none(_cell(values_row, indexes["official_rank"]))
        page = _int_or_default(_cell(values_row, indexes["source_page"]), 1)
        rows.append(_row(job, candidate, name, program, status, rank, page))
    return _deduplicate(rows)


def _extract_json_rows(text: str, job: BrochureExtractJob) -> list[dict[str, Any]]:
    try:
        payload = json.loads(text)
    except json.JSONDecodeError as exc:
        raise InvalidJob("candidate list JSON is invalid") from exc
    if isinstance(payload, dict):
        payload = payload.get("rows", payload.get("data", []))
    if not isinstance(payload, list):
        raise InvalidJob("candidate list JSON rows are invalid")
    rows: list[dict[str, Any]] = []
    for item in payload:
        if not isinstance(item, dict):
            continue
        candidate = _mapping_value(item, FIELD_ALIASES["candidate_number"])
        name = _mapping_value(item, FIELD_ALIASES["name"])
        if not candidate or not name:
            continue
        program = _mapping_value(item, FIELD_ALIASES["program_code"]) or job.program_code
        status = _status_from_text(_mapping_value(item, FIELD_ALIASES["result_status"]))
        rank = _int_or_none(_mapping_value(item, FIELD_ALIASES["official_rank"]))
        page = _int_or_default(_mapping_value(item, FIELD_ALIASES["source_page"]), 1)
        rows.append(_row(job, candidate, name, _normal_program(program), status, rank, page))
    return _deduplicate(rows)


def _row(job: BrochureExtractJob, candidate: str, name: str, program: str, status: str, rank: int | None, page: int) -> dict[str, Any]:
    candidate = _normal_candidate(candidate)
    if not candidate:
        return {}
    output: dict[str, Any] = {
        "program_code": _normal_program(program),
        "candidate_number": candidate,
        "masked_name": _mask_name(name),
        "result_status": status or "unknown",
        "source_page": max(1, min(page, 999)),
    }
    if rank is not None and rank > 0:
        output["official_rank"] = rank
    return output


def _deduplicate(rows: list[dict[str, Any]]) -> list[dict[str, Any]]:
    result: list[dict[str, Any]] = []
    seen: set[tuple[str, str]] = set()
    for row in rows:
        if not row or not row.get("candidate_number"):
            continue
        key = (str(row.get("program_code", "")), str(row["candidate_number"]))
        if key in seen:
            continue
        seen.add(key)
        result.append(row)
    return result[:10000]


def _name_near_candidate(line: str, span: tuple[int, int]) -> str:
    before = line[: span[0]]
    after = line[span[1] :]
    candidates = NAME_RE.findall(after) + NAME_RE.findall(before)
    ignored = {"准考證", "准考證號", "考生姓名", "錄取結果", "正取", "備取", "不錄取"}
    for name in candidates:
        if name not in ignored:
            return name
    return ""


def _status_from_text(value: str) -> str:
    value = str(value or "")
    if "備取" in value or "候補" in value:
        return "waitlisted"
    if "不錄取" in value or "未錄取" in value or "落選" in value:
        return "rejected"
    if "正取" in value or "錄取" in value:
        return "admitted"
    return "unknown"


def _rank_from_text(value: str) -> int | None:
    match = RANK_RE.search(value)
    return int(match.group(1)) if match else None


def _program_from_text(value: str) -> str:
    match = PROGRAM_CODE_RE.search(value)
    return match.group(1) if match else ""


def _normal_program(value: Any) -> str:
    value = str(value or "").strip()
    match = re.search(r"(?<!\d)(\d{3})(?!\d)", value)
    return match.group(1) if match else ""


def _normal_candidate(value: Any) -> str:
    match = CANDIDATE_NUMBER_RE.search(str(value or "").strip())
    return match.group(1) if match else ""


def _mask_name(value: Any) -> str:
    name = str(value or "").strip()
    if not name:
        return "-"
    runes = list(name)
    if len(runes) <= 1:
        return name
    return runes[0] + "○" * (len(runes) - 1)


def _normal_header(value: str) -> str:
    return re.sub(r"[\s_\-（）()：:]+", "", value.strip().lower())


def _find_header_index(headers: list[str], aliases: tuple[str, ...]) -> int:
    normalised = {_normal_header(alias) for alias in aliases}
    for index, header in enumerate(headers):
        if header in normalised:
            return index
    return -1


def _mapping_value(mapping: dict[str, Any], aliases: tuple[str, ...]) -> str:
    normalised = {_normal_header(alias) for alias in aliases}
    for key, value in mapping.items():
        if _normal_header(str(key)) in normalised:
            return str(value or "").strip()
    return ""


def _cell(values: list[str], index: int) -> str:
    return values[index].strip() if 0 <= index < len(values) else ""


def _int_or_none(value: Any) -> int | None:
    try:
        return int(str(value).strip())
    except (TypeError, ValueError):
        return None


def _int_or_default(value: Any, default: int) -> int:
    return _int_or_none(value) or default


def _now_iso() -> str:
    from datetime import datetime, timezone

    return datetime.now(timezone.utc).isoformat()
