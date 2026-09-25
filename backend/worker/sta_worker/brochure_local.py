"""Rule-based extraction for admission brochures.

The output intentionally stays in the brochure candidate staging area. It is
not a published program record: administrators still confirm every field and
can correct OCR/PDF text extraction before the Go service materializes it.
"""

from __future__ import annotations

from dataclasses import dataclass
from datetime import date
import re
from typing import Any
import unicodedata

from .contracts import BrochureExtractJob, ExtractionCandidate


PROGRAM_CODE_PATTERN = re.compile(
    r"(?:科系編號|系所代碼|系所編號|科系代碼|招生系組代碼|組別|序號)\s*[:：#]?\s*([0-9]{3})"
)
ACADEMIC_YEAR_PATTERN = re.compile(r"(?:中華民國|民國)?\s*(1[0-9]{2})\s*學年度")
WESTERN_ACADEMIC_YEAR_PATTERN = re.compile(r"(20[0-9]{2})\s*學年度")
SCHOOL_CODE_PATTERN = re.compile(r"(?:學校代碼|學校編號|校碼|校號)\s*[:：#]?\s*([0-9]{3})")
SCHOOL_NAME_PATTERN = re.compile(r"(?:學校名稱|校名)\s*[:：]\s*(.{2,100})")
LOOSE_PROGRAM_LINE_PATTERN = re.compile(r"^\s*([0-9]{3})\s*[.)、：:\-]\s*(.{2,120})$")
QUOTA_PATTERN = re.compile(r"(?:招生名額|錄取名額|招生人數|名額)\s*[:：]?\s*([0-9]{1,5})\s*(?:名|人|位)?")
QUOTA_VALUE_PATTERN = re.compile(r"(?<![0-9])([0-9]{1,5})\s*(?:名|人|位)")
DATE_PATTERN = re.compile(r"(?:(?P<year>[0-9]{3,4})\s*(?:年|[-/.])\s*)?(?P<month>[0-9]{1,2})\s*(?:月|[-/.])\s*(?P<day>[0-9]{1,2})\s*(?:日)?")
WEIGHT_PATTERN = re.compile(r"([0-9]{1,3}(?:\.[0-9]+)?)\s*%")
PERCENT_ITEM_PATTERN = re.compile(
    r"(?P<name>.+?)[（(]\s*(?P<weight>[0-9]{1,3}(?:\.[0-9]+)?)\s*%\s*[）)]"
)
INSTITUTION_NAME_PATTERN = re.compile(
    r"(?<![\u4e00-\u9fff])"
    r"(?P<name>(?:國立|私立|[\u4e00-\u9fff]{2,8}市立)?"
    r"[\u4e00-\u9fffA-Za-z0-9]{2,40}"
    r"(?:科技大學|技術學院|專科學校|大學|學院))"
)
GROUP_TOKEN_PATTERN = re.compile(r"[\u4e00-\u9fffA-Za-z0-9&＋+/．.]+?組")
HEADING_ORDINAL_PATTERN = re.compile(
    r"^\s*(?:[（(]?\s*(?:[0-9]{1,3}|[一二三四五六七八九十百]+)\s*[）)]?"
    r"\s*[.)、．.：:]?)\s*"
)
REGISTRATION_KEYWORDS = ("報名", "申請", "登記", "收件", "繳費")
EXAM_KEYWORDS = ("面試", "口試", "筆試", "甄試", "考試", "複審", "資料審查", "書面審查", "術科")
EXAM_ITEM_KEYWORDS = ("術科", "面試", "口試", "筆試", "資料審查", "書面審查", "甄試")
RESULT_KEYWORDS = ("放榜", "榜單", "錄取公告", "成績公告")
ANNOUNCEMENT_KEYWORDS = ("簡章公告", "公告簡章", "簡章發售", "簡章下載")
# Dates that appear next to these words describe how the brochure itself was
# approved or printed, not the applicant timetable.
SCHEDULE_EXCLUDE_KEYWORDS = (
    "委員會會議",
    "招生委員會",
    "會議通過",
    "會議決議",
    "決議通過",
    "會議紀錄",
    "編製",
    "編訂",
    "編印",
    "印製",
    "核定",
    "修正通過",
    "審議通過",
)
PHONE_PATTERN = re.compile(
    r"(?:聯絡電話|(?<![\u4e00-\u9fffA-Za-z])電\s*話|(?<![\u4e00-\u9fffA-Za-z])電話)"
    r"\s*[:：]?\s*(?P<value>.*?)"
    r"(?=\s+(?:E[-－–]?mail|電子郵件|電子信箱|信箱|網址|學系網址)\s*[:：]|$)"
)
EMAIL_PATTERN = re.compile(
    r"[A-Za-z0-9._%+\-]+@[A-Za-z0-9](?:[A-Za-z0-9.\-]*[A-Za-z0-9])?\.[A-Za-z]{2,}"
)
URL_PATTERN = re.compile(
    r"(?:學系網址|系所網址|原始簡章網址|系網|學校網址|網址)\s*[:：]?\s*"
    r"(https?://[^\s，,。；;）)]+)"
)
EXAM_ITEM_ROW_PATTERN = re.compile(
    r"^\s*(?P<order>[0-9]{1,3})\s+(?P<name>.+?)\s+(?P<weight>[0-9]{1,3}(?:\.[0-9]+)?)\s*%\s*$"
)
STAGE_LABEL_PATTERN = re.compile(
    r"^(?P<label>初審|複審|初試|複試|初選|複選|"
    r"第[0-9一二三四五六七八九十百]+階段|"
    r"[0-9一二三四五六七八九十百]+階段|"
    r"第[0-9一二三四五六七八九十百]+關|"
    r"[0-9一二三四五六七八九十百]+關|"
    r"[一二三四五六七八九十0-9]+階)"
    r"(?=\s|$|[:：]|[（(])"
)
EXAM_HEADING_PREFIXES = (
    "書面",
    "資料審查",
    "面試",
    "口試",
    "筆試",
    "術科",
    "實作",
    "甄試",
    "學科",
    "專業能力",
    "作品審查",
    "能力測驗",
    "評量",
)
EXAM_SECTION_END_PREFIXES = (
    "總成績",
    "同分參酌",
    "學系網址",
    "系所網址",
    "聯絡資訊",
    "連絡資訊",
    "聯絡方式",
    "連絡方式",
    "其他相關規定",
    "備註",
)
PROGRAM_GROUP_LABELS = (
    "願景計畫組",
    "願景計劃組",
    "願景計畫",
    "願景計劃",
    "一般生組",
    "一般生",
    "一般組",
    "一般",
)

# --- format-agnostic program-section detection ------------------------------
# Universities label the per-department part of a brochure very differently
# ("各學系招生規定", "招生學系、名額…", "系(組)別", "學系招生資訊", a bare
# "捌、招生名額…" heading, …), so matching one school's exact heading does not
# generalise. Instead, treat any heading that combines an enrolment word with a
# scope/quota word as a section start, and confirm every department block
# structurally by requiring an admissions quota a few lines below the heading.
SECTION_HEADER_KEYWORDS = (
    "學系分則",
    "系所分則",
    "各學系招生規定",
    "各系招生規定",
    "學系招生資訊",
    "系所招生資訊",
    "招生學系資訊",
    "招生學系（組）別",
    "招生學系(組)別",
    "招生系所（組）別",
    "招生系所(組)別",
    "招生學系、名額",
    "招生系所、名額",
)
SECTION_HEADER_ORDINAL_PATTERN = re.compile(
    r"^[（(]?[0-9一二三四五六七八九十壹貳參肆伍陸柒捌玖拾]{1,4}[）)]?[、.．)]"
)
PROGRAM_NAME_SUFFIXES = (
    "學士學位學程",
    "學士學位國際學程",
    "學位學程",
    "學士班",
    "學系",
    "研究所",
    "系所",
    "學程",
    "系",
)
PROGRAM_LABEL_TOKENS = (
    "招生學系（組）",
    "招生學系(組)",
    "招生學系",
    "招生系所",
    "報考學系",
    "系所名稱",
    "學系名稱",
    "系所別",
    "系組別",
    "系（組）別",
    "系(組)別",
    "學系",
    "系所",
)
PROGRAM_ROW_TAIL_TOKENS = (
    "代碼",
    "招生名額",
    "招生人數",
    "錄取名額",
    "核定名額",
    "報名費",
    "招生對象",
)
PROGRAM_CODE_PREFIX_PATTERN = re.compile(
    r"^(?:科系編號|系所代碼|科系代碼|系所編號|招生系組代碼|序號|代碼|編號)"
    r"\s*[:：#]?\s*[A-Za-z0-9]{1,12}\s*"
)
PROGRAM_CODE_INLINE_PATTERN = re.compile(r"代\s*碼\s*[:：#]?\s*([A-Za-z0-9]{2,12})")
QUOTA_ONLY_LINE_PATTERN = re.compile(r"^\d{1,3}\s*名(?:\s+\d{1,3}\s*名)*$")
TOC_LEADER_PATTERN = re.compile(r"[.…·]{4,}|\s\d{1,3}\s+\d{1,3}\s*$")
PROGRAM_HEADING_REJECT_PREFIXES = (
    "報考",
    "招生",
    "考試",
    "甄試",
    "甄選",
    "總成績",
    "指定",
    "本校",
    "各學系",
    "各系",
    "前一",
    "上一",
)
PROGRAM_NAME_CLEAN_PATTERN = re.compile(r"[一-鿿A-Za-z·]+")
PROGRAM_QUOTA_LOOKAHEAD = 16
MIN_PROGRAM_NAME_LENGTH = 3
MAX_PROGRAM_NAME_LENGTH = 40
MAX_PROGRAM_BLOCK_LINES = 600


def _clean_line(value: str) -> str:
    value = unicodedata.normalize("NFKC", value or "")
    value = " ".join(value.split()).strip()
    # PDF text extraction often inserts spaces inside short labels such as
    # 「電 話」 and 「連 絡 資 訊」. Keep meaningful column spacing intact while
    # restoring the labels used by the field rules.
    for compact, spaced in (
        ("招生學系", r"招\s*生\s*學\s*系"),
        ("招生名額", r"招\s*生\s*名\s*額"),
        ("考試項目", r"考\s*試\s*項\s*目"),
        ("同分參酌", r"同\s*分\s*參\s*酌"),
        ("成績比例", r"成\s*績\s*比\s*例"),
        ("洽詢電話", r"洽\s*詢\s*電\s*話"),
        ("電話", r"電\s*話"),
        ("連絡", r"連\s*絡"),
        ("聯絡", r"聯\s*絡"),
        ("電子郵件", r"電\s*子\s*郵\s*件"),
    ):
        value = re.sub(spaced, compact, value)
    return value


@dataclass(frozen=True)
class BrochureIdentity:
    academic_year: int = 0
    school_code: str = ""
    school_name: str = ""


@dataclass(frozen=True)
class ProgramHeading:
    name: str
    group: str
    consumed_lines: int = 1


def infer_brochure_identity(job: BrochureExtractJob, pages: list[str]) -> BrochureIdentity:
    """Infer the brochure identity from its first pages for upload-only intake.

    Existing externally sourced jobs already carry the canonical identity. For
    upload-only jobs this is only a candidate value; the Go admin flow lets a
    reviewer correct it before publication.
    """

    sample = "\n".join(pages[:10])
    academic_year = job.academic_year or _academic_year(sample)
    school_code = job.school_code or _school_code(sample)
    school_name = _school_name(sample)
    return BrochureIdentity(academic_year, school_code, school_name)


def extract_local_candidates(
    job: BrochureExtractJob,
    pages: list[str],
    *,
    academic_year: int | None = None,
    school_code: str | None = None,
    school_name: str = "",
) -> list[ExtractionCandidate]:
    lines_by_page = [text.splitlines() for text in pages]
    effective_year = job.academic_year if academic_year is None else academic_year
    effective_school_code = job.school_code if school_code is None else school_code
    global_fields = _collect_schedule_fields(
        job,
        lines_by_page,
        academic_year=effective_year,
    )
    records: list[dict[str, Any]] = []
    records_by_identity: dict[tuple[str, str], dict[str, Any]] = {}
    document_lines, program_blocks = _build_program_blocks(lines_by_page)
    if program_blocks:
        for heading, block_lines, block_code in program_blocks:
            expanded_blocks = _expand_program_block(heading, block_lines)
            for expanded_heading, expanded_lines, group_index, group_count, quota in expanded_blocks:
                record = _get_or_create_record(
                    records,
                    records_by_identity,
                    expanded_heading.name,
                    expanded_heading.group,
                    expanded_lines[0][0],
                    expanded_lines[0][1],
                    printed_program_code=block_code if group_count == 1 else "",
                )
                _update_record_quota(
                    record,
                    [line for _, line in expanded_lines],
                    quota=quota,
                    allow_fallback=group_count == 1,
                )
                _update_record_fields(
                    record,
                    expanded_lines,
                    group_index=group_index,
                    group_count=group_count,
                )
    else:
        _extract_fallback_records(document_lines, records, records_by_identity)

    candidates: list[ExtractionCandidate] = []
    for record in records:
        code = record["program_code"]
        data = {
            "document_type": "brochure",
            "academic_year": str(effective_year) if effective_year else "-",
            "school_code": effective_school_code or "-",
            "school_name": school_name or "-",
            "program_code": code,
            "source_program_code": record.get("source_program_code") or "-",
            "admission_program_name": record["admission_program_name"],
            "admission_quota": record["admission_quota"],
            "consultation_phone": record["consultation_phone"] or "-",
            "consultation_email": record["consultation_email"] or "-",
            "consultation_contact": record["consultation_contact"] or "-",
            "brochure_url": record["brochure_url"] or "-",
            "exam_items": record["exam_items"],
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


def _build_program_blocks(
    lines_by_page: list[list[str]],
) -> tuple[
    list[tuple[int, str]],
    list[tuple[ProgramHeading, list[tuple[int, str]], str]],
]:
    """Split the department part of a brochure into one block per program.

    The detection is deliberately format-agnostic: it does not rely on a single
    school's section wording. A heading is accepted when the department name has
    a recognised suffix and an admissions quota appears within a short window
    below it, which every real department page provides regardless of layout.
    """

    document_lines: list[tuple[int, str]] = []
    for page_number, lines in enumerate(lines_by_page, start=1):
        for raw_line in lines:
            line = _clean_line(raw_line)
            if line:
                document_lines.append((page_number, line))

    quota_positions = {
        index
        for index, (_, line) in enumerate(document_lines)
        if _is_quota_line(line)
    }
    section_positions = [
        index
        for index, (_, line) in enumerate(document_lines)
        if _is_section_header(line)
    ]

    def collect(scan_from: int, *, in_section: bool) -> list[tuple[int, ProgramHeading, str]]:
        found: list[tuple[int, ProgramHeading, str]] = []
        for index in range(scan_from, len(document_lines)):
            heading, code = _program_start_at(
                document_lines,
                index,
                quota_positions,
                in_section=in_section,
            )
            if heading is not None:
                found.append((index, heading, code))
        return found

    starts: list[tuple[int, ProgramHeading, str]] = []
    for section_index in section_positions:
        candidate = collect(section_index + 1, in_section=True)
        if len(candidate) > len(starts):
            starts = candidate
    if not starts:
        starts = collect(0, in_section=False)

    blocks: list[tuple[ProgramHeading, list[tuple[int, str]], str]] = []
    for order, (start, heading, code) in enumerate(starts):
        if order + 1 < len(starts):
            end = starts[order + 1][0]
        else:
            end = min(len(document_lines), start + MAX_PROGRAM_BLOCK_LINES)
        if start < end:
            blocks.append((heading, document_lines[start:end], code))
    return document_lines, blocks


def _is_quota_line(line: str) -> bool:
    line = _clean_line(line)
    if not line:
        return False
    if QUOTA_PATTERN.search(line):
        return True
    if line in ("招生名額", "招生人數", "錄取名額", "核定名額"):
        return True
    return bool(QUOTA_ONLY_LINE_PATTERN.match(line))


def _spaced_token_pattern(token: str) -> str:
    return r"\s*".join(re.escape(char) for char in token)


def _is_section_header(line: str) -> bool:
    line = _clean_line(line)
    if not line or TOC_LEADER_PATTERN.search(line):
        return False
    compact = line.replace(" ", "")
    if not compact or len(compact) > 48:
        return False
    if any(keyword in compact for keyword in SECTION_HEADER_KEYWORDS):
        return True
    if not SECTION_HEADER_ORDINAL_PATTERN.match(compact):
        return False
    has_enrolment = any(word in compact for word in ("招生", "甄選", "甄試"))
    has_scope = any(
        word in compact
        for word in ("學系", "系所", "系（組）", "系(組)", "學系（組）", "學系(組)", "名額")
    )
    has_detail = any(
        word in compact
        for word in ("名額", "考試", "考科", "科目", "甄試", "計分", "配分", "規定")
    )
    return has_enrolment and has_scope and has_detail


def _program_start_at(
    document_lines: list[tuple[int, str]],
    index: int,
    quota_positions: set[int],
    *,
    in_section: bool,
) -> tuple[ProgramHeading | None, str]:
    """Return ``(heading, program_code)`` when a department block starts here."""

    _, line = document_lines[index]
    if not line or len(line) > 90 or TOC_LEADER_PATTERN.search(line):
        return None, ""

    core = line
    labeled = False
    for token in sorted(PROGRAM_LABEL_TOKENS, key=len, reverse=True):
        match = re.match(
            rf"^{_spaced_token_pattern(token)}\s*[:：]?\s+(.+)$",
            core,
        )
        if match:
            core = match.group(1).strip()
            labeled = True
            break

    code_match = PROGRAM_CODE_INLINE_PATTERN.search(core)
    program_code = code_match.group(1).upper() if code_match else ""

    core = _strip_heading_ordinal(core)
    core = PROGRAM_CODE_PREFIX_PATTERN.sub("", core, count=1)
    tail_pattern = "|".join(
        _spaced_token_pattern(token) for token in PROGRAM_ROW_TAIL_TOKENS
    )
    core = re.split(rf"\s*(?:{tail_pattern})", core, maxsplit=1)[0]
    core = core.strip(" ：:、.．-")

    name, group = _split_program_label(core)
    if (
        not name
        or len(name) < MIN_PROGRAM_NAME_LENGTH
        or len(name) > MAX_PROGRAM_NAME_LENGTH
        or not name.endswith(PROGRAM_NAME_SUFFIXES)
        or name.startswith(PROGRAM_HEADING_REJECT_PREFIXES)
        or not PROGRAM_NAME_CLEAN_PATTERN.fullmatch(name)
        or not _looks_like_program_name(name)
    ):
        return None, ""

    if not group:
        # Some layouts drop the group name onto its own line below the heading
        # (「招生學系 國語文學系」 / 「（願景計畫組）」 / 「招生名額 7」).
        for offset in range(1, 4):
            look = index + offset
            if look >= len(document_lines):
                break
            follower = document_lines[look][1]
            if not follower or _is_quota_line(follower):
                break
            candidate_group = _normalize_group_label(follower)
            if candidate_group:
                group = candidate_group
                break
            if _program_detail_starts(follower):
                break

    has_quota = any(
        position in quota_positions
        for position in range(
            index, min(index + PROGRAM_QUOTA_LOOKAHEAD, len(document_lines))
        )
    )
    if not has_quota and not (labeled and in_section):
        return None, ""

    return ProgramHeading(name=name, group=group or "一般組", consumed_lines=1), program_code


def _expand_program_block(
    heading: ProgramHeading,
    block_lines: list[tuple[int, str]],
) -> list[tuple[ProgramHeading, list[tuple[int, str]], int, int, int | None]]:
    """Expand a two-column department table into one candidate per group.

    Several university brochures put the group names and quotas in columns
    rather than giving each group its own heading. The PDF text layer still
    exposes those values in document order, so keeping the group index lets
    the exam-item parser select the matching column while sharing contact and
    URL fields.
    """

    groups = _group_labels(block_lines)
    if len(groups) <= 1:
        group = groups[0] if groups else heading.group
        return [(
            ProgramHeading(heading.name, group or "一般組", heading.consumed_lines),
            block_lines,
            0,
            1,
            None,
        )]

    quotas = _find_group_quotas(block_lines, len(groups))
    return [
        (
            ProgramHeading(heading.name, group, heading.consumed_lines),
            block_lines,
            index,
            len(groups),
            quotas[index] if index < len(quotas) else None,
        )
        for index, group in enumerate(groups)
    ]


def _extract_fallback_records(
    document_lines: list[tuple[int, str]],
    records: list[dict[str, Any]],
    records_by_identity: dict[tuple[str, str], dict[str, Any]],
) -> None:
    starts: list[tuple[int, str, str]] = []
    for position, (page_number, line) in enumerate(document_lines):
        matches = list(PROGRAM_CODE_PATTERN.finditer(line))
        if not matches:
            loose = LOOSE_PROGRAM_LINE_PATTERN.match(line)
            if loose:
                matches = [loose]
        for match in matches:
            name = _program_name(line, match)
            name, group = _split_program_label(name)
            if _looks_like_program_name(name):
                starts.append((position, name, group or "一般組"))

    for index, (start, name, group) in enumerate(starts):
        next_start = starts[index + 1][0] if index + 1 < len(starts) else len(document_lines)
        block_lines = document_lines[start:next_start]
        record = _get_or_create_record(
            records,
            records_by_identity,
            name,
            group,
            block_lines[0][0],
            block_lines[0][1],
        )
        _update_record_quota(record, [line for _, line in block_lines[:6]])
        _update_record_fields(record, block_lines)


def _strip_heading_ordinal(value: str) -> str:
    return HEADING_ORDINAL_PATTERN.sub("", _clean_line(value), count=1).strip()


def _group_labels(block_lines: list[tuple[int, str]]) -> list[str]:
    for _, line in block_lines[:8]:
        line = _clean_line(line)
        if not line.startswith("組別"):
            continue
        rest = line[len("組別") :].strip(" ：:")
        groups: list[str] = []
        for match in GROUP_TOKEN_PATTERN.finditer(rest):
            group = _normalize_group_label(match.group(0))
            if group and group not in groups:
                groups.append(group)
        if groups:
            return groups
    return []


def _get_or_create_record(
    records: list[dict[str, Any]],
    records_by_identity: dict[tuple[str, str], dict[str, Any]],
    name: str,
    group: str,
    page_number: int,
    evidence: str,
    *,
    printed_program_code: str = "",
) -> dict[str, Any]:
    identity = (name, group)
    record = records_by_identity.get(identity)
    if record is None:
        if len(records) >= 999:
            return records[-1]
        # The system-internal code is always a document-order ordinal: brochures
        # rarely carry a UAC/分發 code and the extraction contract requires a
        # 3-digit value. Any code the brochure prints itself (e.g. 「QS116」) is
        # kept as source metadata for the reviewer.
        code = f"{len(records) + 1:03d}"
        record = {
            "program_code": code,
            "source_program_code": printed_program_code,
            "admission_program_name": f"{name} {group}",
            "admission_quota": None,
            "consultation_phone": "",
            "consultation_email": "",
            "consultation_contact": "",
            "brochure_url": "",
            "exam_items": [],
            "source_page": min(page_number, 999),
            "evidence": [],
        }
        records.append(record)
        records_by_identity[identity] = record

    evidence = evidence[:1000]
    if evidence and evidence not in [item["text"] for item in record["evidence"]]:
        record["evidence"].append({"page": min(page_number, 999), "text": evidence})
    return record


def _update_record_quota(
    record: dict[str, Any],
    lines: list[str],
    *,
    quota: int | None = None,
    allow_fallback: bool = True,
) -> None:
    if quota is None and allow_fallback:
        quota = _find_quota(lines)
    if quota is not None and record.get("admission_quota") is None:
        record["admission_quota"] = quota


def _update_record_fields(
    record: dict[str, Any],
    block_lines: list[tuple[int, str]],
    *,
    group_index: int = 0,
    group_count: int = 1,
) -> None:
    fields = _collect_program_fields(
        block_lines,
        group_index=group_index,
        group_count=group_count,
    )
    if fields["consultation_phone"] and not record["consultation_phone"]:
        record["consultation_phone"] = fields["consultation_phone"]
    if fields["consultation_email"] and not record["consultation_email"]:
        record["consultation_email"] = fields["consultation_email"]
    if fields["consultation_contact"] and not record["consultation_contact"]:
        record["consultation_contact"] = fields["consultation_contact"]
    if fields["brochure_url"] and not record["brochure_url"]:
        record["brochure_url"] = fields["brochure_url"]
    if fields["exam_items"] and not record["exam_items"]:
        record["exam_items"] = fields["exam_items"]


def _collect_program_fields(
    block_lines: list[tuple[int, str]],
    *,
    group_index: int = 0,
    group_count: int = 1,
) -> dict[str, Any]:
    exam_items: list[dict[str, Any]] = []
    consultation_phone = ""
    consultation_email = ""
    consultation_contact = ""
    brochure_url = ""
    for index, (page_number, _) in enumerate(block_lines):
        field_window = " ".join(
            _clean_line(value)
            for _, value in block_lines[index : index + 4]
        )
        if not consultation_phone:
            consultation_phone = _phone(field_window)
        if not consultation_email:
            consultation_email = _email(field_window)
        if not consultation_contact:
            consultation_contact = _contact(field_window)
        if not brochure_url:
            brochure_url = _brochure_url(field_window)

    exam_items = _extract_exam_items(
        block_lines,
        group_index=group_index,
        group_count=group_count,
    )
    if not exam_items:
        for page_number, raw_line in block_lines:
            line = _clean_line(raw_line)
            row = _exam_item_row(line, page_number)
            if row is not None:
                _append_exam_item(exam_items, row)

    if not exam_items:
        for page_number, raw_line in block_lines:
            line = _clean_line(raw_line)
            item_name = _exam_name(line)
            if not item_name:
                continue
            weight_match = WEIGHT_PATTERN.search(line)
            _append_exam_item(
                exam_items,
                {
                    "name": item_name,
                    "sort_order": len(exam_items) + 1,
                    "weight_percent": float(weight_match.group(1)) if weight_match else None,
                    "description": line[:1000],
                    "source_page": str(min(page_number, 999)),
                },
            )

    return {
        "consultation_phone": consultation_phone,
        "consultation_email": consultation_email,
        "consultation_contact": consultation_contact,
        "brochure_url": brochure_url,
        "exam_items": exam_items[:100],
    }


def _is_exam_section_header(line: str) -> bool:
    line = _clean_line(line)
    if line == "考試項目":
        return True
    if not line.startswith(("考試項目", "考試科目", "甄試項目", "考試方式")):
        return False
    return any(
        marker in line
        for marker in ("計分", "計算", "配分", "比例", "比率", "方式")
    )


def _is_exam_section_end(line: str) -> bool:
    return _clean_line(line).startswith(EXAM_SECTION_END_PREFIXES)


def _extract_exam_items(
    block_lines: list[tuple[int, str]],
    *,
    group_index: int = 0,
    group_count: int = 1,
) -> list[dict[str, Any]]:
    """Extract the official exam stages from a department table.

    Brochures often show a stage total (for example, ``面試 60%``) and then
    list that stage's supporting documents or scoring dimensions below it.
    ``program_exam_items`` is a flat API, so only the stage totals become
    items. The lower-level rows are kept in the stage description; treating
    them as additional overall percentages would make the data incorrect.
    """

    lines = [_clean_line(line) for _, line in block_lines]
    exam_start = next(
        (
            index
            for index, line in enumerate(lines)
            if _is_exam_section_header(line)
        ),
        None,
    )
    if exam_start is None:
        return []

    exam_end = next(
        (
            index
            for index in range(exam_start + 1, len(lines))
            if _is_exam_section_end(lines[index])
        ),
        len(lines),
    )
    section = lines[exam_start + 1 : exam_end]
    if not section:
        return []

    stage_markers = [
        index for index, line in enumerate(section) if _is_stage_label(line)
    ]
    if not stage_markers:
        return _extract_flat_exam_items(section, block_lines)

    items: list[dict[str, Any]] = []
    stage_items: dict[str, dict[str, Any]] = {}
    for marker_index, stage_index in enumerate(stage_markers):
        stage_end = (
            stage_markers[marker_index + 1]
            if marker_index + 1 < len(stage_markers)
            else len(section)
        )
        stage_kind = _stage_label_kind(section[stage_index]) or (
            "initial" if marker_index == 0 else "review"
        )
        stage_key = (
            stage_kind
            if stage_kind != "stage"
            else f"stage:{_stage_label(section[stage_index]) or marker_index}"
        )
        stage_display = _stage_label(section[stage_index]) or {
            "initial": "初試",
            "review": "複試",
        }.get(stage_kind, "-")
        stage_segment = _select_group_exam_segment(
            section,
            stage_index,
            stage_end,
            group_index,
            group_count,
            kind=stage_kind,
        )
        item = _append_exam_segment_items(
            items,
            stage_segment,
            block_lines,
            stage_kind,
            stage_weight=_stage_weight(section[stage_index:stage_end], stage_kind),
            stage_display=stage_display,
        )

        if item is not None and item.get("weight_percent") is not None:
            stage_items.setdefault(stage_key, item)
            continue

        # Many brochures repeat 「初試（資料審查）」 or 「複試（口試）」
        # below the score table and put the actual documents, time and method
        # there.  It is supporting content for the weighted stage above, not a
        # second exam item.
        existing = stage_items.get(stage_key)
        if existing is not None:
            _merge_exam_description(
                existing,
                _stage_support_description(stage_segment),
            )

    designated_materials = _designated_materials_before_exam(lines, exam_start)
    if designated_materials and items:
        initial_item = stage_items.get("initial") or items[0]
        _merge_exam_description(initial_item, designated_materials)

    return items or _extract_flat_exam_items(section, block_lines)


def _extract_flat_exam_items(
    section: list[str],
    block_lines: list[tuple[int, str]],
) -> list[dict[str, Any]]:
    """Extract ordinary numbered or percentage rows without stage labels."""

    chinese_numbered = _extract_chinese_numbered_exam_items(section, block_lines)
    if chinese_numbered:
        return chinese_numbered

    items: list[dict[str, Any]] = []
    source_page = str(min((page for page, _ in block_lines), default=1))
    for line in _join_wrapped_lines(section):
        row = _exam_item_row(line, int(source_page))
        if row is not None:
            _append_exam_item(items, row)
            continue

        percent_match = PERCENT_ITEM_PATTERN.search(line)
        if percent_match:
            name = _clean_exam_item_name(percent_match.group("name"))
            if name:
                _append_exam_item(
                    items,
                    {
                        "name": name,
                        "sort_order": len(items) + 1,
                        "weight_percent": float(percent_match.group("weight")),
                        "description": line,
                        "source_page": source_page,
                    },
                )
            continue

        trailing_weight = re.match(
            r"^\s*(?P<name>.+?)[：:,，]?\s+(?P<weight>"
            r"[0-9]{1,3}(?:\.[0-9]+)?)\s*%\s*$",
            line,
        )
        if trailing_weight:
            name = _clean_exam_item_name(trailing_weight.group("name"))
            if name:
                _append_exam_item(
                    items,
                    {
                        "name": name,
                        "sort_order": len(items) + 1,
                        "weight_percent": float(trailing_weight.group("weight")),
                        "description": line,
                        "source_page": source_page,
                    },
                )
    return items


def _extract_chinese_numbered_exam_items(
    section: list[str],
    block_lines: list[tuple[int, str]],
) -> list[dict[str, Any]]:
    """Parse table rows labelled 一、二… even when PDF columns wrap."""

    marker_pattern = re.compile(
        r"^(?P<marker>[一二三四五六七八九十])(?:[、.．])?\s*(?P<text>.*)$"
    )
    markers = [
        index
        for index, line in enumerate(section)
        if marker_pattern.match(_clean_line(line))
    ]
    if not markers:
        return []

    source_page = str(min((page for page, _ in block_lines), default=1))
    items: list[dict[str, Any]] = []
    for marker_index, start in enumerate(markers):
        end = markers[marker_index + 1] if marker_index + 1 < len(markers) else len(section)
        match = marker_pattern.match(_clean_line(section[start]))
        if match is None:
            continue
        segment = [match.group("text")] + section[start + 1 : end]
        text = _clean_exam_description(" ".join(_join_wrapped_lines(segment)))
        weights = list(WEIGHT_PATTERN.finditer(text))
        if not weights:
            continue
        weight = float(weights[-1].group(1))
        body = WEIGHT_PATTERN.sub("", text).strip(" ：:,，。；;、．.")
        short_name = _clean_exam_item_name(body)
        if not short_name or len(short_name) > 40:
            short_name = f"審查項目{match.group('marker')}"
        _append_exam_item(
            items,
            {
                "name": short_name,
                "sort_order": len(items) + 1,
                "weight_percent": weight,
                "description": text,
                "source_page": source_page,
            },
        )
    return items


def _select_group_exam_segment(
    section: list[str],
    start: int,
    end: int,
    group_index: int,
    group_count: int,
    *,
    kind: str,
) -> list[str]:
    segment = section[start:end]
    if group_count <= 1:
        return segment

    markers = [
        index for index, value in enumerate(segment)
        if _is_exam_item_heading(value)
    ]
    if len(markers) < group_count:
        return segment

    selected_index = min(group_index, group_count - 1)
    selected_start = 0 if selected_index == 0 else markers[selected_index]
    selected_end = (
        markers[selected_index + 1]
        if selected_index + 1 < len(markers)
        else len(segment)
    )
    return segment[selected_start:selected_end]


def _append_exam_segment_items(
    items: list[dict[str, Any]],
    segment: list[str],
    block_lines: list[tuple[int, str]],
    stage: str,
    stage_weight: float | None = None,
    stage_display: str = "-",
) -> dict[str, Any] | None:
    if not segment:
        return None

    joined_lines = _join_wrapped_lines(segment)
    stage_weight = stage_weight if stage_weight is not None else _stage_weight(joined_lines, stage)
    stage_name = _stage_name(joined_lines, stage)
    source_page = str(min(block_lines[0][0], 999)) if block_lines else "1"
    if stage_name and stage_weight is not None:
        item = {
            "name": stage_name,
            "stage": stage_display or "-",
            "sort_order": len(items) + 1,
            "weight_percent": stage_weight,
            "description": _stage_description(joined_lines, stage_name, stage),
            "source_page": source_page,
        }
        _append_exam_item(
            items,
            item,
        )
        return next((entry for entry in items if entry["name"] == item["name"]), None)
    return None


def _merge_exam_description(item: dict[str, Any], value: str) -> None:
    value = _clean_exam_description(value)
    if not value:
        return
    existing = _clean_exam_description(str(item.get("description", "")))
    if value in existing:
        return
    item["description"] = f"{existing} {value}".strip()[:10000]


def _stage_support_description(lines: list[str]) -> str:
    parts: list[str] = []
    for index, raw_line in enumerate(_join_wrapped_lines(lines)):
        line = _clean_line(raw_line)
        if index == 0 and _is_stage_label(line):
            continue
        if not line or _is_weight_only(line) or _is_stage_marker_only(line):
            continue
        if _is_exam_section_header(line) or _is_exam_section_end(line):
            continue
        if re.fullmatch(r"[【\[].{1,30}[】\]]", line):
            continue
        parts.append(line)
    return _clean_exam_description(" ".join(parts))[:10000]


def _designated_materials_before_exam(lines: list[str], exam_start: int) -> str:
    """Return the nearest explicitly labelled submission block.

    The anchor is deliberately strict: generic mentions of review documents
    in eligibility prose must not leak into a department's exam description.
    """

    anchor_pattern = re.compile(
        r"^[【\[]?(?:指定)?(?:繳交|上傳|應繳|應上傳)(?:之)?"
        r"(?:書面)?(?:審查)?資料[】\]]?\s*[:：]?"
    )
    anchor = None
    for index in range(exam_start - 1, max(-1, exam_start - 100), -1):
        if anchor_pattern.match(_clean_line(lines[index])):
            anchor = index
            break
    if anchor is None:
        return ""

    parts: list[str] = []
    for raw_line in lines[anchor:exam_start]:
        line = _clean_line(raw_line)
        if not line:
            continue
        line = anchor_pattern.sub("", line, count=1).strip()
        if line:
            parts.append(line)
    return _clean_exam_description(" ".join(parts))[:10000]


def _join_wrapped_lines(lines: list[str]) -> list[str]:
    joined: list[str] = []
    current = ""
    for raw_line in lines:
        line = _clean_line(raw_line)
        if not line:
            continue
        if (
            current
            and _has_unclosed_parenthesis(current)
            and not re.match(r"^\s*[0-9]{1,3}\s*[.)、．.]", line)
        ):
            current = f"{current}{line}"
            continue
        if current and _looks_like_wrapped_exam_continuation(current, line):
            current = f"{current}{line}"
            continue
        if current:
            joined.append(current)
        current = line
    if current:
        joined.append(current)
    return joined


def _looks_like_wrapped_exam_continuation(current: str, line: str) -> bool:
    """Join a short PDF text fragment when the preceding row is visibly cut."""
    if WEIGHT_PATTERN.search(current):
        return False
    if re.match(r"^\s*[0-9]{1,3}\s*[.)、．.]", line):
        return False
    if _is_exam_heading_or_meta(line, "review"):
        return False
    if WEIGHT_PATTERN.search(line) or PERCENT_ITEM_PATTERN.search(line):
        return (
            _is_exam_item_heading(current) or _is_stage_label(current)
        ) and not _is_exam_item_heading(line)
    if _is_exam_item_heading(current) and not _is_exam_item_heading(line):
        return True
    if len(current) < 16 or len(line) > 8:
        return False
    if current.endswith((":", "：", ".", "。", ";", "；", ",", "，", "、", ")", "）")):
        return False
    return True


def _has_unclosed_parenthesis(value: str) -> bool:
    return value.count("(") + value.count("（") > value.count(")") + value.count("）")


def _stage_label(value: str) -> str:
    match = STAGE_LABEL_PATTERN.match(_clean_line(value))
    return match.group("label") if match else ""


def _is_stage_label(value: str) -> bool:
    return bool(_stage_label(value))


def _strip_stage_label(value: str) -> str:
    value = _clean_line(value)
    match = STAGE_LABEL_PATTERN.match(value)
    if not match:
        return value
    return value[match.end() :].strip(" ：:")


def _is_stage_marker_only(value: str) -> bool:
    if not _is_stage_label(value):
        return False
    remainder = _strip_stage_label(value)
    return not remainder or _is_weight_only(remainder)


def _stage_label_kind(value: str) -> str:
    label = _stage_label(value)
    if not label:
        return ""
    if label in {"初審", "初試", "初選"} or re.match(
        r"^第?(?:1|一)(?:階段|階|關)$", label
    ):
        return "initial"
    if label in {"複審", "複試", "複選"} or re.match(
        r"^第?(?:2|二)(?:階段|階|關)$", label
    ):
        return "review"
    return "stage"


def _first_stage_marker(
    section: list[str],
    kind: str,
    *,
    start: int = 0,
) -> int | None:
    for index in range(max(0, start), len(section)):
        if _stage_label_kind(section[index]) == kind:
            return index
    return None


def _is_weight_only(value: str) -> bool:
    compact = re.sub(
        r"[\s:：()（）\[\]【】]",
        "",
        _clean_line(value),
    )
    return bool(re.fullmatch(r"[0-9]{1,3}(?:\.[0-9]+)?%", compact))


def _is_exam_item_heading(value: str) -> bool:
    """Identify a stage heading without relying on a particular university."""

    line = _clean_line(value)
    if not line or _is_stage_marker_only(line) or _is_weight_only(line):
        return False
    if re.match(r"^\s*[0-9]{1,3}\s*[.)、．.]", line):
        return False
    candidate = _strip_stage_label(line)
    candidate = re.sub(
        r"^\s*[0-9]{1,3}\s*[.)、．.]\s*",
        "",
        candidate,
    )
    if not candidate or candidate.startswith(("考試項目", "總成績")):
        return False
    if any(candidate.startswith(prefix) for prefix in EXAM_HEADING_PREFIXES):
        return True
    if "書面" in candidate and "審查" in candidate:
        return True
    if candidate.startswith("資料審查"):
        return True
    return False


def _exam_heading_name(value: str) -> str:
    candidate = _strip_stage_label(value)
    candidate = re.sub(
        r"^\s*[0-9]{1,3}\s*[.)、．.]\s*",
        "",
        candidate,
    )
    return _clean_exam_item_name(candidate)


def _exam_line_remainder(value: str, name: str) -> str:
    candidate = _strip_stage_label(value)
    candidate = re.sub(
        r"^\s*[0-9]{1,3}\s*[.)、．.]\s*",
        "",
        candidate,
    )
    if not name or not candidate.startswith(name):
        return ""
    remainder = candidate[len(name) :]
    return remainder.strip(" ：:,，。；;、．.")


def _stage_description(lines: list[str], stage_name: str, stage: str) -> str:
    """Keep stage details as text while excluding labels and total weights."""

    parts: list[str] = []
    found_name = False
    for line in lines:
        line = _clean_line(line)
        if not line or _is_weight_only(line) or _is_stage_marker_only(line):
            continue
        if not found_name:
            if _exam_heading_name(line) == stage_name:
                found_name = True
                remainder = _exam_line_remainder(line, stage_name)
                if remainder and not _is_weight_only(remainder):
                    parts.append(remainder)
                continue
            if _is_exam_item_heading(line):
                # The heading can contain PDF punctuation or a wrapped suffix
                # that is removed from its normalized name. Start collecting
                # from the first plausible heading in that case.
                found_name = True
                remainder = _exam_line_remainder(line, stage_name)
                if remainder and not _is_weight_only(remainder):
                    parts.append(remainder)
                continue
            continue
        if _is_stage_marker_only(line):
            break
        parts.append(line)
    return _clean_exam_description(" ".join(parts))[:10000]


def _stage_weight(lines: list[str], stage: str) -> float | None:
    for line in lines:
        line = _clean_line(line)
        if re.fullmatch(r"[0-9]{1,3}(?:\.[0-9]+)?\s*%", line):
            match = WEIGHT_PATTERN.search(line)
            if match:
                return float(match.group(1))
    for line in lines:
        line = _clean_line(line)
        if _stage_label_kind(line) == stage:
            matches = list(WEIGHT_PATTERN.finditer(line))
            if matches:
                return float(matches[-1].group(1))
    for line in lines:
        line = _clean_line(line)
        if _is_exam_item_heading(line):
            matches = list(WEIGHT_PATTERN.finditer(line))
            if matches:
                return float(matches[-1].group(1))
    all_matches = [
        match
        for line in lines
        for match in WEIGHT_PATTERN.finditer(_clean_line(line))
    ]
    if len(all_matches) == 1:
        return float(all_matches[0].group(1))
    return None


def _stage_name(lines: list[str], stage: str) -> str:
    for line in _join_wrapped_lines(lines):
        if _is_exam_heading_or_meta(line, stage):
            continue
        if not _is_exam_item_heading(line):
            continue
        name = _exam_heading_name(line)
        if name:
            return name
    for line in _join_wrapped_lines(lines):
        if not WEIGHT_PATTERN.search(line):
            continue
        inferred = _infer_exam_name_from_keywords(_strip_stage_label(line))
        if inferred:
            return inferred
    return _infer_exam_name_from_keywords(" ".join(_join_wrapped_lines(lines)))


def _infer_exam_name_from_keywords(value: str) -> str:
    value = _clean_line(value)
    terms: list[tuple[int, str]] = []
    occupied: list[tuple[int, int]] = []
    for term in (
        "書面資料審查",
        "書面審查",
        "資料審查",
        "作品審查",
        "實作測驗",
        "術科測驗",
        "能力測驗",
        "面試",
        "口試",
        "筆試",
        "術科",
    ):
        for match in re.finditer(re.escape(term), value):
            span = match.span()
            if any(span[0] < end and span[1] > start for start, end in occupied):
                continue
            occupied.append(span)
            terms.append((span[0], term))
    names = [name for _, name in sorted(terms)]
    return "及".join(_unique(names))


def _is_exam_heading_or_meta(line: str, stage: str) -> bool:
    if not line or _is_weight_only(line):
        return True
    if _is_stage_marker_only(line):
        return True
    if line in {"初審", "複審", "考試項目", "及計分", "各項目所占比率(%)"}:
        return True
    if "參酌順序" in line or line.startswith("總成績"):
        return True
    if stage == "review" and line.startswith("複審考試時間"):
        return True
    return False


def _clean_exam_item_name(value: str) -> str:
    value = _strip_parenthetical_notes(value)
    value = re.sub(r"^複審\s*", "", value)
    value = re.sub(r"\s*[0-9]{1,3}(?:\.[0-9]+)?\s*%\s*$", "", value)
    # A few PDFs put the explanation after the item name with a comma or
    # colon instead of parentheses (for example, 「面試，含專業認知」).
    value = re.split(r"[,，:：]", value, maxsplit=1)[0]
    value = re.sub(r"^[\s:：、．.\-，,。；;]+|[\s:：、．.\-，,。；;]+$", "", value)
    for canonical in (
        "書面資料審查",
        "書面審查",
        "資料審查",
        "作品審查",
        "面試",
        "口試",
        "筆試",
    ):
        if not value.startswith(f"{canonical} "):
            continue
        remainder = value[len(canonical) :].strip()
        if remainder and not remainder.startswith(("及", "與", "暨", "-", "／", "/")):
            value = canonical
        break
    return " ".join(value.split()).strip()


def _clean_exam_description(value: str) -> str:
    value = _clean_line(value)
    value = re.sub(
        r"(?<![0-9])(?:^|[\s,，;；:：])\s*[0-9]{1,3}"
        r"\s*(?:[.)、．.]|\s+)\s*(?=\D|$)",
        " ",
        value,
    )
    return " ".join(value.split()).strip()


def _strip_parenthetical_notes(value: str) -> str:
    """Remove parenthetical annotations, including an unmatched PDF bracket."""
    result: list[str] = []
    depth = 0
    for character in value:
        if character in "(（":
            depth += 1
            continue
        if character in ")）":
            if depth > 0:
                depth -= 1
            continue
        if depth == 0:
            result.append(character)
    return "".join(result)


_CONTACT_PARENS_PATTERN = re.compile(r"[（(]\s*(?:請洽|洽詢|洽|聯絡人[:：]?|連絡人[:：]?)\s*([^（）()]{1,40}?)\s*[）)]")
_CONTACT_LABEL_PATTERN = re.compile(
    r"(?:聯絡人|連絡人|承辦人|洽詢單位|聯絡單位|系所辦公室|系辦)\s*[:：]\s*([^\s，,。；;]{1,40})"
)


def _phone(line: str) -> str:
    match = PHONE_PATTERN.search(line)
    if not match:
        return ""
    value = match.group("value").strip(" ，,。；;:")
    # The phone value keeps only the number(s) and extension. The e-mail and the
    # contact person / unit are stored on their own fields.
    value = EMAIL_PATTERN.sub("", value)
    value = re.sub(r"[；;，,、\s]*(?:E[-－–]?mail|電子郵件|電子信箱|信箱)\s*[:：]?\s*$", "", value, flags=re.IGNORECASE)
    value = _CONTACT_PARENS_PATTERN.sub("", value)
    value = _CONTACT_LABEL_PATTERN.sub("", value)
    value = re.sub(r"[（(]\s*[）)]", "", value)
    return value.strip(" ，,。；;:、")


def _email(line: str) -> str:
    match = EMAIL_PATTERN.search(line)
    return match.group(0) if match else ""


def _contact(line: str) -> str:
    for pattern in (_CONTACT_PARENS_PATTERN, _CONTACT_LABEL_PATTERN):
        match = pattern.search(line)
        if match:
            return match.group(1).strip(" ，,。；;:、")
    return ""


def _brochure_url(line: str) -> str:
    match = URL_PATTERN.search(line)
    if match:
        return match.group(1).rstrip("，,。；;）)")
    direct = re.search(r"https?://[^\s，,。；;）)]+", line)
    return direct.group(0).rstrip("，,。；;）)") if direct else ""


def _exam_item_row(line: str, page_number: int) -> dict[str, Any] | None:
    match = EXAM_ITEM_ROW_PATTERN.match(line)
    if not match:
        return None
    name = match.group("name").strip(" ：:、．.")
    if not name or name in {"考試項目", "各項目計分比例"}:
        return None
    return {
        "name": name,
        "sort_order": int(match.group("order")),
        "weight_percent": float(match.group("weight")),
        "description": line[:1000],
        "source_page": str(min(page_number, 999)),
    }


def _append_exam_item(items: list[dict[str, Any]], item: dict[str, Any]) -> None:
    item = dict(item)
    item["name"] = _clean_exam_item_name(str(item.get("name", "")))
    item["description"] = _clean_exam_description(str(item.get("description", "")))
    # Stage (初試／複試／第一階段…) is optional; keep the flat item list but let
    # the reviewer see which round each item belongs to. Item names stay
    # whatever the brochure prints (書審／面試／筆試…), which varies per school.
    item.setdefault("stage", "-")
    if not item["name"]:
        return
    existing = next((entry for entry in items if entry["name"] == item["name"]), None)
    if existing is None:
        items.append(item)
        return
    if existing.get("weight_percent") is None and item.get("weight_percent") is not None:
        existing["weight_percent"] = item["weight_percent"]
    if not existing.get("description"):
        existing["description"] = item.get("description", "")
    if not existing.get("source_page"):
        existing["source_page"] = item.get("source_page", "-")
    if existing.get("stage", "-") == "-" and item.get("stage", "-") != "-":
        existing["stage"] = item["stage"]


def _split_program_label(value: str) -> tuple[str, str]:
    value = _clean_line(value)
    if not value:
        return "", ""

    population_group = ""
    population = re.search(r"\s+(新住民(?:生)?組|一般生組|一般組)\s*$", value)
    if population:
        population_group = _normalize_group_label(population.group(1))
        value = value[: population.start()].rstrip()

    for label in sorted(PROGRAM_GROUP_LABELS, key=len, reverse=True):
        if not value.endswith(label):
            continue
        name = value[: -len(label)].rstrip()
        if name.endswith(("(", "（")):
            name = name[:-1].rstrip()
        return name, _combine_program_groups(_normalize_group_label(label), population_group)

    wrapped = re.search(r"[（(]\s*([^（）()]{1,30})\s*[）)]\s*$", value)
    if wrapped:
        group = _normalize_group_label(wrapped.group(1))
        if group:
            return value[: wrapped.start()].rstrip(), _combine_program_groups(group, population_group)

    attached_group = re.match(
        r"^(?P<name>.+?(?:學位學程|學士學位學程|學系|研究所|系所|學程|學士班|系))"
        r"[-－—\s]*(?P<group>[^\s（）()]{1,30}組)$",
        value,
    )
    if attached_group:
        group = _normalize_group_label(attached_group.group("group"))
        if group:
            return (
                attached_group.group("name").strip(),
                _combine_program_groups(group, population_group),
            )

    bare_group = re.search(r"\s+([\u4e00-\u9fffA-Za-z0-9&＋+/．.]{1,30}組)\s*$", value)
    if bare_group:
        return (
            value[: bare_group.start()].rstrip(),
            _combine_program_groups(
                _normalize_group_label(bare_group.group(1)),
                population_group,
            ),
        )
    return value, population_group


def _combine_program_groups(group: str, population_group: str) -> str:
    if not group:
        return population_group
    if not population_group or population_group == "一般組" or population_group in group:
        return group
    return f"{group} {population_group}"


def _normalize_group_label(value: str) -> str:
    value = _clean_line(value).strip("()（）").strip().replace("計劃", "計畫")
    if value in {"一般", "一般生", "一般生組", "一般組"}:
        return "一般組"
    if value in {"願景計畫", "願景計畫組"}:
        return "願景計畫組"
    if value in {"新住民", "新住民生", "新住民組", "新住民生組"}:
        return "新住民組"
    if re.fullmatch(r"[\u4e00-\u9fffA-Za-z0-9&＋+/．.]{1,30}組", value):
        return value
    return ""


def _looks_like_program_name(value: str) -> bool:
    value = value.strip()
    if len(value) < 2:
        return False
    return not any(
        marker in value
        for marker in (
            "分則",
            "考試日期",
            "考試方式",
            "願景計畫",
            "一般生",
            "招生名額",
        )
    )


def _program_detail_starts(value: str) -> bool:
    value = _clean_line(value)
    return value.startswith(
        (
            "招生名額",
            "組別",
            "報考附加",
            "應上傳",
            "願景計畫",
            "報名資格",
            "指定繳交",
            "考試項目",
            "初審",
            "複審",
            "同分參酌",
            "總成績",
            "注意事項",
            "其他注意",
            "學系網址",
            "連絡資訊",
            "聯絡方式",
        )
    )


def _academic_year(value: str) -> int:
    match = ACADEMIC_YEAR_PATTERN.search(value)
    if match:
        return int(match.group(1))
    match = WESTERN_ACADEMIC_YEAR_PATTERN.search(value)
    if match:
        year = int(match.group(1)) - 1911
        return year if 100 <= year <= 999 else 0
    return 0


def _school_code(value: str) -> str:
    for match in SCHOOL_CODE_PATTERN.finditer(value):
        code = match.group(1)
        if code != "000":
            return code
    return ""


def _school_name(value: str) -> str:
    for match in SCHOOL_NAME_PATTERN.finditer(value):
        name = _institution_name(match.group(1))
        if name:
            return name
        candidate = _clean_line(match.group(1)).strip(" ：:，,。；;")
        if candidate:
            return candidate
    for raw_line in value.splitlines():
        line = _clean_line(raw_line)
        if not line or "簡章" in line or len(line) > 100:
            continue
        name = _institution_name(line)
        if name:
            return name
    return ""


def _institution_name(value: str) -> str:
    match = INSTITUTION_NAME_PATTERN.search(_clean_line(value))
    return match.group("name") if match else ""


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
    cleaned = [_clean_line(line) for line in lines]
    for line in cleaned:
        match = QUOTA_PATTERN.search(line)
        if match:
            return int(match.group(1))
    # Some brochures stack the label and the value on separate lines
    # (「招生」 / 「名額」 / 「3名」), so pair a bare "N名" line with a nearby
    # 名額 label instead of requiring the two on one line.
    for index, line in enumerate(cleaned):
        if re.fullmatch(r"\d{1,3}\s*名", line):
            window = cleaned[max(0, index - 3) : index + 4]
            if any("名額" in neighbour for neighbour in window):
                return int(re.match(r"\d+", line).group())
    return None


def _find_group_quotas(
    block_lines: list[tuple[int, str]],
    group_count: int,
) -> list[int]:
    if group_count <= 0:
        return []
    for index, (_, raw_line) in enumerate(block_lines):
        line = _clean_line(raw_line)
        if not line.startswith("招生名額"):
            continue
        window = " ".join(
            _clean_line(value)
            for _, value in block_lines[index : index + 4]
        )
        values = [int(match.group(1)) for match in QUOTA_VALUE_PATTERN.finditer(window)]
        if len(values) >= group_count:
            return values[:group_count]
    return []


def _collect_schedule_fields(
    job: BrochureExtractJob,
    lines_by_page: list[list[str]],
    *,
    academic_year: int = 0,
) -> dict[str, Any]:
    # Each detected milestone becomes one 招生時程 (timeline_events) entry.
    milestones: dict[str, list[str]] = {}
    fallback_year = academic_year or job.academic_year
    for lines in lines_by_page:
        normalized_lines = [_clean_line(raw_line) for raw_line in lines]
        for index, line in enumerate(normalized_lines):
            if not line:
                continue
            # Labels and their dates are frequently split across separate PDF
            # text lines (especially in the important-schedule table).
            window = _schedule_window(normalized_lines, index)
            dates = _dates(window, fallback_year)
            if not dates:
                continue
            # Cover-page notes such as 「本簡章經 115 年 8 月 20 日招生委員會
            # 會議通過」 carry a date but are not part of the applicant schedule.
            if _contains_any(window, SCHEDULE_EXCLUDE_KEYWORDS):
                continue
            if _contains_any(line, REGISTRATION_KEYWORDS):
                milestones.setdefault("網路報名", dates)
            if _is_exam_schedule_label(line):
                milestones.setdefault("甄試", dates)
            if _contains_any(line, RESULT_KEYWORDS):
                milestones.setdefault("錄取放榜", dates[:1])
            if _contains_any(line, ANNOUNCEMENT_KEYWORDS):
                milestones.setdefault("簡章公告", dates[:1])
    if not milestones:
        return {}
    events = []
    for order, (name, dates) in enumerate(milestones.items(), start=1):
        start, end = dates[0], dates[-1]
        events.append(
            {
                "name": name,
                "start_date": start,
                "start_time": "-",
                "end_date": end if end != start else "-",
                "end_time": "-",
                "sort_order": order,
                "notes": "-",
            }
        )
    return {"timeline_events": events}


def _schedule_window(lines: list[str], start: int) -> str:
    values = [lines[start]]
    for line in lines[start + 1 : start + 5]:
        if _is_schedule_boundary(line):
            break
        if line:
            values.append(line)
    return " ".join(values)


def _is_schedule_boundary(line: str) -> bool:
    return _contains_any(
        line,
        (
            "簡章公告",
            "公告簡章",
            "簡章發售",
            "簡章下載",
            "網路報名",
            "報名",
            "申請",
            "繳費",
            "複審",
            "面試",
            "口試",
            "筆試",
            "甄試",
            "考試",
            "放榜",
            "榜單",
            "錄取公告",
            "成績公告",
        ),
    )


def _is_exam_schedule_label(line: str) -> bool:
    if _contains_any(line, ("公告", "名單", "試場", "准考證", "應考須知", "梯次表")):
        return False
    # 「本項考試實際招生名額…」 / 「考試科目及規定」 are not timetable rows.
    if _contains_any(line, ("名額", "資格", "科目及", "及規定", "違規", "試場規則")):
        return False
    return _contains_any(line, EXAM_KEYWORDS)



def _dates(value: str, academic_year: int) -> list[str]:
    result: list[str] = []
    explicit_year: int | None = academic_year or None
    for match in DATE_PATTERN.finditer(value):
        if match.group("year") is not None:
            explicit_year = int(match.group("year"))
        if explicit_year is None:
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
    if "考試項目" in line or "同分參酌" in line:
        return ""
    value = re.split(r"[:：]", line, maxsplit=1)[0]
    for keyword in EXAM_ITEM_KEYWORDS:
        if keyword in value:
            return keyword
    return ""


def _contains_any(value: str, keywords: tuple[str, ...]) -> bool:
    return any(keyword in value for keyword in keywords)


def _unique(values: list[str]) -> list[str]:
    return list(dict.fromkeys(values))


def _excerpt(value: str, limit: int = 3000) -> str:
    return " ".join(value.split())[:limit]
