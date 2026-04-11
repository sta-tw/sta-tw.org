import {
  pgTable, pgEnum, text, varchar, boolean, integer, real, timestamp,
  uniqueIndex, index, json,
} from 'drizzle-orm/pg-core'

// ── Enums ─────────────────────────────────────────────────────

export const userRoleEnum = pgEnum('userrole', [
  'visitor', 'special_student', 'prospective_student', 'student',
  'senior', 'dept_moderator', 'school_moderator', 'admin', 'developer', 'super_admin',
])

export const verificationStatusEnum = pgEnum('verificationstatus', [
  'none', 'pending', 'approved', 'rejected',
])

export const messageStatusEnum = pgEnum('messagestatus', [
  'active', 'withdrawn', 'deleted',
])

export const channelTypeEnum = pgEnum('channeltype', [
  'text', 'announcement',
])

export const channelScopeTypeEnum = pgEnum('channelscopetype', [
  'global', 'school', 'dept',
])

export const ticketStatusEnum = pgEnum('ticketstatus', [
  'open', 'processing', 'pending', 'closed',
])

export const ticketCategoryEnum = pgEnum('ticketcategory', [
  'account', 'verification', 'content', 'bug', 'other',
])

export const requestStatusEnum = pgEnum('requeststatus', [
  'pending', 'approved', 'rejected',
])

// ── Users ─────────────────────────────────────────────────────

export const users = pgTable('users', {
  id: varchar('id', { length: 36 }).primaryKey(),
  username: varchar('username', { length: 50 }).notNull().unique(),
  email: varchar('email', { length: 255 }).notNull().unique(),
  hashedPassword: varchar('hashed_password', { length: 255 }),
  displayName: varchar('display_name', { length: 100 }).notNull(),
  avatarUrl: varchar('avatar_url', { length: 500 }),
  bio: text('bio'),
  role: userRoleEnum('role').notNull().default('visitor'),
  managedSchoolCode: varchar('managed_school_code', { length: 30 }),
  managedDeptName: varchar('managed_dept_name', { length: 120 }),
  verificationStatus: verificationStatusEnum('verification_status').notNull().default('none'),
  reputationScore: integer('reputation_score').notNull().default(0),
  isEmailVerified: boolean('is_email_verified').notNull().default(false),
  isActive: boolean('is_active').notNull().default(true),
  googleId: varchar('google_id', { length: 255 }).unique(),
  discordId: varchar('discord_id', { length: 255 }).unique(),
  createdAt: timestamp('created_at', { withTimezone: true }).notNull().defaultNow(),
  updatedAt: timestamp('updated_at', { withTimezone: true }).notNull().defaultNow(),
}, (t) => [
  index('idx_users_username').on(t.username),
  index('idx_users_email').on(t.email),
])

export const userSessions = pgTable('user_sessions', {
  id: varchar('id', { length: 36 }).primaryKey(),
  userId: varchar('user_id', { length: 36 }).notNull().references(() => users.id, { onDelete: 'cascade' }),
  refreshTokenHash: varchar('refresh_token_hash', { length: 255 }).notNull().unique(),
  deviceInfo: varchar('device_info', { length: 500 }),
  ipAddress: varchar('ip_address', { length: 50 }),
  isRevoked: boolean('is_revoked').notNull().default(false),
  expiresAt: timestamp('expires_at', { withTimezone: true }).notNull(),
  createdAt: timestamp('created_at', { withTimezone: true }).notNull().defaultNow(),
}, (t) => [
  index('idx_sessions_user_id').on(t.userId),
])

// ── Channels ──────────────────────────────────────────────────

export const channels = pgTable('channels', {
  id: varchar('id', { length: 36 }).primaryKey(),
  name: varchar('name', { length: 100 }).notNull(),
  description: text('description'),
  type: channelTypeEnum('type').notNull().default('text'),
  scopeType: channelScopeTypeEnum('scope_type').notNull().default('global'),
  schoolCode: varchar('school_code', { length: 30 }),
  deptCode: varchar('dept_code', { length: 30 }),
  parentId: varchar('parent_id', { length: 36 }),
  isArchived: boolean('is_archived').notNull().default(false),
  cohortYear: integer('cohort_year'),
  audience: varchar('audience', { length: 20 }),
  orderIndex: integer('order_index').notNull().default(0),
  createdAt: timestamp('created_at', { withTimezone: true }).notNull().defaultNow(),
})

// ── Messages ──────────────────────────────────────────────────

export const messages = pgTable('messages', {
  id: varchar('id', { length: 36 }).primaryKey(),
  channelId: varchar('channel_id', { length: 36 }).notNull().references(() => channels.id, { onDelete: 'cascade' }),
  authorId: varchar('author_id', { length: 36 }).notNull().references(() => users.id, { onDelete: 'set null' }),
  content: text('content').notNull(),
  status: messageStatusEnum('status').notNull().default('active'),
  isEdited: boolean('is_edited').notNull().default(false),
  isPinned: boolean('is_pinned').notNull().default(false),
  replyToId: varchar('reply_to_id', { length: 36 }),
  forwardFromId: varchar('forward_from_id', { length: 36 }),
  createdAt: timestamp('created_at', { withTimezone: true }).notNull().defaultNow(),
  updatedAt: timestamp('updated_at', { withTimezone: true }),
}, (t) => [
  index('idx_messages_channel_id').on(t.channelId),
  index('idx_messages_author_id').on(t.authorId),
])

export const messageReactions = pgTable('message_reactions', {
  id: varchar('id', { length: 36 }).primaryKey(),
  messageId: varchar('message_id', { length: 36 }).notNull().references(() => messages.id, { onDelete: 'cascade' }),
  userId: varchar('user_id', { length: 36 }).notNull().references(() => users.id, { onDelete: 'cascade' }),
  emoji: varchar('emoji', { length: 10 }).notNull(),
  createdAt: timestamp('created_at', { withTimezone: true }).notNull().defaultNow(),
}, (t) => [
  uniqueIndex('uq_reaction').on(t.messageId, t.userId, t.emoji),
  index('idx_reactions_message_id').on(t.messageId),
])

// ── Tickets ───────────────────────────────────────────────────

export const tickets = pgTable('tickets', {
  id: varchar('id', { length: 36 }).primaryKey(),
  userId: varchar('user_id', { length: 36 }).notNull().references(() => users.id, { onDelete: 'cascade' }),
  category: ticketCategoryEnum('category').notNull(),
  subject: varchar('subject', { length: 200 }).notNull(),
  status: ticketStatusEnum('status').notNull().default('open'),
  assigneeId: varchar('assignee_id', { length: 36 }).references(() => users.id, { onDelete: 'set null' }),
  createdAt: timestamp('created_at', { withTimezone: true }).notNull().defaultNow(),
  updatedAt: timestamp('updated_at', { withTimezone: true }).notNull().defaultNow(),
}, (t) => [
  index('idx_tickets_user_id').on(t.userId),
  index('idx_tickets_status').on(t.status),
])

export const ticketMessages = pgTable('ticket_messages', {
  id: varchar('id', { length: 36 }).primaryKey(),
  ticketId: varchar('ticket_id', { length: 36 }).notNull().references(() => tickets.id, { onDelete: 'cascade' }),
  authorId: varchar('author_id', { length: 36 }).notNull().references(() => users.id, { onDelete: 'cascade' }),
  content: text('content').notNull(),
  isStaff: boolean('is_staff').notNull().default(false),
  createdAt: timestamp('created_at', { withTimezone: true }).notNull().defaultNow(),
}, (t) => [
  index('idx_ticket_messages_ticket_id').on(t.ticketId),
])

// ── Verification ──────────────────────────────────────────────

export const verificationRequests = pgTable('verification_requests', {
  id: varchar('id', { length: 36 }).primaryKey(),
  userId: varchar('user_id', { length: 36 }).notNull().references(() => users.id, { onDelete: 'cascade' }),
  status: requestStatusEnum('status').notNull().default('pending'),
  fileKey: varchar('file_key', { length: 500 }),
  fileHash: varchar('file_hash', { length: 64 }),
  fileKeys: text('file_keys'), // JSON array
  docType: varchar('doc_type', { length: 30 }),
  adminNote: text('admin_note'),
  submittedAt: timestamp('submitted_at', { withTimezone: true }).notNull().defaultNow(),
  reviewedAt: timestamp('reviewed_at', { withTimezone: true }),
  reviewedById: varchar('reviewed_by_id', { length: 36 }).references(() => users.id, { onDelete: 'set null' }),
}, (t) => [
  index('idx_verification_user_id').on(t.userId),
  index('idx_verification_status').on(t.status),
])

// ── Admissions ────────────────────────────────────────────────

export const admissionDocuments = pgTable('admission_documents', {
  id: varchar('id', { length: 36 }).primaryKey(),
  sourceUrl: varchar('source_url', { length: 1000 }).notNull().unique(),
  title: varchar('title', { length: 300 }),
  schoolName: varchar('school_name', { length: 120 }),
  academicYear: integer('academic_year'),
  pageCount: integer('page_count').notNull().default(0),
  textPreview: text('text_preview'),
  keyDates: json('key_dates').$type<Array<Record<string, unknown>>>().notNull().default([]),
  schoolCode: varchar('school_code', { length: 30 }),
  createdById: varchar('created_by_id', { length: 36 }).references(() => users.id, { onDelete: 'set null' }),
  createdAt: timestamp('created_at', { withTimezone: true }).notNull().defaultNow(),
  updatedAt: timestamp('updated_at', { withTimezone: true }).notNull().defaultNow(),
}, (t) => [
  index('idx_admissions_school_name').on(t.schoolName),
  index('idx_admissions_academic_year').on(t.academicYear),
])

// ── Portfolio ─────────────────────────────────────────────────

export const portfolioDocuments = pgTable('portfolio_documents', {
  id: varchar('id', { length: 36 }).primaryKey(),
  uploaderId: varchar('uploader_id', { length: 36 }).notNull().references(() => users.id, { onDelete: 'cascade' }),
  title: varchar('title', { length: 200 }).notNull(),
  description: text('description'),
  schoolName: varchar('school_name', { length: 120 }).notNull(),
  deptName: varchar('dept_name', { length: 120 }).notNull(),
  admissionYear: integer('admission_year').notNull(),
  category: varchar('category', { length: 50 }),
  applicantName: varchar('applicant_name', { length: 100 }),
  resultType: varchar('result_type', { length: 20 }),
  admittedRank: integer('admitted_rank'),
  totalAdmitted: integer('total_admitted'),
  waitlistRank: integer('waitlist_rank'),
  portfolioScore: real('portfolio_score'),
  fileKey: varchar('file_key', { length: 500 }).notNull(),
  fileName: varchar('file_name', { length: 300 }).notNull(),
  fileSize: integer('file_size').notNull().default(0),
  isApproved: boolean('is_approved').notNull().default(false),
  viewCount: integer('view_count').notNull().default(0),
  longViewCount: integer('long_view_count').notNull().default(0),
  createdAt: timestamp('created_at', { withTimezone: true }).notNull().defaultNow(),
  updatedAt: timestamp('updated_at', { withTimezone: true }).notNull().defaultNow(),
}, (t) => [
  index('idx_portfolio_uploader').on(t.uploaderId),
  index('idx_portfolio_school').on(t.schoolName),
  index('idx_portfolio_dept').on(t.deptName),
  index('idx_portfolio_year').on(t.admissionYear),
])

export const portfolioScoringRules = pgTable('portfolio_scoring_rules', {
  id: varchar('id', { length: 36 }).primaryKey(),
  schoolName: varchar('school_name', { length: 120 }).notNull(),
  schoolAbbr: varchar('school_abbr', { length: 30 }),
  deptName: varchar('dept_name', { length: 120 }).notNull(),
  score: real('score').notNull().default(0),
  note: text('note'),
  createdAt: timestamp('created_at', { withTimezone: true }).notNull().defaultNow(),
  updatedAt: timestamp('updated_at', { withTimezone: true }).notNull().defaultNow(),
})

export const portfolioViewLogs = pgTable('portfolio_view_logs', {
  id: varchar('id', { length: 36 }).primaryKey(),
  docId: varchar('doc_id', { length: 36 }).notNull().references(() => portfolioDocuments.id, { onDelete: 'cascade' }),
  userId: varchar('user_id', { length: 36 }).notNull().references(() => users.id, { onDelete: 'cascade' }),
  longViewGranted: boolean('long_view_granted').notNull().default(false),
  shareRewardGranted: boolean('share_reward_granted').notNull().default(false),
  sessionGraceRemainingS: integer('session_grace_remaining_s').notNull().default(600),
  lastHeartbeatAt: timestamp('last_heartbeat_at', { withTimezone: true }),
  totalEffectiveSeconds: integer('total_effective_seconds').notNull().default(0),
  reputationIntervalsGranted: integer('reputation_intervals_granted').notNull().default(0),
  createdAt: timestamp('created_at', { withTimezone: true }).notNull().defaultNow(),
  updatedAt: timestamp('updated_at', { withTimezone: true }).notNull().defaultNow(),
}, (t) => [
  uniqueIndex('uq_portfolio_view_doc_user').on(t.docId, t.userId),
])

export const portfolioSchoolOptions = pgTable('portfolio_school_options', {
  id: varchar('id', { length: 36 }).primaryKey(),
  schoolName: varchar('school_name', { length: 120 }).notNull(),
  schoolCode: varchar('school_code', { length: 30 }),
  deptName: varchar('dept_name', { length: 120 }).notNull(),
  isActive: boolean('is_active').notNull().default(true),
  sortOrder: integer('sort_order').notNull().default(0),
  createdAt: timestamp('created_at', { withTimezone: true }).notNull().defaultNow(),
  updatedAt: timestamp('updated_at', { withTimezone: true }).notNull().defaultNow(),
})

export const portfolioSchoolRequests = pgTable('portfolio_school_requests', {
  id: varchar('id', { length: 36 }).primaryKey(),
  requesterId: varchar('requester_id', { length: 36 }).notNull().references(() => users.id, { onDelete: 'cascade' }),
  schoolName: varchar('school_name', { length: 120 }).notNull(),
  deptName: varchar('dept_name', { length: 120 }).notNull(),
  status: varchar('status', { length: 20 }).notNull().default('pending'),
  note: text('note'),
  reviewNote: text('review_note'),
  reviewedBy: varchar('reviewed_by', { length: 36 }).references(() => users.id, { onDelete: 'set null' }),
  createdAt: timestamp('created_at', { withTimezone: true }).notNull().defaultNow(),
  updatedAt: timestamp('updated_at', { withTimezone: true }).notNull().defaultNow(),
})

// ── Audit Log ─────────────────────────────────────────────────

export const auditLogs = pgTable('audit_logs', {
  id: varchar('id', { length: 36 }).primaryKey(),
  correlationId: varchar('correlation_id', { length: 36 }).notNull(),
  actorId: varchar('actor_id', { length: 36 }).references(() => users.id, { onDelete: 'set null' }),
  action: varchar('action', { length: 100 }).notNull(),
  targetType: varchar('target_type', { length: 50 }),
  targetId: varchar('target_id', { length: 36 }),
  ip: varchar('ip', { length: 50 }).notNull(),
  userAgent: varchar('user_agent', { length: 500 }),
  metadata: json('metadata').$type<Record<string, unknown>>(),
  createdAt: timestamp('created_at', { withTimezone: true }).notNull().defaultNow(),
}, (t) => [
  index('idx_audit_correlation').on(t.correlationId),
  index('idx_audit_actor').on(t.actorId),
  index('idx_audit_action').on(t.action),
])
