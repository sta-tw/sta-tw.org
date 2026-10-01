export type PolicyBlock =
    | { type: "paragraph"; text: string }
    | { type: "subheading"; text: string }
    | { type: "unordered-list"; items: string[] }
    | { type: "ordered-list"; items: string[] };

export type PolicySection = {
    heading: string;
    blocks: PolicyBlock[];
};

export type PolicyDocumentContent = {
    title: string;
    updatedAt: string;
    intro?: string[];
    sections: PolicySection[];
    closing?: string[];
};

export function policyDocumentToMarkdown(document: PolicyDocumentContent): string {
    const lines = ["# " + document.title, ""];

    for (const paragraph of document.intro ?? []) {
        lines.push(paragraph, "");
    }

    for (const section of document.sections) {
        lines.push("## " + section.heading, "");
        for (const block of section.blocks) {
            switch (block.type) {
                case "paragraph":
                    lines.push(block.text, "");
                    break;
                case "subheading":
                    lines.push("### " + block.text, "");
                    break;
                case "unordered-list":
                    lines.push(...block.items.map((item) => "- " + item), "");
                    break;
                case "ordered-list":
                    lines.push(...block.items.map((item, index) => index + 1 + ". " + item), "");
                    break;
            }
        }
    }

    for (const paragraph of document.closing ?? []) {
        lines.push(paragraph, "");
    }

    return lines.join("\n").trim() + "\n";
}

function cleanHeading(value: string): string {
    return value
        .replaceAll("\\.", ".")
        .replaceAll("\\*", "*")
        .replace(/\*\*(.*?)\*\*/g, "$1")
        .trim();
}

export function parsePolicyMarkdown(
    markdown: string,
    fallback: PolicyDocumentContent
): PolicyDocumentContent {
    const lines = markdown.replace(/\r\n?/g, "\n").split("\n");
    let title = fallback.title;
    const intro: string[] = [];
    const sections: PolicySection[] = [];
    let currentSection: PolicySection | null = null;
    let paragraphLines: string[] = [];
    let listType: "unordered-list" | "ordered-list" | null = null;
    let listItems: string[] = [];

    const flushList = () => {
        if (!listType || listItems.length === 0) {
            listType = null;
            listItems = [];
            return;
        }
        const block: PolicyBlock =
            listType === "ordered-list"
                ? { type: "ordered-list", items: listItems }
                : { type: "unordered-list", items: listItems };
        if (currentSection) {
            currentSection.blocks.push(block);
        }
        listType = null;
        listItems = [];
    };

    const flushParagraph = () => {
        const text = paragraphLines.join(" ").trim();
        paragraphLines = [];
        if (!text) return;
        if (currentSection) {
            currentSection.blocks.push({ type: "paragraph", text });
        } else {
            intro.push(text);
        }
    };

    const flushBlocks = () => {
        flushList();
        flushParagraph();
    };

    for (const rawLine of lines) {
        const line = rawLine.trim();
        if (!line) {
            flushBlocks();
            continue;
        }
        if (/^---+$/.test(line)) {
            flushBlocks();
            continue;
        }

        const sectionMatch = line.match(/^##\s+(.+)$/);
        if (sectionMatch) {
            flushBlocks();
            currentSection = { heading: cleanHeading(sectionMatch[1]), blocks: [] };
            sections.push(currentSection);
            continue;
        }

        const subheadingMatch = line.match(/^###\s+(.+)$/);
        if (subheadingMatch) {
            flushBlocks();
            if (currentSection) {
                currentSection.blocks.push({
                    type: "subheading",
                    text: cleanHeading(subheadingMatch[1])
                });
            } else {
                intro.push(cleanHeading(subheadingMatch[1]));
            }
            continue;
        }

        const titleMatch = line.match(/^#\s+(.+)$/);
        if (titleMatch) {
            flushBlocks();
            title = cleanHeading(titleMatch[1]);
            continue;
        }

        const unorderedMatch = line.match(/^[-*]\s+(.+)$/);
        if (unorderedMatch) {
            flushParagraph();
            if (listType !== "unordered-list") {
                flushList();
                listType = "unordered-list";
            }
            listItems.push(unorderedMatch[1]);
            continue;
        }

        const orderedMatch = line.match(/^\d+[.)]\s+(.+)$/);
        if (orderedMatch) {
            flushParagraph();
            if (listType !== "ordered-list") {
                flushList();
                listType = "ordered-list";
            }
            listItems.push(orderedMatch[1]);
            continue;
        }

        flushList();
        paragraphLines.push(line);
    }
    flushBlocks();

    // closing is deliberately cleared in both branches below: fallback.closing
    // is the *hardcoded* document's closing paragraph, but once markdown was
    // successfully parsed (even into just an intro blob), whatever closing
    // text the admin actually wrote is already captured inside intro/sections
    // — spreading ...fallback without clearing closing left it rendering a
    // second time underneath, since a plain object spread only overrides
    // keys this function explicitly sets afterward.
    if (sections.length === 0) {
        return {
            ...fallback,
            title,
            intro: markdown.trim() ? [markdown.trim()] : intro,
            sections: [],
            closing: []
        };
    }

    return { ...fallback, title, intro, sections, closing: [] };
}

export const serviceTermsPolicy: PolicyDocumentContent = {
    title: "服務條款",
    updatedAt: "2026-09-28",
    intro: [
        "歡迎您使用「S.T.A 特殊選才資源網」（以下簡稱「本平台」）。本服務條款（以下簡稱「本條款」）係規範本平台籌備團隊（未來於依法完成設立登記後，將由「社團法人 S.T.A 特殊選才協會」或相應名稱之社團法人概括承受，以下統稱「本團隊」）與使用本平台服務之使用者（以下簡稱「用戶」或「您」）之間的權利義務關係。",
        "在您完成會員註冊程序或開始使用本平台服務前，請務必詳細閱讀本條款之全部內容。當您完成註冊程序、登入或繼續使用本平台服務時，即視為您已充分閱讀、瞭解並同意接受本條款之所有內容。"
    ],
    sections: [
        {
            heading: "一、帳號註冊與身份驗證",
            blocks: [
                {
                    type: "ordered-list",
                    items: [
                        "註冊資格：本平台主要服務對象為學生、教師及大專院校行政人員。本平台用戶須具備有效的教育部認可機構信箱（以 @*.edu.tw 或相關官方教育網域為準）完成身分驗證，方得使用完整之會員功能（包括但不限於學生驗證專區、參與特定討論等）。",
                        "年齡說明：本平台無特定年齡限制，惟若您依中華民國法律為限制行為能力人或無行為能力人（如未滿 18 歲），您應於父母或法定代理人閱讀、瞭解並同意本條款後，始得註冊或使用本服務。",
                        "資料真實性與帳號安全：您於註冊及驗證（如上傳學生證、在學證明、入學證明等文件）時，應提供真實、正確且最新之個人資料。您應自行妥善保管帳號及密碼，不得將帳號借予、轉讓或出賣予任何第三人。任何以您的帳號登入後於本平台所進行之所有行為，均視為您本人之行為。"
                    ]
                }
            ]
        },
        {
            heading: "二、平台服務內容與資料免責聲明",
            blocks: [
                {
                    type: "ordered-list",
                    items: [
                        "服務範圍：本平台提供特殊選才相關簡章彙整、日程提醒設定、申請進度管理（含准考證號碼記錄）、心得分享、校系討論區、指南手冊及資訊訂閱（含 Email、Discord、Telegram、行事曆同步等）服務。",
                        "簡章與統計數據資料免責：本平台所整理之簡章時程、報名條件、面試筆試規範及放榜結果／就讀意願統計等資料，**僅供參考**。**正式招生簡章、日程與錄取標準請務必以各大學校院及官方招生管道發布之最新正式公告為準**。本團隊盡力維持資料之正確性與即時性，但不對任何資料之完整性、正確性或即時性作任何明示或默示之擔保。",
                        "系統中斷與維護免責：本團隊將盡商業上合理努力維持系統穩定運作。惟因系統維護、升級、電信線路故障、第三方服務（如通訊軟體 API、行事曆同步）異常、天災或不可抗力因素導致服務遲延、中斷、資料遺失或損壞者，**本團隊不負任何損害賠償責任**。用戶應自行妥善備份個人紀錄與資料。"
                    ]
                }
            ]
        },
        {
            heading: "三、用戶內容授權與著作權規範",
            blocks: [
                {
                    type: "ordered-list",
                    items: [
                        "用戶發布內容：您於本平台發表或傳送之文章、心得、討論回覆、評論及其他文字或檔案（以下簡稱「用戶內容」），其著作權仍歸屬於您或原權利人所有。",
                        "授權條款：當您將用戶內容發布至本平台時，即代表您同意無償、非專屬、永久、全世界授權本團隊重製、改作、編輯、公開傳輸、公開發表及利用該內容，並得將其運用於本平台之推廣、宣傳、成果報告及非營利教育推廣等用途。",
                        "文章與留言之保留：若您申請刪除帳號或註銷會員資格，為維護討論區及心得經驗分享之完整性與連貫性，**您過去於本平台發表之文章、留言及討論內容將會以「匿名」方式繼續保留於本網頁上**。",
                        "第三人內容免責：本平台討論區與心得分享係由用戶自主提供，本團隊不對用戶發表之言論或內容負擔保或背書責任，亦不承擔因用戶內容所生之任何法律責任。"
                    ]
                }
            ]
        },
        {
            heading: "四、用戶行為規範與違規處置機制",
            blocks: [
                { type: "paragraph", text: "禁止行為：您使用本服務時，不得從事下列行為：" },
                {
                    type: "unordered-list",
                    items: [
                        "違反中華民國現行法令、公共秩序或善良風俗之行為。",
                        "侵害他人之智慧財產權、隱私權、名譽權或其他合法權利。",
                        "上傳或散布虛偽不實、詐欺、誹謗、侮辱、猥褻、霸凌、歧視或具攻擊性之文字與檔案。",
                        "偽造或冒用他人身分、學生證件或教育信箱進行驗證。",
                        "使用自動化程式（如爬蟲、外掛）惡意擷取本平台資料，或散播病毒、癱瘓本平台系統。",
                        "未經本團隊事前書面同意，以任何方式（包括但不限於自動化程式、爬蟲技術或人工重製）擷取、下載、重製、改作、散布本平台之資料或資料庫內容，並將其用於任何形式之商業、營利或收益用途。"
                    ]
                },
                {
                    type: "paragraph",
                    text: "違規處置累計機制：若用戶違反本條款、平台管理規範或相關法規，本團隊將依以下機制進行處置："
                },
                {
                    type: "unordered-list",
                    items: [
                        "**第一次違規**：刪除違規內容（刪文），並發出警示。",
                        "**第二次違規**：暫時停權一週（7 日）。",
                        "**第三次違規**：永久封鎖帳號，並終止提供本平台任何服務。",
                        "如違規情節重大（如嚴重違法、侵權或干擾系統運作），本團隊有權不經前述漸進程序，直接採取永久封鎖措施。"
                    ]
                }
            ]
        },
        {
            heading: "五、檢舉、申訴與著作權侵權處理",
            blocks: [
                {
                    type: "ordered-list",
                    items: [
                        "檢舉及申訴管道：若您發現本平台上有任何違規內容、個人權利受損或著作權侵權疑慮，請透過官方聯絡信箱與我們聯繫。**檢舉／申訴信箱：report@mail.sta-tw.org**",
                        "處理流程：本團隊接獲檢舉後，得先將涉嫌侵權或違規之內容先行下架或隱藏，並進行審查或通知該內容提供者。經確認違規者，將依第四條規範處理。"
                    ]
                }
            ]
        },
        {
            heading: "六、營運模式、廣告與商業條款預留",
            blocks: [
                {
                    type: "ordered-list",
                    items: [
                        "營運性質：本平台目前由團隊成員及合作夥伴無酬宣傳與營運。",
                        "服務調整與費用預留：本團隊保留未來因應營運需求，加入贊助廣告、第三方合作活動或調整服務內容之權利。若未來推出付費功能或接受捐款，將另行訂立相應之服務條款或捐款須知，並以網站公告方式通知用戶。"
                    ]
                }
            ]
        },
        {
            heading: "七、組織變更與條款修訂",
            blocks: [
                {
                    type: "ordered-list",
                    items: [
                        "組織承受條款：本平台目前由籌備團隊進行營運。您同意未來於社團法人依法完成設立登記後，本團隊得將本條款下之權利義務、本平台營運權利及用戶資料（依個人資料保護法規定）整體移轉交由該社團法人概括承受，無須另行取得您的個別同意。",
                        "條款修訂：本團隊保留隨時修訂本條款之權利。修訂後之條款將公布於本平台網站上，不另行個別通知。若您於條款修訂後繼續使用本服務，即視為您已同意並接受修訂後之條款。"
                    ]
                }
            ]
        },
        {
            heading: "八、準據法與管轄法院",
            blocks: [
                {
                    type: "ordered-list",
                    items: [
                        "準據法：本條款之解釋、補充及適用，均以中華民國法律為準據法。",
                        "管轄法院：因本條款或使用本平台服務所生之任何爭議或訴訟，雙方同意專屬由臺灣台北地方法院為第一審管轄法院。"
                    ]
                }
            ]
        }
    ],
    closing: ["**最後修訂日期：2026 年 09 月 28 日**"]
};

export const privacyPolicy: PolicyDocumentContent = {
    title: "隱私權政策",
    updatedAt: "2026-09-28",
    intro: [
        "STA（以下稱「STA」、「本平台」或「我們」）重視使用者的隱私與個人資料保護。",
        "本隱私權政策說明當您造訪 **sta-tw.org**、建立或使用 STA 帳號，以及使用 STA 所提供之網站、社群、文件、驗證、客服及其他相關服務時，我們如何蒐集、處理、利用及保護您的資料。",
        "使用本平台即表示您已閱讀本隱私權政策。若特定功能另有個別的個人資料蒐集告知或說明，該個別說明將與本政策一併適用。"
    ],
    sections: [
        {
            heading: "一、適用範圍",
            blocks: [
                {
                    type: "paragraph",
                    text: "本隱私權政策適用於："
                },
                {
                    type: "unordered-list",
                    items: [
                        "sta-tw.org 及其由 STA 管理之子網域；",
                        "STA 所提供之網站與 API；",
                        "STA 帳號及身分驗證服務；",
                        "STA 社群、訊息、文件與備審資料相關功能；",
                        "客服、檢舉及平台管理功能；",
                        "與 STA 明確整合之第三方服務。"
                    ]
                },
                {
                    type: "paragraph",
                    text: "透過本平台連結至非由 STA 經營的網站或服務時，其資料處理方式應以該第三方的隱私權政策為準。"
                }
            ]
        },
        {
            heading: "二、我們可能蒐集的資料",
            blocks: [
                {
                    type: "paragraph",
                    text: "依您實際使用的功能不同，我們可能蒐集下列資料："
                },
                { type: "subheading", text: "1. 帳號及基本資料" },
                {
                    type: "paragraph",
                    text: "當您建立或使用 STA 帳號時，我們可能蒐集："
                },
                {
                    type: "unordered-list",
                    items: [
                        "使用者名稱；",
                        "顯示名稱；",
                        "電子郵件地址；",
                        "頭像；",
                        "個人簡介；",
                        "帳號角色、權限及驗證狀態；",
                        "您主動提供的其他個人資料。"
                    ]
                },
                {
                    type: "paragraph",
                    text: "若您以密碼建立帳號，我們亦會處理登入驗證所需的資訊。為維護帳號安全，密碼及其他驗證憑證會以適當的安全機制進行處理。"
                },
                { type: "subheading", text: "2. 第三方登入資料" },
                {
                    type: "paragraph",
                    text: "若您選擇使用 Discord 或其他第三方服務登入 STA，我們可能依您授權的範圍取得："
                },
                {
                    type: "unordered-list",
                    items: [
                        "第三方平台的帳號識別碼；",
                        "使用者名稱或顯示名稱；",
                        "頭像；",
                        "電子郵件地址；",
                        "第三方服務提供且為登入或帳號連結所必要的資訊。"
                    ]
                },
                {
                    type: "paragraph",
                    text: "STA 不會取得您的第三方服務密碼。"
                },
                { type: "subheading", text: "3. 學生及身分驗證資料" },
                {
                    type: "paragraph",
                    text: "部分 STA 功能可能依使用者身分提供不同權限。當您提出學生或其他身分驗證時，我們可能蒐集："
                },
                {
                    type: "unordered-list",
                    items: [
                        "學校或科系資訊；",
                        "學生身分相關資料；",
                        "您主動上傳的學生證或其他證明文件；",
                        "驗證申請時間、審核結果及必要的審核紀錄。"
                    ]
                },
                {
                    type: "paragraph",
                    text: "上述資料僅應用於驗證身分、提供對應權限、防止濫用及維護平台安全。"
                },
                {
                    type: "paragraph",
                    text: "請勿提供驗證所不必要的資料；如文件含有不需要提供的敏感資訊，建議在不影響驗證的情況下先行遮蔽。"
                },
                { type: "subheading", text: "4. 您發布或上傳的內容" },
                {
                    type: "paragraph",
                    text: "使用 STA 的社群或內容功能時，我們可能儲存："
                },
                {
                    type: "unordered-list",
                    items: [
                        "站內訊息及回覆；",
                        "Emoji 或其他互動紀錄；",
                        "上傳的圖片及檔案；",
                        "個人備審或經驗分享文件；",
                        "文件標題、描述及分類；",
                        "學校、科系及申請年度；",
                        "您主動提供的申請結果、名次、成績或其他相關資訊；",
                        "文件瀏覽、分享或互動紀錄。"
                    ]
                },
                {
                    type: "paragraph",
                    text: "依功能及權限設定，其中部分資料可能會顯示給其他 STA 使用者。"
                },
                {
                    type: "paragraph",
                    text: "請勿在公開內容中揭露不必要的個人資料，例如身分證字號、住址、私人電話、金融資訊或帳號密碼。"
                },
                {
                    type: "paragraph",
                    text: "若您上傳的內容包含其他人的個人資料，您應確認自己具有合法提供及使用該資料的權利。"
                },
                { type: "subheading", text: "5. 客服及聯絡資料" },
                {
                    type: "paragraph",
                    text: "當您向 STA 提出客服、檢舉或其他聯絡要求時，我們可能蒐集："
                },
                {
                    type: "unordered-list",
                    items: [
                        "您的 STA 帳號資訊；",
                        "聯絡方式；",
                        "Ticket 主旨、分類及內容；",
                        "您與客服人員的對話紀錄；",
                        "您主動提供的附件；",
                        "處理狀態及必要的內部紀錄。"
                    ]
                },
                { type: "subheading", text: "6. 技術及安全資訊" },
                {
                    type: "paragraph",
                    text: "當您連線至 STA 時，系統可能自動處理："
                },
                {
                    type: "unordered-list",
                    items: [
                        "IP 位址；",
                        "瀏覽器及 User-Agent 資訊；",
                        "裝置相關資訊；",
                        "登入及 Session 資訊；",
                        "存取時間；",
                        "請求及錯誤紀錄；",
                        "資安事件與異常行為紀錄。"
                    ]
                },
                {
                    type: "paragraph",
                    text: "這些資料主要用於提供服務、帳號安全、防止攻擊與濫用、故障排除及維護系統穩定性。"
                }
            ]
        },
        {
            heading: "三、資料蒐集及利用目的",
            blocks: [
                {
                    type: "ordered-list",
                    items: [
                        "建立、驗證及管理 STA 帳號；",
                        "提供登入及 Session 管理；",
                        "提供社群、訊息、文件及搜尋功能；",
                        "進行學生或其他資格驗證；",
                        "提供備審資料與招生資訊相關服務；",
                        "提供客服、檢舉及問題處理；",
                        "寄送帳號驗證、密碼重設、安全通知及必要的服務通知；",
                        "防止垃圾訊息、詐騙、濫用、未授權存取及其他資安威脅；",
                        "維護、除錯、分析及改善 STA 的可靠性與使用體驗；",
                        "執行社群規範及平台管理；",
                        "履行法律義務或配合合法的主管機關要求；",
                        "處理與本平台服務直接相關且具有合理關聯的其他事項。"
                    ]
                },
                {
                    type: "paragraph",
                    text: "我們原則上不會將蒐集的個人資料用於與原始蒐集目的無合理關聯的用途。"
                }
            ]
        },
        {
            heading: "四、Cookie、Session 與類似技術",
            blocks: [
                {
                    type: "paragraph",
                    text: "STA 可能使用 Cookie 或其他本機儲存機制，以維持："
                },
                {
                    type: "unordered-list",
                    items: [
                        "登入狀態；",
                        "Refresh Token 或 Session；",
                        "安全性驗證；",
                        "網站必要設定；",
                        "防止跨站請求、機器人或其他惡意行為。"
                    ]
                },
                {
                    type: "paragraph",
                    text: "部分 Cookie 對登入及平台核心功能而言屬必要項目。若您在瀏覽器中停用必要 Cookie，部分功能可能無法正常使用。"
                },
                {
                    type: "paragraph",
                    text: "若未來 STA 導入非必要的分析、追蹤或廣告 Cookie，我們將依實際情況更新本政策並提供適當說明或控制方式。"
                }
            ]
        },
        {
            heading: "五、第三方服務",
            blocks: [
                {
                    type: "paragraph",
                    text: "為提供及保護 STA 的服務，我們可能使用或整合第三方服務，例如："
                },
                {
                    type: "unordered-list",
                    items: [
                        "網路、DNS、CDN、流量防護及機器人驗證服務；",
                        "Discord 等第三方登入服務；",
                        "電子郵件及通知系統；",
                        "物件儲存及基礎設施服務；",
                        "系統監控及錯誤診斷工具。"
                    ]
                },
                {
                    type: "paragraph",
                    text: "例如，在使用 Cloudflare 等網路或安全性服務時，相關服務商可能處理 IP 位址、HTTP 請求及其他必要的網路資訊。"
                },
                {
                    type: "paragraph",
                    text: "當您主動使用 Discord OAuth 等第三方功能時，相關第三方亦可能依其自身隱私權政策處理您的資料。"
                },
                {
                    type: "paragraph",
                    text: "若 STA 的特定功能明確提供將訊息、客服內容或其他資料同步至 Discord、Telegram 或其他外部平台的功能，相關資料可能依該功能之設定傳送至第三方服務；未啟用相關整合時則不適用。"
                }
            ]
        },
        {
            heading: "六、資料提供及揭露",
            blocks: [
                {
                    type: "paragraph",
                    text: "除以下情況外，STA 不會任意出售或出租您的個人資料："
                },
                {
                    type: "unordered-list",
                    items: [
                        "為提供您要求的服務所必要；",
                        "經您同意或由您主動公開；",
                        "提供予協助 STA 營運服務且有必要存取資料的服務提供者；",
                        "為調查安全事件、詐騙、濫用或違反平台規範之行為；",
                        "為保護 STA、其他使用者或第三人的合法權益；",
                        "依法令要求、法院命令或其他合法政府要求所必要。"
                    ]
                },
                {
                    type: "paragraph",
                    text: "若您主動將內容發布於可供其他使用者存取的區域，該內容將依您發布時的權限與平台設計提供給其他使用者。"
                }
            ]
        },
        {
            heading: "七、資料利用的期間及地區",
            blocks: [
                {
                    type: "paragraph",
                    text: "您的個人資料原則上會保留至："
                },
                {
                    type: "unordered-list",
                    items: [
                        "蒐集目的完成；",
                        "您刪除相關內容；",
                        "您刪除帳號；",
                        "您依法要求停止處理或刪除，且 STA 無其他依法保留之必要；",
                        "或依法令、資安、爭議處理及其他合理必要期間屆滿為止。"
                    ]
                },
                {
                    type: "paragraph",
                    text: "系統備份中的資料可能在正常備份輪替週期內暫時存在，並於後續備份更新時逐步移除。"
                },
                {
                    type: "paragraph",
                    text: "資料原則上由 STA 所使用的伺服器及基礎設施處理。若使用全球性的網路、安全性或第三方整合服務，部分必要資料可能在服務提供者設有基礎設施的其他國家或地區進行處理。"
                }
            ]
        },
        {
            heading: "八、資料安全",
            blocks: [
                {
                    type: "paragraph",
                    text: "STA 將依資料性質及風險採取合理的技術與管理措施保護資料，例如："
                },
                {
                    type: "unordered-list",
                    items: [
                        "加密傳輸；",
                        "身分驗證與權限控管；",
                        "Session 與 Token 管理；",
                        "管理員權限限制；",
                        "系統與安全性紀錄；",
                        "檔案安全檢查；",
                        "備份及服務監控；",
                        "對敏感資料採取適當的保護措施。"
                    ]
                },
                {
                    type: "paragraph",
                    text: "然而，任何透過網際網路提供的系統均無法保證絕對安全。"
                },
                {
                    type: "paragraph",
                    text: "若我們發現可能對使用者權益造成影響的個人資料安全事件，將依事件性質及適用法令採取必要措施。"
                }
            ]
        },
        {
            heading: "九、您的權利",
            blocks: [
                {
                    type: "paragraph",
                    text: "依適用的個人資料保護法令，您可就您的個人資料提出下列要求："
                },
                {
                    type: "unordered-list",
                    items: [
                        "查詢或請求閱覽；",
                        "請求提供複製本；",
                        "請求補充或更正；",
                        "請求停止蒐集、處理或利用；",
                        "請求刪除。"
                    ]
                },
                {
                    type: "paragraph",
                    text: "部分資料可直接透過 STA 帳號設定或相關功能自行修改或刪除。"
                },
                {
                    type: "paragraph",
                    text: "其他個人資料相關要求，可透過 **STA 站內客服／Ticket 系統**提出。"
                },
                {
                    type: "paragraph",
                    text: "為防止未經授權的人存取或刪除他人資料，我們可能在處理要求前進行必要的身分確認。"
                },
                {
                    type: "paragraph",
                    text: "如依法有保存義務、為處理爭議、防止詐騙或維護系統安全所必要，部分刪除或停止處理要求可能受到法律允許範圍內的限制。"
                }
            ]
        },
        {
            heading: "十、不提供資料的影響",
            blocks: [
                {
                    type: "paragraph",
                    text: "您可以選擇不提供非必要的個人資料。"
                },
                {
                    type: "paragraph",
                    text: "但若您拒絕提供某項功能運作所必要的資料，例如："
                },
                {
                    type: "unordered-list",
                    items: [
                        "不提供 Email，可能無法建立或驗證帳號；",
                        "不提供身分證明資料，可能無法取得須經驗證的學生或其他身分權限；",
                        "停用必要 Cookie，可能無法登入或維持登入狀態；",
                        "不提供客服案件所需資訊，可能使我們無法有效處理您的問題。"
                    ]
                },
                {
                    type: "paragraph",
                    text: "除此之外，拒絕提供非必要資料不應影響其他無關功能的使用。"
                }
            ]
        },
        {
            heading: "十一、未成年人",
            blocks: [
                {
                    type: "paragraph",
                    text: "STA 的部分服務可能由學生及未成年人使用。"
                },
                {
                    type: "paragraph",
                    text: "若依適用法律，特定個人資料處理行為需要法定代理人同意，我們可能要求提供適當的同意或驗證。"
                },
                {
                    type: "paragraph",
                    text: "我們鼓勵未成年使用者避免在公開區域揭露不必要的個人資料。"
                }
            ]
        },
        {
            heading: "十二、政策變更",
            blocks: [
                {
                    type: "paragraph",
                    text: "STA 可能因服務功能、技術架構或法令要求變更本隱私權政策。"
                },
                {
                    type: "paragraph",
                    text: "如有更新，我們將於本頁公布新版內容並更新「最後更新日期」。若變更可能對您的權益產生重大影響，我們可能透過網站公告、站內通知、Email 或其他合理方式通知您。"
                }
            ]
        },
        {
            heading: "十三、聯絡我們",
            blocks: [
                {
                    type: "paragraph",
                    text: "如您對本隱私權政策、個人資料處理方式或希望行使個人資料相關權利有任何問題，請透過："
                },
                {
                    type: "paragraph",
                    text: "**STA 站內客服／Ticket 系統、官方電子郵件：contact@mail.sta-tw.org**"
                },
                {
                    type: "paragraph",
                    text: "與我們聯絡。"
                }
            ]
        }
    ],
    closing: [
        "**網站：** [https://sta-tw.org](https://sta-tw.org)",
        "**官方電子郵件：contact@mail.sta-tw.org**",
        "**服務提供者：** STA",
        "**最後更新：** 2026 年 9 月 28 日"
    ]
};
