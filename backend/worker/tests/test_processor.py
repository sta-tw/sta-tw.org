import hashlib
import json
from pathlib import Path
import tempfile
import unittest

from worker.sta_worker.contracts import BrochureExtractJob, InvalidJob
from worker.sta_worker.processor import safe_document_path


class WorkerContractTest(unittest.TestCase):
    def test_rejects_parent_traversal(self):
        with self.assertRaises(InvalidJob):
            BrochureExtractJob.from_mapping({
                "job_id": "job-1",
                "academic_year": 116,
                "school_code": "001",
                "storage_key": "../secret.pdf",
                "sha256_hex": "0" * 64,
                "requested_at": "2026-08-14T00:00:00Z",
            })

    def test_safe_path_stays_in_root(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            path = safe_document_path(root, "brochures/116/001.pdf")
            self.assertEqual(path.parent, (root / "brochures/116").resolve())


if __name__ == "__main__":
    unittest.main()
