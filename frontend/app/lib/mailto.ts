const ARCHIVE_BCC = "zengcode0315@gmail.com";

export type ContactMailContext = "general" | "brochure" | "account" | "report";

type ContactMailDetails = {
    academicYear?: number | string;
    schoolName?: string;
    programName?: string;
};

const recipients: Record<ContactMailContext, string> = {
    general: "contact@mail.sta-tw.org",
    brochure: "brochure@mail.sta-tw.org",
    account: "account@mail.sta-tw.org",
    report: "report@mail.sta-tw.org"
};

function mailContent(context: ContactMailContext, details: ContactMailDetails) {
    const programDescription = [
        details.academicYear ? `${details.academicYear} 學年度` : "",
        details.schoolName,
        details.programName
    ]
        .filter(Boolean)
        .join("／");

    switch (context) {
        case "brochure":
            return {
                subject: `簡章資料補充或更正${programDescription ? `｜${programDescription}` : ""}`,
                body: [
                    "您好，STA-tw 工作團隊：",
                    "",
                    "我想補充或回報簡章資料，資訊如下：",
                    `學年度／學校／科系：${programDescription}`,
                    "回報類型（缺少簡章／內容有誤／資料更新／無法預覽或下載）：",
                    "官方資料來源網址：",
                    "需要補充或更正的內容：",
                    "",
                    "我已附上相關檔案或資料來源，請協助確認。",
                    "",
                    "謝謝。"
                ].join("\n")
            };
        case "account":
            return {
                subject: "STA 帳號問題諮詢",
                body: [
                    "您好，STA-tw 工作團隊：",
                    "",
                    "我遇到帳號使用問題，想請協助確認：",
                    "帳號名稱（如適用）：",
                    "聯絡信箱：",
                    "問題類型（登入／帳號申請／其他）：",
                    "發生時間與操作步驟：",
                    "畫面顯示的錯誤訊息（如有）：",
                    "",
                    "為保護帳號安全，請勿在信件中提供密碼、驗證碼或設定密碼連結。",
                    "謝謝。"
                ].join("\n")
            };
        case "report":
            return {
                subject: "網站內容檢舉／申訴回報",
                body: [
                    "您好，STA-tw 工作團隊：",
                    "",
                    "我想回報網站內容，相關資訊如下：",
                    "問題類型（內容檢舉／著作權申訴／其他）：",
                    "相關頁面網址：",
                    "涉及的學校、科系或內容：",
                    "事實說明與希望處理方式：",
                    "相關來源或佐證資料：",
                    "聯絡姓名／信箱：",
                    "（著作權申訴請補充權利人身分及可供核對的權利證明。）",
                    "",
                    "謝謝。"
                ].join("\n")
            };
        case "general":
        default:
            return {
                subject: "S.T.A 特殊選才資源網｜問題諮詢",
                body: [
                    "您好，STA-tw 工作團隊：",
                    "",
                    "我想詢問網站或特殊選才相關問題：",
                    "問題類型：",
                    "相關頁面網址：",
                    "學校／科系（如適用）：",
                    "問題描述與已嘗試的方式：",
                    "希望協助事項：",
                    "聯絡姓名／信箱：",
                    "",
                    "謝謝。"
                ].join("\n")
            };
    }
}

export function contactMailtoHref(
    context: ContactMailContext,
    details: ContactMailDetails = {}
): string {
    const { subject, body } = mailContent(context, details);
    const encodedBody = encodeURIComponent(body.replace(/\r?\n/g, "\r\n"));
    const query = [
        `subject=${encodeURIComponent(subject)}`,
        `body=${encodedBody}`,
        `bcc=${encodeURIComponent(ARCHIVE_BCC)}`
    ].join("&");

    return `mailto:${recipients[context]}?${query}`;
}
