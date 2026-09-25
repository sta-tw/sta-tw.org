import json
from pathlib import Path
import tempfile
import unittest

from worker.sta_worker.brochure_local import (
    _clean_exam_description,
    _clean_exam_item_name,
    _join_wrapped_lines,
    extract_local_candidates,
    infer_brochure_identity,
)
from worker.sta_worker.candidate_list import extract_candidate_list
from worker.sta_worker.contracts import BrochureExtractJob


def make_job(source_type="brochure", program_code=""):
    return BrochureExtractJob(
        job_id="00000000-0000-0000-0000-000000000001",
        academic_year=115,
        school_code="001",
        storage_key="source.pdf" if source_type == "brochure" else "source.json",
        sha256_hex="a" * 64,
        requested_at="2026-08-14T00:00:00Z",
        source_type=source_type,
        program_code=program_code,
    )


class BrochureLocalExtractionTest(unittest.TestCase):
    def test_infers_upload_only_identity_from_cover(self):
        job = BrochureExtractJob(
            job_id="00000000-0000-0000-0000-000000000001",
            academic_year=0,
            school_code="",
            storage_key="source.pdf",
            sha256_hex="a" * 64,
            requested_at="2026-08-14T00:00:00Z",
            upload_id="00000000-0000-0000-0000-000000000099",
            infer_identity=True,
        )
        identity = infer_brochure_identity(
            job,
            ["國立示範大學 115學年度特殊選才招生簡章\n學校代碼：001\n學校名稱：國立示範大學"],
        )
        self.assertEqual(identity.academic_year, 115)
        self.assertEqual(identity.school_code, "001")
        self.assertEqual(identity.school_name, "國立示範大學")

        committee_identity = infer_brochure_identity(
            job,
            [
                "116 學年度學士班特殊選才招生簡章\n"
                "編印單位：國立體育大學招生委員會\n"
                "聯絡電話：（03）3283201 轉 1593"
            ],
        )
        self.assertEqual(committee_identity.academic_year, 116)
        self.assertEqual(committee_identity.school_name, "國立體育大學")

    def test_extracts_program_quota_dates_and_exam_evidence(self):
        pages = [
            "國立示範大學 115學年度特殊選才招生簡章",
            "科系編號：013 機械工程系　招生名額2名\n"
            "報名暨繳費：115年5月1日上午9時至5月8日下午5時\n"
            "面試：115年6月1日  資料審查 50%",
        ]
        candidates = extract_local_candidates(make_job(), pages)
        self.assertEqual(len(candidates), 1)
        data = candidates[0].data
        self.assertEqual(data["admission_program_name"], "機械工程系 一般組")
        self.assertEqual(data["admission_quota"], 2)
        timeline_by_name = {event["name"]: event for event in data["timeline_events"]}
        self.assertEqual(timeline_by_name["網路報名"]["start_date"], "2026-05-01")
        self.assertEqual(timeline_by_name["網路報名"]["end_date"], "2026-05-08")
        self.assertEqual(timeline_by_name["甄試"]["start_date"], "2026-06-01")
        self.assertEqual(timeline_by_name["甄試"]["end_date"], "-")
        self.assertEqual(data["exam_items"][0]["name"], "面試")

    def test_assigns_internal_codes_to_program_headings_in_document_order(self):
        pages = [
            "國立臺南大學 116 學年度學士班特殊選才招生簡章",
            "招生學系 國語文學系 一般生\n招生名額 2",
            "招生學系 國語文學系\n(願景計劃組)\n願景計畫\n招生名額 7",
            "招生學系 電機工程學系 一般生\n招生名額 2",
            "招生學系 電機工程學系\n(願景計畫組)\n招生名額 2",
            "招生學系 行政管理學系 一般生\n招生名額 3",
        ]

        candidates = extract_local_candidates(make_job(), pages)

        self.assertEqual(
            [candidate.program_code for candidate in candidates],
            ["001", "002", "003", "004", "005"],
        )
        self.assertEqual(
            [candidate.data["admission_program_name"] for candidate in candidates],
            [
                "國語文學系 一般組",
                "國語文學系 願景計畫組",
                "電機工程學系 一般組",
                "電機工程學系 願景計畫組",
                "行政管理學系 一般組",
            ],
        )
        self.assertEqual(
            [candidate.data["admission_quota"] for candidate in candidates],
            [2, 7, 2, 2, 3],
        )

    def test_scopes_contact_source_and_exam_items_to_each_program_block(self):
        pages = [
            "招生學系 國語文學系 一般生\n"
            "招生名額 2\n"
            "不得攜帶行動電話、平板電腦\n"
            "考試項目及計分比例\n"
            "1 術科-書法(含傳統書法) 40%\n"
            "2 資料審查(含書面審查) 60%\n"
            "學系網址 https://chinese.example.edu.tw/\n"
            "聯絡方式 電話：(06)2133111 分機 621 E-mail：chinese@example.edu.tw",
            "招生學系 電機工程學系 一般生\n"
            "招生名額 2\n"
            "考試項目及計分比例\n"
            "1 資料審查 50%\n"
            "2 面試 50%\n"
            "學系網址 https://ee.example.edu.tw/\n"
            "聯絡方式 電話：(06)2602315 E-mail：ee@example.edu.tw",
        ]

        candidates = extract_local_candidates(make_job(), pages)

        self.assertEqual(candidates[0].data["consultation_phone"], "(06)2133111 分機 621")
        self.assertEqual(candidates[0].data["brochure_url"], "https://chinese.example.edu.tw/")
        self.assertEqual(
            [(item["name"], item["weight_percent"]) for item in candidates[0].data["exam_items"]],
            [("術科-書法", 40.0), ("資料審查", 60.0)],
        )
        self.assertEqual(
            [item["description"] for item in candidates[0].data["exam_items"]],
            ["術科-書法(含傳統書法) 40%", "資料審查(含書面審查) 60%"],
        )
        self.assertEqual(_clean_exam_item_name("面試，含術科測驗"), "面試")
        self.assertEqual(
            _clean_exam_description("1.第一項 2.第二項 3.第三項"),
            "第一項 第二項 第三項",
        )
        self.assertEqual(candidates[1].data["consultation_phone"], "(06)2602315")
        self.assertEqual(candidates[1].data["brochure_url"], "https://ee.example.edu.tw/")
        self.assertEqual(
            [(item["name"], item["weight_percent"]) for item in candidates[1].data["exam_items"]],
            [("資料審查", 50.0), ("面試", 50.0)],
        )

    def test_extracts_bare_grouped_department_layout_and_split_schedule(self):
        pages = [
            "116 學年度學士班特殊選才招生簡章\n"
            "編印單位：國立體育大學招生委員會\n"
            "重要日程\n"
            "簡章公告 115.9.1 (二)\n"
            "網路報名填表\n"
            "115.9.21（一）上午9時\n"
            "〜115.11.3（二）下午2時\n"
            "複審 115.11.21(六)\n"
            "放榜（公告錄取名單）\n"
            "寄發成績通知單 115.12.1(二)下午2時",
            "捌、招生名額、考試項目及計分方式：\n"
            "適應體育學系\n"
            "招生名額 4 名\n"
            "考試項目及計分\n"
            "初審\n"
            "書面資料審查：\n"
            "40%\n"
            "複審 面試 60%\n"
            "總成績同分\n"
            "學系網址 https://ape.ntsu.edu.tw/\n"
            "連絡資訊 連絡人：行政秘書\n"
            "電 話：(03)328-3201 轉 8522",
            "體育推廣學系\n"
            "組別 女子啦啦舞組 拔河組\n"
            "招生名額\n"
            "7 名 2 名\n"
            "考試項目及計分\n"
            "初審\n"
            "40%\n"
            "書面資料審查：\n"
            "運動參賽成績證明（30%）\n"
            "書面資料審查：\n"
            "運動參賽成績證明（50%）\n"
            "複審\n"
            "60%\n"
            "面試(含術科測驗)：\n"
            "面試：\n"
            "專項運動經歷與成就分享（40%）\n"
            "總成績同分\n"
            "學系網址 https://dsp.ntsu.edu.tw\n"
            "連絡資訊\n"
            "電 話：(03)328-3201 轉 8515",
        ]

        candidates = extract_local_candidates(
            BrochureExtractJob(
                job_id="00000000-0000-0000-0000-000000000001",
                academic_year=0,
                school_code="",
                storage_key="source.pdf",
                sha256_hex="a" * 64,
                requested_at="2026-08-14T00:00:00Z",
                upload_id="00000000-0000-0000-0000-000000000099",
                infer_identity=True,
            ),
            pages,
            academic_year=116,
            school_code="",
            school_name="國立體育大學",
        )

        self.assertEqual(
            [candidate.data["admission_program_name"] for candidate in candidates],
            [
                "適應體育學系 一般組",
                "體育推廣學系 女子啦啦舞組",
                "體育推廣學系 拔河組",
            ],
        )
        self.assertEqual(
            [candidate.data["admission_quota"] for candidate in candidates],
            [4, 7, 2],
        )
        timeline_by_name = {
            event["name"]: event for event in candidates[0].data["timeline_events"]
        }
        self.assertEqual(timeline_by_name["網路報名"]["end_date"], "2026-11-03")
        self.assertEqual(timeline_by_name["甄試"]["start_date"], "2026-11-21")
        self.assertEqual(timeline_by_name["錄取放榜"]["start_date"], "2026-12-01")
        self.assertEqual(candidates[1].data["consultation_phone"], "(03)328-3201 轉 8515")
        self.assertEqual(candidates[1].data["brochure_url"], "https://dsp.ntsu.edu.tw")
        self.assertEqual(
            [(item["name"], item["weight_percent"]) for item in candidates[2].data["exam_items"]],
            [("書面資料審查", 40.0), ("面試", 60.0)],
        )
        self.assertTrue(
            all(item["weight_percent"] is not None for item in candidates[2].data["exam_items"])
        )
        self.assertIn(
            "運動參賽成績證明(50%)",
            candidates[2].data["exam_items"][0]["description"],
        )
        self.assertNotIn(
            "(",
            " ".join(item["name"] for item in candidates[1].data["exam_items"]),
        )
        self.assertEqual(
            _join_wrapped_lines([
                "面試，含專業認知、作品說明、模擬情",
                "境題等。",
            ]),
            ["面試,含專業認知、作品說明、模擬情境題等。"],
        )

    def test_keeps_stage_total_weight_and_merges_nested_details_generically(self):
        pages = [
            "招生學系 綜合能力學系\n"
            "招生名額 3 名\n"
            "考試項目及計分方式\n"
            "第一階段\n"
            "書面審查\n"
            "40%\n"
            "1.學習歷程（30%）\n"
            "2.作品資料（70%）\n"
            "第二階段\n"
            "面試（含專業問答）\n"
            "60%\n"
            "1.專業能力與學習動機（100%）\n"
            "總成績同分參酌順序 1.面試 2.書面審查",
        ]

        candidates = extract_local_candidates(make_job(), pages)

        self.assertEqual(len(candidates), 1)
        items = candidates[0].data["exam_items"]
        self.assertEqual(
            [(item["name"], item["weight_percent"]) for item in items],
            [("書面審查", 40.0), ("面試", 60.0)],
        )
        self.assertIn("學習歷程(30%)", items[0]["description"])
        self.assertIn("專業能力與學習動機(100%)", items[1]["description"])
        self.assertNotRegex(" ".join(item["description"] for item in items), r"(?:^|\s)[123][.)、．.]\s*")

    def test_supports_inline_stage_labels_and_alternate_exam_header(self):
        pages = [
            "招生學系 商業管理系\n"
            "招生名額 2 名\n"
            "考試科目與配分\n"
            "初試：資料審查（50%）\n"
            "複試：口試 50%\n"
            "學系網址 https://business.example.edu.tw/",
        ]

        candidates = extract_local_candidates(make_job(), pages)

        self.assertEqual(
            [(item["name"], item["weight_percent"]) for item in candidates[0].data["exam_items"]],
            [("資料審查", 50.0), ("口試", 50.0)],
        )

    def test_ignores_toc_and_reference_mentions_before_real_program_pages(self):
        pages = [
            "學系招生資訊（一般生）\n"
            "1. 社會教育學系 6 13\n"
            "2. 國文學系 2 17",
            "符合學系所定之報考資格附加規定\n"
            "（一）社會教育學系\n"
            "曾參加全國性競賽者",
            "學系招生資訊（一般生）\n"
            "1. 社會教育學系\n"
            "招生名額 6 名\n"
            "考試項目及成績計算\n"
            "初試 資料審查 50%\n"
            "複試 口試 50%",
            "2. 國文學系\n"
            "招生名額 2 名\n"
            "考試項目及成績計算\n"
            "資料審查 100%",
        ]

        candidates = extract_local_candidates(make_job(), pages)

        self.assertEqual(
            [candidate.data["admission_program_name"] for candidate in candidates],
            ["社會教育學系 一般組", "國文學系 一般組"],
        )
        self.assertEqual(
            [candidate.data["admission_quota"] for candidate in candidates],
            [6, 2],
        )

    def test_parses_explicit_program_row_with_group_and_inline_quota(self):
        pages = [
            "拾、招生學系、名額、考試科目及規定\n"
            "一、語文與創作學系文學創作組\n"
            "招生學系 語文與創作學系 文學創作組 招 生 名 額 4 名\n"
            "考 試 項 目\n"
            "資料審查 60%\n"
            "面試 40%",
        ]

        candidates = extract_local_candidates(make_job(), pages)

        self.assertEqual(len(candidates), 1)
        self.assertEqual(
            candidates[0].data["admission_program_name"],
            "語文與創作學系 文學創作組",
        )
        self.assertEqual(candidates[0].data["admission_quota"], 4)

    def test_attaches_designated_materials_and_repeated_stage_details(self):
        pages = [
            "招生學系 運動傳播學系\n"
            "招生名額 2 名\n"
            "指定繳交書面資料\n"
            "1.代表作品\n"
            "2.運動傳播經歷與適性\n"
            "考試項目及計分方式\n"
            "初試\n"
            "書面資料審查 40%\n"
            "複試 面試 60%\n"
            "初試（資料審查）\n"
            "【繳交資料】\n"
            "3.社群行銷成果\n"
            "複試（面試）\n"
            "【考試方式】\n"
            "專業認知與作品說明\n"
            "同分參酌順序",
        ]

        candidates = extract_local_candidates(make_job(), pages)
        items = candidates[0].data["exam_items"]

        self.assertEqual(
            [(item["name"], item["weight_percent"]) for item in items],
            [("書面資料審查", 40.0), ("面試", 60.0)],
        )
        self.assertIn("代表作品", items[0]["description"])
        self.assertIn("運動傳播經歷與適性", items[0]["description"])
        self.assertIn("社群行銷成果", items[0]["description"])
        self.assertIn("專業認知與作品說明", items[1]["description"])

    def test_keeps_subgroup_distinct_inside_population_group(self):
        pages = [
            "學系招生資訊（新住民生－外加名額）\n"
            "26. 幼兒與家庭科學學系家庭生活與教育組 新住民組\n"
            "招生名額 1 名\n"
            "考試項目\n"
            "資料審查 100%",
            "27. 幼兒與家庭科學學系幼兒發展與教育組 新住民組\n"
            "招生名額 1 名\n"
            "考試項目\n"
            "資料審查 100%",
        ]

        candidates = extract_local_candidates(make_job(), pages)

        self.assertEqual(
            [candidate.data["admission_program_name"] for candidate in candidates],
            [
                "幼兒與家庭科學學系 家庭生活與教育組 新住民組",
                "幼兒與家庭科學學系 幼兒發展與教育組 新住民組",
            ],
        )

    def test_parses_chinese_numbered_rows_and_infers_combined_stage_name(self):
        pages = [
            "招生學系 公民教育學系\n"
            "招生名額 2 名\n"
            "考試項目及成績計算\n"
            "一\n"
            "成績證明、自傳與讀書計畫 75%\n"
            "二 其他有利審查資料 25%\n"
            "同分參酌順序",
            "招生學系 數學系\n"
            "招生名額 3 名\n"
            "考試項目及成績計算\n"
            "初試\n"
            "資料審查 40%\n"
            "複試 通過上午筆試者方可參加下午口\n"
            "試 60%\n"
            "同分參酌順序",
        ]

        candidates = extract_local_candidates(make_job(), pages)

        self.assertEqual(
            [item["weight_percent"] for item in candidates[0].data["exam_items"]],
            [75.0, 25.0],
        )
        self.assertEqual(
            [(item["name"], item["weight_percent"]) for item in candidates[1].data["exam_items"]],
            [("資料審查", 40.0), ("筆試及口試", 60.0)],
        )


class CandidateListExtractionTest(unittest.TestCase):
    def test_extracts_and_masks_json_rows(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "source.json"
            path.write_text(json.dumps([{
                "准考證號": "AB-123456",
                "姓名": "王小明",
                "系所代碼": "013",
                "錄取結果": "正取",
                "名次": 1,
            }], ensure_ascii=False), encoding="utf-8")
            result = extract_candidate_list(make_job("candidate_list"), path, "local-extraction-v1", 1 << 20)
        self.assertEqual(result["result_type"], "candidate_list")
        self.assertEqual(result["rows"][0]["candidate_number"], "AB-123456")
        self.assertEqual(result["rows"][0]["masked_name"], "王○○")
        self.assertEqual(result["rows"][0]["result_status"], "admitted")
        self.assertEqual(result["rows"][0]["official_rank"], 1)


if __name__ == "__main__":
    unittest.main()
