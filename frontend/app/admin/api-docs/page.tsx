"use client";

import { useState } from "react";
import styles from "./api-docs.module.css";

export default function ApiDocs() {
  const [activeSection, setActiveSection] = useState("auth");

  const sections = [
    { id: "auth", title: "認證 API", icon: "🔐" },
    { id: "users", title: "用戶 API", icon: "👤" },
    { id: "channels", title: "頻道 API", icon: "💬" },
    { id: "messages", title: "訊息 API", icon: "📝" },
    { id: "search", title: "搜尋 API", icon: "🔍" },
    { id: "portfolio", title: "備審資料 API", icon: "📄" },
    { id: "verification", title: "驗證 API", icon: "✅" },
    { id: "tickets", title: "客服票 API", icon: "🎫" },
    { id: "admissions", title: "招生文件 API", icon: "📚" },
    { id: "admin", title: "管理 API", icon: "⚙️" },
    { id: "websocket", title: "WebSocket", icon: "🔌" },
  ];

  return (
    <div className={styles.container}>
      <aside className={styles.sidebar}>
        <div className={styles.sidebarHeader}>
          <h2>STA API 文件</h2>
          <p className={styles.version}>v1.0</p>
        </div>
        <nav className={styles.nav}>
          {sections.map((section) => (
            <button
              key={section.id}
              className={`${styles.navItem} ${
                activeSection === section.id ? styles.active : ""
              }`}
              onClick={() => setActiveSection(section.id)}
            >
              <span className={styles.icon}>{section.icon}</span>
              {section.title}
            </button>
          ))}
        </nav>
      </aside>

      <main className={styles.main}>
        <div className={styles.content}>
          <div className={styles.header}>
            <h1>API 使用說明書</h1>
            <div className={styles.baseUrl}>
              <span className={styles.label}>Base URL:</span>
              <code>http://localhost:12004/api/v1</code>
            </div>
          </div>

          {activeSection === "auth" && (
            <section className={styles.section}>
              <h2>🔐 認證 API</h2>

              <div className={styles.endpoint}>
                <div className={styles.endpointHeader}>
                  <span className={styles.method}>POST</span>
                  <code>/auth/register</code>
                </div>
                <p className={styles.description}>註冊新用戶</p>
                <div className={styles.codeBlock}>
                  <pre>{`{
  "username": "johndoe",
  "email": "john@example.com",
  "displayName": "John Doe",
  "password": "mypassword123"
}`}</pre>
                </div>
              </div>

              <div className={styles.endpoint}>
                <div className={styles.endpointHeader}>
                  <span className={styles.method}>POST</span>
                  <code>/auth/login</code>
                </div>
                <p className={styles.description}>用戶登入</p>
                <div className={styles.codeBlock}>
                  <pre>{`{
  "email": "john@example.com",
  "password": "mypassword123"
}`}</pre>
                </div>
              </div>

              <div className={styles.endpoint}>
                <div className={styles.endpointHeader}>
                  <span className={styles.method}>POST</span>
                  <code>/auth/logout</code>
                </div>
                <p className={styles.description}>登出（需要 Bearer Token）</p>
              </div>

              <div className={styles.endpoint}>
                <div className={styles.endpointHeader}>
                  <span className={styles.method}>POST</span>
                  <code>/auth/refresh</code>
                </div>
                <p className={styles.description}>更新 Access Token</p>
              </div>

              <div className={styles.endpoint}>
                <div className={styles.endpointHeader}>
                  <span className={styles.method}>GET</span>
                  <code>/auth/me</code>
                </div>
                <p className={styles.description}>取得當前用戶資料</p>
              </div>
            </section>
          )}

          {activeSection === "users" && (
            <section className={styles.section}>
              <h2>👤 用戶 API</h2>

              <div className={styles.endpoint}>
                <div className={styles.endpointHeader}>
                  <span className={styles.method}>GET</span>
                  <code>/users/me</code>
                </div>
                <p className={styles.description}>取得自己的完整資料</p>
              </div>

              <div className={styles.endpoint}>
                <div className={styles.endpointHeader}>
                  <span className={styles.method}>PATCH</span>
                  <code>/users/me</code>
                </div>
                <p className={styles.description}>更新個人資料</p>
                <div className={styles.codeBlock}>
                  <pre>{`{
  "displayName": "新名字",
  "bio": "新介紹",
  "avatarUrl": "https://..."
}`}</pre>
                </div>
              </div>

              <div className={styles.endpoint}>
                <div className={styles.endpointHeader}>
                  <span className={styles.method}>GET</span>
                  <code>/users/me/avatar-upload-url</code>
                </div>
                <p className={styles.description}>取得頭像上傳預簽名 URL</p>
              </div>

              <div className={styles.endpoint}>
                <div className={styles.endpointHeader}>
                  <span className={styles.method}>GET</span>
                  <code>/users/:username</code>
                </div>
                <p className={styles.description}>取得他人公開資料</p>
              </div>
            </section>
          )}

          {activeSection === "channels" && (
            <section className={styles.section}>
              <h2>💬 頻道 API</h2>

              <div className={styles.endpoint}>
                <div className={styles.endpointHeader}>
                  <span className={styles.method}>GET</span>
                  <code>/channels</code>
                </div>
                <p className={styles.description}>列出所有頻道</p>
              </div>

              <div className={styles.endpoint}>
                <div className={styles.endpointHeader}>
                  <span className={styles.method}>GET</span>
                  <code>/channels/:channel_id</code>
                </div>
                <p className={styles.description}>取得頻道詳情</p>
              </div>

              <div className={styles.endpoint}>
                <div className={styles.endpointHeader}>
                  <span className={styles.method}>GET</span>
                  <code>/channels/:channel_id/messages</code>
                </div>
                <p className={styles.description}>取得訊息列表（分頁）</p>
                <div className={styles.params}>
                  <p><strong>Query Parameters:</strong></p>
                  <ul>
                    <li><code>limit</code> - 每頁數量（預設 50，最大 100）</li>
                    <li><code>before</code> - 游標：取得此訊息 ID 之前的訊息</li>
                  </ul>
                </div>
              </div>

              <div className={styles.endpoint}>
                <div className={styles.endpointHeader}>
                  <span className={styles.method}>POST</span>
                  <code>/channels/:channel_id/messages</code>
                </div>
                <p className={styles.description}>發送訊息</p>
                <div className={styles.codeBlock}>
                  <pre>{`{
  "content": "訊息內容",
  "replyToId": "<message_id 或 null>"
}`}</pre>
                </div>
              </div>
            </section>
          )}

          {activeSection === "messages" && (
            <section className={styles.section}>
              <h2>📝 訊息 API</h2>

              <div className={styles.endpoint}>
                <div className={styles.endpointHeader}>
                  <span className={styles.method}>PATCH</span>
                  <code>/messages/:message_id</code>
                </div>
                <p className={styles.description}>編輯訊息（作者本人）</p>
                <div className={styles.codeBlock}>
                  <pre>{`{
  "content": "新內容"
}`}</pre>
                </div>
              </div>

              <div className={styles.endpoint}>
                <div className={styles.endpointHeader}>
                  <span className={styles.method}>POST</span>
                  <code>/messages/:message_id/withdraw</code>
                </div>
                <p className={styles.description}>撤回訊息（作者/版主）</p>
              </div>

              <div className={styles.endpoint}>
                <div className={styles.endpointHeader}>
                  <span className={styles.method}>DELETE</span>
                  <code>/messages/:message_id</code>
                </div>
                <p className={styles.description}>刪除訊息（作者/版主）</p>
              </div>

              <div className={styles.endpoint}>
                <div className={styles.endpointHeader}>
                  <span className={styles.method}>POST</span>
                  <code>/messages/:message_id/reactions</code>
                </div>
                <p className={styles.description}>加 Emoji 反應</p>
                <div className={styles.codeBlock}>
                  <pre>{`{
  "emoji": "👍"
}`}</pre>
                </div>
              </div>

              <div className={styles.endpoint}>
                <div className={styles.endpointHeader}>
                  <span className={styles.method}>DELETE</span>
                  <code>/messages/:message_id/reactions/:emoji</code>
                </div>
                <p className={styles.description}>移除反應</p>
              </div>

              <div className={styles.endpoint}>
                <div className={styles.endpointHeader}>
                  <span className={styles.method}>POST</span>
                  <code>/messages/:message_id/pin</code>
                </div>
                <p className={styles.description}>置頂訊息（版主+）</p>
              </div>

              <div className={styles.endpoint}>
                <div className={styles.endpointHeader}>
                  <span className={styles.method}>POST</span>
                  <code>/messages/:message_id/forward</code>
                </div>
                <p className={styles.description}>轉發到其他頻道</p>
                <div className={styles.codeBlock}>
                  <pre>{`{
  "targetChannelId": "<uuid>"
}`}</pre>
                </div>
              </div>
            </section>
          )}

          {activeSection === "search" && (
            <section className={styles.section}>
              <h2>🔍 搜尋 API</h2>

              <div className={styles.endpoint}>
                <div className={styles.endpointHeader}>
                  <span className={styles.method}>GET</span>
                  <code>/search</code>
                </div>
                <p className={styles.description}>全文搜尋</p>
                <div className={styles.params}>
                  <p><strong>Query Parameters:</strong></p>
                  <ul>
                    <li><code>q</code> - 搜尋字串（1-200 字）</li>
                    <li><code>channelId</code> - 限制在特定頻道搜尋（選填）</li>
                    <li><code>limit</code> - 結果數量（預設 20，最大 50）</li>
                  </ul>
                </div>
                <div className={styles.codeBlock}>
                  <pre>{`{
  "messages": [ /* MessageOut[] */ ],
  "channels": [ /* ChannelOut[] */ ],
  "users": [ /* UserOut[] */ ]
}`}</pre>
                </div>
              </div>
            </section>
          )}

          {activeSection === "portfolio" && (
            <section className={styles.section}>
              <h2>📄 備審資料 API</h2>

              <div className={styles.endpoint}>
                <div className={styles.endpointHeader}>
                  <span className={styles.method}>GET</span>
                  <code>/portfolio/schools</code>
                </div>
                <p className={styles.description}>取得學校列表</p>
              </div>

              <div className={styles.endpoint}>
                <div className={styles.endpointHeader}>
                  <span className={styles.method}>GET</span>
                  <code>/portfolio/schools/:school_name/depts</code>
                </div>
                <p className={styles.description}>取得科系列表</p>
              </div>

              <div className={styles.endpoint}>
                <div className={styles.endpointHeader}>
                  <span className={styles.method}>POST</span>
                  <code>/portfolio/school-request</code>
                </div>
                <p className={styles.description}>申請新增學校/科系</p>
              </div>

              <div className={styles.endpoint}>
                <div className={styles.endpointHeader}>
                  <span className={styles.method}>GET</span>
                  <code>/portfolio</code>
                </div>
                <p className={styles.description}>瀏覽備審文件（可篩選）</p>
              </div>

              <div className={styles.endpoint}>
                <div className={styles.endpointHeader}>
                  <span className={styles.method}>POST</span>
                  <code>/portfolio/upload-url</code>
                </div>
                <p className={styles.description}>取得上傳預簽名 URL（學長姐+）</p>
              </div>

              <div className={styles.endpoint}>
                <div className={styles.endpointHeader}>
                  <span className={styles.method}>POST</span>
                  <code>/portfolio</code>
                </div>
                <p className={styles.description}>建立備審文件（學長姐+）</p>
              </div>

              <div className={styles.endpoint}>
                <div className={styles.endpointHeader}>
                  <span className={styles.method}>GET</span>
                  <code>/portfolio/:doc_id</code>
                </div>
                <p className={styles.description}>取得文件詳情</p>
              </div>

              <div className={styles.endpoint}>
                <div className={styles.endpointHeader}>
                  <span className={styles.method}>GET</span>
                  <code>/portfolio/:doc_id/download-url</code>
                </div>
                <p className={styles.description}>取得文件下載連結</p>
              </div>
            </section>
          )}

          {activeSection === "verification" && (
            <section className={styles.section}>
              <h2>✅ 驗證 API</h2>

              <div className={styles.endpoint}>
                <div className={styles.endpointHeader}>
                  <span className={styles.method}>GET</span>
                  <code>/verification/upload-url</code>
                </div>
                <p className={styles.description}>取得驗證文件上傳 URL</p>
              </div>

              <div className={styles.endpoint}>
                <div className={styles.endpointHeader}>
                  <span className={styles.method}>POST</span>
                  <code>/verification</code>
                </div>
                <p className={styles.description}>送出驗證申請</p>
                <div className={styles.codeBlock}>
                  <pre>{`{
  "fileKeys": ["uploads/student-id.jpg"],
  "docType": "學生證"
}`}</pre>
                </div>
              </div>

              <div className={styles.endpoint}>
                <div className={styles.endpointHeader}>
                  <span className={styles.method}>GET</span>
                  <code>/verification/status</code>
                </div>
                <p className={styles.description}>取得自己的驗證申請狀態</p>
              </div>

              <div className={styles.endpoint}>
                <div className={styles.endpointHeader}>
                  <span className={styles.method}>GET</span>
                  <code>/verification/admin/queue</code>
                </div>
                <p className={styles.description}>待審驗證清單（管理員）</p>
              </div>

              <div className={styles.endpoint}>
                <div className={styles.endpointHeader}>
                  <span className={styles.method}>PATCH</span>
                  <code>/verification/admin/:req_id</code>
                </div>
                <p className={styles.description}>審核通過/拒絕（管理員）</p>
              </div>
            </section>
          )}

          {activeSection === "tickets" && (
            <section className={styles.section}>
              <h2>🎫 客服票 API</h2>

              <div className={styles.endpoint}>
                <div className={styles.endpointHeader}>
                  <span className={styles.method}>GET</span>
                  <code>/tickets</code>
                </div>
                <p className={styles.description}>取得自己的客服票列表</p>
              </div>

              <div className={styles.endpoint}>
                <div className={styles.endpointHeader}>
                  <span className={styles.method}>POST</span>
                  <code>/tickets</code>
                </div>
                <p className={styles.description}>建立客服票</p>
                <div className={styles.codeBlock}>
                  <pre>{`{
  "category": "account",
  "subject": "帳號問題",
  "content": "我的帳號無法登入..."
}`}</pre>
                </div>
                <div className={styles.params}>
                  <p><strong>Category 選項:</strong></p>
                  <ul>
                    <li><code>account</code> - 帳號問題</li>
                    <li><code>verification</code> - 驗證問題</li>
                    <li><code>content</code> - 內容問題</li>
                    <li><code>bug</code> - 錯誤回報</li>
                    <li><code>other</code> - 其他</li>
                  </ul>
                </div>
              </div>

              <div className={styles.endpoint}>
                <div className={styles.endpointHeader}>
                  <span className={styles.method}>GET</span>
                  <code>/tickets/:ticket_id</code>
                </div>
                <p className={styles.description}>取得票詳情</p>
              </div>

              <div className={styles.endpoint}>
                <div className={styles.endpointHeader}>
                  <span className={styles.method}>POST</span>
                  <code>/tickets/:ticket_id/messages</code>
                </div>
                <p className={styles.description}>新增訊息</p>
              </div>
            </section>
          )}

          {activeSection === "admissions" && (
            <section className={styles.section}>
              <h2>📚 招生文件 API</h2>

              <div className={styles.endpoint}>
                <div className={styles.endpointHeader}>
                  <span className={styles.method}>GET</span>
                  <code>/admissions</code>
                </div>
                <p className={styles.description}>列出招生文件</p>
              </div>

              <div className={styles.endpoint}>
                <div className={styles.endpointHeader}>
                  <span className={styles.method}>GET</span>
                  <code>/admissions/:doc_id</code>
                </div>
                <p className={styles.description}>取得文件詳情</p>
              </div>

              <div className={styles.endpoint}>
                <div className={styles.endpointHeader}>
                  <span className={styles.method}>POST</span>
                  <code>/admissions/import-url</code>
                </div>
                <p className={styles.description}>從 URL 匯入 PDF（客服+）</p>
              </div>

              <div className={styles.endpoint}>
                <div className={styles.endpointHeader}>
                  <span className={styles.method}>POST</span>
                  <code>/admissions/import-file</code>
                </div>
                <p className={styles.description}>上傳 PDF 匯入（客服+）</p>
              </div>

              <div className={styles.endpoint}>
                <div className={styles.endpointHeader}>
                  <span className={styles.method}>GET</span>
                  <code>/admissions/:doc_id/pdf</code>
                </div>
                <p className={styles.description}>下載 PDF（token 參數認證）</p>
              </div>
            </section>
          )}

          {activeSection === "admin" && (
            <section className={styles.section}>
              <h2>⚙️ 管理 API</h2>
              <div className={styles.alert}>
                <strong>權限要求:</strong> <code>admin</code> 或 <code>developer</code> 角色
              </div>

              <div className={styles.endpoint}>
                <div className={styles.endpointHeader}>
                  <span className={styles.method}>GET</span>
                  <code>/admin/stats</code>
                </div>
                <p className={styles.description}>平台統計</p>
              </div>

              <div className={styles.endpoint}>
                <div className={styles.endpointHeader}>
                  <span className={styles.method}>GET</span>
                  <code>/admin/users</code>
                </div>
                <p className={styles.description}>用戶列表（可搜尋、分頁）</p>
              </div>

              <div className={styles.endpoint}>
                <div className={styles.endpointHeader}>
                  <span className={styles.method}>PATCH</span>
                  <code>/admin/users/:user_id</code>
                </div>
                <p className={styles.description}>修改用戶（角色、封禁等）</p>
              </div>

              <div className={styles.endpoint}>
                <div className={styles.endpointHeader}>
                  <span className={styles.method}>POST</span>
                  <code>/admin/users/:user_id/force-logout</code>
                </div>
                <p className={styles.description}>強制登出用戶</p>
              </div>

              <div className={styles.endpoint}>
                <div className={styles.endpointHeader}>
                  <span className={styles.method}>GET</span>
                  <code>/admin/channels</code>
                </div>
                <p className={styles.description}>頻道列表</p>
              </div>

              <div className={styles.endpoint}>
                <div className={styles.endpointHeader}>
                  <span className={styles.method}>POST</span>
                  <code>/admin/channels</code>
                </div>
                <p className={styles.description}>建立頻道</p>
              </div>

              <div className={styles.endpoint}>
                <div className={styles.endpointHeader}>
                  <span className={styles.method}>GET</span>
                  <code>/admin/audit-log</code>
                </div>
                <p className={styles.description}>稽核日誌（分頁）</p>
              </div>

              <div className={styles.endpoint}>
                <div className={styles.endpointHeader}>
                  <span className={styles.method}>POST</span>
                  <code>/admin/reputation/:user_id/adjust</code>
                </div>
                <p className={styles.description}>手動調整信譽值</p>
              </div>
            </section>
          )}

          {activeSection === "websocket" && (
            <section className={styles.section}>
              <h2>🔌 WebSocket 即時通訊</h2>

              <div className={styles.endpoint}>
                <div className={styles.endpointHeader}>
                  <span className={styles.methodWs}>WS</span>
                  <code>ws://localhost:12004/ws?token=&lt;access_token&gt;</code>
                </div>
                <p className={styles.description}>建立 WebSocket 連線</p>
              </div>

              <h3>客戶端 → 伺服器</h3>

              <div className={styles.endpoint}>
                <p className={styles.description}>訂閱頻道</p>
                <div className={styles.codeBlock}>
                  <pre>{`{
  "type": "subscribe",
  "channelId": "<uuid>"
}`}</pre>
                </div>
              </div>

              <div className={styles.endpoint}>
                <p className={styles.description}>取消訂閱</p>
                <div className={styles.codeBlock}>
                  <pre>{`{
  "type": "unsubscribe",
  "channelId": "<uuid>"
}`}</pre>
                </div>
              </div>

              <div className={styles.endpoint}>
                <p className={styles.description}>發送打字中指示</p>
                <div className={styles.codeBlock}>
                  <pre>{`{
  "type": "typing",
  "channelId": "<uuid>"
}`}</pre>
                </div>
              </div>

              <h3>伺服器 → 客戶端</h3>

              <div className={styles.endpoint}>
                <p className={styles.description}>新訊息</p>
                <div className={styles.codeBlock}>
                  <pre>{`{
  "type": "message.new",
  "data": { /* MessageOut */ }
}`}</pre>
                </div>
              </div>

              <div className={styles.endpoint}>
                <p className={styles.description}>訊息更新（編輯）</p>
                <div className={styles.codeBlock}>
                  <pre>{`{
  "type": "message.update",
  "data": { /* MessageOut */ }
}`}</pre>
                </div>
              </div>

              <div className={styles.endpoint}>
                <p className={styles.description}>打字中廣播</p>
                <div className={styles.codeBlock}>
                  <pre>{`{
  "type": "typing",
  "data": {
    "userId": "<uuid>",
    "channelId": "<uuid>"
  }
}`}</pre>
                </div>
              </div>
            </section>
          )}
        </div>
      </main>
    </div>
  );
}
