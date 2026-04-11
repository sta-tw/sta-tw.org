import { eq, inArray, and, ne, sql } from 'drizzle-orm'
import type { DB } from '../db'
import { messages, messageReactions, users } from '../db/schema'

export async function buildMessagesWithMeta(
  msgList: Array<typeof messages.$inferSelect>,
  db: DB,
) {
  if (msgList.length === 0) return []

  const msgIds = msgList.map(m => m.id)
  const authorIds = [...new Set(msgList.map(m => m.authorId))]

  // Batch load authors
  const authorList = authorIds.length > 0
    ? await db.select().from(users).where(inArray(users.id, authorIds))
    : []
  const authorsMap = Object.fromEntries(authorList.map(u => [u.id, u]))

  // Batch load reply_to messages + their authors
  const replyToIds = [...new Set(msgList.map(m => m.replyToId).filter((id): id is string => id !== null))]
  const replyMessages: Record<string, { content: string; authorDisplayName: string }> = {}
  if (replyToIds.length > 0) {
    const replyRows = await db.select().from(messages).where(inArray(messages.id, replyToIds))
    const replyAuthorIds = [...new Set(replyRows.map(r => r.authorId))]
    const replyAuthors = replyAuthorIds.length > 0
      ? await db.select().from(users).where(inArray(users.id, replyAuthorIds))
      : []
    const replyAuthorsMap = Object.fromEntries(replyAuthors.map(u => [u.id, u]))
    for (const r of replyRows) {
      replyMessages[r.id] = {
        content: r.content,
        authorDisplayName: replyAuthorsMap[r.authorId]?.displayName ?? '',
      }
    }
  }

  // Batch load reactions
  const reactionRows = msgIds.length > 0
    ? await db.select().from(messageReactions).where(inArray(messageReactions.messageId, msgIds))
    : []
  const reactionsMap: Record<string, Array<typeof messageReactions.$inferSelect>> = {}
  for (const r of reactionRows) {
    if (!reactionsMap[r.messageId]) reactionsMap[r.messageId] = []
    reactionsMap[r.messageId].push(r)
  }

  // Batch load thread counts
  const threadRows = msgIds.length > 0
    ? await db.select({
        replyToId: messages.replyToId,
        cnt: sql<number>`count(*)`,
      }).from(messages)
        .where(and(inArray(messages.replyToId, msgIds), ne(messages.status, 'deleted')))
        .groupBy(messages.replyToId)
    : []
  const threadCountMap: Record<string, number> = {}
  for (const r of threadRows) {
    if (r.replyToId) threadCountMap[r.replyToId] = Number(r.cnt)
  }

  return msgList.map(msg => {
    const author = authorsMap[msg.authorId]
    const reactions = groupReactions(reactionsMap[msg.id] ?? [])
    const replyTo = msg.replyToId && replyMessages[msg.replyToId]
      ? { id: msg.replyToId, ...replyMessages[msg.replyToId] }
      : null

    return {
      id: msg.id,
      channelId: msg.channelId,
      authorId: msg.authorId,
      author: author ? {
        id: author.id, username: author.username,
        displayName: author.displayName, role: author.role, avatarUrl: author.avatarUrl,
      } : null,
      content: msg.content,
      status: msg.status,
      createdAt: msg.createdAt.toISOString(),
      updatedAt: msg.updatedAt?.toISOString() ?? null,
      isEdited: msg.isEdited,
      isPinned: msg.isPinned,
      replyTo,
      reactions,
      threadCount: threadCountMap[msg.id] ?? 0,
      forwardFromId: msg.forwardFromId,
    }
  })
}

function groupReactions(reactions: Array<typeof messageReactions.$inferSelect>) {
  const groups: Record<string, string[]> = {}
  for (const r of reactions) {
    if (!groups[r.emoji]) groups[r.emoji] = []
    groups[r.emoji].push(r.userId)
  }
  return Object.entries(groups).map(([emoji, userIds]) => ({
    emoji, count: userIds.length, userIds,
  }))
}
