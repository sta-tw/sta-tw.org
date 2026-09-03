"""Rule-based extraction for admission brochures.

The output intentionally stays in the brochure candidate staging area. It is
not a published program record: administrators still confirm every field and
can correct OCR/PDF text extraction before the Go service materializes it.
"""

from __future__ import annotations

from datetime import date
import re
from typing import Any

from .contracts import BrochureExtractJob, ExtractionCandidate


PROGRAM_CODE_PATTERN = re.compile(
    r"(?:科系編號|系所代碼|系所編號|科系代碼|招生系組代碼|組別|序號)\s*[:：#]?\s*([0-9]{3})"
)
LOOSE_PROGRAM_LINE_PATTERN = re.compile(r"^\s*([0-9]{3})\s*[.)、：:\-]\s*(.{2,120})$")
QUOTA_PATTERN = re.compile(r"(?:招生名額|錄取名額|招生人數|名額)\s*[:：]?\s*([0-9]{1,5})\s*(?:名|人|位)?")
DATE_PATTERN = re.compile(r"(?:(?P<year>[0-9]{3,4})\s*(?:年|[-/.])\s*)?(?P<month>[0-9]{1,2})\s*(?:月|[-/.])\s*(?P<day>[0-9]{1,2})\s*(?:日)?")
WEIGHT_PATTERN = re.compile(r"([0-9]{1,3}(?:\.[0-9]+)?)\s*%")

REGISTRATION_KEYWORDS = ("報名", "申請", "登記", "收件", "繳費")
EXAM_KEYWORDS = ("面試", "口試", "筆試", "甄試", "考試", "資料審查", "書面審查")
RESULT_KEYWORDS = ("放榜", "榜單", "錄取公告", "成績公告")
ANNOUNCEMENT_KEYWORDS = ("簡章公告", "公告簡章", "簡章發售", "簡章下載")


def extract_local_candidates(job: BrochureExtractJob, pages: list[str]) -> list[ExtractionCandidate]:
    lines_by_page = [text.splitlines() for text in pages]
    global_fields = _collect_schedule_fields(job, lines_by_page)
    records: dict[str, dict[str, Any]] = {}
    for page_number, lines in enumerate(lines_by_page, start=1):
        for line_index, raw_line in enumerate(lines):
            line = " ".join(raw_line.split())
            if not line:
                continue
            matches = list(PROGRAM_CODE_PATTERN.finditer(line))
            if not matches:
                loose = LOOSE_PROGRAM_LINE_PATTERN.match(line)
                if loose:
                    matches = [loose]
            for match in matches:
                code = match.group(1)
                name = _program_name(line, match)
                nearby = lines[line_index : min(len(lines), line_index + 4)]
                quota = _find_quota(nearby)
                record = records.setdefault(code, {
                    "program_code": code,
                    "admission_program_name": name or "-",
                    "admission_quota": quota,
                    "source_page": min(page_number, 999),
                    "evidence": [],
                })
                if name and record.get("admission_program_name") == "-":
                    record["admission_program_name"] = name
                if quota is not None and record.get("admission_quota") is None:
                    record["admission_quota"] = quota
                evidence = line[:1000]
                if evidence not in [item["text"] for item in record["evidence"]]:
                    record["evidence"].append({"page": min(page_number, 999), "text": evidence})

    candidates: list[ExtractionCandidate] = []
    for code, record in sorted(records.items()):
        data = {
            "document_type": "brochure",
            "academic_year": str(job.academic_year),
            "school_code": job.school_code,
            "program_code": code,
            "admission_program_name": record["admission_program_name"],
            "admission_quota": record["admission_quota"],
            **global_fields,
            "evidence": record["evidence"][:20],
        }
        data["raw_text_excerpt"] = _excerpt(pages[record["source_page"] - 1] if pages else "")
        confidence = 0.8 if record["admission_program_name"] != "-" and record["admission_quota"] is not None else 0.45
        candidates.append(ExtractionCandidate(
            program_code=code,
            data=data,
            source_page=record["source_page"],
            confidence=confidence,
        ))
    return candidates[:2000]


def _program_name(line: str, match: re.Match[str]) -> str:
    if match.re is LOOSE_PROGRAM_LINE_PATTERN:
        value = match.group(2)
    else:
        value = line[match.end() :]
    value = re.split(r"(?:招生名額|錄取名額|招生人數|名額)\s*[:：]?\s*[0-9]", value, maxsplit=1)[0]
    value = re.split(r"\s{2,}|\u3000", value, maxsplit=1)[0]
    value = re.sub(r"^[\s:：#、.\-]+|[\s:：#、.\-]+$", "", value)
    if value in {"", "招生", "學系", "組"} or len(value) > 120:
        return ""
    return value


def _find_quota(lines: list[str]) -> int | None:
    for line in lines:
        match = QUOTA_PATTERN.search(line)
        if match:
            return int(match.group(1))
    return None


def _collect_schedule_fields(job: BrochureExtractJob, lines_by_page: list[list[str]]) -> dict[str, Any]:
    fields: dict[str, Any] = {}
    exam_items: list[dict[str, Any]] = []
    for page_number, lines in enumerate(lines_by_page, start=1):
        for raw_line in lines:
            line = " ".join(raw_line.split())
            if not line:
                continue
            dates = _dates(line, job.academic_year)
            if dates:
                if _contains_any(line, REGISTRATION_KEYWORDS):
                    _set_range(fields, "registration", dates)
                if _contains_any(line, EXAM_KEYWORDS):
                    _set_range(fields, "exam", dates)
                if _contains_any(line, RESULT_KEYWORDS):
                    fields.setdefault("result_date", dates[0])
                if _contains_any(line, ANNOUNCEMENT_KEYWORDS):
                    fields.setdefault("brochure_announcement_date", dates[0])
            if _contains_any(line, EXAM_KEYWORDS):
                item_name = _exam_name(line)
                if item_name and not any(item["name"] == item_name for item in exam_items):
                    weight_match = WEIGHT_PATTERN.search(line)
                    weight = float(weight_match.group(1)) if weight_match else None
                    exam_items.append({
                        "name": item_name,
                        "sort_order": len(exam_items) + 1,
                        "weight_percent": weight,
                        "description": line[:1000],
                        "source_page": str(min(page_number, 999)),
                    })
    if exam_items:
        fields["exam_items"] = exam_items[:100]
    return fields


def _set_range(fields: dict[str, Any], prefix: str, dates: list[str]) -> None:
    start_key = f"{prefix}_start_date"
    end_key = f"{prefix}_end_date"
    if start_key not in fields:
        fields[start_key] = dates[0]
        fields[end_key] = dates[-1]


def _dates(value: str, academic_year: int) -> list[str]:
    result: list[str] = []
    explicit_year: int | None = None
    for match in DATE_PATTERN.finditer(value):
        if match.group("year") is not None:
            explicit_year = int(match.group("year"))
        if explicit_year is None:
            # Do not guess a year for a month/day-only value.
            continue
        formatted = _format_date(explicit_year, int(match.group("month")), int(match.group("day")))
        if formatted:
            result.append(formatted)
    return _unique(result)


def _format_date(year: int, month: int, day: int) -> str | None:
    if year < 1000:
        year += 1911
    try:
        return date(year, month, day).isoformat()
    except ValueError:
        return None


def _exam_name(line: str) -> str:
    value = re.split(r"[:：]", line, maxsplit=1)[0]
    for keyword in EXAM_KEYWORDS:
        if keyword in value:
            return keyword
    return ""


def _contains_any(value: str, keywords: tuple[str, ...]) -> bool:
    return any(keyword in value for keyword in keywords)


def _unique(values: list[str]) -> list[str]:
    return list(dict.fromkeys(values))


def _excerpt(value: str, limit: int = 3000) -> str:
    return " ".join(value.split())[:limit]
