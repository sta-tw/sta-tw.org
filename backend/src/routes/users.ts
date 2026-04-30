import { Hono } from 'hono'
import { eq, ilike } from 'drizzle-orm'
import type { Env } from '../types'
import { createDb } from '../db'
import { users } from '../db/schema'
import { authMiddleware } from '../middleware/auth'
import { generatePresignedUploadUrl } from '../utils/storage'
import { randomUUID } from '../utils/uuid'

const usersRouter = new Hono<{ Bindings: Env }>()

function userOut(u: typeof users.$inferSelect) {
  return {
    id: u.id, username: u.username, email: u.email,
    displayName: u.displayName, avatarUrl: u.avatarUrl,
    role: u.role, verificationStatus: u.verificationStatus,
    reputationScore: u.reputationScore, bio: u.bio,
    createdAt: u.createdAt.toISOString(),
  }
}

// GET /me
usersRouter.get('/me', authMiddleware, async (c) => {
  const userId = c.get('userId')
  const db = createDb(c.env.DB)
  const [user] = await db.select().from(users).where(eq(users.id, userId)).limit(1)
  if (!user) return c.json({ detail: 'User not found' }, 404)
  return c.json(userOut(user))
})

// PATCH /me
usersRouter.patch('/me', authMiddleware, async (c) => {
  const userId = c.get('userId')
  const body = await c.req.json()
  const db = createDb(c.env.DB)
  const updates: Partial<typeof users.$inferInsert> = {}
  if (body.displayName !== undefined) updates.displayName = body.displayName
  if (body.bio !== undefined) updates.bio = body.bio
  if (body.avatarUrl !== undefined) updates.avatarUrl = body.avatarUrl
  if (Object.keys(updates).length > 0) {
    await db.update(users).set(updates).where(eq(users.id, userId))
  }
  const [user] = await db.select().from(users).where(eq(users.id, userId)).limit(1)
  return c.json(userOut(user))
})

// GET /me/avatar-upload-url
usersRouter.get('/me/avatar-upload-url', authMiddleware, async (c) => {
  const userId = c.get('userId')
  const fileKey = `avatars/${userId}/${randomUUID()}.jpg`
  const url = await generatePresignedUploadUrl(fileKey, 'image/jpeg', {
    accountId: c.env.R2_ACCOUNT_ID,
    accessKeyId: c.env.R2_ACCESS_KEY_ID,
    secretAccessKey: c.env.R2_SECRET_ACCESS_KEY,
    bucket: c.env.R2_BUCKET_NAME,
  })
  return c.json({ uploadUrl: url, fileKey })
})

// GET /:username
usersRouter.get('/:username', authMiddleware, async (c) => {
  const username = c.req.param('username')
  const db = createDb(c.env.DB)
  const [user] = await db.select().from(users).where(eq(users.username, username)).limit(1)
  if (!user) return c.json({ detail: 'User not found' }, 404)
  return c.json(userOut(user))
})

export default usersRouter
