import { Hono } from 'hono'
import { eq, and, ne, asc } from 'drizzle-orm'
import type { Env } from '../types'
import { createDb } from '../db'
import { messages, messageReactions } from '../db/schema'
import { authMiddleware } from '../middleware/auth'
import { buildMessagesWithMeta } from './messages-helper'
import { randomUUID } from '../utils/uuid'

const messagesRouter = new Hono<{ Bindings: Env }>()

const ALLOWED_EMOJIS = new Set(['👍', '❤️', '😂', '😮', '😢', '🎉', '🔥', '✅', '👏', '🙏', '💯', '👋'])

async function broadcastToChannel(env: Env, channelId: string, event: unknown) {
  try {
    const doId = env.CHAT_ROOM.idFromName('global')
    const stub = env.CHAT_ROOM.get(doId)
    await stub.fetch(new Request('https://internal/broadcast', {
      method: 'POST',
      body: JSON.stringify({ channelId, event }),
    }))
  } catch { /* best-effort */ }
}

// PATCH /:message_id
messagesRouter.patch('/:message_id', authMiddleware, async (c) => {
  const msgId = c.req.param('message_id')
  const userId = c.get('userId')
  const { content } = await c.req.json()
  const db = createDb(c.env.DATABASE_URL)
  const [msg] = await db.select().from(messages).where(eq(messages.id, msgId)).limit(1)
  if (!msg) return c.json({ detail: 'Message not found' }, 404)
  if (msg.authorId !== userId) return c.json({ detail: 'Cannot edit another user\'s message' }, 403)
  if (msg.status !== 'active') return c.json({ detail: 'Cannot edit this message' }, 400)

  await db.update(messages).set({ content, isEdited: true, updatedAt: new Date() }).where(eq(messages.id, msgId))
  const [updated] = await db.select().from(messages).where(eq(messages.id, msgId)).limit(1)
  const [out] = await buildMessagesWithMeta([updated], db)
  await broadcastToChannel(c.env, msg.channelId, { type: 'message_updated', message: out })
  return c.json(out)
})

// POST /:message_id/withdraw
messagesRouter.post('/:message_id/withdraw', authMiddleware, async (c) => {
  const msgId = c.req.param('message_id')
  const userId = c.get('userId')
  const db = createDb(c.env.DATABASE_URL)
  const [msg] = await db.select().from(messages).where(eq(messages.id, msgId)).limit(1)
  if (!msg) return c.json({ detail: 'Message not found' }, 404)
  if (msg.authorId !== userId) return c.json({ detail: 'Permission denied' }, 403)
  await db.update(messages).set({ status: 'withdrawn' }).where(eq(messages.id, msgId))
  await broadcastToChannel(c.env, msg.channelId, { type: 'message_withdrawn', messageId: msgId })
  return c.json({ message: 'ok' })
})

// DELETE /:message_id
messagesRouter.delete('/:message_id', authMiddleware, async (c) => {
  const msgId = c.req.param('message_id')
  const userId = c.get('userId')
  const db = createDb(c.env.DATABASE_URL)
  const [msg] = await db.select().from(messages).where(eq(messages.id, msgId)).limit(1)
  if (!msg) return c.json({ detail: 'Message not found' }, 404)
  if (msg.authorId !== userId) return c.json({ detail: 'Permission denied' }, 403)
  await db.update(messages).set({ status: 'deleted' }).where(eq(messages.id, msgId))
  await broadcastToChannel(c.env, msg.channelId, { type: 'message_deleted', messageId: msgId })
  return c.json({ message: 'ok' })
})

// POST /:message_id/reactions
messagesRouter.post('/:message_id/reactions', authMiddleware, async (c) => {
  const msgId = c.req.param('message_id')
  const userId = c.get('userId')
  const { emoji } = await c.req.json()
  if (!ALLOWED_EMOJIS.has(emoji)) return c.json({ detail: 'Emoji not allowed' }, 400)

  const db = createDb(c.env.DATABASE_URL)
  const [existing] = await db.select().from(messageReactions)
    .where(and(
      eq(messageReactions.messageId, msgId),
      eq(messageReactions.userId, userId),
      eq(messageReactions.emoji, emoji),
    )).limit(1)

  if (existing) {
    await db.delete(messageReactions).where(eq(messageReactions.id, existing.id))
  } else {
    await db.insert(messageReactions).values({ id: randomUUID(), messageId: msgId, userId, emoji })
  }
  return c.json({ message: 'ok' })
})

// DELETE /:message_id/reactions/:emoji
messagesRouter.delete('/:message_id/reactions/:emoji', authMiddleware, async (c) => {
  const msgId = c.req.param('message_id')
  const userId = c.get('userId')
  const emoji = decodeURIComponent(c.req.param('emoji'))
  const db = createDb(c.env.DATABASE_URL)
  await db.delete(messageReactions).where(and(
    eq(messageReactions.messageId, msgId),
    eq(messageReactions.userId, userId),
    eq(messageReactions.emoji, emoji),
  ))
  return c.json({ message: 'ok' })
})

// POST /:message_id/pin
messagesRouter.post('/:message_id/pin', authMiddleware, async (c) => {
  const msgId = c.req.param('message_id')
  const db = createDb(c.env.DATABASE_URL)
  const [msg] = await db.select().from(messages).where(eq(messages.id, msgId)).limit(1)
  if (!msg) return c.json({ detail: 'Message not found' }, 404)
  await db.update(messages).set({ isPinned: true }).where(eq(messages.id, msgId))
  return c.json({ message: 'ok' })
})

// DELETE /:message_id/pin
messagesRouter.delete('/:message_id/pin', authMiddleware, async (c) => {
  const msgId = c.req.param('message_id')
  const db = createDb(c.env.DATABASE_URL)
  const [msg] = await db.select().from(messages).where(eq(messages.id, msgId)).limit(1)
  if (!msg) return c.json({ detail: 'Message not found' }, 404)
  await db.update(messages).set({ isPinned: false }).where(eq(messages.id, msgId))
  return c.json({ message: 'ok' })
})

// GET /:message_id/thread
messagesRouter.get('/:message_id/thread', authMiddleware, async (c) => {
  const msgId = c.req.param('message_id')
  const db = createDb(c.env.DATABASE_URL)
  const replies = await db.select().from(messages)
    .where(and(eq(messages.replyToId, msgId), ne(messages.status, 'deleted')))
    .orderBy(asc(messages.createdAt))
  const out = await buildMessagesWithMeta(replies, db)
  return c.json({ messages: out, hasMore: false })
})

// POST /:message_id/forward
messagesRouter.post('/:message_id/forward', authMiddleware, async (c) => {
  const msgId = c.req.param('message_id')
  const userId = c.get('userId')
  const { channelId } = await c.req.json()
  const db = createDb(c.env.DATABASE_URL)
  const [src] = await db.select().from(messages).where(eq(messages.id, msgId)).limit(1)
  if (!src) return c.json({ detail: 'Message not found' }, 404)

  const newId = randomUUID()
  await db.insert(messages).values({
    id: newId, channelId, authorId: userId,
    content: src.content, forwardFromId: src.id,
  })
  const [fwd] = await db.select().from(messages).where(eq(messages.id, newId)).limit(1)
  const [out] = await buildMessagesWithMeta([fwd], db)
  await broadcastToChannel(c.env, channelId, { type: 'new_message', message: out })
  return c.json(out, 201)
})

export default messagesRouter
