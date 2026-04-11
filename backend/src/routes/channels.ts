import { Hono } from 'hono'
import { eq, and, ne, asc, gt } from 'drizzle-orm'
import type { Env } from '../types'
import { createDb } from '../db'
import { channels, messages, users, messageReactions } from '../db/schema'
import { authMiddleware } from '../middleware/auth'
import { randomUUID } from '../utils/uuid'
import { buildMessagesWithMeta } from './messages-helper'

const channelsRouter = new Hono<{ Bindings: Env }>()

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

// GET /
channelsRouter.get('/', authMiddleware, async (c) => {
  const db = createDb(c.env.DATABASE_URL)
  const all = await db.select().from(channels).orderBy(asc(channels.orderIndex))
  return c.json(all.map(channelOut))
})

// GET /:channel_id
channelsRouter.get('/:channel_id', authMiddleware, async (c) => {
  const channelId = c.req.param('channel_id')
  const db = createDb(c.env.DATABASE_URL)
  const [ch] = await db.select().from(channels).where(eq(channels.id, channelId)).limit(1)
  if (!ch) return c.json({ detail: 'Channel not found' }, 404)
  return c.json(channelOut(ch))
})

// GET /:channel_id/messages
channelsRouter.get('/:channel_id/messages', authMiddleware, async (c) => {
  const channelId = c.req.param('channel_id')
  const cursor = c.req.query('cursor')
  const limit = Math.min(parseInt(c.req.query('limit') ?? '50'), 100)
  const db = createDb(c.env.DATABASE_URL)

  let query = db.select().from(messages)
    .where(and(eq(messages.channelId, channelId), ne(messages.status, 'deleted')))
    .orderBy(asc(messages.createdAt))
    .limit(limit + 1)

  if (cursor) {
    query = db.select().from(messages)
      .where(and(
        eq(messages.channelId, channelId),
        ne(messages.status, 'deleted'),
        gt(messages.createdAt, new Date(cursor)),
      ))
      .orderBy(asc(messages.createdAt))
      .limit(limit + 1)
  }

  const rows = await query
  const hasMore = rows.length > limit
  const msgList = hasMore ? rows.slice(0, limit) : rows
  const nextCursor = hasMore && msgList.length > 0
    ? msgList[msgList.length - 1].createdAt.toISOString()
    : null

  const out = await buildMessagesWithMeta(msgList, db)
  return c.json({ messages: out, hasMore, nextCursor })
})

// GET /:channel_id/pinned
channelsRouter.get('/:channel_id/pinned', authMiddleware, async (c) => {
  const channelId = c.req.param('channel_id')
  const db = createDb(c.env.DATABASE_URL)
  const pinned = await db.select().from(messages)
    .where(and(eq(messages.channelId, channelId), eq(messages.isPinned, true), ne(messages.status, 'deleted')))
  const out = await buildMessagesWithMeta(pinned, db)
  return c.json(out)
})

// POST /:channel_id/messages
channelsRouter.post('/:channel_id/messages', authMiddleware, async (c) => {
  const channelId = c.req.param('channel_id')
  const userId = c.get('userId')
  const { content, replyToId } = await c.req.json()
  if (!content?.trim()) return c.json({ detail: 'Content is required' }, 422)

  const db = createDb(c.env.DATABASE_URL)
  const id = randomUUID()
  await db.insert(messages).values({
    id, channelId, authorId: userId, content, replyToId: replyToId ?? null,
  })
  const [msg] = await db.select().from(messages).where(eq(messages.id, id)).limit(1)
  const [out] = await buildMessagesWithMeta([msg], db)

  // Broadcast via Durable Object
  try {
    const doId = c.env.CHAT_ROOM.idFromName('global')
    const stub = c.env.CHAT_ROOM.get(doId)
    await stub.fetch(new Request('https://internal/broadcast', {
      method: 'POST',
      body: JSON.stringify({ channelId, event: { type: 'new_message', message: out } }),
    }))
  } catch { /* best-effort */ }

  return c.json(out, 201)
})

export { channelOut }
export default channelsRouter
