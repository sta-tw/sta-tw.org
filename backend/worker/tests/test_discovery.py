from __future__ import annotations

import json
import socket
import ssl
import unittest
from unittest.mock import patch

from worker.sta_worker.discovery import (
    DiscoveryError,
    NoCandidate,
    SearchResult,
    SearxngSearch,
    _candidate_links,
    _classify_local,
    _official_tls_context,
    _official_url,
)


PUBLIC_ADDRESS = [(socket.AF_INET, socket.SOCK_STREAM, 6, "", ("8.8.8.8", 443))]


class OfficialURLTest(unittest.TestCase):
    @patch("worker.sta_worker.discovery.socket.getaddrinfo", return_value=PUBLIC_ADDRESS)
    def test_accepts_education_and_government_domains(self, _):
        self.assertEqual(_official_url("https://admission.example.edu.tw/a.pdf"), "https://admission.example.edu.tw/a.pdf")
        self.assertEqual(_official_url("https://agency.gov.tw/a.pdf"), "https://agency.gov.tw/a.pdf")

    def test_rejects_non_official_domain(self):
        with self.assertRaises(DiscoveryError):
            _official_url("https://example.com/a.pdf")

    @patch("worker.sta_worker.discovery.socket.getaddrinfo", return_value=[(socket.AF_INET, socket.SOCK_STREAM, 6, "", ("127.0.0.1", 443))])
    def test_rejects_private_or_loopback_resolution(self, _):
        with self.assertRaises(DiscoveryError):
            _official_url("https://admission.example.edu.tw/a.pdf")

    def test_rejects_malformed_port(self):
        with self.assertRaises(DiscoveryError):
            _official_url("https://admission.example.edu.tw:not-a-port/a.pdf")

    @patch("worker.sta_worker.discovery.socket.getaddrinfo", return_value=PUBLIC_ADDRESS)
    def test_percent_encodes_non_ascii_path(self, _):
        got = _official_url("https://www.sggs.hc.edu.tw/news/國立清華大學特殊選才/")
        self.assertEqual(
            got,
            "https://www.sggs.hc.edu.tw/news/%E5%9C%8B%E7%AB%8B%E6%B8%85%E8%8F%AF%E5%A4%A7%E5%AD%B8%E7%89%B9%E6%AE%8A%E9%81%B8%E6%89%8D/",
        )
        got.encode("ascii")  # must not raise

    @patch("worker.sta_worker.discovery.socket.getaddrinfo", return_value=PUBLIC_ADDRESS)
    def test_does_not_double_encode(self, _):
        url = "https://admission.example.edu.tw/files/116%E7%B0%A1%E7%AB%A0.pdf"
        self.assertEqual(_official_url(url), url)


class CandidateLinksTest(unittest.TestCase):
    @patch("worker.sta_worker.discovery.socket.getaddrinfo", return_value=PUBLIC_ADDRESS)
    def test_ranks_target_year_special_selection_pdf(self, _):
        html = """
        <a href="/old/114.pdf">114 學年度特殊選才招生簡章</a>
        <a href="/files/116-special.pdf">116 學年度特殊選才招生簡章</a>
        <a href="/news.html">一般公告</a>
        """.encode()
        values = _candidate_links("https://admission.example.edu.tw/index.html", html, 116)
        self.assertEqual(values[0], "https://admission.example.edu.tw/files/116-special.pdf")
        self.assertNotIn("https://admission.example.edu.tw/news.html", values)


class LocalClassificationTest(unittest.TestCase):
    def test_accepts_exact_configured_year_and_school(self):
        confidence, evidence = _classify_local(
            "測試大學", 116, "https://a.edu.tw/116.pdf", "測試大學 116 學年度特殊選才招生簡章"
        )
        self.assertEqual(confidence, 0.9)
        self.assertEqual(evidence["year"], "116 學年度")

    def test_rejects_wrong_year_or_document_type(self):
        with self.assertRaises(NoCandidate):
            _classify_local("測試大學", 116, "https://a.edu.tw/115.pdf", "測試大學 115 學年度特殊選才招生簡章")


class OfficialTLSContextTest(unittest.TestCase):
    def test_keeps_verification_but_drops_strict(self):
        context = _official_tls_context()
        self.assertEqual(context.verify_mode, ssl.CERT_REQUIRED)
        self.assertTrue(context.check_hostname)
        strict = getattr(ssl, "VERIFY_X509_STRICT", 0)
        if strict:
            self.assertFalse(context.verify_flags & strict)

    def test_missing_ca_bundle_path_raises(self):
        with patch.dict("os.environ", {"STA_BROCHURE_DISCOVERY_CA_BUNDLE": "/no/such/bundle.pem"}):
            with self.assertRaises(OSError):
                _official_tls_context()


class SearxngSearchTest(unittest.TestCase):
    def test_requires_absolute_base_url(self):
        with self.assertRaises(RuntimeError):
            SearxngSearch("")
        with self.assertRaises(RuntimeError):
            SearxngSearch("   ")
        with self.assertRaises(RuntimeError):
            SearxngSearch("http://127.0.0.1:58888/search?format=json")

    @patch("worker.sta_worker.discovery.socket.getaddrinfo", return_value=PUBLIC_ADDRESS)
    @patch("worker.sta_worker.discovery.urllib.request.urlopen")
    def test_parses_results_and_drops_non_official(self, mock_urlopen, _):
        payload = json.dumps({
            "results": [
                {"url": "https://admission.example.edu.tw/116.pdf", "title": "116 特殊選才", "content": "招生簡章"},
                {"url": "https://example.com/116.pdf", "title": "非官方", "description": ""},
                {"title": "沒有網址"},
            ]
        }).encode()
        mock_urlopen.return_value.__enter__.return_value.read.return_value = payload
        results = SearxngSearch("http://127.0.0.1:58888", "google,bing", "zh-TW").search("測試大學", 116)
        self.assertEqual(len(results), 1)
        self.assertEqual(results[0].url, "https://admission.example.edu.tw/116.pdf")
        self.assertEqual(results[0].description, "招生簡章")
        request = mock_urlopen.call_args.args[0]
        query = request.full_url.split("?", 1)[1]
        self.assertIn("format=json", query)
        self.assertIn("language=zh-TW", query)
        self.assertIn("engines=google%2Cbing", query)

    @patch("worker.sta_worker.discovery.urllib.request.urlopen")
    def test_rejects_malformed_response(self, mock_urlopen):
        mock_urlopen.return_value.__enter__.return_value.read.return_value = b'{"results": {}}'
        with self.assertRaises(DiscoveryError):
            SearxngSearch("http://127.0.0.1:58888").search("測試大學", 116)


if __name__ == "__main__":
    unittest.main()
