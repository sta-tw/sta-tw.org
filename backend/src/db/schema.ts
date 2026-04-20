import {
  sqliteTable as table, text, integer, real,
  uniqueIndex, index,
} from 'drizzle-orm/sqlite-core'
import { sql } from 'drizzle-orm'

// ── Users ─────────────────────────────────────────────────────

export const users = table('users', {
  id: text('id', { length: 36 }).primaryKey(),
  username: text('username', { length: 50 }).notNull().unique(),
  email: text('email', { length: 255 }).notNull().unique(),
  hashedPassword: text('hashed_password', { length: 255 }),
  displayName: text('display_name', { length: 100 }).notNull(),
  avatarUrl: text('avatar_url', { length: 500 }),
  bio: text('bio'),
  role: text('role').notNull().default('visitor'),
  managedSchoolCode: text('managed_school_code', { length: 30 }),
  managedDeptName: text('managed_dept_name', { length: 120 }),
  verificationStatus: text('verification_status').notNull().default('none'),
  reputationScore: integer('reputation_score').notNull().default(0),
  isEmailVerified: integer('is_email_verified').notNull().default(false),
  isActive: integer('is_active').notNull().default(true),
  googleId: text('google_id', { length: 255 }).unique(),
  discordId: text('discord_id', { length: 255 }).unique(),
  createdAt: integer('created_at').notNull().default(sql`(unixepoch())`),
  updatedAt: integer('updated_at').notNull().default(sql`(unixepoch())`),
}, (t) => [
  index('idx_users_username').on(t.username),
  index('idx_users_email').on(t.email),
])

export const userSessions = table('user_sessions', {
  id: text('id', { length: 36 }).primaryKey(),
  userId: text('user_id', { length: 36 }).notNull().references(() => users.id, { onDelete: 'cascade' }),
  refreshTokenHash: text('refresh_token_hash', { length: 255 }).notNull().unique(),
  deviceInfo: text('device_info', { length: 500 }),
  ipAddress: text('ip_address', { length: 50 }),
  isRevoked: integer('is_revoked').notNull().default(false),
  expiresAt: integer('expires_at').notNull(),
  createdAt: integer('created_at').notNull().default(sql`(unixepoch())`),
}, (t) => [
  index('idx_sessions_user_id').on(t.userId),
])

// ── Channels ──────────────────────────────────────────────────

export const channels = table('channels', {
  id: text('id', { length: 36 }).primaryKey(),
  name: text('name', { length: 100 }).notNull(),
  description: text('description'),
  type: text('type').notNull().default('text'),
  scopeType: text('scope_type').notNull().default('global'),
  schoolCode: text('school_code', { length: 30 }),
  deptCode: text('dept_code', { length: 30 }),
  parentId: text('parent_id', { length: 36 }),
  isArchived: integer('is_archived').notNull().default(false),
  cohortYear: integer('cohort_year'),
  audience: text('audience', { length: 20 }),
  orderIndex: integer('order_index').notNull().default(0),
  createdAt: integer('created_at').notNull().default(sql`(unixepoch())`),
})

// ── Messages ──────────────────────────────────────────────────

export const messages = table('messages', {
  id: text('id', { length: 36 }).primaryKey(),
  channelId: text('channel_id', { length: 36 }).notNull().references(() => channels.id, { onDelete: 'cascade' }),
  authorId: text('author_id', { length: 36 }).notNull().references(() => users.id, { onDelete: 'set null' }),
  content: text('content').notNull(),
  status: text('status').notNull().default('active'),
  isEdited: integer('is_edited').notNull().default(false),
  isPinned: integer('is_pinned').notNull().default(false),
  replyToId: text('reply_to_id', { length: 36 }),
  forwardFromId: text('forward_from_id', { length: 36 }),
  createdAt: integer('created_at').notNull().default(sql`(unixepoch())`),
  updatedAt: integer('updated_at'),
}, (t) => [
  index('idx_messages_channel_id').on(t.channelId),
  index('idx_messages_author_id').on(t.authorId),
])

export const messageReactions = table('message_reactions', {
  id: text('id', { length: 36 }).primaryKey(),
  messageId: text('message_id', { length: 36 }).notNull().references(() => messages.id, { onDelete: 'cascade' }),
  userId: text('user_id', { length: 36 }).notNull().references(() => users.id, { onDelete: 'cascade' }),
  emoji: text('emoji', { length: 10 }).notNull(),
  createdAt: integer('created_at').notNull().default(sql`(unixepoch())`),
}, (t) => [
  uniqueIndex('uq_reaction').on(t.messageId, t.userId, t.emoji),
  index('idx_reactions_message_id').on(t.messageId),
])

// ── Tickets ───────────────────────────────────────────────────

export const tickets = table('tickets', {
  id: text('id', { length: 36 }).primaryKey(),
  userId: text('user_id', { length: 36 }).notNull().references(() => users.id, { onDelete: 'cascade' }),
  category: text('category').notNull(),
  subject: text('subject', { length: 200 }).notNull(),
  status: text('status').notNull().default('open'),
  assigneeId: text('assignee_id', { length: 36 }).references(() => users.id, { onDelete: 'set null' }),
  createdAt: integer('created_at').notNull().default(sql`(unixepoch())`),
  updatedAt: integer('updated_at').notNull().default(sql`(unixepoch())`),
}, (t) => [
  index('idx_tickets_user_id').on(t.userId),
  index('idx_tickets_status').on(t.status),
])

export const ticketMessages = table('ticket_messages', {
  id: text('id', { length: 36 }).primaryKey(),
  ticketId: text('ticket_id', { length: 36 }).notNull().references(() => tickets.id, { onDelete: 'cascade' }),
  authorId: text('author_id', { length: 36 }).notNull().references(() => users.id, { onDelete: 'cascade' }),
  content: text('content').notNull(),
  isStaff: integer('is_staff').notNull().default(false),
  createdAt: integer('created_at').notNull().default(sql`(unixepoch())`),
}, (t) => [
  index('idx_ticket_messages_ticket_id').on(t.ticketId),
])

// ── Verification ──────────────────────────────────────────────

export const verificationRequests = table('verification_requests', {
  id: text('id', { length: 36 }).primaryKey(),
  userId: text('user_id', { length: 36 }).notNull().references(() => users.id, { onDelete: 'cascade' }),
  status: text('status').notNull().default('pending'),
  fileKey: text('file_key', { length: 500 }),
  fileHash: text('file_hash', { length: 64 }),
  fileKeys: text('file_keys'), // JSON array
  docType: text('doc_type', { length: 30 }),
  adminNote: text('admin_note'),
  submittedAt: integer('submitted_at').notNull().default(sql`(unixepoch())`),
  reviewedAt: integer('reviewed_at'),
  reviewedById: text('reviewed_by_id', { length: 36 }).references(() => users.id, { onDelete: 'set null' }),
}, (t) => [
  index('idx_verification_user_id').on(t.userId),
  index('idx_verification_status').on(t.status),
])

// ── Admissions ────────────────────────────────────────────────

export const admissionDocuments = table('admission_documents', {
  id: text('id', { length: 36 }).primaryKey(),
  sourceUrl: text('source_url', { length: 1000 }).notNull().unique(),
  title: text('title', { length: 300 }),
  schoolName: text('school_name', { length: 120 }),
  academicYear: integer('academic_year'),
  pageCount: integer('page_count').notNull().default(0),
  textPreview: text('text_preview'),
  keyDates: text('key_dates').$type<Array<Record<string, unknown>>>().notNull().default([]),
  schoolCode: text('school_code', { length: 30 }),
  createdById: text('created_by_id', { length: 36 }).references(() => users.id, { onDelete: 'set null' }),
  createdAt: integer('created_at').notNull().default(sql`(unixepoch())`),
  updatedAt: integer('updated_at').notNull().default(sql`(unixepoch())`),
}, (t) => [
  index('idx_admissions_school_name').on(t.schoolName),
  index('idx_admissions_academic_year').on(t.academicYear),
])

// ── Portfolio ─────────────────────────────────────────────────

export const portfolioDocuments = table('portfolio_documents', {
  id: text('id', { length: 36 }).primaryKey(),
  uploaderId: text('uploader_id', { length: 36 }).notNull().references(() => users.id, { onDelete: 'cascade' }),
  title: text('title', { length: 200 }).notNull(),
  description: text('description'),
  schoolName: text('school_name', { length: 120 }).notNull(),
  deptName: text('dept_name', { length: 120 }).notNull(),
  admissionYear: integer('admission_year').notNull(),
  category: text('category', { length: 50 }),
  applicantName: text('applicant_name', { length: 100 }),
  resultType: text('result_type', { length: 20 }),
  admittedRank: integer('admitted_rank'),
  totalAdmitted: integer('total_admitted'),
  waitlistRank: integer('waitlist_rank'),
  portfolioScore: real('portfolio_score'),
  fileKey: text('file_key', { length: 500 }).notNull(),
  fileName: text('file_name', { length: 300 }).notNull(),
  fileSize: integer('file_size').notNull().default(0),
  isApproved: integer('is_approved').notNull().default(false),
  viewCount: integer('view_count').notNull().default(0),
  longViewCount: integer('long_view_count').notNull().default(0),
  createdAt: integer('created_at').notNull().default(sql`(unixepoch())`),
  updatedAt: integer('updated_at').notNull().default(sql`(unixepoch())`),
}, (t) => [
  index('idx_portfolio_uploader').on(t.uploaderId),
  index('idx_portfolio_school').on(t.schoolName),
  index('idx_portfolio_dept').on(t.deptName),
  index('idx_portfolio_year').on(t.admissionYear),
])

export const portfolioScoringRules = table('portfolio_scoring_rules', {
  id: text('id', { length: 36 }).primaryKey(),
  schoolName: text('school_name', { length: 120 }).notNull(),
  schoolAbbr: text('school_abbr', { length: 30 }),
  deptName: text('dept_name', { length: 120 }).notNull(),
  score: real('score').notNull().default(0),
  note: text('note'),
  createdAt: integer('created_at').notNull().default(sql`(unixepoch())`),
  updatedAt: integer('updated_at').notNull().default(sql`(unixepoch())`),
})

export const portfolioViewLogs = table('portfolio_view_logs', {
  id: text('id', { length: 36 }).primaryKey(),
  docId: text('doc_id', { length: 36 }).notNull().references(() => portfolioDocuments.id, { onDelete: 'cascade' }),
  userId: text('user_id', { length: 36 }).notNull().references(() => users.id, { onDelete: 'cascade' }),
  longViewGranted: integer('long_view_granted').notNull().default(false),
  shareRewardGranted: integer('share_reward_granted').notNull().default(false),
  sessionGraceRemainingS: integer('session_grace_remaining_s').notNull().default(600),
  lastHeartbeatAt: integer('last_heartbeat_at'),
  totalEffectiveSeconds: integer('total_effective_seconds').notNull().default(0),
  reputationIntervalsGranted: integer('reputation_intervals_granted').notNull().default(0),
  createdAt: integer('created_at').notNull().default(sql`(unixepoch())`),
  updatedAt: integer('updated_at').notNull().default(sql`(unixepoch())`),
}, (t) => [
  uniqueIndex('uq_portfolio_view_doc_user').on(t.docId, t.userId),
])

export const portfolioSchoolOptions = table('portfolio_school_options', {
  id: text('id', { length: 36 }).primaryKey(),
  schoolName: text('school_name', { length: 120 }).notNull(),
  schoolCode: text('school_code', { length: 30 }),
  deptName: text('dept_name', { length: 120 }).notNull(),
  isActive: integer('is_active').notNull().default(true),
  sortOrder: integer('sort_order').notNull().default(0),
  createdAt: integer('created_at').notNull().default(sql`(unixepoch())`),
  updatedAt: integer('updated_at').notNull().default(sql`(unixepoch())`),
})

export const portfolioSchoolRequests = table('portfolio_school_requests', {
  id: text('id', { length: 36 }).primaryKey(),
  requesterId: text('requester_id', { length: 36 }).notNull().references(() => users.id, { onDelete: 'cascade' }),
  schoolName: text('school_name', { length: 120 }).notNull(),
  deptName: text('dept_name', { length: 120 }).notNull(),
  status: text('status', { length: 20 }).notNull().default('pending'),
  note: text('note'),
  reviewNote: text('review_note'),
  reviewedBy: text('reviewed_by', { length: 36 }).references(() => users.id, { onDelete: 'set null' }),
  createdAt: integer('created_at').notNull().default(sql`(unixepoch())`),
  updatedAt: integer('updated_at').notNull().default(sql`(unixepoch())`),
})

// ── Audit Log ─────────────────────────────────────────────────

export const auditLogs = table('audit_logs', {
  id: text('id', { length: 36 }).primaryKey(),
  correlationId: text('correlation_id', { length: 36 }).notNull(),
  actorId: text('actor_id', { length: 36 }).references(() => users.id, { onDelete: 'set null' }),
  action: text('action', { length: 100 }).notNull(),
  targetType: text('target_type', { length: 50 }),
  targetId: text('target_id', { length: 36 }),
  ip: text('ip', { length: 50 }).notNull(),
  userAgent: text('user_agent', { length: 500 }),
  metadata: text('metadata').$type<Record<string, unknown>>(),
  createdAt: integer('created_at').notNull().default(sql`(unixepoch())`),
}, (t) => [
  index('idx_audit_correlation').on(t.correlationId),
  index('idx_audit_actor').on(t.actorId),
  index('idx_audit_action').on(t.action),
])
