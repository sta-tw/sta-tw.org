import { Hono } from 'hono'
import { eq, desc, inArray } from 'drizzle-orm'
import type { Env } from '../types'
import { createDb } from '../db'
import { verificationRequests, users } from '../db/schema'
import { authMiddleware } from '../middleware/auth'
import { generatePresignedUploadUrl } from '../utils/storage'
import { randomUUID } from '../utils/uuid'

const verificationRouter = new Hono<{ Bindings: Env }>()

function storageConfig(env: Env) {
  return {
    accountId: env.R2_ACCOUNT_ID,
    accessKeyId: env.R2_ACCESS_KEY_ID,
    secretAccessKey: env.R2_SECRET_ACCESS_KEY,
    bucket: env.R2_BUCKET_NAME,
  }
}

function verificationOut(vr: typeof verificationRequests.$inferSelect, submitterName = '') {
  return {
    id: vr.id, userId: vr.userId, submitterDisplayName: submitterName,
    status: vr.status, fileKey: vr.fileKey, fileHash: vr.fileHash,
    fileKeys: vr.fileKeys ? JSON.parse(vr.fileKeys) : null,
    docType: vr.docType, adminNote: vr.adminNote,
    submittedAt: vr.submittedAt.toISOString(),
    reviewedAt: vr.reviewedAt?.toISOString() ?? null,
    reviewedById: vr.reviewedById,
  }
}

// GET /upload-url
verificationRouter.get('/upload-url', authMiddleware, async (c) => {
  const userId = c.get('userId')
  const contentType = c.req.query('content_type') ?? 'application/pdf'
  const fileKey = `verification/${userId}/${randomUUID()}`
  const url = await generatePresignedUploadUrl(fileKey, contentType, storageConfig(c.env))
  return c.json({ uploadUrl: url, fileKey })
})

// POST /
verificationRouter.post('/', authMiddleware, async (c) => {
  const userId = c.get('userId')
  const { fileKey, fileHash, fileKeys, docType } = await c.req.json()
  const db = createDb(c.env.DB)
  const id = randomUUID()
  await db.insert(verificationRequests).values({
    id, userId,
    fileKey: fileKey ?? null,
    fileHash: fileHash ?? null,
    fileKeys: fileKeys ? JSON.stringify(fileKeys) : null,
    docType: docType ?? null,
  })
  const [vr] = await db.select().from(verificationRequests)
    .where(eq(verificationRequests.id, id)).limit(1)
  return c.json(verificationOut(vr), 201)
})

// GET /status
verificationRouter.get('/status', authMiddleware, async (c) => {
  const userId = c.get('userId')
  const db = createDb(c.env.DB)
  const [vr] = await db.select().from(verificationRequests)
    .where(eq(verificationRequests.userId, userId))
    .orderBy(desc(verificationRequests.submittedAt))
    .limit(1)
  if (!vr) return c.json(null)
  return c.json(verificationOut(vr))
})

// GET /admin/queue
verificationRouter.get('/admin/queue', authMiddleware, async (c) => {
  const db = createDb(c.env.DB)
  const pending = await db.select().from(verificationRequests)
    .where(eq(verificationRequests.status, 'pending'))
    .orderBy(desc(verificationRequests.submittedAt))

  const userIds = [...new Set(pending.map(vr => vr.userId))]
  const userList = userIds.length > 0
    ? await db.select().from(users).where(inArray(users.id, userIds))
    : []
  const usersMap = Object.fromEntries(userList.map(u => [u.id, u]))

  return c.json(pending.map(vr => verificationOut(vr, usersMap[vr.userId]?.displayName ?? '')))
})

// PATCH /admin/:request_id
verificationRouter.patch('/admin/:request_id', authMiddleware, async (c) => {
  const requestId = c.req.param('request_id')
  const reviewerId = c.get('userId')
  const { status, adminNote, role } = await c.req.json()
  const db = createDb(c.env.DB)
  const [vr] = await db.select().from(verificationRequests)
    .where(eq(verificationRequests.id, requestId)).limit(1)
  if (!vr) return c.json({ detail: 'Request not found' }, 404)

  await db.update(verificationRequests).set({
    status,
    adminNote: adminNote ?? null,
    reviewedAt: new Date(),
    reviewedById: reviewerId,
  }).where(eq(verificationRequests.id, requestId))

  if (status === 'approved') {
    if (role) {
      await db.update(users).set({ verificationStatus: 'approved', role }).where(eq(users.id, vr.userId))
    } else {
      await db.update(users).set({ verificationStatus: 'approved' }).where(eq(users.id, vr.userId))
    }
  } else if (status === 'rejected') {
    await db.update(users).set({ verificationStatus: 'rejected' }).where(eq(users.id, vr.userId))
  }

  const [updated] = await db.select().from(verificationRequests)
    .where(eq(verificationRequests.id, requestId)).limit(1)
  const [submitter] = await db.select().from(users).where(eq(users.id, vr.userId)).limit(1)
  return c.json(verificationOut(updated, submitter?.displayName ?? ''))
})

export default verificationRouter
