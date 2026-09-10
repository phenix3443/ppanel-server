-- Compensating migration for the October 2025 MySQL renumbering; PostgreSQL
-- deployments postdate it, so this is a no-op kept for parity between the two
-- migration sets.
ALTER TABLE "ads" ADD COLUMN IF NOT EXISTS "description" VARCHAR(255) DEFAULT '';
