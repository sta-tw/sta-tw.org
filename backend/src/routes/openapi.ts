import { Hono } from 'hono'
import type { Env } from '../types'

const openapi = new Hono<{ Bindings: Env }>()

openapi.get('/openapi.json', (c) => {
  const spec = {
    openapi: '3.0.0',
    info: {
      title: 'STA Platform API',
      version: '1.0.0',
      description: 'STA 特殊選才資源網 API',
    },
    servers: [
      { url: 'http://localhost:12004/api/v1', description: 'Local' },
    ],
    paths: {
      '/auth/register': {
        post: {
          tags: ['Auth 認證'],
          summary: '註冊新用戶',
          requestBody: {
            content: {
              'application/json': {
                schema: {
                  type: 'object',
                  properties: {
                    username: { type: 'string' },
                    email: { type: 'string' },
                    displayName: { type: 'string' },
                    password: { type: 'string' },
                  },
                },
              },
            },
          },
        },
      },
      '/auth/login': {
        post: {
          tags: ['Auth 認證'],
          summary: '用戶登入',
        },
      },
      '/auth/logout': {
        post: {
          tags: ['Auth 認證'],
          summary: '登出',
        },
      },
      '/auth/refresh': {
        post: {
          tags: ['Auth 認證'],
          summary: '更新 Access Token',
        },
      },
      '/auth/me': {
        get: {
          tags: ['Auth 認證'],
          summary: '取得當前用戶資料',
        },
      },
      '/users/me': {
        get: {
          tags: ['Users 用戶'],
          summary: '取得自己的完整資料',
        },
        patch: {
          tags: ['Users 用戶'],
          summary: '更新個人資料',
        },
      },
      '/users/me/avatar-upload-url': {
        get: {
          tags: ['Users 用戶'],
          summary: '取得頭像上傳預簽名 URL',
        },
      },
      '/users/{username}': {
        get: {
          tags: ['Users 用戶'],
          summary: '取得他人公開資料',
        },
      },
      '/channels': {
        get: {
          tags: ['Channels 頻道'],
          summary: '列出所有頻道',
        },
      },
      '/channels/{channel_id}': {
        get: {
          tags: ['Channels 頻道'],
          summary: '取得頻道詳情',
        },
      },
      '/channels/{channel_id}/messages': {
        get: {
          tags: ['Channels 頻道'],
          summary: '取得訊息列表（分頁）',
        },
        post: {
          tags: ['Channels 頻道'],
          summary: '發送訊息',
        },
      },
      '/channels/{channel_id}/pinned': {
        get: {
          tags: ['Channels 頻道'],
          summary: '取得置頂訊息',
        },
      },
      '/messages/{message_id}': {
        patch: {
          tags: ['Messages 訊息'],
          summary: '編輯訊息',
        },
        delete: {
          tags: ['Messages 訊息'],
          summary: '刪除訊息',
        },
      },
      '/messages/{message_id}/withdraw': {
        post: {
          tags: ['Messages 訊息'],
          summary: '撤回訊息',
        },
      },
      '/messages/{message_id}/reactions': {
        post: {
          tags: ['Messages 訊息'],
          summary: '加 Emoji 反應',
        },
      },
      '/messages/{message_id}/reactions/{emoji}': {
        delete: {
          tags: ['Messages 訊息'],
          summary: '移除反應',
        },
      },
      '/messages/{message_id}/pin': {
        post: {
          tags: ['Messages 訊息'],
          summary: '置頂訊息',
        },
        delete: {
          tags: ['Messages 訊息'],
          summary: '取消置頂',
        },
      },
      '/messages/{message_id}/thread': {
        get: {
          tags: ['Messages 訊息'],
          summary: '取得 Thread 回覆',
        },
      },
      '/messages/{message_id}/forward': {
        post: {
          tags: ['Messages 訊息'],
          summary: '轉發到其他頻道',
        },
      },
      '/search': {
        get: {
          tags: ['Search 搜尋'],
          summary: '全文搜尋',
        },
      },
      '/portfolio/schools': {
        get: {
          tags: ['Portfolio 備審資料'],
          summary: '取得學校列表',
        },
      },
      '/portfolio/schools/{school_name}/depts': {
        get: {
          tags: ['Portfolio 備審資料'],
          summary: '取得科系列表',
        },
      },
      '/portfolio/school-request': {
        post: {
          tags: ['Portfolio 備審資料'],
          summary: '申請新增學校/科系',
        },
      },
      '/portfolio': {
        get: {
          tags: ['Portfolio 備審資料'],
          summary: '瀏覽備審文件',
        },
        post: {
          tags: ['Portfolio 備審資料'],
          summary: '建立備審文件',
        },
      },
      '/portfolio/upload-url': {
        post: {
          tags: ['Portfolio 備審資料'],
          summary: '取得上傳預簽名 URL',
        },
      },
      '/portfolio/{doc_id}': {
        get: {
          tags: ['Portfolio 備審資料'],
          summary: '取得文件詳情',
        },
        delete: {
          tags: ['Portfolio 備審資料'],
          summary: '刪除文件',
        },
      },
      '/portfolio/{doc_id}/download-url': {
        get: {
          tags: ['Portfolio 備審資料'],
          summary: '取得文件下載連結',
        },
      },
      '/verification/upload-url': {
        get: {
          tags: ['Verification 驗證'],
          summary: '取得驗證文件上傳 URL',
        },
      },
      '/verification': {
        post: {
          tags: ['Verification 驗證'],
          summary: '送出驗證申請',
        },
      },
      '/verification/status': {
        get: {
          tags: ['Verification 驗證'],
          summary: '取得自己的驗證申請狀態',
        },
      },
      '/verification/admin/queue': {
        get: {
          tags: ['Verification 驗證'],
          summary: '待審驗證清單（管理員）',
        },
      },
      '/verification/admin/{req_id}': {
        patch: {
          tags: ['Verification 驗證'],
          summary: '審核通過/拒絕（管理員）',
        },
      },
      '/tickets': {
        get: {
          tags: ['Tickets 客服票'],
          summary: '取得自己的客服票列表',
        },
        post: {
          tags: ['Tickets 客服票'],
          summary: '建立客服票',
        },
      },
      '/tickets/{ticket_id}': {
        get: {
          tags: ['Tickets 客服票'],
          summary: '取得票詳情',
        },
      },
      '/tickets/{ticket_id}/messages': {
        post: {
          tags: ['Tickets 客服票'],
          summary: '新增訊息',
        },
      },
      '/tickets/admin/all': {
        get: {
          tags: ['Tickets 客服票'],
          summary: '所有票（客服人員）',
        },
      },
      '/tickets/admin/{ticket_id}': {
        patch: {
          tags: ['Tickets 客服票'],
          summary: '更新狀態/指派（客服人員）',
        },
      },
      '/admissions': {
        get: {
          tags: ['Admissions 招生文件'],
          summary: '列出招生文件',
        },
      },
      '/admissions/{doc_id}': {
        get: {
          tags: ['Admissions 招生文件'],
          summary: '取得文件詳情',
        },
        delete: {
          tags: ['Admissions 招生文件'],
          summary: '刪除文件',
        },
      },
      '/admissions/import-url': {
        post: {
          tags: ['Admissions 招生文件'],
          summary: '從 URL 匯入 PDF',
        },
      },
      '/admissions/import-file': {
        post: {
          tags: ['Admissions 招生文件'],
          summary: '上傳 PDF 匯入',
        },
      },
      '/admissions/{doc_id}/pdf': {
        get: {
          tags: ['Admissions 招生文件'],
          summary: '下載 PDF',
        },
      },
      '/admin/stats': {
        get: {
          tags: ['Admin 管理'],
          summary: '平台統計',
        },
      },
      '/admin/users': {
        get: {
          tags: ['Admin 管理'],
          summary: '用戶列表',
        },
      },
      '/admin/users/{user_id}': {
        get: {
          tags: ['Admin 管理'],
          summary: '用戶詳情',
        },
        patch: {
          tags: ['Admin 管理'],
          summary: '修改用戶',
        },
      },
      '/admin/users/{user_id}/force-logout': {
        post: {
          tags: ['Admin 管理'],
          summary: '強制登出用戶',
        },
      },
      '/admin/channels': {
        get: {
          tags: ['Admin 管理'],
          summary: '頻道列表',
        },
        post: {
          tags: ['Admin 管理'],
          summary: '建立頻道',
        },
      },
      '/admin/channels/{channel_id}': {
        patch: {
          tags: ['Admin 管理'],
          summary: '更新頻道',
        },
        delete: {
          tags: ['Admin 管理'],
          summary: '刪除頻道',
        },
      },
      '/admin/audit-log': {
        get: {
          tags: ['Admin 管理'],
          summary: '稽核日誌',
        },
      },
      '/admin/audit-log/export': {
        get: {
          tags: ['Admin 管理'],
          summary: '匯出 CSV',
        },
      },
      '/public/stats': {
        get: {
          tags: ['Public 公開'],
          summary: '公開統計資料',
        },
      },
    },
  }

  return c.json(spec)
})

export default openapi
