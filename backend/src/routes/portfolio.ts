import { Hono } from 'hono'
import { eq, and, ilike, asc, sql } from 'drizzle-orm'
import type { Env } from '../types'
import { createDb } from '../db'
import {
  portfolioDocuments, portfolioScoringRules, portfolioViewLogs,
  portfolioSchoolOptions, portfolioSchoolRequests, users, auditLogs,
} from '../db/schema'
import { authMiddleware } from '../middleware/auth'
import { generatePresignedUploadUrl, generatePresignedGetUrl } from '../utils/storage'
import { randomUUID } from '../utils/uuid'

const portfolioRouter = new Hono<{ Bindings: Env }>()

function storageConfig(env: Env) {
  return {
    accountId: env.R2_ACCOUNT_ID,
    accessKeyId: env.R2_ACCESS_KEY_ID,
    secretAccessKey: env.R2_SECRET_ACCESS_KEY,
    bucket: env.R2_BUCKET_NAME,
  }
}

// ── Score helpers ──────────────────────────────────────────────

function currentCohort(): number {
  return new Date().getFullYear() - 1911
}

function cohortScore(admissionYear: number, current: number): number {
  const yearsBack = current - admissionYear
  if (yearsBack === 0) return 3
  if (yearsBack <= 3) return 5
  if (yearsBack <= 5) return 4
  if (yearsBack <= 8) return 3
  if (yearsBack <= 11) return 2
  if (yearsBack <= 16) return 1
  return 0
}

function viewScore(viewCount: number, longViewCount: number): number {
  return Math.round((viewCount * 0.1 + longViewCount * 0.5) * 100) / 100
}

async function getDeptRuleScore(schoolName: string, deptName: string, db: ReturnType<typeof createDb>): Promise<number> {
  const [rule] = await db.select({ score: portfolioScoringRules.score }).from(portfolioScoringRules)
    .where(and(eq(portfolioScoringRules.schoolName, schoolName), eq(portfolioScoringRules.deptName, deptName)))
    .limit(1)
  return rule?.score ?? 0
}

function docOut(
  doc: typeof portfolioDocuments.$inferSelect,
  uploaderName: string,
  recommendationScore: number,
  downloadUrl?: string | null,
) {
  return {
    id: doc.id, uploaderId: doc.uploaderId, uploaderName,
    title: doc.title || `${doc.schoolName} ${doc.deptName}`,
    description: doc.description, schoolName: doc.schoolName, deptName: doc.deptName,
    admissionYear: doc.admissionYear, applicantName: doc.applicantName,
    fileName: doc.fileName, fileSize: doc.fileSize,
    isApproved: doc.isApproved, viewCount: doc.viewCount, longViewCount: doc.longViewCount,
    createdAt: doc.createdAt.toISOString(), updatedAt: doc.updatedAt.toISOString(),
    downloadUrl: downloadUrl ?? null, recommendationScore,
    resultType: doc.resultType, admittedRank: doc.admittedRank,
    totalAdmitted: doc.totalAdmitted, waitlistRank: doc.waitlistRank,
    portfolioScore: doc.portfolioScore,
  }
}

// GET /upload-url
portfolioRouter.get('/upload-url', authMiddleware, async (c) => {
  const userId = c.get('userId')
  const fileName = c.req.query('file_name') ?? 'file.pdf'
  const fileSize = parseInt(c.req.query('file_size') ?? '0')
  const contentType = c.req.query('content_type') ?? 'application/pdf'
  const fileKey = `portfolio/${userId}/${randomUUID()}_${fileName}`
  const url = await generatePresignedUploadUrl(fileKey, contentType, storageConfig(c.env))
  return c.json({ uploadUrl: url, fileKey })
})

// GET /
portfolioRouter.get('/', authMiddleware, async (c) => {
  const schoolName = c.req.query('school_name')
  const deptName = c.req.query('dept_name')
  const admissionYear = c.req.query('admission_year')
  const category = c.req.query('category')
  const approvedOnly = c.req.query('approved_only') !== 'false'

  const db = createDb(c.env.DATABASE_URL)
  let q = db.select().from(portfolioDocuments)
  const conditions = []
  if (approvedOnly) conditions.push(eq(portfolioDocuments.isApproved, true))
  if (schoolName) conditions.push(ilike(portfolioDocuments.schoolName, `%${schoolName}%`))
  if (deptName) conditions.push(ilike(portfolioDocuments.deptName, `%${deptName}%`))
  if (admissionYear) conditions.push(eq(portfolioDocuments.admissionYear, parseInt(admissionYear)))
  if (category) conditions.push(eq(portfolioDocuments.category, category))

  const docs = conditions.length > 0
    ? await db.select().from(portfolioDocuments).where(and(...conditions))
    : await db.select().from(portfolioDocuments)

  const uploaderIds = [...new Set(docs.map(d => d.uploaderId))]
  const uploaderList = uploaderIds.length > 0
    ? await db.select().from(users)
    : []
  const uploaderMap = Object.fromEntries(uploaderList.map(u => [u.id, u.displayName]))

  const current = currentCohort()
  const out = await Promise.all(docs.map(async (doc) => {
    const deptScore = await getDeptRuleScore(doc.schoolName, doc.deptName, db)
    const score = cohortScore(doc.admissionYear, current) + deptScore + viewScore(doc.viewCount, doc.longViewCount)
    return docOut(doc, uploaderMap[doc.uploaderId] ?? 'Unknown', score)
  }))
  out.sort((a, b) => b.recommendationScore - a.recommendationScore)
  return c.json(out)
})

// POST /
portfolioRouter.post('/', authMiddleware, async (c) => {
  const userId = c.get('userId')
  const body = await c.req.json()
  const { title, description, schoolName, deptName, admissionYear, applicantName,
    fileKey, fileName, fileSize, resultType, admittedRank, totalAdmitted,
    waitlistRank, portfolioScore } = body

  const db = createDb(c.env.DATABASE_URL)
  const id = randomUUID()
  const autoTitle = title || `${schoolName} ${deptName} ${applicantName ?? ''}`
  await db.insert(portfolioDocuments).values({
    id, uploaderId: userId, title: autoTitle, description: description ?? null,
    schoolName, deptName, admissionYear, applicantName: applicantName ?? null,
    fileKey, fileName, fileSize: fileSize ?? 0, isApproved: false,
    resultType: resultType ?? null, admittedRank: admittedRank ?? null,
    totalAdmitted: totalAdmitted ?? null, waitlistRank: waitlistRank ?? null,
    portfolioScore: portfolioScore ?? null,
  })
  const [doc] = await db.select().from(portfolioDocuments).where(eq(portfolioDocuments.id, id)).limit(1)
  const [uploader] = await db.select().from(users).where(eq(users.id, userId)).limit(1)
  const current = currentCohort()
  const deptScore = await getDeptRuleScore(doc.schoolName, doc.deptName, db)
  const score = cohortScore(doc.admissionYear, current) + deptScore
  return c.json(docOut(doc, uploader?.displayName ?? 'Unknown', score), 201)
})

// GET /schools
portfolioRouter.get('/schools', authMiddleware, async (c) => {
  const db = createDb(c.env.DATABASE_URL)
  const rows = await db.selectDistinct({ schoolName: portfolioSchoolOptions.schoolName })
    .from(portfolioSchoolOptions)
    .where(eq(portfolioSchoolOptions.isActive, true))
    .orderBy(asc(portfolioSchoolOptions.schoolName))
  return c.json(rows.map(r => r.schoolName))
})

// GET /schools/:school_name/depts
portfolioRouter.get('/schools/:school_name/depts', authMiddleware, async (c) => {
  const schoolName = decodeURIComponent(c.req.param('school_name'))
  const db = createDb(c.env.DATABASE_URL)
  const rows = await db.select({ deptName: portfolioSchoolOptions.deptName })
    .from(portfolioSchoolOptions)
    .where(and(eq(portfolioSchoolOptions.schoolName, schoolName), eq(portfolioSchoolOptions.isActive, true)))
    .orderBy(asc(portfolioSchoolOptions.sortOrder), asc(portfolioSchoolOptions.deptName))
  return c.json(rows.map(r => r.deptName))
})

// GET /:doc_id
portfolioRouter.get('/:doc_id', authMiddleware, async (c) => {
  const docId = c.req.param('doc_id')
  const db = createDb(c.env.DATABASE_URL)
  const [doc] = await db.select().from(portfolioDocuments).where(eq(portfolioDocuments.id, docId)).limit(1)
  if (!doc) return c.json({ detail: 'Not found' }, 404)
  const [uploader] = await db.select().from(users).where(eq(users.id, doc.uploaderId)).limit(1)
  const current = currentCohort()
  const deptScore = await getDeptRuleScore(doc.schoolName, doc.deptName, db)
  const score = cohortScore(doc.admissionYear, current) + deptScore + viewScore(doc.viewCount, doc.longViewCount)
  return c.json(docOut(doc, uploader?.displayName ?? 'Unknown', score))
})

// GET /:doc_id/download-url
portfolioRouter.get('/:doc_id/download-url', authMiddleware, async (c) => {
  const docId = c.req.param('doc_id')
  const db = createDb(c.env.DATABASE_URL)
  const [doc] = await db.select().from(portfolioDocuments).where(eq(portfolioDocuments.id, docId)).limit(1)
  if (!doc) return c.json({ detail: 'Not found' }, 404)
  // Increment view count
  await db.update(portfolioDocuments)
    .set({ viewCount: sql`${portfolioDocuments.viewCount} + 1` })
    .where(eq(portfolioDocuments.id, docId))
  const url = await generatePresignedGetUrl(doc.fileKey, storageConfig(c.env))
  return c.json({ downloadUrl: url })
})

// DELETE /:doc_id
portfolioRouter.delete('/:doc_id', authMiddleware, async (c) => {
  const docId = c.req.param('doc_id')
  const userId = c.get('userId')
  const db = createDb(c.env.DATABASE_URL)
  const [doc] = await db.select().from(portfolioDocuments).where(eq(portfolioDocuments.id, docId)).limit(1)
  if (!doc) return c.json({ detail: 'Not found' }, 404)
  if (doc.uploaderId !== userId) return c.json({ detail: 'Permission denied' }, 403)
  await db.delete(portfolioDocuments).where(eq(portfolioDocuments.id, docId))
  return c.body(null, 204)
})

// POST /:doc_id/long-view
portfolioRouter.post('/:doc_id/long-view', authMiddleware, async (c) => {
  const docId = c.req.param('doc_id')
  const userId = c.get('userId')
  const db = createDb(c.env.DATABASE_URL)
  const [log] = await db.select().from(portfolioViewLogs)
    .where(and(eq(portfolioViewLogs.docId, docId), eq(portfolioViewLogs.userId, userId))).limit(1)

  if (log?.longViewGranted) return c.json({ granted: false })

  if (log) {
    await db.update(portfolioViewLogs).set({ longViewGranted: true }).where(eq(portfolioViewLogs.id, log.id))
  } else {
    await db.insert(portfolioViewLogs).values({ id: randomUUID(), docId, userId, longViewGranted: true })
  }
  await db.update(portfolioDocuments)
    .set({ longViewCount: sql`${portfolioDocuments.longViewCount} + 1` })
    .where(eq(portfolioDocuments.id, docId))
  return c.json({ granted: true })
})

// POST /:doc_id/share-view
portfolioRouter.post('/:doc_id/share-view', authMiddleware, async (c) => {
  const docId = c.req.param('doc_id')
  const userId = c.get('userId')
  const db = createDb(c.env.DATABASE_URL)
  const [doc] = await db.select().from(portfolioDocuments).where(eq(portfolioDocuments.id, docId)).limit(1)
  if (!doc || !doc.isApproved || doc.uploaderId === userId) return c.json({ granted: false })

  const [log] = await db.select().from(portfolioViewLogs)
    .where(and(eq(portfolioViewLogs.docId, docId), eq(portfolioViewLogs.userId, userId))).limit(1)
  if (log?.shareRewardGranted) return c.json({ granted: false })

  if (log) {
    await db.update(portfolioViewLogs).set({ shareRewardGranted: true }).where(eq(portfolioViewLogs.id, log.id))
  } else {
    await db.insert(portfolioViewLogs).values({ id: randomUUID(), docId, userId, shareRewardGranted: true })
  }
  // Award +20 to uploader
  await db.execute(
    sql`UPDATE users SET reputation_score = reputation_score + 20 WHERE id = ${doc.uploaderId}`
  )
  await db.insert(auditLogs).values({
    id: randomUUID(), correlationId: randomUUID(), actorId: null,
    action: 'reputation.auto_awarded', targetType: 'user', targetId: doc.uploaderId,
    ip: 'system', metadata: { delta: 20, reason: 'share_view', docId },
  })
  return c.json({ granted: true })
})

// POST /:doc_id/heartbeat
portfolioRouter.post('/:doc_id/heartbeat', authMiddleware, async (c) => {
  const docId = c.req.param('doc_id')
  const userId = c.get('userId')
  const db = createDb(c.env.DATABASE_URL)
  const [doc] = await db.select().from(portfolioDocuments).where(eq(portfolioDocuments.id, docId)).limit(1)
  if (!doc || !doc.isApproved) return c.json({ message: 'ok' })

  const SESSION_BREAK = 120, GRACE = 600, REWARD_INTERVAL = 1800, CAP = 45
  const now = new Date()

  const [log] = await db.select().from(portfolioViewLogs)
    .where(and(eq(portfolioViewLogs.docId, docId), eq(portfolioViewLogs.userId, userId))).limit(1)

  if (!log) {
    await db.insert(portfolioViewLogs).values({
      id: randomUUID(), docId, userId,
      lastHeartbeatAt: now, sessionGraceRemainingS: GRACE,
    })
    return c.json({ message: 'ok' })
  }

  if (!log.lastHeartbeatAt) {
    await db.update(portfolioViewLogs).set({ lastHeartbeatAt: now }).where(eq(portfolioViewLogs.id, log.id))
    return c.json({ message: 'ok' })
  }

  const gap = (now.getTime() - log.lastHeartbeatAt.getTime()) / 1000
  let graceRemaining = gap > SESSION_BREAK ? GRACE : log.sessionGraceRemainingS
  const elapsed = Math.min(gap, CAP)
  const graceConsumed = Math.min(elapsed, graceRemaining)
  graceRemaining -= graceConsumed
  const effectiveThisTick = graceRemaining === 0 && graceConsumed < elapsed ? elapsed - graceConsumed : 0
  const newTotalEffective = log.totalEffectiveSeconds + Math.floor(effectiveThisTick)

  let newIntervalsGranted = log.reputationIntervalsGranted
  if (doc.uploaderId !== userId) {
    const newIntervals = Math.floor(newTotalEffective / REWARD_INTERVAL)
    if (newIntervals > log.reputationIntervalsGranted) {
      const delta = (newIntervals - log.reputationIntervalsGranted) * 10
      newIntervalsGranted = newIntervals
      await db.execute(sql`UPDATE users SET reputation_score = reputation_score + ${delta} WHERE id = ${doc.uploaderId}`)
      await db.insert(auditLogs).values({
        id: randomUUID(), correlationId: randomUUID(), actorId: null,
        action: 'reputation.auto_awarded', targetType: 'user', targetId: doc.uploaderId,
        ip: 'system', metadata: { delta, reason: 'reading_heartbeat', docId },
      })
    }
  }

  await db.update(portfolioViewLogs).set({
    lastHeartbeatAt: now,
    sessionGraceRemainingS: Math.floor(graceRemaining),
    totalEffectiveSeconds: newTotalEffective,
    reputationIntervalsGranted: newIntervalsGranted,
    updatedAt: now,
  }).where(eq(portfolioViewLogs.id, log.id))

  return c.json({ message: 'ok' })
})

// PATCH /:doc_id/approve
portfolioRouter.patch('/:doc_id/approve', authMiddleware, async (c) => {
  const docId = c.req.param('doc_id')
  const { approved } = await c.req.json()
  const db = createDb(c.env.DATABASE_URL)
  const [doc] = await db.select().from(portfolioDocuments).where(eq(portfolioDocuments.id, docId)).limit(1)
  if (!doc) return c.json({ detail: 'Not found' }, 404)
  await db.update(portfolioDocuments).set({ isApproved: !!approved, updatedAt: new Date() })
    .where(eq(portfolioDocuments.id, docId))
  const [updated] = await db.select().from(portfolioDocuments).where(eq(portfolioDocuments.id, docId)).limit(1)
  const [uploader] = await db.select().from(users).where(eq(users.id, updated.uploaderId)).limit(1)
  const current = currentCohort()
  const deptScore = await getDeptRuleScore(updated.schoolName, updated.deptName, db)
  const score = cohortScore(updated.admissionYear, current) + deptScore + viewScore(updated.viewCount, updated.longViewCount)
  return c.json(docOut(updated, uploader?.displayName ?? 'Unknown', score))
})

// POST /school-request
portfolioRouter.post('/school-request', authMiddleware, async (c) => {
  const userId = c.get('userId')
  const { schoolName, deptName, note } = await c.req.json()
  const db = createDb(c.env.DATABASE_URL)
  const id = randomUUID()
  await db.insert(portfolioSchoolRequests).values({
    id, requesterId: userId, schoolName, deptName, note: note ?? null,
  })
  const [sr] = await db.select().from(portfolioSchoolRequests).where(eq(portfolioSchoolRequests.id, id)).limit(1)
  const [user] = await db.select().from(users).where(eq(users.id, userId)).limit(1)
  return c.json({
    id: sr.id, requesterId: sr.requesterId, requesterName: user?.displayName ?? '',
    schoolName: sr.schoolName, deptName: sr.deptName, status: sr.status,
    note: sr.note, reviewNote: sr.reviewNote, createdAt: sr.createdAt.toISOString(),
  }, 201)
})

export default portfolioRouter
