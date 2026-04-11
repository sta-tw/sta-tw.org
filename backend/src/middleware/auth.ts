import { createMiddleware } from 'hono/factory'
import { eq, and } from 'drizzle-orm'
import type { Env } from '../types'
import { decodeToken } from '../utils/jwt'
import { users } from '../db/schema'
import { createDb } from '../db'

export const authMiddleware = createMiddleware<{ Bindings: Env; Variables: { userId: string; db: ReturnType<typeof createDb> } }>(
  async (c, next) => {
    const auth = c.req.header('Authorization')
    if (!auth?.startsWith('Bearer ')) {
      return c.json({ detail: 'Could not validate credentials' }, 401)
    }
    const token = auth.slice(7)
    try {
      const payload = await decodeToken(token, 'access', c.env.JWT_SECRET)
      c.set('userId', payload.sub as string)
    } catch {
      return c.json({ detail: 'Could not validate credentials' }, 401)
    }
    await next()
  },
)

export function getClientIp(req: Request): string {
  return (
    req.headers.get('CF-Connecting-IP') ||
    req.headers.get('X-Real-IP') ||
    req.headers.get('X-Forwarded-For')?.split(',')[0]?.trim() ||
    ''
  )
}
