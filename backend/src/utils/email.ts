export interface EmailConfig {
  resendApiKey: string
  emailFrom: string
  frontendUrl: string
}

async function sendEmail(to: string, subject: string, html: string, config: EmailConfig): Promise<void> {
  if (!config.resendApiKey) {
    console.log(`[DEV EMAIL] to=${to} | subject=${subject}\n${html}`)
    return
  }
  try {
    const res = await fetch('https://api.resend.com/emails', {
      method: 'POST',
      headers: {
        Authorization: `Bearer ${config.resendApiKey}`,
        'Content-Type': 'application/json',
      },
      body: JSON.stringify({
        from: config.emailFrom,
        to: [to],
        subject,
        html,
      }),
    })
    if (!res.ok) {
      const err = await res.text()
      console.error(`Failed to send email: ${err}`)
    }
  } catch (e) {
    console.error('Email send error:', e)
  }
}

export async function sendVerificationEmail(to: string, token: string, config: EmailConfig): Promise<void> {
  const url = `${config.frontendUrl}/auth/verify-email?token=${token}`
  await sendEmail(
    to,
    '【STA 論壇】請驗證您的電子郵件',
    `<p>請點擊以下連結完成驗證（24 小時內有效）：</p><p><a href="${url}">${url}</a></p>`,
    config,
  )
}

export async function sendPasswordResetEmail(to: string, token: string, config: EmailConfig): Promise<void> {
  const url = `${config.frontendUrl}/auth/reset-password?token=${token}`
  await sendEmail(
    to,
    '【STA 論壇】重設您的密碼',
    `<p>請點擊以下連結重設密碼（1 小時內有效）：</p><p><a href="${url}">${url}</a></p>`,
    config,
  )
}
