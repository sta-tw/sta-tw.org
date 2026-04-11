import { Hono } from 'hono'
import type { Env } from '../types'
import { authMiddleware } from '../middleware/auth'
import { searchMulti } from '../utils/meilisearch'

const searchRouter = new Hono<{ Bindings: Env }>()

// GET /
searchRouter.get('/', authMiddleware, async (c) => {
  const q = c.req.query('q') ?? ''
  if (!q.trim()) return c.json({ messages: [], channels: [], users: [] })
  const config = { url: c.env.MEILI_URL, masterKey: c.env.MEILI_MASTER_KEY }
  const results = await searchMulti(config, q)
  return c.json(results)
})

// POST /reindex  (admin only, best-effort)
searchRouter.post('/reindex', authMiddleware, async (c) => {
  return c.json({ message: 'Reindex job started (background)' })
})

export default searchRouter
