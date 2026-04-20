import { Hono } from 'hono'
import { cors } from 'hono/cors'
import { logger } from 'hono/logger'
import type { Env } from './types'
import { ChatRoom } from './durable-objects/chat-room'
import { createDb } from './db'
import { users } from './db/schema'
import { decodeToken } from './utils/jwt'
import { eq } from 'drizzle-orm'

import authRouter from './routes/auth'
import usersRouter from './routes/users'
import channelsRouter from './routes/channels'
import messagesRouter from './routes/messages'
import ticketsRouter from './routes/tickets'
import searchRouter from './routes/search'
import verificationRouter from './routes/verification'
import admissionsRouter from './routes/admissions'
import portfolioRouter from './routes/portfolio'
import adminRouter from './routes/admin'
import publicRouter from './routes/public'
import openapiRouter from './routes/openapi'

export { ChatRoom }

const app = new Hono<{ Bindings: Env }>()

// ── Middleware ─────────────────────────────────────────────────

app.use('*', async (c, next) => {
  const origin = c.env.FRONTEND_URL || '*'
  return cors({
    origin: [
      origin,
      'http://localhost:3000',
      'http://localhost:12003',
      'https://hoppscotch.io',
      'https://hoppscotch.com',
    ],
    allowHeaders: ['Content-Type', 'Authorization'],
    allowMethods: ['GET', 'POST', 'PUT', 'PATCH', 'DELETE', 'OPTIONS'],
    credentials: true,
  })(c, next)
})

// ── WebSocket endpoint ─────────────────────────────────────────

app.get('/ws', async (c) => {
  const upgradeHeader = c.req.header('Upgrade')
  if (!upgradeHeader || upgradeHeader.toLowerCase() !== 'websocket') {
    return c.json({ detail: 'Expected WebSocket upgrade' }, 426)
  }

  const token = c.req.query('token')
  if (!token) return c.json({ detail: 'Missing token' }, 401)

  try {
    await decodeToken(token, 'access', c.env.JWT_SECRET)
  } catch {
    return c.json({ detail: 'Invalid token' }, 401)
  }

  const doId = c.env.CHAT_ROOM.idFromName('global')
  const stub = c.env.CHAT_ROOM.get(doId)
  return stub.fetch(c.req.raw)
})

// ── API Routes ─────────────────────────────────────────────────

app.route('/api/v1/auth', authRouter)
app.route('/api/v1/users', usersRouter)
app.route('/api/v1/channels', channelsRouter)
app.route('/api/v1/messages', messagesRouter)
app.route('/api/v1/tickets', ticketsRouter)
app.route('/api/v1/search', searchRouter)
app.route('/api/v1/verification', verificationRouter)
app.route('/api/v1/admissions', admissionsRouter)
app.route('/api/v1/portfolio', portfolioRouter)
app.route('/api/v1/admin', adminRouter)
app.route('/api/v1/public', publicRouter)
app.route('/', openapiRouter)

// ── Health check ───────────────────────────────────────────────

app.get('/health', (c) => c.json({ status: 'ok' }))

// ── 404 ───────────────────────────────────────────────────────

app.notFound((c) => c.json({ detail: 'Not found' }, 404))

app.onError((err, c) => {
  console.error(err)
  return c.json({ detail: err.message || 'Internal server error' }, 500)
})

export default app
