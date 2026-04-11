import { Hono } from 'hono'
import { eq, sql } from 'drizzle-orm'
import type { Env } from '../types'
import { createDb } from '../db'
import { portfolioDocuments, admissionDocuments, users } from '../db/schema'

const publicRouter = new Hono<{ Bindings: Env }>()

// GET /stats
publicRouter.get('/stats', async (c) => {
  const db = createDb(c.env.DATABASE_URL)
  const [{ portfolioCount }] = await db.select({ portfolioCount: sql<number>`count(*)` })
    .from(portfolioDocuments).where(eq(portfolioDocuments.isApproved, true))
  const [{ admissionCount }] = await db.select({ admissionCount: sql<number>`count(*)` })
    .from(admissionDocuments)
  const [{ studentCount }] = await db.select({ studentCount: sql<number>`count(*)` })
    .from(users).where(eq(users.isActive, true))

  return c.json({
    portfolioCount: Number(portfolioCount),
    admissionCount: Number(admissionCount),
    studentCount: Number(studentCount),
  })
})

export default publicRouter
