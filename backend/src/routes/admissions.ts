import { Hono } from 'hono'
import { eq, desc, ilike, and } from 'drizzle-orm'
import type { Env } from '../types'
import { createDb } from '../db'
import { admissionDocuments, users } from '../db/schema'
import { authMiddleware } from '../middleware/auth'
import { randomUUID } from '../utils/uuid'
import { generatePresignedGetUrl } from '../utils/storage'

const admissionsRouter = new Hono<{ Bindings: Env }>()

function admissionOut(doc: typeof admissionDocuments.$inferSelect) {
  return {
    id: doc.id, sourceUrl: doc.sourceUrl, title: doc.title,
    schoolName: doc.schoolName, academicYear: doc.academicYear,
    pageCount: doc.pageCount, textPreview: doc.textPreview,
    keyDates: doc.keyDates, schoolCode: doc.schoolCode,
    createdAt: doc.createdAt.toISOString(), updatedAt: doc.updatedAt.toISOString(),
  }
}

// GET /
admissionsRouter.get('/', authMiddleware, async (c) => {
  const schoolName = c.req.query('school_name')
  const academicYear = c.req.query('academic_year')
  const db = createDb(c.env.DB)

  let docs
  if (schoolName || academicYear) {
    const conditions = []
    if (schoolName) conditions.push(ilike(admissionDocuments.schoolName, `%${schoolName}%`))
    if (academicYear) conditions.push(eq(admissionDocuments.academicYear, parseInt(academicYear)))
    docs = await db.select().from(admissionDocuments).where(and(...conditions)).orderBy(desc(admissionDocuments.createdAt))
  } else {
    docs = await db.select().from(admissionDocuments).orderBy(desc(admissionDocuments.createdAt))
  }
  return c.json(docs.map(admissionOut))
})

// GET /:document_id
admissionsRouter.get('/:document_id', authMiddleware, async (c) => {
  const id = c.req.param('document_id')
  const db = createDb(c.env.DB)
  const [doc] = await db.select().from(admissionDocuments).where(eq(admissionDocuments.id, id)).limit(1)
  if (!doc) return c.json({ detail: 'Document not found' }, 404)
  return c.json(admissionOut(doc))
})

// GET /:document_id/pdf  — redirect to R2 presigned URL
admissionsRouter.get('/:document_id/pdf', authMiddleware, async (c) => {
  const id = c.req.param('document_id')
  const db = createDb(c.env.DB)
  const [doc] = await db.select().from(admissionDocuments).where(eq(admissionDocuments.id, id)).limit(1)
  if (!doc) return c.json({ detail: 'Document not found' }, 404)
  // sourceUrl is the direct PDF URL
  return c.redirect(doc.sourceUrl)
})

// POST /import-url
admissionsRouter.post('/import-url', authMiddleware, async (c) => {
  const userId = c.get('userId')
  const { url, schoolName, academicYear, title, schoolCode } = await c.req.json()
  if (!url) return c.json({ detail: 'url is required' }, 422)

  const db = createDb(c.env.DB)
  const [existing] = await db.select().from(admissionDocuments)
    .where(eq(admissionDocuments.sourceUrl, url)).limit(1)
  if (existing) return c.json(admissionOut(existing))

  const id = randomUUID()
  await db.insert(admissionDocuments).values({
    id, sourceUrl: url,
    title: title ?? null, schoolName: schoolName ?? null,
    academicYear: academicYear ? parseInt(academicYear) : null,
    schoolCode: schoolCode ?? null, createdById: userId,
    keyDates: [],
  })
  const [doc] = await db.select().from(admissionDocuments).where(eq(admissionDocuments.id, id)).limit(1)
  return c.json(admissionOut(doc), 201)
})

// POST /import-file  — store metadata, PDF stored in R2
admissionsRouter.post('/import-file', authMiddleware, async (c) => {
  const userId = c.get('userId')
  const { fileUrl, schoolName, academicYear, title, schoolCode } = await c.req.json()
  if (!fileUrl) return c.json({ detail: 'fileUrl is required' }, 422)

  const db = createDb(c.env.DB)
  const id = randomUUID()
  await db.insert(admissionDocuments).values({
    id, sourceUrl: fileUrl, title: title ?? null,
    schoolName: schoolName ?? null,
    academicYear: academicYear ? parseInt(academicYear) : null,
    schoolCode: schoolCode ?? null, createdById: userId,
    keyDates: [],
  })
  const [doc] = await db.select().from(admissionDocuments).where(eq(admissionDocuments.id, id)).limit(1)
  return c.json(admissionOut(doc), 201)
})

// DELETE /:document_id
admissionsRouter.delete('/:document_id', authMiddleware, async (c) => {
  const id = c.req.param('document_id')
  const db = createDb(c.env.DB)
  const [doc] = await db.select().from(admissionDocuments).where(eq(admissionDocuments.id, id)).limit(1)
  if (!doc) return c.json({ detail: 'Document not found' }, 404)
  await db.delete(admissionDocuments).where(eq(admissionDocuments.id, id))
  return c.body(null, 204)
})

export default admissionsRouter
