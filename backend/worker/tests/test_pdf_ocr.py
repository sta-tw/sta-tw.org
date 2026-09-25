import os
from pathlib import Path
import sys
from types import SimpleNamespace
import unittest
from unittest.mock import patch

from worker.sta_worker.pdf_ocr import ocr_sparse_pages, page_needs_ocr


class PDFOCRTest(unittest.TestCase):
    def test_only_sparse_pages_need_ocr(self):
        self.assertTrue(page_needs_ocr(""))
        self.assertTrue(page_needs_ocr("校系"))
        self.assertFalse(
            page_needs_ocr(
                "這是一段足夠長的 PDF 文字內容，可以直接使用文字層，不需要啟用 OCR 來重新辨識這一頁。"
            )
        )

    def test_ocr_replaces_sparse_page_text(self):
        class FakeImage:
            def __init__(self):
                self.closed = False

            def close(self):
                self.closed = True

        image = FakeImage()
        calls = []

        def convert_from_path(path, **kwargs):
            calls.append((path, kwargs))
            return [image]

        fake_tesseract = SimpleNamespace(
            image_to_string=lambda image, **kwargs: "科系編號：013 機械工程系"
        )
        with patch.dict(
            os.environ,
            {
                "STA_WORKER_OCR_ENABLED": "true",
                "STA_WORKER_OCR_MIN_TEXT_CHARS": "40",
            },
            clear=False,
        ), patch.dict(
            sys.modules,
            {
                "pdf2image": SimpleNamespace(convert_from_path=convert_from_path),
                "pytesseract": fake_tesseract,
            },
        ):
            pages = ocr_sparse_pages(
                Path("source.pdf"),
                [
                    "",
                    "這是一段足夠長的文字內容，這一頁可以直接使用 PDF 內嵌文字，因此應該跳過 OCR。",
                ],
            )

        self.assertEqual(pages[0], "科系編號：013 機械工程系")
        self.assertEqual(len(calls), 1)
        self.assertEqual(calls[0][1]["first_page"], 1)
        self.assertTrue(image.closed)


if __name__ == "__main__":
    unittest.main()
