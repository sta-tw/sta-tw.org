CREATE TABLE `admission_documents` (
	`id` text(36) PRIMARY KEY NOT NULL,
	`source_url` text(1000) NOT NULL,
	`title` text(300),
	`school_name` text(120),
	`academic_year` integer,
	`page_count` integer DEFAULT 0 NOT NULL,
	`text_preview` text,
	`key_dates` text DEFAULT '[]' NOT NULL,
	`school_code` text(30),
	`created_by_id` text(36),
	`created_at` integer DEFAULT (unixepoch()) NOT NULL,
	`updated_at` integer DEFAULT (unixepoch()) NOT NULL,
	FOREIGN KEY (`created_by_id`) REFERENCES `users`(`id`) ON UPDATE no action ON DELETE set null
);
--> statement-breakpoint
CREATE UNIQUE INDEX `admission_documents_source_url_unique` ON `admission_documents` (`source_url`);--> statement-breakpoint
CREATE INDEX `idx_admissions_school_name` ON `admission_documents` (`school_name`);--> statement-breakpoint
CREATE INDEX `idx_admissions_academic_year` ON `admission_documents` (`academic_year`);--> statement-breakpoint
CREATE TABLE `audit_logs` (
	`id` text(36) PRIMARY KEY NOT NULL,
	`correlation_id` text(36) NOT NULL,
	`actor_id` text(36),
	`action` text(100) NOT NULL,
	`target_type` text(50),
	`target_id` text(36),
	`ip` text(50) NOT NULL,
	`user_agent` text(500),
	`metadata` text,
	`created_at` integer DEFAULT (unixepoch()) NOT NULL,
	FOREIGN KEY (`actor_id`) REFERENCES `users`(`id`) ON UPDATE no action ON DELETE set null
);
--> statement-breakpoint
CREATE INDEX `idx_audit_correlation` ON `audit_logs` (`correlation_id`);--> statement-breakpoint
CREATE INDEX `idx_audit_actor` ON `audit_logs` (`actor_id`);--> statement-breakpoint
CREATE INDEX `idx_audit_action` ON `audit_logs` (`action`);--> statement-breakpoint
CREATE TABLE `channels` (
	`id` text(36) PRIMARY KEY NOT NULL,
	`name` text(100) NOT NULL,
	`description` text,
	`type` text DEFAULT 'text' NOT NULL,
	`scope_type` text DEFAULT 'global' NOT NULL,
	`school_code` text(30),
	`dept_code` text(30),
	`parent_id` text(36),
	`is_archived` integer DEFAULT false NOT NULL,
	`cohort_year` integer,
	`audience` text(20),
	`order_index` integer DEFAULT 0 NOT NULL,
	`created_at` integer DEFAULT (unixepoch()) NOT NULL
);
--> statement-breakpoint
CREATE TABLE `message_reactions` (
	`id` text(36) PRIMARY KEY NOT NULL,
	`message_id` text(36) NOT NULL,
	`user_id` text(36) NOT NULL,
	`emoji` text(10) NOT NULL,
	`created_at` integer DEFAULT (unixepoch()) NOT NULL,
	FOREIGN KEY (`message_id`) REFERENCES `messages`(`id`) ON UPDATE no action ON DELETE cascade,
	FOREIGN KEY (`user_id`) REFERENCES `users`(`id`) ON UPDATE no action ON DELETE cascade
);
--> statement-breakpoint
CREATE UNIQUE INDEX `uq_reaction` ON `message_reactions` (`message_id`,`user_id`,`emoji`);--> statement-breakpoint
CREATE INDEX `idx_reactions_message_id` ON `message_reactions` (`message_id`);--> statement-breakpoint
CREATE TABLE `messages` (
	`id` text(36) PRIMARY KEY NOT NULL,
	`channel_id` text(36) NOT NULL,
	`author_id` text(36) NOT NULL,
	`content` text NOT NULL,
	`status` text DEFAULT 'active' NOT NULL,
	`is_edited` integer DEFAULT false NOT NULL,
	`is_pinned` integer DEFAULT false NOT NULL,
	`reply_to_id` text(36),
	`forward_from_id` text(36),
	`created_at` integer DEFAULT (unixepoch()) NOT NULL,
	`updated_at` integer,
	FOREIGN KEY (`channel_id`) REFERENCES `channels`(`id`) ON UPDATE no action ON DELETE cascade,
	FOREIGN KEY (`author_id`) REFERENCES `users`(`id`) ON UPDATE no action ON DELETE set null
);
--> statement-breakpoint
CREATE INDEX `idx_messages_channel_id` ON `messages` (`channel_id`);--> statement-breakpoint
CREATE INDEX `idx_messages_author_id` ON `messages` (`author_id`);--> statement-breakpoint
CREATE TABLE `portfolio_documents` (
	`id` text(36) PRIMARY KEY NOT NULL,
	`uploader_id` text(36) NOT NULL,
	`title` text(200) NOT NULL,
	`description` text,
	`school_name` text(120) NOT NULL,
	`dept_name` text(120) NOT NULL,
	`admission_year` integer NOT NULL,
	`category` text(50),
	`applicant_name` text(100),
	`result_type` text(20),
	`admitted_rank` integer,
	`total_admitted` integer,
	`waitlist_rank` integer,
	`portfolio_score` real,
	`file_key` text(500) NOT NULL,
	`file_name` text(300) NOT NULL,
	`file_size` integer DEFAULT 0 NOT NULL,
	`is_approved` integer DEFAULT false NOT NULL,
	`view_count` integer DEFAULT 0 NOT NULL,
	`long_view_count` integer DEFAULT 0 NOT NULL,
	`created_at` integer DEFAULT (unixepoch()) NOT NULL,
	`updated_at` integer DEFAULT (unixepoch()) NOT NULL,
	FOREIGN KEY (`uploader_id`) REFERENCES `users`(`id`) ON UPDATE no action ON DELETE cascade
);
--> statement-breakpoint
CREATE INDEX `idx_portfolio_uploader` ON `portfolio_documents` (`uploader_id`);--> statement-breakpoint
CREATE INDEX `idx_portfolio_school` ON `portfolio_documents` (`school_name`);--> statement-breakpoint
CREATE INDEX `idx_portfolio_dept` ON `portfolio_documents` (`dept_name`);--> statement-breakpoint
CREATE INDEX `idx_portfolio_year` ON `portfolio_documents` (`admission_year`);--> statement-breakpoint
CREATE TABLE `portfolio_school_options` (
	`id` text(36) PRIMARY KEY NOT NULL,
	`school_name` text(120) NOT NULL,
	`school_code` text(30),
	`dept_name` text(120) NOT NULL,
	`is_active` integer DEFAULT true NOT NULL,
	`sort_order` integer DEFAULT 0 NOT NULL,
	`created_at` integer DEFAULT (unixepoch()) NOT NULL,
	`updated_at` integer DEFAULT (unixepoch()) NOT NULL
);
--> statement-breakpoint
CREATE TABLE `portfolio_school_requests` (
	`id` text(36) PRIMARY KEY NOT NULL,
	`requester_id` text(36) NOT NULL,
	`school_name` text(120) NOT NULL,
	`dept_name` text(120) NOT NULL,
	`status` text(20) DEFAULT 'pending' NOT NULL,
	`note` text,
	`review_note` text,
	`reviewed_by` text(36),
	`created_at` integer DEFAULT (unixepoch()) NOT NULL,
	`updated_at` integer DEFAULT (unixepoch()) NOT NULL,
	FOREIGN KEY (`requester_id`) REFERENCES `users`(`id`) ON UPDATE no action ON DELETE cascade,
	FOREIGN KEY (`reviewed_by`) REFERENCES `users`(`id`) ON UPDATE no action ON DELETE set null
);
--> statement-breakpoint
CREATE TABLE `portfolio_scoring_rules` (
	`id` text(36) PRIMARY KEY NOT NULL,
	`school_name` text(120) NOT NULL,
	`school_abbr` text(30),
	`dept_name` text(120) NOT NULL,
	`score` real DEFAULT 0 NOT NULL,
	`note` text,
	`created_at` integer DEFAULT (unixepoch()) NOT NULL,
	`updated_at` integer DEFAULT (unixepoch()) NOT NULL
);
--> statement-breakpoint
CREATE TABLE `portfolio_view_logs` (
	`id` text(36) PRIMARY KEY NOT NULL,
	`doc_id` text(36) NOT NULL,
	`user_id` text(36) NOT NULL,
	`long_view_granted` integer DEFAULT false NOT NULL,
	`share_reward_granted` integer DEFAULT false NOT NULL,
	`session_grace_remaining_s` integer DEFAULT 600 NOT NULL,
	`last_heartbeat_at` integer,
	`total_effective_seconds` integer DEFAULT 0 NOT NULL,
	`reputation_intervals_granted` integer DEFAULT 0 NOT NULL,
	`created_at` integer DEFAULT (unixepoch()) NOT NULL,
	`updated_at` integer DEFAULT (unixepoch()) NOT NULL,
	FOREIGN KEY (`doc_id`) REFERENCES `portfolio_documents`(`id`) ON UPDATE no action ON DELETE cascade,
	FOREIGN KEY (`user_id`) REFERENCES `users`(`id`) ON UPDATE no action ON DELETE cascade
);
--> statement-breakpoint
CREATE UNIQUE INDEX `uq_portfolio_view_doc_user` ON `portfolio_view_logs` (`doc_id`,`user_id`);--> statement-breakpoint
CREATE TABLE `ticket_messages` (
	`id` text(36) PRIMARY KEY NOT NULL,
	`ticket_id` text(36) NOT NULL,
	`author_id` text(36) NOT NULL,
	`content` text NOT NULL,
	`is_staff` integer DEFAULT false NOT NULL,
	`created_at` integer DEFAULT (unixepoch()) NOT NULL,
	FOREIGN KEY (`ticket_id`) REFERENCES `tickets`(`id`) ON UPDATE no action ON DELETE cascade,
	FOREIGN KEY (`author_id`) REFERENCES `users`(`id`) ON UPDATE no action ON DELETE cascade
);
--> statement-breakpoint
CREATE INDEX `idx_ticket_messages_ticket_id` ON `ticket_messages` (`ticket_id`);--> statement-breakpoint
CREATE TABLE `tickets` (
	`id` text(36) PRIMARY KEY NOT NULL,
	`user_id` text(36) NOT NULL,
	`category` text NOT NULL,
	`subject` text(200) NOT NULL,
	`status` text DEFAULT 'open' NOT NULL,
	`assignee_id` text(36),
	`created_at` integer DEFAULT (unixepoch()) NOT NULL,
	`updated_at` integer DEFAULT (unixepoch()) NOT NULL,
	FOREIGN KEY (`user_id`) REFERENCES `users`(`id`) ON UPDATE no action ON DELETE cascade,
	FOREIGN KEY (`assignee_id`) REFERENCES `users`(`id`) ON UPDATE no action ON DELETE set null
);
--> statement-breakpoint
CREATE INDEX `idx_tickets_user_id` ON `tickets` (`user_id`);--> statement-breakpoint
CREATE INDEX `idx_tickets_status` ON `tickets` (`status`);--> statement-breakpoint
CREATE TABLE `user_sessions` (
	`id` text(36) PRIMARY KEY NOT NULL,
	`user_id` text(36) NOT NULL,
	`refresh_token_hash` text(255) NOT NULL,
	`device_info` text(500),
	`ip_address` text(50),
	`is_revoked` integer DEFAULT false NOT NULL,
	`expires_at` integer NOT NULL,
	`created_at` integer DEFAULT (unixepoch()) NOT NULL,
	FOREIGN KEY (`user_id`) REFERENCES `users`(`id`) ON UPDATE no action ON DELETE cascade
);
--> statement-breakpoint
CREATE UNIQUE INDEX `user_sessions_refresh_token_hash_unique` ON `user_sessions` (`refresh_token_hash`);--> statement-breakpoint
CREATE INDEX `idx_sessions_user_id` ON `user_sessions` (`user_id`);--> statement-breakpoint
CREATE TABLE `users` (
	`id` text(36) PRIMARY KEY NOT NULL,
	`username` text(50) NOT NULL,
	`email` text(255) NOT NULL,
	`hashed_password` text(255),
	`display_name` text(100) NOT NULL,
	`avatar_url` text(500),
	`bio` text,
	`role` text DEFAULT 'visitor' NOT NULL,
	`managed_school_code` text(30),
	`managed_dept_name` text(120),
	`verification_status` text DEFAULT 'none' NOT NULL,
	`reputation_score` integer DEFAULT 0 NOT NULL,
	`is_email_verified` integer DEFAULT false NOT NULL,
	`is_active` integer DEFAULT true NOT NULL,
	`google_id` text(255),
	`discord_id` text(255),
	`created_at` integer DEFAULT (unixepoch()) NOT NULL,
	`updated_at` integer DEFAULT (unixepoch()) NOT NULL
);
--> statement-breakpoint
CREATE UNIQUE INDEX `users_username_unique` ON `users` (`username`);--> statement-breakpoint
CREATE UNIQUE INDEX `users_email_unique` ON `users` (`email`);--> statement-breakpoint
CREATE UNIQUE INDEX `users_google_id_unique` ON `users` (`google_id`);--> statement-breakpoint
CREATE UNIQUE INDEX `users_discord_id_unique` ON `users` (`discord_id`);--> statement-breakpoint
CREATE INDEX `idx_users_username` ON `users` (`username`);--> statement-breakpoint
CREATE INDEX `idx_users_email` ON `users` (`email`);--> statement-breakpoint
CREATE TABLE `verification_requests` (
	`id` text(36) PRIMARY KEY NOT NULL,
	`user_id` text(36) NOT NULL,
	`status` text DEFAULT 'pending' NOT NULL,
	`file_key` text(500),
	`file_hash` text(64),
	`file_keys` text,
	`doc_type` text(30),
	`admin_note` text,
	`submitted_at` integer DEFAULT (unixepoch()) NOT NULL,
	`reviewed_at` integer,
	`reviewed_by_id` text(36),
	FOREIGN KEY (`user_id`) REFERENCES `users`(`id`) ON UPDATE no action ON DELETE cascade,
	FOREIGN KEY (`reviewed_by_id`) REFERENCES `users`(`id`) ON UPDATE no action ON DELETE set null
);
--> statement-breakpoint
CREATE INDEX `idx_verification_user_id` ON `verification_requests` (`user_id`);--> statement-breakpoint
CREATE INDEX `idx_verification_status` ON `verification_requests` (`status`);