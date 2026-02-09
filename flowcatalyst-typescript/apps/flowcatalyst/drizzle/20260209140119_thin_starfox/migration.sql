CREATE TABLE "dispatch_jobs" (
	"id" varchar(13) PRIMARY KEY,
	"external_id" varchar(100),
	"source" varchar(500),
	"kind" varchar(20) DEFAULT 'EVENT' NOT NULL,
	"code" varchar(200) NOT NULL,
	"subject" varchar(500),
	"event_id" varchar(13),
	"correlation_id" varchar(100),
	"metadata" jsonb DEFAULT '[]',
	"target_url" varchar(500) NOT NULL,
	"protocol" varchar(30) DEFAULT 'HTTP_WEBHOOK' NOT NULL,
	"payload" text,
	"payload_content_type" varchar(100) DEFAULT 'application/json',
	"data_only" boolean DEFAULT true NOT NULL,
	"service_account_id" varchar(17),
	"client_id" varchar(17),
	"subscription_id" varchar(17),
	"mode" varchar(30) DEFAULT 'IMMEDIATE' NOT NULL,
	"dispatch_pool_id" varchar(17),
	"message_group" varchar(200),
	"sequence" integer DEFAULT 99 NOT NULL,
	"timeout_seconds" integer DEFAULT 30 NOT NULL,
	"schema_id" varchar(17),
	"status" varchar(20) DEFAULT 'PENDING' NOT NULL,
	"max_retries" integer DEFAULT 3 NOT NULL,
	"retry_strategy" varchar(50) DEFAULT 'exponential',
	"scheduled_for" timestamp with time zone,
	"expires_at" timestamp with time zone,
	"attempt_count" integer DEFAULT 0 NOT NULL,
	"last_attempt_at" timestamp with time zone,
	"completed_at" timestamp with time zone,
	"duration_millis" bigint,
	"last_error" text,
	"idempotency_key" varchar(100),
	"created_at" timestamp with time zone DEFAULT now() NOT NULL,
	"updated_at" timestamp with time zone DEFAULT now() NOT NULL
);
--> statement-breakpoint
CREATE INDEX "idx_dispatch_jobs_status" ON "dispatch_jobs" ("status");--> statement-breakpoint
CREATE INDEX "idx_dispatch_jobs_client_id" ON "dispatch_jobs" ("client_id");--> statement-breakpoint
CREATE INDEX "idx_dispatch_jobs_message_group" ON "dispatch_jobs" ("message_group");--> statement-breakpoint
CREATE INDEX "idx_dispatch_jobs_subscription_id" ON "dispatch_jobs" ("subscription_id");--> statement-breakpoint
CREATE INDEX "idx_dispatch_jobs_created_at" ON "dispatch_jobs" ("created_at");--> statement-breakpoint
CREATE INDEX "idx_dispatch_jobs_scheduled_for" ON "dispatch_jobs" ("scheduled_for");