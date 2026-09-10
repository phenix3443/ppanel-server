CREATE TABLE "task_error" (
    "id" bigserial PRIMARY KEY,
    "task_id" bigint NOT NULL,
    "position" bigint NOT NULL,
    "target" varchar(320) NOT NULL DEFAULT '',
    "error" text,
    "occurred_at" bigint NOT NULL DEFAULT 0,
    "created_at" timestamptz,
    CONSTRAINT "uk_task_error_position" UNIQUE ("task_id", "position")
);

CREATE INDEX "idx_task_error_task_id" ON "task_error" ("task_id", "id");
