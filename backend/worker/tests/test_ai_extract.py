"""Unit tests for the Cloudflare Workers AI brochure extractor.

The Cloudflare call is stubbed with a fake runner; these tests only cover the
prompt/merge/mapping logic and the fallback contract.
"""
from __future__ import annotations

import json
import unittest

from worker.sta_worker.ai_extract import (
    AISettings,
    ai_extract_brochure,
    load_ai_settings,
)
from worker.sta_worker.contracts import BrochureExtractJob


def make_job() -> BrochureExtractJob:
    return BrochureExtractJob(
        job_id="00000000-0000-0000-0000-000000000001",
        academic_year=0,
        school_code="",
        storage_key="source.pdf",
        sha256_hex="a" * 64,
        requested_at="2026-08-14T00:00:00Z",
        upload_id="00000000-0000-0000-0000-000000000099",
        infer_identity=True,
    )


SETTINGS = AISettings(
    enabled=True,
    account_id="acct",
    api_token="token",
    model="@cf/meta/llama-3.3-70b-instruct-fp8-fast",
    base_url="https://example.invalid",
    timeout_seconds=30,
    max_input_chars=8000,
    max_output_tokens=2048,
    max_chunks=4,
    confidence=0.3,
)


class AIExtractTest(unittest.TestCase):
    def test_disabled_without_credentials_returns_none(self):
        job = make_job()
        result = ai_extract_brochure(
            job,
            ["some text"],
            settings=AISettings(
                enabled=True, account_id="", api_token="", model="m",
                base_url="x", timeout_seconds=1, max_input_chars=4000,
                max_output_tokens=2048, max_chunks=1, confidence=0.3,
            ),
        )
        self.assertIsNone(result)

    def test_maps_model_json_to_candidates(self):
        answer = {
            "academic_year": 116,
            "school_code": "-",
            "school_name": "國立示範大學",
            "programs": [
                {
                    "program_code": "QS116",
                    "admission_program_name": "資訊工程學系",
                    "admission_quota": 3,
                    "source_page": 7,
                    "consultation_phone": "(07)591-1234 分機 5",
                    "consultation_email": "csie@example.edu.tw",
                    "consultation_contact": "陳小姐",
                    "timeline_events": [
                        {"name": "招生簡章公告", "start_date": "115-09-01", "start_time": "-"},
                        {
                            "name": "網路報名",
                            "start_date": "115-09-30",
                            "start_time": "-",
                            "end_date": "115-10-13",
                            "end_time": "-",
                        },
                        {"name": "上網查詢報名狀態", "start_date": "-", "start_time": "-"},
                        {"name": "二階名單公布", "start_date": "115-11-20", "start_time": "-"},
                        {"name": "申請報名費退費截止", "start_date": "115-11-25", "start_time": "-"},
                        {"name": "錄取放榜", "start_date": "115-12-11", "start_time": "10:00"},
                    ],
                }
            ],
        }
        calls: list[list[dict[str, str]]] = []

        def fake_run(messages):
            calls.append(messages)
            return json.dumps(answer, ensure_ascii=False)

        identity, candidates = ai_extract_brochure(
            make_job(),
            ["cover page text", "department pages"],
            settings=SETTINGS,
            caller=fake_run,
        )

        self.assertEqual(identity.academic_year, 116)
        self.assertEqual(identity.school_name, "國立示範大學")
        self.assertEqual(len(candidates), 1)
        data = candidates[0].data
        self.assertEqual(candidates[0].program_code, "001")
        self.assertEqual(data["source_program_code"], "QS116")
        self.assertEqual(data["admission_program_name"], "資訊工程學系")
        self.assertEqual(data["admission_quota"], 3)
        self.assertEqual(data["consultation_phone"], "(07)591-1234 分機 5")
        self.assertEqual(data["consultation_email"], "csie@example.edu.tw")
        self.assertEqual(data["consultation_contact"], "陳小姐")
        self.assertNotIn("exam_items", data)
        self.assertNotIn("registration_start_date", data)
        self.assertNotIn("special_talent_target", data)
        # 招生簡章公告 (the brochure's own announcement date) now stays in the
        # timeline instead of being policy-excluded; 上網查詢報名狀態 (generic
        # application-status check) and 申請報名費退費截止 (fee refund deadline)
        # are still excluded; 二階名單公布 stays.
        self.assertEqual(
            [
                (e["name"], e["start_date"], e["start_time"], e["end_date"], e["end_time"], e["sort_order"])
                for e in data["timeline_events"]
            ],
            [
                ("招生簡章公告", "2026-09-01", "-", "-", "-", 1),
                ("網路報名", "2026-09-30", "-", "2026-10-13", "-", 2),
                ("二階名單公布", "2026-11-20", "-", "-", "-", 3),
                ("錄取放榜", "2026-12-11", "10:00", "-", "-", 4),
            ],
        )
        self.assertLess(candidates[0].confidence, 0.5)
        # System prompt is sent on every chunk.
        self.assertTrue(all(m[0]["role"] == "system" for m in calls))

    def test_unusable_answer_falls_back_to_none(self):
        identity_none = ai_extract_brochure(
            make_job(),
            ["text"],
            settings=SETTINGS,
            caller=lambda messages: "sorry, I cannot help with that",
        )
        self.assertIsNone(identity_none)

    def test_permanent_api_error_falls_back_to_none(self):
        from worker.sta_worker.ai_extract import _PermanentAIError

        def boom(messages):
            raise _PermanentAIError("HTTP 400: prompt too long")

        result = ai_extract_brochure(
            make_job(), ["text"], settings=SETTINGS, caller=boom
        )
        self.assertIsNone(result)

    def test_merges_programmes_across_chunks_by_name(self):
        pages = ["x" * 6000, "y" * 6000, "z" * 6000]

        def fake_run(messages):
            user = messages[1]["content"]
            if user.count("x") > 1000:
                return json.dumps({
                    "academic_year": 115,
                    "programs": [{"admission_program_name": "電機工程學系", "admission_quota": 2}],
                })
            if user.count("y") > 1000:
                return json.dumps({
                    "programs": [
                        {"admission_program_name": "電機工程學系", "consultation_email": "ee@example.edu.tw"},
                        {"admission_program_name": "機械工程學系", "admission_quota": 4},
                    ]
                })
            return json.dumps({"programs": []})

        identity, candidates = ai_extract_brochure(
            make_job(), pages, settings=SETTINGS, caller=fake_run
        )
        self.assertEqual(identity.academic_year, 115)
        names = sorted(c.data["admission_program_name"] for c in candidates)
        self.assertEqual(names, ["機械工程學系", "電機工程學系"])
        ee = next(c for c in candidates if c.data["admission_program_name"] == "電機工程學系")
        self.assertEqual(ee.data["admission_quota"], 2)
        self.assertEqual(ee.data["consultation_email"], "ee@example.edu.tw")

    def test_summarizes_nested_cloudflare_context_length_error(self):
        from worker.sta_worker.ai_extract import _summarize_cloudflare_error

        raw = (
            r'{"errors":[{"message":"AiError: AiError: {\"error\":{\"message\":'
            r'\"This model\'s maximum context length is 24000 tokens. However, '
            r'you requested 4096 output tokens and your prompt contains at '
            r'least 19905 input tokens, for a total of at least 24001 tokens. '
            r'Please reduce the length of the input prompt or the number of '
            r'requested output tokens. (parameter=input_tokens, value=19905)\",'
            r'\"type\":\"BadRequestError\",\"param\":\"input_tokens\",\"code\":400}} '
            r'(097d3112-3619-4571-b652-6690521234ab)","code":8}],'
            r'"success":false,"result":{},"messages":[]}'
        )
        summary = _summarize_cloudflare_error("Workers AI HTTP 400", raw)

        self.assertTrue(summary.startswith("Workers AI HTTP 400: "))
        self.assertIn("maximum context length is 24000 tokens", summary)
        # The doubled "AiError: AiError:" prefix, the escaped-JSON wrapper and
        # the trailing request id are gone.
        self.assertNotIn("AiError", summary)
        self.assertNotIn("097d3112", summary)
        self.assertNotIn('\\"', summary)
        self.assertLessEqual(len(summary), 180)

    def test_summarizes_daily_quota_error_without_request_id(self):
        from worker.sta_worker.ai_extract import _summarize_cloudflare_error

        raw = (
            '{"errors":[{"message":"AiError: AiError: you have used up your '
            "daily free allocation of 10,000 neurons, please upgrade to "
            "Cloudflare's Workers Paid plan if you would like to continue "
            'usage. (6dca3dce-48ef-41c9-8819-470ad67395dd)","code":4006}],'
            '"success":false,"result":{},"messages":[]}'
        )
        summary = _summarize_cloudflare_error("Workers AI HTTP 429", raw)

        self.assertTrue(
            summary.startswith(
                "Workers AI HTTP 429: you have used up your daily free "
                "allocation of 10,000 neurons"
            )
        )
        self.assertNotIn("6dca3dce", summary)
        self.assertLessEqual(len(summary), 170)

    def test_load_settings_reads_alias_token(self):
        import os

        os.environ["STA_WORKER_AI_ENABLED"] = "true"
        os.environ["STA_WORKER_AI_ACCOUNT_ID"] = "acct-1"
        os.environ.pop("STA_WORKER_AI_API_TOKEN", None)
        os.environ["STA_WORKER_AI_TOKEN"] = "tok-1"
        try:
            settings = load_ai_settings()
            self.assertTrue(settings.usable)
            self.assertEqual(settings.api_token, "tok-1")
        finally:
            for key in ("STA_WORKER_AI_ENABLED", "STA_WORKER_AI_ACCOUNT_ID", "STA_WORKER_AI_TOKEN"):
                os.environ.pop(key, None)


if __name__ == "__main__":
    unittest.main()
