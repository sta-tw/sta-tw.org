import { Hono } from 'hono'
import { getCookie, setCookie, deleteCookie } from 'hono/cookie'
import { eq, or } from 'drizzle-orm'
import type { Env } from '../types'
import { createDb } from '../db'
import { users, userSessions } from '../db/schema'
import { hashPassword, verifyPassword } from '../utils/hash'
import { createAccessToken, createRefreshToken, createEmailToken, decodeToken } from '../utils/jwt'
import { sendVerificationEmail, sendPasswordResetEmail } from '../utils/email'
import { authMiddleware, getClientIp } from '../middleware/auth'
import { randomUUID } from '../utils/uuid'

const auth = new Hono<{ Bindings: Env }>()

const REFRESH_COOKIE = 'refresh_token'

function emailConfig(env: Env) {
  return { resendApiKey: env.RESEND_API_KEY, emailFrom: env.EMAIL_FROM, frontendUrl: env.FRONTEND_URL }
}

function userOut(u: typeof users.$inferSelect) {
  return {
    id: u.id, username: u.username, email: u.email,
    displayName: u.displayName, avatarUrl: u.avatarUrl,
    role: u.role, verificationStatus: u.verificationStatus,
    reputationScore: u.reputationScore, bio: u.bio,
    createdAt: u.createdAt.toISOString(),
  }
}

// POST /register
auth.post('/register', async (c) => {
  const body = await c.req.json()
  const { username, email, displayName, password } = body
  if (!username || !email || !displayName || !password) {
    return c.json({ detail: '缺少必要欄位' }, 422)
  }
  const db = createDb(c.env.DB)
  const existing = await db.select({ id: users.id }).from(users)
    .where(or(eq(users.email, email), eq(users.username, username)))
    .limit(1)
  if (existing.length > 0) return c.json({ detail: 'Email 或用戶名已被使用' }, 409)

  const id = randomUUID()
  await db.insert(users).values({
    id, username, email, displayName,
    hashedPassword: await hashPassword(password),
  })
  const token = await createEmailToken(id, 'email_verification', c.env.JWT_SECRET)
  await sendVerificationEmail(email, token, emailConfig(c.env))
  return c.json({ message: '註冊成功，請至信箱驗證帳號' }, 201)
})

// POST /login
auth.post('/login', async (c) => {
  const body = await c.req.json()
  const { email, password } = body
  const db = createDb(c.env.DB)
  const [user] = await db.select().from(users)
    .where(or(eq(users.email, email), eq(users.username, email)))
    .limit(1)

  if (!user || !user.hashedPassword || !(await verifyPassword(password, user.hashedPassword))) {
    return c.json({ detail: '帳號或密碼錯誤' }, 401)
  }
  if (!user.isActive) return c.json({ detail: '帳號已停用' }, 403)

  const expireDays = parseInt(c.env.REFRESH_TOKEN_EXPIRE_DAYS || '30')
  const sessionId = randomUUID()
  const expiresAt = new Date(Date.now() + expireDays * 86400000)
  const ip = getClientIp(c.req.raw)
  const device = c.req.header('User-Agent')?.slice(0, 500) ?? ''

  const refreshToken = await createRefreshToken(user.id, sessionId, c.env.JWT_SECRET, expireDays)
  await db.insert(userSessions).values({
    id: sessionId, userId: user.id,
    refreshTokenHash: await hashPassword(refreshToken),
    ipAddress: ip, deviceInfo: device, expiresAt,
  })

  const accessToken = await createAccessToken(
    user.id, c.env.JWT_SECRET,
    parseInt(c.env.ACCESS_TOKEN_EXPIRE_MINUTES || '15'),
  )
  setCookie(c, REFRESH_COOKIE, refreshToken, {
    httpOnly: true, sameSite: 'Lax', secure: true,
    maxAge: expireDays * 86400,
  })
  return c.json({ access_token: accessToken, user: userOut(user) })
})

// POST /logout
auth.post('/logout', async (c) => {
  const refreshToken = getCookie(c, REFRESH_COOKIE)
  if (refreshToken) {
    try {
      const payload = await decodeToken(refreshToken, 'refresh', c.env.JWT_SECRET)
      const sessionId = payload.jti as string
      const db = createDb(c.env.DB)
      await db.update(userSessions).set({ isRevoked: true })
        .where(eq(userSessions.id, sessionId))
    } catch { /* ignore */ }
  }
  deleteCookie(c, REFRESH_COOKIE)
  return c.json({ message: 'ok' })
})

// POST /refresh
auth.post('/refresh', async (c) => {
  const refreshToken = getCookie(c, REFRESH_COOKIE)
  if (!refreshToken) return c.json({ detail: 'No refresh token' }, 401)
  try {
    const payload = await decodeToken(refreshToken, 'refresh', c.env.JWT_SECRET)
    const sessionId = payload.jti as string
    const userId = payload.sub as string
    const db = createDb(c.env.DB)
    const [session] = await db.select().from(userSessions)
      .where(eq(userSessions.id, sessionId)).limit(1)
    if (!session || session.isRevoked || !(await verifyPassword(refreshToken, session.refreshTokenHash))) {
      return c.json({ detail: 'Session expired or revoked' }, 401)
    }
    const accessToken = await createAccessToken(
      userId, c.env.JWT_SECRET,
      parseInt(c.env.ACCESS_TOKEN_EXPIRE_MINUTES || '15'),
    )
    return c.json({ access_token: accessToken })
  } catch {
    return c.json({ detail: 'Invalid refresh token' }, 401)
  }
})

// GET /me
auth.get('/me', authMiddleware, async (c) => {
  const userId = c.get('userId')
  const db = createDb(c.env.DB)
  const [user] = await db.select().from(users).where(eq(users.id, userId)).limit(1)
  if (!user) return c.json({ detail: 'User not found' }, 404)
  return c.json(userOut(user))
})

// POST /verify-email
auth.post('/verify-email', async (c) => {
  const { token } = await c.req.json()
  try {
    const payload = await decodeToken(token, 'email_verification', c.env.JWT_SECRET)
    const db = createDb(c.env.DB)
    await db.update(users).set({ isEmailVerified: true }).where(eq(users.id, payload.sub as string))
    return c.json({ message: '信箱驗證成功' })
  } catch {
    return c.json({ detail: '無效或過期的驗證連結' }, 400)
  }
})

// POST /resend-verification
auth.post('/resend-verification', async (c) => {
  const { email } = await c.req.json()
  const db = createDb(c.env.DB)
  const [user] = await db.select().from(users).where(eq(users.email, email)).limit(1)
  if (user && !user.isEmailVerified) {
    const token = await createEmailToken(user.id, 'email_verification', c.env.JWT_SECRET)
    await sendVerificationEmail(email, token, emailConfig(c.env))
  }
  return c.json({ message: '驗證信已重新發送（若信箱存在）' })
})

// POST /forgot-password
auth.post('/forgot-password', async (c) => {
  const { email } = await c.req.json()
  const db = createDb(c.env.DB)
  const [user] = await db.select().from(users).where(eq(users.email, email)).limit(1)
  if (user) {
    const token = await createEmailToken(user.id, 'password_reset', c.env.JWT_SECRET, 1)
    await sendPasswordResetEmail(email, token, emailConfig(c.env))
  }
  return c.json({ message: '重設密碼信已發送（若信箱存在）' })
})

// POST /reset-password
auth.post('/reset-password', async (c) => {
  const { token, password } = await c.req.json()
  try {
    const payload = await decodeToken(token, 'password_reset', c.env.JWT_SECRET)
    if (!password || password.length < 8) return c.json({ detail: '密碼至少 8 個字元' }, 422)
    const db = createDb(c.env.DB)
    await db.update(users).set({ hashedPassword: await hashPassword(password) })
      .where(eq(users.id, payload.sub as string))
    return c.json({ message: '密碼已重設成功' })
  } catch {
    return c.json({ detail: '無效或過期的重設連結' }, 400)
  }
})

// POST /change-password
auth.post('/change-password', authMiddleware, async (c) => {
  const userId = c.get('userId')
  const { currentPassword, newPassword } = await c.req.json()
  const db = createDb(c.env.DB)
  const [user] = await db.select().from(users).where(eq(users.id, userId)).limit(1)
  if (!user || !user.hashedPassword || !(await verifyPassword(currentPassword, user.hashedPassword))) {
    return c.json({ detail: '目前密碼不正確' }, 400)
  }
  if (!newPassword || newPassword.length < 8) return c.json({ detail: '密碼至少 8 個字元' }, 422)
  await db.update(users).set({ hashedPassword: await hashPassword(newPassword) }).where(eq(users.id, userId))
  return c.json({ message: '密碼已更新' })
})

// GET /sessions
auth.get('/sessions', authMiddleware, async (c) => {
  const userId = c.get('userId')
  const refreshToken = getCookie(c, REFRESH_COOKIE)
  let currentSessionId: string | undefined
  if (refreshToken) {
    try {
      const p = await decodeToken(refreshToken, 'refresh', c.env.JWT_SECRET)
      currentSessionId = p.jti as string
    } catch { /* ignore */ }
  }
  const db = createDb(c.env.DB)
  const sessions = await db.select().from(userSessions)
    .where(eq(userSessions.userId, userId))
  return c.json(sessions
    .filter(s => !s.isRevoked)
    .sort((a, b) => b.createdAt.getTime() - a.createdAt.getTime())
    .map(s => ({
      id: s.id, deviceInfo: s.deviceInfo, ipAddress: s.ipAddress,
      createdAt: s.createdAt.toISOString(), expiresAt: s.expiresAt.toISOString(),
      isCurrent: s.id === currentSessionId,
    }))
  )
})

// DELETE /sessions/:session_id
auth.delete('/sessions/:session_id', authMiddleware, async (c) => {
  const userId = c.get('userId')
  const sessionId = c.req.param('session_id')
  const db = createDb(c.env.DB)
  const [session] = await db.select().from(userSessions)
    .where(eq(userSessions.id, sessionId)).limit(1)
  if (!session || session.userId !== userId) return c.json({ detail: 'Session not found' }, 404)
  await db.update(userSessions).set({ isRevoked: true }).where(eq(userSessions.id, sessionId))
  return c.json({ message: '裝置已登出' })
})

// DELETE /sessions
auth.delete('/sessions', authMiddleware, async (c) => {
  const userId = c.get('userId')
  const refreshToken = getCookie(c, REFRESH_COOKIE)
  let keepId: string | undefined
  if (refreshToken) {
    try {
      const p = await decodeToken(refreshToken, 'refresh', c.env.JWT_SECRET)
      keepId = p.jti as string
    } catch { /* ignore */ }
  }
  const db = createDb(c.env.DB)
  const sessions = await db.select().from(userSessions).where(eq(userSessions.userId, userId))
  for (const s of sessions) {
    if (!s.isRevoked && s.id !== keepId) {
      await db.update(userSessions).set({ isRevoked: true }).where(eq(userSessions.id, s.id))
    }
  }
  return c.json({ message: '其他裝置已全部登出' })
})

// ── OAuth ─────────────────────────────────────────────────────

auth.get('/oauth/google', (c) => {
  if (!c.env.GOOGLE_CLIENT_ID) return c.json({ detail: 'Google OAuth not configured' }, 501)
  const redirectUri = `${c.env.FRONTEND_URL}/api/v1/auth/oauth/google/callback`
  const url = `https://accounts.google.com/o/oauth2/v2/auth?client_id=${c.env.GOOGLE_CLIENT_ID}&redirect_uri=${encodeURIComponent(redirectUri)}&response_type=code&scope=openid+email+profile`
  return c.redirect(url)
})

auth.get('/oauth/google/callback', async (c) => {
  const code = c.req.query('code')
  if (!code) return c.json({ detail: 'Missing code' }, 400)
  const redirectUri = `${c.env.FRONTEND_URL}/api/v1/auth/oauth/google/callback`
  const tokenRes = await fetch('https://oauth2.googleapis.com/token', {
    method: 'POST',
    headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
    body: new URLSearchParams({
      code, client_id: c.env.GOOGLE_CLIENT_ID,
      client_secret: c.env.GOOGLE_CLIENT_SECRET,
      redirect_uri: redirectUri, grant_type: 'authorization_code',
    }),
  })
  const tokenData = await tokenRes.json() as { access_token: string }
  const userRes = await fetch('https://www.googleapis.com/oauth2/v3/userinfo', {
    headers: { Authorization: `Bearer ${tokenData.access_token}` },
  })
  const profile = await userRes.json() as { sub: string; email: string; name?: string; picture?: string }
  const { accessToken, refreshToken } = await oauthUpsert(c.env, {
    googleId: profile.sub, email: profile.email,
    displayName: profile.name ?? profile.email, avatarUrl: profile.picture,
  })
  const expireDays = parseInt(c.env.REFRESH_TOKEN_EXPIRE_DAYS || '30')
  setCookie(c, REFRESH_COOKIE, refreshToken, { httpOnly: true, sameSite: 'Lax', secure: true, maxAge: expireDays * 86400 })
  return c.redirect(`${c.env.FRONTEND_URL}/auth/oauth-callback?token=${accessToken}`)
})

auth.get('/oauth/discord', (c) => {
  if (!c.env.DISCORD_CLIENT_ID) return c.json({ detail: 'Discord OAuth not configured' }, 501)
  const redirectUri = `${c.env.FRONTEND_URL}/api/v1/auth/oauth/discord/callback`
  const url = `https://discord.com/api/oauth2/authorize?client_id=${c.env.DISCORD_CLIENT_ID}&redirect_uri=${encodeURIComponent(redirectUri)}&response_type=code&scope=identify+email`
  return c.redirect(url)
})

auth.get('/oauth/discord/callback', async (c) => {
  const code = c.req.query('code')
  if (!code) return c.json({ detail: 'Missing code' }, 400)
  const redirectUri = `${c.env.FRONTEND_URL}/api/v1/auth/oauth/discord/callback`
  const tokenRes = await fetch('https://discord.com/api/oauth2/token', {
    method: 'POST',
    headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
    body: new URLSearchParams({
      client_id: c.env.DISCORD_CLIENT_ID, client_secret: c.env.DISCORD_CLIENT_SECRET,
      grant_type: 'authorization_code', code, redirect_uri: redirectUri,
    }),
  })
  const tokenData = await tokenRes.json() as { access_token: string }
  const userRes = await fetch('https://discord.com/api/users/@me', {
    headers: { Authorization: `Bearer ${tokenData.access_token}` },
  })
  const profile = await userRes.json() as { id: string; email?: string; username: string; global_name?: string; avatar?: string }
  const avatarUrl = profile.avatar
    ? `https://cdn.discordapp.com/avatars/${profile.id}/${profile.avatar}.png`
    : undefined
  const { accessToken, refreshToken } = await oauthUpsert(c.env, {
    discordId: profile.id, email: profile.email ?? `${profile.id}@discord`,
    displayName: profile.global_name ?? profile.username, avatarUrl,
  })
  const expireDays = parseInt(c.env.REFRESH_TOKEN_EXPIRE_DAYS || '30')
  setCookie(c, REFRESH_COOKIE, refreshToken, { httpOnly: true, sameSite: 'Lax', secure: true, maxAge: expireDays * 86400 })
  return c.redirect(`${c.env.FRONTEND_URL}/auth/oauth-callback?token=${accessToken}`)
})

async function oauthUpsert(env: Env, opts: {
  email: string; displayName: string; avatarUrl?: string;
  googleId?: string; discordId?: string;
}): Promise<{ accessToken: string; refreshToken: string }> {
  const db = createDb(env.DATABASE_URL)
  const conditions = [eq(users.email, opts.email)]
  if (opts.googleId) conditions.push(eq(users.googleId, opts.googleId))
  if (opts.discordId) conditions.push(eq(users.discordId, opts.discordId))

  let user = (await db.select().from(users).where(or(...conditions)).limit(1))[0]

  if (!user) {
    const usernameBase = opts.email.split('@')[0].slice(0, 45)
    let uname = usernameBase
    let i = 1
    while (true) {
      const existing = await db.select({ id: users.id }).from(users).where(eq(users.username, uname)).limit(1)
      if (existing.length === 0) break
      uname = `${usernameBase}${i++}`
    }
    const id = randomUUID()
    await db.insert(users).values({
      id, username: uname, email: opts.email, displayName: opts.displayName,
      avatarUrl: opts.avatarUrl, isEmailVerified: true,
      googleId: opts.googleId, discordId: opts.discordId,
    })
    user = (await db.select().from(users).where(eq(users.id, id)).limit(1))[0]
  } else {
    const updates: Partial<typeof users.$inferInsert> = {}
    if (opts.googleId && !user.googleId) updates.googleId = opts.googleId
    if (opts.discordId && !user.discordId) updates.discordId = opts.discordId
    if (opts.avatarUrl && !user.avatarUrl) updates.avatarUrl = opts.avatarUrl
    if (Object.keys(updates).length > 0) {
      await db.update(users).set(updates).where(eq(users.id, user.id))
    }
  }

  const expireDays = parseInt(env.REFRESH_TOKEN_EXPIRE_DAYS || '30')
  const sessionId = randomUUID()
  const refreshToken = await createRefreshToken(user.id, sessionId, env.JWT_SECRET, expireDays)
  await db.insert(userSessions).values({
    id: sessionId, userId: user.id,
    refreshTokenHash: await hashPassword(refreshToken),
    expiresAt: new Date(Date.now() + expireDays * 86400000),
  })
  const accessToken = await createAccessToken(user.id, env.JWT_SECRET, parseInt(env.ACCESS_TOKEN_EXPIRE_MINUTES || '15'))
  return { accessToken, refreshToken }
}

export default auth
