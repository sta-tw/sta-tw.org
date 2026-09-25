"""Golden tests for the format-agnostic brochure extractor.

These run the local extractor over `pdftotext -layout` dumps of four real
special-selection admission brochures, each with a different section layout
(see ``fixtures/brochures/manifest.json``). The point is to keep the extractor
honest across formats: a change that overfits one school's wording will drop
programs from the others and fail here.
"""
from __future__ import annotations

import json
from pathlib import Path
import unittest

from worker.sta_worker.brochure_local import (
    extract_local_candidates,
    infer_brochure_identity,
)
from worker.sta_worker.contracts import BrochureExtractJob

FIXTURES = Path(__file__).parent / "fixtures" / "brochures"


def _upload_job() -> BrochureExtractJob:
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


def _run(fixture_file: str):
    pages = (FIXTURES / fixture_file).read_text(encoding="utf-8").split("\f")
    job = _upload_job()
    identity = infer_brochure_identity(job, pages)
    candidates = extract_local_candidates(
        job,
        pages,
        academic_year=identity.academic_year or 116,
        school_code=identity.school_code,
        school_name=identity.school_name,
    )
    return identity, candidates


class BrochureCorpusTest(unittest.TestCase):
    def test_manifest_matches_extractor_output(self):
        manifest = json.loads((FIXTURES / "manifest.json").read_text(encoding="utf-8"))
        self.assertTrue(manifest["fixtures"], "manifest lists no fixtures")

        for spec in manifest["fixtures"]:
            with self.subTest(fixture=spec["file"]):
                identity, candidates = _run(spec["file"])

                self.assertIn(
                    spec["school_name_contains"],
                    identity.school_name,
                    f"{spec['file']}: school name {identity.school_name!r}",
                )

                names = [candidate.data["admission_program_name"] for candidate in candidates]
                by_name = {
                    candidate.data["admission_program_name"]: candidate
                    for candidate in candidates
                }

                # No two programs collapse onto the same identity, and none of
                # the names are sentence fragments from surrounding prose.
                self.assertEqual(len(names), len(set(names)), f"duplicate program rows: {names}")
                for name in names:
                    self.assertNotIn("。", name)
                    self.assertNotIn("【", name)
                    self.assertLessEqual(len(name), 40, name)

                if "program_count" in spec:
                    self.assertEqual(
                        len(candidates),
                        spec["program_count"],
                        f"{spec['file']}: {names}",
                    )
                if "program_count_min" in spec:
                    self.assertGreaterEqual(
                        len(candidates),
                        spec["program_count_min"],
                        f"{spec['file']}: only {len(candidates)} programs",
                    )

                for expected in spec["programs"]:
                    self.assertIn(expected["name"], by_name, f"{spec['file']}: missing {expected['name']!r}")
                    candidate = by_name[expected["name"]]
                    self.assertEqual(
                        candidate.data["admission_quota"],
                        expected["quota"],
                        f"{spec['file']}: {expected['name']} quota",
                    )
                    self.assertRegex(candidate.program_code, r"^\d{3}$")
                    if "printed_code" in expected:
                        self.assertEqual(
                            candidate.data["source_program_code"],
                            expected["printed_code"],
                            f"{spec['file']}: {expected['name']} printed code",
                        )

    def test_every_fixture_yields_quota_for_most_programs(self):
        manifest = json.loads((FIXTURES / "manifest.json").read_text(encoding="utf-8"))
        for spec in manifest["fixtures"]:
            with self.subTest(fixture=spec["file"]):
                _, candidates = _run(spec["file"])
                self.assertTrue(candidates)
                with_quota = sum(
                    1 for c in candidates if c.data["admission_quota"] is not None
                )
                self.assertGreaterEqual(
                    with_quota,
                    max(1, int(len(candidates) * 0.9)),
                    f"{spec['file']}: only {with_quota}/{len(candidates)} programs got a quota",
                )


if __name__ == "__main__":
    unittest.main()
