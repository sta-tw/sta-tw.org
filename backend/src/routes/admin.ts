import { Hono } from 'hono'
import { eq, ilike, or, desc, asc, sql, and, ne } from 'drizzle-orm'
import type { Env } from '../types'
import { createDb } from '../db'
import {
  users, channels, messages, tickets, verificationRequests, auditLogs,
  portfolioScoringRules, portfolioSchoolOptions, portfolioSchoolRequests,
  userSessions,
} from '../db/schema'
import { authMiddleware, getClientIp } from '../middleware/auth'
import { randomUUID } from '../utils/uuid'

const adminRouter = new Hono<{ Bindings: Env }>()

function requireStaff(role: string) {
  if (role !== 'admin' && role !== 'developer' && role !== 'super_admin') {
    return false
  }
  return true
}

function userAdminOut(u: typeof users.$inferSelect) {
  return {
    id: u.id, username: u.username, email: u.email,
    displayName: u.displayName, avatarUrl: u.avatarUrl,
    role: u.role, verificationStatus: u.verificationStatus,
    reputationScore: u.reputationScore, isActive: u.isActive,
    isEmailVerified: u.isEmailVerified,
    managedSchoolCode: u.managedSchoolCode, managedDeptName: u.managedDeptName,
    createdAt: u.createdAt.toISOString(),
  }
}

function channelOut(ch: typeof channels.$inferSelect) {
  return {
    id: ch.id, name: ch.name, description: ch.description,
    type: ch.type, scopeType: ch.scopeType,
    schoolCode: ch.schoolCode, deptCode: ch.deptCode,
    parentId: ch.parentId, isArchived: ch.isArchived,
    cohortYear: ch.cohortYear, audience: ch.audience,
    unreadCount: 0, order: ch.orderIndex,
  }
}

// Middleware: check staff role
adminRouter.use('*', authMiddleware, async (c, next) => {
  const userId = c.get('userId')
  const db = createDb(c.env.DATABASE_URL)
  const [user] = await db.select().from(users).where(eq(users.id, userId)).limit(1)
  if (!user || !requireStaff(user.role)) {
    return c.json({ detail: 'Insufficient role' }, 403)
  }
  await next()
})

// GET /stats
adminRouter.get('/stats', async (c) => {
  const db = createDb(c.env.DATABASE_URL)

  const userRoles = await db.select({ role: users.role, cnt: sql<number>`count(*)` })
    .from(users).groupBy(users.role)
  const byRole: Record<string, number> = {}
  let totalUsers = 0
  for (const r of userRoles) {
    byRole[r.role] = Number(r.cnt)
    totalUsers += Number(r.cnt)
  }

  const [{ activeTickets }] = await db.select({ activeTickets: sql<number>`count(*)` })
    .from(tickets).where(ne(tickets.status, 'closed'))
  const [{ pendingVer }] = await db.select({ pendingVer: sql<number>`count(*)` })
    .from(verificationRequests).where(eq(verificationRequests.status, 'pending'))
  const [{ totalChannels }] = await db.select({ totalChannels: sql<number>`count(*)` }).from(channels)
  const since = new Date(Date.now() - 86400000)
  const [{ msgCount }] = await db.select({ msgCount: sql<number>`count(*)` })
    .from(messages).where(sql`${messages.createdAt} >= ${since}`)

  return c.json({
    totalUsers, usersByRole: byRole,
    activeTickets: Number(activeTickets),
    pendingVerifications: Number(pendingVer),
    totalChannels: Number(totalChannels),
    messagesLast24h: Number(msgCount),
  })
})

// GET /users
adminRouter.get('/users', async (c) => {
  const q = c.req.query('q')
  const role = c.req.query('role')
  const page = parseInt(c.req.query('page') ?? '1')
  const pageSize = Math.min(parseInt(c.req.query('page_size') ?? '20'), 100)
  const db = createDb(c.env.DATABASE_URL)

  const conditions = []
  if (q) conditions.push(or(ilike(users.username, `%${q}%`), ilike(users.email, `%${q}%`), ilike(users.displayName, `%${q}%`)))
  if (role) conditions.push(eq(users.role, role as typeof users.role._.data))

  const [{ total }] = await db.select({ total: sql<number>`count(*)` }).from(users)
    .where(conditions.length ? and(...conditions) : undefined)
  const items = await db.select().from(users)
    .where(conditions.length ? and(...conditions) : undefined)
    .orderBy(desc(users.createdAt))
    .limit(pageSize).offset((page - 1) * pageSize)

  return c.json({ items: items.map(userAdminOut), total: Number(total), page, pageSize })
})

// GET /users/:user_id
adminRouter.get('/users/:user_id', async (c) => {
  const db = createDb(c.env.DATABASE_URL)
  const [user] = await db.select().from(users).where(eq(users.id, c.req.param('user_id'))).limit(1)
  if (!user) return c.json({ detail: 'User not found' }, 404)
  return c.json(userAdminOut(user))
})

// PATCH /users/:user_id
adminRouter.patch('/users/:user_id', async (c) => {
  const { role, isActive, managedSchoolCode, managedDeptName } = await c.req.json()
  const db = createDb(c.env.DATABASE_URL)
  const [user] = await db.select().from(users).where(eq(users.id, c.req.param('user_id'))).limit(1)
  if (!user) return c.json({ detail: 'User not found' }, 404)
  const updates: Partial<typeof users.$inferInsert> = {}
  if (role !== undefined) updates.role = role
  if (isActive !== undefined) updates.isActive = isActive
  if (managedSchoolCode !== undefined) updates.managedSchoolCode = managedSchoolCode || null
  if (managedDeptName !== undefined) updates.managedDeptName = managedDeptName || null
  await db.update(users).set(updates).where(eq(users.id, c.req.param('user_id')))
  const [updated] = await db.select().from(users).where(eq(users.id, c.req.param('user_id'))).limit(1)
  return c.json(userAdminOut(updated))
})

// POST /users/:user_id/force-logout
adminRouter.post('/users/:user_id/force-logout', async (c) => {
  const db = createDb(c.env.DATABASE_URL)
  await db.update(userSessions).set({ isRevoked: true })
    .where(and(eq(userSessions.userId, c.req.param('user_id')), eq(userSessions.isRevoked, false)))
  return c.body(null, 204)
})

// GET /channels
adminRouter.get('/channels', async (c) => {
  const db = createDb(c.env.DATABASE_URL)
  const all = await db.select().from(channels).orderBy(asc(channels.orderIndex))
  return c.json(all.map(channelOut))
})

// POST /channels
adminRouter.post('/channels', async (c) => {
  const body = await c.req.json()
  const db = createDb(c.env.DATABASE_URL)
  const id = randomUUID()
  await db.insert(channels).values({
    id, name: body.name, description: body.description ?? null,
    type: body.type ?? 'text', scopeType: body.scopeType ?? 'global',
    schoolCode: body.schoolCode ?? null, deptCode: body.deptCode ?? null,
    parentId: body.parentId ?? null, cohortYear: body.cohortYear ?? null,
    audience: body.audience ?? null, orderIndex: body.order ?? 0,
  })
  const [ch] = await db.select().from(channels).where(eq(channels.id, id)).limit(1)
  return c.json(channelOut(ch), 201)
})

// PATCH /channels/:channel_id
adminRouter.patch('/channels/:channel_id', async (c) => {
  const body = await c.req.json()
  const db = createDb(c.env.DATABASE_URL)
  const [ch] = await db.select().from(channels).where(eq(channels.id, c.req.param('channel_id'))).limit(1)
  if (!ch) return c.json({ detail: 'Channel not found' }, 404)
  const updates: Partial<typeof channels.$inferInsert> = {}
  if (body.name !== undefined) updates.name = body.name
  if (body.description !== undefined) updates.description = body.description
  if (body.type !== undefined) updates.type = body.type
  if (body.scopeType !== undefined) updates.scopeType = body.scopeType
  if (body.schoolCode !== undefined) updates.schoolCode = body.schoolCode
  if (body.deptCode !== undefined) updates.deptCode = body.deptCode
  if (body.parentId !== undefined) updates.parentId = body.parentId
  if (body.isArchived !== undefined) updates.isArchived = body.isArchived
  if (body.cohortYear !== undefined) updates.cohortYear = body.cohortYear
  if (body.audience !== undefined) updates.audience = body.audience
  if (body.order !== undefined) updates.orderIndex = body.order
  await db.update(channels).set(updates).where(eq(channels.id, c.req.param('channel_id')))
  const [updated] = await db.select().from(channels).where(eq(channels.id, c.req.param('channel_id'))).limit(1)
  return c.json(channelOut(updated))
})

// DELETE /channels/:channel_id
adminRouter.delete('/channels/:channel_id', async (c) => {
  const db = createDb(c.env.DATABASE_URL)
  const [ch] = await db.select().from(channels).where(eq(channels.id, c.req.param('channel_id'))).limit(1)
  if (!ch) return c.json({ detail: 'Channel not found' }, 404)
  await db.delete(channels).where(eq(channels.id, c.req.param('channel_id')))
  return c.body(null, 204)
})

// GET /audit-log
adminRouter.get('/audit-log', async (c) => {
  const action = c.req.query('action')
  const actorId = c.req.query('actor_id')
  const correlationId = c.req.query('correlation_id')
  const page = parseInt(c.req.query('page') ?? '1')
  const pageSize = Math.min(parseInt(c.req.query('page_size') ?? '50'), 200)
  const db = createDb(c.env.DATABASE_URL)

  const conditions = []
  if (action) conditions.push(ilike(auditLogs.action, `%${action}%`))
  if (actorId) conditions.push(eq(auditLogs.actorId, actorId))
  if (correlationId) conditions.push(eq(auditLogs.correlationId, correlationId))

  const [{ total }] = await db.select({ total: sql<number>`count(*)` }).from(auditLogs)
    .where(conditions.length ? and(...conditions) : undefined)
  const rows = await db.select({
    log: auditLogs, actorName: users.displayName,
  }).from(auditLogs)
    .leftJoin(users, eq(auditLogs.actorId, users.id))
    .where(conditions.length ? and(...conditions) : undefined)
    .orderBy(desc(auditLogs.createdAt))
    .limit(pageSize).offset((page - 1) * pageSize)

  return c.json({
    items: rows.map(r => ({
      id: r.log.id, correlationId: r.log.correlationId,
      actorId: r.log.actorId, actorDisplayName: r.actorName ?? null,
      action: r.log.action, targetType: r.log.targetType, targetId: r.log.targetId,
      ip: r.log.ip, userAgent: r.log.userAgent, createdAt: r.log.createdAt.toISOString(),
    })),
    total: Number(total), page, pageSize,
  })
})

// GET /audit-log/export
adminRouter.get('/audit-log/export', async (c) => {
  const action = c.req.query('action')
  const db = createDb(c.env.DATABASE_URL)
  const rows = await db.select({ log: auditLogs, actorName: users.displayName })
    .from(auditLogs).leftJoin(users, eq(auditLogs.actorId, users.id))
    .where(action ? ilike(auditLogs.action, `%${action}%`) : undefined)
    .orderBy(desc(auditLogs.createdAt)).limit(10000)

  const lines = ['id,correlationId,actorId,actor,action,targetType,targetId,ip,createdAt']
  for (const r of rows) {
    lines.push([
      r.log.id, r.log.correlationId, r.log.actorId ?? '',
      r.actorName ?? '', r.log.action, r.log.targetType ?? '',
      r.log.targetId ?? '', r.log.ip, r.log.createdAt.toISOString(),
    ].map(v => `"${String(v).replace(/"/g, '""')}"`).join(','))
  }

  return new Response(lines.join('\n'), {
    headers: {
      'Content-Type': 'text/csv; charset=utf-8',
      'Content-Disposition': 'attachment; filename=audit-log.csv',
    },
  })
})

// GET /reputation/:user_id
adminRouter.get('/reputation/:user_id', async (c) => {
  const userId = c.req.param('user_id')
  const db = createDb(c.env.DATABASE_URL)
  const [target] = await db.select().from(users).where(eq(users.id, userId)).limit(1)
  if (!target) return c.json({ detail: 'User not found' }, 404)

  const rows = await db.select({ log: auditLogs, actorName: users.displayName })
    .from(auditLogs).leftJoin(users, eq(auditLogs.actorId, users.id))
    .where(and(eq(auditLogs.action, 'reputation.adjusted'), eq(auditLogs.targetId, userId)))
    .orderBy(desc(auditLogs.createdAt)).limit(200)

  return c.json({
    userId: target.id, username: target.username, displayName: target.displayName,
    currentScore: target.reputationScore,
    events: rows.map(r => ({
      id: r.log.id, userId,
      delta: (r.log.metadata as Record<string, number> | null)?.delta ?? 0,
      reason: (r.log.metadata as Record<string, string> | null)?.reason ?? '',
      createdAt: r.log.createdAt.toISOString(),
      actorId: r.log.actorId, actorDisplayName: r.actorName ?? null,
    })),
  })
})

// POST /reputation/:user_id/adjust
adminRouter.post('/reputation/:user_id/adjust', async (c) => {
  const userId = c.req.param('user_id')
  const actorId = c.get('userId')
  const { delta, reason } = await c.req.json()
  if (!delta) return c.json({ detail: 'delta must not be 0' }, 422)
  if (!reason?.trim()) return c.json({ detail: 'reason is required' }, 422)
  const db = createDb(c.env.DATABASE_URL)
  const [target] = await db.select().from(users).where(eq(users.id, userId)).limit(1)
  if (!target) return c.json({ detail: 'User not found' }, 404)

  const before = target.reputationScore
  await db.update(users).set({ reputationScore: before + delta }).where(eq(users.id, userId))
  await db.insert(auditLogs).values({
    id: randomUUID(), correlationId: randomUUID(),
    actorId, action: 'reputation.adjusted', targetType: 'user', targetId: userId,
    ip: getClientIp(c.req.raw),
    metadata: { delta, reason: reason.trim(), before, after: before + delta },
  })

  const [updated] = await db.select().from(users).where(eq(users.id, userId)).limit(1)
  const rows = await db.select({ log: auditLogs, actorName: users.displayName })
    .from(auditLogs).leftJoin(users, eq(auditLogs.actorId, users.id))
    .where(and(eq(auditLogs.action, 'reputation.adjusted'), eq(auditLogs.targetId, userId)))
    .orderBy(desc(auditLogs.createdAt)).limit(200)

  return c.json({
    userId: updated.id, username: updated.username, displayName: updated.displayName,
    currentScore: updated.reputationScore,
    events: rows.map(r => ({
      id: r.log.id, userId,
      delta: (r.log.metadata as Record<string, number> | null)?.delta ?? 0,
      reason: (r.log.metadata as Record<string, string> | null)?.reason ?? '',
      createdAt: r.log.createdAt.toISOString(),
      actorId: r.log.actorId, actorDisplayName: r.actorName ?? null,
    })),
  })
})

// ── Portfolio Rules ────────────────────────────────────────────

adminRouter.get('/portfolio-rules', async (c) => {
  const db = createDb(c.env.DATABASE_URL)
  const rules = await db.select().from(portfolioScoringRules)
    .orderBy(asc(portfolioScoringRules.schoolName), asc(portfolioScoringRules.deptName))
  return c.json(rules.map(r => ({
    id: r.id, schoolName: r.schoolName, schoolAbbr: r.schoolAbbr,
    deptName: r.deptName, score: r.score, note: r.note,
    createdAt: r.createdAt.toISOString(), updatedAt: r.updatedAt.toISOString(),
  })))
})

adminRouter.post('/portfolio-rules', async (c) => {
  const { schoolName, schoolAbbr, deptName, score, note } = await c.req.json()
  const db = createDb(c.env.DATABASE_URL)
  const id = randomUUID()
  await db.insert(portfolioScoringRules).values({ id, schoolName, schoolAbbr: schoolAbbr ?? null, deptName, score, note: note ?? null })
  const [rule] = await db.select().from(portfolioScoringRules).where(eq(portfolioScoringRules.id, id)).limit(1)
  return c.json({ id: rule.id, schoolName: rule.schoolName, schoolAbbr: rule.schoolAbbr, deptName: rule.deptName, score: rule.score, note: rule.note, createdAt: rule.createdAt.toISOString(), updatedAt: rule.updatedAt.toISOString() }, 201)
})

adminRouter.get('/portfolio-rules/export', async (c) => {
  const db = createDb(c.env.DATABASE_URL)
  const rules = await db.select().from(portfolioScoringRules)
    .orderBy(asc(portfolioScoringRules.schoolName), asc(portfolioScoringRules.deptName))
  const lines = ['schoolName,schoolAbbr,deptName,score,note']
  for (const r of rules) {
    lines.push([r.schoolName, r.schoolAbbr ?? '', r.deptName, r.score, r.note ?? '']
      .map(v => `"${String(v).replace(/"/g, '""')}"`).join(','))
  }
  return new Response('\uFEFF' + lines.join('\n'), {
    headers: { 'Content-Type': 'text/csv; charset=utf-8', 'Content-Disposition': 'attachment; filename=portfolio-rules.csv' },
  })
})

adminRouter.post('/portfolio-rules/import', async (c) => {
  const { rows } = await c.req.json()
  if (!rows?.length) return c.json({ detail: 'rows is empty' }, 422)
  const db = createDb(c.env.DATABASE_URL)
  let created = 0, updated = 0
  const errors: string[] = []
  for (let i = 0; i < rows.length; i++) {
    try {
      const row = rows[i]
      const [existing] = await db.select().from(portfolioScoringRules)
        .where(and(eq(portfolioScoringRules.schoolName, row.schoolName), eq(portfolioScoringRules.deptName, row.deptName))).limit(1)
      if (existing) {
        await db.update(portfolioScoringRules).set({ schoolAbbr: row.schoolAbbr ?? null, score: row.score, note: row.note ?? null, updatedAt: new Date() }).where(eq(portfolioScoringRules.id, existing.id))
        updated++
      } else {
        await db.insert(portfolioScoringRules).values({ id: randomUUID(), schoolName: row.schoolName, schoolAbbr: row.schoolAbbr ?? null, deptName: row.deptName, score: row.score, note: row.note ?? null })
        created++
      }
    } catch (e) {
      errors.push(`第 ${i + 1} 筆：${e}`)
    }
  }
  return c.json({ created, updated, errors })
})

adminRouter.patch('/portfolio-rules/:rule_id', async (c) => {
  const { schoolName, schoolAbbr, deptName, score, note } = await c.req.json()
  const db = createDb(c.env.DATABASE_URL)
  const [rule] = await db.select().from(portfolioScoringRules).where(eq(portfolioScoringRules.id, c.req.param('rule_id'))).limit(1)
  if (!rule) return c.json({ detail: 'Rule not found' }, 404)
  const updates: Partial<typeof portfolioScoringRules.$inferInsert> = { updatedAt: new Date() }
  if (schoolName !== undefined) updates.schoolName = schoolName
  if (schoolAbbr !== undefined) updates.schoolAbbr = schoolAbbr || null
  if (deptName !== undefined) updates.deptName = deptName
  if (score !== undefined) updates.score = score
  if (note !== undefined) updates.note = note
  await db.update(portfolioScoringRules).set(updates).where(eq(portfolioScoringRules.id, c.req.param('rule_id')))
  const [updated] = await db.select().from(portfolioScoringRules).where(eq(portfolioScoringRules.id, c.req.param('rule_id'))).limit(1)
  return c.json({ id: updated.id, schoolName: updated.schoolName, schoolAbbr: updated.schoolAbbr, deptName: updated.deptName, score: updated.score, note: updated.note, createdAt: updated.createdAt.toISOString(), updatedAt: updated.updatedAt.toISOString() })
})

adminRouter.delete('/portfolio-rules/:rule_id', async (c) => {
  const db = createDb(c.env.DATABASE_URL)
  const [rule] = await db.select().from(portfolioScoringRules).where(eq(portfolioScoringRules.id, c.req.param('rule_id'))).limit(1)
  if (!rule) return c.json({ detail: 'Rule not found' }, 404)
  await db.delete(portfolioScoringRules).where(eq(portfolioScoringRules.id, c.req.param('rule_id')))
  return c.body(null, 204)
})

// ── School Options ─────────────────────────────────────────────

adminRouter.get('/school-options', async (c) => {
  const db = createDb(c.env.DATABASE_URL)
  const opts = await db.select().from(portfolioSchoolOptions)
    .where(eq(portfolioSchoolOptions.isActive, true))
    .orderBy(asc(portfolioSchoolOptions.sortOrder), asc(portfolioSchoolOptions.schoolName), asc(portfolioSchoolOptions.deptName))
  return c.json(opts.map(o => ({ id: o.id, schoolName: o.schoolName, schoolCode: o.schoolCode, deptName: o.deptName, sortOrder: o.sortOrder })))
})

adminRouter.post('/school-options', async (c) => {
  const { schoolName, schoolCode, deptName, sortOrder } = await c.req.json()
  const db = createDb(c.env.DATABASE_URL)
  const id = randomUUID()
  await db.insert(portfolioSchoolOptions).values({ id, schoolName, schoolCode: schoolCode ?? null, deptName, sortOrder: sortOrder ?? 0 })
  const [opt] = await db.select().from(portfolioSchoolOptions).where(eq(portfolioSchoolOptions.id, id)).limit(1)
  return c.json({ id: opt.id, schoolName: opt.schoolName, schoolCode: opt.schoolCode, deptName: opt.deptName, sortOrder: opt.sortOrder }, 201)
})

adminRouter.delete('/school-options/:option_id', async (c) => {
  const db = createDb(c.env.DATABASE_URL)
  const [opt] = await db.select().from(portfolioSchoolOptions).where(eq(portfolioSchoolOptions.id, c.req.param('option_id'))).limit(1)
  if (!opt) return c.json({ detail: 'Option not found' }, 404)
  await db.delete(portfolioSchoolOptions).where(eq(portfolioSchoolOptions.id, c.req.param('option_id')))
  return c.body(null, 204)
})

// ── School Requests ────────────────────────────────────────────

adminRouter.get('/school-requests', async (c) => {
  const status = c.req.query('status')
  const db = createDb(c.env.DATABASE_URL)
  const rows = status
    ? await db.select().from(portfolioSchoolRequests).where(eq(portfolioSchoolRequests.status, status)).orderBy(desc(portfolioSchoolRequests.createdAt))
    : await db.select().from(portfolioSchoolRequests).orderBy(desc(portfolioSchoolRequests.createdAt))

  const requesterIds = [...new Set(rows.map(r => r.requesterId))]
  const requesterList = requesterIds.length > 0 ? await db.select().from(users) : []
  const reqMap = Object.fromEntries(requesterList.map(u => [u.id, u.displayName]))

  return c.json(rows.map(r => ({
    id: r.id, requesterId: r.requesterId, requesterName: reqMap[r.requesterId] ?? '',
    schoolName: r.schoolName, deptName: r.deptName, status: r.status,
    note: r.note, reviewNote: r.reviewNote, createdAt: r.createdAt.toISOString(),
  })))
})

adminRouter.patch('/school-requests/:request_id', async (c) => {
  const { status, reviewNote, addToOptions } = await c.req.json()
  const reviewerId = c.get('userId')
  const db = createDb(c.env.DATABASE_URL)
  const [sr] = await db.select().from(portfolioSchoolRequests).where(eq(portfolioSchoolRequests.id, c.req.param('request_id'))).limit(1)
  if (!sr) return c.json({ detail: 'Request not found' }, 404)

  await db.update(portfolioSchoolRequests).set({ status, reviewNote: reviewNote ?? null, reviewedBy: reviewerId, updatedAt: new Date() }).where(eq(portfolioSchoolRequests.id, c.req.param('request_id')))

  if (status === 'approved' && addToOptions) {
    const [existing] = await db.select().from(portfolioSchoolOptions)
      .where(and(eq(portfolioSchoolOptions.schoolName, sr.schoolName), eq(portfolioSchoolOptions.deptName, sr.deptName))).limit(1)
    if (!existing) {
      await db.insert(portfolioSchoolOptions).values({ id: randomUUID(), schoolName: sr.schoolName, deptName: sr.deptName })
    }
  }

  const [updated] = await db.select().from(portfolioSchoolRequests).where(eq(portfolioSchoolRequests.id, c.req.param('request_id'))).limit(1)
  const [requester] = await db.select().from(users).where(eq(users.id, sr.requesterId)).limit(1)
  return c.json({
    id: updated.id, requesterId: updated.requesterId, requesterName: requester?.displayName ?? '',
    schoolName: updated.schoolName, deptName: updated.deptName, status: updated.status,
    note: updated.note, reviewNote: updated.reviewNote, createdAt: updated.createdAt.toISOString(),
  })
})

export default adminRouter
