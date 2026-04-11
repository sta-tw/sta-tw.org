import type { DB } from './db'
import type { users } from './db/schema'
import type { InferSelectModel } from 'drizzle-orm'

export type Env = {
  // Secrets
  DATABASE_URL: string
  JWT_SECRET: string
  R2_ACCOUNT_ID: string
  R2_ACCESS_KEY_ID: string
  R2_SECRET_ACCESS_KEY: string
  MEILI_MASTER_KEY: string
  RESEND_API_KEY: string
  GOOGLE_CLIENT_ID: string
  GOOGLE_CLIENT_SECRET: string
  DISCORD_CLIENT_ID: string
  DISCORD_CLIENT_SECRET: string
  // Vars
  FRONTEND_URL: string
  ACCESS_TOKEN_EXPIRE_MINUTES: string
  REFRESH_TOKEN_EXPIRE_DAYS: string
  R2_BUCKET_NAME: string
  MEILI_URL: string
  EMAIL_FROM: string
  // Durable Objects
  CHAT_ROOM: DurableObjectNamespace
}

export type User = InferSelectModel<typeof users>

export type AppContext = {
  db: DB
  user?: User
  correlationId: string
}
