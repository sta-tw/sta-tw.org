import { currentRocYear, rocYearOptions } from "../../lib/format";
import { identityStatusLabels } from "../../lib/api/profile";
import type {
    VerificationDocumentType,
    VerificationIdentityType
} from "../../lib/api/verification";

export const identityTypeOptions: { value: VerificationIdentityType; label: string }[] = (
    ["student", "senior"] as const
).map((value) => ({ value, label: identityStatusLabels[value] }));

export const documentTypeOptions: { value: VerificationDocumentType; label: string }[] = [
    { value: "student_id", label: "學生證" },
    { value: "admission_letter", label: "錄取通知" },
    { value: "other", label: "其他" }
];

export const academicFields = [
    "資訊學群",
    "工程學群",
    "數理化學群",
    "醫藥衛生學群",
    "生命科學學群",
    "生物資源學群",
    "地球與環境學群",
    "建築與設計學群",
    "藝術學群",
    "社會與心理學群",
    "大眾傳播學群",
    "外語學群",
    "文史哲學群",
    "教育學群",
    "法政學群",
    "管理學群",
    "財經學群",
    "遊憩與運動學群",
    "跨域／不分系"
] as const;

/** ROC academic years a document can be submitted for: next year's
 * admissions cycle, the current one and the previous one. */
export function academicYearOptions(now = new Date()) {
    const rocYear = currentRocYear(now);
    return rocYearOptions({ from: rocYear - 1, to: rocYear + 1, suffix: " 學年度" });
}
