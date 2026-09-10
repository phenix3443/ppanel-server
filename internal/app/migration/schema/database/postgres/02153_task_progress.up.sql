ALTER TABLE "task"
    ADD COLUMN IF NOT EXISTS "daily_date" varchar(10) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS "daily_sent" bigint NOT NULL DEFAULT 0;
