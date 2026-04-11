import { Hono } from 'hono'
import { eq, and, desc } from 'drizzle-orm'
import type { Env } from '../types'
import { createDb } from '../db'
import { tickets, ticketMessages, users } from '../db/schema'
import { authMiddleware } from '../middleware/auth'
import { randomUUID } from '../utils/uuid'

const ticketsRouter = new Hono<{ Bindings: Env }>()

async function ticketOut(ticket: typeof tickets.$inferSelect, db: ReturnType<typeof createDb>) {
  const [submitter] = await db.select().from(users).where(eq(users.id, ticket.userId)).limit(1)
  const [assignee] = ticket.assigneeId
    ? await db.select().from(users).where(eq(users.id, ticket.assigneeId)).limit(1)
    : [null]
  const msgs = await db.select().from(ticketMessages)
    .where(eq(ticketMessages.ticketId, ticket.id))
    .orderBy(ticketMessages.createdAt)
  const authorIds = [...new Set(msgs.map(m => m.authorId))]
  const authorList = await db.select().from(users)
    .where(authorIds.length > 0 ? eq(users.id, authorIds[0]) : eq(users.id, ''))
  const authorsMap = Object.fromEntries(authorList.map(u => [u.id, u]))

  return {
    id: ticket.id, userId: ticket.userId,
    userDisplayName: submitter?.displayName ?? '',
    category: ticket.category, subject: ticket.subject, status: ticket.status,
    assigneeId: ticket.assigneeId, assigneeDisplayName: assignee?.displayName ?? null,
    createdAt: ticket.createdAt.toISOString(), updatedAt: ticket.updatedAt.toISOString(),
    messages: msgs.map(m => ({
      id: m.id, ticketId: m.ticketId, authorId: m.authorId,
      authorDisplayName: authorsMap[m.authorId]?.displayName ?? '',
      content: m.content, isStaff: m.isStaff,
      createdAt: m.createdAt.toISOString(),
    })),
  }
}

// GET /
ticketsRouter.get('/', authMiddleware, async (c) => {
  const userId = c.get('userId')
  const db = createDb(c.env.DATABASE_URL)
  const myTickets = await db.select().from(tickets)
    .where(eq(tickets.userId, userId)).orderBy(desc(tickets.updatedAt))
  return c.json(await Promise.all(myTickets.map(t => ticketOut(t, db))))
})

// POST /
ticketsRouter.post('/', authMiddleware, async (c) => {
  const userId = c.get('userId')
  const { category, subject, message } = await c.req.json()
  if (!category || !subject || !message) return c.json({ detail: '缺少必要欄位' }, 422)

  const db = createDb(c.env.DATABASE_URL)
  const ticketId = randomUUID()
  await db.insert(tickets).values({ id: ticketId, userId, category, subject })
  await db.insert(ticketMessages).values({
    id: randomUUID(), ticketId, authorId: userId, content: message, isStaff: false,
  })
  const [ticket] = await db.select().from(tickets).where(eq(tickets.id, ticketId)).limit(1)
  return c.json(await ticketOut(ticket, db), 201)
})

// GET /:ticket_id
ticketsRouter.get('/:ticket_id', authMiddleware, async (c) => {
  const ticketId = c.req.param('ticket_id')
  const userId = c.get('userId')
  const db = createDb(c.env.DATABASE_URL)
  const [ticket] = await db.select().from(tickets).where(eq(tickets.id, ticketId)).limit(1)
  if (!ticket || ticket.userId !== userId) return c.json({ detail: 'Ticket not found' }, 404)
  return c.json(await ticketOut(ticket, db))
})

// POST /:ticket_id/messages
ticketsRouter.post('/:ticket_id/messages', authMiddleware, async (c) => {
  const ticketId = c.req.param('ticket_id')
  const userId = c.get('userId')
  const { content } = await c.req.json()
  const db = createDb(c.env.DATABASE_URL)
  const [ticket] = await db.select().from(tickets).where(eq(tickets.id, ticketId)).limit(1)
  if (!ticket || ticket.userId !== userId) return c.json({ detail: 'Ticket not found' }, 404)
  await db.insert(ticketMessages).values({
    id: randomUUID(), ticketId, authorId: userId, content, isStaff: false,
  })
  if (ticket.status === 'closed') {
    await db.update(tickets).set({ status: 'open', updatedAt: new Date() }).where(eq(tickets.id, ticketId))
  } else {
    await db.update(tickets).set({ updatedAt: new Date() }).where(eq(tickets.id, ticketId))
  }
  const [updated] = await db.select().from(tickets).where(eq(tickets.id, ticketId)).limit(1)
  return c.json(await ticketOut(updated, db), 201)
})

// GET /admin/all
ticketsRouter.get('/admin/all', authMiddleware, async (c) => {
  const status = c.req.query('status')
  const db = createDb(c.env.DATABASE_URL)
  let query = db.select().from(tickets).orderBy(desc(tickets.updatedAt))
  if (status) {
    query = db.select().from(tickets)
      .where(eq(tickets.status, status as 'open' | 'processing' | 'pending' | 'closed'))
      .orderBy(desc(tickets.updatedAt))
  }
  const all = await query
  return c.json(await Promise.all(all.map(t => ticketOut(t, db))))
})

// PATCH /admin/:ticket_id
ticketsRouter.patch('/admin/:ticket_id', authMiddleware, async (c) => {
  const ticketId = c.req.param('ticket_id')
  const { status, assigneeId, message } = await c.req.json()
  const userId = c.get('userId')
  const db = createDb(c.env.DATABASE_URL)
  const [ticket] = await db.select().from(tickets).where(eq(tickets.id, ticketId)).limit(1)
  if (!ticket) return c.json({ detail: 'Ticket not found' }, 404)

  const updates: Partial<typeof tickets.$inferInsert> = { updatedAt: new Date() }
  if (status) updates.status = status
  if (assigneeId !== undefined) updates.assigneeId = assigneeId
  await db.update(tickets).set(updates).where(eq(tickets.id, ticketId))

  if (message) {
    await db.insert(ticketMessages).values({
      id: randomUUID(), ticketId, authorId: userId, content: message, isStaff: true,
    })
  }

  const [updated] = await db.select().from(tickets).where(eq(tickets.id, ticketId)).limit(1)
  return c.json(await ticketOut(updated, db))
})

export default ticketsRouter
