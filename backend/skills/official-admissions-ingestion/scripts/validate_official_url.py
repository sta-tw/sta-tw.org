#!/usr/bin/env python3
"""Validate the URL boundary used by the official-admissions-ingestion skill.

The script deliberately performs no network request. It checks URL syntax and
the allowed Taiwan official suffixes so callers can apply the same rule to
seeds, links, redirects, and attachments.
"""

from __future__ import annotations

import argparse
import ipaddress
import json
import sys
from urllib.parse import SplitResult, urlsplit, urlunsplit


ALLOWED_SUFFIXES = (".edu.tw", ".gov.tw")


def _normalise_hostname(hostname: str) -> str:
    return hostname.rstrip(".").encode("idna").decode("ascii").lower()


def _allowed_hostname(hostname: str) -> bool:
    return any(hostname.endswith(suffix) and hostname != suffix[1:] for suffix in ALLOWED_SUFFIXES)


def _normalised_url(parts: SplitResult, hostname: str) -> str:
    netloc = hostname
    if parts.port is not None:
        netloc = f"{hostname}:{parts.port}"
    return urlunsplit((parts.scheme.lower(), netloc, parts.path or "/", parts.query, ""))


def validate(raw_url: str) -> dict[str, object]:
    result: dict[str, object] = {"input": raw_url, "accepted": False}
    try:
        parts = urlsplit(raw_url)
        if parts.scheme.lower() not in {"http", "https"}:
            result["reason"] = "scheme_must_be_http_or_https"
            return result
        if not parts.hostname:
            result["reason"] = "hostname_required"
            return result
        if parts.username is not None or parts.password is not None:
            result["reason"] = "userinfo_not_allowed"
            return result
        hostname = _normalise_hostname(parts.hostname)
        try:
            ipaddress.ip_address(hostname)
        except ValueError:
            pass
        else:
            result["reason"] = "ip_address_not_allowed"
            return result
        if parts.port not in (None, 80, 443):
            result["reason"] = "non_default_port_not_allowed"
            return result
        if not _allowed_hostname(hostname):
            result["reason"] = "hostname_not_under_edu_tw_or_gov_tw"
            return result
        result.update(
            {
                "accepted": True,
                "hostname": hostname,
                "normalized_url": _normalised_url(parts, hostname),
            }
        )
        return result
    except ValueError as exc:
        result["reason"] = f"invalid_url: {exc}"
        return result


def main() -> int:
    parser = argparse.ArgumentParser(description="Validate official .edu.tw/.gov.tw URLs")
    parser.add_argument("urls", nargs="+", help="URLs to validate")
    args = parser.parse_args()

    for raw_url in args.urls:
        print(json.dumps(validate(raw_url), ensure_ascii=False, sort_keys=True))
    return 0


if __name__ == "__main__":
    sys.exit(main())
