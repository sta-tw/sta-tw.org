#!/usr/bin/env python3
"""Validate candidate JSON emitted by an admissions extractor or external agent."""

from __future__ import annotations

import argparse
import json
import re
import sys
from pathlib import Path
from typing import Any

from validate_official_url import validate as validate_url


DOCUMENT_TYPES = {"brochure", "announcement", "stage_notice", "result", "waitlist_notice", "unknown"}
REVIEW_STATUSES = {"pending", "needs_review", "approved", "rejected"}
CONFIDENCE_VALUES = {"high", "medium", "low"}
ISO_DATE = re.compile(r"^\d{4}-\d{2}-\d{2}$")
ISO_TIME = re.compile(r"^(?:[01]\d|2[0-3]):[0-5]\d$")
SHA256 = re.compile(r"^[0-9a-fA-F]{64}$")


def _is_nonempty_string(value: Any) -> bool:
    return isinstance(value, str) and bool(value.strip())


def _require(mapping: dict[str, Any], key: str, path: str, errors: list[str]) -> Any:
    if key not in mapping:
        errors.append(f"{path}.{key}: required")
        return None
    return mapping[key]


def _check_enum(mapping: dict[str, Any], key: str, values: set[str], path: str, errors: list[str]) -> None:
    value = mapping.get(key)
    if value not in values:
        errors.append(f"{path}.{key}: must be one of {sorted(values)}")


def _check_evidence_ref(mapping: dict[str, Any], path: str, errors: list[str]) -> None:
    value = mapping.get("evidence_ref")
    if not _is_nonempty_string(value):
        errors.append(f"{path}.evidence_ref: required")


def _check_date(value: Any, path: str, errors: list[str]) -> None:
    if value is not None and (not isinstance(value, str) or not ISO_DATE.fullmatch(value)):
        errors.append(f"{path}: must be ISO date or null")


def _check_time(value: Any, path: str, errors: list[str]) -> None:
    if value is not None and (not isinstance(value, str) or not ISO_TIME.fullmatch(value)):
        errors.append(f"{path}: must be HH:MM or null")


def validate_payload(payload: Any) -> list[str]:
    errors: list[str] = []
    if not isinstance(payload, dict):
        return ["$: top-level value must be an object"]

    document = payload.get("document")
    if not isinstance(document, dict):
        return ["document: required object"]

    document_type = _require(document, "document_type", "document", errors)
    if document_type not in DOCUMENT_TYPES:
        errors.append(f"document.document_type: must be one of {sorted(DOCUMENT_TYPES)}")
    for key in ("title", "academic_year", "school_code", "source_url", "source_sha256"):
        value = _require(document, key, "document", errors)
        if value is not None and not _is_nonempty_string(value):
            errors.append(f"document.{key}: must be a non-empty string")

    source_url = document.get("source_url")
    if _is_nonempty_string(source_url):
        source_check = validate_url(source_url)
        if not source_check.get("accepted"):
            errors.append(f"document.source_url: {source_check.get('reason', 'invalid official URL')}")

    source_sha256 = document.get("source_sha256")
    if _is_nonempty_string(source_sha256) and not SHA256.fullmatch(source_sha256):
        errors.append("document.source_sha256: must be a 64-character hexadecimal SHA-256")
    _check_enum(document, "review_status", REVIEW_STATUSES, "document", errors)

    evidence = document.get("evidence")
    if not isinstance(evidence, list):
        errors.append("document.evidence: must be an array")
    else:
        for index, item in enumerate(evidence):
            path = f"document.evidence[{index}]"
            if not isinstance(item, dict):
                errors.append(f"{path}: must be an object")
                continue
            for key in ("page_or_locator", "text"):
                if not _is_nonempty_string(item.get(key)):
                    errors.append(f"{path}.{key}: required non-empty string")

    schedules = payload.get("schedules")
    if not isinstance(schedules, list):
        errors.append("schedules: must be an array")
    else:
        for index, item in enumerate(schedules):
            path = f"schedules[{index}]"
            if not isinstance(item, dict):
                errors.append(f"{path}: must be an object")
                continue
            for key in ("original_label", "original_text"):
                if not _is_nonempty_string(item.get(key)):
                    errors.append(f"{path}.{key}: required non-empty string")
            _check_date(item.get("start_date"), f"{path}.start_date", errors)
            _check_date(item.get("end_date"), f"{path}.end_date", errors)
            _check_time(item.get("start_time"), f"{path}.start_time", errors)
            _check_time(item.get("end_time"), f"{path}.end_time", errors)
            if item.get("start_time") is not None and item.get("start_date") is None:
                errors.append(f"{path}: start_time requires start_date")
            if item.get("end_time") is not None and item.get("end_date") is None:
                errors.append(f"{path}: end_time requires end_date")
            _check_enum(item, "confidence", CONFIDENCE_VALUES, path, errors)
            _check_enum(item, "review_status", REVIEW_STATUSES, path, errors)
            _check_evidence_ref(item, path, errors)

    results = payload.get("results")
    if not isinstance(results, list):
        errors.append("results: must be an array")
    else:
        for index, item in enumerate(results):
            path = f"results[{index}]"
            if not isinstance(item, dict):
                errors.append(f"{path}: must be an object")
                continue
            if not _is_nonempty_string(item.get("result_status_original")):
                errors.append(f"{path}.result_status_original: required non-empty string")
            _check_enum(item, "confidence", CONFIDENCE_VALUES, path, errors)
            _check_evidence_ref(item, path, errors)

    return errors


def main() -> int:
    parser = argparse.ArgumentParser(description="Validate admissions extraction JSON")
    parser.add_argument("input", nargs="?", help="JSON file; read stdin when omitted")
    args = parser.parse_args()

    try:
        raw = Path(args.input).read_text(encoding="utf-8") if args.input else sys.stdin.read()
        payload = json.loads(raw)
    except (OSError, json.JSONDecodeError) as exc:
        print(json.dumps({"valid": False, "errors": [f"input: {exc}"]}, ensure_ascii=False))
        return 1

    errors = validate_payload(payload)
    print(json.dumps({"valid": not errors, "errors": errors}, ensure_ascii=False, sort_keys=True))
    return 0 if not errors else 1


if __name__ == "__main__":
    sys.exit(main())
