"""Autonomous brochure discovery worker.

The worker searches the public web, follows only official Taiwanese education
or government URLs, validates the PDF content with deterministic rules, and
submits the file to the Go control plane. It never writes PostgreSQL or object
storage directly. Document classification is intentionally local and
deterministic; external search services are only used to return public URLs.
"""

from __future__ import annotations

from dataclasses import dataclass
from html.parser import HTMLParser
from io import BytesIO
import ipaddress
import json
import logging
import mimetypes
import os
import socket
import ssl
import urllib.error
import urllib.parse
import urllib.request
from typing import Any

from .runtime import configure_logging, install_shutdown


LOGGER = logging.getLogger("sta-brochure-discovery")
SEARXNG_SEARCH_PATH = "/search"
MAX_SEARCH_RESPONSE_BYTES = 2 * 1024 * 1024
MAX_HTML_BYTES = 2 * 1024 * 1024
MAX_PDF_BYTES = 50 * 1024 * 1024
MAX_PDF_PAGES = 2000
CLASSIFICATION_PAGES = 20


class DiscoveryError(RuntimeError):
    pass


class NoCandidate(DiscoveryError):
    pass


@dataclass(frozen=True)
class SearchResult:
    title: str
    url: str
    description: str = ""


@dataclass(frozen=True)
class DownloadedPDF:
    source_url: str
    document_url: str
    filename: str
    body: bytes
    confidence: float
    evidence: dict[str, Any]


def _duration(name: str, default: float) -> float:
    raw = os.environ.get(name, "").strip()
    if not raw:
        return default
    multiplier = 1.0
    if raw[-1:].lower() in {"s", "m", "h"}:
        unit = raw[-1:].lower()
        raw = raw[:-1]
        multiplier = {"s": 1.0, "m": 60.0, "h": 3600.0}[unit]
    try:
        value = float(raw) * multiplier
    except ValueError as exc:
        raise RuntimeError(f"{name} is not a valid duration") from exc
    if value <= 0:
        raise RuntimeError(f"{name} must be positive")
    return value


def _official_url(raw: str) -> str:
    value = raw.strip()
    parsed = urllib.parse.urlsplit(value)
    if parsed.scheme not in {"http", "https"} or not parsed.hostname or parsed.username or parsed.password:
        raise DiscoveryError("URL is not an absolute public HTTP(S) URL")
    hostname = parsed.hostname.rstrip(".").lower().encode("idna").decode("ascii")
    if not (hostname.endswith(".edu.tw") or hostname.endswith(".gov.tw")):
        raise DiscoveryError("URL is outside the official domain boundary")
    try:
        port = parsed.port
    except ValueError as exc:
        raise DiscoveryError("URL port is invalid") from exc
    if port not in {None, 80, 443}:
        raise DiscoveryError("URL uses a disallowed port")
    _require_public_addresses(hostname, port or (443 if parsed.scheme == "https" else 80))
    # Official school pages often carry Chinese characters in the path or query.
    # Percent-encode them (leaving existing %XX untouched) so urllib can issue
    # the request instead of raising UnicodeEncodeError, and normalise the host.
    netloc = hostname if port is None else f"{hostname}:{port}"
    path = urllib.parse.quote(parsed.path, safe="/%:@-._~!$&'()*+,;=~")
    query = urllib.parse.quote(parsed.query, safe="/%:@-._~!$'()*+,;=&~")
    return urllib.parse.urlunsplit((parsed.scheme, netloc, path, query, ""))


def _require_public_addresses(hostname: str, port: int) -> None:
    try:
        addresses = socket.getaddrinfo(hostname, port, type=socket.SOCK_STREAM)
    except OSError as exc:
        raise DiscoveryError("official hostname could not be resolved") from exc
    if not addresses:
        raise DiscoveryError("official hostname has no address")
    for item in addresses:
        address = ipaddress.ip_address(item[4][0])
        if not address.is_global:
            raise DiscoveryError("official hostname resolved to a non-public address")


class _SafeRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        _official_url(newurl)
        return super().redirect_request(req, fp, code, msg, headers, newurl)


def _official_tls_context() -> ssl.SSLContext:
    """TLS context for fetching official .edu.tw / .gov.tw hosts.

    Certificate validation (chain + hostname) stays on. Python 3.13 turned on
    VERIFY_X509_STRICT by default, which rejects the technically non-compliant
    certificates (e.g. missing Subject Key Identifier) still served by many
    Taiwanese university sites; that strict flag is cleared here. The domain
    suffix check and the public-address SSRF guard remain the trust boundary.
    STA_BROCHURE_DISCOVERY_CA_BUNDLE, if set, adds intermediate CAs that the
    system trust store is missing.
    """
    context = ssl.create_default_context()
    context.verify_flags &= ~getattr(ssl, "VERIFY_X509_STRICT", 0)
    bundle = os.environ.get("STA_BROCHURE_DISCOVERY_CA_BUNDLE", "").strip()
    if bundle:
        context.load_verify_locations(cafile=bundle)
    return context


class SafeFetcher:
    def __init__(self, timeout: float = 30.0) -> None:
        self.timeout = timeout
        self.opener = urllib.request.build_opener(
            _SafeRedirect(),
            urllib.request.HTTPSHandler(context=_official_tls_context()),
        )

    def fetch(self, url: str, limit: int) -> tuple[str, str, bytes]:
        url = _official_url(url)
        request = urllib.request.Request(url, headers={"Accept": "application/pdf,text/html;q=0.9", "User-Agent": "STA-Brochure-Discovery/1.0"})
        try:
            with self.opener.open(request, timeout=self.timeout) as response:
                final_url = _official_url(response.geturl())
                content_type = response.headers.get_content_type().lower()
                effective_limit = min(limit, MAX_HTML_BYTES) if content_type == "text/html" else limit
                body = response.read(effective_limit + 1)
        except (urllib.error.URLError, TimeoutError, OSError) as exc:
            raise DiscoveryError("official document request failed") from exc
        if len(body) > effective_limit:
            raise DiscoveryError("official document exceeds the size limit")
        return final_url, content_type, body


class SearxngSearch:
    """Query a configured SearXNG instance.

    Only result metadata is consumed, and every result URL is still forced
    through the ``.edu.tw`` / ``.gov.tw`` boundary before it can become a
    candidate. SearXNG is intentionally configured outside this worker so the
    worker does not need a commercial search API key.
    """

    def __init__(self, base_url: str, engines: str = "", language: str = "zh-TW", timeout: float = 20.0) -> None:
        value = base_url.strip().rstrip("/")
        parsed = urllib.parse.urlsplit(value)
        if (
            parsed.scheme not in {"http", "https"}
            or not parsed.hostname
            or parsed.username
            or parsed.password
            or parsed.query
            or parsed.fragment
        ):
            raise RuntimeError("STA_SEARXNG_URL must be an absolute HTTP(S) URL without credentials, query, or fragment")
        try:
            parsed.port
        except ValueError as exc:
            raise RuntimeError("STA_SEARXNG_URL port is invalid") from exc
        if timeout <= 0:
            raise RuntimeError("SearXNG timeout must be positive")
        if not language.strip():
            raise RuntimeError("STA_SEARXNG_LANGUAGE must not be empty")
        self.base_url = value
        self.engines = ",".join(part.strip() for part in engines.split(",") if part.strip())
        self.language = language.strip()
        self.timeout = timeout

    def search(self, school_name: str, academic_year: int) -> list[SearchResult]:
        query = f'"{school_name}" "{academic_year} 學年度" "特殊選才" 招生簡章 PDF'
        parsed = urllib.parse.urlsplit(self.base_url)
        path = parsed.path.rstrip("/") + SEARXNG_SEARCH_PATH
        endpoint = urllib.parse.urlunsplit((parsed.scheme, parsed.netloc, path, "", ""))
        params = {"q": query, "format": "json", "language": self.language}
        if self.engines:
            params["engines"] = self.engines
        request = urllib.request.Request(endpoint + "?" + urllib.parse.urlencode(params), headers={
            "Accept": "application/json", "User-Agent": "STA-Brochure-Discovery/1.0",
        })
        try:
            with urllib.request.urlopen(request, timeout=self.timeout) as response:
                body = response.read(MAX_SEARCH_RESPONSE_BYTES + 1)
        except (urllib.error.URLError, TimeoutError, OSError) as exc:
            raise DiscoveryError("search provider request failed") from exc
        if len(body) > MAX_SEARCH_RESPONSE_BYTES:
            raise DiscoveryError("search provider response is too large")
        try:
            values = json.loads(body.decode("utf-8"))["results"]
        except (ValueError, KeyError, TypeError) as exc:
            raise DiscoveryError("search provider response is invalid") from exc
        if not isinstance(values, list):
            raise DiscoveryError("search provider response is invalid")
        results: list[SearchResult] = []
        seen: set[str] = set()
        for value in values:
            if not isinstance(value, dict) or not isinstance(value.get("url"), str):
                continue
            try:
                url = _official_url(value["url"])
            except DiscoveryError:
                continue
            if url in seen:
                continue
            seen.add(url)
            description = value.get("content", value.get("description", ""))
            results.append(SearchResult(str(value.get("title", "")), url, str(description)))
        return results


class _LinkParser(HTMLParser):
    def __init__(self) -> None:
        super().__init__()
        self.links: list[tuple[str, str]] = []
        self._href = ""
        self._text: list[str] = []

    def handle_starttag(self, tag: str, attrs) -> None:
        if tag.lower() == "a":
            self._href = dict(attrs).get("href", "")
            self._text = []

    def handle_data(self, data: str) -> None:
        if self._href:
            self._text.append(data)

    def handle_endtag(self, tag: str) -> None:
        if tag.lower() == "a" and self._href:
            self.links.append((self._href, " ".join(self._text)))
            self._href = ""
            self._text = []


def _candidate_links(page_url: str, html: bytes, academic_year: int) -> list[str]:
    parser = _LinkParser()
    try:
        parser.feed(html.decode("utf-8", "replace"))
    except Exception:
        return []
    ranked: list[tuple[int, str]] = []
    seen: set[str] = set()
    for href, text in parser.links:
        url = urllib.parse.urljoin(page_url, href)
        try:
            url = _official_url(url)
        except DiscoveryError:
            continue
        if url in seen:
            continue
        seen.add(url)
        haystack = urllib.parse.unquote(url) + " " + text
        score = 0
        score += 8 if str(academic_year) in haystack else 0
        score += 6 if "特殊選才" in haystack else 0
        score += 4 if "簡章" in haystack else 0
        score += 3 if urllib.parse.urlsplit(url).path.lower().endswith(".pdf") else 0
        if score >= 7:
            ranked.append((score, url))
    ranked.sort(reverse=True)
    return [url for _, url in ranked[:12]]


def _pdf_text(body: bytes) -> str:
    try:
        from pypdf import PdfReader

        reader = PdfReader(BytesIO(body), strict=False)
        if len(reader.pages) > MAX_PDF_PAGES:
            raise DiscoveryError("candidate PDF has too many pages")
        pages = [(page.extract_text() or "") for page in reader.pages[:CLASSIFICATION_PAGES]]
    except DiscoveryError:
        raise
    except ImportError as exc:
        raise DiscoveryError("pypdf is required for brochure discovery") from exc
    except Exception as exc:
        raise DiscoveryError("candidate PDF could not be parsed") from exc
    return "\n\n".join(pages)


def _compact_match(value: str) -> str:
    return "".join(character for character in value.casefold() if character.isalnum() or "\u4e00" <= character <= "\u9fff")


def _classify_local(school_name: str, academic_year: int, url: str, text: str) -> tuple[float, dict[str, Any]]:
    """Conservatively verify a brochure without a model or remote API.

    A candidate must expose the requested year, the special-selection
    admission wording, and the school name in either the extracted PDF text or
    the official URL. Missing text is rejected so a random binary PDF cannot
    be accepted solely because its filename looks plausible.
    """
    compact_text = _compact_match(text)
    compact_url = _compact_match(urllib.parse.unquote(url))
    year_token = str(academic_year)
    type_match = "特殊選才" in text and ("簡章" in text or "招生" in text)
    year_match = year_token in text
    school_compact = _compact_match(school_name)
    school_match = bool(school_compact) and (school_compact in compact_text or school_compact in compact_url)
    if not year_match or not type_match or not school_match:
        raise NoCandidate("local rules rejected the candidate")
    evidence = {
        "year": f"{academic_year} 學年度",
        "type": "特殊選才" if "特殊選才" in text else "",
        "school": school_name if school_compact in compact_text else "official URL match",
    }
    confidence = 0.9 if school_compact in compact_text else 0.75
    return confidence, evidence


class ControlPlaneClient:
    def __init__(self, base_url: str, token: str, timeout: float = 60.0) -> None:
        self.base_url = base_url.rstrip("/")
        self.token = token.strip()
        self.timeout = timeout
        if not self.base_url.startswith(("http://", "https://")) or len(self.token) < 32:
            raise RuntimeError("discovery control-plane URL or token is invalid")

    def _json(self, path: str, payload: dict | None = None) -> dict:
        body = None if payload is None else json.dumps(payload).encode("utf-8")
        request = urllib.request.Request(self.base_url + path, data=body, method="POST", headers={
            "Authorization": f"Bearer {self.token}", "Content-Type": "application/json",
        })
        try:
            with urllib.request.urlopen(request, timeout=self.timeout) as response:
                return json.loads(response.read(2 * 1024 * 1024).decode("utf-8"))
        except urllib.error.HTTPError as exc:
            if exc.code == 404:
                raise NoCandidate("no discovery task is ready") from exc
            raise DiscoveryError(f"control plane returned HTTP {exc.code}") from exc
        except (urllib.error.URLError, TimeoutError, ValueError, OSError) as exc:
            raise DiscoveryError("control plane request failed") from exc

    def claim(self) -> dict:
        return self._json("/api/v1/internal/admissions/brochure-discovery/claim")["data"]

    def no_match(self, year: int, code: str) -> None:
        self._json(f"/api/v1/internal/admissions/brochure-discovery/cycles/{year}/tasks/{code}/no-match", {})

    def failure(self, year: int, code: str, error: Exception) -> None:
        self._json(f"/api/v1/internal/admissions/brochure-discovery/cycles/{year}/tasks/{code}/failure", {
            "code": "discovery_failed", "message": str(error)[:2000] or "discovery failed",
        })

    def candidate(self, year: int, code: str, candidate: DownloadedPDF) -> None:
        boundary = "sta-discovery-boundary-7b8f68df"
        fields = {
            "detected_academic_year": str(year), "source_url": candidate.source_url,
            "document_url": candidate.document_url, "confidence": str(candidate.confidence),
            "evidence": json.dumps(candidate.evidence, ensure_ascii=False),
        }
        chunks: list[bytes] = []
        for name, value in fields.items():
            chunks.append(f"--{boundary}\r\nContent-Disposition: form-data; name=\"{name}\"\r\n\r\n{value}\r\n".encode("utf-8"))
        filename = candidate.filename.replace('"', "") or f"{year}-{code}.pdf"
        chunks.append(f"--{boundary}\r\nContent-Disposition: form-data; name=\"file\"; filename=\"{filename}\"\r\nContent-Type: application/pdf\r\n\r\n".encode("utf-8"))
        chunks.extend([candidate.body, f"\r\n--{boundary}--\r\n".encode("ascii")])
        request = urllib.request.Request(
            self.base_url + f"/api/v1/internal/admissions/brochure-discovery/cycles/{year}/tasks/{code}/candidate",
            data=b"".join(chunks), method="POST", headers={
                "Authorization": f"Bearer {self.token}", "Content-Type": f"multipart/form-data; boundary={boundary}",
            },
        )
        try:
            with urllib.request.urlopen(request, timeout=self.timeout) as response:
                response.read(2 * 1024 * 1024)
        except urllib.error.HTTPError as exc:
            raise DiscoveryError(f"candidate upload returned HTTP {exc.code}") from exc
        except (urllib.error.URLError, TimeoutError, OSError) as exc:
            raise DiscoveryError("candidate upload failed") from exc


def discover(search: SearxngSearch, fetcher: SafeFetcher, task: dict) -> DownloadedPDF:
    year = int(task["academic_year"])
    school_name = str(task["school_name"])
    results = search.search(school_name, year)
    attempted: set[str] = set()
    for result in results:
        urls = [result.url]
        source_url = result.url
        try:
            final_url, content_type, body = fetcher.fetch(result.url, MAX_PDF_BYTES)
        except DiscoveryError:
            continue
        if content_type == "text/html" or not body.startswith(b"%PDF-"):
            if len(body) <= MAX_HTML_BYTES:
                urls = _candidate_links(final_url, body, year)
            else:
                urls = []
        for url in urls:
            if len(attempted) >= 20:
                break
            if url in attempted:
                continue
            attempted.add(url)
            try:
                document_url, _, pdf = fetcher.fetch(url, MAX_PDF_BYTES)
                if not pdf.startswith(b"%PDF-"):
                    continue
                confidence, evidence = _classify_local(school_name, year, document_url, _pdf_text(pdf))
            except (DiscoveryError, NoCandidate):
                continue
            filename = os.path.basename(urllib.parse.urlsplit(document_url).path) or f"{year}-{task['school_code']}.pdf"
            if not filename.lower().endswith(".pdf"):
                filename += ".pdf"
            evidence = {**evidence, "search_title": result.title, "search_description": result.description}
            return DownloadedPDF(source_url, document_url, filename[:255], pdf, confidence, evidence)
    raise NoCandidate("no verified brochure candidate was found")


def run() -> None:
    control = ControlPlaneClient(
        os.environ.get("STA_BROCHURE_DISCOVERY_API_BASE_URL", "http://localhost:8080"),
        os.environ.get("STA_BROCHURE_DISCOVERY_AGENT_TOKEN", ""),
    )
    search = SearxngSearch(
        os.environ.get("STA_SEARXNG_URL", ""),
        os.environ.get("STA_SEARXNG_ENGINES", ""),
        os.environ.get("STA_SEARXNG_LANGUAGE", "zh-TW"),
    )
    fetcher = SafeFetcher(_duration("STA_BROCHURE_DISCOVERY_FETCH_TIMEOUT", 30.0))
    poll_interval = _duration("STA_BROCHURE_DISCOVERY_POLL_INTERVAL", 30.0)
    stop = install_shutdown()
    LOGGER.info("local brochure discovery worker started")
    while not stop.is_set():
        try:
            task = control.claim()
        except NoCandidate:
            stop.wait(poll_interval)
            continue
        try:
            candidate = discover(search, fetcher, task)
            control.candidate(int(task["academic_year"]), str(task["school_code"]), candidate)
            LOGGER.info("candidate submitted", extra={"academic_year": task["academic_year"], "school_code": task["school_code"]})
        except NoCandidate:
            control.no_match(int(task["academic_year"]), str(task["school_code"]))
        except Exception as exc:
            LOGGER.exception("brochure discovery task failed")
            try:
                control.failure(int(task["academic_year"]), str(task["school_code"]), exc)
            except Exception:
                LOGGER.exception("could not report discovery failure")
    LOGGER.info("local brochure discovery worker stopped")


if __name__ == "__main__":
    configure_logging()
    run()
